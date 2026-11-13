package ledger

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"slices"
	"strings"
	"time"
	"unicode"

	"github.com/google/uuid"
	"github.com/reusing-code/kontor/backend/internal/llm"
	"github.com/reusing-code/kontor/backend/internal/storage"
)

const (
	maxFetchedMessages        = 250
	maxConsecutiveLLMFailures = 3
)

var errTransientLLM = errors.New("transient LLM failure")

// emailMessage is one candidate message of a scan, from an upload or IMAP.
type emailMessage struct {
	Label  string                      // "uid 42" or the file name; only used in warnings and logs
	Key    string                      // emailDedupKey
	Header ParsedEmail                 // MessageID, From, Subject, Date only
	Load   func() (ParsedEmail, error) // full message, called only when extraction is needed
}

func newScanResult() LedgerEmailScanResult {
	return LedgerEmailScanResult{Warnings: []string{}, Orders: []LedgerEmailOrder{}}
}

// warn adds a per-message warning. The reason comes from a closed set of fixed
// texts so no email data ends up in a warning.
func warn(result *LedgerEmailScanResult, label, reason string) {
	result.Warnings = append(result.Warnings, label+": "+reason)
}

// maxMessageIDBytes bounds the Message-ID part of a storage key: the header is
// sender-controlled and Badger rejects keys above 65000 bytes.
const maxMessageIDBytes = 200

// emailDedupKey identifies a message across scans and sources. It does not
// depend on model output: the Message-ID (verbatim when short and printable,
// else its hash), or a hash of the normalised header block when there is none.
func emailDedupKey(messageID string, header []byte) string {
	id := strings.TrimSpace(messageID)
	switch {
	case id == "":
		normalised := strings.TrimSpace(strings.ReplaceAll(string(header), "\r\n", "\n"))
		sum := sha256.Sum256([]byte(normalised))
		return "nomid:" + hex.EncodeToString(sum[:16])
	case len(id) > maxMessageIDBytes || strings.ContainsFunc(id, unicode.IsControl):
		sum := sha256.Sum256([]byte(id))
		return "mid:" + hex.EncodeToString(sum[:16])
	}
	return id
}

// processMessages is the one path that turns candidate messages into orders:
// known messages are dropped before any LLM call, the rest is extracted,
// validated, auto-linked and stored.
func (s *EmailService) processMessages(ctx context.Context, userID string, account LedgerEmailAccount, msgs []emailMessage, preClassify bool, result *LedgerEmailScanResult) error {
	todo, err := s.dropKnownMessages(ctx, userID, account, msgs, result)
	if err != nil {
		return err
	}
	s.publishScan(account.ID, result)
	batchSize := max(len(todo), 1)
	if preClassify {
		batchSize = preClassifyBatchSize
	}
	failures := 0
	// transient reports whether the scan should stop after another transient failure.
	transient := func() bool {
		failures++
		if failures < maxConsecutiveLLMFailures {
			return false
		}
		result.Partial = true
		result.Warnings = append(result.Warnings, "scan stopped after repeated LLM failures")
		return true
	}
	for batch := range slices.Chunk(todo, batchSize) {
		if ctx.Err() != nil {
			result.Partial = true
			return ctx.Err()
		}
		if preClassify {
			batch, err = s.preClassify(ctx, userID, account, batch, result)
			s.publishScan(account.ID, result)
			if errors.Is(err, errTransientLLM) {
				if transient() {
					return nil
				}
				continue
			}
			if err != nil {
				return s.interrupted(ctx, result, err)
			}
			failures = 0
		}
		for _, m := range batch {
			if ctx.Err() != nil {
				result.Partial = true
				return ctx.Err()
			}
			err := s.processMessage(ctx, userID, account, m, result)
			s.publishScan(account.ID, result)
			switch {
			case errors.Is(err, errTransientLLM):
				if transient() {
					return nil
				}
			case err != nil:
				return s.interrupted(ctx, result, err)
			default:
				failures = 0
			}
		}
	}
	return nil
}

// interrupted marks the result partial when the scan context ended.
func (s *EmailService) interrupted(ctx context.Context, result *LedgerEmailScanResult, err error) error {
	if ctx.Err() != nil {
		result.Partial = true
	}
	return err
}

