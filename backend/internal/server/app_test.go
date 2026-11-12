package server

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/reusing-code/kontor/backend/internal/config"
	"github.com/reusing-code/kontor/backend/internal/middleware"
	"github.com/reusing-code/kontor/backend/internal/paperless/paperlesstest"
	"github.com/reusing-code/kontor/backend/internal/pat"
	"github.com/reusing-code/kontor/backend/internal/storage"
)

var appJWTSecret = []byte("test-secret-key")

type testApp struct {
	*app
	srv    *httptest.Server
	engine *storage.Engine
}

// newTestApp builds the real application (LLM and FX disabled, no
// background services) behind an httptest server.
func newTestApp(t *testing.T, mutate ...func(*config.Config)) *testApp {
	t.Helper()
	return newTestAppWithLogger(t, slog.New(slog.DiscardHandler), mutate...)
}

func newTestAppWithLogger(t *testing.T, logger *slog.Logger, mutate ...func(*config.Config)) *testApp {
	t.Helper()
	engine, err := storage.Open(t.TempDir(), logger)
	if err != nil {
		t.Fatalf("opening engine: %v", err)
	}
	t.Cleanup(func() { engine.Close() })

	cfg := config.Config{
		JWTSecret:          string(appJWTSecret),
		FXRatesURL:         "off",
		EmailEncryptionKey: testEncryptionKey,
		AuthRateLimit:      0,
	}
	for _, m := range mutate {
		m(&cfg)
	}
	a, err := New(cfg, logger, engine).buildApp()
	if err != nil {
		t.Fatalf("building app: %v", err)
	}
	srv := httptest.NewServer(a.handler)
	t.Cleanup(srv.Close)
	return &testApp{app: a, srv: srv, engine: engine}
}

// request sends a request with an optional bearer token and returns status and body.
func (a *testApp) request(t *testing.T, method, path, token, body string) (int, string) {
	t.Helper()
	req, err := http.NewRequest(method, a.srv.URL+path, bytes.NewBufferString(body))
	if err != nil {
		t.Fatal(err)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b)
}

// register creates a user through the API and returns the user ID and JWT.
func (a *testApp) register(t *testing.T, email string) (userID, jwt string) {
	t.Helper()
	status, body := a.request(t, "POST", "/api/v1/auth/register", "", `{"email":"`+email+`","password":"password123"}`)
	if status != http.StatusCreated {
		t.Fatalf("register = %d %s", status, body)
	}
	var resp struct {
		Token string `json:"token"`
		User  struct {
			ID string `json:"id"`
		} `json:"user"`
	}
	if err := json.Unmarshal([]byte(body), &resp); err != nil {
		t.Fatal(err)
	}
	return resp.User.ID, resp.Token
}

func (a *testApp) patStore() *pat.Store {
	return pat.NewStore(a.engine, slog.New(slog.DiscardHandler))
}

func (a *testApp) newPAT(t *testing.T, userID string, expires *time.Time, scopes ...string) (pat.Token, string) {
	t.Helper()
	tok, raw, err := a.patStore().Create(t.Context(), userID, pat.CreateInput{Name: "test", Scopes: scopes, ExpiresAt: expires})
	if err != nil {
		t.Fatal(err)
	}
	return tok, raw
}

