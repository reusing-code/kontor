// Package link connects ledger transactions with items in other modules
// (contracts, purchases, vehicles) without the modules importing each other.
// All operations run inside the caller's badger transaction so cross-module
// link maintenance stays atomic.
package link

import (
	"fmt"

	"github.com/dgraph-io/badger/v4"
	"github.com/google/uuid"
)

// Reference types for cross-module links; values match the ledger
// transaction reference type field.
const (
	RefContract = "contract"
	RefPurchase = "purchase"
	RefVehicle  = "vehicle"
)

// Target is implemented by module stores whose items can be referenced by
// ledger transactions.
type Target interface {
	// AddLink records transactionID on the target item; returns
	// storage.ErrNotFound if the item does not exist.
	AddLink(txn *badger.Txn, userID string, targetID, transactionID uuid.UUID) error
	// RemoveLink removes transactionID from the target item; missing items
	// are tolerated.
	RemoveLink(txn *badger.Txn, userID string, targetID, transactionID uuid.UUID) error
	// Exists reports whether the target item exists.
	Exists(txn *badger.Txn, userID string, targetID uuid.UUID) bool
}

// TransactionSide is implemented by the ledger store to strip references
// from transactions when a referenced item is deleted.
type TransactionSide interface {
	RemoveReferences(txn *badger.Txn, userID string, transactionIDs []uuid.UUID, refType string, targetID uuid.UUID) error
	// TransactionExists reports whether a ledger transaction exists.
	TransactionExists(txn *badger.Txn, userID string, id uuid.UUID) bool
}

type Registry struct {
	targets map[string]Target
	txnSide TransactionSide
}

func NewRegistry() *Registry {
	return &Registry{targets: map[string]Target{}}
}

func (r *Registry) RegisterTarget(refType string, t Target) {
	r.targets[refType] = t
}

func (r *Registry) SetTransactionSide(ts TransactionSide) {
	r.txnSide = ts
}

func (r *Registry) Target(refType string) (Target, error) {
	t, ok := r.targets[refType]
	if !ok {
		return nil, fmt.Errorf("no link target registered for reference type %q", refType)
	}
	return t, nil
}

// HasTarget reports whether a link target is registered for refType.
func (r *Registry) HasTarget(refType string) bool {
	_, ok := r.targets[refType]
	return ok
}

// TransactionExists reports whether a ledger transaction exists; false when
// no transaction side is registered.
func (r *Registry) TransactionExists(txn *badger.Txn, userID string, id uuid.UUID) bool {
	if r.txnSide == nil {
		return false
	}
	return r.txnSide.TransactionExists(txn, userID, id)
}

// RemoveReferencesTo strips references to a deleted item from the given
// transactions, inside the caller's transaction.
func (r *Registry) RemoveReferencesTo(txn *badger.Txn, userID string, transactionIDs []uuid.UUID, refType string, targetID uuid.UUID) error {
	if r.txnSide == nil {
		return nil
	}
	return r.txnSide.RemoveReferences(txn, userID, transactionIDs, refType, targetID)
}

// NormalizeIDs removes nil and duplicate UUIDs, preserving order.
func NormalizeIDs(ids []uuid.UUID) []uuid.UUID {
	if len(ids) == 0 {
		return []uuid.UUID{}
	}
	seen := make(map[uuid.UUID]struct{}, len(ids))
	normalized := make([]uuid.UUID, 0, len(ids))
	for _, id := range ids {
		if id == uuid.Nil {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		normalized = append(normalized, id)
	}
	return normalized
}

// Contains reports whether target is in ids.
func Contains(ids []uuid.UUID, target uuid.UUID) bool {
	for _, id := range ids {
		if id == target {
			return true
		}
	}
	return false
}

// Without returns ids with target removed, normalized.
func Without(ids []uuid.UUID, target uuid.UUID) []uuid.UUID {
	filtered := make([]uuid.UUID, 0, len(ids))
	for _, id := range ids {
		if id == target {
			continue
		}
		filtered = append(filtered, id)
	}
	return NormalizeIDs(filtered)
}
