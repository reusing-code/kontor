package ledger

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/reusing-code/kontor/backend/internal/core"
	"github.com/reusing-code/kontor/backend/internal/enablebanking"
)

type fakeMailer struct {
	mu         sync.Mutex
	configured bool
	sendErr    error
	sent       []sentMail
}

type sentMail struct {
	to            []string
	subject, body string
}

func (m *fakeMailer) IsConfigured() bool { return m.configured }

func (m *fakeMailer) Send(to []string, subject, body string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.sendErr != nil {
		return m.sendErr
	}
	m.sent = append(m.sent, sentMail{to, subject, body})
	return nil
}

func (m *fakeMailer) count() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.sent)
}

type fakeUsers struct {
	users      []core.User
	disabled   map[string]bool
	failEnable map[string]bool
}

func (u *fakeUsers) ListUsers(context.Context) ([]core.User, error) { return u.users, nil }

func (u *fakeUsers) ModuleEnabled(_ context.Context, userID, _ string) (bool, error) {
	if u.failEnable[userID] {
		return false, errors.New("settings unavailable")
	}
	return !u.disabled[userID], nil
}

type schedFixture struct {
	*syncFixture
	mailer *fakeMailer
	users  *fakeUsers
	logs   *bytes.Buffer
	sched  *BankSyncScheduler
}

func newSchedFixture(t *testing.T, interval time.Duration) *schedFixture {
	t.Helper()
	f := &schedFixture{
		syncFixture: newSyncFixture(t),
		mailer:      &fakeMailer{configured: true},
		logs:        &bytes.Buffer{},
	}
	f.users = &fakeUsers{
		users:      []core.User{{ID: uuid.MustParse(testUserID), Email: "owner@example.com"}},
		disabled:   map[string]bool{},
		failEnable: map[string]bool{},
	}
	f.sched = NewBankSyncScheduler(BankSyncSchedulerConfig{
		Service: f.svc, Store: f.Store, Users: f.users, Mailer: f.mailer, PublicURL: bankTestPublicURL,
		Interval: interval, Now: func() time.Time { return f.now }, Logger: slog.New(slog.NewTextHandler(f.logs, nil)),
	})
	f.fake.SetTransactions("uid-1", []enablebanking.Transaction{apiTxn("a", "2026-04-01", "10.00", "DBIT")})
	return f
}

func (f *schedFixture) pass(t *testing.T) { t.Helper(); f.sched.runPass(t.Context()) }

func (f *schedFixture) queries() int { return len(f.fake.TransactionQueries()) }

func TestScheduler_SyncsDueAccountsAndKeepsInterval(t *testing.T) {
	f := newSchedFixture(t, 24*time.Hour)
	f.pass(t)
	if f.queries() != 1 || len(f.stored(t)) != 1 {
		t.Fatalf("first pass: %d queries, %d stored", f.queries(), len(f.stored(t)))
	}
	f.pass(t)
	if f.queries() != 1 {
		t.Fatal("account synced again before its interval")
	}
	f.now = f.now.Add(25 * time.Hour)
	f.pass(t)
	if f.queries() != 2 {
		t.Fatalf("queries = %d, want 2 after the interval", f.queries())
	}
}

func TestScheduler_FailingAccountKeepsInterval(t *testing.T) {
	f := newSchedFixture(t, 24*time.Hour)
	f.fake.FailNext("/accounts", 500, "")
	f.pass(t)
	if a := f.account(t, "uid-1"); a.LastSyncAttemptAt == nil || a.LastSyncStatus != BankSyncBankUnavailable {
		t.Fatalf("account = %+v", a)
	}
	f.pass(t)
	if f.queries() != 1 {
		t.Fatalf("failed account retried on the next tick: %d queries", f.queries())
	}
}

