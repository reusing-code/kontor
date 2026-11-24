package auto

import (
	"context"
	"errors"
	"log/slog"
	"slices"
	"testing"
	"time"

	"github.com/dgraph-io/badger/v4"
	"github.com/google/uuid"
	"github.com/reusing-code/kontor/backend/internal/storage"
	"github.com/reusing-code/kontor/backend/internal/storage/link"
)

const (
	storeUser     = "00000000-0000-0000-0000-0000000000c1"
	storeTestUser = "test-user"
)

type removedRefs struct {
	transactionIDs []uuid.UUID
	refType        string
	targetID       uuid.UUID
}

// fakeTxnSide stands in for the ledger: it records reference removals and
// knows a fixed set of transactions.
type fakeTxnSide struct {
	removed  []removedRefs
	existing map[uuid.UUID]bool
	failWith error
}

func (f *fakeTxnSide) RemoveReferences(_ *badger.Txn, _ string, transactionIDs []uuid.UUID, refType string, targetID uuid.UUID) error {
	if f.failWith != nil {
		return f.failWith
	}
	f.removed = append(f.removed, removedRefs{slices.Clone(transactionIDs), refType, targetID})
	return nil
}

func (f *fakeTxnSide) TransactionExists(_ *badger.Txn, _ string, id uuid.UUID) bool {
	return f.existing[id]
}

func (f *fakeTxnSide) removedFor(refType string, targetID uuid.UUID) (removedRefs, bool) {
	for _, r := range f.removed {
		if r.refType == refType && r.targetID == targetID {
			return r, true
		}
	}
	return removedRefs{}, false
}

type storeEnv struct {
	store  *Store
	engine *storage.Engine
	links  *link.Registry
	side   *fakeTxnSide
}

func newStoreEnv(t *testing.T) storeEnv {
	t.Helper()
	engine, err := storage.Open(t.TempDir(), slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { engine.Close() })
	links := link.NewRegistry()
	side := &fakeTxnSide{existing: map[uuid.UUID]bool{}}
	links.SetTransactionSide(side)
	return storeEnv{store: NewStore(engine, links), engine: engine, links: links, side: side}
}

func (e storeEnv) vehicle(t *testing.T, name string, linked ...uuid.UUID) Vehicle {
	t.Helper()
	now := time.Now().UTC()
	v := Vehicle{ID: uuid.New(), Name: name, LinkedTransactionIDs: linked, CreatedAt: now, UpdatedAt: now}
	if err := e.store.CreateVehicle(t.Context(), storeUser, v); err != nil {
		t.Fatal(err)
	}
	return v
}

func (e storeEnv) cost(t *testing.T, vehicleID uuid.UUID, linked ...uuid.UUID) CostEntry {
	t.Helper()
	now := time.Now().UTC()
	c := CostEntry{ID: uuid.New(), VehicleID: vehicleID, Type: CostTypeService, Amount: fp(100), Date: "2026-01-01", LinkedTransactionIDs: linked, CreatedAt: now, UpdatedAt: now}
	if err := e.store.CreateCostEntry(t.Context(), storeUser, c); err != nil {
		t.Fatal(err)
	}
	return c
}

