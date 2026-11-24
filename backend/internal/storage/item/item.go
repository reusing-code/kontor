// Package item is the storage shared by the module items that ledger
// transactions can link to: contracts, purchases, vehicles and vehicle cost
// entries. A Store keeps one kind of item as JSON under
// u/{userId}/mod/{module}/{kind}/{id}, optionally indexed by a parent under
// u/{userId}/mod/{module}/idx/{index}/{parentId}/{id}, and maintains the
// item's linked transaction IDs together with the ledger side:
//
//   - Update keeps the stored links; they only change through AddLink and
//     RemoveLink, which the link registry calls.
//   - Every delete, including the cascade by parent, strips the ledger
//     references to the deleted items in the same badger transaction.
package item

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/dgraph-io/badger/v4"
	"github.com/google/uuid"
	"github.com/reusing-code/kontor/backend/internal/storage"
	"github.com/reusing-code/kontor/backend/internal/storage/link"
)

// Spec describes one kind of item.
type Spec[T any] struct {
	// Module is the module ID of the key prefix.
	Module string
	// Kind is the key segment of the items, e.g. "con".
	Kind string
	// Index is the key segment of the parent index, e.g. "cat_con"; empty
	// for items without a parent.
	Index string
	// RefType is the link reference type the store registers for.
	RefType string
	// ID returns the item's ID.
	ID func(T) uuid.UUID
	// Parent returns the item's parent ID; required with Index.
	Parent func(T) uuid.UUID
	// Links points at the item's linked transaction IDs.
	Links func(*T) *[]uuid.UUID
}

// Store keeps the items of one Spec. It is the link.Target of its RefType.
type Store[T any] struct {
	e     *storage.Engine
	links *link.Registry
	spec  Spec[T]
}

// New builds the store and registers it as the link target of spec.RefType.
func New[T any](e *storage.Engine, links *link.Registry, spec Spec[T]) *Store[T] {
	if spec.Module == "" || spec.Kind == "" || spec.RefType == "" || spec.ID == nil || spec.Links == nil {
		panic("item: incomplete spec for " + spec.Module + "/" + spec.Kind)
	}
	if (spec.Index == "") != (spec.Parent == nil) {
		panic("item: Index and Parent go together for " + spec.Module + "/" + spec.Kind)
	}
	s := &Store[T]{e: e, links: links, spec: spec}
	links.RegisterTarget(spec.RefType, s)
	return s
}

func (s *Store[T]) key(userID string, id uuid.UUID) []byte {
	return fmt.Appendf(nil, "u/%s/mod/%s/%s/%s", userID, s.spec.Module, s.spec.Kind, id)
}

func (s *Store[T]) prefix(userID string) []byte {
	return fmt.Appendf(nil, "u/%s/mod/%s/%s/", userID, s.spec.Module, s.spec.Kind)
}

func (s *Store[T]) indexKey(userID string, parentID, id uuid.UUID) []byte {
	return fmt.Appendf(nil, "u/%s/mod/%s/idx/%s/%s/%s", userID, s.spec.Module, s.spec.Index, parentID, id)
}

func (s *Store[T]) indexPrefix(userID string, parentID uuid.UUID) []byte {
	return fmt.Appendf(nil, "u/%s/mod/%s/idx/%s/%s/", userID, s.spec.Module, s.spec.Index, parentID)
}

func (s *Store[T]) indexed() bool { return s.spec.Index != "" }

func (s *Store[T]) normalize(item T) T {
	links := s.spec.Links(&item)
	*links = link.NormalizeIDs(*links)
	return item
}

func (s *Store[T]) get(txn *badger.Txn, userID string, id uuid.UUID) (T, error) {
	var item T
	if err := storage.GetJSON(txn, s.key(userID, id), &item); err != nil {
		return item, err
	}
	return s.normalize(item), nil
}

func (s *Store[T]) put(txn *badger.Txn, userID string, item T) error {
	return storage.SetJSON(txn, s.key(userID, s.spec.ID(item)), s.normalize(item))
}

