package auto

import (
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/dgraph-io/badger/v4"
	"github.com/google/uuid"
	"github.com/reusing-code/kontor/backend/internal/module"
)

func TestModule_Basics(t *testing.T) {
	env := newHandlerEnv(t)
	if env.mod.ID() != ModuleID {
		t.Errorf("ID = %q, want %q", env.mod.ID(), ModuleID)
	}
	if got, want := string(env.mod.Prefix(storeUser)), "u/"+storeUser+"/mod/auto/"; got != want {
		t.Errorf("Prefix = %q, want %q", got, want)
	}
	// Every key the store writes lives under the module prefix.
	id := uuid.New()
	for _, key := range [][]byte{vehKey(storeUser, id), costKey(storeUser, id), idxVehCostKey(storeUser, id, id)} {
		if !strings.HasPrefix(string(key), string(env.mod.Prefix(storeUser))) {
			t.Errorf("key %q outside the module prefix", key)
		}
	}
}

func TestModule_ExportImportRoundTrip(t *testing.T) {
	src := newStoreEnv(t)
	mod := &Module{store: src.store}
	ctx := t.Context()

	if empty, err := mod.IsEmpty(ctx, storeUser); err != nil || !empty {
		t.Fatalf("IsEmpty on a new store = %v, %v", empty, err)
	}
	raw, err := mod.ExportSection(ctx, storeUser)
	if err != nil {
		t.Fatal(err)
	}
	var empty exportSection
	if err := json.Unmarshal(raw, &empty); err != nil || empty.CostEntries == nil || len(empty.Vehicles) != 0 {
		t.Fatalf("empty export = %s (%v); want costEntries as [] and no vehicles", raw, err)
	}

	txnID := uuid.New()
	v1 := src.vehicle(t, "A", txnID)
	v2 := src.vehicle(t, "B")
	c1 := src.cost(t, v1.ID, txnID)
	c2 := src.cost(t, v1.ID)
	c3 := src.cost(t, v2.ID)

	if empty, err := mod.IsEmpty(ctx, storeUser); err != nil || empty {
		t.Fatalf("IsEmpty with vehicles = %v, %v", empty, err)
	}
	raw, err = mod.ExportSection(ctx, storeUser)
	if err != nil {
		t.Fatal(err)
	}
	var section exportSection
	if err := json.Unmarshal(raw, &section); err != nil {
		t.Fatal(err)
	}
	if section.SchemaVersion != sectionSchemaVersion || len(section.Vehicles) != 2 || len(section.CostEntries) != 3 {
		t.Fatalf("export = %+v", section)
	}

	dst := newStoreEnv(t)
	dst.side.existing[txnID] = true
	dstMod := &Module{store: dst.store}
	res := module.NewImportResult()
	if err := dstMod.ImportSection(ctx, userB, raw, res); err != nil {
		t.Fatal(err)
	}
	if res.Restored["vehicles"] != 2 || res.Restored["costEntries"] != 3 {
		t.Errorf("restored = %v", res.Restored)
	}
	got, err := dst.store.GetVehicle(ctx, userB, v1.ID)
	if err != nil || got.Name != "A" || !slices.Equal(got.LinkedTransactionIDs, []uuid.UUID{txnID}) {
		t.Errorf("imported vehicle = %+v, %v", got, err)
	}
	for vehicleID, want := range map[uuid.UUID][]uuid.UUID{v1.ID: {c1.ID, c2.ID}, v2.ID: {c3.ID}} {
		entries, err := dst.store.ListCostEntries(ctx, userB, vehicleID)
		if err != nil {
			t.Fatal(err)
		}
		var ids []uuid.UUID
		for _, e := range entries {
			ids = append(ids, e.ID)
		}
		slices.SortFunc(ids, func(a, b uuid.UUID) int { return strings.Compare(a.String(), b.String()) })
		slices.SortFunc(want, func(a, b uuid.UUID) int { return strings.Compare(a.String(), b.String()) })
		if !slices.Equal(ids, want) {
			t.Errorf("imported costs of %s = %v, want %v", vehicleID, ids, want)
		}
	}

	res = module.NewImportResult()
	if err := dstMod.PruneDeadLinks(ctx, userB, res); err != nil || len(res.Warnings) != 0 {
		t.Errorf("prune with live links = %v, warnings %v", err, res.Warnings)
	}
	delete(dst.side.existing, txnID)
	if err := dstMod.PruneDeadLinks(ctx, userB, res); err != nil {
		t.Fatal(err)
	}
	if len(res.Warnings) != 1 || res.Warnings[0] != "auto: removed 2 link(s) to missing ledger transactions" {
		t.Errorf("warnings = %v", res.Warnings)
	}
}

func TestModule_ImportSectionErrors(t *testing.T) {
	env := newStoreEnv(t)
	mod := &Module{store: env.store}
	for _, tt := range []struct{ name, data string }{
		{"not json", "{"},
		{"wrong schema version", `{"schemaVersion": 1, "vehicles": [], "costEntries": []}`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			err := mod.ImportSection(t.Context(), storeUser, json.RawMessage(tt.data), module.NewImportResult())
			if !errors.Is(err, module.ErrInvalidSection) {
				t.Errorf("ImportSection = %v, want ErrInvalidSection", err)
			}
		})
	}
}

func TestModule_ExportErrors(t *testing.T) {
	env := newStoreEnv(t)
	mod := &Module{store: env.store}
	v := env.vehicle(t, "V")
	badCost := uuid.New()
	if err := env.update(t, func(txn *badger.Txn) error {
		if err := txn.Set(costKey(storeUser, badCost), []byte("{")); err != nil {
			return err
		}
		return txn.Set(idxVehCostKey(storeUser, v.ID, badCost), nil)
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := mod.ExportSection(t.Context(), storeUser); err == nil {
		t.Error("ExportSection ignored a corrupt cost record")
	}
	if err := mod.PruneDeadLinks(t.Context(), storeUser, module.NewImportResult()); err == nil {
		t.Error("PruneDeadLinks ignored a corrupt cost record")
	}

	if err := env.update(t, func(txn *badger.Txn) error { return txn.Set(vehKey(storeUser, uuid.New()), []byte("{")) }); err != nil {
		t.Fatal(err)
	}
	if _, err := mod.ExportSection(t.Context(), storeUser); err == nil {
		t.Error("ExportSection ignored a corrupt vehicle record")
	}
	if _, err := mod.IsEmpty(t.Context(), storeUser); err == nil {
		t.Error("IsEmpty ignored a corrupt vehicle record")
	}
}
