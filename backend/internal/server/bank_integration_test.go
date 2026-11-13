package server

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/reusing-code/kontor/backend/internal/config"
	"github.com/reusing-code/kontor/backend/internal/enablebanking"
	"github.com/reusing-code/kontor/backend/internal/enablebanking/enablebankingtest"
)

const bankPublicURL = "https://kontor.example"

func newBankTestApp(t *testing.T) (*testApp, *enablebankingtest.Server) {
	t.Helper()
	fake := enablebankingtest.New(t)
	fake.SetASPSPs([]enablebanking.ASPSP{{Name: "DKB", Country: "DE", MaximumConsentValidity: 90 * 86400}})
	a := newTestApp(t, func(c *config.Config) {
		c.EnableBankingAppID = fake.AppID
		c.EnableBankingKeyPath = fake.KeyFile(t)
		c.EnableBankingAPIURL = fake.URL
		c.PublicURL = bankPublicURL
	})
	return a, fake
}

// noFollow sends a request without following redirects.
func (a *testApp) noFollow(t *testing.T, method, path string, headers map[string]string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(method, a.srv.URL+path, nil)
	if err != nil {
		t.Fatal(err)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { resp.Body.Close() })
	return resp
}

func TestBankCapability(t *testing.T) {
	disabled := newTestApp(t)
	_, jwt := disabled.register(t, "cap@example.com")
	status, body := disabled.request(t, "GET", "/api/v1/capabilities", jwt, "")
	var caps map[string]bool
	if status != 200 || json.Unmarshal([]byte(body), &caps) != nil || len(caps) != 2 || caps["bankImport"] {
		t.Fatalf("disabled capabilities = %d %s", status, body)
	}
	if _, ok := caps["llm"]; !ok {
		t.Fatalf("capabilities lack llm: %s", body)
	}

	enabled, _ := newBankTestApp(t)
	_, jwt = enabled.register(t, "cap@example.com")
	status, body = enabled.request(t, "GET", "/api/v1/capabilities", jwt, "")
	if status != 200 || json.Unmarshal([]byte(body), &caps) != nil || !caps["bankImport"] {
		t.Fatalf("enabled capabilities = %d %s", status, body)
	}
}

func TestBankImportWithoutEncryptionKeyStaysDisabled(t *testing.T) {
	fake := enablebankingtest.New(t)
	a := newTestApp(t, func(c *config.Config) {
		c.EnableBankingAppID = fake.AppID
		c.EnableBankingKeyPath = fake.KeyFile(t)
		c.EnableBankingAPIURL = fake.URL
		c.EmailEncryptionKey = ""
	})
	_, jwt := a.register(t, "nokey@example.com")
	_, body := a.request(t, "GET", "/api/v1/capabilities", jwt, "")
	if !strings.Contains(body, `"bankImport":false`) {
		t.Fatalf("capabilities = %s", body)
	}
}

func TestBankImportBadKeyFailsStartup(t *testing.T) {
	fake := enablebankingtest.New(t)
	engine := newTestApp(t).engine
	cfg := config.Config{JWTSecret: "x", FXRatesURL: "off", EmailEncryptionKey: testEncryptionKey,
		EnableBankingAppID: fake.AppID, EnableBankingKeyPath: "/nonexistent/key.pem", EnableBankingAPIURL: fake.URL}
	if _, err := New(cfg, slog.New(slog.DiscardHandler), engine).buildApp(); err == nil {
		t.Fatal("expected buildApp to fail for an unreadable key")
	}
}

func TestBankRoutesDisabled(t *testing.T) {
	a := newTestApp(t)
	_, jwt := a.register(t, "off@example.com")
	for _, tt := range []struct{ method, path, body string }{
		{"GET", "/api/v1/ledger/bank/aspsps?country=DE", ""},
		{"GET", "/api/v1/ledger/bank/connections", ""},
		{"POST", "/api/v1/ledger/bank/connections/authorize", `{"aspspName":"DKB","aspspCountry":"DE"}`},
		{"DELETE", "/api/v1/ledger/bank/connections/00000000-0000-0000-0000-000000000001", ""},
		{"PUT", "/api/v1/ledger/bank/connections/00000000-0000-0000-0000-000000000001/accounts/u", `{"ledgerAccountId":"00000000-0000-0000-0000-000000000002"}`},
	} {
		status, body := a.request(t, tt.method, tt.path, jwt, tt.body)
		if status != http.StatusConflict || !strings.Contains(body, `"bank_import_disabled"`) {
			t.Errorf("%s %s = %d %s", tt.method, tt.path, status, body)
		}
	}
}

func TestBankRoutesModuleDisabledForUser(t *testing.T) {
	a, _ := newBankTestApp(t)
	_, jwt := a.register(t, "nomod@example.com")
	status, body := a.request(t, "PUT", "/api/v1/settings", jwt, `{"renewalDays":90,"reminderFrequency":"disabled","enabledModules":["purchases"]}`)
	if status != 200 {
		t.Fatalf("settings = %d %s", status, body)
	}
	for _, path := range []string{"/api/v1/ledger/bank/connections", "/api/v1/ledger/bank/aspsps?country=DE"} {
		if status, body := a.request(t, "GET", path, jwt, ""); status != http.StatusForbidden || !strings.Contains(body, "module_disabled") {
			t.Errorf("GET %s = %d %s", path, status, body)
		}
	}
}

