package ledger

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/dgraph-io/badger/v4"
	"github.com/google/uuid"
	"github.com/reusing-code/kontor/backend/internal/cryptoutil"
	"github.com/reusing-code/kontor/backend/internal/enablebanking"
	"github.com/reusing-code/kontor/backend/internal/enablebanking/enablebankingtest"
	"github.com/reusing-code/kontor/backend/internal/storage"
)

const (
	bankTestPublicURL = "https://kontor.example"
	bankTestIBAN1     = "DE02120300000000202051"
	bankTestIBAN2     = "DE89370400440532013000"
)

type bankFixture struct {
	*testStores
	fake *enablebankingtest.Server
	svc  *BankService
	now  time.Time
}

type moduleSwitch struct{ on bool }

func (m *moduleSwitch) ModuleEnabled(context.Context, string, string) (bool, error) { return m.on, nil }

func newBankFixture(t *testing.T) *bankFixture {
	t.Helper()
	f := &bankFixture{
		testStores: newTestStore(t),
		fake:       enablebankingtest.New(t),
		now:        time.Date(2026, 11, 13, 10, 0, 0, 0, time.UTC),
	}
	f.fake.SetASPSPs([]enablebanking.ASPSP{
		{Name: "DKB", Country: "DE", MaximumConsentValidity: 90 * 86400},
		{Name: "comdirect", Country: "DE"},
		{Name: "Longbank", Country: "DE", MaximumConsentValidity: 365 * 86400},
	})
	key, err := enablebanking.LoadPrivateKey(f.fake.KeyFile(t))
	if err != nil {
		t.Fatal(err)
	}
	client, err := enablebanking.New(enablebanking.Config{BaseURL: f.fake.URL, AppID: f.fake.AppID, Key: key})
	if err != nil {
		t.Fatal(err)
	}
	f.svc = NewBankService(BankServiceConfig{
		Client: client, Store: f.Store, EncryptionKey: bankTestKey, PublicURL: bankTestPublicURL,
		Logger: slog.New(slog.DiscardHandler), Now: func() time.Time { return f.now },
	})
	return f
}

func bankTestSession(id string, accounts ...enablebanking.SessionAccount) enablebanking.Session {
	return enablebanking.Session{
		ID:         id,
		ASPSP:      enablebanking.ASPSP{Name: "DKB", Country: "DE"},
		ValidUntil: time.Date(2027, 2, 10, 9, 55, 0, 0, time.UTC),
		Accounts:   accounts,
	}
}

func bankTestAccount(uid, iban string) enablebanking.SessionAccount {
	return enablebanking.SessionAccount{UID: uid, IBAN: iban, Currency: "EUR", Name: "Account " + uid, IdentificationHash: "hash-" + uid}
}

// authorize starts a consent and returns the state the bank would send back.
func (f *bankFixture) authorize(t *testing.T, bank string) string {
	t.Helper()
	if _, err := f.svc.StartAuthorization(t.Context(), testUserID, BankAuthorizeInput{ASPSPName: bank, ASPSPCountry: "DE"}); err != nil {
		t.Fatalf("StartAuthorization: %v", err)
	}
	reqs := f.fake.AuthRequests()
	return reqs[len(reqs)-1]["state"].(string)
}

// connect runs a full consent for DKB with the given session.
func (f *bankFixture) connect(t *testing.T, code string, session enablebanking.Session) LedgerBankConnection {
	t.Helper()
	f.fake.AllowCode(code, session)
	state := f.authorize(t, "DKB")
	if res := f.svc.CompleteAuthorization(t.Context(), code, state, ""); !res.Connected {
		t.Fatalf("callback failed: %+v", res)
	}
	conn, err := f.FindLedgerBankConnectionByASPSP(t.Context(), testUserID, "DKB", "DE")
	if err != nil {
		t.Fatal(err)
	}
	return conn
}

func (f *bankFixture) addLedgerAccount(t *testing.T, iban, currency string) uuid.UUID {
	t.Helper()
	id := uuid.New()
	if err := f.CreateLedgerAccount(t.Context(), testUserID, LedgerAccount{
		ID: id, Name: "Ledger " + iban, Bank: "DKB", IBAN: iban, Currency: currency, CreatedAt: f.now, UpdatedAt: f.now,
	}); err != nil {
		t.Fatal(err)
	}
	return id
}