func TestScheduler_IntervalZeroSkipsSyncButMailsExpiry(t *testing.T) {
	f := newSchedFixture(t, 0)
	f.now = f.conn.ValidUntil.Add(-10 * 24 * time.Hour)
	f.pass(t)
	if f.queries() != 0 {
		t.Fatal("automatic sync ran with interval 0")
	}
	if f.mailer.count() != 1 {
		t.Fatalf("mails = %d, want the 14-day reminder", f.mailer.count())
	}
}

func TestScheduler_SkipsUsersWithModuleDisabled(t *testing.T) {
	f := newSchedFixture(t, 24*time.Hour)
	f.users.disabled[testUserID] = true
	f.now = f.conn.ValidUntil.Add(-2 * 24 * time.Hour)
	if err := f.CreateLedgerBankAuthState(t.Context(), testUserID, "stale", BankAuthState{ExpiresAt: f.now.Add(-time.Hour)}); err != nil {
		t.Fatal(err)
	}
	f.pass(t)
	if f.queries() != 0 || f.mailer.count() != 0 {
		t.Fatalf("disabled user touched: %d queries, %d mails", f.queries(), f.mailer.count())
	}
	if f.countStates(t) != 1 {
		t.Fatal("state of a disabled user was pruned")
	}
}

func (f *schedFixture) countStates(t *testing.T) int {
	t.Helper()
	n, err := f.PruneLedgerBankAuthStates(t.Context(), testUserID, f.now.Add(-48*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatal("unexpected prune")
	}
	// a state that expired before `now` is still stored when nothing pruned it
	n, err = f.PruneLedgerBankAuthStates(t.Context(), testUserID, f.now)
	if err != nil {
		t.Fatal(err)
	}
	return n
}

func TestScheduler_PrunesExpiredStates(t *testing.T) {
	f := newSchedFixture(t, 24*time.Hour)
	if err := f.CreateLedgerBankAuthState(t.Context(), testUserID, "stale", BankAuthState{ExpiresAt: f.now.Add(-time.Hour)}); err != nil {
		t.Fatal(err)
	}
	f.pass(t)
	if n := f.countStates(t); n != 0 {
		t.Fatalf("stale state survived the pass (%d left)", n)
	}
}

func TestScheduler_ExpiredOrLostConsentIsNeverUsed(t *testing.T) {
	t.Run("expired", func(t *testing.T) {
		f := newSchedFixture(t, 24*time.Hour)
		f.now = f.conn.ValidUntil.Add(time.Minute)
		f.pass(t)
		if f.queries() != 0 || f.mailer.count() != 0 {
			t.Fatalf("expired consent used: %d queries, %d mails", f.queries(), f.mailer.count())
		}
	})
	t.Run("lost", func(t *testing.T) {
		f := newSchedFixture(t, 24*time.Hour)
		lost := f.now
		if _, err := f.ModifyLedgerBankConnection(t.Context(), testUserID, f.conn.ID, func(c *LedgerBankConnection) { c.LostAt = &lost }); err != nil {
			t.Fatal(err)
		}
		f.now = f.conn.ValidUntil.Add(-2 * 24 * time.Hour)
		f.pass(t)
		if f.queries() != 0 || f.mailer.count() != 0 {
			t.Fatalf("lost consent used: %d queries, %d mails", f.queries(), f.mailer.count())
		}
	})
	t.Run("lost during the pass sends no mail", func(t *testing.T) {
		f := newSchedFixture(t, 24*time.Hour)
		f.now = f.conn.ValidUntil.Add(-2 * 24 * time.Hour)
		f.fake.FailNext("/accounts", 401, "EXPIRED_SESSION")
		f.pass(t)
		if f.mailer.count() != 0 {
			t.Fatal("reminder for a consent the bank just revoked")
		}
	})
}

func TestScheduler_OneUsersFailureDoesNotStopTheNext(t *testing.T) {
	f := newSchedFixture(t, 24*time.Hour)
	bad := uuid.NewString()
	f.users.users = []core.User{{ID: uuid.MustParse(bad), Email: "bad@example.com"}, f.users.users[0]}
	f.users.failEnable[bad] = true
	f.pass(t)
	if f.queries() != 1 {
		t.Fatalf("queries = %d, want the healthy user synced", f.queries())
	}
	if !strings.Contains(f.logs.String(), "bank sync pass failed") {
		t.Fatal("failure of the first user not logged")
	}
}

func TestDueExpiryReminder(t *testing.T) {
	now := time.Date(2026, 11, 13, 10, 0, 0, 0, time.UTC)
	until := func(d time.Duration) LedgerBankConnection { return LedgerBankConnection{ValidUntil: now.Add(d)} }
	days := func(n float64) time.Duration { return time.Duration(n * 24 * float64(time.Hour)) }
	tests := []struct {
		name     string
		c        LedgerBankConnection
		wantDays int
		wantMark []int
		wantOK   bool
	}{
		{"20 days: nothing", until(days(20)), 0, nil, false},
		{"exactly 14 days", until(days(14)), 14, []int{14}, true},
		{"13 days", until(days(13)), 14, []int{14}, true},
		{"13 days after the 14-day mail", func() LedgerBankConnection {
			c := until(days(13))
			c.NotifiedDays = []int{14}
			return c
		}(), 0, nil, false},
		{"3 days after the 14-day mail", func() LedgerBankConnection {
			c := until(days(3))
			c.NotifiedDays = []int{14}
			return c
		}(), 3, []int{3}, true},
		{"created 2 days before the end: one mail, both marked", until(days(2)), 3, []int{14, 3}, true},
		{"everything sent", func() LedgerBankConnection {
			c := until(days(1))
			c.NotifiedDays = []int{14, 3}
			return c
		}(), 0, nil, false},
		{"0 days left", until(0), 0, nil, false},
		{"already expired", until(-time.Hour), 0, nil, false},
		{"re-authorized (flags reset)", until(days(10)), 14, []int{14}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotDays, gotMark, ok := dueExpiryReminder(tt.c, now)
			if ok != tt.wantOK || gotDays != tt.wantDays || len(gotMark) != len(tt.wantMark) {
				t.Fatalf("= %d %v %v, want %d %v %v", gotDays, gotMark, ok, tt.wantDays, tt.wantMark, tt.wantOK)
			}
			for i := range gotMark {
				if gotMark[i] != tt.wantMark[i] {
					t.Fatalf("mark = %v, want %v", gotMark, tt.wantMark)
				}
			}
		})
	}
}

