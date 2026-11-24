package ledger

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/dgraph-io/badger/v4"
	"github.com/google/uuid"
	"github.com/reusing-code/kontor/backend/internal/storage"
	"github.com/reusing-code/kontor/backend/internal/storage/link"
)

type LedgerTransactionPage struct {
	Items      []LedgerTransaction
	NextCursor string
	// Offset is the position of the page's first item in the whole list.
	Offset int
	// Total is the number of matching transactions on all pages; only set by
	// ListLedgerTransactionsFiltered.
	Total int
}

type LedgerTransactionListOptions struct {
	AccountID *uuid.UUID
	// AccountIDs keeps the transactions of any of these accounts; empty does
	// not filter.
	AccountIDs   []uuid.UUID
	CategoryID   *uuid.UUID
	ReviewStatus string
	Limit        int
	Cursor       string
	// Offset is where the page starts when there is no Cursor.
	Offset int
	// All returns every match in one page; Limit, Cursor and Offset are ignored.
	All bool
}

// LedgerTransactionSearch filters SearchLedgerTransactions; empty fields do
// not filter. Amount bounds compare the absolute amount in minor units.
type LedgerTransactionSearch struct {
	Query       string
	DateFrom    string
	DateTo      string
	MinAbsMinor *int64
	MaxAbsMinor *int64
	AccountID   *uuid.UUID
	Limit       int
}

var (
	ErrTransactionNotFound = fmt.Errorf("%w: transaction", storage.ErrNotFound)
	ErrTargetNotFound      = fmt.Errorf("%w: target", storage.ErrNotFound)
)

type LedgerReviewResult struct {
	Transaction LedgerTransaction
	Category    *LedgerCategory
}

func normalizeLedgerTransaction(t LedgerTransaction) LedgerTransaction {
	if t.ReviewStatus == "" {
		t.ReviewStatus = LedgerTransactionReviewNeedsReview
	}
	if t.CategorizationSource == "" {
		t.CategorizationSource = LedgerCategorizationNone
	}
	t.Links = NormalizeLedgerTransactionLinks(t.Links)
	t.References = NormalizeLedgerTransactionReferences(t.References)
	t.EmailOrderIDs = link.NormalizeIDs(t.EmailOrderIDs)
	if t.SpecialCategory == LedgerSpecialCategoryInternalTransfer && t.CategorizationSource == LedgerCategorizationNone {
		t.CategorizationSource = LedgerCategorizationManual
	}
	return t
}

func loadLedgerTransaction(txn *badger.Txn, userID string, id uuid.UUID) (LedgerTransaction, error) {
	t, err := storage.Get[LedgerTransaction](txn, ledTxnKey(userID, id))
	if err != nil {
		return t, err
	}
	return normalizeLedgerTransaction(t), nil
}

func storeLedgerTransaction(txn *badger.Txn, userID string, t LedgerTransaction) error {
	t = normalizeLedgerTransaction(t)
	return storage.SetJSON(txn, ledTxnKey(userID, t.ID), t)
}

func collectLedgerTransactions(txn *badger.Txn, userID string) ([]LedgerTransaction, error) {
	txns, err := storage.List[LedgerTransaction](txn, ledTxnPrefix(userID))
	if err != nil {
		return nil, err
	}
	for i := range txns {
		txns[i] = normalizeLedgerTransaction(txns[i])
	}
	return txns, nil
}

func sortLedgerTransactions(txns []LedgerTransaction) {
	sort.Slice(txns, func(i, j int) bool {
		if txns[i].BookingDate == txns[j].BookingDate {
			return txns[i].ID.String() > txns[j].ID.String()
		}
		return txns[i].BookingDate > txns[j].BookingDate
	})
}

func paginateLedgerTransactions(txns []LedgerTransaction, limit int, cursor string, offset int) LedgerTransactionPage {
	start, end := ledgerPageBounds(txns, limit, cursor, offset)
	page := LedgerTransactionPage{Items: txns[start:end], Offset: start}
	if page.Items == nil {
		page.Items = []LedgerTransaction{}
	}
	if end < len(txns) && end > start {
		page.NextCursor = txns[end-1].ID.String()
	}
	return page
}

// clampLedgerPageLimit applies the default (100) and the maximum (500) page size.
func clampLedgerPageLimit(limit int) int {
	if limit <= 0 {
		return 100
	}
	return min(limit, 500)
}

// ledgerPageBounds returns the slice bounds of the page after the cursor (the
// last ID of the previous page; an unknown cursor starts at the beginning) or,
// without a cursor, of the page at offset (past the end: an empty page).
func ledgerPageBounds(txns []LedgerTransaction, limit int, cursor string, offset int) (start, end int) {
	limit = clampLedgerPageLimit(limit)
	if cursor == "" {
		start = min(max(offset, 0), len(txns))
	} else if i := slices.IndexFunc(txns, func(txn LedgerTransaction) bool { return txn.ID.String() == cursor }); i >= 0 {
		start = i + 1
	}
	return start, min(start+limit, len(txns))
}

