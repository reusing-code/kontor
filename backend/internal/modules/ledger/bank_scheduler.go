package ledger

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/reusing-code/kontor/backend/internal/core"
	"github.com/reusing-code/kontor/backend/internal/storage"
)

// BankMailer sends the expiry reminders; satisfied by *email.Client.
type BankMailer interface {
	IsConfigured() bool
	Send(to []string, subject, body string) error
}

// BankUserSource lists users and their module settings; satisfied by *core.Store.
type BankUserSource interface {
	ListUsers(ctx context.Context) ([]core.User, error)
	ModuleEnabled(ctx context.Context, userID, moduleID string) (bool, error)
}

type BankSyncSchedulerConfig struct {
	Service   *BankService
	Store     *Store
	Users     BankUserSource
	Mailer    BankMailer
	PublicURL string
	// Interval between automatic syncs of an account; 0 disables the sync only.
	Interval time.Duration
	Now      func() time.Time
	Logger   *slog.Logger
}

// BankSyncScheduler syncs due bank accounts, mails consent expiry reminders
// and prunes stale authorization states.
type BankSyncScheduler struct {
	service   *BankService
	store     *Store
	users     BankUserSource
	mailer    BankMailer
	publicURL string
	interval  time.Duration
	now       func() time.Time
	logger    *slog.Logger
}

func NewBankSyncScheduler(cfg BankSyncSchedulerConfig) *BankSyncScheduler {
	s := &BankSyncScheduler{
		service: cfg.Service, store: cfg.Store, users: cfg.Users, mailer: cfg.Mailer,
		publicURL: cfg.PublicURL, interval: cfg.Interval, now: cfg.Now, logger: cfg.Logger,
	}
	if s.now == nil {
		s.now = time.Now
	}
	if s.logger == nil {
		s.logger = slog.New(slog.DiscardHandler)
	}
	s.logger = s.logger.With("component", "ledger-bank-sync")
	return s
}

// Start runs the scheduler in the background until ctx ends.
func (s *BankSyncScheduler) Start(ctx context.Context) {
	go s.Run(ctx)
}

// Run does an immediate pass, then one per tick, until ctx ends. The tick is
// hourly when the interval is 0 so reminders and pruning keep working.
func (s *BankSyncScheduler) Run(ctx context.Context) {
	s.logger.Info("bank sync scheduler started", "interval", s.interval)
	s.runPass(ctx)

	tick := time.Hour
	if s.interval > 0 {
		tick = schedulerTickInterval(s.interval)
	}
	ticker := time.NewTicker(tick)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			s.logger.Info("bank sync scheduler stopped")
			return
		case <-ticker.C:
			s.runPass(ctx)
		}
	}
}

func (s *BankSyncScheduler) runPass(ctx context.Context) {
	users, err := s.users.ListUsers(ctx)
	if err != nil {
		s.logger.Error("listing users for bank sync", "error", err)
		return
	}
	for _, u := range users {
		if ctx.Err() != nil {
			return
		}
		if err := s.runUser(ctx, u); err != nil && ctx.Err() == nil {
			s.logger.Error("bank sync pass failed", "user_id", u.ID, "error", err)
		}
	}
}

func (s *BankSyncScheduler) runUser(ctx context.Context, u core.User) error {
	userID := u.ID.String()
	enabled, err := s.users.ModuleEnabled(ctx, userID, ModuleID)
	if err != nil {
		return fmt.Errorf("checking module enablement: %w", err)
	}
	if !enabled {
		return nil
	}
	if _, err := s.store.PruneLedgerBankAuthStates(ctx, userID, s.now()); err != nil {
		s.logger.Error("pruning bank auth states failed", "user_id", userID, "error", err)
	}
	conns, err := s.store.ListLedgerBankConnections(ctx, userID)
	if err != nil {
		return fmt.Errorf("listing bank connections: %w", err)
	}
	for _, c := range conns {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if BankConnectionStatus(c, s.now()) == BankStatusExpired {
			continue
		}
		s.syncConnection(ctx, userID, c)
		s.remind(ctx, u, c)
	}
	return nil
}

func (s *BankSyncScheduler) syncConnection(ctx context.Context, userID string, c LedgerBankConnection) {
	if s.interval <= 0 {
		return
	}
	summary, err := s.service.SyncConnection(ctx, userID, c.ID, BankSyncOptions{OnlyDue: true, Interval: s.interval})
	switch {
	case err == nil:
		if len(summary.Accounts) > 0 {
			s.logger.Info("bank connection synced", "user_id", userID, "connection_id", c.ID,
				"accounts", len(summary.Accounts), "imported", summary.Imported, "duplicates", summary.Duplicates)
		}
	case errors.Is(err, ErrBankSyncRunning), errors.Is(err, ErrBankConsentExpired), ctx.Err() != nil:
	default:
		s.logger.Error("bank connection sync failed", "user_id", userID, "connection_id", c.ID, "error", err)
	}
}

// remind sends the due expiry mail. Without SMTP nothing is sent or marked, so
// the mail goes out once SMTP exists; a send error is retried at the next tick.
func (s *BankSyncScheduler) remind(ctx context.Context, u core.User, c LedgerBankConnection) {
	userID := u.ID.String()
	// the sync may have changed the connection (lost consent)
	fresh, err := s.store.GetLedgerBankConnection(ctx, userID, c.ID)
	if err != nil {
		if !errors.Is(err, storage.ErrNotFound) {
			s.logger.Error("reloading bank connection failed", "user_id", userID, "connection_id", c.ID, "error", err)
		}
		return
	}
	if BankConnectionStatus(fresh, s.now()) == BankStatusExpired {
		return
	}
	days, mark, ok := dueExpiryReminder(fresh, s.now())
	if !ok {
		return
	}
	if s.mailer == nil || !s.mailer.IsConfigured() {
		s.logger.Debug("bank expiry reminder skipped", "user_id", userID, "connection_id", c.ID, "reason", "SMTP is not configured")
		return
	}
	subject, body := bankExpiryMail(fresh.ASPSPName, fresh.ValidUntil, s.publicURL)
	if err := s.mailer.Send([]string{u.Email}, subject, body); err != nil {
		s.logger.Error("sending bank expiry reminder failed", "user_id", userID, "connection_id", c.ID, "days", days)
		return
	}
	if err := s.markNotified(ctx, userID, c.ID, mark); err != nil {
		s.logger.Error("recording bank expiry reminder failed", "user_id", userID, "connection_id", c.ID, "error", err)
		return
	}
	s.logger.Info("bank expiry reminder sent", "user_id", userID, "connection_id", c.ID, "days", days)
}

func (s *BankSyncScheduler) markNotified(ctx context.Context, userID string, id uuid.UUID, mark []int) error {
	_, err := s.store.ModifyLedgerBankConnection(ctx, userID, id, func(c *LedgerBankConnection) {
		for _, d := range mark {
			if !slices.Contains(c.NotifiedDays, d) {
				c.NotifiedDays = append(c.NotifiedDays, d)
			}
		}
		slices.Sort(c.NotifiedDays)
	})
	return err
}