func TestScheduler_ExpiryMailContentAndMarking(t *testing.T) {
	f := newSchedFixture(t, 24*time.Hour)
	f.now = f.conn.ValidUntil.Add(-13 * 24 * time.Hour)
	f.pass(t)
	if f.mailer.count() != 1 {
		t.Fatalf("mails = %d", f.mailer.count())
	}
	m := f.mailer.sent[0]
	if len(m.to) != 1 || m.to[0] != "owner@example.com" {
		t.Fatalf("recipient = %v", m.to)
	}
	text := m.subject + "\n" + m.body
	for _, want := range []string{"DKB", "2027-02-10", bankTestPublicURL + "/ledger/bank-connections"} {
		if !strings.Contains(text, want) {
			t.Errorf("mail lacks %q:\n%s", want, text)
		}
	}
	for _, secret := range []string{bankTestIBAN1, "Account uid-1", "SESSION", "10.00", f.conn.EncryptedSessionID, "Shop"} {
		if strings.Contains(text, secret) {
			t.Errorf("mail contains %q:\n%s", secret, text)
		}
	}
	if c, _ := f.GetLedgerBankConnection(t.Context(), testUserID, f.conn.ID); len(c.NotifiedDays) != 1 || c.NotifiedDays[0] != 14 {
		t.Fatalf("notifiedDays = %v", c.NotifiedDays)
	}
	f.pass(t)
	if f.mailer.count() != 1 {
		t.Fatal("same threshold mailed twice")
	}
	f.now = f.conn.ValidUntil.Add(-2 * 24 * time.Hour)
	f.pass(t)
	if f.mailer.count() != 2 {
		t.Fatalf("mails = %d, want the 3-day reminder", f.mailer.count())
	}
}