// ListLedgerTransactions returns every transaction of an account, unpaged.
func (s *Store) ListLedgerTransactions(ctx context.Context, userID string, accountID uuid.UUID) ([]LedgerTransaction, error) {
	page, err := s.ListLedgerTransactionsFiltered(ctx, userID, LedgerTransactionListOptions{AccountID: &accountID, All: true})
	if err != nil {
		return nil, err
	}
	return page.Items, nil
}

// ListAllLedgerTransactions returns every transaction of the user, including
// those of an account that no longer exists, in key order.
func (s *Store) ListAllLedgerTransactions(_ context.Context, userID string) ([]LedgerTransaction, error) {
	var txns []LedgerTransaction
	err := s.e.View(func(txn *badger.Txn) error {
		var err error
		txns, err = collectLedgerTransactions(txn, userID)
		return err
	})
	return txns, err
}

func (s *Store) ListLedgerTransactionsPage(ctx context.Context, userID string, accountID uuid.UUID, limit int, cursor string) (LedgerTransactionPage, error) {
	return s.ListLedgerTransactionsFiltered(ctx, userID, LedgerTransactionListOptions{
		AccountID: &accountID,
		Limit:     limit,
		Cursor:    cursor,
	})
}

func (s *Store) ListLedgerTransactionsFiltered(_ context.Context, userID string, options LedgerTransactionListOptions) (LedgerTransactionPage, error) {
	page := LedgerTransactionPage{Items: []LedgerTransaction{}}
	var filtered []LedgerTransaction
	err := s.e.View(func(txn *badger.Txn) error {
		var err error
		filtered, err = filterLedgerTransactions(txn, userID, options)
		return err
	})
	if err != nil {
		return page, err
	}
	if options.All {
		if filtered != nil {
			page.Items = filtered
		}
		page.Total = len(filtered)
		return page, nil
	}
	page = paginateLedgerTransactions(filtered, options.Limit, options.Cursor, options.Offset)
	page.Total = len(filtered)
	return page, nil
}

// filterLedgerTransactions returns the matching transactions in list order
// (newest booking date first); Limit, Cursor and All are not looked at.
func filterLedgerTransactions(txn *badger.Txn, userID string, options LedgerTransactionListOptions) ([]LedgerTransaction, error) {
	all, err := collectLedgerTransactions(txn, userID)
	if err != nil {
		return nil, err
	}
	accountSet := make(map[uuid.UUID]struct{}, len(options.AccountIDs))
	for _, id := range options.AccountIDs {
		accountSet[id] = struct{}{}
	}
	var filtered []LedgerTransaction
	for _, ledgerTxn := range all {
		if options.AccountID != nil && ledgerTxn.AccountID != *options.AccountID {
			continue
		}
		if _, ok := accountSet[ledgerTxn.AccountID]; len(accountSet) > 0 && !ok {
			continue
		}
		if options.CategoryID != nil && !hasLedgerCategory(ledgerTxn, *options.CategoryID) {
			continue
		}
		if options.ReviewStatus != "" && ledgerTxn.ReviewStatus != options.ReviewStatus {
			continue
		}
		filtered = append(filtered, ledgerTxn)
	}
	sortLedgerTransactions(filtered)
	return filtered, nil
}

func (s *Store) GetLedgerTransaction(_ context.Context, userID string, id uuid.UUID) (LedgerTransaction, error) {
	var ledgerTxn LedgerTransaction
	err := s.e.View(func(txn *badger.Txn) error {
		var err error
		ledgerTxn, err = loadLedgerTransaction(txn, userID, id)
		return err
	})
	return ledgerTxn, err
}

func (s *Store) UpdateLedgerTransactionDetails(_ context.Context, userID string, id uuid.UUID, input LedgerTransactionDetailsInput) (LedgerTransaction, error) {
	updatedAt := time.Now().UTC()
	var updated LedgerTransaction
	err := s.e.Update(func(txn *badger.Txn) error {
		ledgerTxn, err := loadLedgerTransaction(txn, userID, id)
		if err != nil {
			return err
		}
		if err := s.syncLedgerReferences(txn, userID, ledgerTxn.ID, ledgerTxn.References, input.References); err != nil {
			return err
		}
		ledgerTxn.Note = input.Note
		ledgerTxn.Links = input.Links
		ledgerTxn.References = input.References
		ledgerTxn.UpdatedAt = updatedAt
		if err := storeLedgerTransaction(txn, userID, ledgerTxn); err != nil {
			return err
		}
		updated = ledgerTxn
		return nil
	})
	if err != nil {
		return LedgerTransaction{}, err
	}
	return updated, nil
}

