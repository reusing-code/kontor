package server

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/reusing-code/kontor/backend/internal/config"
	"github.com/reusing-code/kontor/backend/internal/modules/ledger"
	"github.com/reusing-code/kontor/backend/internal/paperless/paperlesstest"
	"github.com/reusing-code/kontor/backend/internal/pat"
	"github.com/reusing-code/kontor/backend/internal/storage/link"
)

type bearerTransport struct{ token string }

func (b bearerTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	r.Header.Set("Authorization", "Bearer "+b.token)
	return http.DefaultTransport.RoundTrip(r)
}

func (a *testApp) mcpClient(t *testing.T, token string) *mcp.ClientSession {
	t.Helper()
	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "1"}, nil)
	cs, err := client.Connect(t.Context(), &mcp.StreamableClientTransport{
		Endpoint:             a.srv.URL + "/mcp",
		HTTPClient:           &http.Client{Transport: bearerTransport{token}},
		DisableStandaloneSSE: true,
	}, nil)
	if err != nil {
		t.Fatalf("connecting to /mcp: %v", err)
	}
	t.Cleanup(func() { cs.Close() })
	return cs
}

func (a *testApp) mcpPost(t *testing.T, authorization string) *http.Response {
	t.Helper()
	req, err := http.NewRequest("POST", a.srv.URL+"/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"ping"}`))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	if authorization != "" {
		req.Header.Set("Authorization", authorization)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	return resp
}

type moduleList struct {
	Items []struct {
		ID      string `json:"id"`
		Enabled bool   `json:"enabled"`
	} `json:"items"`
}

func listModules(t *testing.T, cs *mcp.ClientSession) moduleList {
	t.Helper()
	res, err := cs.CallTool(t.Context(), &mcp.CallToolParams{Name: "list_modules", Arguments: map[string]any{}})
	if err != nil || res.IsError {
		t.Fatalf("list_modules: %v %+v", err, res)
	}
	b, _ := json.Marshal(res.StructuredContent)
	var out moduleList
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestMCPThroughFullChain(t *testing.T) {
	a := newTestApp(t)
	userID, _ := a.register(t, "mcp@example.com")
	_, raw := a.newPAT(t, userID, nil, pat.ScopeRead)
	cs := a.mcpClient(t, raw)

	tools, err := cs.ListTools(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, tool := range tools.Tools {
		names = append(names, tool.Name)
	}
	if !strings.Contains(strings.Join(names, ","), "list_modules") {
		t.Fatalf("tools = %v", names)
	}
	out := listModules(t, cs)
	if len(out.Items) != 4 {
		t.Fatalf("modules = %+v", out.Items)
	}
	for _, m := range out.Items {
		if !m.Enabled {
			t.Errorf("%s disabled by default", m.ID)
		}
	}
}

func TestMCPAuthFailures(t *testing.T) {
	a := newTestApp(t)
	userID, jwt := a.register(t, "auth@example.com")
	past := time.Now().Add(-time.Minute)
	_, expired := a.newPAT(t, userID, &past, pat.ScopeRead)
	tok, revoked := a.newPAT(t, userID, nil, pat.ScopeRead)
	if err := a.patStore().Revoke(t.Context(), userID, tok.ID); err != nil {
		t.Fatal(err)
	}

	for name, header := range map[string]string{
		"missing": "",
		"invalid": "Bearer kpat_invalid",
		"expired": "Bearer " + expired,
		"revoked": "Bearer " + revoked,
		"jwt":     "Bearer " + jwt,
	} {
		t.Run(name, func(t *testing.T) {
			resp := a.mcpPost(t, header)
			if resp.StatusCode != http.StatusUnauthorized {
				t.Fatalf("status = %d, want 401", resp.StatusCode)
			}
			if resp.Header.Get("WWW-Authenticate") != "Bearer" {
				t.Fatalf("WWW-Authenticate = %q", resp.Header.Get("WWW-Authenticate"))
			}
		})
	}
}

func TestMCPRevocationAppliesToNextRequest(t *testing.T) {
	a := newTestApp(t)
	userID, _ := a.register(t, "revoke@example.com")
	tok, raw := a.newPAT(t, userID, nil, pat.ScopeRead)
	cs := a.mcpClient(t, raw)
	listModules(t, cs)

	if err := a.patStore().Revoke(t.Context(), userID, tok.ID); err != nil {
		t.Fatal(err)
	}
	if resp := a.mcpPost(t, "Bearer "+raw); resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("after revoke = %d", resp.StatusCode)
	}
	if _, err := cs.CallTool(t.Context(), &mcp.CallToolParams{Name: "list_modules", Arguments: map[string]any{}}); err == nil {
		t.Fatal("call with a revoked token succeeded")
	}
}

func TestMCPModuleToggleAppliesWithoutRestart(t *testing.T) {
	a := newTestApp(t)
	userID, jwt := a.register(t, "toggle@example.com")
	_, raw := a.newPAT(t, userID, nil, pat.ScopeRead)
	cs := a.mcpClient(t, raw)

	enabled := func() map[string]bool {
		m := map[string]bool{}
		for _, item := range listModules(t, cs).Items {
			m[item.ID] = item.Enabled
		}
		return m
	}
	if !enabled()["ledger"] {
		t.Fatal("ledger should start enabled")
	}
	status, body := a.request(t, "PUT", "/api/v1/settings", jwt, `{"renewalDays":90,"reminderFrequency":"disabled","enabledModules":["contracts","purchases","auto"]}`)
	if status != 200 {
		t.Fatalf("settings = %d %s", status, body)
	}
	if enabled()["ledger"] {
		t.Fatal("ledger still enabled after the toggle")
	}
}

func TestMCPAndRESTNeverLogTheToken(t *testing.T) {
	var logs bytes.Buffer
	a := newTestAppWithLogger(t, slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug})))
	userID, _ := a.register(t, "log@example.com")
	_, raw := a.newPAT(t, userID, nil, pat.ScopeRead)

	if status, _ := a.request(t, "GET", "/api/v1/export", raw, ""); status != 200 {
		t.Fatalf("export = %d", status)
	}
	listModules(t, a.mcpClient(t, raw))
	a.mcpPost(t, "Bearer kpat_wrong")

	out := logs.String()
	if !strings.Contains(out, "path=/mcp") {
		t.Fatalf("expected request logs for /mcp: %s", out)
	}
	if strings.Contains(out, raw) || strings.Contains(out, strings.TrimPrefix(raw, "kpat_")) || strings.Contains(out, "kpat_wrong") {
		t.Fatal("a token reached the log")
	}
}

