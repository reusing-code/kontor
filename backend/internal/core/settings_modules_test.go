package core

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/reusing-code/kontor/backend/internal/middleware"
	"github.com/reusing-code/kontor/backend/internal/module"
)

type fakeModule struct {
	module.Base
	id     string
	seeded int
}

func (f *fakeModule) ID() string                    { return f.id }
func (f *fakeModule) Prefix(userID string) []byte   { return module.Prefix(userID, f.id) }
func (f *fakeModule) RegisterRoutes(*module.Router) {}
func (f *fakeModule) Seed(context.Context, string) error {
	f.seeded++
	return nil
}
func (f *fakeModule) IsEmpty(context.Context, string) (bool, error) { return true, nil }
func (f *fakeModule) ExportSection(context.Context, string) (json.RawMessage, error) {
	return json.RawMessage(`{}`), nil
}
func (f *fakeModule) ImportSection(context.Context, string, json.RawMessage, *module.ImportResult) error {
	return nil
}

func newModulesHandler(t *testing.T) (*Handler, *fakeModule, *fakeModule, http.Handler) {
	t.Helper()
	s := newTestStore(t)
	alpha := &fakeModule{id: "alpha"}
	beta := &fakeModule{id: "beta"}
	registry := module.NewRegistry(alpha, beta)
	h := NewHandler(s, slog.New(slog.DiscardHandler), testJWTSecret, nil, nil, registry)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/settings", h.GetSettings)
	mux.HandleFunc("PUT /api/v1/settings", h.UpdateSettings)
	mux.HandleFunc("GET /api/v1/modules", h.ListModules)
	wrapped := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := middleware.SetUserID(r.Context(), testUserID)
		mux.ServeHTTP(w, r.WithContext(ctx))
	})
	return h, alpha, beta, wrapped
}

func TestSettings_EnabledModulesDefaultAllOn(t *testing.T) {
	_, _, _, mux := newModulesHandler(t)

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("GET", "/api/v1/settings", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	resp := decodeJSON[SettingsResponse](t, rec)
	if len(resp.EnabledModules) != 2 || resp.EnabledModules[0] != "alpha" || resp.EnabledModules[1] != "beta" {
		t.Fatalf("EnabledModules = %v, want [alpha beta]", resp.EnabledModules)
	}
}

func TestSettings_DisableAndReenableModule(t *testing.T) {
	_, alpha, beta, mux := newModulesHandler(t)

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("PUT", "/api/v1/settings", jsonBody(map[string]any{
		"renewalDays":    90,
		"enabledModules": []string{"alpha"},
	})))
	if rec.Code != http.StatusOK {
		t.Fatalf("disable: status = %d, want %d; body: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	resp := decodeJSON[SettingsResponse](t, rec)
	if len(resp.EnabledModules) != 1 || resp.EnabledModules[0] != "alpha" {
		t.Fatalf("EnabledModules = %v, want [alpha]", resp.EnabledModules)
	}

	// module directory reflects the disabled module
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("GET", "/api/v1/modules", nil))
	statuses := decodeJSON[[]moduleStatus](t, rec)
	if len(statuses) != 2 || statuses[0].Enabled != true || statuses[1].Enabled != false {
		t.Fatalf("statuses = %v, want alpha enabled, beta disabled", statuses)
	}

	// re-enabling runs the module's seed again
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("PUT", "/api/v1/settings", jsonBody(map[string]any{
		"renewalDays":    90,
		"enabledModules": []string{"alpha", "beta"},
	})))
	if rec.Code != http.StatusOK {
		t.Fatalf("re-enable: status = %d, want %d", rec.Code, http.StatusOK)
	}
	if beta.seeded != 1 {
		t.Fatalf("beta seeded %d times, want 1", beta.seeded)
	}
	if alpha.seeded != 0 {
		t.Fatalf("alpha seeded %d times, want 0 (was never disabled)", alpha.seeded)
	}
}

func TestSettings_UnknownModuleRejected(t *testing.T) {
	_, _, _, mux := newModulesHandler(t)

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("PUT", "/api/v1/settings", jsonBody(map[string]any{
		"renewalDays":    90,
		"enabledModules": []string{"alpha", "nope"},
	})))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestSettings_OmittedEnabledModulesLeavesStateUnchanged(t *testing.T) {
	h, _, _, mux := newModulesHandler(t)

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("PUT", "/api/v1/settings", jsonBody(map[string]any{
		"renewalDays":    90,
		"enabledModules": []string{"alpha"},
	})))
	if rec.Code != http.StatusOK {
		t.Fatalf("disable: status = %d", rec.Code)
	}

	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("PUT", "/api/v1/settings", jsonBody(map[string]any{
		"renewalDays": 30,
	})))
	if rec.Code != http.StatusOK {
		t.Fatalf("update: status = %d", rec.Code)
	}
	resp := decodeJSON[SettingsResponse](t, rec)
	if len(resp.EnabledModules) != 1 || resp.EnabledModules[0] != "alpha" {
		t.Fatalf("EnabledModules = %v, want [alpha] preserved", resp.EnabledModules)
	}

	enabled, err := h.store.ModuleEnabled(t.Context(), testUserID, "beta")
	if err != nil {
		t.Fatalf("ModuleEnabled: %v", err)
	}
	if enabled {
		t.Fatal("beta should stay disabled after unrelated settings update")
	}
}
