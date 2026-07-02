package contracts

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/dgraph-io/badger/v4"
	"github.com/google/uuid"
	"github.com/reusing-code/kontor/backend/internal/storage"
	"github.com/reusing-code/kontor/backend/internal/storage/link"
)

type Store struct {
	e     *storage.Engine
	links *link.Registry
}

func NewStore(e *storage.Engine, links *link.Registry) *Store {
	s := &Store{e: e, links: links}
	links.RegisterTarget(link.RefContract, s)
	return s
}

func conKey(userID string, id uuid.UUID) []byte {
	return []byte(fmt.Sprintf("u/%s/con/%s", userID, id))
}

func conPrefix(userID string) []byte {
	return []byte(fmt.Sprintf("u/%s/con/", userID))
}

func idxCatConKey(userID string, categoryID, contractID uuid.UUID) []byte {
	return []byte(fmt.Sprintf("u/%s/idx/cat_con/%s/%s", userID, categoryID, contractID))
}

func idxCatConPrefix(userID string, categoryID uuid.UUID) []byte {
	return []byte(fmt.Sprintf("u/%s/idx/cat_con/%s/", userID, categoryID))
}

func normalize(c Contract) Contract {
	c.LinkedTransactionIDs = link.NormalizeIDs(c.LinkedTransactionIDs)
	return c
}

func storeContract(txn *badger.Txn, userID string, c Contract) error {
	return storage.SetJSON(txn, conKey(userID, c.ID), normalize(c))
}

func (s *Store) List(_ context.Context, userID string) ([]Contract, error) {
	contracts := []Contract{}
	err := s.e.View(func(txn *badger.Txn) error {
		return storage.IteratePrefix(txn, conPrefix(userID), func(_, val []byte) error {
			var con Contract
			if err := json.Unmarshal(val, &con); err != nil {
				return err
			}
			contracts = append(contracts, normalize(con))
			return nil
		})
	})
	if err != nil {
		return nil, err
	}
	return contracts, nil
}

