package ledgeremail

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"mime/multipart"
	"net/mail"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/tobi/contracts/backend/internal/model"
	"github.com/tobi/contracts/backend/internal/store"
)

type Importer interface {
	Info() model.LedgerEmailImporterInfo
	Matches(from string, subject string) bool
	Parse(parsed ParsedEmail) ([]ParsedOrder, []string, error)
	CounterpartyPattern() *regexp.Regexp
}

type ParsedEmail struct {
	MessageID string
	From      string
	Subject   string
	Date      time.Time
	TextBody  string
	HTMLBody  string
}

type ParsedOrder struct {
	ExternalOrderID string
	OrderDate       string
	TotalMinor      int64
	Currency        string
	Items           []model.LedgerEmailOrderItem
}

type UploadedMessage struct {
	Filename  string
	Reader    io.Reader
	MessageID string
}

type Service struct {
	store     store.Store
	logger    *slog.Logger
	importers []Importer
}

func NewService(s store.Store, logger *slog.Logger) *Service {
	return &Service{
		store:  s,
		logger: logger,
		importers: []Importer{
			newAmazonDEImporter(),
			newPayPalDEImporter(),
		},
	}
}

func (s *Service) ListImporters() []model.LedgerEmailImporterInfo {
	items := make([]model.LedgerEmailImporterInfo, 0, len(s.importers))
	for _, importer := range s.importers {
		items = append(items, importer.Info())
	}
	return items
}

