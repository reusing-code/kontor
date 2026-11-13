package ledger

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/reusing-code/kontor/backend/internal/llm"
)

// errMessageParse is returned by a message loader that fetched the message but
// could not parse it; the parser's own error can quote header text.
var errMessageParse = errors.New("could not parse message")

var (
	ErrEmailFolderMissing = errors.New("choose a folder to scan")
	ErrEmailLLMDisabled   = errors.New("the LLM is not configured")
	ErrEmailScanRunning   = errors.New("a scan of this account is already running")
	ErrIMAP               = errors.New("imap")
)

func imapError(err error) error {
	return fmt.Errorf("%w: %w", ErrIMAP, err)
}

type UploadedMessage struct {
	Filename  string
	Reader    io.Reader
	MessageID string
}

type EmailService struct {
	store     *Store
	extractor llm.Extractor
	logger    *slog.Logger
	scanLocks sync.Map // uuid.UUID -> *sync.Mutex
	// maxExtractionFailures is the number of unusable answers after which a
	// message is ignored.
	maxExtractionFailures int
}

func NewEmailService(s *Store, extractor llm.Extractor, logger *slog.Logger) *EmailService {
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}
	return &EmailService{store: s, extractor: extractor, logger: logger, maxExtractionFailures: defaultMaxExtractionFailures}
}

// SetMaxExtractionFailures overrides the default limit; values below 1 are ignored.
func (s *EmailService) SetMaxExtractionFailures(n int) {
	if n >= 1 {
		s.maxExtractionFailures = n
	}
}

// lockAccount allows one scan per account at a time, so a manual and a
// scheduled scan cannot extract the same messages twice.
func (s *EmailService) lockAccount(id uuid.UUID) (unlock func(), ok bool) {
	lock, _ := s.scanLocks.LoadOrStore(id, &sync.Mutex{})
	mu := lock.(*sync.Mutex)
	if !mu.TryLock() {
		return nil, false
	}
	return mu.Unlock, true
}

// CanScan reports why a scan cannot start, before anything is downloaded.
func (s *EmailService) CanScan(account LedgerEmailAccount, needFolder bool) error {
	if needFolder && account.Folder == "" {
		return ErrEmailFolderMissing
	}
	if !s.extractor.Enabled() {
		return ErrEmailLLMDisabled
	}
	return nil
}

func (s *EmailService) ScanUploadedMessages(ctx context.Context, userID string, account LedgerEmailAccount, files []UploadedMessage) (LedgerEmailScanResult, error) {
	result := newScanResult()
	if err := s.CanScan(account, false); err != nil {
		return result, err
	}
	unlock, ok := s.lockAccount(account.ID)
	if !ok {
		return result, ErrEmailScanRunning
	}
	defer unlock()
	msgs := make([]emailMessage, 0, len(files))
	for _, file := range files {
		result.EmailsScanned++
		raw, err := io.ReadAll(file.Reader)
		if err != nil {
			warn(&result, file.Filename, "could not read file")
			continue
		}
		parsed, err := parseMessage(bytes.NewReader(raw))
		if err != nil {
			warn(&result, file.Filename, "could not parse message")
			continue
		}
		if parsed.MessageID == "" {
			parsed.MessageID = strings.TrimSpace(file.MessageID)
		}
		msgs = append(msgs, emailMessage{
			Label:  file.Filename,
			Key:    emailDedupKey(parsed.MessageID, headerBlock(raw)),
			Header: parsed,
			Load:   func() (ParsedEmail, error) { return parsed, nil },
		})
	}
	err := s.processMessages(ctx, userID, account, msgs, false, &result)
	return result, err
}

func (s *EmailService) ScanMailbox(ctx context.Context, userID string, account LedgerEmailAccount, password string) (LedgerEmailScanResult, error) {
	result := newScanResult()
	if err := s.CanScan(account, true); err != nil {
		return result, err
	}
	unlock, ok := s.lockAccount(account.ID)
	if !ok {
		return result, ErrEmailScanRunning
	}
	defer unlock()
	client, err := newIMAPClient(account.IMAPHost, account.IMAPPort, account.UseTLS)
	if err != nil {
		return result, imapError(err)
	}
	defer client.Close()
	if err := client.Login(account.Username, password); err != nil {
		return result, imapError(err)
	}
	defer client.Logout()
	if err := client.SelectFolder(account.Folder); err != nil {
		return result, imapError(err)
	}
	searchSince := account.ScanSince
	if account.LastScanAt != nil {
		lastScan := account.LastScanAt.Add(-48 * time.Hour).Format("2006-01-02")
		if searchSince == "" || lastScan > searchSince {
			searchSince = lastScan
		}
	}
	uids, err := client.SearchSince(searchSince)
	if err != nil {
		return result, imapError(err)
	}
	if len(uids) == 0 {
		return result, nil
	}
	if len(uids) > maxFetchedMessages {
		result.Warnings = append(result.Warnings, fmt.Sprintf("scan limited to the newest %d matching messages in the selected folder", maxFetchedMessages))
		uids = uids[len(uids)-maxFetchedMessages:]
	}
	msgs := make([]emailMessage, 0, len(uids))
	for i := len(uids) - 1; i >= 0; i-- {
		if ctx.Err() != nil {
			result.Partial = true
			return result, ctx.Err()
		}
		uid := uids[i]
		label := fmt.Sprintf("uid %d", uid)
		uidKey := fmt.Sprintf("uid:%d", uid)
		ignored, err := s.store.HasLedgerEmailSeen(ctx, userID, account.ID, uidKey)
		if err != nil {
			return result, err
		}
		if ignored {
			continue
		}
		raw, err := client.FetchHeader(uid)
		switch {
		case errors.Is(err, errMessageTooLarge):
			warn(&result, label, "message is too large")
			continue
		case errors.Is(err, errIMAPRejected):
			if err := s.handleRejectedFetch(ctx, userID, account, uidKey, label, "could not read message headers", &result); err != nil {
				return result, err
			}
			continue
		case err != nil:
			// The connection is unusable after an I/O error.
			result.Partial = true
			return result, imapError(err)
		}
		result.EmailsScanned++
		header, err := parseHeader(raw)
		if err != nil {
			warn(&result, label, "could not read message headers")
			continue
		}
		msgs = append(msgs, emailMessage{
			Label:  label,
			Key:    emailDedupKey(header.MessageID, headerBlock(raw)),
			Header: header,
			Load: func() (ParsedEmail, error) {
				full, err := client.FetchMessage(uid)
				if err != nil {
					return ParsedEmail{}, err
				}
				parsed, err := parseMessage(bytes.NewReader(full))
				if err != nil {
					return ParsedEmail{}, errMessageParse
				}
				return parsed, nil
			},
		})
	}
	err = s.processMessages(ctx, userID, account, msgs, account.PreClassify, &result)
	return result, err
}