func TestScheduler_ExpiryMailWithoutPublicURL(t *testing.T) {
	_, body := bankExpiryMail("DKB", time.Date(2027, 2, 10, 0, 0, 0, 0, time.UTC), "")
	if strings.Contains(body, "http") || !strings.Contains(body, "DKB") || !strings.Contains(body, "2027-02-10") {
		t.Fatalf("body = %q", body)
	}
}

func TestScheduler_NoSMTPThenSMTP(t *testing.T) {
	f := newSchedFixture(t, 0)
	f.mailer.configured = false
	f.now = f.conn.ValidUntil.Add(-5 * 24 * time.Hour)
	f.pass(t)
	if f.mailer.count() != 0 {
		t.Fatal("mail sent without SMTP")
	}
	if c, _ := f.GetLedgerBankConnection(t.Context(), testUserID, f.conn.ID); len(c.NotifiedDays) != 0 {
		t.Fatalf("marked without sending: %v", c.NotifiedDays)
	}
	f.mailer.configured = true
	f.pass(t)
	if f.mailer.count() != 1 {
		t.Fatalf("mails = %d after SMTP became available", f.mailer.count())
	}
}

func TestScheduler_SendErrorIsRetried(t *testing.T) {
	f := newSchedFixture(t, 0)
	f.now = f.conn.ValidUntil.Add(-5 * 24 * time.Hour)
	f.mailer.sendErr = errors.New("smtp down: owner@example.com")
	f.pass(t)
	if c, _ := f.GetLedgerBankConnection(t.Context(), testUserID, f.conn.ID); len(c.NotifiedDays) != 0 {
		t.Fatalf("marked after a failed send: %v", c.NotifiedDays)
	}
	if strings.Contains(f.logs.String(), "owner@example.com") {
		t.Fatalf("address logged: %s", f.logs.String())
	}
	f.mailer.sendErr = nil
	f.pass(t)
	if f.mailer.count() != 1 {
		t.Fatalf("mails = %d, want a retry", f.mailer.count())
	}
}

func TestScheduler_ReauthorizationResetsReminders(t *testing.T) {
	f := newSchedFixture(t, 0)
	f.now = f.conn.ValidUntil.Add(-5 * 24 * time.Hour)
	f.pass(t)
	if f.mailer.count() != 1 {
		t.Fatalf("mails = %d", f.mailer.count())
	}
	renewed := bankTestSession("SESSION-2", bankTestAccount("uid-1", bankTestIBAN1))
	renewed.ValidUntil = f.now.Add(10 * 24 * time.Hour)
	conn := f.connect(t, "code-2", renewed)
	if len(conn.NotifiedDays) != 0 {
		t.Fatalf("notifiedDays = %v", conn.NotifiedDays)
	}
	f.pass(t)
	if f.mailer.count() != 2 {
		t.Fatalf("mails = %d, want a new reminder for the renewed consent", f.mailer.count())
	}
}

func TestSchedulerShutdown(t *testing.T) {
	f := newSchedFixture(t, time.Hour)
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	go func() {
		f.sched.Run(ctx)
		close(done)
	}()
	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("scheduler did not stop after the context ended")
	}
}

func TestBankExpiryMailStripsControlCharactersFromTheSubject(t *testing.T) {
	// email.Client.Send writes the subject as a raw header without any CR/LF handling
	subject, body := bankExpiryMail("Evil\r\nBcc: attacker@example.com\x00 Bank", time.Date(2027, 2, 10, 0, 0, 0, 0, time.UTC), "")
	if strings.ContainsAny(subject, "\r\n\x00") {
		t.Fatalf("subject has control characters: %q", subject)
	}
	if !strings.Contains(subject, "Evil") || !strings.Contains(subject, "2027-02-10") {
		t.Fatalf("subject = %q", subject)
	}
	if strings.Contains(body, "\r\nBcc:") {
		t.Fatalf("body = %q", body)
	}
}
