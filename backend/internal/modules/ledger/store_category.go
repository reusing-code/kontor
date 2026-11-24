package ledger

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/dgraph-io/badger/v4"
	"github.com/google/uuid"
	"github.com/reusing-code/kontor/backend/internal/storage"
)

func normalizeLedgerCategory(c LedgerCategory) LedgerCategory {
	c.MatchWords = NormalizeLedgerMatchWords(c.MatchWords)
	if c.MatchWords == nil {
		c.MatchWords = []string{}
	}
	return c
}

func loadLedgerCategory(txn *badger.Txn, userID string, id uuid.UUID) (LedgerCategory, error) {
	cat, err := storage.Get[LedgerCategory](txn, ledCatKey(userID, id))
	if err != nil {
		return cat, err
	}
	return normalizeLedgerCategory(cat), nil
}

func storeLedgerCategory(txn *badger.Txn, userID string, cat LedgerCategory) error {
	cat = normalizeLedgerCategory(cat)
	return storage.SetJSON(txn, ledCatKey(userID, cat.ID), cat)
}

func collectLedgerCategories(txn *badger.Txn, userID string) ([]LedgerCategory, error) {
	categories, err := storage.List[LedgerCategory](txn, ledCatPrefix(userID))
	if err != nil {
		return nil, err
	}
	for i := range categories {
		categories[i] = normalizeLedgerCategory(categories[i])
	}
	return categories, nil
}

func ensureLedgerCategoryParentValid(txn *badger.Txn, userID string, categoryID uuid.UUID, parentID *uuid.UUID) error {
	if parentID == nil {
		return nil
	}
	if *parentID == categoryID {
		return ErrLedgerCategoryHasCycle
	}
	current, err := loadLedgerCategory(txn, userID, *parentID)
	if err != nil {
		return err
	}
	for current.ParentID != nil {
		if *current.ParentID == categoryID {
			return ErrLedgerCategoryHasCycle
		}
		next, err := loadLedgerCategory(txn, userID, *current.ParentID)
		if err != nil {
			return err
		}
		current = next
	}
	return nil
}

func sortLedgerCategories(categories []LedgerCategory) {
	sort.Slice(categories, func(i, j int) bool {
		leftParent := ""
		rightParent := ""
		if categories[i].ParentID != nil {
			leftParent = categories[i].ParentID.String()
		}
		if categories[j].ParentID != nil {
			rightParent = categories[j].ParentID.String()
		}
		if leftParent != rightParent {
			return leftParent < rightParent
		}
		leftName := strings.ToLower(categories[i].Name)
		rightName := strings.ToLower(categories[j].Name)
		if leftName != rightName {
			return leftName < rightName
		}
		return categories[i].CreatedAt.Before(categories[j].CreatedAt)
	})
}

func (s *Store) ListLedgerCategories(_ context.Context, userID string) ([]LedgerCategory, error) {
	var categories []LedgerCategory
	err := s.e.View(func(txn *badger.Txn) error {
		var err error
		categories, err = collectLedgerCategories(txn, userID)
		return err
	})
	if err != nil {
		return nil, err
	}
	sortLedgerCategories(categories)
	return categories, nil
}

func (s *Store) GetLedgerCategory(_ context.Context, userID string, id uuid.UUID) (LedgerCategory, error) {
	var cat LedgerCategory
	err := s.e.View(func(txn *badger.Txn) error {
		var err error
		cat, err = loadLedgerCategory(txn, userID, id)
		return err
	})
	return cat, err
}

func (s *Store) CreateLedgerCategory(_ context.Context, userID string, c LedgerCategory) error {
	c = normalizeLedgerCategory(c)
	return s.e.Update(func(txn *badger.Txn) error {
		if err := ensureLedgerCategoryParentValid(txn, userID, c.ID, c.ParentID); err != nil {
			return err
		}
		return storeLedgerCategory(txn, userID, c)
	})
}

func (s *Store) UpdateLedgerCategory(_ context.Context, userID string, c LedgerCategory) error {
	c = normalizeLedgerCategory(c)
	return s.e.Update(func(txn *badger.Txn) error {
		if _, err := loadLedgerCategory(txn, userID, c.ID); err != nil {
			return err
		}
		if err := ensureLedgerCategoryParentValid(txn, userID, c.ID, c.ParentID); err != nil {
			return err
		}
		return storeLedgerCategory(txn, userID, c)
	})
}

func (s *Store) DeleteLedgerCategory(_ context.Context, userID string, id uuid.UUID) error {
	return s.e.Update(func(txn *badger.Txn) error {
		if _, err := loadLedgerCategory(txn, userID, id); err != nil {
			return err
		}

		categories, err := collectLedgerCategories(txn, userID)
		if err != nil {
			return err
		}
		for _, category := range categories {
			if category.ParentID != nil && *category.ParentID == id {
				return ErrLedgerCategoryHasChild
			}
		}

		transactions, err := collectLedgerTransactions(txn, userID)
		if err != nil {
			return err
		}
		for _, ledgerTxn := range transactions {
			if !hasLedgerCategory(ledgerTxn, id) {
				continue
			}
			if ledgerTxn.CategoryID != nil && *ledgerTxn.CategoryID == id {
				if ledgerTxn.TransferPairTransactionID != nil {
					if _, err := unlinkLedgerTransferTxn(txn, userID, ledgerTxn.ID); err != nil {
						return err
					}
					ledgerTxn, err = loadLedgerTransaction(txn, userID, ledgerTxn.ID)
					if err != nil {
						return err
					}
				}
				ledgerTxn.CategoryID = nil
				ledgerTxn.CategorizationSource = LedgerCategorizationNone
			}
			// a part keeps its amount without category until it is reviewed again
			clearLedgerSplitCategory(&ledgerTxn, id)
			ledgerTxn.ReviewStatus = LedgerTransactionReviewNeedsReview
			ledgerTxn.UpdatedAt = time.Now().UTC()
			if err := storeLedgerTransaction(txn, userID, ledgerTxn); err != nil {
				return err
			}
		}

		return txn.Delete(ledCatKey(userID, id))
	})
}
