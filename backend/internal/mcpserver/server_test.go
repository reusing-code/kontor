package mcpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/reusing-code/kontor/backend/internal/cryptoutil"
	"github.com/reusing-code/kontor/backend/internal/drafts"
	"github.com/reusing-code/kontor/backend/internal/mcptool"
	"github.com/reusing-code/kontor/backend/internal/middleware"
	"github.com/reusing-code/kontor/backend/internal/paperless"
	"github.com/reusing-code/kontor/backend/internal/paperless/paperlesstest"
	"github.com/reusing-code/kontor/backend/internal/pat"
	"github.com/reusing-code/kontor/backend/internal/storage"
)

const (
	userA     = "00000000-0000-0000-0000-00000000000a"
	userB     = "00000000-0000-0000-0000-00000000000b"
	publicURL = "https://kontor.example.com"
	encKey    = "0123456789abcdef0123456789abcdef"
)

type env struct {
	srv      *httptest.Server
	store    *paperless.Store
	svc      *paperless.Service
	fake     *paperlesstest.Server
	pats     *pat.Store
	disabled map[string]bool
	entities map[string]EntityRef // "userID/type/id" → ref
	logs     *bytes.Buffer
}

func newEnv(t *testing.T, mutate ...func(*Deps)) *env {
	t.Helper()
	logs := &bytes.Buffer{}
	logger := slog.New(slog.NewTextHandler(logs, nil))
	engine, err := storage.Open(t.TempDir(), slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { engine.Close() })

	e := &env{
		store:    paperless.NewStore(engine),
		fake:     paperlesstest.New(t),
		pats:     pat.NewStore(engine, logger),
		disabled: map[string]bool{},
		entities: map[string]EntityRef{},
		logs:     logs,
	}
	e.svc = paperless.NewService(e.store, logger, encKey)

	reg := mcptool.NewRegistrar(mcptool.Env{
		Drafts: drafts.NewStore(engine),
		Enabled: func(_ context.Context, _, moduleID string) (bool, error) {
			return !e.disabled[moduleID], nil
		},
		Logger: logger,
	})
	deps := Deps{
		Registrar: reg,
		Paperless: e.svc,
		Modules: func(context.Context, string) ([]ModuleStatus, error) {
			return []ModuleStatus{{ID: "contracts", Enabled: !e.disabled["contracts"]}, {ID: "ledger", Enabled: true}}, nil
		},
		Entities: func(_ context.Context, userID, entityType string, id uuid.UUID) (EntityRef, error) {
			if ref, ok := e.entities[userID+"/"+entityType+"/"+id.String()]; ok {
				return ref, nil
			}
			return EntityRef{}, storage.ErrNotFound
		},
		PublicURL: publicURL,
	}
	for _, m := range mutate {
		m(&deps)
	}
	RegisterCoreTools(deps)

	handler := middleware.Auth([]byte("secret"), middleware.WithPATs(e.pats), middleware.PATOnly(), middleware.WithChallenge("Bearer"))(Handler(deps))
	e.srv = httptest.NewServer(handler)
	t.Cleanup(e.srv.Close)
	return e
}

func (e *env) configurePaperless(t *testing.T, userID string) {
	t.Helper()
	key, err := cryptoutil.NormalizeEncryptionKey(encKey)
	if err != nil {
		t.Fatal(err)
	}
	token, err := cryptoutil.EncryptString("paperless-secret", key)
	if err != nil {
		t.Fatal(err)
	}
	if err := e.store.PutConfig(t.Context(), userID, paperless.Config{BaseURL: e.fake.URL, EncryptedToken: token, UpdatedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
}

func (e *env) addEntity(userID, entityType, moduleID, path string) uuid.UUID {
	id := uuid.New()
	e.entities[userID+"/"+entityType+"/"+id.String()] = EntityRef{ModuleID: moduleID, Path: strings.ReplaceAll(path, "{id}", id.String())}
	return id
}

type bearer struct{ token string }

func (b bearer) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	r.Header.Set("Authorization", "Bearer "+b.token)
	return http.DefaultTransport.RoundTrip(r)
}

func (e *env) token(t *testing.T, userID string, scopes ...string) string {
	t.Helper()
	_, raw, err := e.pats.Create(t.Context(), userID, pat.CreateInput{Name: "t", Scopes: scopes})
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func (e *env) connect(t *testing.T, token string) *mcp.ClientSession {
	t.Helper()
	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "1"}, nil)
	cs, err := client.Connect(t.Context(), &mcp.StreamableClientTransport{
		Endpoint:             e.srv.URL,
		HTTPClient:           &http.Client{Transport: bearer{token}},
		DisableStandaloneSSE: true,
	}, nil)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { cs.Close() })
	return cs
}

