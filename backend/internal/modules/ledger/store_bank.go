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

func ledBankConnKey(userID string, id uuid.UUID) []byte {
	return []byte(fmt.Sprintf("u/%s/mod/ledger/bconn/%s", userID, id))
}

func ledBankConnPrefix(userID string) []byte {
	return []byte(fmt.Sprintf("u/%s/mod/ledger/bconn/", userID))
}

func ledBankStateKey(userID string, hash string) []byte {
	return []byte(fmt.Sprintf("u/%s/mod/ledger/bstate/%s", userID, hash))
}

func ledBankStatePrefix(userID string) []byte {
	return []byte(fmt.Sprintf("u/%s/mod/ledger/bstate/", userID))
}

func loadLedgerBankConnection(txn *badger.Txn, userID string, id uuid.UUID) (LedgerBankConnection, error) {
	item, err := txn.Get(ledBankConnKey(userID, id))
	if err != nil {
		if errors.Is(err, badger.ErrKeyNotFound) {
			return LedgerBankConnection{}, storage.ErrNotFound
		}
		return LedgerBankConnection{}, err
	}
	var stored storableLedgerBankConnection
	err = item.Value(func(val []byte) error { return json.Unmarshal(val, &stored) })
	return LedgerBankConnection(stored), err
}

func storeLedgerBankConnection(txn *badger.Txn, userID string, c LedgerBankConnection) error {
	data, err := json.Marshal(storableLedgerBankConnection(c))
	if err != nil {
		return err
	}
	return txn.Set(ledBankConnKey(userID, c.ID), data)
}

