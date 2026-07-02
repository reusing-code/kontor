package auto

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
	links.RegisterTarget(link.RefVehicle, s)
	return s
}

func vehKey(userID string, id uuid.UUID) []byte {
	return []byte(fmt.Sprintf("u/%s/mod/auto/veh/%s", userID, id))
}

func vehPrefix(userID string) []byte {
	return []byte(fmt.Sprintf("u/%s/mod/auto/veh/", userID))
}

func costKey(userID string, id uuid.UUID) []byte {
	return []byte(fmt.Sprintf("u/%s/mod/auto/cost/%s", userID, id))
}

func idxVehCostKey(userID string, vehicleID, costID uuid.UUID) []byte {
	return []byte(fmt.Sprintf("u/%s/mod/auto/idx/veh_cost/%s/%s", userID, vehicleID, costID))
}

func idxVehCostPrefix(userID string, vehicleID uuid.UUID) []byte {
	return []byte(fmt.Sprintf("u/%s/mod/auto/idx/veh_cost/%s/", userID, vehicleID))
}

func normalizeVehicle(v Vehicle) Vehicle {
	v.LinkedTransactionIDs = link.NormalizeIDs(v.LinkedTransactionIDs)
	return v
}

func storeVehicle(txn *badger.Txn, userID string, v Vehicle) error {
	return storage.SetJSON(txn, vehKey(userID, v.ID), normalizeVehicle(v))
}

func (s *Store) ListVehicles(_ context.Context, userID string) ([]Vehicle, error) {
	vehicles := []Vehicle{}
	err := s.e.View(func(txn *badger.Txn) error {
		return storage.IteratePrefix(txn, vehPrefix(userID), func(_, val []byte) error {
			var v Vehicle
			if err := json.Unmarshal(val, &v); err != nil {
				return err
			}
			vehicles = append(vehicles, normalizeVehicle(v))
			return nil
		})
	})
	if err != nil {
		return nil, err
	}
	return vehicles, nil
}

func (s *Store) GetVehicle(_ context.Context, userID string, id uuid.UUID) (Vehicle, error) {
	var v Vehicle
	err := s.e.View(func(txn *badger.Txn) error {
		return storage.GetJSON(txn, vehKey(userID, id), &v)
	})
	return normalizeVehicle(v), err
}

func (s *Store) CreateVehicle(_ context.Context, userID string, v Vehicle) error {
	v = normalizeVehicle(v)
	return s.e.Update(func(txn *badger.Txn) error {
		return storeVehicle(txn, userID, v)
	})
}

func (s *Store) UpdateVehicle(_ context.Context, userID string, v Vehicle) error {
	v = normalizeVehicle(v)
	return s.e.Update(func(txn *badger.Txn) error {
		var existing Vehicle
		if err := storage.GetJSON(txn, vehKey(userID, v.ID), &existing); err != nil {
			return err
		}
		return storeVehicle(txn, userID, v)
	})
}

func (s *Store) DeleteVehicle(_ context.Context, userID string, id uuid.UUID) error {
	return s.e.Update(func(txn *badger.Txn) error {
		var vehicle Vehicle
		if err := storage.GetJSON(txn, vehKey(userID, id), &vehicle); err != nil {
			return err
		}
		vehicle = normalizeVehicle(vehicle)
		if err := s.links.RemoveReferencesTo(txn, userID, vehicle.LinkedTransactionIDs, link.RefVehicle, id); err != nil {
			return err
		}

		if err := txn.Delete(vehKey(userID, id)); err != nil {
			return err
		}

		// Cascade delete all cost entries for this vehicle
		idxPrefix := idxVehCostPrefix(userID, id)
		opts := badger.DefaultIteratorOptions
		opts.Prefix = idxPrefix
		opts.PrefetchValues = false
		it := txn.NewIterator(opts)

		var costIDs []uuid.UUID
		for it.Rewind(); it.Valid(); it.Next() {
			key := it.Item().Key()
			cID, err := uuid.Parse(string(key[len(idxPrefix):]))
			if err != nil {
				continue
			}
			costIDs = append(costIDs, cID)
		}
		it.Close()

		for _, cID := range costIDs {
			if err := txn.Delete(costKey(userID, cID)); err != nil {
				return err
			}
			if err := txn.Delete(idxVehCostKey(userID, id, cID)); err != nil {
				return err
			}
		}
		return nil
	})
}

// AddLink implements link.Target: records a ledger transaction on a vehicle.
func (s *Store) AddLink(txn *badger.Txn, userID string, targetID, transactionID uuid.UUID) error {
	var v Vehicle
	if err := storage.GetJSON(txn, vehKey(userID, targetID), &v); err != nil {
		return err
	}
	v = normalizeVehicle(v)
	if !link.Contains(v.LinkedTransactionIDs, transactionID) {
		v.LinkedTransactionIDs = append(v.LinkedTransactionIDs, transactionID)
	}
	return storeVehicle(txn, userID, v)
}

