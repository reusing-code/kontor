package contracts

import (
	"context"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/reusing-code/kontor/backend/internal/core"
	"github.com/reusing-code/kontor/backend/internal/storage"
	"github.com/reusing-code/kontor/backend/internal/storage/link"
)

func newTestStores(t *testing.T) (*core.Store, *Store) {
	t.Helper()
	e, err := storage.Open(t.TempDir(), slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatalf("opening engine: %v", err)
	}
	t.Cleanup(func() { e.Close() })
	return core.NewStore(e), NewStore(e, link.NewRegistry())
}

func newTestScheduler(t *testing.T) (*Scheduler, *core.Store) {
	t.Helper()
	coreStore, contractStore := newTestStores(t)
	return NewScheduler(coreStore, contractStore, nil, slog.New(slog.DiscardHandler)), coreStore
}

func newTestUser() core.User {
	return core.User{
		ID:    uuid.New(),
		Email: "test@example.com",
	}
}

func TestCheckUser_SkipsDisabled(t *testing.T) {
	user := newTestUser()
	sched, coreStore := newTestScheduler(t)

	if err := coreStore.UpdateSettings(t.Context(), user.ID.String(), core.UserSettings{
		RenewalDays: 90, ReminderFrequency: "disabled",
	}); err != nil {
		t.Fatalf("updating settings: %v", err)
	}

	if err := sched.checkUser(context.Background(), user); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCheckUser_SkipsEmptyFrequency(t *testing.T) {
	user := newTestUser()
	sched, coreStore := newTestScheduler(t)

	if err := coreStore.UpdateSettings(t.Context(), user.ID.String(), core.UserSettings{
		RenewalDays: 90, ReminderFrequency: "",
	}); err != nil {
		t.Fatalf("updating settings: %v", err)
	}

	if err := sched.checkUser(context.Background(), user); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCheckUser_SkipsWhenNotEnoughTimeElapsed(t *testing.T) {
	user := newTestUser()
	sched, coreStore := newTestScheduler(t)

	if err := coreStore.UpdateSettings(t.Context(), user.ID.String(), core.UserSettings{
		RenewalDays:       90,
		ReminderFrequency: "weekly",
		LastReminderSent:  time.Now().Add(-1 * time.Hour), // sent 1 hour ago
	}); err != nil {
		t.Fatalf("updating settings: %v", err)
	}

	if err := sched.checkUser(context.Background(), user); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCheckUser_DefaultSettingsSkips(t *testing.T) {
	user := newTestUser()
	sched, _ := newTestScheduler(t)

	if err := sched.checkUser(context.Background(), user); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestBuildEmail_FormatsCorrectly(t *testing.T) {
	matches := []upcomingContract{
		{
			contract:         Contract{Name: "Phone Plan", Company: "Telco Inc"},
			cancellationDate: "2025-08-15",
		},
		{
			contract:         Contract{Name: "Gym Membership"},
			cancellationDate: "2025-09-01",
		},
	}

	body := buildEmail(matches)

	if !strings.Contains(body, "Phone Plan (Telco Inc)") {
		t.Errorf("expected body to contain 'Phone Plan (Telco Inc)', got:\n%s", body)
	}
	if !strings.Contains(body, "cancellation by 2025-08-15") {
		t.Errorf("expected body to contain cancellation date 2025-08-15")
	}
	if !strings.Contains(body, "- Gym Membership — cancellation by 2025-09-01") {
		t.Errorf("expected body to contain Gym Membership without company, got:\n%s", body)
	}
	if strings.Contains(body, "Gym Membership (") {
		t.Errorf("Gym Membership should not have company parentheses")
	}
}

func TestBuildEmail_SingleContract(t *testing.T) {
	matches := []upcomingContract{
		{
			contract:         Contract{Name: "Insurance", Company: "ACME"},
			cancellationDate: "2025-07-01",
		},
	}

	body := buildEmail(matches)
	lines := strings.Split(body, "\n")

	found := false
	for _, line := range lines {
		if strings.HasPrefix(line, "- Insurance") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected line starting with '- Insurance', got:\n%s", body)
	}
}
