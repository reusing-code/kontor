package server

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"slices"
	"strings"
	"sync"
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

// allowedTools is the complete tool surface an agent may see, by scope. A new
// tool must be added here on purpose: the drafts-only guarantee test fails
// for anything else.
var allowedTools = map[string][]string{
	pat.ScopeRead: {
		"list_modules", "search_paperless_documents",
		"list_contract_categories", "search_contracts",
		"list_purchase_categories", "search_purchases",
		"list_vehicles", "search_vehicle_costs",
		"list_ledger_accounts", "list_ledger_categories", "search_ledger_transactions",
	},
	pat.ScopeDrafts: {
		"create_contract_draft", "create_contract_category_draft",
		"create_purchase_draft", "create_purchase_category_draft",
		"create_vehicle_cost_draft", "create_vehicle_draft",
		"create_ledger_category_draft", "create_ledger_account_draft",
	},
	pat.ScopeLinks: {
		"attach_paperless_documents", "link_transaction",
	},
}

func (a *testApp) toolNames(t *testing.T, cs *mcp.ClientSession) []string {
	t.Helper()
	res, err := cs.ListTools(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	names := []string{}
	for _, tool := range res.Tools {
		names = append(names, tool.Name)
	}
	slices.Sort(names)
	return names
}

func sorted(lists ...[]string) []string {
	var all []string
	for _, l := range lists {
		all = append(all, l...)
	}
	slices.Sort(all)
	return all
}

func TestMCPToolAllowList(t *testing.T) {
	if n := len(sorted(allowedTools[pat.ScopeRead], allowedTools[pat.ScopeDrafts], allowedTools[pat.ScopeLinks])); n != 21 {
		t.Fatalf("the allow-list has %d tools, want 21", n)
	}
	a := newTestApp(t)
	userID, _ := a.register(t, "allow@example.com")

	_, allTok := a.newPAT(t, userID, nil, pat.ScopeRead, pat.ScopeDrafts, pat.ScopeLinks)
	want := sorted(allowedTools[pat.ScopeRead], allowedTools[pat.ScopeDrafts], allowedTools[pat.ScopeLinks])
	if got := a.toolNames(t, a.mcpClient(t, allTok)); !slices.Equal(got, want) {
		t.Fatalf("tools with all scopes:\n got %v\nwant %v", got, want)
	}

	// every tool needs exactly one scope: the single-scope listings partition the set
	var union []string
	for scope, names := range allowedTools {
		_, tok := a.newPAT(t, userID, nil, scope)
		if got := a.toolNames(t, a.mcpClient(t, tok)); !slices.Equal(got, sorted(names)) {
			t.Fatalf("tools with only %s:\n got %v\nwant %v", scope, got, sorted(names))
		}
		union = append(union, names...)
	}
	if !slices.Equal(sorted(union), want) {
		t.Fatalf("scope groups do not partition the tool set: %v", sorted(union))
	}
}

func TestMCPToolListFollowsModuleToggles(t *testing.T) {
	a := newTestApp(t)
	userID, jwt := a.register(t, "toggle-tools@example.com")
	_, raw := a.newPAT(t, userID, nil, pat.ScopeRead, pat.ScopeDrafts, pat.ScopeLinks)
	cs := a.mcpClient(t, raw)

	setModules := func(modules string) {
		t.Helper()
		status, body := a.request(t, "PUT", "/api/v1/settings", jwt, `{"renewalDays":90,"reminderFrequency":"disabled","enabledModules":`+modules+`}`)
		if status != 200 {
			t.Fatalf("settings = %d %s", status, body)
		}
	}
	all := sorted(allowedTools[pat.ScopeRead], allowedTools[pat.ScopeDrafts], allowedTools[pat.ScopeLinks])
	without := func(prefixes ...string) []string {
		var out []string
		for _, n := range all {
			skip := false
			for _, p := range prefixes {
				skip = skip || strings.Contains(n, p)
			}
			if !skip {
				out = append(out, n)
			}
		}
		return out
	}

	setModules(`["purchases","auto","ledger"]`)
	if got := a.toolNames(t, cs); !slices.Equal(got, without("contract")) {
		t.Fatalf("contracts disabled: %v", got)
	}
	res, err := cs.CallTool(t.Context(), &mcp.CallToolParams{Name: "search_contracts", Arguments: map[string]any{}})
	if err != nil || !res.IsError || !strings.HasPrefix(res.Content[0].(*mcp.TextContent).Text, "module_disabled") {
		t.Fatalf("call to a disabled module's tool: %v %+v", err, res)
	}

	setModules(`["contracts"]`)
	if got := a.toolNames(t, cs); !slices.Equal(got, without("purchase", "vehicle", "ledger", "link_transaction")) {
		t.Fatalf("only contracts enabled: %v", got)
	}

	setModules(`["contracts","purchases","auto","ledger"]`)
	if got := a.toolNames(t, cs); !slices.Equal(got, all) {
		t.Fatalf("all enabled again: %v", got)
	}
}

func TestMCPBackfillCreatesDraftsOnly(t *testing.T) {
	a := newTestApp(t)
	userID, jwt := a.register(t, "backfill@example.com")
	_, raw := a.newPAT(t, userID, nil, pat.ScopeRead, pat.ScopeDrafts)
	cs := a.mcpClient(t, raw)

	structured := func(name string, args any, out any) {
		t.Helper()
		res, err := cs.CallTool(t.Context(), &mcp.CallToolParams{Name: name, Arguments: args})
		if err != nil || res.IsError {
			t.Fatalf("%s: %v %+v", name, err, res)
		}
		b, _ := json.Marshal(res.StructuredContent)
		if err := json.Unmarshal(b, out); err != nil {
			t.Fatal(err)
		}
	}
	var contractCats, purchaseCats struct {
		Items []struct {
			ID string `json:"id"`
		} `json:"items"`
	}
	structured("list_contract_categories", map[string]any{}, &contractCats)
	structured("list_purchase_categories", map[string]any{}, &purchaseCats)
	if len(contractCats.Items) == 0 || len(purchaseCats.Items) == 0 {
		t.Fatal("registration should have seeded categories")
	}

	// a vehicle must exist for the user before a cost draft can name it
	status, body := a.request(t, "POST", "/api/v1/vehicles", jwt, `{"name":"Golf"}`)
	if status != http.StatusCreated {
		t.Fatalf("vehicle = %d %s", status, body)
	}
	var vehicle struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal([]byte(body), &vehicle)

	var created struct {
		DraftID string `json:"draftId"`
	}
	drafted := map[string]string{} // targetType → draft ID
	create := func(target, tool string, args map[string]any) {
		t.Helper()
		args["title"] = tool
		args["key"] = tool
		structured(tool, args, &created)
		drafted[target] = created.DraftID
	}
	create("contract", "create_contract_draft", map[string]any{"categoryId": contractCats.Items[0].ID, "input": map[string]any{
		"name": "Home insurance", "billingInterval": "monthly", "startDate": "2026-03-01", "minimumDurationMonths": 12, "extensionDurationMonths": 12, "noticePeriodMonths": 3}})
	create("contract_category", "create_contract_category_draft", map[string]any{"input": map[string]any{"name": "Energy"}})
	create("purchase", "create_purchase_draft", map[string]any{"categoryId": purchaseCats.Items[0].ID, "input": map[string]any{"itemName": "Drill"}})
	create("purchase_category", "create_purchase_category_draft", map[string]any{"input": map[string]any{"name": "Garden"}})
	create("cost", "create_vehicle_cost_draft", map[string]any{"vehicleId": vehicle.ID, "input": map[string]any{"type": "service", "date": "2026-09-15"}})
	create("vehicle", "create_vehicle_draft", map[string]any{"input": map[string]any{"name": "Polo"}})
	create("ledger_category", "create_ledger_category_draft", map[string]any{"input": map[string]any{"name": "Energy costs", "matchWords": []string{"stadtwerke"}}})
	create("ledger_account", "create_ledger_account_draft", map[string]any{"input": map[string]any{"name": "Main", "bank": "DKB", "iban": "DE89370400440532013000"}})

	status, body = a.request(t, "GET", "/api/v1/drafts", jwt, "")
	if status != 200 {
		t.Fatalf("drafts = %d %s", status, body)
	}
	var list []struct {
		ID         string `json:"id"`
		TargetType string `json:"targetType"`
		Status     string `json:"status"`
		Source     struct {
			Kind string `json:"kind"`
		} `json:"source"`
	}
	if err := json.Unmarshal([]byte(body), &list); err != nil {
		t.Fatal(err)
	}
	if len(list) != len(drafted) {
		t.Fatalf("%d drafts visible, created %d: %s", len(list), len(drafted), body)
	}
	for _, d := range list {
		if drafted[d.TargetType] != d.ID || d.Status != "pending" || d.Source.Kind != "mcp" {
			t.Errorf("unexpected draft %+v", d)
		}
	}
	if status, body := a.request(t, "GET", "/api/v1/drafts/summary", jwt, ""); status != 200 || !strings.Contains(body, `"pending":8`) {
		t.Fatalf("summary = %d %s", status, body)
	}

	// nothing but the user's own vehicle exists: the drafts created no entity
	for path, want := range map[string]string{
		"/api/v1/contracts":                         "[]",
		"/api/v1/purchases":                         "[]",
		"/api/v1/vehicles/" + vehicle.ID + "/costs": "[]",
	} {
		if status, body := a.request(t, "GET", path, jwt, ""); status != 200 || strings.TrimSpace(body) != want {
			t.Errorf("%s = %d %s, want %s", path, status, body, want)
		}
	}
	if _, body := a.request(t, "GET", "/api/v1/vehicles", jwt, ""); strings.Contains(body, "Polo") {
		t.Error("the vehicle draft created a vehicle")
	}
	if status, body := a.request(t, "GET", "/api/v1/ledger/accounts", jwt, ""); status != 200 || strings.TrimSpace(body) != "[]" {
		t.Errorf("accounts = %d %s", status, body)
	}
	if _, body := a.request(t, "GET", "/api/v1/ledger/categories", jwt, ""); strings.Contains(body, "Energy costs") {
		t.Error("the ledger category draft created a category")
	}
	for _, module := range []string{"contracts", "purchases"} {
		_, body := a.request(t, "GET", "/api/v1/modules/"+module+"/categories", jwt, "")
		if strings.Contains(body, "Energy") || strings.Contains(body, "Garden") {
			t.Errorf("a category draft created a %s category", module)
		}
	}
}

func TestMCPLinkTransactionToRealEntities(t *testing.T) {
	a := newTestApp(t)
	userID, jwt := a.register(t, "link-real@example.com")
	otherID, _ := a.register(t, "link-other@example.com")
	ids := a.entities(t, userID, jwt)
	_, raw := a.newPAT(t, userID, nil, pat.ScopeLinks, pat.ScopeRead)
	cs := a.mcpClient(t, raw)

	link := func(client *mcp.ClientSession, txn, targetType, target string) (*mcp.CallToolResult, string) {
		t.Helper()
		res, err := client.CallTool(t.Context(), &mcp.CallToolParams{Name: "link_transaction", Arguments: map[string]any{
			"transactionId": txn, "targetType": targetType, "targetId": target}})
		if err != nil {
			t.Fatal(err)
		}
		var text string
		for _, c := range res.Content {
			text += c.(*mcp.TextContent).Text
		}
		return res, text
	}

	for _, target := range []string{"contract", "purchase", "vehicle"} {
		if res, text := link(cs, ids["transaction"], target, ids[target]); res.IsError {
			t.Fatalf("link to %s: %s", target, text)
		}
	}
	status, body := a.request(t, "GET", "/api/v1/ledger/transactions/"+ids["transaction"], jwt, "")
	if status != 200 || strings.Count(body, `"targetId"`) != 3 {
		t.Fatalf("transaction = %d %s", status, body)
	}
	for path, wantID := range map[string]string{"/api/v1/contracts/" + ids["contract"]: ids["transaction"], "/api/v1/purchases/" + ids["purchase"]: ids["transaction"], "/api/v1/vehicles/" + ids["vehicle"]: ids["transaction"]} {
		if status, body := a.request(t, "GET", path, jwt, ""); status != 200 || !strings.Contains(body, wantID) {
			t.Errorf("%s lacks the back-link: %d %s", path, status, body)
		}
	}
	if res, text := link(cs, ids["transaction"], "purchase", ids["purchase"]); res.IsError || !strings.Contains(text, `"alreadyLinked":true`) {
		t.Fatalf("repeat: %v %s", res.IsError, text)
	}

	// another user's token cannot touch this user's transaction
	_, otherRaw := a.newPAT(t, otherID, nil, pat.ScopeLinks)
	if res, text := link(a.mcpClient(t, otherRaw), ids["transaction"], "purchase", ids["purchase"]); !res.IsError || !strings.HasPrefix(text, "not_found") {
		t.Fatalf("foreign token: %q", text)
	}

	// a disabled target module blocks the link without changing anything
	status, body = a.request(t, "PUT", "/api/v1/settings", jwt, `{"renewalDays":90,"reminderFrequency":"disabled","enabledModules":["purchases","contracts","ledger"]}`)
	if status != 200 {
		t.Fatalf("settings = %d %s", status, body)
	}
	if res, text := link(cs, ids["transaction"], "vehicle", ids["vehicle"]); !res.IsError || !strings.HasPrefix(text, "module_disabled") {
		t.Fatalf("disabled target module: %q", text)
	}
}

func (a *testApp) mcpRaw(t *testing.T, token, contentType, body string) (int, string) {
	t.Helper()
	req, err := http.NewRequest("POST", a.srv.URL+"/mcp", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var buf bytes.Buffer
	buf.ReadFrom(resp.Body)
	return resp.StatusCode, buf.String()
}

func TestMCPHostileBodies(t *testing.T) {
	a := newTestApp(t)
	userID, _ := a.register(t, "hostile@example.com")
	_, raw := a.newPAT(t, userID, nil, pat.ScopeRead, pat.ScopeDrafts)

	tests := map[string]struct {
		contentType, body string
	}{
		"not json":            {"application/json", "not json"},
		"empty body":          {"application/json", ""},
		"wrong content type":  {"text/plain", `{"jsonrpc":"2.0","id":1,"method":"ping"}`},
		"unknown method":      {"application/json", `{"jsonrpc":"2.0","id":1,"method":"does/not/exist"}`},
		"wrong jsonrpc":       {"application/json", `{"jsonrpc":"1.0","id":1,"method":"ping"}`},
		"call without params": {"application/json", `{"jsonrpc":"2.0","id":1,"method":"tools/call"}`},
		"null params":         {"application/json", `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":null}`},
		"tool name is number": {"application/json", `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":5}}`},
		"oversized body":      {"application/json", `{"jsonrpc":"2.0","id":1,"method":"ping","x":"` + strings.Repeat("a", 5<<20) + `"}`},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			status, body := a.mcpRaw(t, raw, tt.contentType, tt.body)
			if status >= 500 {
				t.Fatalf("status = %d, body = %s", status, body)
			}
			if strings.Contains(body, raw) || strings.Contains(body, "goroutine") || strings.Contains(body, ".go:") {
				t.Fatalf("response leaks internals: %s", body)
			}
		})
	}

	// The token still works after the hostile requests.
	cs := a.mcpClient(t, raw)
	listModules(t, cs)
}

