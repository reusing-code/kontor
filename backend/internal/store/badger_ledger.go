package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/dgraph-io/badger/v4"
	"github.com/google/uuid"
	"github.com/tobi/contracts/backend/internal/model"
)

var (
	ErrLedgerPreviewExpired = errors.New("ledger preview expired")
	ErrLedgerFileImported   = errors.New("ledger file already imported")
)

// Ledger account key helpers

func ledAccKey(userID string, id uuid.UUID) []byte {
	return []byte(fmt.Sprintf("u/%s/led/acc/%s", userID, id))
}

func ledAccPrefix(userID string) []byte {
	return []byte(fmt.Sprintf("u/%s/led/acc/", userID))
}

func idxLedAccIBANKey(userID string, iban string) []byte {
	return []byte(fmt.Sprintf("u/%s/idx/led_acc_iban/%s", userID, iban))
}

// Ledger transaction key helpers

func ledTxnKey(userID string, id uuid.UUID) []byte {
	return []byte(fmt.Sprintf("u/%s/led/txn/%s", userID, id))
}

func idxLedAccTxnKey(userID string, accountID uuid.UUID, bookingDate string, txnID uuid.UUID) []byte {
	return []byte(fmt.Sprintf("u/%s/idx/led_acc_txn/%s/%s/%s", userID, accountID, bookingDate, txnID))
}

func idxLedAccTxnPrefix(userID string, accountID uuid.UUID) []byte {
	return []byte(fmt.Sprintf("u/%s/idx/led_acc_txn/%s/", userID, accountID))
}

func idxLedTxnFPKey(userID string, fingerprint string) []byte {
	return []byte(fmt.Sprintf("u/%s/idx/led_txn_fp/%s", userID, fingerprint))
}

func idxLedImpTxnKey(userID string, batchID uuid.UUID, txnID uuid.UUID) []byte {
	return []byte(fmt.Sprintf("u/%s/idx/led_imp_txn/%s/%s", userID, batchID, txnID))
}

// Ledger import batch key helpers

func ledImpKey(userID string, id uuid.UUID) []byte {
	return []byte(fmt.Sprintf("u/%s/led/imp/%s", userID, id))
}

func ledImpPrefix(userID string) []byte {
	return []byte(fmt.Sprintf("u/%s/led/imp/", userID))
}

func idxLedFileHashKey(userID string, sha256 string) []byte {
	return []byte(fmt.Sprintf("u/%s/idx/led_file_hash/%s", userID, sha256))
}

// Ledger Accounts

func (s *BadgerStore) ListLedgerAccounts(_ context.Context, userID string) ([]model.LedgerAccount, error) {
	var accounts []model.LedgerAccount
	prefix := ledAccPrefix(userID)

	err := s.db.View(func(txn *badger.Txn) error {
		it := txn.NewIterator(badger.DefaultIteratorOptions)
		defer it.Close()

		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			var a model.LedgerAccount
			if err := it.Item().Value(func(val []byte) error {
				return json.Unmarshal(val, &a)
			}); err != nil {
				return err
			}
			accounts = append(accounts, a)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if accounts == nil {
		accounts = []model.LedgerAccount{}
	}
	sort.Slice(accounts, func(i, j int) bool {
		if accounts[i].Name == accounts[j].Name {
			return accounts[i].CreatedAt.Before(accounts[j].CreatedAt)
		}
		return strings.ToLower(accounts[i].Name) < strings.ToLower(accounts[j].Name)
	})
	return accounts, nil
}

func (s *BadgerStore) GetLedgerAccount(_ context.Context, userID string, id uuid.UUID) (model.LedgerAccount, error) {
	var a model.LedgerAccount
	err := s.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get(ledAccKey(userID, id))
		if err != nil {
			return err
		}
		return item.Value(func(val []byte) error {
			return json.Unmarshal(val, &a)
		})
	})
	if errors.Is(err, badger.ErrKeyNotFound) {
		return a, ErrNotFound
	}
	return a, err
}

