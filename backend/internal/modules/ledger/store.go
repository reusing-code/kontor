package ledger

import (
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

// retryOnConflict reruns a transaction that lost an optimistic-concurrency
// race; the rerun sees the committed state of the winner.
func retryOnConflict(fn func() error) error {
	var err error
	for range 3 {
		if err = fn(); !errors.Is(err, badger.ErrConflict) {
			return err
		}
	}
	return err
}
