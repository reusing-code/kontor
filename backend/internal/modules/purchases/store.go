package purchases

import (
	"context"

	"github.com/dgraph-io/badger/v4"
	"github.com/google/uuid"
	"github.com/reusing-code/kontor/backend/internal/storage"
	"github.com/reusing-code/kontor/backend/internal/storage/item"
	"github.com/reusing-code/kontor/backend/internal/storage/link"
)

type itemStore = item.Store[Purchase]

// Store keeps the purchases on the shared item store (pur/{id},
// idx/cat_pur/{catId}/{id}); it is the link target of purchase references.
type Store struct {
	*itemStore
	e *storage.Engine
}

func NewStore(e *storage.Engine, links *link.Registry) *Store {
	return &Store{
		itemStore: item.New(e, links, item.Spec[Purchase]{
			Module:  ModuleID,
			Kind:    "pur",
			Index:   "cat_pur",
			RefType: link.RefPurchase,
			ID:      func(p Purchase) uuid.UUID { return p.ID },
			Parent:  func(p Purchase) uuid.UUID { return p.CategoryID },
			Links:   func(p *Purchase) *[]uuid.UUID { return &p.LinkedTransactionIDs },
		}),
		e: e,
	}
}

func (s *Store) ListByCategory(ctx context.Context, userID string, categoryID uuid.UUID) ([]Purchase, error) {
	return s.ListByParent(ctx, userID, categoryID)
}

// CategoryCascade deletes all purchases in a category with their ledger
// references, inside the given transaction. Registered with the shared
// categories store.
func (s *Store) CategoryCascade(txn *badger.Txn, userID string, categoryID uuid.UUID) error {
	return s.DeleteByParentTxn(txn, userID, categoryID)
}

func (s *Store) PruneDeadTransactionLinks(ctx context.Context, userID string) (int, error) {
	return s.PruneDeadLinks(ctx, userID)
}
