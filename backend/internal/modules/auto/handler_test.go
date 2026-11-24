package auto

import (
	"bytes"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/dgraph-io/badger/v4"
	"github.com/google/uuid"
	"github.com/reusing-code/kontor/backend/internal/middleware"
	"github.com/reusing-code/kontor/backend/internal/module"
	"github.com/reusing-code/kontor/backend/internal/storage"
	"github.com/reusing-code/kontor/backend/internal/storage/link"
)

type handlerEnv struct {
	mod    *Module
	links  *link.Registry
	engine *storage.Engine
	side   *fakeTxnSide
	mux    http.Handler
}

func newHandlerEnv(t *testing.T) handlerEnv {
	t.Helper()
	engine, err := storage.Open(t.TempDir(), slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { engine.Close() })
	links := link.NewRegistry()
	side := &fakeTxnSide{existing: map[uuid.UUID]bool{}}
	links.SetTransactionSide(side)
	mod := New(engine, links, slog.New(slog.DiscardHandler))

	mux := http.NewServeMux()
	mod.RegisterRoutes(module.NewRouter(mux, nil))
	wrapped := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mux.ServeHTTP(w, r.WithContext(middleware.SetUserID(r.Context(), storeUser)))
	})
	return handlerEnv{mod: mod, links: links, engine: engine, side: side, mux: wrapped}
}

func (e handlerEnv) do(t *testing.T, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var reader *bytes.Reader
	switch b := body.(type) {
	case nil:
		reader = bytes.NewReader(nil)
	case string:
		reader = bytes.NewReader([]byte(b))
	default:
		data, err := json.Marshal(b)
		if err != nil {
			t.Fatal(err)
		}
		reader = bytes.NewReader(data)
	}
	rec := httptest.NewRecorder()
	e.mux.ServeHTTP(rec, httptest.NewRequest(method, path, reader))
	return rec
}

func decode[T any](t *testing.T, rec *httptest.ResponseRecorder) T {
	t.Helper()
	var v T
	if err := json.NewDecoder(rec.Body).Decode(&v); err != nil {
		t.Fatalf("decoding %q: %v", rec.Body.String(), err)
	}
	return v
}

func errorText(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	return decode[map[string]string](t, rec)["error"]
}

func (e handlerEnv) addLink(t *testing.T, refType string, id, txnID uuid.UUID) {
	t.Helper()
	target, err := e.links.Target(refType)
	if err != nil {
		t.Fatal(err)
	}
	if err := e.engine.Update(func(txn *badger.Txn) error { return target.AddLink(txn, storeUser, id, txnID) }); err != nil {
		t.Fatal(err)
	}
}

