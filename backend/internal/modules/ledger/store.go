package ledger

import (
	"context"
	"errors"

	"github.com/dgraph-io/badger/v4"
	"github.com/google/uuid"
	"github.com/reusing-code/kontor/backend/internal/storage"
	"github.com/reusing-code/kontor/backend/internal/storage/link"
)

var (
	ErrLedgerPreviewExpired   = storage.ErrLedgerPreviewExpired
	ErrLedgerFileImported     = storage.ErrLedgerFileImported
	ErrLedgerCategoryHasChild = storage.ErrLedgerCategoryHasChild
	ErrLedgerCategoryHasCycle = storage.ErrLedgerCategoryHasCycle
	ErrLedgerTransferInvalid  = storage.ErrLedgerTransferInvalid
	ErrLedgerTransferLinked   = storage.ErrLedgerTransferLinked
)

type Store struct {
	e     *storage.Engine
	links *link.Registry
}

// NewStore builds the ledger store and registers it as the transaction side
// of the link registry.
func NewStore(e *storage.Engine, links *link.Registry) *Store {
	s := &Store{e: e, links: links}
	links.SetTransactionSide(s)
	return s
}

func containsUUID(ids []uuid.UUID, target uuid.UUID) bool {
	for _, id := range ids {
		if id == target {
			return true
		}
	}
	return false
}

func withoutUUID(ids []uuid.UUID, target uuid.UUID) []uuid.UUID {
	filtered := make([]uuid.UUID, 0, len(ids))
	for _, id := range ids {
		if id == target {
			continue
		}
		filtered = append(filtered, id)
	}
	return link.NormalizeIDs(filtered)
}

func deleteIfPresent(txn *badger.Txn, key []byte) error {
	_, err := txn.Get(key)
	if errors.Is(err, badger.ErrKeyNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	return txn.Delete(key)
}

const conflictAttempts = 3

// retryConflicts reruns a write that lost an optimistic-concurrency race
// against a parallel scan, sync or request. Writes finish even after the
// caller's context ended (a scan at its time limit still records what it
// found), so cancellation is not checked between attempts.
func retryConflicts(ctx context.Context, fn func() error) error {
	return storage.RetryConflicts(context.WithoutCancel(ctx), conflictAttempts, fn)
}
