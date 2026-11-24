package ledger

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/dgraph-io/badger/v4"
	"github.com/google/uuid"
	"github.com/reusing-code/kontor/backend/internal/storage"
)

func loadLedgerAccountTxn(txn *badger.Txn, userID string, id uuid.UUID) (LedgerAccount, error) {
	return storage.Get[LedgerAccount](txn, ledAccKey(userID, id))
}

// UpdateLedgerAccount changes name and bank; IBAN and currency stay as they
// are (the IBAN index and the bank mappings depend on them).
func (s *Store) UpdateLedgerAccount(_ context.Context, userID string, id uuid.UUID, input LedgerAccountUpdateInput) (LedgerAccount, error) {
	var updated LedgerAccount
	err := s.e.Update(func(txn *badger.Txn) error {
		a, err := loadLedgerAccountTxn(txn, userID, id)
		if err != nil {
			return err
		}
		a.Name, a.Bank, a.UpdatedAt = input.Name, input.Bank, time.Now().UTC()
		updated = a
		return storage.SetJSON(txn, ledAccKey(userID, id), a)
	})
	return updated, err
}

// LedgerAccountDeleteSummary is what deleting an account removes, for the
// confirmation.
type LedgerAccountDeleteSummary struct {
	Transactions int `json:"transactions"`
	Imports      int `json:"imports"`
	// links to contracts, purchases, vehicles and costs
	References int `json:"references"`
	// internal transfers paired with another account
	Transfers    int                        `json:"transfers"`
	EmailOrders  int                        `json:"emailOrders"`
	BankAccounts []LedgerAccountBankMapping `json:"bankAccounts"`
}

// LedgerAccountBankMapping names a bank account mapped to the ledger account.
type LedgerAccountBankMapping struct {
	ConnectionID uuid.UUID `json:"connectionId"`
	Bank         string    `json:"bank"`
	UID          string    `json:"uid"`
	Name         string    `json:"name"`
	IBAN         string    `json:"iban"`
}

func (s *Store) LedgerAccountDeleteSummary(_ context.Context, userID string, id uuid.UUID) (LedgerAccountDeleteSummary, error) {
	summary := LedgerAccountDeleteSummary{BankAccounts: []LedgerAccountBankMapping{}}
	err := s.e.View(func(txn *badger.Txn) error {
		if _, err := loadLedgerAccountTxn(txn, userID, id); err != nil {
			return err
		}
		ids, err := accountTransactionIDs(txn, userID, id, 0)
		if err != nil {
			return err
		}
		orders := map[uuid.UUID]bool{}
		for _, txnID := range ids {
			t, err := loadLedgerTransaction(txn, userID, txnID)
			if errors.Is(err, storage.ErrNotFound) {
				continue
			}
			if err != nil {
				return err
			}
			summary.Transactions++
			summary.References += len(t.References)
			if t.TransferPairTransactionID != nil {
				summary.Transfers++
			}
			for _, orderID := range t.EmailOrderIDs {
				orders[orderID] = true
			}
		}
		summary.EmailOrders = len(orders)
		batches, err := accountImportBatches(txn, userID, id)
		if err != nil {
			return err
		}
		summary.Imports = len(batches)
		conns, err := collectLedgerBankConnections(txn, userID)
		if err != nil {
			return err
		}
		for _, c := range conns {
			for _, a := range c.Accounts {
				if a.LedgerAccountID != nil && *a.LedgerAccountID == id {
					summary.BankAccounts = append(summary.BankAccounts, LedgerAccountBankMapping{
						ConnectionID: c.ID, Bank: c.ASPSPName, UID: a.UID, Name: a.Name, IBAN: a.IBAN,
					})
				}
			}
		}
		return nil
	})
	return summary, err
}

// deleteAccountChunk is how many transactions one badger transaction deletes;
// every transaction can touch several other records (link targets, the
// transfer partner, email orders), so a whole account would not fit.
const deleteAccountChunk = 200

