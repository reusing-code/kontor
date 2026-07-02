package contracts

import (
	"testing"
	"time"
)

func today() string {
	return time.Now().UTC().Truncate(24 * time.Hour).Format(dateFormat)
}

func monthsAgo(n int) string {
	return time.Now().UTC().Truncate(24*time.Hour).AddDate(0, -n, 0).Format(dateFormat)
}

func monthsFromNow(n int) string {
	return time.Now().UTC().Truncate(24*time.Hour).AddDate(0, n, 0).Format(dateFormat)
}

func TestCancellationDate_WithEndDate_ReturnsNil(t *testing.T) {
	c := Contract{StartDate: "2024-01-01", EndDate: "2025-12-31"}
	if c.CancellationDate() != nil {
		t.Error("expected nil for contract with endDate")
	}
}

func TestCancellationDate_NoExtension(t *testing.T) {
	// Start 6 months ago, 12 month minimum, 3 month notice, no extension
	// minEnd = 6 months from now, cancellation = 3 months from now
	c := Contract{
		StartDate:               monthsAgo(6),
		MinimumDurationMonths:   12,
		NoticePeriodMonths:      3,
		ExtensionDurationMonths: 0,
	}
	got := c.CancellationDate()
	if got == nil {
		t.Fatal("expected non-nil cancellation date")
	}
	want := monthsFromNow(3)
	if *got != want {
		t.Errorf("got %s, want %s", *got, want)
	}
}

func TestCancellationDate_NoExtension_PastMinEnd(t *testing.T) {
	// Start 24 months ago, 12 month minimum, 3 month notice, no extension
	// minEnd = 12 months ago, cancellation = 15 months ago (past) → clamp to today
	c := Contract{
		StartDate:               monthsAgo(24),
		MinimumDurationMonths:   12,
		NoticePeriodMonths:      3,
		ExtensionDurationMonths: 0,
	}
	got := c.CancellationDate()
	if got == nil {
		t.Fatal("expected non-nil cancellation date")
	}
	if *got != today() {
		t.Errorf("got %s, want today %s", *got, today())
	}
}

func TestCancellationDate_WithExtension_FutureMinEnd(t *testing.T) {
	// Start 6 months ago, 24 month minimum, 3 month notice, 12 month extension
	// minEnd = 18 months from now (future), periodEnd = minEnd
	// cancellation = 15 months from now
	c := Contract{
		StartDate:               monthsAgo(6),
		MinimumDurationMonths:   24,
		NoticePeriodMonths:      3,
		ExtensionDurationMonths: 12,
	}
	got := c.CancellationDate()
	if got == nil {
		t.Fatal("expected non-nil cancellation date")
	}
	want := monthsFromNow(15)
	if *got != want {
		t.Errorf("got %s, want %s", *got, want)
	}
}

func TestCancellationDate_WithExtension_PastMinEnd(t *testing.T) {
	// Start 30 months ago, 12 month minimum, 3 month notice, 12 month extension
	// minEnd = 18 months ago
	// periodEnd advances: -18, -6, +6 (first future)
	// cancellation = +6 - 3 = +3
	c := Contract{
		StartDate:               monthsAgo(30),
		MinimumDurationMonths:   12,
		NoticePeriodMonths:      3,
		ExtensionDurationMonths: 12,
	}
	got := c.CancellationDate()
	if got == nil {
		t.Fatal("expected non-nil cancellation date")
	}
	want := monthsFromNow(3)
	if *got != want {
		t.Errorf("got %s, want %s", *got, want)
	}
}

func TestCancellationDate_WithExtension_NoticePastToday(t *testing.T) {
	// Start 22 months ago, 12 month minimum, 3 month notice, 12 month extension
	// minEnd = 10 months ago
	// periodEnd advances: -10, +2 (first future)
	// cancellation = +2 - 3 = -1 (past!) → advance one more: +14 - 3 = +11
	c := Contract{
		StartDate:               monthsAgo(22),
		MinimumDurationMonths:   12,
		NoticePeriodMonths:      3,
		ExtensionDurationMonths: 12,
	}
	got := c.CancellationDate()
	if got == nil {
		t.Fatal("expected non-nil cancellation date")
	}
	want := monthsFromNow(11)
	if *got != want {
		t.Errorf("got %s, want %s", *got, want)
	}
}

func TestCancellationDate_InvalidStartDate(t *testing.T) {
	c := Contract{StartDate: "not-a-date"}
	if c.CancellationDate() != nil {
		t.Error("expected nil for invalid start date")
	}
}

func TestIsExpired_NoEndDate(t *testing.T) {
	c := Contract{StartDate: "2024-01-01"}
	if c.IsExpired() {
		t.Error("contract without endDate should not be expired")
	}
}

func TestIsExpired_FutureEndDate(t *testing.T) {
	c := Contract{StartDate: "2024-01-01", EndDate: monthsFromNow(6)}
	if c.IsExpired() {
		t.Error("contract with future endDate should not be expired")
	}
}

func TestIsExpired_PastEndDate(t *testing.T) {
	c := Contract{StartDate: "2024-01-01", EndDate: monthsAgo(1)}
	if !c.IsExpired() {
		t.Error("contract with past endDate should be expired")
	}
}

func TestIsExpired_InvalidEndDate(t *testing.T) {
	c := Contract{StartDate: "2024-01-01", EndDate: "bad"}
	if c.IsExpired() {
		t.Error("contract with invalid endDate should not be expired")
	}
}