func (f *bankFixture) decryptSession(t *testing.T, c LedgerBankConnection) string {
	t.Helper()
	id, err := cryptoutil.DecryptString(c.EncryptedSessionID, bankTestKey)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func TestStartAuthorization(t *testing.T) {
	tests := []struct {
		bank      string
		wantValid time.Duration
	}{
		{"DKB", 90*24*time.Hour - 5*time.Minute},
		{"comdirect", 90*24*time.Hour - 5*time.Minute},
		{"Longbank", 180*24*time.Hour - 5*time.Minute},
	}
	for _, tt := range tests {
		t.Run(tt.bank, func(t *testing.T) {
			f := newBankFixture(t)
			url, err := f.svc.StartAuthorization(t.Context(), testUserID, BankAuthorizeInput{ASPSPName: strings.ToLower(tt.bank), ASPSPCountry: "DE"})
			if err != nil || !strings.HasPrefix(url, "https://bank.example/") {
				t.Fatalf("url = %q, err = %v", url, err)
			}
			req := f.fake.AuthRequests()[0]
			state := req["state"].(string)
			if raw, err := base64.RawURLEncoding.DecodeString(state); err != nil || len(raw) < 64 {
				t.Fatalf("state is not URL-safe or too short: %v (%d)", err, len(raw))
			}
			if got, err := openBankAuthState(state, bankTestKey); err != nil || got != testUserID {
				t.Fatalf("state does not open: %q, %v", got, err)
			}
			if strings.Contains(state, testUserID) {
				t.Fatal("state exposes the user ID")
			}
			if req["redirect_url"] != bankTestPublicURL+"/api/v1/ledger/bank/callback" || req["psu_type"] != "personal" {
				t.Fatalf("request = %v", req)
			}
			access := req["access"].(map[string]any)
			validUntil, err := time.Parse(time.RFC3339, access["valid_until"].(string))
			if err != nil || !validUntil.Equal(f.now.Add(tt.wantValid)) {
				t.Fatalf("valid_until = %v (%v), want %v", access["valid_until"], err, f.now.Add(tt.wantValid))
			}
			if access["transactions_extended_history"] != true || access["transactions"] != true {
				t.Fatalf("access = %v", access)
			}
			aspsp := req["aspsp"].(map[string]any)
			if aspsp["name"] != tt.bank || aspsp["country"] != "DE" {
				t.Fatalf("aspsp = %v", aspsp)
			}
		})
	}
}

func TestStartAuthorizationErrors(t *testing.T) {
	f := newBankFixture(t)
	ctx := t.Context()
	in := BankAuthorizeInput{ASPSPName: "DKB", ASPSPCountry: "DE"}

	if _, err := f.svc.StartAuthorization(ctx, testUserID, BankAuthorizeInput{ASPSPName: "Nope", ASPSPCountry: "DE"}); !errors.Is(err, ErrBankUnknownASPSP) {
		t.Fatalf("unknown bank: %v", err)
	}
	if _, err := f.svc.StartAuthorization(ctx, testUserID, BankAuthorizeInput{ASPSPName: "DKB", ASPSPCountry: "Germany"}); !errors.Is(err, ErrBankInvalidCountry) {
		t.Fatalf("invalid country: %v", err)
	}
	if len(f.fake.AuthRequests()) != 0 {
		t.Fatal("auth request sent for invalid input")
	}

	var nilService *BankService
	if _, err := nilService.StartAuthorization(ctx, testUserID, in); !errors.Is(err, ErrBankImportDisabled) {
		t.Fatalf("nil service: %v", err)
	}
	if _, err := NewBankService(BankServiceConfig{}).StartAuthorization(ctx, testUserID, in); !errors.Is(err, ErrBankImportDisabled) {
		t.Fatalf("service without client: %v", err)
	}
	if res := nilService.CompleteAuthorization(ctx, "c", "s", ""); res.Connected || res.Reason != BankReasonDisabled {
		t.Fatalf("nil service callback: %+v", res)
	}

	noURL := NewBankService(BankServiceConfig{Client: f.svc.client, Store: f.Store, EncryptionKey: bankTestKey})
	if _, err := noURL.StartAuthorization(ctx, testUserID, in); !errors.Is(err, ErrBankPublicURLMissing) {
		t.Fatalf("no PUBLIC_URL: %v", err)
	}

	for _, tt := range []struct {
		status int
		code   string
		want   error
	}{
		{500, "", ErrBankUnavailable},
		{429, "", ErrBankRateLimited},
		{403, "ACCESS_DENIED", ErrBankRestricted},
		{422, "NO_ACCOUNTS_ADDED", ErrBankRestricted},
	} {
		f.fake.FailNext("/auth", tt.status, tt.code)
		_, err := f.svc.StartAuthorization(ctx, testUserID, in)
		if !errors.Is(err, tt.want) {
			t.Fatalf("status %d %s: %v, want %v", tt.status, tt.code, err, tt.want)
		}
		if strings.Contains(err.Error(), enablebankingtest.FailureMessage) {
			t.Fatalf("upstream text leaked: %v", err)
		}
	}
	// failed starts leave no pending state behind
	if n, err := f.PruneLedgerBankAuthStates(ctx, testUserID, f.now.Add(24*time.Hour)); err != nil || n != 0 {
		t.Fatalf("pending states after failures: %d, %v", n, err)
	}
}

func TestListASPSPsCachesPerCountry(t *testing.T) {
	f := newBankFixture(t)
	ctx := t.Context()
	banks, err := f.svc.ListASPSPs(ctx, "DE")
	if err != nil || len(banks) != 3 {
		t.Fatalf("banks = %+v, %v", banks, err)
	}
	verified := map[string]bool{}
	for _, b := range banks {
		verified[b.Name] = b.Verified
	}
	if !verified["DKB"] || !verified["comdirect"] || verified["Longbank"] {
		t.Fatalf("verified = %v", verified)
	}
	if banks[0].MaxConsentDays != 90 {
		t.Fatalf("maxConsentDays = %d", banks[0].MaxConsentDays)
	}

	f.fake.FailAlways("/aspsps", 500, "")
	if _, err := f.svc.ListASPSPs(ctx, "DE"); err != nil {
		t.Fatalf("cached call failed: %v", err)
	}
	f.now = f.now.Add(11 * time.Minute)
	if _, err := f.svc.ListASPSPs(ctx, "DE"); !errors.Is(err, ErrBankUnavailable) {
		t.Fatalf("expired cache: %v", err)
	}
}

func TestCallbackSuccess(t *testing.T) {
	f := newBankFixture(t)
	mappedID := f.addLedgerAccount(t, bankTestIBAN1, "EUR")
	mismatchID := f.addLedgerAccount(t, "DE75512108001245126199", "USD")

	session := bankTestSession("SESSION-ID-1",
		bankTestAccount("uid-1", bankTestIBAN1),
		bankTestAccount("uid-2", bankTestIBAN2),
		bankTestAccount("uid-3", "DE75512108001245126199"),
	)
	conn := f.connect(t, "code-1", session)

	if got := f.decryptSession(t, conn); got != "SESSION-ID-1" || strings.Contains(conn.EncryptedSessionID, "SESSION-ID-1") {
		t.Fatalf("session not encrypted: %q", conn.EncryptedSessionID)
	}
	if !conn.ValidUntil.Equal(session.ValidUntil) || !conn.AuthorizedAt.Equal(f.now) || conn.LostAt != nil || len(conn.NotifiedDays) != 0 {
		t.Fatalf("conn = %+v", conn)
	}
	if len(conn.Accounts) != 3 {
		t.Fatalf("accounts = %+v", conn.Accounts)
	}
	if a := conn.Accounts[0]; a.LedgerAccountID == nil || *a.LedgerAccountID != mappedID {
		t.Fatalf("uid-1 not mapped by IBAN: %+v", a)
	}
	if conn.Accounts[1].LedgerAccountID != nil {
		t.Fatalf("uid-2 mapped without a ledger account: %+v", conn.Accounts[1])
	}
	if conn.Accounts[2].LedgerAccountID != nil || mismatchID == uuid.Nil {
		t.Fatalf("currency mismatch must stay unmapped: %+v", conn.Accounts[2])
	}

	views, err := f.svc.Connections(t.Context(), testUserID)
	if err != nil || len(views) != 1 || views[0].Status != BankStatusActive {
		t.Fatalf("views = %+v, %v", views, err)
	}
	if views[0].Accounts[1].Mapped || views[0].Accounts[1].LastSyncStatus != BankSyncUnmapped {
		t.Fatalf("unmapped account view = %+v", views[0].Accounts[1])
	}
}

func TestCallbackRejects(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		name       string
		setup      func(t *testing.T, f *bankFixture) (code, state, bankError string)
		wantReason string
		noSession  bool
	}{
		{"missing state", func(t *testing.T, f *bankFixture) (string, string, string) { return "code", "", "" }, BankReasonInvalidState, true},
		{"garbled state", func(t *testing.T, f *bankFixture) (string, string, string) { return "code", "%%%garbage", "" }, BankReasonInvalidState, true},
		{"unknown state", func(t *testing.T, f *bankFixture) (string, string, string) {
			state, err := newBankAuthState(testUserID, bankTestKey)
			if err != nil {
				t.Fatal(err)
			}
			return "code", state, ""
		}, BankReasonInvalidState, true},
		{"state of another key", func(t *testing.T, f *bankFixture) (string, string, string) {
			state, err := newBankAuthState(testUserID, bankTestOtherKey)
			if err != nil {
				t.Fatal(err)
			}
			return "code", state, ""
		}, BankReasonInvalidState, true},
		{"expired state", func(t *testing.T, f *bankFixture) (string, string, string) {
			f.fake.AllowCode("code", bankTestSession("s"))
			state := f.authorize(t, "DKB")
			f.now = f.now.Add(bankAuthStateTTL + time.Second)
			return "code", state, ""
		}, BankReasonInvalidState, true},
		{"bank denied", func(t *testing.T, f *bankFixture) (string, string, string) {
			return "", f.authorize(t, "DKB"), "access_denied"
		}, BankReasonDenied, true},
		{"other bank error", func(t *testing.T, f *bankFixture) (string, string, string) {
			return "", f.authorize(t, "DKB"), "server_error"
		}, BankReasonBankError, true},
		{"missing code", func(t *testing.T, f *bankFixture) (string, string, string) {
			return "", f.authorize(t, "DKB"), ""
		}, BankReasonMissingCode, true},
		{"unknown code", func(t *testing.T, f *bankFixture) (string, string, string) {
			return "not-allowed", f.authorize(t, "DKB"), ""
		}, BankReasonFailed, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newBankFixture(t)
			code, state, bankErr := tt.setup(t, f)
			res := f.svc.CompleteAuthorization(ctx, code, state, bankErr)
			if res.Connected || res.Reason != tt.wantReason {
				t.Fatalf("result = %+v, want reason %q", res, tt.wantReason)
			}
			if conns, _ := f.ListLedgerBankConnections(ctx, testUserID); len(conns) != 0 {
				t.Fatalf("connection stored: %+v", conns)
			}
			if tt.noSession && len(f.fake.SessionCalls()) != 0 {
				t.Fatalf("session calls = %+v", f.fake.SessionCalls())
			}
		})
	}

	t.Run("reused state", func(t *testing.T) {
		f := newBankFixture(t)
		f.fake.AllowCode("code", bankTestSession("s1"))
		state := f.authorize(t, "DKB")
		if res := f.svc.CompleteAuthorization(ctx, "code", state, ""); !res.Connected {
			t.Fatalf("first use: %+v", res)
		}
		calls := len(f.fake.SessionCalls())
		if res := f.svc.CompleteAuthorization(ctx, "code", state, ""); res.Connected || res.Reason != BankReasonInvalidState {
			t.Fatalf("second use: %+v", res)
		}
		if len(f.fake.SessionCalls()) != calls {
			t.Fatal("session call for a reused state")
		}
	})

	t.Run("state is consumed even when the bank reports an error", func(t *testing.T) {
		f := newBankFixture(t)
		state := f.authorize(t, "DKB")
		f.svc.CompleteAuthorization(ctx, "", state, "access_denied")
		f.fake.AllowCode("code", bankTestSession("s"))
		if res := f.svc.CompleteAuthorization(ctx, "code", state, ""); res.Reason != BankReasonInvalidState {
			t.Fatalf("result = %+v", res)
		}
	})

	t.Run("module disabled for the user", func(t *testing.T) {
		f := newBankFixture(t)
		f.svc.enabled = &moduleSwitch{on: false}
		f.fake.AllowCode("code", bankTestSession("s"))
		state := f.authorize(t, "DKB")
		if res := f.svc.CompleteAuthorization(ctx, "code", state, ""); res.Connected || res.Reason != BankReasonFailed {
			t.Fatalf("result = %+v", res)
		}
		if len(f.fake.SessionCalls()) != 0 {
			t.Fatal("session created for a disabled module")
		}
	})
}

