package ledger

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/reusing-code/kontor/backend/internal/cryptoutil"
	"github.com/reusing-code/kontor/backend/internal/llm"
	"github.com/reusing-code/kontor/backend/internal/llm/llmtest"
)

func TestAccountDue(t *testing.T) {
	now := time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC)
	interval := 6 * time.Hour
	recent := now.Add(-1 * time.Hour)
	stale := now.Add(-7 * time.Hour)
	exact := now.Add(-interval)

	cases := []struct {
		name       string
		lastScanAt *time.Time
		want       bool
	}{
		{"never scanned", nil, true},
		{"scanned recently", &recent, false},
		{"scan overdue", &stale, true},
		{"exactly at interval", &exact, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			account := LedgerEmailAccount{LastScanAt: tc.lastScanAt}
			if got := accountDue(account, now, interval); got != tc.want {
				t.Errorf("accountDue() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestSchedulerTickInterval(t *testing.T) {
	if got := schedulerTickInterval(6 * time.Hour); got != time.Hour {
		t.Errorf("tick for 6h interval = %v, want 1h", got)
	}
	if got := schedulerTickInterval(15 * time.Minute); got != 15*time.Minute {
		t.Errorf("tick for 15m interval = %v, want 15m", got)
	}
}

func TestEmailSchedulerTick(t *testing.T) {
	if got := emailSchedulerTick(6 * time.Hour); got != emailScanResumeDelay {
		t.Errorf("tick for 6h interval = %v, want %v", got, emailScanResumeDelay)
	}
	if got := emailSchedulerTick(time.Minute); got != time.Minute {
		t.Errorf("tick for 1m interval = %v, want 1m", got)
	}
}

func TestAccountDue_PartialScanResumesAfterTheDelay(t *testing.T) {
	now := time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC)
	before := now.Add(-emailScanResumeDelay + time.Second)
	after := now.Add(-emailScanResumeDelay)
	for _, tc := range []struct {
		name    string
		attempt time.Time
		pending bool
		want    bool
	}{
		{"pending, delay not over", before, true, false},
		{"pending, delay over", after, true, true},
		{"not pending keeps the interval", after, false, false},
	} {
		account := LedgerEmailAccount{LastScanAttemptAt: &tc.attempt, ScanResumePending: tc.pending}
		if got := accountDue(account, now, 6*time.Hour); got != tc.want {
			t.Errorf("%s: accountDue() = %v, want %v", tc.name, got, tc.want)
		}
	}
	short := now.Add(-time.Minute)
	if !accountDue(LedgerEmailAccount{LastScanAttemptAt: &short, ScanResumePending: true}, now, time.Minute) {
		t.Error("an interval below the resume delay must win")
	}
}

func TestScanStatusMessage(t *testing.T) {
	msg := scanStatusMessage("Rechnungen", LedgerEmailScanResult{EmailsScanned: 12, OrdersNew: 3, OrdersLinked: 2})
	want := `Background scan of folder "Rechnungen": 12 emails scanned, 3 new orders, 2 linked`
	if msg != want {
		t.Errorf("scanStatusMessage() = %q, want %q", msg, want)
	}
}

func TestScanAccount_WithoutFolderIsSkippedAndStaysDue(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	past := time.Now().UTC().Add(-48 * time.Hour)
	account := LedgerEmailAccount{
		ID: uuid.New(), Name: "Mailbox", IMAPHost: "imap.invalid", IMAPPort: 993, Username: "u",
		EncryptedPassword: "not-decryptable", CreatedAt: past, UpdatedAt: past,
	}
	if err := s.CreateLedgerEmailAccount(ctx, testUserID, account); err != nil {
		t.Fatalf("CreateLedgerEmailAccount: %v", err)
	}
	scheduler := &EmailScanScheduler{
		store:    s.Store,
		service:  NewEmailService(s.Store, llm.Disabled{}, nil, slog.New(slog.DiscardHandler)),
		interval: time.Hour,
		logger:   slog.New(slog.DiscardHandler),
	}

	scheduler.scanAccount(ctx, testUserID, account)
	first, err := s.GetLedgerEmailAccount(ctx, testUserID, account.ID)
	if err != nil {
		t.Fatalf("GetLedgerEmailAccount: %v", err)
	}
	if first.LastScanAt != nil {
		t.Errorf("LastScanAt = %v, want nil", first.LastScanAt)
	}
	if want := "Background scan skipped: choose a folder to scan"; first.LastScanStatusMessage != want {
		t.Errorf("status = %q, want %q", first.LastScanStatusMessage, want)
	}

	scheduler.scanAccount(ctx, testUserID, first)
	second, err := s.GetLedgerEmailAccount(ctx, testUserID, account.ID)
	if err != nil {
		t.Fatalf("GetLedgerEmailAccount: %v", err)
	}
	if !second.UpdatedAt.Equal(first.UpdatedAt) {
		t.Errorf("UpdatedAt changed from %v to %v on a repeated skip", first.UpdatedAt, second.UpdatedAt)
	}
}

func TestFinishScan_AttemptAndCompletion(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	past := time.Now().UTC().Add(-48 * time.Hour)
	account := LedgerEmailAccount{ID: uuid.New(), Name: "Mailbox", IMAPHost: "h", IMAPPort: 993, Username: "u", LastScanAt: &past, CreatedAt: past, UpdatedAt: past}
	if err := s.CreateLedgerEmailAccount(ctx, testUserID, account); err != nil {
		t.Fatalf("CreateLedgerEmailAccount: %v", err)
	}
	scheduler := &EmailScanScheduler{store: s.Store, logger: slog.New(slog.DiscardHandler)}
	get := func() LedgerEmailAccount {
		got, err := s.GetLedgerEmailAccount(ctx, testUserID, account.ID)
		if err != nil {
			t.Fatalf("GetLedgerEmailAccount: %v", err)
		}
		return got
	}

	scheduler.finishScan(ctx, testUserID, account, "skipped", false, false, false)
	got := get()
	if got.LastScanAt == nil || !got.LastScanAt.Equal(past) || got.LastScanAttemptAt != nil || got.LastScanStatusMessage != "skipped" {
		t.Errorf("skip: %+v", got)
	}

	scheduler.finishScan(ctx, testUserID, account, "failed", true, false, false)
	got = get()
	if got.LastScanAt == nil || !got.LastScanAt.Equal(past) || got.LastScanAttemptAt == nil || !got.LastScanAttemptAt.After(past) {
		t.Errorf("failed attempt must keep LastScanAt and stamp the attempt: %+v", got)
	}

	scheduler.finishScan(ctx, testUserID, account, "done", true, true, false)
	got = get()
	if got.LastScanAt == nil || !got.LastScanAt.After(past) || got.LastScanStatusMessage != "done" {
		t.Errorf("complete: %+v", got)
	}
}

func TestAccountDue_UsesTheLastAttempt(t *testing.T) {
	now := time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC)
	interval := 6 * time.Hour
	oldComplete := now.Add(-30 * time.Hour)
	recentAttempt := now.Add(-time.Hour)
	staleAttempt := now.Add(-7 * time.Hour)
	tests := []struct {
		name    string
		account LedgerEmailAccount
		want    bool
	}{
		{"failed recently, old complete scan", LedgerEmailAccount{LastScanAt: &oldComplete, LastScanAttemptAt: &recentAttempt}, false},
		{"attempt overdue", LedgerEmailAccount{LastScanAt: &oldComplete, LastScanAttemptAt: &staleAttempt}, true},
		{"legacy account without attempt", LedgerEmailAccount{LastScanAt: &oldComplete}, true},
		{"never scanned", LedgerEmailAccount{}, true},
	}
	for _, tt := range tests {
		if got := accountDue(tt.account, now, interval); got != tt.want {
			t.Errorf("%s: accountDue = %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestScanAccount_LLMDisabledIsSkippedAndStaysDue(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	past := time.Now().UTC().Add(-48 * time.Hour)
	account := LedgerEmailAccount{
		ID: uuid.New(), Name: "Mailbox", IMAPHost: "imap.invalid", IMAPPort: 993, Username: "u", Folder: "INBOX",
		EncryptedPassword: "not-decryptable", LastScanAt: &past, CreatedAt: past, UpdatedAt: past,
	}
	if err := s.CreateLedgerEmailAccount(ctx, testUserID, account); err != nil {
		t.Fatalf("CreateLedgerEmailAccount: %v", err)
	}
	scheduler := &EmailScanScheduler{
		store:    s.Store,
		service:  NewEmailService(s.Store, llm.Disabled{}, nil, slog.New(slog.DiscardHandler)),
		interval: time.Hour,
		logger:   slog.New(slog.DiscardHandler),
	}

	scheduler.scanAccount(ctx, testUserID, account)
	got, err := s.GetLedgerEmailAccount(ctx, testUserID, account.ID)
	if err != nil {
		t.Fatalf("GetLedgerEmailAccount: %v", err)
	}
	if got.LastScanAt == nil || !got.LastScanAt.Equal(past) {
		t.Errorf("LastScanAt = %v, want unchanged %v", got.LastScanAt, past)
	}
	if want := "Background scan skipped: the LLM is not configured"; got.LastScanStatusMessage != want {
		t.Errorf("status = %q, want %q", got.LastScanStatusMessage, want)
	}
}

func TestScanOutcome(t *testing.T) {
	ok := LedgerEmailScanResult{EmailsScanned: 2, OrdersNew: 1}
	partial := LedgerEmailScanResult{Partial: true}
	tests := []struct {
		name        string
		result      LedgerEmailScanResult
		err         error
		timedOut    bool
		wantStatus  string
		wantAdvance bool
	}{
		{"complete", ok, nil, false, `Background scan of folder "F": 2 emails scanned, 1 new orders, 0 linked`, true},
		{"repeated llm failures", partial, nil, false, `Background scan of folder "F" stopped after repeated LLM failures; it is retried shortly`, false},
		{"timeout", partial, context.DeadlineExceeded, true, `Background scan of folder "F" timed out; it continues shortly`, false},
		{"deadline without scan timeout", ok, context.DeadlineExceeded, false, "Background scan failed: context deadline exceeded", false},
		{"imap failure", ok, ErrIMAP, false, "Background scan failed: imap", false},
		{"skipped messages", LedgerEmailScanResult{EmailsScanned: 3, OrdersNew: 1, Skipped: 2}, nil, false, `Background scan of folder "F": 3 emails scanned, 1 new orders, 0 linked; 2 skipped and retried on the next run`, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, advance := scanOutcome("F", tt.result, tt.err, tt.timedOut)
			if status != tt.wantStatus || advance != tt.wantAdvance {
				t.Errorf("scanOutcome() = %q, %v; want %q, %v", status, advance, tt.wantStatus, tt.wantAdvance)
			}
		})
	}
}

type imapSchedulerTest struct {
	scheduler *EmailScanScheduler
	store     *testStores
	account   LedgerEmailAccount
	past      time.Time
}

func newIMAPSchedulerTest(t *testing.T, server *fakeIMAP, fake llm.Extractor) *imapSchedulerTest {
	t.Helper()
	s := newTestStore(t)
	key := []byte("0123456789abcdef0123456789abcdef")
	encrypted, err := cryptoutil.EncryptString("pw", key)
	if err != nil {
		t.Fatalf("EncryptString: %v", err)
	}
	host, port := server.host()
	past := time.Now().UTC().Add(-72 * time.Hour)
	account := LedgerEmailAccount{
		ID: uuid.New(), Name: "Mailbox", IMAPHost: host, IMAPPort: port, UseTLS: false, Username: "u",
		EncryptedPassword: encrypted, Folder: "Orders", ScanSince: "2026-01-01", LastScanAt: &past, CreatedAt: past, UpdatedAt: past,
	}
	if err := s.CreateLedgerEmailAccount(context.Background(), testUserID, account); err != nil {
		t.Fatalf("CreateLedgerEmailAccount: %v", err)
	}
	logger := slog.New(slog.DiscardHandler)
	return &imapSchedulerTest{
		scheduler: &EmailScanScheduler{store: s.Store, service: NewEmailService(s.Store, fake, nil, logger), encryptionKey: key, interval: 6 * time.Hour, logger: logger},
		store:     s, account: account, past: past,
	}
}

func (x *imapSchedulerTest) run(t *testing.T) LedgerEmailAccount {
	t.Helper()
	x.scheduler.scanAccount(context.Background(), testUserID, x.account)
	got, err := x.store.GetLedgerEmailAccount(context.Background(), testUserID, x.account.ID)
	if err != nil {
		t.Fatalf("GetLedgerEmailAccount: %v", err)
	}
	return got
}

func (x *imapSchedulerTest) assertIncomplete(t *testing.T, got LedgerEmailAccount) {
	t.Helper()
	if got.LastScanAt == nil || !got.LastScanAt.Equal(x.past) {
		t.Errorf("LastScanAt = %v, want unchanged %v", got.LastScanAt, x.past)
	}
	if got.LastScanAttemptAt == nil || !got.LastScanAttemptAt.After(x.past) {
		t.Errorf("LastScanAttemptAt = %v, want stamped", got.LastScanAttemptAt)
	}
	if got.ScanResumePending || accountDue(got, time.Now().UTC().Add(emailScanResumeDelay), x.scheduler.interval) {
		t.Error("an incomplete scan must keep the normal retry interval")
	}
}

func TestScanAccount_CompleteScanAdvancesLastScanAt(t *testing.T) {
	server := newFakeIMAP(t, "AmazonDE-1.eml", "AmazonDE-2.eml")
	x := newIMAPSchedulerTest(t, server, preClassFake(nil))
	got := x.run(t)
	if got.LastScanAt == nil || !got.LastScanAt.After(x.past) || got.LastScanAttemptAt == nil {
		t.Fatalf("account = %+v", got)
	}
	if !strings.Contains(got.LastScanStatusMessage, "2 emails scanned, 2 new orders") {
		t.Errorf("status = %q", got.LastScanStatusMessage)
	}
}

func TestScanAccount_SkippedMessagesKeepLastScanAt(t *testing.T) {
	t.Run("rejected header fetch", func(t *testing.T) {
		server := newFakeIMAP(t, "AmazonDE-1.eml", "AmazonDE-2.eml")
		server.set(server.rejectHeader, 1)
		x := newIMAPSchedulerTest(t, server, preClassFake(nil))
		got := x.run(t)
		x.assertIncomplete(t, got)
		if !strings.Contains(got.LastScanStatusMessage, "1 skipped") {
			t.Errorf("status = %q", got.LastScanStatusMessage)
		}
	})
	t.Run("transient LLM failure for one message", func(t *testing.T) {
		server := newFakeIMAP(t, "AmazonDE-1.eml", "AmazonDE-2.eml")
		fake := llmtest.New(llmtest.Error(&llm.APIError{StatusCode: 503}), llmtest.JSON(goldenOrders[1].extraction()))
		x := newIMAPSchedulerTest(t, server, fake)
		got := x.run(t)
		x.assertIncomplete(t, got)
		if !strings.Contains(got.LastScanStatusMessage, "1 new orders") || !strings.Contains(got.LastScanStatusMessage, "1 skipped") {
			t.Errorf("status = %q", got.LastScanStatusMessage)
		}
	})
	t.Run("skipped pre-classification batch", func(t *testing.T) {
		server := newFakeIMAP(t, "AmazonDE-1.eml", "AmazonDE-2.eml")
		fake := preClassFake(func(llm.Request) (json.RawMessage, error) { return nil, &llm.APIError{StatusCode: 503} })
		x := newIMAPSchedulerTest(t, server, fake)
		x.account.PreClassify = true
		if err := x.store.UpdateLedgerEmailAccount(context.Background(), testUserID, x.account); err != nil {
			t.Fatalf("UpdateLedgerEmailAccount: %v", err)
		}
		x.assertIncomplete(t, x.run(t))
	})
	t.Run("connection dropped mid-scan", func(t *testing.T) {
		server := newFakeIMAP(t, "AmazonDE-1.eml", "AmazonDE-2.eml")
		server.set(server.dropOnHeader, 2)
		x := newIMAPSchedulerTest(t, server, preClassFake(nil))
		got := x.run(t)
		x.assertIncomplete(t, got)
		if !strings.HasPrefix(got.LastScanStatusMessage, "Background scan failed") {
			t.Errorf("status = %q", got.LastScanStatusMessage)
		}
	})
}

func TestScanAccount_PersistentlyRejectedMessageStopsBlockingCompleteScans(t *testing.T) {
	server := newFakeIMAP(t, "AmazonDE-1.eml", "AmazonDE-2.eml")
	server.set(server.rejectHeader, 1)
	x := newIMAPSchedulerTest(t, server, preClassFake(nil))

	for scan := 1; scan < maxFetchRejections; scan++ {
		x.assertIncomplete(t, x.run(t))
	}
	// The third refusal gives up on the message and the scan completes.
	got := x.run(t)
	if got.LastScanAt == nil || !got.LastScanAt.After(x.past) {
		t.Fatalf("LastScanAt = %v, want advanced after the message was given up on", got.LastScanAt)
	}
	if strings.Contains(got.LastScanStatusMessage, "skipped") {
		t.Errorf("status = %q", got.LastScanStatusMessage)
	}
	if seen, _ := x.store.HasLedgerEmailSeen(context.Background(), testUserID, x.account.ID, "uid:1"); !seen {
		t.Error("rejected message not marked as ignored")
	}
	// Later scans do not even ask the server for it.
	x.account.LastScanAt = got.LastScanAt
	if again := x.run(t); again.LastScanAt == nil || !again.LastScanAt.After(*got.LastScanAt) {
		t.Errorf("fourth scan did not complete: %+v", again)
	}
}