func TestMCPHostileToolArguments(t *testing.T) {
	a := newTestApp(t)
	userID, _ := a.register(t, "args@example.com")
	_, raw := a.newPAT(t, userID, nil, pat.ScopeRead, pat.ScopeDrafts, pat.ScopeLinks)
	cs := a.mcpClient(t, raw)

	calls := []struct {
		name string
		args map[string]any
	}{
		{"create_contract_draft", map[string]any{}},
		{"create_contract_draft", map[string]any{"title": strings.Repeat("x", 100000), "input": map[string]any{}}},
		{"create_contract_draft", map[string]any{"title": "t", "input": "not an object"}},
		{"create_contract_draft", map[string]any{"title": "t", "key": strings.Repeat("k", 100000), "input": map[string]any{}}},
		{"attach_paperless_documents", map[string]any{"entityType": "../../etc", "entityId": "x", "documentIds": []int{1}}},
		{"attach_paperless_documents", map[string]any{"entityType": "contract", "entityId": "not-a-uuid", "documentIds": []int{}}},
		{"attach_paperless_documents", map[string]any{"entityType": "contract", "entityId": "x", "documentIds": []int{-1}}},
		{"link_transaction", map[string]any{"transactionId": "' or 1=1", "targetType": "contract", "targetId": "x"}},
		{"link_transaction", map[string]any{}},
		{"search_paperless_documents", map[string]any{"query": strings.Repeat("q", 100000)}},
	}
	for _, c := range calls {
		res, err := cs.CallTool(t.Context(), &mcp.CallToolParams{Name: c.name, Arguments: c.args})
		if err != nil {
			continue // protocol-level rejection of the arguments is fine
		}
		if !res.IsError {
			t.Errorf("%s(%v) succeeded", c.name, c.args)
			continue
		}
		b, _ := json.Marshal(res.Content)
		if strings.Contains(string(b), "goroutine") || strings.Contains(string(b), ".go:") {
			t.Errorf("%s leaks internals: %s", c.name, b)
		}
	}
	listModules(t, cs)
}

