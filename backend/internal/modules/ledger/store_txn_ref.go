package ledger

import (
	"context"
	"errors"
	"slices"
	"time"

	"github.com/dgraph-io/badger/v4"
	"github.com/google/uuid"
	"github.com/reusing-code/kontor/backend/internal/storage"
	"github.com/reusing-code/kontor/backend/internal/storage/link"
)

func (s *Store) syncLedgerReferences(txn *badger.Txn, userID string, transactionID uuid.UUID, oldReferences, newReferences []LedgerTransactionReference) error {
	remove := make(map[string]LedgerTransactionReference, len(oldReferences))
	for _, reference := range oldReferences {
		remove[reference.Type+":"+reference.TargetID.String()] = reference
	}
	add := make(map[string]LedgerTransactionReference, len(newReferences))
	for _, reference := range newReferences {
		key := reference.Type + ":" + reference.TargetID.String()
		add[key] = reference
		delete(remove, key)
	}
	for key := range add {
		if _, existed := remove[key]; existed {
			delete(remove, key)
			delete(add, key)
		}
	}
	for _, reference := range remove {
		target, err := s.links.Target(reference.Type)
		if err != nil {
			return err
		}
		if err := target.RemoveLink(txn, userID, reference.TargetID, transactionID); err != nil {
			return err
		}
	}
	for _, reference := range add {
		target, err := s.links.Target(reference.Type)
		if err != nil {
			return err
		}
		if err := target.AddLink(txn, userID, reference.TargetID, transactionID); err != nil {
			return err
		}
	}
	return nil
}

// TransactionExists implements link.TransactionSide.
func (s *Store) TransactionExists(txn *badger.Txn, userID string, id uuid.UUID) bool {
	_, err := loadLedgerTransaction(txn, userID, id)
	return err == nil
}

// RemoveReferences implements link.TransactionSide: strips references to a
// deleted item from the given transactions.
func (s *Store) RemoveReferences(txn *badger.Txn, userID string, transactionIDs []uuid.UUID, referenceType string, targetID uuid.UUID) error {
	for _, transactionID := range link.NormalizeIDs(transactionIDs) {
		ledgerTxn, err := loadLedgerTransaction(txn, userID, transactionID)
		if err != nil {
			if errors.Is(err, storage.ErrNotFound) {
				continue
			}
			return err
		}
		nextReferences := make([]LedgerTransactionReference, 0, len(ledgerTxn.References))
		for _, reference := range ledgerTxn.References {
			if reference.Type == referenceType && reference.TargetID == targetID {
				continue
			}
			nextReferences = append(nextReferences, reference)
		}
		if len(nextReferences) == len(ledgerTxn.References) {
			continue
		}
		if err := s.syncLedgerReferences(txn, userID, ledgerTxn.ID, ledgerTxn.References, nextReferences); err != nil {
			return err
		}
		ledgerTxn.References = nextReferences
		ledgerTxn.UpdatedAt = time.Now().UTC()
		if err := storeLedgerTransaction(txn, userID, ledgerTxn); err != nil {
			return err
		}
	}
	return nil
}

// AddLedgerTransactionReference appends one reference and the target's
// back-link in a single transaction. A reference that already exists leaves
// the transaction unchanged and reports added=false. It touches nothing else (category, transfer
// state, note, links), so linked internal transfers keep their protections.
func (s *Store) AddLedgerTransactionReference(ctx context.Context, userID string, id uuid.UUID, ref LedgerTransactionReference) (LedgerTransaction, bool, error) {
	var (
		result LedgerTransaction
		added  bool
	)
	err := storage.RetryConflicts(ctx, referenceRetries, func() error {
		var err error
		result, added, err = s.addLedgerTransactionReference(userID, id, ref)
		return err
	})
	if err != nil {
		return LedgerTransaction{}, false, err
	}
	return result, added, nil
}

const referenceRetries = 10

func (s *Store) addLedgerTransactionReference(userID string, id uuid.UUID, ref LedgerTransactionReference) (LedgerTransaction, bool, error) {
	var result LedgerTransaction
	added := false
	err := s.e.Update(func(txn *badger.Txn) error {
		ledgerTxn, err := loadLedgerTransaction(txn, userID, id)
		if err != nil {
			if errors.Is(err, storage.ErrNotFound) {
				return ErrTransactionNotFound
			}
			return err
		}
		if slices.Contains(ledgerTxn.References, ref) {
			result = ledgerTxn
			return nil
		}
		next := append(slices.Clone(ledgerTxn.References), ref)
		if err := s.syncLedgerReferences(txn, userID, ledgerTxn.ID, ledgerTxn.References, next); err != nil {
			if errors.Is(err, storage.ErrNotFound) {
				return ErrTargetNotFound
			}
			return err
		}
		ledgerTxn.References = next
		ledgerTxn.UpdatedAt = time.Now().UTC()
		if err := storeLedgerTransaction(txn, userID, ledgerTxn); err != nil {
			return err
		}
		result = ledgerTxn
		added = true
		return nil
	})
	return result, added && err == nil, err
}
