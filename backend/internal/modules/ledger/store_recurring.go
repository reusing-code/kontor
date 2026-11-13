package ledger

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/dgraph-io/badger/v4"
	"github.com/google/uuid"
	"github.com/reusing-code/kontor/backend/internal/storage"
	"github.com/reusing-code/kontor/backend/internal/storage/link"
)

const (
	recurringStatusPending  = "pending"
	recurringStatusRejected = "rejected"
)

// recurringRecord remembers a proposed recurring group: the transactions the
// accept hook links, and a rejection, since deciding a draft frees its source
// and the detector would otherwise propose the group again.
type recurringRecord struct {
	TransactionIDs []uuid.UUID `json:"transactionIds"`
	Status         string      `json:"status"`
	CreatedAt      time.Time   `json:"createdAt"`
}

func ledRecurKey(userID, ref string) []byte {
	return []byte(fmt.Sprintf("u/%s/mod/ledger/recur/%s", userID, ref))
}

func validRecurringRef(ref string) bool {
	if len(ref) != recurringRefLength {
		return false
	}
	_, err := hex.DecodeString(ref)
	return err == nil
}

func (s *Store) getRecurringRecord(userID, ref string) (recurringRecord, bool, error) {
	var rec recurringRecord
	err := s.e.View(func(txn *badger.Txn) error {
		return storage.GetJSON(txn, ledRecurKey(userID, ref), &rec)
	})
	if errors.Is(err, storage.ErrNotFound) {
		return recurringRecord{}, false, nil
	}
	return rec, err == nil, err
}

func (s *Store) putRecurringRecord(userID, ref string, rec recurringRecord) error {
	return s.e.Update(func(txn *badger.Txn) error {
		return storage.SetJSON(txn, ledRecurKey(userID, ref), rec)
	})
}

func (s *Store) deleteRecurringRecord(userID, ref string) error {
	return s.e.Update(func(txn *badger.Txn) error {
		return txn.Delete(ledRecurKey(userID, ref))
	})
}

// rejectRecurringRecord marks a group as rejected, creating the record when
// it is missing so the group is never proposed again.
func (s *Store) rejectRecurringRecord(ctx context.Context, userID, ref string) error {
	return storage.RetryConflicts(ctx, referenceRetries, func() error {
		return s.e.Update(func(txn *badger.Txn) error {
			var rec recurringRecord
			err := storage.GetJSON(txn, ledRecurKey(userID, ref), &rec)
			if errors.Is(err, storage.ErrNotFound) {
				rec = recurringRecord{TransactionIDs: []uuid.UUID{}, CreatedAt: time.Now().UTC()}
			} else if err != nil {
				return err
			}
			rec.Status = recurringStatusRejected
			return storage.SetJSON(txn, ledRecurKey(userID, ref), rec)
		})
	})
}

// LinkLedgerTransactionsToTarget adds ref and the target's back-link to all
// given transactions in one transaction. Transactions that do not exist are
// returned in missing; ErrTargetNotFound when the target does not exist.
func (s *Store) LinkLedgerTransactionsToTarget(ctx context.Context, userID string, txnIDs []uuid.UUID, ref LedgerTransactionReference) (int, []uuid.UUID, error) {
	var (
		linked  int
		missing []uuid.UUID
	)
	err := storage.RetryConflicts(ctx, referenceRetries, func() error {
		var err error
		linked, missing, err = s.linkLedgerTransactionsToTarget(userID, txnIDs, ref)
		return err
	})
	if err != nil {
		return 0, nil, err
	}
	return linked, missing, nil
}

func (s *Store) linkLedgerTransactionsToTarget(userID string, txnIDs []uuid.UUID, ref LedgerTransactionReference) (int, []uuid.UUID, error) {
	linked := 0
	missing := []uuid.UUID{}
	err := s.e.Update(func(txn *badger.Txn) error {
		target, err := s.links.Target(ref.Type)
		if err != nil {
			return err
		}
		if !target.Exists(txn, userID, ref.TargetID) {
			return ErrTargetNotFound
		}
		now := time.Now().UTC()
		for _, id := range link.NormalizeIDs(txnIDs) {
			ledgerTxn, err := loadLedgerTransaction(txn, userID, id)
			if errors.Is(err, storage.ErrNotFound) {
				missing = append(missing, id)
				continue
			} else if err != nil {
				return err
			}
			if slices.Contains(ledgerTxn.References, ref) {
				continue
			}
			next := append(slices.Clone(ledgerTxn.References), ref)
			if err := s.syncLedgerReferences(txn, userID, ledgerTxn.ID, ledgerTxn.References, next); err != nil {
				if errors.Is(err, storage.ErrNotFound) {
					return ErrTargetNotFound
				}
				return err
			}
			ledgerTxn.References = next
			ledgerTxn.UpdatedAt = now
			if err := storeLedgerTransaction(txn, userID, ledgerTxn); err != nil {
				return err
			}
			linked++
		}
		return nil
	})
	return linked, missing, err
}