func TestMCPMethodNotAllowedForGET(t *testing.T) {
	a := newTestApp(t)
	userID, _ := a.register(t, "get@example.com")
	_, raw := a.newPAT(t, userID, nil, pat.ScopeRead)
	if status, _ := a.request(t, "GET", "/mcp", raw, ""); status != http.StatusMethodNotAllowed {
		t.Fatalf("GET /mcp = %d, want 405", status)
	}
}

// entities creates one entity of every attachable type for the user through
// the real routes and returns their IDs by entity type.
func (a *testApp) entities(t *testing.T, userID, jwt string) map[string]string {
	t.Helper()
	create := func(method, path, body string) string {
		t.Helper()
		status, resp := a.request(t, method, path, jwt, body)
		if status != http.StatusCreated {
			t.Fatalf("%s %s = %d %s", method, path, status, resp)
		}
		var out struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal([]byte(resp), &out); err != nil {
			t.Fatal(err)
		}
		return out.ID
	}
	firstCategory := func(module string) string {
		t.Helper()
		status, resp := a.request(t, "GET", "/api/v1/modules/"+module+"/categories", jwt, "")
		var cats []struct {
			ID string `json:"id"`
		}
		if status != 200 || json.Unmarshal([]byte(resp), &cats) != nil || len(cats) == 0 {
			t.Fatalf("categories of %s = %d %s", module, status, resp)
		}
		return cats[0].ID
	}

	ids := map[string]string{}
	ids["contract"] = create("POST", "/api/v1/categories/"+firstCategory("contracts")+"/contracts",
		`{"name":"Home insurance","billingInterval":"monthly","startDate":"2026-01-01","minimumDurationMonths":12,"extensionDurationMonths":12,"noticePeriodMonths":3}`)
	ids["purchase"] = create("POST", "/api/v1/categories/"+firstCategory("purchases")+"/purchases", `{"itemName":"Drill"}`)
	ids["vehicle"] = create("POST", "/api/v1/vehicles", `{"name":"Golf"}`)
	ids["cost"] = create("POST", "/api/v1/vehicles/"+ids["vehicle"]+"/costs", `{"type":"service","date":"2026-09-15"}`)
	ids["transaction"] = a.ledgerTransaction(t, userID)
	return ids
}