func (s *Service) ScanUploadedMessages(ctx context.Context, userID string, account model.LedgerEmailAccount, files []UploadedMessage) (model.LedgerEmailScanResult, error) {
	result := model.LedgerEmailScanResult{Warnings: []string{}, Orders: []model.LedgerEmailOrder{}}
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
			if err != nil && err != store.ErrNotFound {
				return result, err
			}
			now := time.Now().UTC()
			order := model.LedgerEmailOrder{
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
				MatchStatus:     model.LedgerEmailOrderStatusUnmatched,
				CreatedAt:       now,
				UpdatedAt:       now,
			}
			linked, warnings, err := s.autoLinkOrder(ctx, userID, importer, &order)
			result.Warnings = append(result.Warnings, warnings...)
			if err != nil {
				return result, err
			}
			if err := s.store.CreateLedgerEmailOrder(ctx, userID, order); err != nil {
				return result, err
			}
			if linked {
				if _, err := s.store.LinkLedgerEmailOrder(ctx, userID, order.ID, model.LedgerEmailOrderLinkInput{TransactionIDs: order.LinkedTransactionIDs}); err != nil {
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

func (s *Service) matchImporter(from string, subject string) Importer {
	for _, importer := range s.importers {
		if importer.Matches(from, subject) {
			return importer
		}
	}
	return nil
}

func (s *Service) autoLinkOrder(ctx context.Context, userID string, importer Importer, order *model.LedgerEmailOrder) (bool, []string, error) {
	transactions, err := s.store.ListLedgerTransactionsFiltered(ctx, userID, store.LedgerTransactionListOptions{Limit: 500})
	if err != nil {
		return false, nil, err
	}
	candidates := make([]model.LedgerTransaction, 0)
	for _, txn := range transactions.Items {
		if txn.AmountMinor >= 0 {
			continue
		}
		if len(txn.EmailOrderIDs) > 0 {
			continue
		}
		if !importer.CounterpartyPattern().MatchString(strings.TrimSpace(txn.CounterpartyName + "\n" + txn.Purpose)) {
			continue
		}
		if order.OrderDate != "" && ledgerDateDeltaDays(txn.BookingDate, order.OrderDate) > 7 {
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
	exact := make([]model.LedgerTransaction, 0)
	for _, candidate := range candidates {
		if -candidate.AmountMinor == target {
			exact = append(exact, candidate)
		}
	}
	if len(exact) == 1 {
		order.LinkedTransactionIDs = []uuid.UUID{exact[0].ID}
		order.MatchStatus = model.LedgerEmailOrderStatusMatched
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
		order.MatchStatus = model.LedgerEmailOrderStatusMatched
		return true, []string{fmt.Sprintf("linked split payment for order %s", order.ExternalOrderID)}, nil
	}
	return false, nil, nil
}

func findTransactionCombination(candidates []model.LedgerTransaction, target int64) []model.LedgerTransaction {
	ordered := append([]model.LedgerTransaction(nil), candidates...)
	sort.Slice(ordered, func(i, j int) bool {
		return ordered[i].BookingDate > ordered[j].BookingDate
	})
	var best []model.LedgerTransaction
	var walk func(start int, sum int64, picked []model.LedgerTransaction)
	walk = func(start int, sum int64, picked []model.LedgerTransaction) {
		if best != nil {
			return
		}
		if sum == target && len(picked) > 1 {
			best = append([]model.LedgerTransaction(nil), picked...)
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

func ledgerDateDeltaDays(left, right string) int {
	leftDate, leftErr := time.Parse("2006-01-02", left)
	rightDate, rightErr := time.Parse("2006-01-02", right)
	if leftErr != nil || rightErr != nil {
		return 999
	}
	delta := int(leftDate.Sub(rightDate).Hours() / 24)
	if delta < 0 {
		return -delta
	}
	return delta
}

func parseMessage(r io.Reader) (ParsedEmail, error) {
	msg, err := mail.ReadMessage(bufio.NewReader(r))
	if err != nil {
		return ParsedEmail{}, fmt.Errorf("reading message: %w", err)
	}
	parsed := ParsedEmail{
		MessageID: strings.TrimSpace(msg.Header.Get("Message-Id")),
		From:      strings.TrimSpace(msg.Header.Get("From")),
		Subject:   strings.TrimSpace(msg.Header.Get("Subject")),
	}
	if dateHeader := strings.TrimSpace(msg.Header.Get("Date")); dateHeader != "" {
		if parsedDate, err := mail.ParseDate(dateHeader); err == nil {
			parsed.Date = parsedDate.UTC()
		}
	}
	bodyBytes, err := io.ReadAll(msg.Body)
	if err != nil {
		return ParsedEmail{}, fmt.Errorf("reading body: %w", err)
	}
	mediaType, params, _ := mime.ParseMediaType(msg.Header.Get("Content-Type"))
	if strings.HasPrefix(mediaType, "multipart/") {
		reader := multipart.NewReader(bytes.NewReader(bodyBytes), params["boundary"])
		for {
			part, err := reader.NextPart()
			if err == io.EOF {
				break
			}
			if err != nil {
				return ParsedEmail{}, fmt.Errorf("reading multipart body: %w", err)
			}
			partBytes, err := io.ReadAll(part)
			if err != nil {
				return ParsedEmail{}, fmt.Errorf("reading part body: %w", err)
			}
			partType, _, _ := mime.ParseMediaType(part.Header.Get("Content-Type"))
			switch partType {
			case "text/plain":
				parsed.TextBody = string(partBytes)
			case "text/html":
				parsed.HTMLBody = string(partBytes)
			}
		}
	} else if mediaType == "text/html" {
		parsed.HTMLBody = string(bodyBytes)
	} else {
		parsed.TextBody = string(bodyBytes)
	}
	if parsed.TextBody == "" && parsed.HTMLBody != "" {
		parsed.TextBody = stripHTML(parsed.HTMLBody)
	}
	return parsed, nil
}

func stripHTML(input string) string {
	replacer := strings.NewReplacer("<br>", "\n", "<br/>", "\n", "<br />", "\n", "</p>", "\n", "</div>", "\n")
	cleaned := replacer.Replace(input)
	tagPattern := regexp.MustCompile(`<[^>]+>`)
	cleaned = tagPattern.ReplaceAllString(cleaned, " ")
	spacePattern := regexp.MustCompile(`[ \t\f\v]+`)
	cleaned = spacePattern.ReplaceAllString(cleaned, " ")
	linePattern := regexp.MustCompile(`\n\s+`)
	cleaned = linePattern.ReplaceAllString(cleaned, "\n")
	return strings.TrimSpace(cleaned)
}