func (s *Store) ListLedgerBankConnections(_ context.Context, userID string) ([]LedgerBankConnection, error) {
	items := []LedgerBankConnection{}
	err := s.e.View(func(txn *badger.Txn) error {
		it := txn.NewIterator(badger.DefaultIteratorOptions)
		defer it.Close()
		prefix := ledBankConnPrefix(userID)
		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			var stored storableLedgerBankConnection
			if err := it.Item().Value(func(val []byte) error { return json.Unmarshal(val, &stored) }); err != nil {
				return err
			}
			items = append(items, LedgerBankConnection(stored))
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(items, func(i, j int) bool {
		a, b := strings.ToLower(items[i].ASPSPName), strings.ToLower(items[j].ASPSPName)
		if a == b {
			return items[i].CreatedAt.Before(items[j].CreatedAt)
		}
		return a < b
	})
	return items, nil
}

func (s *Store) GetLedgerBankConnection(_ context.Context, userID string, id uuid.UUID) (LedgerBankConnection, error) {
	var c LedgerBankConnection
	err := s.e.View(func(txn *badger.Txn) error {
		var err error
		c, err = loadLedgerBankConnection(txn, userID, id)
		return err
	})
	return c, err
}

// FindLedgerBankConnectionByASPSP returns the user's connection to a bank
// (name compared case-insensitively), or storage.ErrNotFound.
func (s *Store) FindLedgerBankConnectionByASPSP(ctx context.Context, userID, name, country string) (LedgerBankConnection, error) {
	items, err := s.ListLedgerBankConnections(ctx, userID)
	if err != nil {
		return LedgerBankConnection{}, err
	}
	for _, c := range items {
		if strings.EqualFold(c.ASPSPName, name) && strings.EqualFold(c.ASPSPCountry, country) {
			return c, nil
		}
	}
	return LedgerBankConnection{}, storage.ErrNotFound
}

// PutLedgerBankConnection creates or replaces a connection.
func (s *Store) PutLedgerBankConnection(_ context.Context, userID string, c LedgerBankConnection) error {
	return s.e.Update(func(txn *badger.Txn) error {
		return storeLedgerBankConnection(txn, userID, c)
	})
}

// ModifyLedgerBankConnection applies fn to the stored connection in one
// transaction, so concurrent per-account state writes are not lost.
func (s *Store) ModifyLedgerBankConnection(_ context.Context, userID string, id uuid.UUID, fn func(*LedgerBankConnection)) (LedgerBankConnection, error) {
	var updated LedgerBankConnection
	err := retryOnConflict(func() error {
		return s.e.Update(func(txn *badger.Txn) error {
			c, err := loadLedgerBankConnection(txn, userID, id)
			if err != nil {
				return err
			}
			fn(&c)
			updated = c
			return storeLedgerBankConnection(txn, userID, c)
		})
	})
	return updated, err
}

func (s *Store) DeleteLedgerBankConnection(_ context.Context, userID string, id uuid.UUID) error {
	return s.e.Update(func(txn *badger.Txn) error {
		if _, err := txn.Get(ledBankConnKey(userID, id)); err != nil {
			if errors.Is(err, badger.ErrKeyNotFound) {
				return storage.ErrNotFound
			}
			return err
		}
		return txn.Delete(ledBankConnKey(userID, id))
	})
}

func (s *Store) CreateLedgerBankAuthState(_ context.Context, userID, hash string, state BankAuthState) error {
	data, err := json.Marshal(state)
	if err != nil {
		return err
	}
	return s.e.Update(func(txn *badger.Txn) error {
		return txn.Set(ledBankStateKey(userID, hash), data)
	})
}

// DeleteLedgerBankAuthState removes a pending state; a missing one is not an error.
func (s *Store) DeleteLedgerBankAuthState(_ context.Context, userID, hash string) error {
	return s.e.Update(func(txn *badger.Txn) error {
		return txn.Delete(ledBankStateKey(userID, hash))
	})
}

// ConsumeLedgerBankAuthState deletes the state and returns it. A missing,
// expired (deleted on touch) or already used state is ErrBankStateInvalid.
func (s *Store) ConsumeLedgerBankAuthState(_ context.Context, userID, hash string, now time.Time) (BankAuthState, error) {
	var state BankAuthState
	expired := false
	err := retryOnConflict(func() error {
		expired = false
		return s.e.Update(func(txn *badger.Txn) error {
			item, err := txn.Get(ledBankStateKey(userID, hash))
			if err != nil {
				if errors.Is(err, badger.ErrKeyNotFound) {
					return ErrBankStateInvalid
				}
				return err
			}
			if err := item.Value(func(val []byte) error { return json.Unmarshal(val, &state) }); err != nil {
				return err
			}
			if err := txn.Delete(ledBankStateKey(userID, hash)); err != nil {
				return err
			}
			expired = !now.Before(state.ExpiresAt)
			return nil
		})
	})
	if err != nil {
		return BankAuthState{}, err
	}
	if expired {
		return BankAuthState{}, ErrBankStateInvalid
	}
	return state, nil
}

// PruneLedgerBankAuthStates deletes expired states and returns how many.
func (s *Store) PruneLedgerBankAuthStates(_ context.Context, userID string, now time.Time) (int, error) {
	var keys [][]byte
	err := s.e.View(func(txn *badger.Txn) error {
		it := txn.NewIterator(badger.DefaultIteratorOptions)
		defer it.Close()
		prefix := ledBankStatePrefix(userID)
		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			var state BankAuthState
			if err := it.Item().Value(func(val []byte) error { return json.Unmarshal(val, &state) }); err != nil {
				return err
			}
			if !now.Before(state.ExpiresAt) {
				keys = append(keys, it.Item().KeyCopy(nil))
			}
		}
		return nil
	})
	if err != nil || len(keys) == 0 {
		return 0, err
	}
	err = s.e.Update(func(txn *badger.Txn) error {
		for _, k := range keys {
			if err := txn.Delete(k); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	return len(keys), nil
}

// bankCrossSourceDays is how far apart (in days) the booking dates of an API
// transaction and a CSV transaction may be for them to count as the same.
const bankCrossSourceDays = 2

func idxLedAccTxnPrefix(userID string, accountID uuid.UUID) []byte {
	return []byte(fmt.Sprintf("u/%s/mod/ledger/idx/acc_txn/%s/", userID, accountID))
}

func idxLedXDupKey(userID, apiFingerprint string) []byte {
	return []byte(fmt.Sprintf("u/%s/mod/ledger/idx/xdup/%s", userID, apiFingerprint))
}

func idxLedXAbsKey(userID string, txnID uuid.UUID) []byte {
	return []byte(fmt.Sprintf("u/%s/mod/ledger/idx/xabs/%s", userID, txnID))
}

// LedgerAccountBookingRange returns the newest stored booking date of an
// account that is not after notAfter (YYYY-MM-DD), and whether there is one.
// The sync passes today: a row dated in the future (bad bank data) must not
// become the anchor of the next window, or the gap before it would never be
// fetched again.
func (s *Store) LedgerAccountBookingRange(_ context.Context, userID string, accountID uuid.UUID, notAfter string) (string, bool, error) {
	prefix := idxLedAccTxnPrefix(userID, accountID)
	newest := ""
	err := s.e.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.PrefetchValues = false
		opts.Reverse = true
		it := txn.NewIterator(opts)
		defer it.Close()
		// Reverse iteration seeks to the last key at or below the argument.
		// Keys are prefix + date + "/" + id, and "0" sorts after "/", so
		// notAfter+"0" is above every key of that date and below the next date.
		it.Seek(append(append([]byte{}, prefix...), notAfter+"0"...))
		if !it.ValidForPrefix(prefix) {
			return nil
		}
		date, _, _ := strings.Cut(string(it.Item().Key()[len(prefix):]), "/")
		newest = date
		return nil
	})
	return newest, newest != "", err
}

type LedgerSyncCommitResult struct {
	Imported              int
	Duplicates            int
	CrossSourceDuplicates int
}

// CommitLedgerSync stores API-synced transactions in one transaction. A row
// whose fingerprint (or absorption) is known is a duplicate; the remaining rows
// are matched against free non-API transactions of the same account (equal
// signed amount and currency, booking dates at most bankCrossSourceDays
// apart, exact dates first) and absorbed into them instead of stored; the
// rest is inserted. The batch record accumulates counts across calls and is
// only written once something was imported.
func (s *Store) CommitLedgerSync(_ context.Context, userID string, batch LedgerImportBatch, txns []LedgerTransaction) (LedgerSyncCommitResult, error) {
	var result LedgerSyncCommitResult
	// The transaction reads candidate transactions the user may be reviewing at
	// the same time (the review queue edits them), so a lost optimistic
	// concurrency race is retried instead of failing the whole sync of an
	// account until the next interval. The closure resets its result.
	err := retryOnConflict(func() error {
		return s.e.Update(func(txn *badger.Txn) error {
			result = LedgerSyncCommitResult{}
			pending, err := pendingSyncRows(txn, userID, txns, &result)
			if err != nil {
				return err
			}
			pending, err = absorbCrossSource(txn, userID, pending, &result)
			if err != nil {
				return err
			}
			for _, t := range pending {
				if err := insertLedgerImportedTxn(txn, userID, batch.ID, t); err != nil {
					return err
				}
				result.Imported++
			}
			return upsertSyncBatch(txn, userID, batch, len(txns), result)
		})
	})
	if err != nil {
		return LedgerSyncCommitResult{}, err
	}
	return result, nil
}

func keyExists(txn *badger.Txn, key []byte) (bool, error) {
	_, err := txn.Get(key)
	switch {
	case err == nil:
		return true, nil
	case errors.Is(err, badger.ErrKeyNotFound):
		return false, nil
	default:
		return false, err
	}
}

// pendingSyncRows drops rows that are already known by fingerprint or
// absorption and rows repeated within the call.
func pendingSyncRows(txn *badger.Txn, userID string, txns []LedgerTransaction, result *LedgerSyncCommitResult) ([]LedgerTransaction, error) {
	seen := map[string]bool{}
	pending := make([]LedgerTransaction, 0, len(txns))
	for _, t := range txns {
		t = normalizeLedgerTransaction(t)
		known := seen[t.Fingerprint]
		if !known {
			var err error
			if known, err = keyExists(txn, idxLedTxnFPKey(userID, t.Fingerprint)); err != nil {
				return nil, err
			}
		}
		if !known {
			var err error
			if known, err = keyExists(txn, idxLedXDupKey(userID, t.Fingerprint)); err != nil {
				return nil, err
			}
		}
		if known {
			result.Duplicates++
			continue
		}
		seen[t.Fingerprint] = true
		pending = append(pending, t)
	}
	return pending, nil
}

func dateOffset(date string, days int) (string, bool) {
	d, err := time.Parse(time.DateOnly, date)
	if err != nil {
		return "", false
	}
	return d.AddDate(0, 0, days).Format(time.DateOnly), true
}

// absorbCrossSource matches pending rows against free non-API transactions in
// rounds of growing date distance and records each absorption.
func absorbCrossSource(txn *badger.Txn, userID string, pending []LedgerTransaction, result *LedgerSyncCommitResult) ([]LedgerTransaction, error) {
	if len(pending) == 0 {
		return pending, nil
	}
	type dayKey struct {
		account uuid.UUID
		date    string
	}
	cache := map[dayKey][]LedgerTransaction{}
	load := func(account uuid.UUID, date string) ([]LedgerTransaction, error) {
		k := dayKey{account, date}
		if c, ok := cache[k]; ok {
			return c, nil
		}
		prefix := []byte(fmt.Sprintf("%s%s/", idxLedAccTxnPrefix(userID, account), date))
		var ids []uuid.UUID
		opts := badger.DefaultIteratorOptions
		opts.PrefetchValues = false
		it := txn.NewIterator(opts)
		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			id, err := uuid.Parse(string(it.Item().Key()[len(prefix):]))
			if err == nil {
				ids = append(ids, id)
			}
		}
		it.Close()
		var found []LedgerTransaction
		for _, id := range ids {
			item, err := txn.Get(ledTxnKey(userID, id))
			if err != nil {
				return nil, err
			}
			var t LedgerTransaction
			if err := item.Value(func(val []byte) error { return json.Unmarshal(val, &t) }); err != nil {
				return nil, err
			}
			if t.SourceType == string(SourceEnableBanking) {
				continue
			}
			absorbed, err := keyExists(txn, idxLedXAbsKey(userID, t.ID))
			if err != nil {
				return nil, err
			}
			if !absorbed {
				found = append(found, t)
			}
		}
		sort.Slice(found, func(i, j int) bool { return found[i].ID.String() < found[j].ID.String() })
		cache[k] = found
		return found, nil
	}

	used := map[uuid.UUID]bool{}
	absorbedRow := make([]bool, len(pending))
	for d := 0; d <= bankCrossSourceDays; d++ {
		for i, row := range pending {
			if absorbedRow[i] {
				continue
			}
			offsets := []int{-d, d}
			if d == 0 {
				offsets = []int{0}
			}
			matched := false
			for _, off := range offsets {
				date, ok := dateOffset(row.BookingDate, off)
				if !ok {
					continue
				}
				candidates, err := load(row.AccountID, date)
				if err != nil {
					return nil, err
				}
				for _, c := range candidates {
					if used[c.ID] || c.AmountMinor != row.AmountMinor || !strings.EqualFold(c.Currency, row.Currency) {
						continue
					}
					if err := txn.Set(idxLedXDupKey(userID, row.Fingerprint), []byte(c.ID.String())); err != nil {
						return nil, err
					}
					if err := txn.Set(idxLedXAbsKey(userID, c.ID), []byte(row.Fingerprint)); err != nil {
						return nil, err
					}
					used[c.ID] = true
					absorbedRow[i] = true
					result.CrossSourceDuplicates++
					matched = true
					break
				}
				if matched {
					break
				}
			}
		}
	}

	rest := pending[:0:0]
	for i, row := range pending {
		if !absorbedRow[i] {
			rest = append(rest, row)
		}
	}
	return rest, nil
}

func upsertSyncBatch(txn *badger.Txn, userID string, batch LedgerImportBatch, rows int, result LedgerSyncCommitResult) error {
	stored := batch
	stored.TotalRows, stored.ImportedRows, stored.DuplicateRows = 0, 0, 0
	exists := false
	item, err := txn.Get(ledImpKey(userID, batch.ID))
	switch {
	case err == nil:
		exists = true
		if err := item.Value(func(val []byte) error { return json.Unmarshal(val, &stored) }); err != nil {
			return err
		}
		if len(stored.Warnings) == 0 {
			stored.Warnings = batch.Warnings
		}
		stored.UpdatedAt = batch.UpdatedAt
	case !errors.Is(err, badger.ErrKeyNotFound):
		return err
	}
	if !exists && result.Imported == 0 {
		return nil
	}
	stored.TotalRows += rows
	stored.ImportedRows += result.Imported
	stored.DuplicateRows += result.Duplicates + result.CrossSourceDuplicates
	data, err := json.Marshal(stored)
	if err != nil {
		return err
	}
	return txn.Set(ledImpKey(userID, batch.ID), data)
}