func TestRestrictedModeError(t *testing.T) {
	f := newBankFixture(t)
	f.fake.AllowCode("code", bankTestSession("s"))
	state := f.authorize(t, "DKB")
	f.fake.FailNext("/sessions", 403, "ACCESS_DENIED")
	if res := f.svc.CompleteAuthorization(t.Context(), "code", state, ""); res.Connected || res.Reason != BankReasonRestricted {
		t.Fatalf("result = %+v", res)
	}
	f.fake.FailNext("/auth", 422, "NO_ACCOUNTS_ADDED")
	_, err := f.svc.StartAuthorization(t.Context(), testUserID, BankAuthorizeInput{ASPSPName: "DKB", ASPSPCountry: "DE"})
	if !errors.Is(err, ErrBankRestricted) || strings.Contains(err.Error(), enablebankingtest.FailureMessage) {
		t.Fatalf("err = %v", err)
	}
}

func TestReauthorizeKeepsMapping(t *testing.T) {
	f := newBankFixture(t)
	ledgerID := f.addLedgerAccount(t, bankTestIBAN1, "EUR")
	first := f.connect(t, "code-1", bankTestSession("SESSION-1", bankTestAccount("uid-1", bankTestIBAN1), bankTestAccount("uid-2", bankTestIBAN2)))

	attempt := f.now.Add(-time.Hour)
	lost := f.now.Add(-time.Minute)
	if _, err := f.ModifyLedgerBankConnection(t.Context(), testUserID, first.ID, func(c *LedgerBankConnection) {
		c.Accounts[0].LastSyncAt = &attempt
		c.Accounts[0].LastSyncAttemptAt = &attempt
		c.Accounts[0].LastSyncStatus = BankSyncOK
		c.Accounts[0].LastImported = 5
		c.NotifiedDays = []int{14}
		c.LostAt = &lost
	}); err != nil {
		t.Fatal(err)
	}

	f.now = f.now.Add(24 * time.Hour)
	// the bank hands out new account UIDs; the identification hash still matches
	second := f.connect(t, "code-2", bankTestSession("SESSION-2",
		enablebanking.SessionAccount{UID: "new-uid-1", IBAN: bankTestIBAN1, Currency: "EUR", Name: "Giro", IdentificationHash: "hash-uid-1"},
		bankTestAccount("uid-2", bankTestIBAN2),
	))

	if second.ID != first.ID || !second.CreatedAt.Equal(first.CreatedAt) || !second.AuthorizedAt.Equal(f.now) {
		t.Fatalf("connection not replaced in place: %+v", second)
	}
	if second.LostAt != nil || len(second.NotifiedDays) != 0 {
		t.Fatalf("expiry state not reset: %+v", second)
	}
	a := second.Accounts[0]
	if a.UID != "new-uid-1" || a.LedgerAccountID == nil || *a.LedgerAccountID != ledgerID || a.LastSyncStatus != BankSyncOK || a.LastImported != 5 || a.LastSyncAt == nil {
		t.Fatalf("mapping or sync state lost: %+v", a)
	}
	if got := f.decryptSession(t, second); got != "SESSION-2" {
		t.Fatalf("session = %q", got)
	}
	if deleted := f.fake.Deleted(); len(deleted) != 1 || deleted[0] != "SESSION-1" {
		t.Fatalf("old session not closed: %v", deleted)
	}
	if conns, _ := f.ListLedgerBankConnections(t.Context(), testUserID); len(conns) != 1 {
		t.Fatalf("connections = %d", len(conns))
	}
}