func TestPATCreateRESTHostileInput(t *testing.T) {
	a := newTestApp(t)
	_, jwt := a.register(t, "hostilepat@example.com")
	for name, body := range map[string]string{
		"not json":          `nope`,
		"scopes null":       `{"name":"a","scopes":null}`,
		"scopes wrong type": `{"name":"a","scopes":"read"}`,
		"write scope":       `{"name":"a","scopes":["write"]}`,
		"admin scope":       `{"name":"a","scopes":["admin"]}`,
		"expiry in past":    `{"name":"a","scopes":["read"],"expiresAt":"2000-01-01T00:00:00Z"}`,
		"expiry not date":   `{"name":"a","scopes":["read"],"expiresAt":"tomorrow"}`,
		"huge name":         `{"name":"` + strings.Repeat("a", 1<<20) + `","scopes":["read"]}`,
	} {
		t.Run(name, func(t *testing.T) {
			status, resp := a.request(t, "POST", "/api/v1/tokens", jwt, body)
			if status < 400 || status >= 500 {
				t.Fatalf("status = %d, body = %s", status, resp)
			}
		})
	}
}

func TestMCPConcurrentSameKeyCreatesOneDraft(t *testing.T) {
	a := newTestApp(t)
	userID, jwt := a.register(t, "concurrent@example.com")
	_, raw := a.newPAT(t, userID, nil, pat.ScopeRead, pat.ScopeDrafts)

	const n = 12
	var wg sync.WaitGroup
	ids := make([]string, n)
	errs := make([]string, n)
	for i := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			cs := a.mcpClient(t, raw)
			res, err := cs.CallTool(t.Context(), &mcp.CallToolParams{
				Name: "create_vehicle_draft",
				Arguments: map[string]any{
					"title": "Polo", "key": "same-key",
					"input": map[string]any{"name": "Polo"},
				},
			})
			if err != nil {
				t.Errorf("call %d: %v", i, err)
				return
			}
			if res.IsError {
				b, _ := json.Marshal(res.Content)
				errs[i] = string(b)
				return
			}
			b, _ := json.Marshal(res.StructuredContent)
			var out struct {
				DraftID string `json:"draftId"`
			}
			_ = json.Unmarshal(b, &out)
			ids[i] = out.DraftID
		}()
	}
	wg.Wait()
	created := 0
	for i := range n {
		switch {
		case ids[i] != "":
			created++
		case !strings.Contains(errs[i], "already_pending"):
			t.Errorf("call %d: unexpected result %q", i, errs[i])
		}
	}
	if created != 1 {
		t.Fatalf("%d calls created a draft, want exactly 1 (ids=%v errs=%v)", created, ids, errs)
	}
	status, body := a.request(t, "GET", "/api/v1/drafts", jwt, "")
	var list []json.RawMessage
	if status != 200 || json.Unmarshal([]byte(body), &list) != nil || len(list) != 1 {
		t.Fatalf("drafts = %d %s", status, body)
	}
}