// dropKnownMessages removes messages that already produced an order or were
// processed as non-orders (and repeated keys within the batch).
func (s *EmailService) dropKnownMessages(ctx context.Context, userID string, account LedgerEmailAccount, msgs []emailMessage, result *LedgerEmailScanResult) ([]emailMessage, error) {
	todo := make([]emailMessage, 0, len(msgs))
	queued := make(map[string]bool, len(msgs))
	for _, m := range msgs {
		if queued[m.Key] {
			continue
		}
		queued[m.Key] = true
		existing, err := s.store.GetLedgerEmailOrderByMessageID(ctx, userID, m.Key)
		if err == nil {
			result.OrdersFound++
			result.Orders = append(result.Orders, existing)
			continue
		}
		if !errors.Is(err, storage.ErrNotFound) {
			return nil, err
		}
		seen, err := s.store.HasLedgerEmailSeen(ctx, userID, account.ID, m.Key)
		if err != nil {
			return nil, err
		}
		if !seen {
			todo = append(todo, m)
		}
	}
	return todo, nil
}

// processMessage returns errTransientLLM for transient LLM failures, any other
// error is fatal for the scan.
func (s *EmailService) processMessage(ctx context.Context, userID string, account LedgerEmailAccount, m emailMessage, result *LedgerEmailScanResult) error {
	parsed, err := m.Load()
	if errors.Is(err, errMessageTooLarge) {
		warn(result, m.Label, "message is too large")
		return s.store.MarkLedgerEmailsSeen(ctx, userID, account.ID, EmailSeenTooLarge, m.Key)
	}
	switch {
	case errors.Is(err, errIMAPRejected):
		return s.handleRejectedFetch(ctx, userID, account, m.Key, m.Label, "could not read message", result)
	case errors.Is(err, errMessageParse):
		warn(result, m.Label, "could not parse message")
		return nil
	case err != nil:
		// An I/O error leaves the IMAP connection unusable.
		return imapError(err)
	}
	ext, err := s.extractOrder(ctx, parsed)
	switch {
	case errors.Is(err, llm.ErrDisabled):
		return ErrEmailLLMDisabled
	case err != nil && ctx.Err() != nil:
		return ctx.Err()
	case err != nil && isTransientLLMError(err):
		class := "request"
		if errors.Is(err, context.DeadlineExceeded) {
			class = "timeout"
			warn(result, m.Label, "LLM request timed out")
		} else {
			warn(result, m.Label, "LLM request failed")
		}
		s.logger.Warn("email extraction failed", "message", m.Label, "class", class)
		result.Skipped++
		return errTransientLLM
	case err != nil && isUnusableLLMOutput(err):
		s.logger.Warn("email extraction failed", "message", m.Label, "class", "unusable")
		return s.handleFailedExtraction(ctx, userID, account, m, "LLM output was unusable", result)
	case err != nil:
		warn(result, m.Label, "LLM request was rejected")
		s.logger.Warn("email extraction failed", "message", m.Label, "class", "rejected")
		return nil
	}
	if !ext.IsOrder {
		return s.store.MarkLedgerEmailsSeen(ctx, userID, account.ID, EmailSeenNotOrder, m.Key)
	}
	extracted, err := validateExtraction(ext, parsed.Date)
	if err != nil {
		s.logger.Warn("email extraction failed", "message", m.Label, "class", "incomplete")
		return s.handleFailedExtraction(ctx, userID, account, m, "extraction result was incomplete", result)
	}
	return s.storeOrder(ctx, userID, account, m, parsed, extracted, result)
}

