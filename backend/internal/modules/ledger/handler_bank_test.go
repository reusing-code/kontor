package ledger

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/reusing-code/kontor/backend/internal/enablebanking"
	"github.com/reusing-code/kontor/backend/internal/enablebanking/enablebankingtest"
	"github.com/reusing-code/kontor/backend/internal/middleware"
)

func newBankMux(h *Handler) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/ledger/bank/aspsps", h.ListBankASPSPs)
	mux.HandleFunc("GET /api/v1/ledger/bank/connections", h.ListBankConnections)
	mux.HandleFunc("POST /api/v1/ledger/bank/connections/authorize", h.AuthorizeBank)
	mux.HandleFunc("DELETE /api/v1/ledger/bank/connections/{id}", h.DisconnectBank)
	mux.HandleFunc("PUT /api/v1/ledger/bank/connections/{id}/accounts/{uid}", h.MapBankAccount)
	mux.HandleFunc("POST /api/v1/ledger/bank/connections/{id}/sync", h.SyncBankConnection)
	mux.HandleFunc("GET /api/v1/ledger/bank/callback", h.BankCallback)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mux.ServeHTTP(w, r.WithContext(middleware.SetUserID(r.Context(), testUserID)))
	})
}

func bankRequest(t *testing.T, h http.Handler, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func assertBankError(t *testing.T, rec *httptest.ResponseRecorder, status int, code string) {
	t.Helper()
	if rec.Code != status {
		t.Fatalf("status = %d, want %d (%s)", rec.Code, status, rec.Body.String())
	}
	got := decodeJSON[map[string]string](t, rec)
	if got["code"] != code || got["error"] == "" {
		t.Fatalf("body = %v, want code %q", got, code)
	}
	if strings.Contains(rec.Body.String(), enablebankingtest.FailureMessage) {
		t.Fatalf("upstream text leaked: %s", rec.Body.String())
	}
}

func newBankHandler(t *testing.T) (*Handler, *bankFixture) {
	t.Helper()
	f := newBankFixture(t)
	h, _ := newTestHandler(t)
	h.store = f.Store
	h.bank = f.svc
	return h, f
}

func TestBankHandlers_Disabled(t *testing.T) {
	h, _ := newTestHandler(t)
	mux := newBankMux(h)
	for _, tt := range []struct{ method, path, body string }{
		{"GET", "/api/v1/ledger/bank/aspsps?country=DE", ""},
		{"GET", "/api/v1/ledger/bank/connections", ""},
		{"POST", "/api/v1/ledger/bank/connections/authorize", `{"aspspName":"DKB","aspspCountry":"DE"}`},
		{"DELETE", "/api/v1/ledger/bank/connections/" + uuid.NewString(), ""},
		{"POST", "/api/v1/ledger/bank/connections/" + uuid.NewString() + "/sync", ""},
		{"PUT", "/api/v1/ledger/bank/connections/" + uuid.NewString() + "/accounts/uid", `{"ledgerAccountId":"` + uuid.NewString() + `"}`},
	} {
		assertBankError(t, bankRequest(t, mux, tt.method, tt.path, tt.body), http.StatusConflict, "bank_import_disabled")
	}

	// the disabled check comes before the body is read
	assertBankError(t, bankRequest(t, mux, "POST", "/api/v1/ledger/bank/connections/authorize", "not json"), http.StatusConflict, "bank_import_disabled")
	assertBankError(t, bankRequest(t, mux, "PUT", "/api/v1/ledger/bank/connections/"+uuid.NewString()+"/accounts/uid", "not json"), http.StatusConflict, "bank_import_disabled")

	rec := bankRequest(t, mux, "GET", "/api/v1/ledger/bank/callback?code=c&state=s", "")
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/ledger/bank-connections/result?bank=failed&reason=disabled" {
		t.Fatalf("callback = %d %q", rec.Code, rec.Header().Get("Location"))
	}
}

