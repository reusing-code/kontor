package contracts

import (
	"context"

	"github.com/dgraph-io/badger/v4"
	"github.com/google/uuid"
	"github.com/reusing-code/kontor/backend/internal/storage"
	"github.com/reusing-code/kontor/backend/internal/storage/item"
	"github.com/reusing-code/kontor/backend/internal/storage/link"
)

type itemStore = item.Store[Contract]

// Store keeps the contracts on the shared item store (con/{id},
// idx/cat_con/{catId}/{id}); it is the link target of contract references.
type Store struct {
	*itemStore
	e *storage.Engine
}

func NewStore(e *storage.Engine, links *link.Registry) *Store {
	return &Store{
		itemStore: item.New(e, links, item.Spec[Contract]{
			Module:  ModuleID,
			Kind:    "con",
			Index:   "cat_con",
			RefType: link.RefContract,
			ID:      func(c Contract) uuid.UUID { return c.ID },
			Parent:  func(c Contract) uuid.UUID { return c.CategoryID },
			Links:   func(c *Contract) *[]uuid.UUID { return &c.LinkedTransactionIDs },
		}),
		e: e,
	}
}

func (s *Store) ListByCategory(ctx context.Context, userID string, categoryID uuid.UUID) ([]Contract, error) {
	return s.ListByParent(ctx, userID, categoryID)
}

// CategoryCascade deletes all contracts in a category with their ledger
// references, inside the given transaction. Registered with the shared
// categories store.
func (s *Store) CategoryCascade(txn *badger.Txn, userID string, categoryID uuid.UUID) error {
	return s.DeleteByParentTxn(txn, userID, categoryID)
}

func (s *Store) PruneDeadTransactionLinks(ctx context.Context, userID string) (int, error) {
	return s.PruneDeadLinks(ctx, userID)
}
