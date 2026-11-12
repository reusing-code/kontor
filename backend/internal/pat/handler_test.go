package pat

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/reusing-code/kontor/backend/internal/middleware"
)

func newTestHandler(t *testing.T) (http.Handler, *Store) {
	t.Helper()
	s, _, _ := newTestStore(t)
	s.now = time.Now
	h := NewHandler(s, s.logger)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/tokens", h.List)
	mux.HandleFunc("POST /api/v1/tokens", h.Create)
	mux.HandleFunc("DELETE /api/v1/tokens/{id}", h.Revoke)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user := userA
		if r.Header.Get("X-Test-User") != "" {
			user = r.Header.Get("X-Test-User")
		}
		mux.ServeHTTP(w, r.WithContext(middleware.SetUserID(r.Context(), user)))
	}), s
}

func do(h http.Handler, method, path, user, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	if user != "" {
		req.Header.Set("X-Test-User", user)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestHandlerCreateReturnsTokenOnce(t *testing.T) {
	h, _ := newTestHandler(t)
	rec := do(h, "POST", "/api/v1/tokens", "", `{"name":"agent","scopes":["read","drafts"]}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create = %d %s", rec.Code, rec.Body)
	}
	if rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("Cache-Control = %q", rec.Header().Get("Cache-Control"))
	}
	var created createResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(created.Raw, middleware.PATPrefix) || created.Name != "agent" {
		t.Fatalf("created = %+v", created)
	}

	list := do(h, "GET", "/api/v1/tokens", "", "")
	if list.Code != http.StatusOK {
		t.Fatalf("list = %d", list.Code)
	}
	body := list.Body.String()
	if strings.Contains(body, created.Raw) || strings.Contains(body, `"token"`) || strings.Contains(body, "hash") {
		t.Fatalf("list leaks secrets: %s", body)
	}
	if !strings.Contains(body, created.ID.String()) {
		t.Fatalf("list lacks the token: %s", body)
	}
}

func TestHandlerCreateValidation(t *testing.T) {
	h, _ := newTestHandler(t)
	past := time.Now().Add(-time.Hour).UTC().Format(time.RFC3339)
	bodies := map[string]string{
		"missing name":  `{"scopes":["read"]}`,
		"long name":     `{"name":"` + strings.Repeat("x", 101) + `","scopes":["read"]}`,
		"no scopes":     `{"name":"a","scopes":[]}`,
		"unknown scope": `{"name":"a","scopes":["admin"]}`,
		"write scope":   `{"name":"a","scopes":["write"]}`,
		"past expiry":   `{"name":"a","scopes":["read"],"expiresAt":"` + past + `"}`,
		"unknown field": `{"name":"a","scopes":["read"],"x":1}`,
		"not json":      `nope`,
	}
	for name, body := range bodies {
		t.Run(name, func(t *testing.T) {
			if rec := do(h, "POST", "/api/v1/tokens", "", body); rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d %s", rec.Code, rec.Body)
			}
		})
	}
}

func TestHandlerRevoke(t *testing.T) {
	h, _ := newTestHandler(t)
	rec := do(h, "POST", "/api/v1/tokens", "", `{"name":"a","scopes":["read"]}`)
	var created createResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &created)
	path := "/api/v1/tokens/" + created.ID.String()

	if rec := do(h, "DELETE", path, userB, ""); rec.Code != http.StatusNotFound {
		t.Fatalf("foreign revoke = %d", rec.Code)
	}
	if rec := do(h, "GET", "/api/v1/tokens", userB, ""); strings.Contains(rec.Body.String(), created.ID.String()) {
		t.Fatal("user B sees A's token")
	}
	if rec := do(h, "DELETE", "/api/v1/tokens/nope", "", ""); rec.Code != http.StatusBadRequest {
		t.Fatalf("bad id = %d", rec.Code)
	}
	if rec := do(h, "DELETE", path, "", ""); rec.Code != http.StatusNoContent {
		t.Fatalf("revoke = %d", rec.Code)
	}
	if rec := do(h, "DELETE", path, "", ""); rec.Code != http.StatusNotFound {
		t.Fatalf("second revoke = %d", rec.Code)
	}
	if rec := do(h, "GET", "/api/v1/tokens", "", ""); strings.Contains(rec.Body.String(), created.ID.String()) {
		t.Fatal("revoked token still listed")
	}
}

func TestHandlerLimit(t *testing.T) {
	h, _ := newTestHandler(t)
	for range MaxPerUser {
		if rec := do(h, "POST", "/api/v1/tokens", "", `{"name":"a","scopes":["read"]}`); rec.Code != http.StatusCreated {
			t.Fatalf("create = %d", rec.Code)
		}
	}
	if rec := do(h, "POST", "/api/v1/tokens", "", `{"name":"a","scopes":["read"]}`); rec.Code != http.StatusConflict {
		t.Fatalf("over cap = %d", rec.Code)
	}
}