func (a *testApp) ledgerTransaction(t *testing.T, userID string) string {
	t.Helper()
	store := ledger.NewStore(a.engine, link.NewRegistry())
	now := time.Now().UTC()
	accountID, txnID, batchID := uuid.New(), uuid.New(), uuid.New()
	if err := store.CreateLedgerAccount(t.Context(), userID, ledger.LedgerAccount{ID: accountID, Name: "Main", Bank: "DKB", Currency: "EUR", CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
	batch := ledger.LedgerImportBatch{ID: batchID, AccountID: accountID, SourceType: "dkb.csv", ParserVersion: "1", Filename: "t.csv",
		FileSHA256: "hash-" + batchID.String(), Status: ledger.ImportStatusCommitted, TotalRows: 1, CreatedAt: now, UpdatedAt: now}
	txn := ledger.LedgerTransaction{ID: txnID, AccountID: accountID, BookingDate: "2026-10-01", AmountMinor: -1299, Currency: "EUR",
		Fingerprint: "fp-" + txnID.String(), ImportBatchID: batchID, CreatedAt: now, UpdatedAt: now}
	if _, err := store.CommitLedgerImport(t.Context(), userID, batch, []ledger.LedgerTransaction{txn}); err != nil {
		t.Fatal(err)
	}
	return txnID.String()
}

const testPublicURL = "https://kontor.example.com"

func (a *testApp) configurePaperless(t *testing.T, jwt, baseURL string) {
	t.Helper()
	if status, body := a.request(t, "PUT", "/api/v1/paperless/config", jwt, `{"baseUrl":"`+baseURL+`","token":"secret"}`); status != 200 {
		t.Fatalf("paperless config = %d %s", status, body)
	}
}

func TestMCPAttachToRealEntities(t *testing.T) {
	fake := paperlesstest.New(t)
	fake.AddDocument(paperlesstest.Document{ID: 7, Title: "Invoice"})
	a := newTestApp(t, func(c *config.Config) { c.PublicURL = testPublicURL })
	userID, jwt := a.register(t, "attach-real@example.com")
	a.configurePaperless(t, jwt, fake.URL)
	ids := a.entities(t, userID, jwt)
	_, raw := a.newPAT(t, userID, nil, pat.ScopeLinks)
	cs := a.mcpClient(t, raw)

	wantPath := map[string]string{
		"contract":    "/contracts/" + ids["contract"],
		"purchase":    "/purchases/" + ids["purchase"],
		"vehicle":     "/auto/vehicles/" + ids["vehicle"],
		"cost":        "/auto/vehicles/" + ids["vehicle"],
		"transaction": "/ledger/transactions/" + ids["transaction"],
	}
	for entityType, id := range ids {
		t.Run(entityType, func(t *testing.T) {
			res, err := cs.CallTool(t.Context(), &mcp.CallToolParams{Name: "attach_paperless_documents", Arguments: map[string]any{
				"entityType": entityType, "entityId": id, "documents": []map[string]any{{"id": 7, "title": "Invoice"}},
			}})
			if err != nil || res.IsError {
				t.Fatalf("attach: %v %+v", err, res)
			}
			status, body := a.request(t, "GET", "/api/v1/paperless/links/"+entityType+"/"+id, jwt, "")
			if status != 200 || !strings.Contains(body, `"documentId":7`) || !strings.Contains(body, testPublicURL+wantPath[entityType]) {
				t.Fatalf("links = %d %s", status, body)
			}
		})
	}
}

func TestMCPAttachIsolationAndModuleState(t *testing.T) {
	fake := paperlesstest.New(t)
	a := newTestApp(t, func(c *config.Config) { c.PublicURL = testPublicURL })
	ownerID, ownerJWT := a.register(t, "owner@example.com")
	otherID, otherJWT := a.register(t, "other@example.com")
	a.configurePaperless(t, ownerJWT, fake.URL)
	a.configurePaperless(t, otherJWT, fake.URL)
	ids := a.entities(t, ownerID, ownerJWT)

	attach := func(cs *mcp.ClientSession, entityType, id string) (bool, string) {
		res, err := cs.CallTool(t.Context(), &mcp.CallToolParams{Name: "attach_paperless_documents", Arguments: map[string]any{
			"entityType": entityType, "entityId": id, "documents": []map[string]any{{"id": 1}},
		}})
		if err != nil {
			t.Fatal(err)
		}
		var text string
		for _, c := range res.Content {
			text += c.(*mcp.TextContent).Text
		}
		return res.IsError, text
	}

	_, otherRaw := a.newPAT(t, otherID, nil, pat.ScopeLinks)
	otherClient := a.mcpClient(t, otherRaw)
	for entityType, id := range ids {
		if isErr, text := attach(otherClient, entityType, id); !isErr || !strings.HasPrefix(text, "not_found") {
			t.Errorf("%s of another user: %q", entityType, text)
		}
	}

	_, ownerRaw := a.newPAT(t, ownerID, nil, pat.ScopeLinks)
	ownerClient := a.mcpClient(t, ownerRaw)
	status, body := a.request(t, "PUT", "/api/v1/settings", ownerJWT, `{"renewalDays":90,"reminderFrequency":"disabled","enabledModules":["purchases","auto"]}`)
	if status != 200 {
		t.Fatalf("settings = %d %s", status, body)
	}
	for entityType, wantModule := range map[string]string{"contract": "contracts", "transaction": "ledger"} {
		if isErr, text := attach(ownerClient, entityType, ids[entityType]); !isErr || !strings.HasPrefix(text, "module_disabled") || !strings.Contains(text, wantModule) {
			t.Errorf("%s with disabled module: %q", entityType, text)
		}
	}
	if isErr, text := attach(ownerClient, "purchase", ids["purchase"]); isErr {
		t.Errorf("purchase with enabled module: %q", text)
	}
}