func (s *EmailService) TestConnection(account LedgerEmailAccount, password string) error {
	client, err := newIMAPClient(account.IMAPHost, account.IMAPPort, account.UseTLS)
	if err != nil {
		return err
	}
	defer client.Close()
	if err := client.Login(account.Username, password); err != nil {
		return err
	}
	defer client.Logout()
	if account.Folder == "" {
		return nil
	}
	return client.SelectFolder(account.Folder)
}

// ListOrderCandidates returns link candidates for an email order within its
// charge window (derived from the order); afterDays > 0 overrides the window's upper bound.
func (s *EmailService) ListOrderCandidates(ctx context.Context, userID string, orderID uuid.UUID, afterDays int) (LedgerEmailOrderCandidatesResult, error) {
	order, err := s.store.GetLedgerEmailOrder(ctx, userID, orderID)
	if err != nil {
		return LedgerEmailOrderCandidatesResult{}, err
	}
	window := matchRuleForOrder(order).Window
	if afterDays > 0 {
		window.AfterDays = afterDays
	}
	return s.store.ListLedgerEmailOrderCandidates(ctx, userID, order, window)
}

func (s *EmailService) autoLinkOrder(ctx context.Context, userID string, rule matchRule, order *LedgerEmailOrder) (bool, []string, error) {
	if rule.Pattern == nil {
		return false, nil, nil
	}
	transactions, err := s.store.ListLedgerTransactionsFiltered(ctx, userID, LedgerTransactionListOptions{Limit: 500})
	if err != nil {
		return false, nil, err
	}
	candidates := make([]LedgerTransaction, 0)
	for _, txn := range transactions.Items {
		if txn.AmountMinor >= 0 {
			continue
		}
		if len(txn.EmailOrderIDs) > 0 {
			continue
		}
		if !rule.Pattern.MatchString(strings.TrimSpace(txn.CounterpartyName + "\n" + txn.Purpose)) {
			continue
		}
		if !rule.Window.Contains(txn.BookingDate, order.OrderDate) {
			continue
		}
		candidates = append(candidates, txn)
	}
	if len(candidates) == 0 {
		return false, nil, nil
	}
	target := order.TotalMinor
	if target < 0 {
		target = -target
	}
	exact := make([]LedgerTransaction, 0)
	for _, candidate := range candidates {
		if -candidate.AmountMinor == target {
			exact = append(exact, candidate)
		}
	}
	if len(exact) == 1 {
		order.LinkedTransactionIDs = []uuid.UUID{exact[0].ID}
		order.MatchStatus = LedgerEmailOrderStatusMatched
		return true, nil, nil
	}
	if len(exact) > 1 {
		return false, []string{"multiple exact transaction matches"}, nil
	}
	combo := findTransactionCombination(candidates, target)
	if len(combo) > 0 {
		ids := make([]uuid.UUID, 0, len(combo))
		for _, txn := range combo {
			ids = append(ids, txn.ID)
		}
		order.LinkedTransactionIDs = ids
		order.MatchStatus = LedgerEmailOrderStatusMatched
		return true, []string{"linked split payment"}, nil
	}
	return false, nil, nil
}

func findTransactionCombination(candidates []LedgerTransaction, target int64) []LedgerTransaction {
	ordered := append([]LedgerTransaction(nil), candidates...)
	sort.Slice(ordered, func(i, j int) bool {
		return ordered[i].BookingDate > ordered[j].BookingDate
	})
	var best []LedgerTransaction
	var walk func(start int, sum int64, picked []LedgerTransaction)
	walk = func(start int, sum int64, picked []LedgerTransaction) {
		if best != nil {
			return
		}
		if sum == target && len(picked) > 1 {
			best = append([]LedgerTransaction(nil), picked...)
			return
		}
		if sum > target || len(picked) >= 4 {
			return
		}
		for i := start; i < len(ordered); i++ {
			amount := -ordered[i].AmountMinor
			if amount <= 0 {
				continue
			}
			walk(i+1, sum+amount, append(picked, ordered[i]))
		}
	}
	walk(0, 0, nil)
	return best
}
