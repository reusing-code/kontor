package auto

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/dgraph-io/badger/v4"
	"github.com/google/uuid"
	"github.com/reusing-code/kontor/backend/internal/drafts"
)

func draftRequest(t *testing.T, parentID string, input any) drafts.CreateRequest {
	t.Helper()
	raw, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	return drafts.CreateRequest{EntityID: uuid.New(), ParentID: parentID, Input: raw}
}

func createCode(err error) string {
	var ce *drafts.CreateError
	if errors.As(err, &ce) {
		return ce.Code
	}
	return ""
}

func TestDraftCreators_Targets(t *testing.T) {
	env := newStoreEnv(t)
	creators := (&Module{store: env.store}).DraftCreators()
	if len(creators) != 2 || creators[drafts.TargetVehicle] == nil || creators[drafts.TargetCost] == nil {
		t.Errorf("DraftCreators = %v, want vehicle and cost", creators)
	}
}

func TestCreateVehicleFromDraft(t *testing.T) {
	env := newStoreEnv(t)
	mod := &Module{store: env.store}
	ctx := t.Context()

	req := draftRequest(t, "", map[string]any{"name": "Golf", "purchaseDate": "2024-01-01", "purchasePrice": 1000})
	path, err := mod.DraftCreators()[drafts.TargetVehicle](ctx, storeUser, req)
	if err != nil {
		t.Fatal(err)
	}
	if path != "/auto/vehicles/"+req.EntityID.String() {
		t.Errorf("path = %q", path)
	}
	v, err := env.store.GetVehicle(ctx, storeUser, req.EntityID)
	if err != nil || v.Name != "Golf" || *v.PurchasePrice != 1000 || v.CreatedAt.IsZero() {
		t.Fatalf("created vehicle = %+v, %v", v, err)
	}

	// A repeat finds the entity and changes nothing, even with other input.
	again := req
	again.Input = json.RawMessage(`{"name": "Other"}`)
	if p, err := mod.DraftCreators()[drafts.TargetVehicle](ctx, storeUser, again); err != nil || p != path {
		t.Errorf("repeat = %q, %v", p, err)
	}
	if v, _ := env.store.GetVehicle(ctx, storeUser, req.EntityID); v.Name != "Golf" {
		t.Errorf("repeat changed the vehicle: %+v", v)
	}

	for _, tt := range []struct {
		name  string
		input any
	}{
		{"unknown field", map[string]any{"name": "V", "color": "red"}},
		{"invalid", map[string]any{"name": ""}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			req := draftRequest(t, "", tt.input)
			_, err := mod.DraftCreators()[drafts.TargetVehicle](ctx, storeUser, req)
			if createCode(err) != drafts.CodeInvalidInput {
				t.Errorf("err = %v, want %s", err, drafts.CodeInvalidInput)
			}
			if _, err := env.store.GetVehicle(ctx, storeUser, req.EntityID); err == nil {
				t.Error("vehicle stored despite the error")
			}
		})
	}
}

func TestCreateCostFromDraft(t *testing.T) {
	env := newStoreEnv(t)
	mod := &Module{store: env.store}
	ctx := t.Context()
	vehicle := env.vehicle(t, "V")
	vehiclePath := "/auto/vehicles/" + vehicle.ID.String()

	req := draftRequest(t, vehicle.ID.String(), map[string]any{"type": "fuel", "amount": 70, "date": "2026-02-01"})
	path, err := mod.DraftCreators()[drafts.TargetCost](ctx, storeUser, req)
	if err != nil {
		t.Fatal(err)
	}
	if path != vehiclePath {
		t.Errorf("path = %q, want the vehicle's %q", path, vehiclePath)
	}
	c, err := env.store.GetCostEntry(ctx, storeUser, req.EntityID)
	if err != nil || c.VehicleID != vehicle.ID || c.Type != CostTypeFuel || *c.Amount != 70 {
		t.Fatalf("created cost = %+v, %v", c, err)
	}

	// A repeat returns the stored entry's vehicle, whatever the draft says now.
	again := req
	again.ParentID = uuid.New().String()
	if p, err := mod.DraftCreators()[drafts.TargetCost](ctx, storeUser, again); err != nil || p != vehiclePath {
		t.Errorf("repeat = %q, %v", p, err)
	}
	if list, _ := env.store.ListCostEntries(ctx, storeUser, vehicle.ID); len(list) != 1 {
		t.Errorf("repeat created another entry: %d", len(list))
	}

	valid := map[string]any{"type": "misc", "date": "2026-02-01"}
	for _, tt := range []struct {
		name, parent string
		input        any
		code         string
	}{
		{"no vehicle", "", valid, drafts.CodeParentMissing},
		{"vehicle not a uuid", "nope", valid, drafts.CodeParentNotFound},
		{"unknown vehicle", uuid.New().String(), valid, drafts.CodeParentNotFound},
		{"unknown field", vehicle.ID.String(), map[string]any{"type": "misc", "date": "2026-02-01", "x": 1}, drafts.CodeInvalidInput},
		{"invalid", vehicle.ID.String(), map[string]any{"type": "toll", "date": "2026-02-01"}, drafts.CodeInvalidInput},
	} {
		t.Run(tt.name, func(t *testing.T) {
			req := draftRequest(t, tt.parent, tt.input)
			_, err := mod.DraftCreators()[drafts.TargetCost](ctx, storeUser, req)
			if createCode(err) != tt.code {
				t.Errorf("err = %v, want %s", err, tt.code)
			}
			if _, err := env.store.GetCostEntry(ctx, storeUser, req.EntityID); err == nil {
				t.Error("cost stored despite the error")
			}
		})
	}
}

func TestDraftCreators_StoreErrors(t *testing.T) {
	env := newStoreEnv(t)
	mod := &Module{store: env.store}
	vehicle := env.vehicle(t, "V")
	badVehicle, badCost := uuid.New(), uuid.New()
	if err := env.update(t, func(txn *badger.Txn) error {
		if err := txn.Set(vehKey(storeUser, badVehicle), []byte("{")); err != nil {
			return err
		}
		return txn.Set(costKey(storeUser, badCost), []byte("{"))
	}); err != nil {
		t.Fatal(err)
	}

	req := draftRequest(t, "", map[string]any{"name": "V"})
	req.EntityID = badVehicle
	if _, err := mod.DraftCreators()[drafts.TargetVehicle](t.Context(), storeUser, req); err == nil || createCode(err) != "" {
		t.Errorf("vehicle lookup failure = %v, want a plain error", err)
	}

	req = draftRequest(t, vehicle.ID.String(), map[string]any{"type": "misc", "date": "2026-02-01"})
	req.EntityID = badCost
	if _, err := mod.DraftCreators()[drafts.TargetCost](t.Context(), storeUser, req); err == nil || createCode(err) != "" {
		t.Errorf("cost lookup failure = %v, want a plain error", err)
	}

	req = draftRequest(t, badVehicle.String(), map[string]any{"type": "misc", "date": "2026-02-01"})
	if _, err := mod.DraftCreators()[drafts.TargetCost](t.Context(), storeUser, req); err == nil || createCode(err) != "" {
		t.Errorf("parent lookup failure = %v, want a plain error", err)
	}
}