func TestHandler_VehicleLifecycle(t *testing.T) {
	env := newHandlerEnv(t)

	rec := env.do(t, "GET", "/api/v1/vehicles", nil)
	if rec.Code != http.StatusOK || strings.TrimSpace(rec.Body.String()) != "[]" {
		t.Fatalf("empty list = %d %q, want 200 []", rec.Code, rec.Body.String())
	}

	rec = env.do(t, "POST", "/api/v1/vehicles", map[string]any{
		"name": "Golf", "make": "VW", "model": "Golf 8", "year": 2021, "licensePlate": "B-AB 123",
		"purchaseDate": "2021-03-01", "purchasePrice": 25000, "purchaseMileage": 10,
		"targetMileage": 150000, "targetMonths": 96, "annualInsurance": 600, "annualTax": 120,
		"maintenanceFactor": 1.2, "comments": "first car",
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("create = %d %s", rec.Code, rec.Body.String())
	}
	created := decode[Vehicle](t, rec)
	if created.ID == uuid.Nil || created.Name != "Golf" || created.Make != "VW" || created.Model != "Golf 8" ||
		*created.Year != 2021 || created.LicensePlate != "B-AB 123" || created.PurchaseDate != "2021-03-01" ||
		*created.PurchasePrice != 25000 || *created.PurchaseMileage != 10 || *created.TargetMileage != 150000 ||
		*created.TargetMonths != 96 || *created.AnnualInsurance != 600 || *created.AnnualTax != 120 ||
		*created.MaintenanceFactor != 1.2 || created.Comments != "first car" ||
		created.CreatedAt.IsZero() || !created.CreatedAt.Equal(created.UpdatedAt) {
		t.Errorf("created vehicle = %+v", created)
	}
	path := "/api/v1/vehicles/" + created.ID.String()

	rec = env.do(t, "GET", path, nil)
	if rec.Code != http.StatusOK || decode[Vehicle](t, rec).Name != "Golf" {
		t.Fatalf("get = %d", rec.Code)
	}
	rec = env.do(t, "GET", "/api/v1/vehicles", nil)
	if list := decode[[]Vehicle](t, rec); len(list) != 1 || list[0].ID != created.ID {
		t.Errorf("list = %+v", list)
	}

	txnID := uuid.New()
	env.addLink(t, link.RefVehicle, created.ID, txnID)

	rec = env.do(t, "PUT", path, map[string]any{"name": "Golf GTI", "purchaseDate": "2021-04-01"})
	if rec.Code != http.StatusOK {
		t.Fatalf("update = %d %s", rec.Code, rec.Body.String())
	}
	updated := decode[Vehicle](t, rec)
	if updated.Name != "Golf GTI" || updated.Make != "" || updated.Year != nil || updated.PurchasePrice != nil ||
		updated.PurchaseDate != "2021-04-01" || updated.Comments != "" {
		t.Errorf("update must replace all input fields: %+v", updated)
	}
	if !updated.CreatedAt.Equal(created.CreatedAt) || updated.UpdatedAt.Before(created.UpdatedAt) {
		t.Errorf("timestamps after update: created %v updated %v", updated.CreatedAt, updated.UpdatedAt)
	}
	stored, err := env.mod.store.GetVehicle(t.Context(), storeUser, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Name != "Golf GTI" || !slices.Equal(stored.LinkedTransactionIDs, []uuid.UUID{txnID}) {
		t.Errorf("stored after update = %+v, want links [%s] kept", stored, txnID)
	}

	rec = env.do(t, "DELETE", path, nil)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("delete = %d", rec.Code)
	}
	if r, ok := env.side.removedFor(link.RefVehicle, created.ID); !ok || !slices.Equal(r.transactionIDs, []uuid.UUID{txnID}) {
		t.Errorf("RemoveReferences = %+v (found %v)", r, ok)
	}
	if rec = env.do(t, "GET", path, nil); rec.Code != http.StatusNotFound {
		t.Errorf("get after delete = %d, want 404", rec.Code)
	}
}

func TestHandler_VehicleErrors(t *testing.T) {
	env := newHandlerEnv(t)
	unknown := "/api/v1/vehicles/" + uuid.New().String()
	existing := env.do(t, "POST", "/api/v1/vehicles", map[string]any{"name": "V"})
	existingPath := "/api/v1/vehicles/" + decode[Vehicle](t, existing).ID.String()

	tests := []struct {
		name, method, path string
		body               any
		status             int
		errText            string
	}{
		{"get invalid id", "GET", "/api/v1/vehicles/nope", nil, http.StatusBadRequest, "invalid id"},
		{"get unknown", "GET", unknown, nil, http.StatusNotFound, "not found"},
		{"create bad json", "POST", "/api/v1/vehicles", "{", http.StatusBadRequest, "invalid request body"},
		{"create unknown field", "POST", "/api/v1/vehicles", map[string]any{"name": "V", "color": "red"}, http.StatusBadRequest, "invalid request body"},
		{"create without name", "POST", "/api/v1/vehicles", map[string]any{"make": "VW"}, http.StatusBadRequest, "name is required"},
		{"create bad date", "POST", "/api/v1/vehicles", map[string]any{"name": "V", "purchaseDate": "01.02.2024"}, http.StatusBadRequest, "purchaseDate must be a date in YYYY-MM-DD format"},
		{"update invalid id", "PUT", "/api/v1/vehicles/nope", map[string]any{"name": "V"}, http.StatusBadRequest, "invalid id"},
		{"update unknown", "PUT", unknown, map[string]any{"name": "V"}, http.StatusNotFound, "not found"},
		{"update bad json", "PUT", existingPath, "{", http.StatusBadRequest, "invalid request body"},
		{"update without name", "PUT", existingPath, map[string]any{"name": ""}, http.StatusBadRequest, "name is required"},
		{"delete invalid id", "DELETE", "/api/v1/vehicles/nope", nil, http.StatusBadRequest, "invalid id"},
		{"delete unknown", "DELETE", unknown, nil, http.StatusNotFound, "not found"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := env.do(t, tt.method, tt.path, tt.body)
			if rec.Code != tt.status {
				t.Fatalf("status = %d, want %d (%s)", rec.Code, tt.status, rec.Body.String())
			}
			if got := errorText(t, rec); got != tt.errText {
				t.Errorf("error = %q, want %q", got, tt.errText)
			}
		})
	}
}