func (s *BadgerStore) FindLedgerAccountByIBAN(_ context.Context, userID string, iban string) (model.LedgerAccount, error) {
	var a model.LedgerAccount
	err := s.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get(idxLedAccIBANKey(userID, iban))
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
		accID, err := uuid.Parse(idStr)
		if err != nil {
			return err
		}
		accItem, err := txn.Get(ledAccKey(userID, accID))
		if err != nil {
			return err
		}
		return accItem.Value(func(val []byte) error {
			return json.Unmarshal(val, &a)
		})
	})
	if errors.Is(err, badger.ErrKeyNotFound) {
		return a, ErrNotFound
	}
	return a, err
}

func (s *BadgerStore) CreateLedgerAccount(_ context.Context, userID string, a model.LedgerAccount) error {
	data, err := json.Marshal(a)
	if err != nil {
		return err
	}
	return s.db.Update(func(txn *badger.Txn) error {
		if a.IBAN != "" {
			if _, err := txn.Get(idxLedAccIBANKey(userID, a.IBAN)); err == nil {
				return ErrConflict
			} else if !errors.Is(err, badger.ErrKeyNotFound) {
				return err
			}
		}
		if err := txn.Set(ledAccKey(userID, a.ID), data); err != nil {
			return err
		}
		if a.IBAN != "" {
			if err := txn.Set(idxLedAccIBANKey(userID, a.IBAN), []byte(a.ID.String())); err != nil {
				return err
			}
		}
		return nil
	})
}

// Ledger Import Batches

func (s *BadgerStore) GetLedgerImportByFileHash(_ context.Context, userID string, sha256 string) (model.LedgerImportBatch, error) {
	var batch model.LedgerImportBatch
	err := s.db.View(func(txn *badger.Txn) error {
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
		batchItem, err := txn.Get(ledImpKey(userID, batchID))
		if err != nil {
			return err
		}
		return batchItem.Value(func(val []byte) error {
			return json.Unmarshal(val, &batch)
		})
	})
	if errors.Is(err, badger.ErrKeyNotFound) {
		return batch, ErrNotFound
	}
	return batch, err
}

