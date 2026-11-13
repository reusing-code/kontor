package enablebanking_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/reusing-code/kontor/backend/internal/enablebanking"
	"github.com/reusing-code/kontor/backend/internal/enablebanking/enablebankingtest"
)

func newClient(t *testing.T, fake *enablebankingtest.Server) *enablebanking.Client {
	t.Helper()
	key, err := enablebanking.LoadPrivateKey(fake.KeyFile(t))
	if err != nil {
		t.Fatal(err)
	}
	c, err := enablebanking.New(enablebanking.Config{BaseURL: fake.URL, AppID: fake.AppID, Key: key})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestNewValidates(t *testing.T) {
	if _, err := enablebanking.New(enablebanking.Config{}); err == nil {
		t.Fatal("expected error for empty config")
	}
}

func TestASPSPs(t *testing.T) {
	fake := enablebankingtest.New(t)
	fake.SetASPSPs([]enablebanking.ASPSP{
		{Name: "DKB", Country: "DE", MaximumConsentValidity: 7776000},
		{Name: "Other", Country: "FI"},
	})
	got, err := newClient(t, fake).ASPSPs(t.Context(), "DE")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Name != "DKB" || got[0].MaximumConsentValidity != 7776000 {
		t.Fatalf("got %+v", got)
	}
}

func TestStartAuth(t *testing.T) {
	fake := enablebankingtest.New(t)
	validUntil := time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)
	url, err := newClient(t, fake).StartAuth(t.Context(), enablebanking.AuthRequest{
		ASPSPName: "DKB", ASPSPCountry: "DE", State: "st", RedirectURL: "https://k.example/cb", ValidUntil: validUntil, ExtendedHistory: true,
	})
	if err != nil || !strings.HasPrefix(url, "https://bank.example/") {
		t.Fatalf("url = %q, err = %v", url, err)
	}
	reqs := fake.AuthRequests()
	if len(reqs) != 1 {
		t.Fatalf("auth requests = %d", len(reqs))
	}
	body := reqs[0]
	access := body["access"].(map[string]any)
	if access["valid_until"] != "2027-01-01T00:00:00Z" || access["transactions"] != true || access["transactions_extended_history"] != true {
		t.Fatalf("access = %v", access)
	}
	if aspsp := body["aspsp"].(map[string]any); aspsp["name"] != "DKB" || aspsp["country"] != "DE" {
		t.Fatalf("aspsp = %v", aspsp)
	}
	if body["state"] != "st" || body["redirect_url"] != "https://k.example/cb" || body["psu_type"] != "personal" {
		t.Fatalf("body = %v", body)
	}
}

func TestSessionLifecycle(t *testing.T) {
	fake := enablebankingtest.New(t)
	validUntil := time.Date(2027, 1, 1, 12, 0, 0, 0, time.UTC)
	fake.AllowCode("good", enablebanking.Session{
		ID:         "sess-1",
		ASPSP:      enablebanking.ASPSP{Name: "DKB", Country: "DE"},
		ValidUntil: validUntil,
		Accounts:   []enablebanking.SessionAccount{{UID: "uid-1", IBAN: "DE02120300000000202051", Currency: "EUR", Name: "Giro", IdentificationHash: "h1"}},
	})
	c := newClient(t, fake)

	s, err := c.CreateSession(t.Context(), "good")
	if err != nil {
		t.Fatal(err)
	}
	if s.ID != "sess-1" || !s.ValidUntil.Equal(validUntil) || s.ASPSP.Name != "DKB" || len(s.Accounts) != 1 {
		t.Fatalf("session = %+v", s)
	}
	if a := s.Accounts[0]; a.UID != "uid-1" || a.IBAN != "DE02120300000000202051" || a.Currency != "EUR" || a.Name != "Giro" || a.IdentificationHash != "h1" {
		t.Fatalf("account = %+v", a)
	}

	if _, err := c.CreateSession(t.Context(), "bad"); err == nil {
		t.Fatal("expected error for unknown code")
	} else if apiErr := (*enablebanking.APIError)(nil); !errors.As(err, &apiErr) || apiErr.Status != 400 || apiErr.Code != "WRONG_AUTHORIZATION_CODE" {
		t.Fatalf("err = %v", err)
	}

	fake.SetSessionStatus("sess-1", "REVOKED")
	st, err := c.SessionStatus(t.Context(), "sess-1")
	if err != nil || st.Status != "REVOKED" {
		t.Fatalf("status = %+v, err = %v", st, err)
	}

	if err := c.DeleteSession(t.Context(), "sess-1"); err != nil {
		t.Fatal(err)
	}
	if got := fake.Deleted(); len(got) != 1 || got[0] != "sess-1" {
		t.Fatalf("deleted = %v", got)
	}
}

