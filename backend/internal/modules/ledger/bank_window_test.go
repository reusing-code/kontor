package ledger

import (
	"testing"
	"time"
)

func TestBankSyncWindow(t *testing.T) {
	today := time.Date(2026, 4, 20, 15, 30, 0, 0, time.UTC)
	tests := []struct {
		name   string
		newest string
		has    bool
		today  time.Time
		want   BankWindow
	}{
		{"no transactions", "", false, today, BankWindow{Longest: true}},
		{"no transactions ignores newest", "2026-04-07", false, today, BankWindow{Longest: true}},
		{"regular", "2026-04-07", true, today, BankWindow{DateFrom: "2026-03-24", DateTo: "2026-04-20"}},
		{"newest is today", "2026-04-20", true, today, BankWindow{DateFrom: "2026-04-06", DateTo: "2026-04-20"}},
		{"future newest inside overlap", "2026-04-25", true, today, BankWindow{DateFrom: "2026-04-11", DateTo: "2026-04-20"}},
		{"future newest clamped", "2026-05-30", true, today, BankWindow{DateFrom: "2026-04-20", DateTo: "2026-04-20"}},
		{"far future newest clamped to today", "2027-01-01", true, today, BankWindow{DateFrom: "2026-04-20", DateTo: "2026-04-20"}},
		{"unparsable falls back to 90 days", "garbage", true, today, BankWindow{DateFrom: "2026-01-20", DateTo: "2026-04-20"}},
		{"month boundary", "2026-03-05", true, today, BankWindow{DateFrom: "2026-02-19", DateTo: "2026-04-20"}},
		{"leap day", "2024-03-10", true, time.Date(2024, 3, 12, 0, 0, 0, 0, time.UTC), BankWindow{DateFrom: "2024-02-25", DateTo: "2024-03-12"}},
		{"year boundary", "2026-01-05", true, time.Date(2026, 1, 8, 0, 0, 0, 0, time.UTC), BankWindow{DateFrom: "2025-12-22", DateTo: "2026-01-08"}},
		{"non-UTC today is read as UTC", "2026-04-07", true, time.Date(2026, 4, 20, 23, 30, 0, 0, time.FixedZone("x", -3600)), BankWindow{DateFrom: "2026-03-24", DateTo: "2026-04-21"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := BankSyncWindow(tt.newest, tt.has, tt.today); got != tt.want {
				t.Fatalf("window = %+v, want %+v", got, tt.want)
			}
		})
	}
}
