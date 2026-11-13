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
