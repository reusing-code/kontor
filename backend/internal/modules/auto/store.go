package auto

import (
	"context"

	"github.com/dgraph-io/badger/v4"
	"github.com/google/uuid"
	"github.com/reusing-code/kontor/backend/internal/storage"
	"github.com/reusing-code/kontor/backend/internal/storage/item"
	"github.com/reusing-code/kontor/backend/internal/storage/link"
)

// Store keeps vehicles (veh/{id}) and their cost entries (cost/{id},
// idx/veh_cost/{vehId}/{costId}) on two shared item stores, the link targets
// of vehicle and cost references.
type Store struct {
	e        *storage.Engine
	vehicles *item.Store[Vehicle]
	costs    *item.Store[CostEntry]
}

func NewStore(e *storage.Engine, links *link.Registry) *Store {
	return &Store{
		e: e,
		vehicles: item.New(e, links, item.Spec[Vehicle]{
			Module:  ModuleID,
			Kind:    "veh",
			RefType: link.RefVehicle,
			ID:      func(v Vehicle) uuid.UUID { return v.ID },
			Links:   func(v *Vehicle) *[]uuid.UUID { return &v.LinkedTransactionIDs },
		}),
		costs: item.New(e, links, item.Spec[CostEntry]{
			Module:  ModuleID,
			Kind:    "cost",
			Index:   "veh_cost",
			RefType: link.RefCost,
			ID:      func(c CostEntry) uuid.UUID { return c.ID },
			Parent:  func(c CostEntry) uuid.UUID { return c.VehicleID },
			Links:   func(c *CostEntry) *[]uuid.UUID { return &c.LinkedTransactionIDs },
		}),
	}
}

func (s *Store) ListVehicles(ctx context.Context, userID string) ([]Vehicle, error) {
	return s.vehicles.List(ctx, userID)
}

func (s *Store) GetVehicle(ctx context.Context, userID string, id uuid.UUID) (Vehicle, error) {
	return s.vehicles.Get(ctx, userID, id)
}

func (s *Store) CreateVehicle(ctx context.Context, userID string, v Vehicle) error {
	return s.vehicles.Create(ctx, userID, v)
}

// UpdateVehicle keeps the stored linked transaction IDs; they are only
// changed through the link registry.
func (s *Store) UpdateVehicle(ctx context.Context, userID string, v Vehicle) error {
	return s.vehicles.Update(ctx, userID, v)
}

// DeleteVehicle deletes the vehicle and all its cost entries, with the
// ledger references to each of them, in one transaction.
func (s *Store) DeleteVehicle(_ context.Context, userID string, id uuid.UUID) error {
	return s.e.Update(func(txn *badger.Txn) error {
		if err := s.vehicles.DeleteTxn(txn, userID, id); err != nil {
			return err
		}
		return s.costs.DeleteByParentTxn(txn, userID, id)
	})
}

func (s *Store) ListCostEntries(ctx context.Context, userID string, vehicleID uuid.UUID) ([]CostEntry, error) {
	return s.costs.ListByParent(ctx, userID, vehicleID)
}

func (s *Store) GetCostEntry(ctx context.Context, userID string, id uuid.UUID) (CostEntry, error) {
	return s.costs.Get(ctx, userID, id)
}

func (s *Store) CreateCostEntry(ctx context.Context, userID string, c CostEntry) error {
	return s.costs.Create(ctx, userID, c)
}

// UpdateCostEntry keeps the stored linked transaction IDs; they are only
// changed through the link registry.
func (s *Store) UpdateCostEntry(ctx context.Context, userID string, c CostEntry) error {
	return s.costs.Update(ctx, userID, c)
}

func (s *Store) DeleteCostEntry(ctx context.Context, userID string, id uuid.UUID) error {
	return s.costs.Delete(ctx, userID, id)
}

// PruneDeadTransactionLinks drops linked-transaction IDs of vehicles and cost
// entries that do not resolve to an existing ledger transaction, returning
// how many were removed.
func (s *Store) PruneDeadTransactionLinks(_ context.Context, userID string) (int, error) {
	pruned := 0
	err := s.e.Update(func(txn *badger.Txn) error {
		for _, prune := range []func(*badger.Txn, string) (int, error){s.vehicles.PruneDeadLinksTxn, s.costs.PruneDeadLinksTxn} {
			n, err := prune(txn, userID)
			if err != nil {
				return err
			}
			pruned += n
		}
		return nil
	})
	return pruned, err
}