func call(t *testing.T, cs *mcp.ClientSession, name string, args any) (*mcp.CallToolResult, string) {
	t.Helper()
	res, err := cs.CallTool(t.Context(), &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatalf("CallTool(%s): %v", name, err)
	}
	var sb strings.Builder
	for _, c := range res.Content {
		if tc, ok := c.(*mcp.TextContent); ok {
			sb.WriteString(tc.Text)
		}
	}
	return res, sb.String()
}

func structured[T any](t *testing.T, res *mcp.CallToolResult) T {
	t.Helper()
	b, err := json.Marshal(res.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	var v T
	if err := json.Unmarshal(b, &v); err != nil {
		t.Fatal(err)
	}
	return v
}

func TestInitializeListAndListModules(t *testing.T) {
	e := newEnv(t)
	cs := e.connect(t, e.token(t, userA, pat.ScopeRead, pat.ScopeLinks))

	if got := cs.InitializeResult().Instructions; got != Instructions {
		t.Fatalf("instructions = %q", got)
	}
	list, err := cs.ListTools(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	for _, tool := range list.Tools {
		names[tool.Name] = true
	}
	for _, want := range []string{"list_modules", "search_paperless_documents", "attach_paperless_documents"} {
		if !names[want] {
			t.Errorf("missing tool %s in %v", want, names)
		}
	}

	res, _ := call(t, cs, "list_modules", map[string]any{})
	if res.IsError {
		t.Fatal("list_modules failed")
	}
	out := structured[listModulesOut](t, res)
	if len(out.Items) != 2 || out.Items[0].ID != "contracts" || !out.Items[0].Enabled {
		t.Fatalf("items = %+v", out.Items)
	}

	e.disabled["contracts"] = true
	res, _ = call(t, cs, "list_modules", map[string]any{})
	if out := structured[listModulesOut](t, res); out.Items[0].Enabled {
		t.Fatal("module toggle not visible on the next call")
	}
}

func TestNoSchemaMentionsUserID(t *testing.T) {
	e := newEnv(t)
	cs := e.connect(t, e.token(t, userA, pat.ScopeRead, pat.ScopeLinks, pat.ScopeDrafts))
	list, err := cs.ListTools(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, tool := range list.Tools {
		b, _ := json.Marshal(tool.InputSchema)
		if strings.Contains(strings.ToLower(string(b)), "userid") {
			t.Errorf("%s input schema mentions userId: %s", tool.Name, b)
		}
	}
}

func TestSearchPaperless(t *testing.T) {
	e := newEnv(t)
	e.fake.AddDocument(paperlesstest.Document{ID: 7, Title: "Invoice", Created: "2026-10-01"})
	e.configurePaperless(t, userA)
	cs := e.connect(t, e.token(t, userA, pat.ScopeRead))

	res, text := call(t, cs, "search_paperless_documents", map[string]any{"query": "invoice"})
	if res.IsError {
		t.Fatalf("error: %s", text)
	}
	out := structured[searchOut](t, res)
	if out.Count != 1 || out.Page != 1 || len(out.Results) != 1 || out.Results[0].ID != 7 || out.Results[0].Snippet == "" {
		t.Fatalf("out = %+v", out)
	}

	if res, text := call(t, cs, "search_paperless_documents", map[string]any{"page": -1}); !res.IsError || !strings.HasPrefix(text, mcptool.CodeInvalidInput) {
		t.Fatalf("page -1: %v %q", res.IsError, text)
	}
}

func TestSearchPaperlessErrors(t *testing.T) {
	e := newEnv(t)
	cs := e.connect(t, e.token(t, userA, pat.ScopeRead))
	if res, text := call(t, cs, "search_paperless_documents", map[string]any{}); !res.IsError || !strings.HasPrefix(text, mcptool.CodePaperlessNotConfigured) {
		t.Fatalf("unconfigured: %q", text)
	}

	e.configurePaperless(t, userA)
	e.fake.FailAll(500)
	res, text := call(t, cs, "search_paperless_documents", map[string]any{})
	if !res.IsError || text != "paperless_unavailable: paperless did not answer the search" {
		t.Fatalf("upstream failure: %q", text)
	}
}

func attachArgs(entityType string, entityID uuid.UUID, docIDs ...int) map[string]any {
	docs := []map[string]any{}
	for _, id := range docIDs {
		docs = append(docs, map[string]any{"id": id, "title": " Doc "})
	}
	return map[string]any{"entityType": entityType, "entityId": entityID.String(), "documents": docs}
}

func TestAttachWritesLinkAndBackLink(t *testing.T) {
	e := newEnv(t)
	e.fake.AddDocument(paperlesstest.Document{ID: 7, Title: "Invoice"})
	e.configurePaperless(t, userA)
	id := e.addEntity(userA, "contract", "contracts", "/contracts/{id}")
	cs := e.connect(t, e.token(t, userA, pat.ScopeLinks))

	res, text := call(t, cs, "attach_paperless_documents", attachArgs("contract", id, 7))
	if res.IsError {
		t.Fatalf("error: %s", text)
	}
	if out := structured[attachOut](t, res); out.Linked != 1 || out.Warnings == nil {
		t.Fatalf("out = %+v", out)
	}
	links, err := e.store.ListLinks(t.Context(), userA, "contract", id.String())
	if err != nil || len(links) != 1 {
		t.Fatalf("links = %v, %v", links, err)
	}
	if links[0].EntityURL != publicURL+"/contracts/"+id.String() || links[0].Title != "Doc" || links[0].DocumentID != 7 {
		t.Fatalf("link = %+v", links[0])
	}
	doc, _ := e.fake.Document(7)
	if len(doc.CustomFields) != 1 {
		t.Fatalf("back-link not written: %+v", doc.CustomFields)
	}
}

func TestAttachWarningsAreReturned(t *testing.T) {
	e := newEnv(t)
	e.configurePaperless(t, userA)
	e.fake.FailAll(500)
	id := e.addEntity(userA, "purchase", "purchases", "/purchases/{id}")
	cs := e.connect(t, e.token(t, userA, pat.ScopeLinks))

	res, text := call(t, cs, "attach_paperless_documents", attachArgs("purchase", id, 9))
	if res.IsError {
		t.Fatalf("error: %s", text)
	}
	out := structured[attachOut](t, res)
	if out.Linked != 1 || len(out.Warnings) == 0 {
		t.Fatalf("out = %+v", out)
	}
	if links, _ := e.store.ListLinks(t.Context(), userA, "purchase", id.String()); len(links) != 1 {
		t.Fatal("link must be stored even when paperless fails")
	}
}

func TestAttachErrors(t *testing.T) {
	e := newEnv(t)
	e.configurePaperless(t, userA)
	id := e.addEntity(userA, "contract", "contracts", "/contracts/{id}")
	cs := e.connect(t, e.token(t, userA, pat.ScopeLinks))

	tooMany := make([]int, 51)
	for i := range tooMany {
		tooMany[i] = i + 1
	}
	cases := []struct {
		name string
		args map[string]any
		code string
	}{
		{"bad type", attachArgs("cost2", id, 1), mcptool.CodeInvalidInput},
		{"bad id", map[string]any{"entityType": "contract", "entityId": "nope", "documents": []map[string]any{{"id": 1}}}, mcptool.CodeInvalidInput},
		{"no documents", attachArgs("contract", id), mcptool.CodeInvalidInput},
		{"too many", attachArgs("contract", id, tooMany...), mcptool.CodeInvalidInput},
		{"bad document id", attachArgs("contract", id, 0), mcptool.CodeInvalidInput},
		{"unknown entity", attachArgs("contract", uuid.New(), 1), mcptool.CodeNotFound},
		{"wrong type for id", attachArgs("purchase", id, 1), mcptool.CodeNotFound},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			res, text := call(t, cs, "attach_paperless_documents", c.args)
			if !res.IsError || !strings.HasPrefix(text, c.code) {
				t.Fatalf("result = %v %q, want code %s", res.IsError, text, c.code)
			}
		})
	}
	if links, _ := e.store.ListLinks(t.Context(), userA, "contract", id.String()); len(links) != 0 {
		t.Fatal("failed calls stored links")
	}
}

func TestAttachWithoutPublicURL(t *testing.T) {
	e := newEnv(t, func(d *Deps) { d.PublicURL = "" })
	e.configurePaperless(t, userA)
	id := e.addEntity(userA, "contract", "contracts", "/contracts/{id}")
	cs := e.connect(t, e.token(t, userA, pat.ScopeLinks))

	res, text := call(t, cs, "attach_paperless_documents", attachArgs("contract", id, 7))
	if !res.IsError || !strings.HasPrefix(text, mcptool.CodePublicURLNotConfigured) {
		t.Fatalf("result = %q", text)
	}
	if links, _ := e.store.ListLinks(t.Context(), userA, "contract", id.String()); len(links) != 0 {
		t.Fatal("link stored without PUBLIC_URL")
	}
}

func TestAttachDisabledModuleAndUnconfiguredPaperless(t *testing.T) {
	e := newEnv(t)
	id := e.addEntity(userA, "contract", "contracts", "/contracts/{id}")
	cs := e.connect(t, e.token(t, userA, pat.ScopeLinks))

	if res, text := call(t, cs, "attach_paperless_documents", attachArgs("contract", id, 7)); !res.IsError || !strings.HasPrefix(text, mcptool.CodePaperlessNotConfigured) {
		t.Fatalf("unconfigured: %q", text)
	}
	e.configurePaperless(t, userA)
	e.disabled["contracts"] = true
	if res, text := call(t, cs, "attach_paperless_documents", attachArgs("contract", id, 7)); !res.IsError || !strings.HasPrefix(text, mcptool.CodeModuleDisabled) {
		t.Fatalf("disabled: %q", text)
	}
	if links, _ := e.store.ListLinks(t.Context(), userA, "contract", id.String()); len(links) != 0 {
		t.Fatal("link stored for a disabled module")
	}
}

func TestAttachScopeRequired(t *testing.T) {
	e := newEnv(t)
	e.configurePaperless(t, userA)
	id := e.addEntity(userA, "contract", "contracts", "/contracts/{id}")
	cs := e.connect(t, e.token(t, userA, pat.ScopeRead))
	if res, text := call(t, cs, "attach_paperless_documents", attachArgs("contract", id, 7)); !res.IsError || !strings.HasPrefix(text, mcptool.CodeInsufficientScope) {
		t.Fatalf("read token: %q", text)
	}
}

func TestNoCrossUserAccess(t *testing.T) {
	e := newEnv(t)
	e.configurePaperless(t, userA)
	e.configurePaperless(t, userB)
	idA := e.addEntity(userA, "contract", "contracts", "/contracts/{id}")
	cs := e.connect(t, e.token(t, userB, pat.ScopeLinks))

	res, text := call(t, cs, "attach_paperless_documents", attachArgs("contract", idA, 7))
	if !res.IsError || !strings.HasPrefix(text, mcptool.CodeNotFound) {
		t.Fatalf("result = %q", text)
	}
	for _, user := range []string{userA, userB} {
		if links, _ := e.store.ListLinks(t.Context(), user, "contract", idA.String()); len(links) != 0 {
			t.Fatalf("link stored for %s", user)
		}
	}
}

func TestHandlerRejectsMissingOrWrongAuth(t *testing.T) {
	e := newEnv(t)
	for name, header := range map[string]string{"none": "", "unknown pat": "Bearer kpat_unknown", "jwt-like": "Bearer a.b.c"} {
		req, _ := http.NewRequest("POST", e.srv.URL, strings.NewReader(`{}`))
		if header != "" {
			req.Header.Set("Authorization", header)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusUnauthorized || resp.Header.Get("WWW-Authenticate") != "Bearer" {
			t.Errorf("%s: status %d, challenge %q", name, resp.StatusCode, resp.Header.Get("WWW-Authenticate"))
		}
	}
}

func TestHandlerWithoutAuthMiddlewareIs401(t *testing.T) {
	e := newEnv(t)
	bare := httptest.NewServer(Handler(Deps{Registrar: mcptool.NewRegistrar(mcptool.Env{Logger: slog.New(slog.DiscardHandler)})}))
	defer bare.Close()
	_ = e
	resp, err := http.Post(bare.URL, "application/json", strings.NewReader(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d", resp.StatusCode)
	}
}