func TestDisconnect(t *testing.T) {
	f := newBankFixture(t)
	conn := f.connect(t, "code-1", bankTestSession("SESSION-1", bankTestAccount("uid-1", bankTestIBAN1)))

	remote, err := f.svc.Disconnect(t.Context(), testUserID, conn.ID)
	if err != nil || !remote {
		t.Fatalf("remote = %v, err = %v", remote, err)
	}
	if deleted := f.fake.Deleted(); len(deleted) != 1 || deleted[0] != "SESSION-1" {
		t.Fatalf("deleted = %v", deleted)
	}
	if _, err := f.GetLedgerBankConnection(t.Context(), testUserID, conn.ID); !errors.Is(err, storage.ErrNotFound) {
		t.Fatalf("record still there: %v", err)
	}

	conn = f.connect(t, "code-2", bankTestSession("SESSION-2", bankTestAccount("uid-1", bankTestIBAN1)))
	f.fake.FailNext("/sessions/", 500, "")
	remote, err = f.svc.Disconnect(t.Context(), testUserID, conn.ID)
	if err != nil || remote {
		t.Fatalf("remote failure: remote = %v, err = %v", remote, err)
	}
	if _, err := f.GetLedgerBankConnection(t.Context(), testUserID, conn.ID); !errors.Is(err, storage.ErrNotFound) {
		t.Fatalf("record still there after remote failure: %v", err)
	}
	if _, err := f.svc.Disconnect(t.Context(), testUserID, uuid.New()); !errors.Is(err, storage.ErrNotFound) {
		t.Fatalf("unknown connection: %v", err)
	}
}