func (s *EmailService) storeOrder(ctx context.Context, userID string, account LedgerEmailAccount, m emailMessage, parsed ParsedEmail, e extractedOrder, result *LedgerEmailScanResult) error {
	now := time.Now().UTC()
	order := LedgerEmailOrder{
		ID:              uuid.New(),
		EmailAccountID:  account.ID,
		ImporterID:      LedgerEmailImporterLLM,
		Merchant:        e.Merchant,
		PaymentMethod:   e.PaymentMethod,
		ExternalOrderID: e.ExternalOrderID,
		OrderDate:       e.OrderDate,
		TotalMinor:      e.TotalMinor,
		Currency:        e.Currency,
		Items:           e.Items,
		EmailMessageID:  m.Key,
		EmailSubject:    parsed.Subject,
		MatchStatus:     LedgerEmailOrderStatusUnmatched,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	linked, linkWarnings, err := s.autoLinkOrder(ctx, userID, matchRuleForOrder(order), &order)
	if err != nil {
		return err
	}
	for _, w := range linkWarnings {
		warn(result, m.Label, w)
	}
	if err := s.store.CreateScannedLedgerEmailOrder(ctx, userID, order); err != nil {
		if errors.Is(err, storage.ErrConflict) {
			return s.useExistingOrder(ctx, userID, m, result)
		}
		return err
	}
	if linked {
		if _, err := s.store.LinkLedgerEmailOrder(ctx, userID, order.ID, LedgerEmailOrderLinkInput{TransactionIDs: order.LinkedTransactionIDs}); err != nil {
			return err
		}
		result.OrdersLinked++
	}
	stored, err := s.store.GetLedgerEmailOrder(ctx, userID, order.ID)
	if err != nil {
		return err
	}
	result.OrdersFound++
	result.OrdersNew++
	result.Orders = append(result.Orders, stored)
	return nil
}

// maxFetchRejections is how often the server may refuse to deliver the same
// message before it is ignored. Until then a refusal makes the scan
// incomplete; afterwards a permanently unreadable message (corrupt, no
// permission) cannot keep every scan incomplete.
const maxFetchRejections = 3

func (s *EmailService) handleRejectedFetch(ctx context.Context, userID string, account LedgerEmailAccount, key, label, reason string, result *LedgerEmailScanResult) error {
	count, err := s.store.RecordLedgerEmailRejection(ctx, userID, account.ID, key)
	if err != nil {
		return err
	}
	if count >= maxFetchRejections {
		warn(result, label, "message could not be read repeatedly and is ignored from now on")
		return s.store.MarkLedgerEmailsSeen(ctx, userID, account.ID, EmailSeenUnreadable, key)
	}
	warn(result, label, reason)
	result.Skipped++
	return nil
}

// defaultMaxExtractionFailures is how often the model may give no usable
// answer for the same message before it is ignored.
const defaultMaxExtractionFailures = 3

// handleFailedExtraction counts an answer that is unusable for this message.
// Below the limit the message stays unmarked and is retried by the next scan.
func (s *EmailService) handleFailedExtraction(ctx context.Context, userID string, account LedgerEmailAccount, m emailMessage, reason string, result *LedgerEmailScanResult) error {
	count, err := s.store.RecordLedgerEmailExtractionFailure(ctx, userID, account.ID, m.Key)
	if err != nil {
		return err
	}
	if count >= s.maxExtractionFailures {
		warn(result, m.Label, "message could not be extracted repeatedly and is ignored from now on")
		return s.store.MarkLedgerEmailsSeen(ctx, userID, account.ID, EmailSeenUnusable, m.Key)
	}
	warn(result, m.Label, reason)
	return nil
}

// useExistingOrder handles a message that another scan stored in the meantime.
func (s *EmailService) useExistingOrder(ctx context.Context, userID string, m emailMessage, result *LedgerEmailScanResult) error {
	existing, err := s.store.GetLedgerEmailOrderByMessageID(ctx, userID, m.Key)
	if err != nil {
		return err
	}
	result.OrdersFound++
	result.Orders = append(result.Orders, existing)
	return nil
}

// isTransientLLMError separates failures that say nothing about the message
// (timeouts, network, rate limits, server and auth errors) from answers that
// are unusable for this message. Callers check the scan context first.
func isTransientLLMError(err error) bool {
	if isUnusableLLMOutput(err) {
		return false
	}
	if apiErr, ok := errors.AsType[*llm.APIError](err); ok {
		switch status := apiErr.StatusCode; {
		case status == 408, status == 429, status == 401, status == 403, status >= 500:
			return true
		default:
			return false
		}
	}
	return true
}

// isUnusableLLMOutput reports a model answer that cannot be used for this
// message. Only these count towards the per-message failure limit: a rejected
// request (4xx) may be a configuration error that says nothing about the message.
func isUnusableLLMOutput(err error) bool {
	return errors.Is(err, llm.ErrInvalidOutput) || errors.Is(err, llm.ErrRefused) || errors.Is(err, llm.ErrTruncated)
}