func TestMCPInvalidDraftInputIsRejectedBeforeCreation(t *testing.T) {
	a := newTestApp(t)
	userID, jwt := a.register(t, "invalid@example.com")
	_, raw := a.newPAT(t, userID, nil, pat.ScopeRead, pat.ScopeDrafts)
	cs := a.mcpClient(t, raw)

	for name, c := range map[string]struct {
		tool string
		args map[string]any
	}{
		"bad IBAN checksum": {"create_ledger_account_draft", map[string]any{"title": "a", "input": map[string]any{"name": "Main", "bank": "DKB", "iban": "DE00370400440532013000"}}},
		"empty vehicle":     {"create_vehicle_draft", map[string]any{"title": "a", "input": map[string]any{}}},
		"blank category":    {"create_contract_category_draft", map[string]any{"title": "a", "input": map[string]any{"name": "   "}}},
		"unknown category":  {"create_contract_draft", map[string]any{"title": "a", "categoryId": uuid.NewString(), "input": map[string]any{"name": "x"}}},
		"foreign vehicle":   {"create_vehicle_cost_draft", map[string]any{"title": "a", "vehicleId": uuid.NewString(), "input": map[string]any{"type": "service", "date": "2026-09-15"}}},
	} {
		t.Run(name, func(t *testing.T) {
			res, err := cs.CallTool(t.Context(), &mcp.CallToolParams{Name: c.tool, Arguments: c.args})
			if err == nil && !res.IsError {
				t.Fatalf("accepted: %+v", res.StructuredContent)
			}
		})
	}
	status, body := a.request(t, "GET", "/api/v1/drafts", jwt, "")
	if status != 200 || strings.TrimSpace(body) != "[]" && !strings.Contains(body, "null") {
		t.Fatalf("invalid input created drafts: %d %s", status, body)
	}
}