func TestBankHandlers_Statuses(t *testing.T) {
	h, f := newBankHandler(t)
	mux := newBankMux(h)
	const authorize = "/api/v1/ledger/bank/connections/authorize"

	rec := bankRequest(t, mux, "GET", "/api/v1/ledger/bank/aspsps?country=DE", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("aspsps = %d", rec.Code)
	}
	if banks := decodeJSON[[]BankInfo](t, rec); len(banks) != 3 || !banks[0].Verified {
		t.Fatalf("banks = %+v", banks)
	}
	assertBankError(t, bankRequest(t, mux, "GET", "/api/v1/ledger/bank/aspsps?country=xx1", ""), http.StatusBadRequest, "invalid_country")
	assertBankError(t, bankRequest(t, mux, "GET", "/api/v1/ledger/bank/aspsps", ""), http.StatusBadRequest, "invalid_country")

	assertBankError(t, bankRequest(t, mux, "POST", authorize, `{"aspspName":"Nope","aspspCountry":"DE"}`), http.StatusBadRequest, "unknown_bank")
	assertBankError(t, bankRequest(t, mux, "POST", authorize, `{"aspspName":""}`), http.StatusBadRequest, "invalid_request")
	assertBankError(t, bankRequest(t, mux, "POST", authorize, `not json`), http.StatusBadRequest, "invalid_request")

	f.fake.FailNext("/auth", 403, "ACCESS_DENIED")
	assertBankError(t, bankRequest(t, mux, "POST", authorize, `{"aspspName":"DKB","aspspCountry":"DE"}`), http.StatusUnprocessableEntity, "bank_restricted")
	f.fake.FailNext("/auth", 500, "")
	assertBankError(t, bankRequest(t, mux, "POST", authorize, `{"aspspName":"DKB","aspspCountry":"DE"}`), http.StatusBadGateway, "bank_unavailable")
	f.fake.FailNext("/auth", 429, "")
	assertBankError(t, bankRequest(t, mux, "POST", authorize, `{"aspspName":"DKB","aspspCountry":"DE"}`), http.StatusTooManyRequests, "bank_rate_limited")

	rec = bankRequest(t, mux, "POST", authorize, `{"aspspName":"DKB","aspspCountry":"DE"}`)
	if rec.Code != http.StatusOK || !strings.HasPrefix(decodeJSON[map[string]string](t, rec)["url"], "https://bank.example/") {
		t.Fatalf("authorize = %d %s", rec.Code, rec.Body.String())
	}

	assertBankError(t, bankRequest(t, mux, "DELETE", "/api/v1/ledger/bank/connections/"+uuid.NewString(), ""), http.StatusNotFound, "not_found")
	assertBankError(t, bankRequest(t, mux, "DELETE", "/api/v1/ledger/bank/connections/not-a-uuid", ""), http.StatusNotFound, "not_found")

	conn := f.connect(t, "code-1", bankTestSession("SESSION-1", bankTestAccount("uid-1", bankTestIBAN1)))
	usd := f.addLedgerAccount(t, "DE75512108001245126199", "USD")
	base := "/api/v1/ledger/bank/connections/" + conn.ID.String() + "/accounts/uid-1"
	assertBankError(t, bankRequest(t, mux, "PUT", base, `{"ledgerAccountId":"`+usd.String()+`"}`), http.StatusBadRequest, "currency_mismatch")
	assertBankError(t, bankRequest(t, mux, "PUT", base, `{"ledgerAccountId":"`+uuid.NewString()+`"}`), http.StatusBadRequest, "invalid_mapping")
	assertBankError(t, bankRequest(t, mux, "PUT", base, `{}`), http.StatusBadRequest, "invalid_request")
	assertBankError(t, bankRequest(t, mux, "PUT", strings.Replace(base, "uid-1", "nope", 1), `{"ledgerAccountId":"`+uuid.NewString()+`"}`), http.StatusNotFound, "not_found")

	ok := f.addLedgerAccount(t, "", "EUR")
	rec = bankRequest(t, mux, "PUT", base, `{"ledgerAccountId":"`+ok.String()+`"}`)
	if rec.Code != http.StatusOK || !decodeJSON[BankConnectionView](t, rec).Accounts[0].Mapped {
		t.Fatalf("map = %d %s", rec.Code, rec.Body.String())
	}

	rec = bankRequest(t, mux, "GET", "/api/v1/ledger/bank/connections", "")
	if rec.Code != http.StatusOK || strings.Contains(strings.ToLower(rec.Body.String()), "session") {
		t.Fatalf("connections = %d %s", rec.Code, rec.Body.String())
	}

	f.fake.FailNext("/sessions/", 500, "")
	rec = bankRequest(t, mux, "DELETE", "/api/v1/ledger/bank/connections/"+conn.ID.String(), "")
	if rec.Code != http.StatusOK || decodeJSON[map[string]bool](t, rec)["remoteClosed"] {
		t.Fatalf("disconnect = %d %s", rec.Code, rec.Body.String())
	}
}

func TestBankHandlers_PublicURLMissing(t *testing.T) {
	h, f := newBankHandler(t)
	h.bank = NewBankService(BankServiceConfig{Client: f.svc.client, Store: f.Store, EncryptionKey: bankTestKey})
	assertBankError(t, bankRequest(t, newBankMux(h), "POST", "/api/v1/ledger/bank/connections/authorize", `{"aspspName":"DKB","aspspCountry":"DE"}`),
		http.StatusConflict, "public_url_not_configured")
}