func TestRouteRulesCoverRoutes(t *testing.T) {
	a := newTestApp(t)

	registered := map[string]bool{}
	for _, p := range a.patterns {
		if registered[p] {
			t.Errorf("pattern registered twice: %s", p)
		}
		registered[p] = true
		if _, ok := routeRules[p]; !ok {
			t.Errorf("route %q has no entry in routeRules", p)
		}
	}
	for p := range routeRules {
		if !registered[p] {
			t.Errorf("routeRules has a stale entry %q", p)
		}
	}

	jwtOnly := []string{
		"GET /api/v1/tokens", "POST /api/v1/tokens", "DELETE /api/v1/tokens/{id}",
		"PUT /api/v1/settings/password",
	}
	drafts := []string{
		"PUT /api/v1/drafts/{id}", "POST /api/v1/drafts/{id}/accept", "POST /api/v1/drafts/{id}/reject",
	}
	links := []string{
		"POST /api/v1/paperless/links/{entityType}/{entityId}",
		"POST /api/v1/ledger/transactions/{transactionId}/references",
	}
	exact := func(rule middleware.Rule, want []string) {
		var got []string
		for p, r := range routeRules {
			if r == rule {
				got = append(got, p)
			}
		}
		slices.Sort(got)
		slices.Sort(want)
		if !slices.Equal(got, want) {
			t.Errorf("rule %s patterns = %v, want %v", rule, got, want)
		}
	}
	exact(middleware.RuleJWTOnly, jwtOnly)
	exact(middleware.RuleDrafts, drafts)
	exact(middleware.RuleLinks, links)

	for p, r := range routeRules {
		isGet := strings.HasPrefix(p, "GET ")
		switch {
		case isGet && r != middleware.RuleRead && r != middleware.RuleJWTOnly:
			t.Errorf("GET route %q has rule %s", p, r)
		case !isGet && r == middleware.RuleRead:
			t.Errorf("non-GET route %q has rule read", p)
		}
	}
}

func TestPATRESTAccess(t *testing.T) {
	a := newTestApp(t)
	userID, jwt := a.register(t, "pat@example.com")
	_, readTok := a.newPAT(t, userID, nil, pat.ScopeRead)
	_, draftTok := a.newPAT(t, userID, nil, pat.ScopeDrafts)
	_, linkTok := a.newPAT(t, userID, nil, pat.ScopeLinks)
	_, allTok := a.newPAT(t, userID, nil, pat.ScopeRead, pat.ScopeDrafts, pat.ScopeLinks)

	t.Run("read reaches export", func(t *testing.T) {
		if status, body := a.request(t, "GET", "/api/v1/export", readTok, ""); status != 200 {
			t.Fatalf("export = %d %s", status, body)
		}
	})

	insufficient := func(t *testing.T, status int, body string) {
		t.Helper()
		if status != http.StatusForbidden || !strings.Contains(body, `"insufficient_scope"`) {
			t.Fatalf("got %d %s, want 403 insufficient_scope", status, body)
		}
	}

	t.Run("denied routes", func(t *testing.T) {
		for _, tok := range []string{readTok, allTok} {
			status, body := a.request(t, "POST", "/api/v1/categories/"+uuid.NewString()+"/contracts", tok, `{}`)
			insufficient(t, status, body)
			status, body = a.request(t, "PUT", "/api/v1/settings", tok, `{}`)
			insufficient(t, status, body)
		}
	})

	t.Run("jwt-only routes", func(t *testing.T) {
		for _, route := range [][2]string{
			{"GET", "/api/v1/tokens"},
			{"POST", "/api/v1/tokens"},
			{"DELETE", "/api/v1/tokens/" + uuid.NewString()},
			{"PUT", "/api/v1/settings/password"},
		} {
			status, body := a.request(t, route[0], route[1], allTok, `{}`)
			insufficient(t, status, body)
		}
		if status, _ := a.request(t, "GET", "/api/v1/tokens", jwt, ""); status != 200 {
			t.Fatalf("jwt list tokens = %d", status)
		}
	})

	t.Run("drafts scope", func(t *testing.T) {
		path := "/api/v1/drafts/" + uuid.NewString()
		status, body := a.request(t, "PUT", path, readTok, `{}`)
		insufficient(t, status, body)
		if status, body := a.request(t, "PUT", path, draftTok, `{}`); status == http.StatusForbidden {
			t.Fatalf("drafts token blocked: %d %s", status, body)
		}
	})

	t.Run("links scope", func(t *testing.T) {
		path := "/api/v1/paperless/links/contract/" + uuid.NewString()
		status, body := a.request(t, "POST", path, readTok, `{}`)
		insufficient(t, status, body)
		status, body = a.request(t, "POST", path, linkTok, `{}`)
		if status == http.StatusForbidden && strings.Contains(body, "insufficient_scope") {
			t.Fatalf("links token blocked: %d %s", status, body)
		}
	})

	t.Run("transaction reference needs links", func(t *testing.T) {
		path := "/api/v1/ledger/transactions/" + uuid.NewString() + "/references"
		body := `{"type":"purchase","targetId":"` + uuid.NewString() + `"}`
		for _, tok := range []string{readTok, draftTok} {
			status, resp := a.request(t, "POST", path, tok, body)
			insufficient(t, status, resp)
		}
		if status, resp := a.request(t, "POST", path, linkTok, body); status != http.StatusNotFound {
			t.Fatalf("links token = %d %s, want 404 from the handler", status, resp)
		}
	})

	t.Run("404 and 405 are not rewritten", func(t *testing.T) {
		if status, _ := a.request(t, "GET", "/api/v1/nope", allTok, ""); status != http.StatusNotFound {
			t.Fatalf("unknown route = %d", status)
		}
		if status, _ := a.request(t, "PATCH", "/api/v1/export", allTok, ""); status != http.StatusMethodNotAllowed {
			t.Fatalf("wrong method = %d", status)
		}
	})

	t.Run("module gate applies", func(t *testing.T) {
		status, body := a.request(t, "PUT", "/api/v1/settings", jwt, `{"renewalDays":90,"reminderFrequency":"disabled","enabledModules":["purchases","auto","ledger"]}`)
		if status != 200 {
			t.Fatalf("settings = %d %s", status, body)
		}
		status, body = a.request(t, "GET", "/api/v1/contracts", readTok, "")
		if status != http.StatusForbidden || !strings.Contains(body, `"module_disabled"`) {
			t.Fatalf("got %d %s, want 403 module_disabled", status, body)
		}
	})

	t.Run("jwt unchanged", func(t *testing.T) {
		if status, _ := a.request(t, "GET", "/api/v1/export", jwt, ""); status != 200 {
			t.Fatalf("jwt export = %d", status)
		}
		if status, _ := a.request(t, "PUT", "/api/v1/settings", jwt, `{"renewalDays":30,"reminderFrequency":"disabled"}`); status != 200 {
			t.Fatalf("jwt settings = %d", status)
		}
	})
}