func (e storeEnv) hasKey(t *testing.T, key []byte) bool {
	t.Helper()
	found := false
	if err := e.engine.View(func(txn *badger.Txn) error {
		_, err := txn.Get(key)
		found = err == nil
		if err != nil && !errors.Is(err, badger.ErrKeyNotFound) {
			return err
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	return found
}

func (e storeEnv) update(t *testing.T, fn func(txn *badger.Txn) error) error {
	t.Helper()
	return e.engine.Update(fn)
}

func TestStore_ListAndGetVehicles(t *testing.T) {
	env := newStoreEnv(t)
	ctx := t.Context()
	list, err := env.store.ListVehicles(ctx, storeUser)
	if err != nil || list == nil || len(list) != 0 {
		t.Fatalf("ListVehicles on empty store = %v, %v; want empty non-nil list", list, err)
	}

	txnID := uuid.New()
	a := env.vehicle(t, "A", txnID, txnID, uuid.Nil)
	env.vehicle(t, "B")

	got, err := env.store.GetVehicle(ctx, storeUser, a.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "A" || !slices.Equal(got.LinkedTransactionIDs, []uuid.UUID{txnID}) {
		t.Errorf("GetVehicle = %+v, want name A with links normalized to [%s]", got, txnID)
	}
	if list, _ = env.store.ListVehicles(ctx, storeUser); len(list) != 2 {
		t.Errorf("ListVehicles = %d vehicles, want 2", len(list))
	}
	if other, _ := env.store.ListVehicles(ctx, userB); len(other) != 0 {
		t.Errorf("ListVehicles of another user = %d vehicles, want 0", len(other))
	}
	if _, err := env.store.GetVehicle(ctx, storeUser, uuid.New()); !errors.Is(err, storage.ErrNotFound) {
		t.Errorf("GetVehicle unknown = %v, want ErrNotFound", err)
	}
}

func TestStore_UpdateVehicle(t *testing.T) {
	env := newStoreEnv(t)
	ctx := t.Context()
	v := env.vehicle(t, "Old")

	v.Name = "New"
	v.TargetMonths = ip(48)
	if err := env.store.UpdateVehicle(ctx, storeUser, v); err != nil {
		t.Fatal(err)
	}
	got, err := env.store.GetVehicle(ctx, storeUser, v.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "New" || got.TargetMonths == nil || *got.TargetMonths != 48 {
		t.Errorf("GetVehicle after update = %+v", got)
	}

	ghost := Vehicle{ID: uuid.New(), Name: "Ghost"}
	if err := env.store.UpdateVehicle(ctx, storeUser, ghost); !errors.Is(err, storage.ErrNotFound) {
		t.Errorf("UpdateVehicle unknown = %v, want ErrNotFound", err)
	}
	if env.hasKey(t, vehKey(storeUser, ghost.ID)) {
		t.Error("UpdateVehicle of an unknown vehicle created it")
	}
}

func TestStore_DeleteVehicle_CascadesCostsAndLinks(t *testing.T) {
	env := newStoreEnv(t)
	ctx := t.Context()
	vehTxn, costTxn1, costTxn2 := uuid.New(), uuid.New(), uuid.New()
	v := env.vehicle(t, "Doomed", vehTxn)
	c1 := env.cost(t, v.ID, costTxn1)
	c2 := env.cost(t, v.ID, costTxn2, costTxn1)
	c3 := env.cost(t, v.ID)
	other := env.vehicle(t, "Kept")
	otherCost := env.cost(t, other.ID, uuid.New())

	if err := env.store.DeleteVehicle(ctx, storeUser, v.ID); err != nil {
		t.Fatal(err)
	}

	if _, err := env.store.GetVehicle(ctx, storeUser, v.ID); !errors.Is(err, storage.ErrNotFound) {
		t.Errorf("vehicle still there: %v", err)
	}
	for _, c := range []CostEntry{c1, c2, c3} {
		if env.hasKey(t, costKey(storeUser, c.ID)) || env.hasKey(t, idxVehCostKey(storeUser, v.ID, c.ID)) {
			t.Errorf("cost %s or its index key survived the cascade", c.ID)
		}
	}
	if entries, _ := env.store.ListCostEntries(ctx, storeUser, v.ID); len(entries) != 0 {
		t.Errorf("ListCostEntries after delete = %d entries, want 0", len(entries))
	}

	want := map[uuid.UUID]removedRefs{
		v.ID:  {[]uuid.UUID{vehTxn}, link.RefVehicle, v.ID},
		c1.ID: {[]uuid.UUID{costTxn1}, link.RefCost, c1.ID},
		c2.ID: {[]uuid.UUID{costTxn2, costTxn1}, link.RefCost, c2.ID},
		c3.ID: {[]uuid.UUID{}, link.RefCost, c3.ID},
	}
	if len(env.side.removed) != len(want) {
		t.Fatalf("RemoveReferences calls = %+v, want %d", env.side.removed, len(want))
	}
	for id, w := range want {
		got, ok := env.side.removedFor(w.refType, id)
		if !ok || !slices.Equal(got.transactionIDs, w.transactionIDs) {
			t.Errorf("RemoveReferences for %s %s = %+v (found %v), want %v", w.refType, id, got, ok, w.transactionIDs)
		}
	}

	if _, err := env.store.GetVehicle(ctx, storeUser, other.ID); err != nil {
		t.Errorf("other vehicle: %v", err)
	}
	if entries, _ := env.store.ListCostEntries(ctx, storeUser, other.ID); len(entries) != 1 || entries[0].ID != otherCost.ID {
		t.Errorf("other vehicle's costs = %+v", entries)
	}
}

func TestStore_DeleteVehicle_DanglingIndexEntry(t *testing.T) {
	env := newStoreEnv(t)
	v := env.vehicle(t, "V")
	orphan := uuid.New()
	if err := env.update(t, func(txn *badger.Txn) error {
		if err := txn.Set(idxVehCostKey(storeUser, v.ID, orphan), nil); err != nil {
			return err
		}
		// An index key whose suffix is not a UUID is skipped.
		return txn.Set(append(idxVehCostPrefix(storeUser, v.ID), "junk"...), nil)
	}); err != nil {
		t.Fatal(err)
	}

	if err := env.store.DeleteVehicle(t.Context(), storeUser, v.ID); err != nil {
		t.Fatal(err)
	}
	if env.hasKey(t, idxVehCostKey(storeUser, v.ID, orphan)) {
		t.Error("dangling index entry survived")
	}
	if _, ok := env.side.removedFor(link.RefCost, orphan); ok {
		t.Error("RemoveReferences called for a cost entry that does not exist")
	}
}

func TestStore_DeleteVehicle_Errors(t *testing.T) {
	env := newStoreEnv(t)
	if err := env.store.DeleteVehicle(t.Context(), storeUser, uuid.New()); !errors.Is(err, storage.ErrNotFound) {
		t.Errorf("DeleteVehicle unknown = %v, want ErrNotFound", err)
	}

	v := env.vehicle(t, "V")
	c := env.cost(t, v.ID)
	env.side.failWith = errors.New("ledger down")
	if err := env.store.DeleteVehicle(t.Context(), storeUser, v.ID); err == nil {
		t.Fatal("DeleteVehicle succeeded although the reference removal failed")
	}
	env.side.failWith = nil
	if _, err := env.store.GetVehicle(t.Context(), storeUser, v.ID); err != nil {
		t.Errorf("vehicle gone after failed delete: %v", err)
	}
	if _, err := env.store.GetCostEntry(t.Context(), storeUser, c.ID); err != nil {
		t.Errorf("cost gone after failed delete: %v", err)
	}
}

func TestStore_DeleteVehicle_CostReferenceFailure(t *testing.T) {
	env := newStoreEnv(t)
	v := env.vehicle(t, "V")
	c := env.cost(t, v.ID)
	failing := &failOnRefType{fakeTxnSide: env.side, refType: link.RefCost}
	env.links.SetTransactionSide(failing)

	if err := env.store.DeleteVehicle(t.Context(), storeUser, v.ID); err == nil {
		t.Fatal("DeleteVehicle succeeded although removing the cost references failed")
	}
	if _, err := env.store.GetCostEntry(t.Context(), storeUser, c.ID); err != nil {
		t.Errorf("cost gone after failed delete: %v", err)
	}
}

type failOnRefType struct {
	*fakeTxnSide
	refType string
}

func (f *failOnRefType) RemoveReferences(txn *badger.Txn, userID string, ids []uuid.UUID, refType string, targetID uuid.UUID) error {
	if refType == f.refType {
		return errors.New("ledger down")
	}
	return f.fakeTxnSide.RemoveReferences(txn, userID, ids, refType, targetID)
}

func TestStore_CostEntries(t *testing.T) {
	env := newStoreEnv(t)
	ctx := t.Context()
	v := env.vehicle(t, "V")
	if list, err := env.store.ListCostEntries(ctx, storeUser, v.ID); err != nil || list == nil || len(list) != 0 {
		t.Fatalf("ListCostEntries empty = %v, %v; want empty non-nil list", list, err)
	}
	txnID := uuid.New()
	c := env.cost(t, v.ID, txnID, txnID)

	got, err := env.store.GetCostEntry(ctx, storeUser, c.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(got.LinkedTransactionIDs, []uuid.UUID{txnID}) {
		t.Errorf("links = %v, want normalized [%s]", got.LinkedTransactionIDs, txnID)
	}
	if _, err := env.store.GetCostEntry(ctx, storeUser, uuid.New()); !errors.Is(err, storage.ErrNotFound) {
		t.Errorf("GetCostEntry unknown = %v, want ErrNotFound", err)
	}

	// An index entry without its cost entry is skipped by the list.
	if err := env.update(t, func(txn *badger.Txn) error {
		return txn.Set(idxVehCostKey(storeUser, v.ID, uuid.New()), nil)
	}); err != nil {
		t.Fatal(err)
	}
	if list, _ := env.store.ListCostEntries(ctx, storeUser, v.ID); len(list) != 1 || list[0].ID != c.ID {
		t.Errorf("ListCostEntries = %+v, want only %s", list, c.ID)
	}
}

func TestStore_UpdateCostEntry(t *testing.T) {
	env := newStoreEnv(t)
	ctx := t.Context()
	v1 := env.vehicle(t, "V1")
	v2 := env.vehicle(t, "V2")
	stored := uuid.New()
	c := env.cost(t, v1.ID, stored)

	c.Description = "Oil change"
	c.LinkedTransactionIDs = []uuid.UUID{uuid.New()}
	if err := env.store.UpdateCostEntry(ctx, storeUser, c); err != nil {
		t.Fatal(err)
	}
	got, _ := env.store.GetCostEntry(ctx, storeUser, c.ID)
	if got.Description != "Oil change" {
		t.Errorf("Description = %q", got.Description)
	}
	if !slices.Equal(got.LinkedTransactionIDs, []uuid.UUID{stored}) {
		t.Errorf("links = %v, want the stored [%s]", got.LinkedTransactionIDs, stored)
	}

	c.VehicleID = v2.ID
	if err := env.store.UpdateCostEntry(ctx, storeUser, c); err != nil {
		t.Fatal(err)
	}
	if l1, _ := env.store.ListCostEntries(ctx, storeUser, v1.ID); len(l1) != 0 {
		t.Errorf("old vehicle still lists %d entries", len(l1))
	}
	if l2, _ := env.store.ListCostEntries(ctx, storeUser, v2.ID); len(l2) != 1 || l2[0].VehicleID != v2.ID {
		t.Errorf("new vehicle lists %+v", l2)
	}

	if err := env.store.UpdateCostEntry(ctx, storeUser, CostEntry{ID: uuid.New(), VehicleID: v1.ID}); !errors.Is(err, storage.ErrNotFound) {
		t.Errorf("UpdateCostEntry unknown = %v, want ErrNotFound", err)
	}
}

func TestStore_DeleteCostEntry(t *testing.T) {
	env := newStoreEnv(t)
	ctx := t.Context()
	v := env.vehicle(t, "V")
	txnID := uuid.New()
	c := env.cost(t, v.ID, txnID)
	kept := env.cost(t, v.ID)

	if err := env.store.DeleteCostEntry(ctx, storeUser, c.ID); err != nil {
		t.Fatal(err)
	}
	if env.hasKey(t, costKey(storeUser, c.ID)) || env.hasKey(t, idxVehCostKey(storeUser, v.ID, c.ID)) {
		t.Error("cost entry or index key survived")
	}
	got, ok := env.side.removedFor(link.RefCost, c.ID)
	if !ok || !slices.Equal(got.transactionIDs, []uuid.UUID{txnID}) {
		t.Errorf("RemoveReferences = %+v (found %v), want [%s]", got, ok, txnID)
	}
	if list, _ := env.store.ListCostEntries(ctx, storeUser, v.ID); len(list) != 1 || list[0].ID != kept.ID {
		t.Errorf("remaining entries = %+v", list)
	}

	if err := env.store.DeleteCostEntry(ctx, storeUser, c.ID); !errors.Is(err, storage.ErrNotFound) {
		t.Errorf("DeleteCostEntry twice = %v, want ErrNotFound", err)
	}
	env.side.failWith = errors.New("ledger down")
	if err := env.store.DeleteCostEntry(ctx, storeUser, kept.ID); err == nil {
		t.Error("DeleteCostEntry succeeded although the reference removal failed")
	}
	env.side.failWith = nil
	if _, err := env.store.GetCostEntry(ctx, storeUser, kept.ID); err != nil {
		t.Errorf("cost gone after failed delete: %v", err)
	}
}

func TestStore_LinkTargets(t *testing.T) {
	env := newStoreEnv(t)
	v := env.vehicle(t, "V")
	c := env.cost(t, v.ID)
	txnID := uuid.New()

	targets := []struct {
		refType string
		id      uuid.UUID
		links   func() []uuid.UUID
	}{
		{link.RefVehicle, v.ID, func() []uuid.UUID {
			got, _ := env.store.GetVehicle(t.Context(), storeUser, v.ID)
			return got.LinkedTransactionIDs
		}},
		{link.RefCost, c.ID, func() []uuid.UUID {
			got, _ := env.store.GetCostEntry(t.Context(), storeUser, c.ID)
			return got.LinkedTransactionIDs
		}},
	}
	for _, tt := range targets {
		t.Run(tt.refType, func(t *testing.T) {
			target, err := env.links.Target(tt.refType)
			if err != nil {
				t.Fatal(err)
			}
			for range 2 {
				if err := env.update(t, func(txn *badger.Txn) error { return target.AddLink(txn, storeUser, tt.id, txnID) }); err != nil {
					t.Fatal(err)
				}
			}
			if got := tt.links(); !slices.Equal(got, []uuid.UUID{txnID}) {
				t.Errorf("links after AddLink twice = %v, want [%s]", got, txnID)
			}

			if err := env.update(t, func(txn *badger.Txn) error { return target.AddLink(txn, storeUser, uuid.New(), txnID) }); !errors.Is(err, storage.ErrNotFound) {
				t.Errorf("AddLink unknown target = %v, want ErrNotFound", err)
			}

			if err := env.update(t, func(txn *badger.Txn) error { return target.RemoveLink(txn, storeUser, tt.id, txnID) }); err != nil {
				t.Fatal(err)
			}
			if got := tt.links(); len(got) != 0 {
				t.Errorf("links after RemoveLink = %v, want none", got)
			}
			if err := env.update(t, func(txn *badger.Txn) error { return target.RemoveLink(txn, storeUser, uuid.New(), txnID) }); err != nil {
				t.Errorf("RemoveLink unknown target = %v, want nil", err)
			}

			if err := env.engine.View(func(txn *badger.Txn) error {
				if !target.Exists(txn, storeUser, tt.id) {
					t.Error("Exists = false for an existing target")
				}
				if target.Exists(txn, storeUser, uuid.New()) {
					t.Error("Exists = true for an unknown target")
				}
				return nil
			}); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestStore_PruneDeadTransactionLinks(t *testing.T) {
	env := newStoreEnv(t)
	ctx := t.Context()
	alive1, alive2, dead1, dead2, dead3 := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	env.side.existing[alive1] = true
	env.side.existing[alive2] = true

	v1 := env.vehicle(t, "Mixed", alive1, dead1)
	v2 := env.vehicle(t, "Clean", alive2)
	c1 := env.cost(t, v1.ID, dead2, alive1, dead3)
	c2 := env.cost(t, v2.ID)

	pruned, err := env.store.PruneDeadTransactionLinks(ctx, storeUser)
	if err != nil {
		t.Fatal(err)
	}
	if pruned != 3 {
		t.Errorf("pruned = %d, want 3", pruned)
	}
	checks := []struct {
		name string
		got  func() []uuid.UUID
		want []uuid.UUID
	}{
		{"mixed vehicle", func() []uuid.UUID { v, _ := env.store.GetVehicle(ctx, storeUser, v1.ID); return v.LinkedTransactionIDs }, []uuid.UUID{alive1}},
		{"clean vehicle", func() []uuid.UUID { v, _ := env.store.GetVehicle(ctx, storeUser, v2.ID); return v.LinkedTransactionIDs }, []uuid.UUID{alive2}},
		{"mixed cost", func() []uuid.UUID {
			c, _ := env.store.GetCostEntry(ctx, storeUser, c1.ID)
			return c.LinkedTransactionIDs
		}, []uuid.UUID{alive1}},
		{"cost without links", func() []uuid.UUID {
			c, _ := env.store.GetCostEntry(ctx, storeUser, c2.ID)
			return c.LinkedTransactionIDs
		}, []uuid.UUID{}},
	}
	for _, c := range checks {
		if got := c.got(); !slices.Equal(got, c.want) {
			t.Errorf("%s: links = %v, want %v", c.name, got, c.want)
		}
	}

	if pruned, err := env.store.PruneDeadTransactionLinks(ctx, storeUser); err != nil || pruned != 0 {
		t.Errorf("second prune = %d, %v; want 0, nil", pruned, err)
	}
}

func TestStore_CorruptRecords(t *testing.T) {
	env := newStoreEnv(t)
	ctx := t.Context()
	env.vehicle(t, "V")
	if err := env.update(t, func(txn *badger.Txn) error {
		return txn.Set(vehKey(storeUser, uuid.New()), []byte("{"))
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := env.store.ListVehicles(ctx, storeUser); err == nil {
		t.Error("ListVehicles ignored a corrupt vehicle record")
	}
	if _, err := env.store.PruneDeadTransactionLinks(ctx, storeUser); err == nil {
		t.Error("PruneDeadTransactionLinks ignored a corrupt vehicle record")
	}

	env2 := newStoreEnv(t)
	v := env2.vehicle(t, "V")
	badCost := uuid.New()
	if err := env2.update(t, func(txn *badger.Txn) error {
		if err := txn.Set(costKey(storeUser, badCost), []byte("{")); err != nil {
			return err
		}
		return txn.Set(idxVehCostKey(storeUser, v.ID, badCost), nil)
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := env2.store.ListCostEntries(ctx, storeUser, v.ID); err == nil {
		t.Error("ListCostEntries ignored a corrupt cost record")
	}
	if _, err := env2.store.PruneDeadTransactionLinks(ctx, storeUser); err == nil {
		t.Error("PruneDeadTransactionLinks ignored a corrupt cost record")
	}
	if err := env2.store.DeleteVehicle(ctx, storeUser, v.ID); err == nil {
		t.Error("DeleteVehicle ignored a corrupt cost record")
	}
}

func TestUpdateVehicle_KeepsLinksAddedMeanwhile(t *testing.T) {
	engine, err := storage.Open(t.TempDir(), slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatalf("opening engine: %v", err)
	}
	t.Cleanup(func() { engine.Close() })
	s := NewStore(engine, link.NewRegistry())
	ctx := context.Background()
	now := time.Now().UTC()
	v := Vehicle{ID: uuid.New(), Name: "Car", CreatedAt: now, UpdatedAt: now}
	if err := s.CreateVehicle(ctx, storeTestUser, v); err != nil {
		t.Fatalf("CreateVehicle: %v", err)
	}

	stale, err := s.GetVehicle(ctx, storeTestUser, v.ID)
	if err != nil {
		t.Fatal(err)
	}
	txnID := uuid.New()
	if err := engine.Update(func(txn *badger.Txn) error {
		return s.vehicles.AddLink(txn, storeTestUser, v.ID, txnID)
	}); err != nil {
		t.Fatalf("AddLink: %v", err)
	}

	stale.Name = "Car 2"
	if err := s.UpdateVehicle(ctx, storeTestUser, stale); err != nil {
		t.Fatalf("UpdateVehicle: %v", err)
	}
	got, err := s.GetVehicle(ctx, storeTestUser, v.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "Car 2" {
		t.Errorf("Name = %q, want Car 2", got.Name)
	}
	if !slices.Equal(got.LinkedTransactionIDs, []uuid.UUID{txnID}) {
		t.Errorf("LinkedTransactionIDs = %v, want [%s]", got.LinkedTransactionIDs, txnID)
	}
}