func TestBankCallback_RedirectCarriesOnlyFixedCodes(t *testing.T) {
	h, f := newBankHandler(t)
	mux := newBankMux(h)
	const sessionID = "SESSION-ID-SECRET"
	f.fake.AllowCode("CODE-SECRET", bankTestSession(sessionID, bankTestAccount("uid-1", bankTestIBAN1)))
	state := f.authorize(t, "DKB")

	check := func(query, wantLocation string) {
		t.Helper()
		rec := bankRequest(t, mux, "GET", "/api/v1/ledger/bank/callback?"+query, "")
		loc := rec.Header().Get("Location")
		if rec.Code != http.StatusSeeOther || loc != wantLocation {
			t.Fatalf("%s: %d %q, want 303 %q", query, rec.Code, loc, wantLocation)
		}
		if rec.Header().Get("Cache-Control") != "no-store" || rec.Header().Get("Referrer-Policy") != "no-referrer" {
			t.Fatalf("headers = %v", rec.Header())
		}
		if u, err := url.Parse(loc); err != nil || u.IsAbs() || u.Host != "" {
			t.Fatalf("location must be relative: %q", loc)
		}
		for _, secret := range []string{"CODE-SECRET", state, sessionID, "DESCRIPTION-SECRET"} {
			if strings.Contains(loc, secret) || strings.Contains(rec.Body.String(), secret) {
				t.Fatalf("secret %q reflected: %q", secret, loc)
			}
		}
	}

	check("state=garbled&error_description=DESCRIPTION-SECRET", "/ledger/bank-connections/result?bank=failed&reason=invalid_state")
	check("code=CODE-SECRET&state="+url.QueryEscape(state)+"&error_description=DESCRIPTION-SECRET", "/ledger/bank-connections/result?bank=connected")
	check("code=CODE-SECRET&state="+url.QueryEscape(state), "/ledger/bank-connections/result?bank=failed&reason=invalid_state")
	check("", "/ledger/bank-connections/result?bank=failed&reason=invalid_state")

	state = f.authorize(t, "DKB")
	check("error=access_denied&error_description=DESCRIPTION-SECRET&state="+url.QueryEscape(state), "/ledger/bank-connections/result?bank=failed&reason=denied")
}

func TestSyncBankConnectionHandler(t *testing.T) {
	h, f := newBankHandler(t)
	mux := newBankMux(h)
	f.addLedgerAccount(t, bankTestIBAN1, "EUR")
	conn := f.connect(t, "code", bankTestSession("SESSION-SECRET", bankTestAccount("uid-1", bankTestIBAN1)))
	f.fake.SetTransactions("uid-1", []enablebanking.Transaction{apiTxn("a", "2026-04-01", "10.00", "DBIT"), apiTxn("b", "2026-04-02", "5.00", "CRDT")})
	path := "/api/v1/ledger/bank/connections/" + conn.ID.String() + "/sync"

	assertBankError(t, bankRequest(t, mux, "POST", "/api/v1/ledger/bank/connections/"+uuid.NewString()+"/sync", ""), http.StatusNotFound, "not_found")
	assertBankError(t, bankRequest(t, mux, "POST", "/api/v1/ledger/bank/connections/nope/sync", ""), http.StatusNotFound, "not_found")

	unlock, _ := f.svc.lockSync(testUserID, conn.ID)
	assertBankError(t, bankRequest(t, mux, "POST", path, ""), http.StatusConflict, "sync_running")
	unlock()

	rec := bankRequest(t, mux, "POST", path, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("sync = %d %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, secret := range []string{"SESSION-SECRET", bankTestIBAN1, "Shop a"} {
		if strings.Contains(body, secret) {
			t.Fatalf("response contains %q: %s", secret, body)
		}
	}
	got := decodeJSON[BankSyncSummary](t, rec)
	if got.Imported != 2 || len(got.Accounts) != 1 || got.Accounts[0].UID != "uid-1" || got.Accounts[0].Status != BankSyncOK {
		t.Fatalf("summary = %+v", got)
	}
	for _, key := range []string{`"skippedNotBooked"`, `"skippedCurrency"`, `"skippedInvalid"`, `"duplicates"`} {
		if !strings.Contains(body, key) {
			t.Fatalf("missing %s in %s", key, body)
		}
	}

	f.now = conn.ValidUntil.Add(time.Hour)
	assertBankError(t, bankRequest(t, mux, "POST", path, ""), http.StatusConflict, "consent_expired")
}

func TestBankError_SyncTimeoutIsFixedAndNotAnInternalError(t *testing.T) {
	h, _ := newTestHandler(t)
	rec := httptest.NewRecorder()
	h.bankError(rec, fmt.Errorf("sync: %w", context.DeadlineExceeded))
	assertBankError(t, rec, http.StatusGatewayTimeout, "sync_timeout")
}