func TestMapAccount(t *testing.T) {
	f := newBankFixture(t)
	conn := f.connect(t, "code-1", bankTestSession("S", bankTestAccount("uid-1", bankTestIBAN1), bankTestAccount("uid-2", bankTestIBAN2)))

	noIBAN := f.addLedgerAccount(t, "", "EUR")
	usd := f.addLedgerAccount(t, "DE75512108001245126199", "USD")
	otherIBAN := f.addLedgerAccount(t, "DE12500105170648489890", "EUR")
	sameIBANSpaced := uuid.New()
	if err := f.CreateLedgerAccount(t.Context(), testUserID, LedgerAccount{
		ID: sameIBANSpaced, Name: "Spaced", Bank: "DKB", IBAN: "de89 3704 0044 0532 0130 00", Currency: "EUR", CreatedAt: f.now, UpdatedAt: f.now,
	}); err != nil {
		t.Fatal(err)
	}

	view, err := f.svc.MapAccount(t.Context(), testUserID, conn.ID, "uid-1", noIBAN)
	if err != nil || !view.Accounts[0].Mapped || *view.Accounts[0].LedgerAccountID != noIBAN {
		t.Fatalf("ok: %+v, %v", view, err)
	}
	if _, err := f.svc.MapAccount(t.Context(), testUserID, conn.ID, "uid-1", noIBAN); err != nil {
		t.Fatalf("remapping the same pair: %v", err)
	}
	for name, tt := range map[string]struct {
		uid  string
		acc  uuid.UUID
		want error
	}{
		"currency mismatch":           {"uid-2", usd, ErrBankCurrencyMismatch},
		"unknown ledger account":      {"uid-2", uuid.New(), ErrBankInvalidMapping},
		"iban mismatch":               {"uid-2", otherIBAN, ErrBankInvalidMapping},
		"ledger account already used": {"uid-2", noIBAN, ErrBankInvalidMapping},
		"unknown bank account":        {"nope", noIBAN, storage.ErrNotFound},
	} {
		if _, err := f.svc.MapAccount(t.Context(), testUserID, conn.ID, tt.uid, tt.acc); !errors.Is(err, tt.want) {
			t.Errorf("%s: err = %v, want %v", name, err, tt.want)
		}
	}
	if _, err := f.svc.MapAccount(t.Context(), testUserID, uuid.New(), "uid-1", noIBAN); !errors.Is(err, storage.ErrNotFound) {
		t.Errorf("unknown connection: %v", err)
	}
	// an IBAN stored with spaces and lower case still matches
	if _, err := f.svc.MapAccount(t.Context(), testUserID, conn.ID, "uid-2", sameIBANSpaced); err != nil {
		t.Errorf("normalized IBAN: %v", err)
	}
}

