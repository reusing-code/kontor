package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/dgraph-io/badger/v4"
	"github.com/google/uuid"
	"github.com/tobi/contracts/backend/internal/model"
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

func (s *BadgerStore) CommitLedgerImport(_ context.Context, userID string, batch model.LedgerImportBatch, txns []model.LedgerTransaction) error {
	batchData, err := json.Marshal(batch)
	if err != nil {
		return err
	}

	return s.db.Update(func(txn *badger.Txn) error {
		if err := txn.Set(ledImpKey(userID, batch.ID), batchData); err != nil {
			return err
		}
		if err := txn.Set(idxLedFileHashKey(userID, batch.FileSHA256), []byte(batch.ID.String())); err != nil {
			return err
		}

		for i := range txns {
			t := &txns[i]
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
		}
		return nil
	})
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
	return batches, nil
}

// Ledger Transactions

func (s *BadgerStore) ListLedgerTransactions(_ context.Context, userID string, accountID uuid.UUID) ([]model.LedgerTransaction, error) {
	var txns []model.LedgerTransaction
	prefix := idxLedAccTxnPrefix(userID, accountID)

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
		return nil, err
	}
	if txns == nil {
		txns = []model.LedgerTransaction{}
	}
	return txns, nil
}