// DeleteLedgerAccount removes an account with its transactions and import
// batches. First the bank accounts mapped to it are unmapped, so no sync adds
// transactions; then the transactions go in chunks, and the account and its
// batches last, once no transaction is left. An interrupted delete can be
// repeated and continues where it stopped.
func (s *Store) DeleteLedgerAccount(ctx context.Context, userID string, id uuid.UUID) error {
	if err := retryOnConflict(func() error {
		return s.e.Update(func(txn *badger.Txn) error {
			if _, err := loadLedgerAccountTxn(txn, userID, id); err != nil {
				return err
			}
			return unmapLedgerAccountTxn(txn, userID, id)
		})
	}); err != nil {
		return err
	}
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		done := false
		err := retryOnConflict(func() error {
			return s.e.Update(func(txn *badger.Txn) error {
				ids, err := accountTransactionIDs(txn, userID, id, deleteAccountChunk)
				if err != nil {
					return err
				}
				if len(ids) == 0 {
					done = true
					return s.deleteLedgerAccountRecordTxn(txn, userID, id)
				}
				for _, txnID := range ids {
					if err := s.deleteLedgerTransactionTxn(txn, userID, id, txnID); err != nil {
						return err
					}
				}
				return nil
			})
		})
		if err != nil {
			return err
		}
		if done {
			return nil
		}
	}
}

// unmapLedgerAccountTxn clears the mapping of every bank account mapped to the
// ledger account and resets its sync state. The auto-mapping by IBAN stays
// allowed, so a new account with the same IBAN is picked up again.
func unmapLedgerAccountTxn(txn *badger.Txn, userID string, id uuid.UUID) error {
	conns, err := collectLedgerBankConnections(txn, userID)
	if err != nil {
		return err
	}
	for _, c := range conns {
		changed := false
		for i := range c.Accounts {
			if a := &c.Accounts[i]; a.LedgerAccountID != nil && *a.LedgerAccountID == id {
				a.LedgerAccountID = nil
				resetBankAccountSyncState(a)
				changed = true
			}
		}
		if changed {
			if err := storeLedgerBankConnection(txn, userID, c); err != nil {
				return err
			}
		}
	}
	return nil
}

// deleteLedgerTransactionTxn removes one transaction of account accountID and
// everything that points at it.
func (s *Store) deleteLedgerTransactionTxn(txn *badger.Txn, userID string, accountID, txnID uuid.UUID) error {
	t, err := loadLedgerTransaction(txn, userID, txnID)
	if errors.Is(err, storage.ErrNotFound) {
		// a dangling index entry
		return deleteKeysWithPrefix(txn, idxLedAccTxnPrefix(userID, accountID), func(key []byte) bool {
			return len(key) >= 36 && string(key[len(key)-36:]) == txnID.String()
		})
	}
	if err != nil {
		return err
	}
	for _, ref := range t.References {
		target, err := s.links.Target(ref.Type)
		if err != nil {
			continue
		}
		if err := target.RemoveLink(txn, userID, ref.TargetID, t.ID); err != nil && !errors.Is(err, storage.ErrNotFound) {
			return err
		}
	}
	if t.TransferPairTransactionID != nil {
		paired, err := loadLedgerTransaction(txn, userID, *t.TransferPairTransactionID)
		switch {
		case err == nil && paired.AccountID != accountID && paired.TransferPairTransactionID != nil && *paired.TransferPairTransactionID == t.ID:
			clearLedgerTransferState(&paired)
			if err := storeLedgerTransaction(txn, userID, paired); err != nil {
				return err
			}
		case err != nil && !errors.Is(err, storage.ErrNotFound):
			return err
		}
	}
	for _, orderID := range t.EmailOrderIDs {
		if err := detachLedgerEmailOrderTxn(txn, userID, orderID, t.ID); err != nil {
			return err
		}
	}
	if err := deleteIfValue(txn, idxLedTxnFPKey(userID, t.Fingerprint), t.ID.String()); err != nil {
		return err
	}
	// an API row absorbed into this (CSV) transaction becomes importable again
	if item, err := txn.Get(idxLedXAbsKey(userID, t.ID)); err == nil {
		var fp []byte
		if fp, err = item.ValueCopy(nil); err != nil {
			return err
		}
		if err := deleteIfValue(txn, idxLedXDupKey(userID, string(fp)), t.ID.String()); err != nil {
			return err
		}
		if err := txn.Delete(idxLedXAbsKey(userID, t.ID)); err != nil {
			return err
		}
	} else if !errors.Is(err, badger.ErrKeyNotFound) {
		return err
	}
	for _, key := range [][]byte{
		idxLedImpTxnKey(userID, t.ImportBatchID, t.ID),
		idxLedAccTxnKey(userID, t.AccountID, t.BookingDate, t.ID),
		ledSugKey(userID, t.ID),
		ledTxnKey(userID, t.ID),
	} {
		if err := txn.Delete(key); err != nil {
			return err
		}
	}
	return nil
}