func TestBankRoutesPATScopes(t *testing.T) {
	a, _ := newBankTestApp(t)
	userID, jwt := a.register(t, "pat@example.com")
	_, read := a.newPAT(t, userID, nil, "read")

	if status, body := a.request(t, "GET", "/api/v1/ledger/bank/connections", read, ""); status != 200 {
		t.Fatalf("read token GET connections = %d %s", status, body)
	}
	for _, tt := range []struct{ method, path, body string }{
		{"GET", "/api/v1/ledger/bank/aspsps?country=DE", ""},
		{"POST", "/api/v1/ledger/bank/connections/authorize", `{"aspspName":"DKB","aspspCountry":"DE"}`},
		{"DELETE", "/api/v1/ledger/bank/connections/00000000-0000-0000-0000-000000000001", ""},
		{"PUT", "/api/v1/ledger/bank/connections/00000000-0000-0000-0000-000000000001/accounts/u", `{"ledgerAccountId":"00000000-0000-0000-0000-000000000002"}`},
	} {
		status, body := a.request(t, tt.method, tt.path, read, tt.body)
		if status != http.StatusForbidden || !strings.Contains(body, "insufficient_scope") {
			t.Errorf("%s %s with PAT = %d %s", tt.method, tt.path, status, body)
		}
	}
	if status, body := a.request(t, "GET", "/api/v1/ledger/bank/aspsps?country=DE", jwt, ""); status != 200 {
		t.Fatalf("JWT GET aspsps = %d %s", status, body)
	}
}

func TestBankCallbackIsPublic(t *testing.T) {
	a, _ := newBankTestApp(t)
	userID, jwt := a.register(t, "cb@example.com")
	_, read := a.newPAT(t, userID, nil, "read")

	for p := range routeRules {
		if strings.Contains(p, "bank/callback") {
			t.Errorf("callback has a scope table entry: %s", p)
		}
	}
	for _, p := range a.patterns {
		if strings.Contains(p, "bank/callback") {
			t.Errorf("callback registered on the API mux: %s", p)
		}
	}

	const want = "/ledger/bank-connections/result?bank=failed&reason=invalid_state"
	for name, headers := range map[string]map[string]string{
		"none":    nil,
		"garbage": {"Authorization": "Bearer garbage"},
		"pat":     {"Authorization": "Bearer " + read},
		"jwt":     {"Authorization": "Bearer " + jwt},
	} {
		resp := a.noFollow(t, "GET", "/api/v1/ledger/bank/callback", headers)
		if resp.StatusCode != http.StatusSeeOther || resp.Header.Get("Location") != want {
			t.Errorf("%s: %d %q", name, resp.StatusCode, resp.Header.Get("Location"))
		}
		if resp.Header.Get("Cache-Control") != "no-store" || resp.Header.Get("Referrer-Policy") != "no-referrer" {
			t.Errorf("%s: headers = %v", name, resp.Header)
		}
	}
}

func TestBankConsentFlow(t *testing.T) {
	a, fake := newBankTestApp(t)
	_, jwt := a.register(t, "flow@example.com")

	status, body := a.request(t, "POST", "/api/v1/ledger/bank/connections/authorize", jwt, `{"aspspName":"DKB","aspspCountry":"DE"}`)
	if status != 200 || !strings.Contains(body, "https://bank.example/") {
		t.Fatalf("authorize = %d %s", status, body)
	}
	req := fake.AuthRequests()[0]
	state := req["state"].(string)
	if req["redirect_url"] != bankPublicURL+"/api/v1/ledger/bank/callback" {
		t.Fatalf("redirect_url = %v", req["redirect_url"])
	}

	fake.AllowCode("CODE", enablebanking.Session{
		ID:         "SESSION-ID-FLOW",
		ASPSP:      enablebanking.ASPSP{Name: "DKB", Country: "DE"},
		ValidUntil: time.Now().Add(80 * 24 * time.Hour),
		Accounts:   []enablebanking.SessionAccount{{UID: "uid-1", IBAN: "DE02120300000000202051", Currency: "EUR", Name: "Giro"}},
	})
	resp := a.noFollow(t, "GET", "/api/v1/ledger/bank/callback?code=CODE&state="+url.QueryEscape(state), nil)
	if resp.StatusCode != http.StatusSeeOther || resp.Header.Get("Location") != "/ledger/bank-connections/result?bank=connected" {
		t.Fatalf("callback = %d %q", resp.StatusCode, resp.Header.Get("Location"))
	}

	status, body = a.request(t, "GET", "/api/v1/ledger/bank/connections", jwt, "")
	if status != 200 || !strings.Contains(body, `"aspspName":"DKB"`) || strings.Contains(body, "SESSION-ID-FLOW") || strings.Contains(strings.ToLower(body), "session") {
		t.Fatalf("connections = %d %s", status, body)
	}
	var views []struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal([]byte(body), &views); err != nil || len(views) != 1 {
		t.Fatalf("decoding views: %v", err)
	}

	status, body = a.request(t, "DELETE", "/api/v1/ledger/bank/connections/"+views[0].ID, jwt, "")
	if status != 200 || !strings.Contains(body, `"remoteClosed":true`) {
		t.Fatalf("disconnect = %d %s", status, body)
	}
}