func (s *BadgerStore) LedgerTransactionFingerprintExists(_ context.Context, userID string, fingerprint string) (bool, error) {
	exists := false
	err := s.db.View(func(txn *badger.Txn) error {
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

func (s *BadgerStore) CommitLedgerImport(_ context.Context, userID string, batch model.LedgerImportBatch, txns []model.LedgerTransaction) (LedgerImportCommitResult, error) {
	batchData, err := json.Marshal(batch)
	if err != nil {
		return LedgerImportCommitResult{}, err
	}

	result := LedgerImportCommitResult{}

	err = s.db.Update(func(txn *badger.Txn) error {
		if _, err := txn.Get(idxLedFileHashKey(userID, batch.FileSHA256)); err == nil {
			return ErrLedgerFileImported
		} else if !errors.Is(err, badger.ErrKeyNotFound) {
			return err
		}

		if err := txn.Set(ledImpKey(userID, batch.ID), batchData); err != nil {
			return err
		}
		if err := txn.Set(idxLedFileHashKey(userID, batch.FileSHA256), []byte(batch.ID.String())); err != nil {
			return err
		}

		for i := range txns {
			t := &txns[i]
			if _, err := txn.Get(idxLedTxnFPKey(userID, t.Fingerprint)); err == nil {
				result.DuplicateRows++
				continue
			} else if !errors.Is(err, badger.ErrKeyNotFound) {
				return err
			}

			data, err := json.Marshal(t)
			if err != nil {
				return err
			}
			if err := txn.Set(ledTxnKey(userID, t.ID), data); err != nil {
				return err
			}
			if err := txn.Set(idxLedAccTxnKey(userID, t.AccountID, t.BookingDate, t.ID), []byte{}); err != nil {
				return err
			}
			if err := txn.Set(idxLedTxnFPKey(userID, t.Fingerprint), []byte(t.ID.String())); err != nil {
				return err
			}
			if err := txn.Set(idxLedImpTxnKey(userID, batch.ID, t.ID), []byte{}); err != nil {
				return err
			}
			result.ImportedRows++
		}
		return nil
	})
	if err != nil {
		return LedgerImportCommitResult{}, err
	}
	return result, nil
}

func (s *BadgerStore) ListLedgerImports(_ context.Context, userID string) ([]model.LedgerImportBatch, error) {
	var batches []model.LedgerImportBatch
	prefix := ledImpPrefix(userID)

	err := s.db.View(func(txn *badger.Txn) error {
		it := txn.NewIterator(badger.DefaultIteratorOptions)
		defer it.Close()

		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			var b model.LedgerImportBatch
			if err := it.Item().Value(func(val []byte) error {
				return json.Unmarshal(val, &b)
			}); err != nil {
				return err
			}
			batches = append(batches, b)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if batches == nil {
		batches = []model.LedgerImportBatch{}
	}
	sort.Slice(batches, func(i, j int) bool {
		if batches[i].CreatedAt.Equal(batches[j].CreatedAt) {
			return batches[i].ID.String() > batches[j].ID.String()
		}
		return batches[i].CreatedAt.After(batches[j].CreatedAt)
	})
	return batches, nil
}

// Ledger Transactions

func (s *BadgerStore) ListLedgerTransactions(_ context.Context, userID string, accountID uuid.UUID) ([]model.LedgerTransaction, error) {
	page, err := s.ListLedgerTransactionsPage(context.Background(), userID, accountID, 1000, "")
	if err != nil {
		return nil, err
	}
	return page.Items, nil
}

func (s *BadgerStore) ListLedgerTransactionsPage(_ context.Context, userID string, accountID uuid.UUID, limit int, cursor string) (LedgerTransactionPage, error) {
	if limit <= 0 {
		limit = 100
	}
	if limit > 500 {
		limit = 500
	}

	var txns []model.LedgerTransaction
	prefix := idxLedAccTxnPrefix(userID, accountID)
	page := LedgerTransactionPage{Items: []model.LedgerTransaction{}}

	err := s.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.PrefetchValues = false
		it := txn.NewIterator(opts)
		defer it.Close()

		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			key := it.Item().Key()
			// Key format: .../{bookingDate}/{txnID}
			// Extract txnID from the last 36 chars (UUID length)
			keyStr := string(key)
			if len(keyStr) < 36 {
				continue
			}
			txnIDStr := keyStr[len(keyStr)-36:]
			txnID, err := uuid.Parse(txnIDStr)
			if err != nil {
				continue
			}

			item, err := txn.Get(ledTxnKey(userID, txnID))
			if err != nil {
				if errors.Is(err, badger.ErrKeyNotFound) {
					continue
				}
				return err
			}

			var t model.LedgerTransaction
			if err := item.Value(func(val []byte) error {
				return json.Unmarshal(val, &t)
			}); err != nil {
				return err
			}
			txns = append(txns, t)
		}
		return nil
	})
	if err != nil {
		return page, err
	}
	sort.Slice(txns, func(i, j int) bool {
		if txns[i].BookingDate == txns[j].BookingDate {
			return txns[i].ID.String() > txns[j].ID.String()
		}
		return txns[i].BookingDate > txns[j].BookingDate
	})

	start := 0
	if cursor != "" {
		for i, txn := range txns {
			if txn.ID.String() == cursor {
				start = i + 1
				break
			}
		}
	}
	if start > len(txns) {
		start = len(txns)
	}
	end := start + limit
	if end > len(txns) {
		end = len(txns)
	}
	page.Items = txns[start:end]
	if end < len(txns) && end > start {
		page.NextCursor = txns[end-1].ID.String()
	}
	return page, nil
}
