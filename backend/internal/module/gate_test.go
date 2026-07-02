package module

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/reusing-code/kontor/backend/internal/middleware"
)

type fakeSource struct {
	disabled map[string]bool
}

func (f fakeSource) ModuleEnabled(_ context.Context, _ string, moduleID string) (bool, error) {
	return !f.disabled[moduleID], nil
}

func serveGated(t *testing.T, handler http.Handler, path string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", path, nil)
	req = req.WithContext(middleware.SetUserID(req.Context(), "user-1"))
	handler.ServeHTTP(rec, req)
	return rec
}

func TestGate_AllowsEnabledModule(t *testing.T) {
	gate := Gate("contracts", fakeSource{})
	handler := gate(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	rec := serveGated(t, handler, "/api/v1/contracts")
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNoContent)
	}
}

func TestGate_RejectsDisabledModule(t *testing.T) {
	gate := Gate("ledger", fakeSource{disabled: map[string]bool{"ledger": true}})
	handler := gate(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Error("handler must not run for a disabled module")
	}))

	rec := serveGated(t, handler, "/api/v1/ledger/accounts")
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusForbidden)
	}
	var body map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decoding body: %v", err)
	}
	if body["code"] != "module_disabled" {
		t.Fatalf("code = %q, want %q", body["code"], "module_disabled")
	}
}

func TestGateParam_UnknownModuleIs404(t *testing.T) {
	gate := GateParam(fakeSource{}, "contracts", "purchases")
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/modules/{module}/categories", gate(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	rec := serveGated(t, mux, "/api/v1/modules/bogus/categories")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestGateParam_DisabledModuleIs403(t *testing.T) {
	gate := GateParam(fakeSource{disabled: map[string]bool{"purchases": true}}, "contracts", "purchases")
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/modules/{module}/categories", gate(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	if rec := serveGated(t, mux, "/api/v1/modules/purchases/categories"); rec.Code != http.StatusForbidden {
		t.Fatalf("disabled: status = %d, want %d", rec.Code, http.StatusForbidden)
	}
	if rec := serveGated(t, mux, "/api/v1/modules/contracts/categories"); rec.Code != http.StatusNoContent {
		t.Fatalf("enabled: status = %d, want %d", rec.Code, http.StatusNoContent)
	}
}