func TestPATLifecycleOverREST(t *testing.T) {
	a := newTestApp(t)
	userID, jwt := a.register(t, "life@example.com")

	status, body := a.request(t, "POST", "/api/v1/tokens", jwt, `{"name":"agent","scopes":["read"]}`)
	if status != http.StatusCreated {
		t.Fatalf("create = %d %s", status, body)
	}
	var created struct {
		ID    string `json:"id"`
		Token string `json:"token"`
	}
	if err := json.Unmarshal([]byte(body), &created); err != nil {
		t.Fatal(err)
	}
	if status, _ := a.request(t, "GET", "/api/v1/export", created.Token, ""); status != 200 {
		t.Fatalf("new token export = %d", status)
	}
	if status, _ := a.request(t, "DELETE", "/api/v1/tokens/"+created.ID, jwt, ""); status != http.StatusNoContent {
		t.Fatalf("revoke = %d", status)
	}
	if status, _ := a.request(t, "GET", "/api/v1/export", created.Token, ""); status != http.StatusUnauthorized {
		t.Fatalf("revoked token = %d, want 401", status)
	}

	past := time.Now().Add(-time.Minute)
	_, expired := a.newPAT(t, userID, &past, pat.ScopeRead)
	if status, _ := a.request(t, "GET", "/api/v1/export", expired, ""); status != http.StatusUnauthorized {
		t.Fatalf("expired token = %d, want 401", status)
	}
	if status, _ := a.request(t, "GET", "/api/v1/export", "kpat_unknown", ""); status != http.StatusUnauthorized {
		t.Fatalf("unknown token = %d, want 401", status)
	}
}

func TestExportContainsNoTokenData(t *testing.T) {
	a := newTestApp(t)
	userID, jwt := a.register(t, "export@example.com")
	tok, raw := a.newPAT(t, userID, nil, pat.ScopeRead)

	status, body := a.request(t, "GET", "/api/v1/export", jwt, "")
	if status != 200 {
		t.Fatalf("export = %d", status)
	}
	if strings.Contains(body, raw) || strings.Contains(body, tok.ID.String()) || strings.Contains(body, "kpat_") {
		t.Fatal("export contains token data")
	}
}

