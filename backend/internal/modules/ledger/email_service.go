package ledger

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/reusing-code/kontor/backend/internal/storage"

	"github.com/google/uuid"
)

type Importer interface {
	Info() LedgerEmailImporterInfo
	Matches(from string, subject string) bool
	Parse(parsed ParsedEmail) ([]ParsedOrder, []string, error)
	CounterpartyPattern() *regexp.Regexp
	// ChargeWindow is the range around an order's orderDate in which the
	// payment for it is expected to be booked.
	ChargeWindow() ChargeWindow
}

type ParsedOrder struct {
	ExternalOrderID string
	OrderDate       string
	TotalMinor      int64
	Currency        string
	Items           []LedgerEmailOrderItem
}

var (
	ErrEmailFolderMissing = errors.New("choose a folder to scan")
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
	logger    *slog.Logger
	importers []Importer
}

func NewEmailService(s *Store, logger *slog.Logger) *EmailService {
	return &EmailService{
		store:  s,
		logger: logger,
		importers: []Importer{
			newAmazonDEImporter(),
			newPayPalDEImporter(),
		},
	}
}

func (s *EmailService) ListImporters() []LedgerEmailImporterInfo {
	items := make([]LedgerEmailImporterInfo, 0, len(s.importers))
	for _, importer := range s.importers {
		items = append(items, importer.Info())
	}
	return items
}

func (s *EmailService) ScanUploadedMessages(ctx context.Context, userID string, account LedgerEmailAccount, files []UploadedMessage) (LedgerEmailScanResult, error) {
	result := LedgerEmailScanResult{Warnings: []string{}, Orders: []LedgerEmailOrder{}}
	for _, file := range files {
		result.EmailsScanned++
		parsed, err := parseMessage(file.Reader)
		if err != nil {
			result.Warnings = append(result.Warnings, fmt.Sprintf("%s: %v", file.Filename, err))
			continue
		}
		if parsed.MessageID == "" {
			parsed.MessageID = strings.TrimSpace(file.MessageID)
		}
		importer := s.matchImporter(parsed.From, parsed.Subject)
		if importer == nil {
			result.Warnings = append(result.Warnings, fmt.Sprintf("%s: no importer matched sender/subject", file.Filename))
			continue
		}
		orders, warnings, err := importer.Parse(parsed)
		result.Warnings = append(result.Warnings, warnings...)
		if err != nil {
			result.Warnings = append(result.Warnings, fmt.Sprintf("%s: %v", file.Filename, err))
			continue
		}
		result.OrdersFound += len(orders)
		for _, parsedOrder := range orders {
			select {
			case <-ctx.Done():
				return result, ctx.Err()
			default:
			}
			messageID := parsed.MessageID
			if messageID == "" {
				messageID = file.Filename + ":" + parsedOrder.ExternalOrderID + ":" + parsedOrder.OrderDate
			}
			existing, err := s.store.GetLedgerEmailOrderByMessageID(ctx, userID, messageID)
			if err == nil {
				result.Orders = append(result.Orders, existing)
				continue
			}
			if err != nil && err != storage.ErrNotFound {
				return result, err
			}
			now := time.Now().UTC()
			order := LedgerEmailOrder{
				ID:              uuid.New(),
				EmailAccountID:  account.ID,
				ImporterID:      importer.Info().ID,
				ExternalOrderID: parsedOrder.ExternalOrderID,
				OrderDate:       parsedOrder.OrderDate,
				TotalMinor:      parsedOrder.TotalMinor,
				Currency:        parsedOrder.Currency,
				Items:           parsedOrder.Items,
				EmailMessageID:  messageID,
				EmailSubject:    parsed.Subject,
				MatchStatus:     LedgerEmailOrderStatusUnmatched,
				CreatedAt:       now,
				UpdatedAt:       now,
			}
			linked, warnings, err := s.autoLinkOrder(ctx, userID, matchRule{Pattern: importer.CounterpartyPattern(), Window: importer.ChargeWindow()}, &order)
			result.Warnings = append(result.Warnings, warnings...)
			if err != nil {
				return result, err
			}
			if err := s.store.CreateLedgerEmailOrder(ctx, userID, order); err != nil {
				return result, err
			}
			if linked {
				if _, err := s.store.LinkLedgerEmailOrder(ctx, userID, order.ID, LedgerEmailOrderLinkInput{TransactionIDs: order.LinkedTransactionIDs}); err != nil {
					return result, err
				}
				result.OrdersLinked++
			}
			result.OrdersNew++
			stored, err := s.store.GetLedgerEmailOrder(ctx, userID, order.ID)
			if err != nil {
				return result, err
			}
			result.Orders = append(result.Orders, stored)
		}
	}
	return result, nil
}

