package ledger

import (
	"context"
	"errors"
	"time"

	"github.com/dgraph-io/badger/v4"
	"github.com/google/uuid"
	"github.com/reusing-code/kontor/backend/internal/storage"
)

// SetLedgerTransactionSplits replaces the parts of a transaction; no parts
// removes the split. Linked internal transfers cannot be split. Category,
// review state and everything else stay as they are.
func (s *Store) SetLedgerTransactionSplits(_ context.Context, userID string, id uuid.UUID, splits []LedgerTransactionSplit) (LedgerTransaction, error) {
	var updated LedgerTransaction
	err := s.e.Update(func(txn *badger.Txn) error {
		ledgerTxn, err := loadLedgerTransaction(txn, userID, id)
		if err != nil {
			return err
		}
		if ledgerTxn.TransferPairTransactionID != nil {
			return ErrLedgerTransferLinked
		}
		parts, err := normalizeLedgerSplits(ledgerTxn.AmountMinor, splits)
		if err != nil {
			return err
		}
		for _, part := range parts {
			if _, err := loadLedgerCategory(txn, userID, *part.CategoryID); err != nil {
				if errors.Is(err, storage.ErrNotFound) {
					return splitError("unknown category")
				}
				return err
			}
		}
		ledgerTxn.Splits = parts
		ledgerTxn.UpdatedAt = time.Now().UTC()
		if err := storeLedgerTransaction(txn, userID, ledgerTxn); err != nil {
			return err
		}
		updated = ledgerTxn
		return nil
	})
	return updated, err
}
