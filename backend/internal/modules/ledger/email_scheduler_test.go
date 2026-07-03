package ledger

import (
	"testing"
	"time"
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
	msg := scanStatusMessage(LedgerEmailScanResult{EmailsScanned: 12, OrdersNew: 3, OrdersLinked: 2})
	want := "Background scan: 12 emails scanned, 3 new orders, 2 linked"
	if msg != want {
		t.Errorf("scanStatusMessage() = %q, want %q", msg, want)
	}
}