func TestHandler_CostLifecycle(t *testing.T) {
	env := newHandlerEnv(t)
	vehicle := decode[Vehicle](t, env.do(t, "POST", "/api/v1/vehicles", map[string]any{"name": "V"}))
	other := decode[Vehicle](t, env.do(t, "POST", "/api/v1/vehicles", map[string]any{"name": "W"}))
	costsPath := "/api/v1/vehicles/" + vehicle.ID.String() + "/costs"

	rec := env.do(t, "GET", costsPath, nil)
	if rec.Code != http.StatusOK || strings.TrimSpace(rec.Body.String()) != "[]" {
		t.Fatalf("empty cost list = %d %q", rec.Code, rec.Body.String())
	}

	rec = env.do(t, "POST", costsPath, map[string]any{
		"type": "service", "description": "Inspection", "vendor": "Garage", "amount": 250.5,
		"date": "2026-09-15", "mileage": 42000, "comments": "with oil",
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("create cost = %d %s", rec.Code, rec.Body.String())
	}
	created := decode[CostEntry](t, rec)
	if created.ID == uuid.Nil || created.VehicleID != vehicle.ID || created.Type != CostTypeService ||
		created.Description != "Inspection" || created.Vendor != "Garage" || *created.Amount != 250.5 ||
		created.Date != "2026-09-15" || *created.Mileage != 42000 || created.Comments != "with oil" ||
		created.CreatedAt.IsZero() || !created.CreatedAt.Equal(created.UpdatedAt) {
		t.Errorf("created cost = %+v", created)
	}
	path := "/api/v1/costs/" + created.ID.String()

	if rec = env.do(t, "GET", path, nil); rec.Code != http.StatusOK || decode[CostEntry](t, rec).ID != created.ID {
		t.Fatalf("get cost = %d", rec.Code)
	}
	if list := decode[[]CostEntry](t, env.do(t, "GET", costsPath, nil)); len(list) != 1 {
		t.Errorf("cost list = %+v", list)
	}

	txnID := uuid.New()
	env.addLink(t, link.RefCost, created.ID, txnID)

	rec = env.do(t, "PUT", path, map[string]any{"type": "fuel", "amount": 60, "date": "2026-09-20"})
	if rec.Code != http.StatusOK {
		t.Fatalf("update cost = %d %s", rec.Code, rec.Body.String())
	}
	updated := decode[CostEntry](t, rec)
	if updated.Type != CostTypeFuel || *updated.Amount != 60 || updated.Date != "2026-09-20" ||
		updated.Description != "" || updated.Vendor != "" || updated.Mileage != nil || updated.Comments != "" ||
		updated.VehicleID != vehicle.ID || updated.UpdatedAt.Before(created.UpdatedAt) {
		t.Errorf("updated cost = %+v", updated)
	}
	stored, _ := env.mod.store.GetCostEntry(t.Context(), storeUser, created.ID)
	if !slices.Equal(stored.LinkedTransactionIDs, []uuid.UUID{txnID}) {
		t.Errorf("links after update = %v, want [%s]", stored.LinkedTransactionIDs, txnID)
	}

	// The other vehicle's list is unaffected.
	if list := decode[[]CostEntry](t, env.do(t, "GET", "/api/v1/vehicles/"+other.ID.String()+"/costs", nil)); len(list) != 0 {
		t.Errorf("other vehicle's costs = %+v", list)
	}

	if rec = env.do(t, "DELETE", path, nil); rec.Code != http.StatusNoContent {
		t.Fatalf("delete cost = %d", rec.Code)
	}
	if r, ok := env.side.removedFor(link.RefCost, created.ID); !ok || !slices.Equal(r.transactionIDs, []uuid.UUID{txnID}) {
		t.Errorf("RemoveReferences = %+v (found %v)", r, ok)
	}
	if rec = env.do(t, "GET", path, nil); rec.Code != http.StatusNotFound {
		t.Errorf("get after delete = %d, want 404", rec.Code)
	}
}

func TestHandler_CostErrors(t *testing.T) {
	env := newHandlerEnv(t)
	vehicle := decode[Vehicle](t, env.do(t, "POST", "/api/v1/vehicles", map[string]any{"name": "V"}))
	costsPath := "/api/v1/vehicles/" + vehicle.ID.String() + "/costs"
	cost := decode[CostEntry](t, env.do(t, "POST", costsPath, map[string]any{"type": "misc", "date": "2026-01-01"}))
	costPath := "/api/v1/costs/" + cost.ID.String()
	unknownVehicle := "/api/v1/vehicles/" + uuid.New().String() + "/costs"
	unknownCost := "/api/v1/costs/" + uuid.New().String()

	tests := []struct {
		name, method, path string
		body               any
		status             int
		errText            string
	}{
		{"list invalid vehicle id", "GET", "/api/v1/vehicles/nope/costs", nil, http.StatusBadRequest, "invalid vehicle id"},
		{"create invalid vehicle id", "POST", "/api/v1/vehicles/nope/costs", map[string]any{"type": "misc", "date": "2026-01-01"}, http.StatusBadRequest, "invalid vehicle id"},
		{"create unknown vehicle", "POST", unknownVehicle, map[string]any{"type": "misc", "date": "2026-01-01"}, http.StatusNotFound, "not found"},
		{"create bad json", "POST", costsPath, "{", http.StatusBadRequest, "invalid request body"},
		{"create without type", "POST", costsPath, map[string]any{"date": "2026-01-01"}, http.StatusBadRequest, "type is required"},
		{"create bad type", "POST", costsPath, map[string]any{"type": "toll", "date": "2026-01-01"}, http.StatusBadRequest, "invalid cost type"},
		{"create without date", "POST", costsPath, map[string]any{"type": "misc"}, http.StatusBadRequest, "date is required"},
		{"create mileage without value", "POST", costsPath, map[string]any{"type": "mileage", "date": "2026-01-01"}, http.StatusBadRequest, "mileage is required for mileage entries"},
		{"get invalid id", "GET", "/api/v1/costs/nope", nil, http.StatusBadRequest, "invalid id"},
		{"get unknown", "GET", unknownCost, nil, http.StatusNotFound, "not found"},
		{"update invalid id", "PUT", "/api/v1/costs/nope", map[string]any{"type": "misc", "date": "2026-01-01"}, http.StatusBadRequest, "invalid id"},
		{"update unknown", "PUT", unknownCost, map[string]any{"type": "misc", "date": "2026-01-01"}, http.StatusNotFound, "not found"},
		{"update bad json", "PUT", costPath, "{", http.StatusBadRequest, "invalid request body"},
		{"update invalid", "PUT", costPath, map[string]any{"type": "misc"}, http.StatusBadRequest, "date is required"},
		{"delete invalid id", "DELETE", "/api/v1/costs/nope", nil, http.StatusBadRequest, "invalid id"},
		{"delete unknown", "DELETE", unknownCost, nil, http.StatusNotFound, "not found"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := env.do(t, tt.method, tt.path, tt.body)
			if rec.Code != tt.status {
				t.Fatalf("status = %d, want %d (%s)", rec.Code, tt.status, rec.Body.String())
			}
			if got := errorText(t, rec); got != tt.errText {
				t.Errorf("error = %q, want %q", got, tt.errText)
			}
		})
	}
}