// SearchLedgerTransactions returns the newest matching transactions, at most
// Limit; truncated reports that more matched. Like the list endpoints it
// reads all transactions of the user and filters in memory.
func (s *Store) SearchLedgerTransactions(_ context.Context, userID string, q LedgerTransactionSearch) ([]LedgerTransaction, bool, error) {
	var matched []LedgerTransaction
	err := s.e.View(func(txn *badger.Txn) error {
		all, err := collectLedgerTransactions(txn, userID)
		if err != nil {
			return err
		}
		sortLedgerTransactions(all)
		for _, t := range all {
			if q.AccountID != nil && t.AccountID != *q.AccountID {
				continue
			}
			if q.DateFrom != "" && t.BookingDate < q.DateFrom || q.DateTo != "" && t.BookingDate > q.DateTo {
				continue
			}
			abs := t.AmountMinor
			if abs < 0 {
				abs = -abs
			}
			if q.MinAbsMinor != nil && abs < *q.MinAbsMinor || q.MaxAbsMinor != nil && abs > *q.MaxAbsMinor {
				continue
			}
			if q.Query != "" && !containsFold(q.Query, t.CounterpartyName, t.Purpose, t.Note) {
				continue
			}
			matched = append(matched, t)
			if q.Limit > 0 && len(matched) > q.Limit {
				break
			}
		}
		return nil
	})
	if err != nil {
		return nil, false, err
	}
	truncated := q.Limit > 0 && len(matched) > q.Limit
	if truncated {
		matched = matched[:q.Limit]
	}
	if matched == nil {
		matched = []LedgerTransaction{}
	}
	return matched, truncated, nil
}

func containsFold(needle string, fields ...string) bool {
	needle = strings.ToLower(needle)
	for _, f := range fields {
		if strings.Contains(strings.ToLower(f), needle) {
			return true
		}
	}
	return false
}

func (s *Store) ReviewLedgerTransaction(_ context.Context, userID string, id uuid.UUID, input LedgerTransactionReviewInput) (LedgerReviewResult, error) {
	result := LedgerReviewResult{}
	now := time.Now().UTC()
	err := s.e.Update(func(txn *badger.Txn) error {
		ledgerTxn, err := loadLedgerTransaction(txn, userID, id)
		if err != nil {
			return err
		}

		var selectedCategory *LedgerCategory
		if input.CategoryID != nil {
			cat, err := loadLedgerCategory(txn, userID, *input.CategoryID)
			if err != nil {
				return err
			}
			selectedCategory = &cat
		}

		if input.NewCategory != nil {
			categoryID := uuid.New()
			newCategory := LedgerCategory{
				ID:         categoryID,
				Name:       input.NewCategory.Name,
				ParentID:   input.NewCategory.ParentID,
				MatchWords: input.NewCategory.MatchWords,
				CreatedAt:  now,
				UpdatedAt:  now,
			}
			if err := ensureLedgerCategoryParentValid(txn, userID, categoryID, newCategory.ParentID); err != nil {
				return err
			}
			if err := storeLedgerCategory(txn, userID, newCategory); err != nil {
				return err
			}
			selectedCategory = &newCategory
		}

		if selectedCategory == nil && ledgerTxn.CategoryID != nil {
			cat, err := loadLedgerCategory(txn, userID, *ledgerTxn.CategoryID)
			if err == nil {
				selectedCategory = &cat
			} else if !errors.Is(err, storage.ErrNotFound) {
				return err
			}
		}

		if len(input.AddMatchWords) > 0 {
			if selectedCategory == nil {
				return storage.ErrNotFound
			}
			selectedCategory.MatchWords = append(selectedCategory.MatchWords, input.AddMatchWords...)
			selectedCategory.MatchWords = NormalizeLedgerMatchWords(selectedCategory.MatchWords)
			selectedCategory.UpdatedAt = now
			if err := storeLedgerCategory(txn, userID, *selectedCategory); err != nil {
				return err
			}
		}

		if selectedCategory != nil {
			if ledgerTxn.TransferPairTransactionID != nil {
				return ErrLedgerTransferLinked
			}
			categoryID := selectedCategory.ID
			ledgerTxn.CategoryID = &categoryID
			ledgerTxn.SpecialCategory = ""
			ledgerTxn.CategorizationSource = LedgerCategorizationManual
		}
		ledgerTxn.ReviewStatus = LedgerTransactionReviewConfirmed
		ledgerTxn.UpdatedAt = now
		if err := storeLedgerTransaction(txn, userID, ledgerTxn); err != nil {
			return err
		}
		if err := txn.Delete(ledSugKey(userID, ledgerTxn.ID)); err != nil {
			return err
		}

		result.Transaction = ledgerTxn
		if selectedCategory != nil {
			catCopy := *selectedCategory
			result.Category = &catCopy
		}
		return nil
	})
	if err != nil {
		return LedgerReviewResult{}, err
	}
	return result, nil
}
