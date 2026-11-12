package ledger

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"
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
		service:  NewEmailService(s.Store, slog.New(slog.DiscardHandler)),
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

	scheduler.finishScan(ctx, testUserID, account, "skipped", false, false)
	got := get()
	if got.LastScanAt == nil || !got.LastScanAt.Equal(past) || got.LastScanAttemptAt != nil || got.LastScanStatusMessage != "skipped" {
		t.Errorf("skip: %+v", got)
	}

	scheduler.finishScan(ctx, testUserID, account, "failed", true, false)
	got = get()
	if got.LastScanAt == nil || !got.LastScanAt.Equal(past) || got.LastScanAttemptAt == nil || !got.LastScanAttemptAt.After(past) {
		t.Errorf("failed attempt must keep LastScanAt and stamp the attempt: %+v", got)
	}

	scheduler.finishScan(ctx, testUserID, account, "done", true, true)
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