func TestHandler_VehicleSummary(t *testing.T) {
	env := newHandlerEnv(t)
	vehicle := decode[Vehicle](t, env.do(t, "POST", "/api/v1/vehicles", map[string]any{
		"name": "V", "purchaseDate": "2024-01-01", "purchasePrice": 10000, "targetMonths": 60,
	}))
	costsPath := "/api/v1/vehicles/" + vehicle.ID.String() + "/costs"
	for _, body := range []map[string]any{
		{"type": "service", "amount": 300, "date": "2024-06-01"},
		{"type": "fuel", "amount": 50, "date": "2024-07-01", "mileage": 1000},
		{"type": "mileage", "date": "2024-08-01", "mileage": 2000},
	} {
		if rec := env.do(t, "POST", costsPath, body); rec.Code != http.StatusCreated {
			t.Fatalf("create cost = %d %s", rec.Code, rec.Body.String())
		}
	}

	rec := env.do(t, "GET", "/api/v1/vehicles/"+vehicle.ID.String()+"/summary", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("summary = %d %s", rec.Code, rec.Body.String())
	}
	s := decode[VehicleSummary](t, rec)
	if s.Vehicle.ID != vehicle.ID || s.EntryCount != 3 || s.TotalCost != 10350 || s.CurrentMileage != 2000 {
		t.Errorf("summary = %+v", s)
	}
	if s.CostsByType[CostTypeService] != 300 || s.CostsByType[CostTypeFuel] != 50 || len(s.CostsByType) != 2 {
		t.Errorf("CostsByType = %v", s.CostsByType)
	}
	if s.Projection == nil || s.Projection.TargetMonths == nil || *s.Projection.TargetMonths != 60 {
		t.Errorf("Projection = %+v", s.Projection)
	}
	if len(s.CostsByYear) == 0 || s.CostsByYear[0].Year != 2024 || s.CostsByYear[0].Total != 350 {
		t.Errorf("CostsByYear = %+v", s.CostsByYear)
	}

	for _, tt := range []struct {
		path   string
		status int
	}{
		{"/api/v1/vehicles/nope/summary", http.StatusBadRequest},
		{"/api/v1/vehicles/" + uuid.New().String() + "/summary", http.StatusNotFound},
	} {
		if rec := env.do(t, "GET", tt.path, nil); rec.Code != tt.status {
			t.Errorf("GET %s = %d, want %d", tt.path, rec.Code, tt.status)
		}
	}
}