func TestLazyMappingAfterAccountCreated(t *testing.T) {
	f := newBankFixture(t)
	conn := f.connect(t, "code-1", bankTestSession("S", bankTestAccount("uid-1", bankTestIBAN1)))
	if conn.Accounts[0].LedgerAccountID != nil {
		t.Fatal("mapped without a ledger account")
	}
	views, err := f.svc.Connections(t.Context(), testUserID)
	if err != nil || views[0].Accounts[0].Mapped {
		t.Fatalf("views = %+v, %v", views, err)
	}

	id := f.addLedgerAccount(t, bankTestIBAN1, "EUR")
	views, err = f.svc.Connections(t.Context(), testUserID)
	if err != nil || !views[0].Accounts[0].Mapped || *views[0].Accounts[0].LedgerAccountID != id {
		t.Fatalf("not mapped lazily: %+v, %v", views, err)
	}
	stored, err := f.GetLedgerBankConnection(t.Context(), testUserID, conn.ID)
	if err != nil || stored.Accounts[0].LedgerAccountID == nil || *stored.Accounts[0].LedgerAccountID != id {
		t.Fatalf("mapping not persisted: %+v, %v", stored, err)
	}
}

func TestConnectionViewNeverContainsSession(t *testing.T) {
	f := newBankFixture(t)
	f.connect(t, "code-1", bankTestSession("SESSION-SECRET", bankTestAccount("uid-1", bankTestIBAN1)))
	views, err := f.svc.Connections(t.Context(), testUserID)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.ToLower(mustJSON(t, views)); strings.Contains(got, "session") {
		t.Fatalf("views mention the session: %s", got)
	}
}

func TestListASPSPsDoesNotCacheTheCallersCancellation(t *testing.T) {
	f := newBankFixture(t)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := f.svc.ListASPSPs(ctx, "DE"); !errors.Is(err, ErrBankUnavailable) {
		t.Fatalf("cancelled call: %v", err)
	}
	if banks, err := f.svc.ListASPSPs(t.Context(), "DE"); err != nil || len(banks) != 3 {
		t.Fatalf("a cancelled request poisoned the cache: %v, %v", banks, err)
	}
}

