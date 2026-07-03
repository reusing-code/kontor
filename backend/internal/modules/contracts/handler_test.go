package contracts

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/reusing-code/kontor/backend/internal/categories"
	"github.com/reusing-code/kontor/backend/internal/middleware"
	"github.com/reusing-code/kontor/backend/internal/storage"
	"github.com/reusing-code/kontor/backend/internal/storage/link"
)

const testUserID = "00000000-0000-0000-0000-000000000001"

type testEnv struct {
	store      *Store
	categories *categories.Store
	mux        http.Handler
}

func newTestEnv(t *testing.T) testEnv {
	t.Helper()
	e, err := storage.Open(t.TempDir(), slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatalf("opening engine: %v", err)
	}
	t.Cleanup(func() { e.Close() })

	catStore := categories.NewStore(e)
	store := NewStore(e, link.NewRegistry())
	h := &Handler{store: store, categories: catStore, logger: slog.New(slog.DiscardHandler)}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/categories/{id}/contracts", h.ListContractsByCategory)
	mux.HandleFunc("POST /api/v1/categories/{id}/contracts", h.CreateContractInCategory)
	mux.HandleFunc("GET /api/v1/contracts/upcoming-renewals", h.UpcomingRenewals)
	mux.HandleFunc("GET /api/v1/contracts", h.ListContracts)
	mux.HandleFunc("GET /api/v1/contracts/{id}", h.GetContract)
	mux.HandleFunc("PUT /api/v1/contracts/{id}", h.UpdateContract)
	mux.HandleFunc("DELETE /api/v1/contracts/{id}", h.DeleteContract)
	mux.HandleFunc("GET /api/v1/summary", h.Summary)

	wrapped := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := middleware.SetUserID(r.Context(), testUserID)
		mux.ServeHTTP(w, r.WithContext(ctx))
	})
	return testEnv{store: store, categories: catStore, mux: wrapped}
}

func (env testEnv) addCategory(t *testing.T, name string) categories.Category {
	t.Helper()
	cat := categories.Category{ID: uuid.New(), Name: name, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
	if err := env.categories.Create(t.Context(), testUserID, ModuleID, cat); err != nil {
		t.Fatalf("creating category: %v", err)
	}
	return cat
}

func jsonBody(v any) *bytes.Buffer {
	b, _ := json.Marshal(v)
	return bytes.NewBuffer(b)
}

func decodeJSON[T any](t *testing.T, rec *httptest.ResponseRecorder) T {
	t.Helper()
	var v T
	if err := json.NewDecoder(rec.Body).Decode(&v); err != nil {
		t.Fatalf("decoding response: %v", err)
	}
	return v
}

func TestCreateContract_Success(t *testing.T) {
	env := newTestEnv(t)
	cat := env.addCategory(t, "Cat")

	body := map[string]any{
		"name":                    "Phone",
		"startDate":               "2025-01-01",
		"minimumDurationMonths":   12,
		"extensionDurationMonths": 12,
		"noticePeriodMonths":      3,
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/v1/categories/"+cat.ID.String()+"/contracts", jsonBody(body))
	env.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusCreated)
	}

	con := decodeJSON[Contract](t, rec)
	if con.Name != "Phone" {
		t.Errorf("Name = %q, want %q", con.Name, "Phone")
	}
	if con.CategoryID != cat.ID {
		t.Errorf("CategoryID = %s, want %s", con.CategoryID, cat.ID)
	}
}

func TestCreateContract_MissingName(t *testing.T) {
	env := newTestEnv(t)
	cat := env.addCategory(t, "Cat")

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/v1/categories/"+cat.ID.String()+"/contracts", jsonBody(map[string]any{"startDate": "2025-01-01"}))
	env.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestCreateContract_MissingStartDate(t *testing.T) {
	env := newTestEnv(t)
	cat := env.addCategory(t, "Cat")

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/v1/categories/"+cat.ID.String()+"/contracts", jsonBody(map[string]any{"name": "X"}))
	env.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestCreateContract_CategoryNotFound(t *testing.T) {
	env := newTestEnv(t)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/v1/categories/"+uuid.New().String()+"/contracts", jsonBody(map[string]any{"name": "X", "startDate": "2025-01-01"}))
	env.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestGetContract_NotFound(t *testing.T) {
	env := newTestEnv(t)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/v1/contracts/"+uuid.New().String(), nil)
	env.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestGetContract_InvalidUUID(t *testing.T) {
	env := newTestEnv(t)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/v1/contracts/not-valid", nil)
	env.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestDeleteContract_Success(t *testing.T) {
	env := newTestEnv(t)

	con := Contract{ID: uuid.New(), Name: "X"}
	if err := env.store.Create(t.Context(), testUserID, con); err != nil {
		t.Fatalf("creating contract: %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("DELETE", "/api/v1/contracts/"+con.ID.String(), nil)
	env.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNoContent)
	}
}

func TestDeleteContract_NotFound(t *testing.T) {
	env := newTestEnv(t)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("DELETE", "/api/v1/contracts/"+uuid.New().String(), nil)
	env.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestUpdateContract_NotFound(t *testing.T) {
	env := newTestEnv(t)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("PUT", "/api/v1/contracts/"+uuid.New().String(), jsonBody(map[string]any{"name": "X", "startDate": "2025-01-01"}))
	env.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}
