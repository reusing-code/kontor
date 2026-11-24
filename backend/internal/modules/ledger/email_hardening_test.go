package ledger

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/reusing-code/kontor/backend/internal/cryptoutil"
	"github.com/reusing-code/kontor/backend/internal/llm/llmtest"
	"github.com/reusing-code/kontor/backend/internal/module"
)

// bannerServer answers every connection with a fixed first line and closes,
// like a non-IMAP service on the probed port.
func bannerServer(t *testing.T, banner string) (string, int) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			_, _ = conn.Write([]byte(banner + "\r\n"))
			conn.Close()
		}
	}()
	addr := ln.Addr().(*net.TCPAddr)
	return addr.IP.String(), addr.Port
}

// closedPort returns a port nothing listens on.
func closedPort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()
	return port
}

const leakMarker = "SECRET-UPSTREAM-TEXT"

func TestEmailAccountInput_RefusesControlCharacters(t *testing.T) {
	h, s := newTestHandler(t)
	h.encryptionKey = []byte("0123456789abcdef0123456789abcdef")
	mux := newMux(h)
	now := time.Now().UTC()
	account := uuid.New()
	if err := s.CreateLedgerEmailAccount(context.Background(), testUserID, LedgerEmailAccount{
		ID: account, Name: "M", IMAPHost: "h", IMAPPort: 993, Username: "u", Folder: "INBOX", ScanSince: "2026-01-01", CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"imapHost", "username", "password"} {
		for _, bad := range []string{"a\r\nA1 DELETE INBOX", "a\nb", "a\x00b"} {
			create := map[string]any{"name": "M", "imapHost": "imap.example.com", "imapPort": 993, "username": "u", "password": "pw", "folder": "INBOX", "scanSince": "2026-01-01"}
			create[field] = bad
			for _, req := range []struct{ method, path string }{
				{http.MethodPost, "/api/v1/ledger/email-accounts"},
				{http.MethodPut, "/api/v1/ledger/email-accounts/" + account.String()},
				{http.MethodPost, "/api/v1/ledger/email-accounts/folders"},
			} {
				body := create
				if req.path == "/api/v1/ledger/email-accounts/folders" {
					body = map[string]any{"imapHost": create["imapHost"], "imapPort": 993, "username": create["username"], "password": create["password"]}
				}
				rec := httptest.NewRecorder()
				mux.ServeHTTP(rec, httptest.NewRequest(req.method, req.path, jsonBody(body)))
				if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "control characters") {
					t.Errorf("%s %s with %s=%q: %d %s", req.method, req.path, field, bad, rec.Code, rec.Body.String())
				}
			}
		}
	}
}

func TestEmailAccountInput_PortRange(t *testing.T) {
	for _, tt := range []struct {
		port int
		want int
		ok   bool
	}{{0, 993, true}, {143, 143, true}, {65535, 65535, true}, {-1, 0, false}, {65536, 0, false}} {
		in := LedgerEmailAccountInput{Name: "M", IMAPHost: "h", IMAPPort: tt.port, Username: "u", Password: "p", Folder: "INBOX", ScanSince: "2026-01-01"}
		err := in.Validate()
		if (err == nil) != tt.ok || (tt.ok && in.IMAPPort != tt.want) {
			t.Errorf("port %d: err = %v, port = %d", tt.port, err, in.IMAPPort)
		}
	}
}

func TestIMAPCommand_RefusesLineBreaks(t *testing.T) {
	server := newFakeIMAP(t)
	host, port := server.host()
	client, err := newIMAPClient(host, port, false)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	for _, value := range []string{"u\r\nA1 DELETE INBOX", "u\nx", "u\x00"} {
		if err := client.Login(value, "pw"); err == nil {
			t.Errorf("Login(%q) succeeded", value)
		}
	}
	server.mu.Lock()
	defer server.mu.Unlock()
	if len(server.commands) != 0 {
		t.Errorf("commands reached the server: %q", server.commands)
	}
	// folder names are modified UTF-7 encoded, which leaves no line break
	if got := imapMailboxName("a\r\nb"); strings.ContainsAny(got, "\r\n") {
		t.Errorf("imapMailboxName = %q", got)
	}
}

func TestImportSection_RefusesEmailAccountWithControlCharacters(t *testing.T) {
	ok := LedgerEmailAccount{ID: uuid.New(), IMAPHost: "h", IMAPPort: 993, Username: "u", Folder: "INBOX"}
	if !validImportedEmailAccount(ok) {
		t.Fatal("valid account refused")
	}
	for _, bad := range []func(*LedgerEmailAccount){
		func(a *LedgerEmailAccount) { a.IMAPHost = "h\r\nx" },
		func(a *LedgerEmailAccount) { a.Username = "u\n" },
		func(a *LedgerEmailAccount) { a.IMAPPort = 70000 },
		func(a *LedgerEmailAccount) { a.Folder = "IN\rBOX" },
		func(a *LedgerEmailAccount) { a.ProcessedFolder = "Done\n" },
	} {
		a := ok
		bad(&a)
		if validImportedEmailAccount(a) {
			t.Errorf("account %+v accepted", a)
		}
	}
}