func (s *Store) ListByCategory(_ context.Context, userID string, categoryID uuid.UUID) ([]Contract, error) {
	contracts := []Contract{}
	err := s.e.View(func(txn *badger.Txn) error {
		prefix := idxCatConPrefix(userID, categoryID)
		opts := badger.DefaultIteratorOptions
		opts.Prefix = prefix
		opts.PrefetchValues = false
		it := txn.NewIterator(opts)
		defer it.Close()

		for it.Rewind(); it.Valid(); it.Next() {
			key := it.Item().Key()
			conID, err := uuid.Parse(string(key[len(prefix):]))
			if err != nil {
				continue
			}

			var con Contract
			if err := storage.GetJSON(txn, conKey(userID, conID), &con); err != nil {
				if err == storage.ErrNotFound {
					continue
				}
				return err
			}
			contracts = append(contracts, normalize(con))
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return contracts, nil
}

func (s *Store) Get(_ context.Context, userID string, id uuid.UUID) (Contract, error) {
	var con Contract
	err := s.e.View(func(txn *badger.Txn) error {
		return storage.GetJSON(txn, conKey(userID, id), &con)
	})
	return normalize(con), err
}

func (s *Store) Create(_ context.Context, userID string, c Contract) error {
	c = normalize(c)
	return s.e.Update(func(txn *badger.Txn) error {
		if err := storeContract(txn, userID, c); err != nil {
			return err
		}
		return txn.Set(idxCatConKey(userID, c.CategoryID, c.ID), []byte{})
	})
}

func (s *Store) Update(_ context.Context, userID string, c Contract) error {
	c = normalize(c)
	return s.e.Update(func(txn *badger.Txn) error {
		var old Contract
		if err := storage.GetJSON(txn, conKey(userID, c.ID), &old); err != nil {
			return err
		}

		if err := storeContract(txn, userID, c); err != nil {
			return err
		}

		if old.CategoryID != c.CategoryID {
			if err := txn.Delete(idxCatConKey(userID, old.CategoryID, c.ID)); err != nil {
				return err
			}
			if err := txn.Set(idxCatConKey(userID, c.CategoryID, c.ID), []byte{}); err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *Store) Delete(_ context.Context, userID string, id uuid.UUID) error {
	return s.e.Update(func(txn *badger.Txn) error {
		var con Contract
		if err := storage.GetJSON(txn, conKey(userID, id), &con); err != nil {
			return err
		}
		con = normalize(con)
		if err := s.links.RemoveReferencesTo(txn, userID, con.LinkedTransactionIDs, link.RefContract, id); err != nil {
			return err
		}

		if err := txn.Delete(conKey(userID, id)); err != nil {
			return err
		}
		return txn.Delete(idxCatConKey(userID, con.CategoryID, id))
	})
}

// AddLink implements link.Target: records a ledger transaction on a contract.
func (s *Store) AddLink(txn *badger.Txn, userID string, targetID, transactionID uuid.UUID) error {
	var con Contract
	if err := storage.GetJSON(txn, conKey(userID, targetID), &con); err != nil {
		return err
	}
	con = normalize(con)
	if !link.Contains(con.LinkedTransactionIDs, transactionID) {
		con.LinkedTransactionIDs = append(con.LinkedTransactionIDs, transactionID)
	}
	return storeContract(txn, userID, con)
}

// RemoveLink implements link.Target: drops a ledger transaction from a
// contract, tolerating a missing contract.
func (s *Store) RemoveLink(txn *badger.Txn, userID string, targetID, transactionID uuid.UUID) error {
	var con Contract
	if err := storage.GetJSON(txn, conKey(userID, targetID), &con); err != nil {
		if err == storage.ErrNotFound {
			return nil
		}
		return err
	}
	con = normalize(con)
	con.LinkedTransactionIDs = link.Without(con.LinkedTransactionIDs, transactionID)
	return storeContract(txn, userID, con)
}

// CategoryCascade deletes all contracts in a category, inside the given
// transaction. Registered with the shared categories store.
func (s *Store) CategoryCascade(txn *badger.Txn, userID string, categoryID uuid.UUID) error {
	prefix := idxCatConPrefix(userID, categoryID)
	opts := badger.DefaultIteratorOptions
	opts.Prefix = prefix
	opts.PrefetchValues = false
	it := txn.NewIterator(opts)

	var contractIDs []uuid.UUID
	for it.Rewind(); it.Valid(); it.Next() {
		key := it.Item().Key()
		conID, err := uuid.Parse(string(key[len(prefix):]))
		if err != nil {
			continue
		}
		contractIDs = append(contractIDs, conID)
	}
	it.Close()

	for _, cID := range contractIDs {
		if err := txn.Delete(conKey(userID, cID)); err != nil {
			return err
		}
		if err := txn.Delete(idxCatConKey(userID, categoryID, cID)); err != nil {
			return err
		}
	}
	return nil
}

// Exists implements link.Target.
func (s *Store) Exists(txn *badger.Txn, userID string, targetID uuid.UUID) bool {
	_, err := txn.Get(conKey(userID, targetID))
	return err == nil
}

// PruneDeadTransactionLinks drops linked-transaction IDs that do not resolve
// to an existing ledger transaction, returning how many were removed.
func (s *Store) PruneDeadTransactionLinks(_ context.Context, userID string) (int, error) {
	pruned := 0
	err := s.e.Update(func(txn *badger.Txn) error {
		var items []Contract
		if err := storage.IteratePrefix(txn, conPrefix(userID), func(_, val []byte) error {
			var c Contract
			if err := json.Unmarshal(val, &c); err != nil {
				return err
			}
			items = append(items, normalize(c))
			return nil
		}); err != nil {
			return err
		}
		for _, c := range items {
			kept := c.LinkedTransactionIDs[:0:0]
			for _, id := range c.LinkedTransactionIDs {
				if s.links.TransactionExists(txn, userID, id) {
					kept = append(kept, id)
				}
			}
			if len(kept) == len(c.LinkedTransactionIDs) {
				continue
			}
			pruned += len(c.LinkedTransactionIDs) - len(kept)
			c.LinkedTransactionIDs = kept
			if err := storeContract(txn, userID, c); err != nil {
				return err
			}
		}
		return nil
	})
	return pruned, err
}