func TestTransactionsFollowsContinuationKey(t *testing.T) {
	fake := enablebankingtest.New(t)
	var txns []enablebanking.Transaction
	for _, d := range []string{"2026-04-01", "2026-04-02", "2026-04-03", "2026-04-04", "2026-04-05"} {
		txns = append(txns, enablebanking.Transaction{EntryReference: "ref-" + d, BookingDate: d, Status: "BOOK", CreditDebitIndicator: "DBIT",
			TransactionAmount: enablebanking.Amount{Currency: "EUR", Amount: "1.00"}, RemittanceInformation: []string{"x"}})
	}
	fake.SetTransactions("uid-1", txns)
	fake.SetPageSize(2)
	c := newClient(t, fake)

	var got []enablebanking.Transaction
	key := ""
	for pages := 0; ; pages++ {
		page, err := c.Transactions(t.Context(), enablebanking.TransactionsQuery{AccountUID: "uid-1", DateFrom: "2026-04-02", DateTo: "2026-04-05", ContinuationKey: key})
		if err != nil {
			t.Fatal(err)
		}
		got = append(got, page.Transactions...)
		if key = page.ContinuationKey; key == "" {
			if pages != 1 {
				t.Fatalf("pages = %d, want 2", pages+1)
			}
			break
		}
	}
	if len(got) != 4 || got[0].BookingDate != "2026-04-02" || got[3].BookingDate != "2026-04-05" {
		t.Fatalf("got %d transactions: %+v", len(got), got)
	}
	if q := fake.TransactionQueries(); len(q) != 2 || q[0].From != "2026-04-02" || q[1].Key == "" {
		t.Fatalf("queries = %+v", q)
	}

	fake.SetPageSize(0)
	page, err := c.Transactions(t.Context(), enablebanking.TransactionsQuery{AccountUID: "uid-1", DateFrom: "2026-04-05", Strategy: "longest"})
	if err != nil || len(page.Transactions) != 5 {
		t.Fatalf("longest ignores dates: %d, %v", len(page.Transactions), err)
	}
	if _, err := c.Transactions(t.Context(), enablebanking.TransactionsQuery{}); err == nil {
		t.Fatal("expected error without account UID")
	}
}

func TestAPIErrors(t *testing.T) {
	tests := []struct {
		name   string
		status int
		code   string
		check  func(error) bool
		want   string
	}{
		{"rate limit by status", 429, "", enablebanking.IsRateLimited, "status 429"},
		{"rate limit by code", 400, "ASPSP_RATE_LIMIT_EXCEEDED", enablebanking.IsRateLimited, "ASPSP_RATE_LIMIT_EXCEEDED"},
		{"expired session", 401, "EXPIRED_SESSION", enablebanking.IsSessionUnusable, "EXPIRED_SESSION"},
		{"revoked session", 403, "REVOKED_SESSION", enablebanking.IsSessionUnusable, "REVOKED_SESSION"},
		{"account not accessible", 403, "ASPSP_ACCOUNT_NOT_ACCESSIBLE", enablebanking.IsAccountNotAccessible, ""},
		{"app auth", 401, "UNAUTHORIZED_ACCESS", enablebanking.IsAppAuth, ""},
		{"wrong period", 422, "WRONG_TRANSACTIONS_PERIOD", enablebanking.IsWrongTransactionsPeriod, ""},
		{"server error", 502, "", enablebanking.IsUnavailable, "status 502"},
		{"bank error", 400, "ASPSP_ERROR", enablebanking.IsUnavailable, ""},
		{"bank timeout", 408, "ASPSP_TIMEOUT", enablebanking.IsUnavailable, ""},
		{"lowercase code dropped", 400, "not a code!", func(err error) bool {
			var apiErr *enablebanking.APIError
			return errors.As(err, &apiErr) && apiErr.Code == ""
		}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake := enablebankingtest.New(t)
			fake.FailNext("/aspsps", tt.status, tt.code)
			_, err := newClient(t, fake).ASPSPs(t.Context(), "DE")
			if err == nil {
				t.Fatal("expected error")
			}
			if !tt.check(err) {
				t.Fatalf("classifier rejected %v", err)
			}
			if strings.Contains(err.Error(), enablebankingtest.FailureMessage) {
				t.Fatalf("upstream message leaked: %v", err)
			}
			if tt.want != "" && !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("err = %v, want containing %q", err, tt.want)
			}
			var apiErr *enablebanking.APIError
			if !errors.As(err, &apiErr) || apiErr.Status != tt.status {
				t.Fatalf("err = %v", err)
			}
		})
	}
}

func TestWrongJWTRejectedByFake(t *testing.T) {
	fake := enablebankingtest.New(t)
	req, _ := http.NewRequestWithContext(t.Context(), http.MethodGet, fake.URL+"/aspsps", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d", resp.StatusCode)
	}
}

func TestTransportErrorDoesNotContainSessionID(t *testing.T) {
	fake := enablebankingtest.New(t)
	c := newClient(t, fake)
	closed := httptest.NewServer(http.NotFoundHandler())
	closed.Close()
	key, err := enablebanking.LoadPrivateKey(fake.KeyFile(t))
	if err != nil {
		t.Fatal(err)
	}
	dead, err := enablebanking.New(enablebanking.Config{BaseURL: closed.URL, AppID: fake.AppID, Key: key})
	if err != nil {
		t.Fatal(err)
	}

	const sessionID = "SECRET-SESSION-ID"
	err = dead.DeleteSession(t.Context(), sessionID)
	if err == nil {
		t.Fatal("expected error")
	}
	if strings.Contains(err.Error(), sessionID) || strings.Contains(err.Error(), "/sessions") {
		t.Fatalf("transport error leaks the request URL: %v", err)
	}
	if !enablebanking.IsUnavailable(err) {
		t.Fatal("transport error should count as unavailable")
	}

	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := c.DeleteSession(ctx, sessionID); !errors.Is(err, context.Canceled) || strings.Contains(err.Error(), sessionID) {
		t.Fatalf("cancelled request: %v", err)
	}
}
