package middleware

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func scopeFixture() (*http.ServeMux, map[string]Rule) {
	mux := http.NewServeMux()
	ok := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })
	table := map[string]Rule{}
	add := func(pattern string, rule Rule) {
		mux.Handle(pattern, ok)
		table[pattern] = rule
	}
	add("GET /r", RuleRead)
	add("PUT /d/{id}", RuleDrafts)
	add("POST /l", RuleLinks)
	add("POST /x", RuleDenied)
	add("GET /j", RuleJWTOnly)
	mux.Handle("GET /unlisted", ok)
	return mux, table
}

func serve(mux *http.ServeMux, table map[string]Rule, method, path string, p *Principal) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, nil)
	if p != nil {
		req = req.WithContext(WithPrincipal(context.Background(), *p))
	}
	rec := httptest.NewRecorder()
	ScopeGuard(mux, table).ServeHTTP(rec, req)
	return rec
}

func pat(scopes ...string) *Principal {
	return &Principal{UserID: "u", Kind: AuthPAT, TokenID: "t", Scopes: scopes}
}

func TestScopeGuardMatrix(t *testing.T) {
	mux, table := scopeFixture()
	all := pat("read", "drafts", "links")
	tests := []struct {
		name   string
		method string
		path   string
		p      *Principal
		want   int
	}{
		{"read with read", "GET", "/r", pat("read"), 204},
		{"read without read", "GET", "/r", pat("drafts", "links"), 403},
		{"drafts with drafts", "PUT", "/d/1", pat("drafts"), 204},
		{"drafts with read only", "PUT", "/d/1", pat("read"), 403},
		{"links with links", "POST", "/l", pat("links"), 204},
		{"links with drafts", "POST", "/l", pat("drafts"), 403},
		{"denied with all scopes", "POST", "/x", all, 403},
		{"jwt-only with all scopes", "GET", "/j", all, 403},
		{"unlisted pattern denied", "GET", "/unlisted", all, 403},
		{"jwt passes denied", "POST", "/x", &Principal{UserID: "u", Kind: AuthJWT}, 204},
		{"jwt passes jwt-only", "GET", "/j", &Principal{UserID: "u", Kind: AuthJWT}, 204},
		{"jwt passes unlisted", "GET", "/unlisted", &Principal{UserID: "u", Kind: AuthJWT}, 204},
		{"no principal is rejected", "GET", "/unlisted", nil, 401},
		{"no principal on a listed route is rejected", "GET", "/r", nil, 401},
		{"principal of an unknown kind is rejected", "GET", "/unlisted", &Principal{UserID: "u"}, 401},
		{"head classified like get", "HEAD", "/r", pat("read"), 204},
		{"head without read", "HEAD", "/r", pat("drafts"), 403},
		{"404 stays 404", "GET", "/nope", pat(), 404},
		{"405 stays 405", "DELETE", "/r", pat(), 405},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if rec := serve(mux, table, tt.method, tt.path, tt.p); rec.Code != tt.want {
				t.Fatalf("status = %d, want %d", rec.Code, tt.want)
			}
		})
	}
}

func TestScopeGuardForbiddenBody(t *testing.T) {
	mux, table := scopeFixture()
	rec := serve(mux, table, "POST", "/x", pat("read"))
	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["code"] != "insufficient_scope" || body["error"] == "" {
		t.Fatalf("body = %v", body)
	}
	if rec.Header().Get("Content-Type") != "application/json" {
		t.Fatalf("content type = %q", rec.Header().Get("Content-Type"))
	}
}
