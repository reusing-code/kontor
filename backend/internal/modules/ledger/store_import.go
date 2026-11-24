package ledger

import (
	"context"
	"errors"
	"sort"

	"github.com/dgraph-io/badger/v4"
	"github.com/google/uuid"
	"github.com/reusing-code/kontor/backend/internal/storage"
)

type LedgerImportCommitResult struct {
	ImportedRows  int
	DuplicateRows int
}

func (s *Store) GetLedgerImportByFileHash(_ context.Context, userID string, sha256 string) (LedgerImportBatch, error) {
	var batch LedgerImportBatch
	err := s.e.View(func(txn *badger.Txn) error {
		item, err := txn.Get(idxLedFileHashKey(userID, sha256))
		if err != nil {
			return err
		}
		var idStr string
		if err := item.Value(func(val []byte) error {
			idStr = string(val)
			return nil
		}); err != nil {
			return err
		}
		batchID, err := uuid.Parse(idStr)
		if err != nil {
			return err
		}
		batch, err = storage.Get[LedgerImportBatch](txn, ledImpKey(userID, batchID))
		return err
	})
	if errors.Is(err, badger.ErrKeyNotFound) {
		return batch, storage.ErrNotFound
	}
	return batch, err
}

func (s *Store) LedgerTransactionFingerprintExists(_ context.Context, userID string, fingerprint string) (bool, error) {
	exists := false
	err := s.e.View(func(txn *badger.Txn) error {
		_, err := txn.Get(idxLedTxnFPKey(userID, fingerprint))
		if err == nil {
			exists = true
			return nil
		}
		if errors.Is(err, badger.ErrKeyNotFound) {
			return nil
		}
		return err
	})
	return exists, err
}

func (s *Store) CommitLedgerImport(ctx context.Context, userID string, batch LedgerImportBatch, txns []LedgerTransaction) (LedgerImportCommitResult, error) {
	return s.CommitLedgerImportWithBalance(ctx, userID, batch, txns, nil)
}

// CommitLedgerImportWithBalance is CommitLedgerImport that also stores the
// balance the file reported (nil: none), in the same badger transaction.
func (s *Store) CommitLedgerImportWithBalance(_ context.Context, userID string, batch LedgerImportBatch, txns []LedgerTransaction, balance *LedgerBalanceSnapshot) (LedgerImportCommitResult, error) {
	result := LedgerImportCommitResult{}

	err := s.e.Update(func(txn *badger.Txn) error {
		if batch.FileSHA256 != "" {
			if _, err := txn.Get(idxLedFileHashKey(userID, batch.FileSHA256)); err == nil {
				return ErrLedgerFileImported
			} else if !errors.Is(err, badger.ErrKeyNotFound) {
				return err
			}
		}

		for i := range txns {
			t := normalizeLedgerTransaction(txns[i])
			if _, err := txn.Get(idxLedTxnFPKey(userID, t.Fingerprint)); err == nil {
				result.DuplicateRows++
				continue
			} else if !errors.Is(err, badger.ErrKeyNotFound) {
				return err
			}

			if err := insertLedgerImportedTxn(txn, userID, batch.ID, t); err != nil {
				return err
			}
			result.ImportedRows++
		}

		batch.ImportedRows = result.ImportedRows
		batch.DuplicateRows = result.DuplicateRows
		if err := storage.SetJSON(txn, ledImpKey(userID, batch.ID), batch); err != nil {
			return err
		}
		if batch.FileSHA256 != "" {
			if err := txn.Set(idxLedFileHashKey(userID, batch.FileSHA256), []byte(batch.ID.String())); err != nil {
				return err
			}
		}
		if balance != nil {
			return putLedgerBalanceSnapshotTxn(txn, userID, *balance)
		}
		return nil
	})
	if err != nil {
		return LedgerImportCommitResult{}, err
	}
	return result, nil
}

// insertLedgerImportedTxn writes txn/, idx/acc_txn/, idx/txn_fp/ and idx/imp_txn/ for one transaction.
func insertLedgerImportedTxn(txn *badger.Txn, userID string, batchID uuid.UUID, t LedgerTransaction) error {
	if err := storeLedgerTransaction(txn, userID, t); err != nil {
		return err
	}
	if err := txn.Set(idxLedAccTxnKey(userID, t.AccountID, t.BookingDate, t.ID), []byte{}); err != nil {
		return err
	}
	if err := txn.Set(idxLedTxnFPKey(userID, t.Fingerprint), []byte(t.ID.String())); err != nil {
		return err
	}
	return txn.Set(idxLedImpTxnKey(userID, batchID, t.ID), []byte{})
}

func (s *Store) ListLedgerImports(_ context.Context, userID string) ([]LedgerImportBatch, error) {
	var batches []LedgerImportBatch
	err := s.e.View(func(txn *badger.Txn) error {
		var err error
		batches, err = storage.List[LedgerImportBatch](txn, ledImpPrefix(userID))
		return err
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(batches, func(i, j int) bool {
		if batches[i].CreatedAt.Equal(batches[j].CreatedAt) {
			return batches[i].ID.String() > batches[j].ID.String()
		}
		return batches[i].CreatedAt.After(batches[j].CreatedAt)
	})
	return batches, nil
}