func (s *EmailService) ScanMailbox(ctx context.Context, userID string, account LedgerEmailAccount, password string) (LedgerEmailScanResult, error) {
	result := LedgerEmailScanResult{Warnings: []string{}, Orders: []LedgerEmailOrder{}}
	if account.Folder == "" {
		return result, ErrEmailFolderMissing
	}
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
	const maxFetchedMessages = 250
	if len(uids) > maxFetchedMessages {
		result.Warnings = append(result.Warnings, fmt.Sprintf("scan limited to the newest %d matching messages in the selected folder", maxFetchedMessages))
		uids = uids[len(uids)-maxFetchedMessages:]
	}
	for i := len(uids) - 1; i >= 0; i-- {
		select {
		case <-ctx.Done():
			return result, ctx.Err()
		default:
		}
		uid := uids[i]
		rawMessage, err := client.FetchMessage(uid)
		if err != nil {
			result.Warnings = append(result.Warnings, fmt.Sprintf("uid %d: %v", uid, err))
			continue
		}
		result.EmailsScanned++
		parsed, err := parseMessage(bytes.NewReader(rawMessage))
		if err != nil {
			result.Warnings = append(result.Warnings, fmt.Sprintf("uid %d: %v", uid, err))
			continue
		}
		messageResult, err := s.processParsedEmail(ctx, userID, account, parsed, fmt.Sprintf("uid-%d", uid))
		if err != nil {
			return result, err
		}
		result.OrdersFound += messageResult.OrdersFound
		result.OrdersNew += messageResult.OrdersNew
		result.OrdersLinked += messageResult.OrdersLinked
		result.Warnings = append(result.Warnings, messageResult.Warnings...)
		result.Orders = append(result.Orders, messageResult.Orders...)
	}
	return result, nil
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

func (s *EmailService) processParsedEmail(ctx context.Context, userID string, account LedgerEmailAccount, parsed ParsedEmail, sourceLabel string) (LedgerEmailScanResult, error) {
	result := LedgerEmailScanResult{Warnings: []string{}, Orders: []LedgerEmailOrder{}}
	importer := s.matchImporter(parsed.From, parsed.Subject)
	if importer == nil {
		result.Warnings = append(result.Warnings, fmt.Sprintf("%s: no importer matched sender/subject", sourceLabel))
		return result, nil
	}
	orders, warnings, err := importer.Parse(parsed)
	result.Warnings = append(result.Warnings, warnings...)
	if err != nil {
		result.Warnings = append(result.Warnings, fmt.Sprintf("%s: %v", sourceLabel, err))
		return result, nil
	}
	result.OrdersFound += len(orders)
	for _, parsedOrder := range orders {
		messageID := parsed.MessageID
		if messageID == "" {
			messageID = sourceLabel + ":" + parsedOrder.ExternalOrderID + ":" + parsedOrder.OrderDate
		}
		existing, err := s.store.GetLedgerEmailOrderByMessageID(ctx, userID, messageID)
		if err == nil {
			result.Orders = append(result.Orders, existing)
			continue
		}
		if err != nil && err != storage.ErrNotFound {
			return result, err
		}
		now := time.Now().UTC()
		order := LedgerEmailOrder{
			ID:              uuid.New(),
			EmailAccountID:  account.ID,
			ImporterID:      importer.Info().ID,
			ExternalOrderID: parsedOrder.ExternalOrderID,
			OrderDate:       parsedOrder.OrderDate,
			TotalMinor:      parsedOrder.TotalMinor,
			Currency:        parsedOrder.Currency,
			Items:           parsedOrder.Items,
			EmailMessageID:  messageID,
			EmailSubject:    parsed.Subject,
			MatchStatus:     LedgerEmailOrderStatusUnmatched,
			CreatedAt:       now,
			UpdatedAt:       now,
		}
		linked, linkWarnings, err := s.autoLinkOrder(ctx, userID, matchRule{Pattern: importer.CounterpartyPattern(), Window: importer.ChargeWindow()}, &order)
		result.Warnings = append(result.Warnings, linkWarnings...)
		if err != nil {
			return result, err
		}
		if err := s.store.CreateLedgerEmailOrder(ctx, userID, order); err != nil {
			return result, err
		}
		if linked {
			if _, err := s.store.LinkLedgerEmailOrder(ctx, userID, order.ID, LedgerEmailOrderLinkInput{TransactionIDs: order.LinkedTransactionIDs}); err != nil {
				return result, err
			}
			result.OrdersLinked++
		}
		result.OrdersNew++
		stored, err := s.store.GetLedgerEmailOrder(ctx, userID, order.ID)
		if err != nil {
			return result, err
		}
		result.Orders = append(result.Orders, stored)
	}
	return result, nil
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

func (s *EmailService) matchImporter(from string, subject string) Importer {
	for _, importer := range s.importers {
		if importer.Matches(from, subject) {
			return importer
		}
	}
	return nil
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
		return false, []string{fmt.Sprintf("multiple exact transaction matches for email order %s", order.ExternalOrderID)}, nil
	}
	combo := findTransactionCombination(candidates, target)
	if len(combo) > 0 {
		ids := make([]uuid.UUID, 0, len(combo))
		for _, txn := range combo {
			ids = append(ids, txn.ID)
		}
		order.LinkedTransactionIDs = ids
		order.MatchStatus = LedgerEmailOrderStatusMatched
		return true, []string{fmt.Sprintf("linked split payment for order %s", order.ExternalOrderID)}, nil
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