func TestHandler_StoreFailures(t *testing.T) {
	env := newHandlerEnv(t)
	vehicle := decode[Vehicle](t, env.do(t, "POST", "/api/v1/vehicles", map[string]any{"name": "V"}))
	cost := decode[CostEntry](t, env.do(t, "POST", "/api/v1/vehicles/"+vehicle.ID.String()+"/costs", map[string]any{"type": "misc", "date": "2026-01-01"}))
	badCost := uuid.New()
	if err := env.engine.Update(func(txn *badger.Txn) error {
		if err := txn.Set(costKey(storeUser, badCost), []byte("{")); err != nil {
			return err
		}
		if err := txn.Set(idxVehCostKey(storeUser, vehicle.ID, badCost), nil); err != nil {
			return err
		}
		return txn.Set(vehKey(storeUser, uuid.New()), []byte("{"))
	}); err != nil {
		t.Fatal(err)
	}
	env.side.failWith = errors.New("ledger down")

	for _, tt := range []struct{ method, path string }{
		{"GET", "/api/v1/vehicles"},
		{"GET", "/api/v1/vehicles/" + vehicle.ID.String() + "/costs"},
		{"GET", "/api/v1/vehicles/" + vehicle.ID.String() + "/summary"},
		{"DELETE", "/api/v1/vehicles/" + vehicle.ID.String()},
		{"DELETE", "/api/v1/costs/" + cost.ID.String()},
	} {
		if rec := env.do(t, tt.method, tt.path, nil); rec.Code != http.StatusInternalServerError || errorText(t, rec) != "internal error" {
			t.Errorf("%s %s = %d %s, want 500 internal error", tt.method, tt.path, rec.Code, rec.Body.String())
		}
	}
}
