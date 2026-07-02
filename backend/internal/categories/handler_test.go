package categories

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/reusing-code/kontor/backend/internal/middleware"
	"github.com/reusing-code/kontor/backend/internal/storage"
)

const testUserID = "00000000-0000-0000-0000-000000000001"

func newTestStore(t *testing.T) *Store {
	t.Helper()
	e, err := storage.Open(t.TempDir(), slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatalf("opening engine: %v", err)
	}
	t.Cleanup(func() { e.Close() })
	return NewStore(e)
}

func newMux(t *testing.T) (http.Handler, *Store) {
	s := newTestStore(t)
	h := NewHandler(s, slog.New(slog.DiscardHandler))
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/modules/{module}/categories", h.List)
	mux.HandleFunc("POST /api/v1/modules/{module}/categories", h.Create)
	mux.HandleFunc("GET /api/v1/modules/{module}/categories/{id}", h.Get)
	mux.HandleFunc("PUT /api/v1/modules/{module}/categories/{id}", h.Update)
	mux.HandleFunc("DELETE /api/v1/modules/{module}/categories/{id}", h.Delete)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := middleware.SetUserID(r.Context(), testUserID)
		mux.ServeHTTP(w, r.WithContext(ctx))
	}), s
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

func TestCreateCategory_Success(t *testing.T) {
	mux, _ := newMux(t)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/v1/modules/contracts/categories", jsonBody(map[string]string{"name": "Test"}))
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusCreated)
	}

	cat := decodeJSON[Category](t, rec)
	if cat.Name != "Test" {
		t.Errorf("Name = %q, want %q", cat.Name, "Test")
	}
	if cat.ID == uuid.Nil {
		t.Error("ID should not be nil")
	}
}

func TestCreateCategory_EmptyName(t *testing.T) {
	mux, _ := newMux(t)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/v1/modules/contracts/categories", jsonBody(map[string]string{"name": ""}))
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestCreateCategory_InvalidJSON(t *testing.T) {
	mux, _ := newMux(t)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/v1/modules/contracts/categories", bytes.NewBufferString("{bad"))
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestCreateCategory_UnknownField(t *testing.T) {
	mux, _ := newMux(t)

	rec := httptest.NewRecorder()
	body := `{"name":"Test","bogus":"field"}`
	req := httptest.NewRequest("POST", "/api/v1/modules/contracts/categories", bytes.NewBufferString(body))
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d (unknown fields should be rejected)", rec.Code, http.StatusBadRequest)
	}
}

func TestGetCategory_NotFound(t *testing.T) {
	mux, _ := newMux(t)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/v1/modules/contracts/categories/"+uuid.New().String(), nil)
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestGetCategory_InvalidUUID(t *testing.T) {
	mux, _ := newMux(t)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/v1/modules/contracts/categories/not-a-uuid", nil)
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestListCategories_Empty(t *testing.T) {
	mux, _ := newMux(t)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/v1/modules/contracts/categories", nil)
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	cats := decodeJSON[[]Category](t, rec)
	if len(cats) != 0 {
		t.Errorf("expected empty list, got %d", len(cats))
	}
}

func TestUpdateCategory_NotFound(t *testing.T) {
	mux, _ := newMux(t)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("PUT", "/api/v1/modules/contracts/categories/"+uuid.New().String(), jsonBody(map[string]string{"name": "X"}))
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestDeleteCategory_Success(t *testing.T) {
	mux, s := newMux(t)

	cat := Category{ID: uuid.New(), Name: "X"}
	if err := s.Create(t.Context(), testUserID, "contracts", cat); err != nil {
		t.Fatalf("creating category: %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("DELETE", "/api/v1/modules/contracts/categories/"+cat.ID.String(), nil)
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNoContent)
	}
}

func TestDeleteCategory_NotFound(t *testing.T) {
	mux, _ := newMux(t)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("DELETE", "/api/v1/modules/contracts/categories/"+uuid.New().String(), nil)
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestResponses_HaveJSONContentType(t *testing.T) {
	mux, _ := newMux(t)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/v1/modules/contracts/categories", nil)
	mux.ServeHTTP(rec, req)

	ct := rec.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("Content-Type = %q, want %q", ct, "application/json")
	}
}

func TestSeedDefaults_OnlyWhenEmpty(t *testing.T) {
	s := newTestStore(t)
	defaults := []Default{{Name: "A", NameKey: "k.a"}, {Name: "B", NameKey: "k.b"}}

	if err := s.SeedDefaults(t.Context(), testUserID, "contracts", defaults); err != nil {
		t.Fatalf("seeding: %v", err)
	}
	cats, err := s.List(t.Context(), testUserID, "contracts")
	if err != nil {
		t.Fatalf("listing: %v", err)
	}
	if len(cats) != 2 {
		t.Fatalf("got %d categories, want 2", len(cats))
	}

	if err := s.SeedDefaults(t.Context(), testUserID, "contracts", defaults); err != nil {
		t.Fatalf("re-seeding: %v", err)
	}
	cats, _ = s.List(t.Context(), testUserID, "contracts")
	if len(cats) != 2 {
		t.Fatalf("re-seed duplicated defaults: got %d, want 2", len(cats))
	}
}
