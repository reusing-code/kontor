package reminder

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"

	"github.com/reusing-code/kontor/backend/internal/email"
	"github.com/reusing-code/kontor/backend/internal/model"
	"github.com/reusing-code/kontor/backend/internal/store"
)

var frequencyDurations = map[string]time.Duration{
	"weekly":   7 * 24 * time.Hour,
	"biweekly": 14 * 24 * time.Hour,
	"monthly":  30 * 24 * time.Hour,
}

type upcomingContract struct {
	contract         model.Contract
	cancellationDate string
}

type Scheduler struct {
	store  store.Store
	email  *email.Client
	logger *slog.Logger
}

func New(s store.Store, e *email.Client, logger *slog.Logger) *Scheduler {
	return &Scheduler{store: s, email: e, logger: logger.With("component", "reminder")}
}

func (s *Scheduler) Start(ctx context.Context) {
	go func() {
		s.logger.Info("reminder scheduler started")
		s.checkAll(ctx)

		ticker := time.NewTicker(1 * time.Hour)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				s.logger.Info("reminder scheduler stopped")
				return
			case <-ticker.C:
				s.checkAll(ctx)
			}
		}
	}()
}

func (s *Scheduler) checkAll(ctx context.Context) {
	users, err := s.store.ListUsers(ctx)
	if err != nil {
		s.logger.Error("listing users for reminders", "error", err)
		return
	}

	for _, u := range users {
		if err := s.checkUser(ctx, u); err != nil {
			s.logger.Error("checking reminders for user", "userID", u.ID, "error", err)
		}
	}
}

func (s *Scheduler) checkUser(ctx context.Context, u model.User) error {
	settings, err := s.store.GetSettings(ctx, u.ID.String())
	if err != nil {
		return fmt.Errorf("getting settings: %w", err)
	}

	if settings.ReminderFrequency == "" || settings.ReminderFrequency == "disabled" {
		return nil
	}

	dur, ok := frequencyDurations[settings.ReminderFrequency]
	if !ok {
		return nil
	}

	if !settings.LastReminderSent.IsZero() && time.Since(settings.LastReminderSent) < dur {
		return nil
	}

	contracts, err := s.store.ListContracts(ctx, u.ID.String())
	if err != nil {
		return fmt.Errorf("listing contracts: %w", err)
	}

	today := time.Now().UTC().Truncate(24 * time.Hour)
	deadline := today.AddDate(0, 0, settings.RenewalDays)

	var matches []upcomingContract
	for _, c := range contracts {
		cd := c.CancellationDate()
		if cd == nil {
			continue
		}
		d, err := time.Parse("2006-01-02", *cd)
		if err != nil {
			continue
		}
		if !d.Before(today) && !d.After(deadline) {
			matches = append(matches, upcomingContract{contract: c, cancellationDate: *cd})
		}
	}

	if len(matches) == 0 {
		return nil
	}

	sort.Slice(matches, func(i, j int) bool {
		return matches[i].cancellationDate < matches[j].cancellationDate
	})

	body := buildEmail(matches)

	if err := s.email.Send([]string{u.Email}, "Upcoming contract renewals", body); err != nil {
		return fmt.Errorf("sending email: %w", err)
	}

	settings.LastReminderSent = time.Now().UTC()
	if err := s.store.UpdateSettings(ctx, u.ID.String(), settings); err != nil {
		return fmt.Errorf("updating last reminder sent: %w", err)
	}

	s.logger.Info("sent reminder email", "userID", u.ID, "contracts", len(matches))
	return nil
}

func buildEmail(matches []upcomingContract) string {
	var b strings.Builder
	b.WriteString("The following contracts have upcoming renewal deadlines:\n\n")

	for _, m := range matches {
		b.WriteString(fmt.Sprintf("- %s", m.contract.Name))
		if m.contract.Company != "" {
			b.WriteString(fmt.Sprintf(" (%s)", m.contract.Company))
		}
		b.WriteString(fmt.Sprintf(" — cancellation by %s\n", m.cancellationDate))
	}

	b.WriteString("\nPlease review these contracts and take action if needed.")
	return b.String()
}
