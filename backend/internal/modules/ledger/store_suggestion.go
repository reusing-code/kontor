package ledger

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/dgraph-io/badger/v4"
	"github.com/google/uuid"
	"github.com/reusing-code/kontor/backend/internal/storage"
)

const suggestionStoreAttempts = 5

func ledSugKey(userID string, txnID uuid.UUID) []byte {
	return []byte(fmt.Sprintf("u/%s/mod/ledger/sug/%s", userID, txnID))
}

func ledSugPrefix(userID string) []byte {
	return []byte(fmt.Sprintf("u/%s/mod/ledger/sug/", userID))
}

// isSuggestionCandidate reports whether a transaction is open for a category
// suggestion: unreviewed, uncategorized and not part of a transfer.
func isSuggestionCandidate(t LedgerTransaction) bool {
	return t.ReviewStatus == LedgerTransactionReviewNeedsReview &&
		t.CategoryID == nil &&
		t.SpecialCategory == "" &&
		t.TransferPairTransactionID == nil
}

func collectLedgerCategorySuggestions(txn *badger.Txn, userID string) (map[uuid.UUID]LedgerCategorySuggestion, error) {
	items := map[uuid.UUID]LedgerCategorySuggestion{}
	prefix := ledSugPrefix(userID)
	it := txn.NewIterator(badger.DefaultIteratorOptions)
	defer it.Close()
	for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
		var sug LedgerCategorySuggestion
		if err := it.Item().Value(func(val []byte) error { return json.Unmarshal(val, &sug) }); err != nil {
			return nil, err
		}
		items[sug.TransactionID] = sug
	}
	return items, nil
}

func storeLedgerCategorySuggestion(txn *badger.Txn, userID string, sug LedgerCategorySuggestion) error {
	data, err := json.Marshal(sug)
	if err != nil {
		return err
	}
	return txn.Set(ledSugKey(userID, sug.TransactionID), data)
}

type suggestionState struct {
	Categories   []LedgerCategory
	Transactions []LedgerTransaction
	Suggestions  map[uuid.UUID]LedgerCategorySuggestion
}

func (s *Store) loadSuggestionState(userID string) (suggestionState, error) {
	var state suggestionState
	err := s.e.View(func(txn *badger.Txn) error {
		var err error
		if state.Categories, err = collectLedgerCategories(txn, userID); err != nil {
			return err
		}
		if state.Transactions, err = collectLedgerTransactions(txn, userID); err != nil {
			return err
		}
		state.Suggestions, err = collectLedgerCategorySuggestions(txn, userID)
		return err
	})
	return state, err
}

// ApplyLedgerRuleMatches categorizes transactions that a match-word rule now
// catches (transaction ID → category ID) and drops their suggestions. A
// transaction that is no longer a candidate is left alone.
func (s *Store) ApplyLedgerRuleMatches(ctx context.Context, userID string, matches map[uuid.UUID]uuid.UUID) (int, error) {
	if len(matches) == 0 {
		return 0, nil
	}
	applied := 0
	err := storage.RetryConflicts(ctx, suggestionStoreAttempts, func() error {
		applied = 0
		now := time.Now().UTC()
		return s.e.Update(func(txn *badger.Txn) error {
			for txnID, categoryID := range matches {
				t, err := loadLedgerTransaction(txn, userID, txnID)
				if errors.Is(err, storage.ErrNotFound) {
					continue
				}
				if err != nil {
					return err
				}
				if !isSuggestionCandidate(t) {
					continue
				}
				if _, err := loadLedgerCategory(txn, userID, categoryID); errors.Is(err, storage.ErrNotFound) {
					continue
				} else if err != nil {
					return err
				}
				id := categoryID
				t.CategoryID = &id
				t.CategorizationSource = LedgerCategorizationKeyword
				t.UpdatedAt = now
				if err := storeLedgerTransaction(txn, userID, t); err != nil {
					return err
				}
				if err := txn.Delete(ledSugKey(userID, txnID)); err != nil {
					return err
				}
				applied++
			}
			return nil
		})
	})
	return applied, err
}

// PutLedgerCategorySuggestions stores suggestions for transactions that are
// still candidates and returns the ones it stored.
func (s *Store) PutLedgerCategorySuggestions(ctx context.Context, userID string, sugs []LedgerCategorySuggestion) ([]LedgerCategorySuggestion, error) {
	var stored []LedgerCategorySuggestion
	err := storage.RetryConflicts(ctx, suggestionStoreAttempts, func() error {
		stored = stored[:0]
		return s.e.Update(func(txn *badger.Txn) error {
			for _, sug := range sugs {
				t, err := loadLedgerTransaction(txn, userID, sug.TransactionID)
				if errors.Is(err, storage.ErrNotFound) {
					continue
				}
				if err != nil {
					return err
				}
				if !isSuggestionCandidate(t) {
					continue
				}
				if err := storeLedgerCategorySuggestion(txn, userID, sug); err != nil {
					return err
				}
				stored = append(stored, sug)
			}
			return nil
		})
	})
	return stored, err
}

// LedgerSuggestionViews returns the suggestions to show for the given
// transactions: only for candidates and only when the category still exists.
func (s *Store) LedgerSuggestionViews(_ context.Context, userID string, txns []LedgerTransaction) (map[uuid.UUID]LedgerSuggestionView, error) {
	views := map[uuid.UUID]LedgerSuggestionView{}
	err := s.e.View(func(txn *badger.Txn) error {
		sugs, err := collectLedgerCategorySuggestions(txn, userID)
		if err != nil || len(sugs) == 0 {
			return err
		}
		categories, err := collectLedgerCategories(txn, userID)
		if err != nil {
			return err
		}
		exists := make(map[uuid.UUID]bool, len(categories))
		for _, c := range categories {
			exists[c.ID] = true
		}
		for _, t := range txns {
			sug, ok := sugs[t.ID]
			if !ok || sug.CategoryID == nil || !exists[*sug.CategoryID] || !isSuggestionCandidate(t) {
				continue
			}
			views[t.ID] = LedgerSuggestionView{CategoryID: *sug.CategoryID, Confidence: sug.Confidence, MatchWord: sug.MatchWord}
		}
		return nil
	})
	return views, err
}
