package ledger

import (
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestParseIMAPListLine(t *testing.T) {
	for line, want := range map[string]string{
		`* LIST (\HasNoChildren) "/" "INBOX"`:                            "INBOX",
		`* LIST (\HasNoChildren) "/" INBOX`:                              "INBOX",
		`* list (\HasNoChildren) "." INBOX.Orders`:                       "INBOX.Orders",
		`* LIST (\HasNoChildren \Sent) "/" "[Gmail]/Gesendet"`:           "[Gmail]/Gesendet",
		`* LIST (\HasNoChildren \All) "/" "[Gmail]/Alle Nachrichten"`:    "[Gmail]/Alle Nachrichten",
		`* LIST (\HasNoChildren) "/" "[Gmail]/Entw&APw-rfe"`:             "[Gmail]/Entwürfe",
		`* LIST (\HasNoChildren) "/" "Bestellungen/B&APw-cher &- Musik"`: "Bestellungen/Bücher & Musik",
		`* LIST (\HasNoChildren) "/" "&ZeVnLIqe-"`:                       "日本語",
		`* LIST (\HasNoChildren) "/" "&2D3eAA-"`:                         "😀",
		`* LIST (\HasNoChildren) "\\" "Public Folders\\Team \"A\""`:      `Public Folders\Team "A"`,
		`* LIST () NIL Flat`:                                             "Flat",
		`* LIST (\HasChildren) "/" "Not &broken"`:                        "Not &broken",
		`* LIST (\HasChildren) "/" "Odd &AA-"`:                           "Odd &AA-",
		`* LIST (\Noselect \HasChildren) "/" "[Gmail]"`:                  "",
		`* LIST (\NonExistent) "/" "Ghost"`:                              "",
		`* LIST (\HasNoChildren) "/" "Unterminated`:                      "",
		`* LIST (\HasNoChildren) "/" "Two" words`:                        "",
		`* LIST (\HasNoChildren "/" "NoParen"`:                           "",
		`* LIST (\HasNoChildren) "/" ""`:                                 "",
		`* LIST (\HasNoChildren) "/" "` + strings.Repeat("x", 300) + `"`: "",
		`* LSUB (\HasNoChildren) "/" "INBOX"`:                            "",
		`* 3 EXISTS`:                                                     "",
		`A0002 OK LIST completed`:                                        "",
	} {
		got, ok := parseIMAPListLine(line)
		if got != want || ok != (want != "") {
			t.Errorf("parseIMAPListLine(%q) = %q, %v; want %q", line, got, ok, want)
		}
	}
}

func TestIMAPMailboxNameRoundTrip(t *testing.T) {
	for _, name := range []string{"INBOX", "[Gmail]/Entwürfe", "Bücher & Musik", "日本語", "😀 mixed ü", "a&b&&c", "-&-"} {
		if got := decodeIMAPMailboxName(imapMailboxName(name)); got != name {
			t.Errorf("round trip of %q = %q", name, got)
		}
	}
}

func folderIMAP(t *testing.T) *fakeIMAP {
	t.Helper()
	fake := newFakeIMAP(t)
	fake.listLines = []string{
		`* LIST (\HasNoChildren) "/" "Orders"`,
		`* LIST (\Noselect \HasChildren) "/" "[Gmail]"`,
		`* LIST (\HasNoChildren \Drafts) "/" "[Gmail]/Entw&APw-rfe"`,
		"* LIST (\\HasNoChildren) \"/\" {6}\r\nLit\"er",
		`* LIST (\HasNoChildren) "/" "INBOX"`,
		`* LIST (\HasNoChildren) "/" "archive"`,
	}
	return fake
}

func TestIMAPClientListFolders(t *testing.T) {
	fake := folderIMAP(t)
	host, port := fake.host()
	client, err := newIMAPClient(host, port, false)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	if err := client.Login("u", "p"); err != nil {
		t.Fatal(err)
	}
	got, err := client.ListFolders()
	want := []string{"Orders", "[Gmail]/Entwürfe", `Lit"er`, "INBOX", "archive"}
	if err != nil || !slices.Equal(got, want) {
		t.Fatalf("ListFolders = %q, %v; want %q", got, err, want)
	}
	// the connection is still in sync after the literal
	if err := client.SelectFolder("INBOX"); err != nil {
		t.Fatalf("select after list: %v", err)
	}
}

func TestEmailServiceListFolders(t *testing.T) {
	fake := folderIMAP(t)
	fake.password = "right"
	host, port := fake.host()
	svc := NewEmailService(nil, nil, nil, nil)
	account := LedgerEmailAccount{IMAPHost: host, IMAPPort: port, Username: "u"}

	got, err := svc.ListFolders(account, "right")
	want := []string{"INBOX", "[Gmail]/Entwürfe", "archive", `Lit"er`, "Orders"}
	if err != nil || !slices.Equal(got, want) {
		t.Fatalf("ListFolders = %q, %v; want %q", got, err, want)
	}
	if _, err := svc.ListFolders(account, "wrong"); !errors.Is(err, ErrIMAPLogin) {
		t.Fatalf("wrong password: %v", err)
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	closed := ln.Addr().(*net.TCPAddr).Port
	ln.Close()
	account.IMAPPort = closed
	if _, err := svc.ListFolders(account, "right"); !errors.Is(err, ErrIMAP) || errors.Is(err, ErrIMAPLogin) {
		t.Fatalf("unreachable server: %v", err)
	}
}

func TestListLedgerEmailFoldersHandler(t *testing.T) {
	h, _ := newTestHandlerWithEmailKey(t)
	mux := newMux(h)
	fake := folderIMAP(t)
	fake.password = "s3cret"
	host, port := fake.host()

	post := func(path string, body map[string]any) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, path, jsonBody(body)))
		return rec
	}
	folders := func(body map[string]any) (int, map[string]any) {
		rec := post("/api/v1/ledger/email-accounts/folders", body)
		var out map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatalf("decode %q: %v", rec.Body.String(), err)
		}
		return rec.Code, out
	}
	conn := func(extra map[string]any) map[string]any {
		body := map[string]any{"imapHost": host, "imapPort": port, "username": "u", "useTls": false}
		for k, v := range extra {
			body[k] = v
		}
		return body
	}

	status, out := folders(conn(map[string]any{"password": "s3cret"}))
	if status != http.StatusOK {
		t.Fatalf("entered credentials: %d %v", status, out)
	}
	got, _ := json.Marshal(out["folders"])
	if string(got) != `["INBOX","[Gmail]/Entwürfe","archive","Lit\"er","Orders"]` {
		t.Fatalf("folders = %s", got)
	}

	if status, out := folders(conn(map[string]any{"password": "wrong"})); status != http.StatusBadGateway || out["code"] != "imap_login_failed" {
		t.Fatalf("bad credentials: %d %v", status, out)
	} else if msg, _ := out["error"].(string); strings.Contains(msg, "AUTHENTICATIONFAILED") || strings.Contains(msg, "Invalid credentials") {
		t.Fatalf("server text reached the client: %q", msg)
	}
	if status, out := folders(conn(nil)); status != http.StatusConflict || out["code"] != "password_missing" {
		t.Fatalf("no password, no account: %d %v", status, out)
	}
	if status, _ := folders(conn(map[string]any{"accountId": uuid.NewString()})); status != http.StatusNotFound {
		t.Fatalf("unknown account: %d", status)
	}
	for name, body := range map[string]map[string]any{
		"no host":           {"imapHost": " ", "imapPort": port, "username": "u", "password": "p"},
		"no username":       {"imapHost": host, "imapPort": port, "username": "", "password": "p"},
		"port out of range": {"imapHost": host, "imapPort": 70000, "username": "u", "password": "p"},
		"command injection": {"imapHost": host, "imapPort": port, "username": "u\" x\r\nA9 DELETE INBOX", "password": "p"},
		"password newline":  {"imapHost": host, "imapPort": port, "username": "u", "password": "p\r\nA9 LOGOUT"},
	} {
		if status, out := folders(body); status != http.StatusBadRequest {
			t.Fatalf("%s: %d %v", name, status, out)
		}
	}

	// without a password the stored one of the account is used, with the entered connection
	rec := post("/api/v1/ledger/email-accounts", map[string]any{
		"name": "Mail", "imapHost": "imap.invalid", "imapPort": 993, "username": "old", "password": "s3cret",
		"useTls": true, "scanSince": "2026-01-01", "folder": "INBOX",
	})
	var account LedgerEmailAccount
	if err := json.Unmarshal(rec.Body.Bytes(), &account); err != nil || rec.Code != http.StatusCreated {
		t.Fatalf("create account: %d %v", rec.Code, err)
	}
	if status, out := folders(conn(map[string]any{"accountId": account.ID.String()})); status != http.StatusOK {
		t.Fatalf("stored password: %d %v", status, out)
	}

	fake.mu.Lock()
	logins := strings.Join(fake.logins, "\n")
	fake.mu.Unlock()
	if strings.Contains(logins, "DELETE") || strings.Contains(logins, "A9") {
		t.Fatalf("injected command reached the server: %s", logins)
	}

	fake.ln.Close()
	if status, out := folders(conn(map[string]any{"password": "s3cret"})); status != http.StatusBadGateway || out["code"] != "imap_unavailable" {
		t.Fatalf("server down: %d %v", status, out)
	}
}