func TestCallbackKeepsSessionWhenOnlyMappingResolutionFails(t *testing.T) {
	f := newBankFixture(t)
	ctx := t.Context()
	ledgerID := f.addLedgerAccount(t, bankTestIBAN1, "EUR")

	// a corrupt ledger account makes the IBAN lookup of the mapping step fail
	var good []byte
	if err := f.e.Update(func(txn *badger.Txn) error {
		item, err := txn.Get(ledAccKey(testUserID, ledgerID))
		if err != nil {
			return err
		}
		if good, err = item.ValueCopy(nil); err != nil {
			return err
		}
		return txn.Set(ledAccKey(testUserID, ledgerID), []byte("{not json"))
	}); err != nil {
		t.Fatal(err)
	}

	f.fake.AllowCode("code", bankTestSession("SESSION-KEEP", bankTestAccount("uid-1", bankTestIBAN1)))
	state := f.authorize(t, "DKB")
	if res := f.svc.CompleteAuthorization(ctx, "code", state, ""); !res.Connected {
		t.Fatalf("callback failed although the connection was stored: %+v", res)
	}
	if deleted := f.fake.Deleted(); len(deleted) != 0 {
		t.Fatalf("the stored connection's session was closed: %v", deleted)
	}
	conn, err := f.FindLedgerBankConnectionByASPSP(ctx, testUserID, "DKB", "DE")
	if err != nil || f.decryptSession(t, conn) != "SESSION-KEEP" {
		t.Fatalf("connection = %+v, %v", conn, err)
	}

	// once the account is readable again the lazy mapping picks it up
	if err := f.e.Update(func(txn *badger.Txn) error { return txn.Set(ledAccKey(testUserID, ledgerID), good) }); err != nil {
		t.Fatal(err)
	}
	views, err := f.svc.Connections(ctx, testUserID)
	if err != nil || len(views) != 1 || !views[0].Accounts[0].Mapped {
		t.Fatalf("connection not usable afterwards: %+v, %v", views, err)
	}
}

func TestReauthorizeClearsStaleConsentExpiredStatus(t *testing.T) {
	f := newBankFixture(t)
	first := f.connect(t, "code-1", bankTestSession("S1", bankTestAccount("uid-1", bankTestIBAN1), bankTestAccount("uid-2", bankTestIBAN2)))
	if _, err := f.ModifyLedgerBankConnection(t.Context(), testUserID, first.ID, func(c *LedgerBankConnection) {
		attempt := f.now.Add(-time.Hour)
		c.Accounts[0].LastSyncStatus = BankSyncConsentExpired
		c.Accounts[0].LastSyncAttemptAt = &attempt
		c.Accounts[1].LastSyncAttemptAt = &attempt
		c.Accounts[0].LastImported = 4
		c.Accounts[1].LastSyncStatus = BankSyncRateLimited
	}); err != nil {
		t.Fatal(err)
	}
	renewed := f.connect(t, "code-2", bankTestSession("S2", bankTestAccount("uid-1", bankTestIBAN1), bankTestAccount("uid-2", bankTestIBAN2)))
	if a := renewed.Accounts[0]; a.LastSyncStatus != "" || a.LastImported != 4 || a.LastSyncAttemptAt != nil {
		t.Fatalf("a status of the old consent survived: %+v", a)
	}
	if renewed.Accounts[1].LastSyncStatus != BankSyncRateLimited || renewed.Accounts[1].LastSyncAttemptAt == nil {
		t.Fatalf("other statuses must be kept: %+v", renewed.Accounts[1])
	}
}