// detachLedgerEmailOrderTxn removes one transaction from an email order; an
// order without transactions is unmatched again, so it can be linked to the
// newly imported transaction.
func detachLedgerEmailOrderTxn(txn *badger.Txn, userID string, orderID, txnID uuid.UUID) error {
	order, err := loadLedgerEmailOrder(txn, userID, orderID)
	if errors.Is(err, storage.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	order.LinkedTransactionIDs = withoutUUID(order.LinkedTransactionIDs, txnID)
	if len(order.LinkedTransactionIDs) == 0 && order.MatchStatus == LedgerEmailOrderStatusMatched {
		order.MatchStatus = LedgerEmailOrderStatusUnmatched
	}
	order.UpdatedAt = time.Now().UTC()
	return storeLedgerEmailOrder(txn, userID, order)
}

// deleteLedgerAccountRecordTxn removes the account, its IBAN index, its
// balance snapshots and its import batches (with their file hashes, so the files can be imported
// again). It runs once the account has no transactions left.
func (s *Store) deleteLedgerAccountRecordTxn(txn *badger.Txn, userID string, id uuid.UUID) error {
	a, err := loadLedgerAccountTxn(txn, userID, id)
	if err != nil {
		return err
	}
	batches, err := accountImportBatches(txn, userID, id)
	if err != nil {
		return err
	}
	for _, b := range batches {
		if b.FileSHA256 != "" {
			if err := deleteIfValue(txn, idxLedFileHashKey(userID, b.FileSHA256), b.ID.String()); err != nil {
				return err
			}
		}
		if err := deleteKeysWithPrefix(txn, idxLedImpTxnPrefix(userID, b.ID), nil); err != nil {
			return err
		}
		if err := txn.Delete(ledImpKey(userID, b.ID)); err != nil {
			return err
		}
	}
	if err := deleteKeysWithPrefix(txn, ledBalAccountPrefix(userID, id), nil); err != nil {
		return err
	}
	if a.IBAN != "" {
		if err := deleteIfValue(txn, idxLedAccIBANKey(userID, a.IBAN), id.String()); err != nil {
			return err
		}
	}
	return txn.Delete(ledAccKey(userID, id))
}

// accountTransactionIDs lists up to limit transaction IDs of an account from
// its booking-date index (limit 0: all).
func accountTransactionIDs(txn *badger.Txn, userID string, accountID uuid.UUID, limit int) ([]uuid.UUID, error) {
	prefix := idxLedAccTxnPrefix(userID, accountID)
	opts := badger.DefaultIteratorOptions
	opts.PrefetchValues = false
	opts.Prefix = prefix
	it := txn.NewIterator(opts)
	defer it.Close()
	var ids []uuid.UUID
	for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
		key := it.Item().Key()
		if len(key) < 36 {
			continue
		}
		id, err := uuid.Parse(string(key[len(key)-36:]))
		if err != nil {
			continue
		}
		ids = append(ids, id)
		if limit > 0 && len(ids) == limit {
			break
		}
	}
	return ids, nil
}

func accountImportBatches(txn *badger.Txn, userID string, accountID uuid.UUID) ([]LedgerImportBatch, error) {
	all, err := storage.List[LedgerImportBatch](txn, ledImpPrefix(userID))
	if err != nil {
		return nil, err
	}
	var batches []LedgerImportBatch
	for _, b := range all {
		if b.AccountID == accountID {
			batches = append(batches, b)
		}
	}
	return batches, nil
}

