package ledger

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/dgraph-io/badger/v4"
	"github.com/google/uuid"
	"github.com/reusing-code/kontor/backend/internal/storage"
)

const categorizeRetries = 3

// Both wrap storage.ErrNotFound, so callers that only know that keep working;
// the MCP tools tell them apart from a missing transaction or category.
var (
	errLedgerParentMissing   = fmt.Errorf("%w: parent category", storage.ErrNotFound)
	errLedgerCategoryMissing = fmt.Errorf("%w: category", storage.ErrNotFound)
)

// EditLedgerCategory loads a category, lets edit change it and stores it in
// one transaction, so parallel edits never lose each other's changes. The
// parent must exist and must not be the category or one of its descendants.
func (s *Store) EditLedgerCategory(ctx context.Context, userID string, id uuid.UUID, edit func(*LedgerCategory) error) (LedgerCategory, error) {
	var updated LedgerCategory
	err := storage.RetryConflicts(ctx, categorizeRetries, func() error {
		return s.e.Update(func(txn *badger.Txn) error {
			c, err := loadLedgerCategory(txn, userID, id)
			if err != nil {
				return err
			}
			if err := edit(&c); err != nil {
				return err
			}
			c = normalizeLedgerCategory(c)
			c.ID = id
			c.UpdatedAt = time.Now().UTC()
			if err := ensureLedgerCategoryParentValid(txn, userID, c.ID, c.ParentID); errors.Is(err, storage.ErrNotFound) {
				return errLedgerParentMissing
			} else if err != nil {
				return err
			}
			updated = c
			return storeLedgerCategory(txn, userID, c)
		})
	})
	return updated, err
}

// SetLedgerTransactionCategory sets the main category of a transaction and
// leaves its review status and splits alone. A linked internal transfer is
// refused (ErrLedgerTransferLinked); it has to be unlinked first.
func (s *Store) SetLedgerTransactionCategory(ctx context.Context, userID string, id, categoryID uuid.UUID) (LedgerTransaction, error) {
	var updated LedgerTransaction
	err := storage.RetryConflicts(ctx, categorizeRetries, func() error {
		return s.e.Update(func(txn *badger.Txn) error {
			t, err := loadLedgerTransaction(txn, userID, id)
			if err != nil {
				return err
			}
			if t.TransferPairTransactionID != nil {
				return ErrLedgerTransferLinked
			}
			if _, err := loadLedgerCategory(txn, userID, categoryID); errors.Is(err, storage.ErrNotFound) {
				return errLedgerCategoryMissing
			} else if err != nil {
				return err
			}
			t.CategoryID = &categoryID
			t.SpecialCategory = ""
			t.CategorizationSource = LedgerCategorizationManual
			t.UpdatedAt = time.Now().UTC()
			if err := storeLedgerTransaction(txn, userID, t); err != nil {
				return err
			}
			if err := txn.Delete(ledSugKey(userID, t.ID)); err != nil {
				return err
			}
			updated = t
			return nil
		})
	})
	return updated, err
}