func mustJSON(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestStartAuthorizationKeepsShortMaximumExactly(t *testing.T) {
	f := newBankFixture(t)
	f.fake.SetASPSPs([]enablebanking.ASPSP{{Name: "Shortbank", Country: "DE", MaximumConsentValidity: 12 * 3600}})
	if _, err := f.svc.StartAuthorization(t.Context(), testUserID, BankAuthorizeInput{ASPSPName: "Shortbank", ASPSPCountry: "DE"}); err != nil {
		t.Fatal(err)
	}
	access := f.fake.AuthRequests()[0]["access"].(map[string]any)
	validUntil, err := time.Parse(time.RFC3339, access["valid_until"].(string))
	if want := f.now.Add(12*time.Hour - 5*time.Minute); err != nil || !validUntil.Equal(want) {
		t.Fatalf("valid_until = %v (%v), want %v: a maximum below one day must not fall back to 90 days", access["valid_until"], err, want)
	}
}

func TestListASPSPsRemembersFailureBriefly(t *testing.T) {
	f := newBankFixture(t)
	f.fake.FailAlways("/aspsps", 500, "")
	if _, err := f.svc.ListASPSPs(t.Context(), "DE"); !errors.Is(err, ErrBankUnavailable) {
		t.Fatalf("first call: %v", err)
	}
	f.fake.ClearFailures()
	if _, err := f.svc.ListASPSPs(t.Context(), "DE"); !errors.Is(err, ErrBankUnavailable) {
		t.Fatalf("a failure just seen must not hit upstream again: %v", err)
	}
	f.now = f.now.Add(bankASPSPFailureTTL + time.Second)
	if banks, err := f.svc.ListASPSPs(t.Context(), "DE"); err != nil || len(banks) != 3 {
		t.Fatalf("after the failure TTL: %v, %v", banks, err)
	}
}

func TestCallbackClosesNewSessionWhenItCannotBeStored(t *testing.T) {
	f := newBankFixture(t)
	session := bankTestSession("SESSION-LEAK", bankTestAccount("uid-1", bankTestIBAN1))
	session.ValidUntil = time.Time{}
	f.fake.AllowCode("code", session)
	state := f.authorize(t, "DKB")
	if res := f.svc.CompleteAuthorization(t.Context(), "code", state, ""); res.Connected || res.Reason != BankReasonFailed {
		t.Fatalf("result = %+v", res)
	}
	if deleted := f.fake.Deleted(); len(deleted) != 1 || deleted[0] != "SESSION-LEAK" {
		t.Fatalf("new remote session not closed: %v", deleted)
	}
}

func TestReauthorizeWithSameSessionKeepsItOpen(t *testing.T) {
	f := newBankFixture(t)
	session := bankTestSession("SAME", bankTestAccount("uid-1", bankTestIBAN1))
	f.connect(t, "code-1", session)
	f.connect(t, "code-2", session)
	if deleted := f.fake.Deleted(); len(deleted) != 0 {
		t.Fatalf("the still-current session was closed: %v", deleted)
	}
}

func TestConcurrentCallbacksCreateOneConnection(t *testing.T) {
	f := newBankFixture(t)
	var wg sync.WaitGroup
	for i := range 4 {
		code := fmt.Sprintf("code-%d", i)
		f.fake.AllowCode(code, bankTestSession(fmt.Sprintf("SESSION-%d", i), bankTestAccount("uid-1", bankTestIBAN1)))
		state := f.authorize(t, "DKB")
		wg.Add(1)
		go func() {
			defer wg.Done()
			f.svc.CompleteAuthorization(context.Background(), code, state, "")
		}()
	}
	wg.Wait()
	if conns, err := f.ListLedgerBankConnections(t.Context(), testUserID); err != nil || len(conns) != 1 {
		t.Fatalf("connections = %d, %v; want exactly one for one bank", len(conns), err)
	}
}

func TestCallbackMapsMessyIBANAndSkipsNonEURAccount(t *testing.T) {
	f := newBankFixture(t)
	eur := f.addLedgerAccount(t, bankTestIBAN1, "EUR")
	usd := enablebanking.SessionAccount{UID: "uid-usd", IBAN: bankTestIBAN2, Currency: "USD", Name: "Dollar"}
	f.addLedgerAccount(t, bankTestIBAN2, "EUR")
	conn := f.connect(t, "code", bankTestSession("S",
		bankTestAccount("uid-eur", " de02 1203 0000 0000 2020 51 "), usd))
	byUID := map[string]LedgerBankAccount{}
	for _, a := range conn.Accounts {
		byUID[a.UID] = a
	}
	if a := byUID["uid-eur"]; a.LedgerAccountID == nil || *a.LedgerAccountID != eur {
		t.Fatalf("lower-case, spaced IBAN not mapped: %+v", a)
	}
	if byUID["uid-usd"].LedgerAccountID != nil {
		t.Fatalf("a USD bank account was mapped to an EUR ledger account: %+v", byUID["uid-usd"])
	}
}

// countKeys counts the keys below u/{user}/mod/ledger/idx/{sub}.
func (f *bankFixture) countKeys(t *testing.T, sub string) int {
	t.Helper()
	prefix := []byte("u/" + testUserID + "/mod/ledger/idx/" + sub)
	n := 0
	err := f.e.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.PrefetchValues = false
		it := txn.NewIterator(opts)
		defer it.Close()
		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			n++
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return n
}
