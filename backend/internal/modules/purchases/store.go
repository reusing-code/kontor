package purchases

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
	links.RegisterTarget(link.RefPurchase, s)
	return s
}

func purKey(userID string, id uuid.UUID) []byte {
	return []byte(fmt.Sprintf("u/%s/pur/%s", userID, id))
}

func purPrefix(userID string) []byte {
	return []byte(fmt.Sprintf("u/%s/pur/", userID))
}

func idxCatPurKey(userID string, categoryID, purchaseID uuid.UUID) []byte {
	return []byte(fmt.Sprintf("u/%s/idx/cat_pur/%s/%s", userID, categoryID, purchaseID))
}

func idxCatPurPrefix(userID string, categoryID uuid.UUID) []byte {
	return []byte(fmt.Sprintf("u/%s/idx/cat_pur/%s/", userID, categoryID))
}

func normalize(p Purchase) Purchase {
	p.LinkedTransactionIDs = link.NormalizeIDs(p.LinkedTransactionIDs)
	return p
}

func storePurchase(txn *badger.Txn, userID string, p Purchase) error {
	return storage.SetJSON(txn, purKey(userID, p.ID), normalize(p))
}

func (s *Store) List(_ context.Context, userID string) ([]Purchase, error) {
	purchases := []Purchase{}
	err := s.e.View(func(txn *badger.Txn) error {
		return storage.IteratePrefix(txn, purPrefix(userID), func(_, val []byte) error {
			var p Purchase
			if err := json.Unmarshal(val, &p); err != nil {
				return err
			}
			purchases = append(purchases, normalize(p))
			return nil
		})
	})
	if err != nil {
		return nil, err
	}
	return purchases, nil
}