func collectLedgerBankConnections(txn *badger.Txn, userID string) ([]LedgerBankConnection, error) {
	stored, err := storage.List[storableLedgerBankConnection](txn, ledBankConnPrefix(userID))
	if err != nil {
		return nil, err
	}
	var items []LedgerBankConnection
	for _, c := range stored {
		items = append(items, LedgerBankConnection(c))
	}
	return items, nil
}

func idxLedImpTxnPrefix(userID string, batchID uuid.UUID) []byte {
	return []byte(fmt.Sprintf("u/%s/mod/ledger/idx/imp_txn/%s/", userID, batchID))
}

// deleteIfValue deletes key when it holds want (an index entry of this
// record, not of another one with the same key).
func deleteIfValue(txn *badger.Txn, key []byte, want string) error {
	item, err := txn.Get(key)
	if errors.Is(err, badger.ErrKeyNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	val, err := item.ValueCopy(nil)
	if err != nil {
		return err
	}
	if string(val) != want {
		return nil
	}
	return txn.Delete(key)
}

// deleteKeysWithPrefix deletes the keys under prefix that match accepts (all
// of them when match is nil).
func deleteKeysWithPrefix(txn *badger.Txn, prefix []byte, match func(key []byte) bool) error {
	opts := badger.DefaultIteratorOptions
	opts.PrefetchValues = false
	opts.Prefix = prefix
	it := txn.NewIterator(opts)
	var keys [][]byte
	for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
		key := it.Item().KeyCopy(nil)
		if match == nil || match(key) {
			keys = append(keys, key)
		}
	}
	it.Close()
	for _, key := range keys {
		if err := txn.Delete(key); err != nil {
			return err
		}
	}
	return nil
}

func collectLedgerAccounts(txn *badger.Txn, userID string) ([]LedgerAccount, error) {
	return storage.List[LedgerAccount](txn, ledAccPrefix(userID))
}

func (s *Store) ListLedgerAccounts(_ context.Context, userID string) ([]LedgerAccount, error) {
	var accounts []LedgerAccount
	err := s.e.View(func(txn *badger.Txn) error {
		var err error
		accounts, err = collectLedgerAccounts(txn, userID)
		return err
	})
	if err != nil {
		return nil, err
	}
	sortLedgerAccounts(accounts)
	return accounts, nil
}

// sortLedgerAccounts puts accounts into list order (name).
func sortLedgerAccounts(accounts []LedgerAccount) {
	sort.Slice(accounts, func(i, j int) bool {
		if accounts[i].Name == accounts[j].Name {
			return accounts[i].CreatedAt.Before(accounts[j].CreatedAt)
		}
		return strings.ToLower(accounts[i].Name) < strings.ToLower(accounts[j].Name)
	})
}

func (s *Store) GetLedgerAccount(_ context.Context, userID string, id uuid.UUID) (LedgerAccount, error) {
	var a LedgerAccount
	err := s.e.View(func(txn *badger.Txn) error {
		var err error
		a, err = loadLedgerAccountTxn(txn, userID, id)
		return err
	})
	return a, err
}

func (s *Store) FindLedgerAccountByIBAN(_ context.Context, userID string, iban string) (LedgerAccount, error) {
	var a LedgerAccount
	err := s.e.View(func(txn *badger.Txn) error {
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
		a, err = loadLedgerAccountTxn(txn, userID, accID)
		return err
	})
	if errors.Is(err, badger.ErrKeyNotFound) {
		return a, storage.ErrNotFound
	}
	return a, err
}

func (s *Store) CreateLedgerAccount(_ context.Context, userID string, a LedgerAccount) error {
	data, err := json.Marshal(a)
	if err != nil {
		return err
	}
	return s.e.Update(func(txn *badger.Txn) error {
		if a.IBAN != "" {
			if _, err := txn.Get(idxLedAccIBANKey(userID, a.IBAN)); err == nil {
				return storage.ErrConflict
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