// childIDs lists the item IDs indexed under parentID; index keys whose
// suffix is not a UUID are skipped.
func (s *Store[T]) childIDs(txn *badger.Txn, userID string, parentID uuid.UUID) []uuid.UUID {
	prefix := s.indexPrefix(userID, parentID)
	opts := badger.DefaultIteratorOptions
	opts.Prefix = prefix
	opts.PrefetchValues = false
	it := txn.NewIterator(opts)
	defer it.Close()

	var ids []uuid.UUID
	for it.Rewind(); it.Valid(); it.Next() {
		id, err := uuid.Parse(string(it.Item().Key()[len(prefix):]))
		if err != nil {
			continue
		}
		ids = append(ids, id)
	}
	return ids
}

func (s *Store[T]) listTxn(txn *badger.Txn, userID string) ([]T, error) {
	items := []T{}
	err := storage.IteratePrefix(txn, s.prefix(userID), func(_, val []byte) error {
		var item T
		if err := json.Unmarshal(val, &item); err != nil {
			return err
		}
		items = append(items, s.normalize(item))
		return nil
	})
	return items, err
}

// List returns all items of the user; never nil.
func (s *Store[T]) List(_ context.Context, userID string) ([]T, error) {
	var items []T
	err := s.e.View(func(txn *badger.Txn) error {
		var err error
		items, err = s.listTxn(txn, userID)
		return err
	})
	if err != nil {
		return nil, err
	}
	return items, nil
}