// RemoveLink implements link.Target: drops a ledger transaction from a
// vehicle, tolerating a missing vehicle.
func (s *Store) RemoveLink(txn *badger.Txn, userID string, targetID, transactionID uuid.UUID) error {
	var v Vehicle
	if err := storage.GetJSON(txn, vehKey(userID, targetID), &v); err != nil {
		if err == storage.ErrNotFound {
			return nil
		}
		return err
	}
	v = normalizeVehicle(v)
	v.LinkedTransactionIDs = link.Without(v.LinkedTransactionIDs, transactionID)
	return storeVehicle(txn, userID, v)
}

func (s *Store) ListCostEntries(_ context.Context, userID string, vehicleID uuid.UUID) ([]CostEntry, error) {
	entries := []CostEntry{}
	err := s.e.View(func(txn *badger.Txn) error {
		prefix := idxVehCostPrefix(userID, vehicleID)
		opts := badger.DefaultIteratorOptions
		opts.Prefix = prefix
		opts.PrefetchValues = false
		it := txn.NewIterator(opts)
		defer it.Close()

		for it.Rewind(); it.Valid(); it.Next() {
			key := it.Item().Key()
			cID, err := uuid.Parse(string(key[len(prefix):]))
			if err != nil {
				continue
			}

			var c CostEntry
			if err := storage.GetJSON(txn, costKey(userID, cID), &c); err != nil {
				if err == storage.ErrNotFound {
					continue
				}
				return err
			}
			entries = append(entries, c)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return entries, nil
}

func (s *Store) GetCostEntry(_ context.Context, userID string, id uuid.UUID) (CostEntry, error) {
	var c CostEntry
	err := s.e.View(func(txn *badger.Txn) error {
		return storage.GetJSON(txn, costKey(userID, id), &c)
	})
	return c, err
}

func (s *Store) CreateCostEntry(_ context.Context, userID string, c CostEntry) error {
	return s.e.Update(func(txn *badger.Txn) error {
		if err := storage.SetJSON(txn, costKey(userID, c.ID), c); err != nil {
			return err
		}
		return txn.Set(idxVehCostKey(userID, c.VehicleID, c.ID), []byte{})
	})
}

func (s *Store) UpdateCostEntry(_ context.Context, userID string, c CostEntry) error {
	return s.e.Update(func(txn *badger.Txn) error {
		var old CostEntry
		if err := storage.GetJSON(txn, costKey(userID, c.ID), &old); err != nil {
			return err
		}

		if err := storage.SetJSON(txn, costKey(userID, c.ID), c); err != nil {
			return err
		}

		if old.VehicleID != c.VehicleID {
			if err := txn.Delete(idxVehCostKey(userID, old.VehicleID, c.ID)); err != nil {
				return err
			}
			if err := txn.Set(idxVehCostKey(userID, c.VehicleID, c.ID), []byte{}); err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *Store) DeleteCostEntry(_ context.Context, userID string, id uuid.UUID) error {
	return s.e.Update(func(txn *badger.Txn) error {
		var c CostEntry
		if err := storage.GetJSON(txn, costKey(userID, id), &c); err != nil {
			return err
		}

		if err := txn.Delete(costKey(userID, id)); err != nil {
			return err
		}
		return txn.Delete(idxVehCostKey(userID, c.VehicleID, id))
	})
}

// Exists implements link.Target.
func (s *Store) Exists(txn *badger.Txn, userID string, targetID uuid.UUID) bool {
	_, err := txn.Get(vehKey(userID, targetID))
	return err == nil
}

// PruneDeadTransactionLinks drops linked-transaction IDs that do not resolve
// to an existing ledger transaction, returning how many were removed.
func (s *Store) PruneDeadTransactionLinks(_ context.Context, userID string) (int, error) {
	pruned := 0
	err := s.e.Update(func(txn *badger.Txn) error {
		var items []Vehicle
		if err := storage.IteratePrefix(txn, vehPrefix(userID), func(_, val []byte) error {
			var v Vehicle
			if err := json.Unmarshal(val, &v); err != nil {
				return err
			}
			items = append(items, normalizeVehicle(v))
			return nil
		}); err != nil {
			return err
		}
		for _, v := range items {
			kept := v.LinkedTransactionIDs[:0:0]
			for _, id := range v.LinkedTransactionIDs {
				if s.links.TransactionExists(txn, userID, id) {
					kept = append(kept, id)
				}
			}
			if len(kept) == len(v.LinkedTransactionIDs) {
				continue
			}
			pruned += len(v.LinkedTransactionIDs) - len(kept)
			v.LinkedTransactionIDs = kept
			if err := storeVehicle(txn, userID, v); err != nil {
				return err
			}
		}
		return nil
	})
	return pruned, err
}