func testConnection(t *testing.T, host string, port int, password string) *httptest.ResponseRecorder {
	t.Helper()
	h, s := newTestHandler(t)
	key := []byte("0123456789abcdef0123456789abcdef")
	h.encryptionKey = key
	encrypted, err := cryptoutil.EncryptString(password, key)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	account := LedgerEmailAccount{ID: uuid.New(), Name: "M", IMAPHost: host, IMAPPort: port, Username: "u",
		EncryptedPassword: encrypted, Folder: "Orders", ScanSince: "2026-01-01", CreatedAt: now, UpdatedAt: now}
	if err := s.CreateLedgerEmailAccount(context.Background(), testUserID, account); err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	newMux(h).ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/ledger/email-accounts/"+account.ID.String()+"/test", nil))
	return rec
}

func TestTestConnection_ReturnsNoUpstreamText(t *testing.T) {
	bannerHost, bannerPort := bannerServer(t, "SSH-2.0-OpenSSH_9.6 "+leakMarker)
	wrongPassword := newFakeIMAP(t)
	wrongPassword.password = "right"
	imapHost, imapPort := wrongPassword.host()
	refusedFolder := newFakeIMAP(t)
	refusedFolder.refuse["SELECT"] = true
	folderHost, folderPort := refusedFolder.host()

	tests := []struct {
		name     string
		host     string
		port     int
		code     string
		mustSkip []string
	}{
		{"foreign banner", bannerHost, bannerPort, "imap_unavailable", []string{leakMarker, "SSH"}},
		{"closed port", "127.0.0.1", closedPort(t), "imap_unavailable", []string{"refused", "127.0.0.1", "dial"}},
		{"refused login", imapHost, imapPort, "imap_login_failed", []string{"Invalid credentials", "AUTHENTICATIONFAILED"}},
		{"refused folder", folderHost, folderPort, "imap_folder_failed", []string{"refused", "NO"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := testConnection(t, tt.host, tt.port, "wrong")
			if rec.Code != http.StatusBadGateway || !strings.Contains(rec.Body.String(), `"code":"`+tt.code+`"`) {
				t.Fatalf("status = %d, body: %s", rec.Code, rec.Body.String())
			}
			for _, s := range tt.mustSkip {
				if strings.Contains(rec.Body.String(), s) {
					t.Errorf("body %s contains %q", rec.Body.String(), s)
				}
			}
		})
	}
}

func TestScanStatus_ContainsNoUpstreamText(t *testing.T) {
	host, port := bannerServer(t, "SSH-2.0-OpenSSH_9.6 "+leakMarker)
	x := newScanJobTest(t, newFakeIMAP(t), llmtest.New())
	if _, err := x.store.ModifyLedgerEmailAccount(context.Background(), testUserID, x.account.ID, func(a *LedgerEmailAccount) {
		a.IMAPHost, a.IMAPPort = host, port
	}); err != nil {
		t.Fatal(err)
	}
	x.start(t)
	st := x.waitFinished(t)
	if st.State != EmailScanStateFailed || st.Error != imapFailureText {
		t.Fatalf("status = %+v", st)
	}
	if got := x.stored(t).LastScanStatusMessage; strings.Contains(got, leakMarker) || strings.Contains(got, "SSH") {
		t.Errorf("stored status = %q", got)
	}

	status, _ := scanOutcome("F", LedgerEmailScanResult{}, imapError(errors.New("unexpected imap greeting: "+leakMarker)), false)
	if strings.Contains(status, leakMarker) {
		t.Errorf("scheduler status = %q", status)
	}
}

func TestImportSection_RefusesSectionBeforeWriting(t *testing.T) {
	dst := newTestStore(t)
	accountID := uuid.New()
	data := []byte(`{"schemaVersion":0,"accounts":[{"id":"` + accountID.String() + `","name":"Main","bank":"DKB","currency":"EUR"}],"categories":[],"imports":[],"transactions":[],` +
		`"emailAccounts":[{"id":"` + uuid.NewString() + `","name":"M","imapHost":"h\r\nA1 DELETE INBOX","imapPort":993,"username":"u","folder":"INBOX"}],"emailOrders":[]}`)
	err := (&Module{store: dst.Store}).ImportSection(t.Context(), "user-2", data, module.NewImportResult())
	if !errors.Is(err, module.ErrInvalidSection) {
		t.Fatalf("err = %v, want ErrInvalidSection", err)
	}
	if accounts, _ := dst.ListLedgerAccounts(t.Context(), "user-2"); len(accounts) != 0 {
		t.Errorf("accounts were imported: %+v", accounts)
	}
}