// ListByParent returns the items indexed under parentID; never nil. Index
// entries without an item are skipped.
func (s *Store[T]) ListByParent(_ context.Context, userID string, parentID uuid.UUID) ([]T, error) {
	items := []T{}
	err := s.e.View(func(txn *badger.Txn) error {
		for _, id := range s.childIDs(txn, userID, parentID) {
			item, err := s.get(txn, userID, id)
			if errors.Is(err, storage.ErrNotFound) {
				continue
			}
			if err != nil {
				return err
			}
			items = append(items, item)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return items, nil
}

// Get returns the item or storage.ErrNotFound.
func (s *Store[T]) Get(_ context.Context, userID string, id uuid.UUID) (T, error) {
	var item T
	err := s.e.View(func(txn *badger.Txn) error {
		var err error
		item, err = s.get(txn, userID, id)
		return err
	})
	return item, err
}

// Create stores a new item with its links as given (import restores them)
// and its parent index entry.
func (s *Store[T]) Create(_ context.Context, userID string, item T) error {
	return s.e.Update(func(txn *badger.Txn) error {
		if err := s.put(txn, userID, item); err != nil {
			return err
		}
		if !s.indexed() {
			return nil
		}
		return txn.Set(s.indexKey(userID, s.spec.Parent(item), s.spec.ID(item)), []byte{})
	})
}

// Update replaces a stored item, keeping its stored linked transaction IDs,
// and moves the index entry when the parent changed. A missing item is
// storage.ErrNotFound.
func (s *Store[T]) Update(_ context.Context, userID string, item T) error {
	return s.e.Update(func(txn *badger.Txn) error {
		id := s.spec.ID(item)
		old, err := s.get(txn, userID, id)
		if err != nil {
			return err
		}
		*s.spec.Links(&item) = *s.spec.Links(&old)
		if err := s.put(txn, userID, item); err != nil {
			return err
		}
		if !s.indexed() {
			return nil
		}
		oldParent, newParent := s.spec.Parent(old), s.spec.Parent(item)
		if oldParent == newParent {
			return nil
		}
		if err := txn.Delete(s.indexKey(userID, oldParent, id)); err != nil {
			return err
		}
		return txn.Set(s.indexKey(userID, newParent, id), []byte{})
	})
}

// Delete removes an item, its index entry and the ledger references to it.
func (s *Store[T]) Delete(_ context.Context, userID string, id uuid.UUID) error {
	return s.e.Update(func(txn *badger.Txn) error {
		return s.DeleteTxn(txn, userID, id)
	})
}

// DeleteTxn is Delete inside the caller's transaction.
func (s *Store[T]) DeleteTxn(txn *badger.Txn, userID string, id uuid.UUID) error {
	item, err := s.get(txn, userID, id)
	if err != nil {
		return err
	}
	if err := s.links.RemoveReferencesTo(txn, userID, *s.spec.Links(&item), s.spec.RefType, id); err != nil {
		return err
	}
	if err := txn.Delete(s.key(userID, id)); err != nil {
		return err
	}
	if !s.indexed() {
		return nil
	}
	return txn.Delete(s.indexKey(userID, s.spec.Parent(item), id))
}

// DeleteByParentTxn deletes every item indexed under parentID with its
// index entry and the ledger references to it, inside the caller's
// transaction (the cascade of a parent delete). Dangling index entries are
// removed too.
func (s *Store[T]) DeleteByParentTxn(txn *badger.Txn, userID string, parentID uuid.UUID) error {
	for _, id := range s.childIDs(txn, userID, parentID) {
		item, err := s.get(txn, userID, id)
		switch {
		case err == nil:
			if err := s.links.RemoveReferencesTo(txn, userID, *s.spec.Links(&item), s.spec.RefType, id); err != nil {
				return err
			}
		case !errors.Is(err, storage.ErrNotFound):
			return err
		}
		if err := txn.Delete(s.key(userID, id)); err != nil {
			return err
		}
		if err := txn.Delete(s.indexKey(userID, parentID, id)); err != nil {
			return err
		}
	}
	return nil
}

// AddLink implements link.Target: records a ledger transaction on an item.
func (s *Store[T]) AddLink(txn *badger.Txn, userID string, targetID, transactionID uuid.UUID) error {
	item, err := s.get(txn, userID, targetID)
	if err != nil {
		return err
	}
	links := s.spec.Links(&item)
	if !link.Contains(*links, transactionID) {
		*links = append(*links, transactionID)
	}
	return s.put(txn, userID, item)
}

// RemoveLink implements link.Target: drops a ledger transaction from an
// item, tolerating a missing item.
func (s *Store[T]) RemoveLink(txn *badger.Txn, userID string, targetID, transactionID uuid.UUID) error {
	item, err := s.get(txn, userID, targetID)
	if errors.Is(err, storage.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	links := s.spec.Links(&item)
	*links = link.Without(*links, transactionID)
	return s.put(txn, userID, item)
}

// Exists implements link.Target.
func (s *Store[T]) Exists(txn *badger.Txn, userID string, targetID uuid.UUID) bool {
	_, err := txn.Get(s.key(userID, targetID))
	return err == nil
}

// PruneDeadLinks drops linked transaction IDs that do not resolve to an
// existing ledger transaction and returns how many were removed.
func (s *Store[T]) PruneDeadLinks(_ context.Context, userID string) (int, error) {
	var pruned int
	err := s.e.Update(func(txn *badger.Txn) error {
		var err error
		pruned, err = s.PruneDeadLinksTxn(txn, userID)
		return err
	})
	return pruned, err
}

// PruneDeadLinksTxn is PruneDeadLinks inside the caller's transaction.
func (s *Store[T]) PruneDeadLinksTxn(txn *badger.Txn, userID string) (int, error) {
	items, err := s.listTxn(txn, userID)
	if err != nil {
		return 0, err
	}
	pruned := 0
	for _, item := range items {
		links := s.spec.Links(&item)
		kept := make([]uuid.UUID, 0, len(*links))
		for _, id := range *links {
			if s.links.TransactionExists(txn, userID, id) {
				kept = append(kept, id)
			}
		}
		if len(kept) == len(*links) {
			continue
		}
		pruned += len(*links) - len(kept)
		*links = kept
		if err := s.put(txn, userID, item); err != nil {
			return 0, err
		}
	}
	return pruned, nil
}