func TestPATAttachesPaperlessDocuments(t *testing.T) {
	fake := paperlesstest.New(t)
	fake.AddDocument(paperlesstest.Document{ID: 7, Title: "Invoice"})

	a := newTestApp(t)
	userID, jwt := a.register(t, "attach@example.com")
	_, readTok := a.newPAT(t, userID, nil, pat.ScopeRead)
	_, linkTok := a.newPAT(t, userID, nil, pat.ScopeLinks)

	status, body := a.request(t, "PUT", "/api/v1/paperless/config", jwt, `{"baseUrl":"`+fake.URL+`","token":"secret"}`)
	if status != 200 {
		t.Fatalf("paperless config = %d %s", status, body)
	}

	entityID := uuid.NewString()
	path := "/api/v1/paperless/links/purchase/" + entityID
	payload := `{"entityUrl":"https://kontor.example.com/purchases/x","documents":[{"id":7,"title":"Invoice"}]}`

	if status, body := a.request(t, "POST", path, readTok, payload); status != http.StatusForbidden || !strings.Contains(body, "insufficient_scope") {
		t.Fatalf("read token attach = %d %s", status, body)
	}
	if status, body := a.request(t, "POST", path, linkTok, payload); status != 200 {
		t.Fatalf("links token attach = %d %s", status, body)
	}
	status, body = a.request(t, "GET", path, readTok, "")
	if status != 200 || !strings.Contains(body, `"documentId":7`) {
		t.Fatalf("list links = %d %s", status, body)
	}
}

func TestLedgerAccountCreation(t *testing.T) {
	a := newTestApp(t)
	userID, jwt := a.register(t, "acct@example.com")
	_, allTok := a.newPAT(t, userID, nil, pat.ScopeRead, pat.ScopeDrafts, pat.ScopeLinks)
	body := `{"name":"Main","bank":"DKB","iban":"DE89370400440532013000"}`

	if status, resp := a.request(t, "POST", "/api/v1/ledger/accounts", allTok, body); status != http.StatusForbidden || !strings.Contains(resp, "insufficient_scope") {
		t.Fatalf("pat create = %d %s", status, resp)
	}
	if status, resp := a.request(t, "POST", "/api/v1/ledger/accounts", jwt, body); status != http.StatusCreated {
		t.Fatalf("jwt create = %d %s", status, resp)
	}
	if status, resp := a.request(t, "GET", "/api/v1/ledger/accounts", allTok, ""); status != 200 || !strings.Contains(resp, "Main") {
		t.Fatalf("list = %d %s", status, resp)
	}

	status, resp := a.request(t, "PUT", "/api/v1/settings", jwt, `{"renewalDays":90,"reminderFrequency":"disabled","enabledModules":["contracts","purchases","auto"]}`)
	if status != 200 {
		t.Fatalf("settings = %d %s", status, resp)
	}
	if status, resp := a.request(t, "POST", "/api/v1/ledger/accounts", jwt, `{"name":"Other","bank":"B"}`); status != http.StatusForbidden || !strings.Contains(resp, "module_disabled") {
		t.Fatalf("disabled module = %d %s", status, resp)
	}
}

func TestDraftSummaryRoute(t *testing.T) {
	a := newTestApp(t)
	userID, jwt := a.register(t, "sum@example.com")
	_, readTok := a.newPAT(t, userID, nil, pat.ScopeRead)

	for _, tok := range []string{jwt, readTok} {
		status, resp := a.request(t, "GET", "/api/v1/drafts/summary", tok, "")
		if status != 200 || strings.TrimSpace(resp) != `{"pending":0,"total":0}` {
			t.Fatalf("summary = %d %s", status, resp)
		}
	}
	_, draftTok := a.newPAT(t, userID, nil, pat.ScopeDrafts)
	if status, _ := a.request(t, "GET", "/api/v1/drafts/summary", draftTok, ""); status != http.StatusForbidden {
		t.Fatalf("drafts-only token = %d, want 403", status)
	}
}