func (s *Store) ListByCategory(_ context.Context, userID string, categoryID uuid.UUID) ([]Purchase, error) {
	purchases := []Purchase{}
	err := s.e.View(func(txn *badger.Txn) error {
		prefix := idxCatPurPrefix(userID, categoryID)
		opts := badger.DefaultIteratorOptions
		opts.Prefix = prefix
		opts.PrefetchValues = false
		it := txn.NewIterator(opts)
		defer it.Close()

		for it.Rewind(); it.Valid(); it.Next() {
			key := it.Item().Key()
			pID, err := uuid.Parse(string(key[len(prefix):]))
			if err != nil {
				continue
			}

			var p Purchase
			if err := storage.GetJSON(txn, purKey(userID, pID), &p); err != nil {
				if err == storage.ErrNotFound {
					continue
				}
				return err
			}
			purchases = append(purchases, normalize(p))
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return purchases, nil
}

func (s *Store) Get(_ context.Context, userID string, id uuid.UUID) (Purchase, error) {
	var p Purchase
	err := s.e.View(func(txn *badger.Txn) error {
		return storage.GetJSON(txn, purKey(userID, id), &p)
	})
	return normalize(p), err
}

func (s *Store) Create(_ context.Context, userID string, p Purchase) error {
	p = normalize(p)
	return s.e.Update(func(txn *badger.Txn) error {
		if err := storePurchase(txn, userID, p); err != nil {
			return err
		}
		return txn.Set(idxCatPurKey(userID, p.CategoryID, p.ID), []byte{})
	})
}

func (s *Store) Update(_ context.Context, userID string, p Purchase) error {
	p = normalize(p)
	return s.e.Update(func(txn *badger.Txn) error {
		var old Purchase
		if err := storage.GetJSON(txn, purKey(userID, p.ID), &old); err != nil {
			return err
		}

		if err := storePurchase(txn, userID, p); err != nil {
			return err
		}

		if old.CategoryID != p.CategoryID {
			if err := txn.Delete(idxCatPurKey(userID, old.CategoryID, p.ID)); err != nil {
				return err
			}
			if err := txn.Set(idxCatPurKey(userID, p.CategoryID, p.ID), []byte{}); err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *Store) Delete(_ context.Context, userID string, id uuid.UUID) error {
	return s.e.Update(func(txn *badger.Txn) error {
		var p Purchase
		if err := storage.GetJSON(txn, purKey(userID, id), &p); err != nil {
			return err
		}
		p = normalize(p)
		if err := s.links.RemoveReferencesTo(txn, userID, p.LinkedTransactionIDs, link.RefPurchase, id); err != nil {
			return err
		}

		if err := txn.Delete(purKey(userID, id)); err != nil {
			return err
		}
		return txn.Delete(idxCatPurKey(userID, p.CategoryID, id))
	})
}

// AddLink implements link.Target: records a ledger transaction on a purchase.
func (s *Store) AddLink(txn *badger.Txn, userID string, targetID, transactionID uuid.UUID) error {
	var p Purchase
	if err := storage.GetJSON(txn, purKey(userID, targetID), &p); err != nil {
		return err
	}
	p = normalize(p)
	if !link.Contains(p.LinkedTransactionIDs, transactionID) {
		p.LinkedTransactionIDs = append(p.LinkedTransactionIDs, transactionID)
	}
	return storePurchase(txn, userID, p)
}

// RemoveLink implements link.Target: drops a ledger transaction from a
// purchase, tolerating a missing purchase.
func (s *Store) RemoveLink(txn *badger.Txn, userID string, targetID, transactionID uuid.UUID) error {
	var p Purchase
	if err := storage.GetJSON(txn, purKey(userID, targetID), &p); err != nil {
		if err == storage.ErrNotFound {
			return nil
		}
		return err
	}
	p = normalize(p)
	p.LinkedTransactionIDs = link.Without(p.LinkedTransactionIDs, transactionID)
	return storePurchase(txn, userID, p)
}

// CategoryCascade deletes all purchases in a category, inside the given
// transaction. Registered with the shared categories store.
func (s *Store) CategoryCascade(txn *badger.Txn, userID string, categoryID uuid.UUID) error {
	prefix := idxCatPurPrefix(userID, categoryID)
	opts := badger.DefaultIteratorOptions
	opts.Prefix = prefix
	opts.PrefetchValues = false
	it := txn.NewIterator(opts)

	var purchaseIDs []uuid.UUID
	for it.Rewind(); it.Valid(); it.Next() {
		key := it.Item().Key()
		pID, err := uuid.Parse(string(key[len(prefix):]))
		if err != nil {
			continue
		}
		purchaseIDs = append(purchaseIDs, pID)
	}
	it.Close()

	for _, pID := range purchaseIDs {
		if err := txn.Delete(purKey(userID, pID)); err != nil {
			return err
		}
		if err := txn.Delete(idxCatPurKey(userID, categoryID, pID)); err != nil {
			return err
		}
	}
	return nil
}

// Exists implements link.Target.
func (s *Store) Exists(txn *badger.Txn, userID string, targetID uuid.UUID) bool {
	_, err := txn.Get(purKey(userID, targetID))
	return err == nil
}

// PruneDeadTransactionLinks drops linked-transaction IDs that do not resolve
// to an existing ledger transaction, returning how many were removed.
func (s *Store) PruneDeadTransactionLinks(_ context.Context, userID string) (int, error) {
	pruned := 0
	err := s.e.Update(func(txn *badger.Txn) error {
		var items []Purchase
		if err := storage.IteratePrefix(txn, purPrefix(userID), func(_, val []byte) error {
			var p Purchase
			if err := json.Unmarshal(val, &p); err != nil {
				return err
			}
			items = append(items, normalize(p))
			return nil
		}); err != nil {
			return err
		}
		for _, p := range items {
			kept := p.LinkedTransactionIDs[:0:0]
			for _, id := range p.LinkedTransactionIDs {
				if s.links.TransactionExists(txn, userID, id) {
					kept = append(kept, id)
				}
			}
			if len(kept) == len(p.LinkedTransactionIDs) {
				continue
			}
			pruned += len(p.LinkedTransactionIDs) - len(kept)
			p.LinkedTransactionIDs = kept
			if err := storePurchase(txn, userID, p); err != nil {
				return err
			}
		}
		return nil
	})
	return pruned, err
}
