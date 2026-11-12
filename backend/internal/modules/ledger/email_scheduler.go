package ledger

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/reusing-code/kontor/backend/internal/core"
	"github.com/reusing-code/kontor/backend/internal/cryptoutil"
)

const accountScanTimeout = 10 * time.Minute

type EmailScanScheduler struct {
	store         *Store
	core          *core.Store
	service       *EmailService
	encryptionKey []byte
	interval      time.Duration
	logger        *slog.Logger
}

func NewEmailScanScheduler(s *Store, coreStore *core.Store, service *EmailService, encryptionKey []byte, interval time.Duration, logger *slog.Logger) *EmailScanScheduler {
	return &EmailScanScheduler{
		store:         s,
		core:          coreStore,
		service:       service,
		encryptionKey: encryptionKey,
		interval:      interval,
		logger:        logger.With("component", "ledger-email-scan"),
	}
}

func (s *EmailScanScheduler) Start(ctx context.Context) {
	go func() {
		s.logger.Info("ledger email scan scheduler started", "interval", s.interval)
		s.scanAll(ctx)

		ticker := time.NewTicker(schedulerTickInterval(s.interval))
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				s.logger.Info("ledger email scan scheduler stopped")
				return
			case <-ticker.C:
				s.scanAll(ctx)
			}
		}
	}()
}

// schedulerTickInterval checks more often than the scan interval so accounts
// become due close to their actual deadline, mirroring the reminder scheduler.
func schedulerTickInterval(interval time.Duration) time.Duration {
	if interval < time.Hour {
		return interval
	}
	return time.Hour
}

// accountDue is based on the last scan attempt, so a failing account keeps the
// normal interval instead of being retried every tick. Accounts from before the
// attempt timestamp existed fall back to their last complete scan. Skipped
// accounts (no LLM, no folder) record no attempt and stay due, which costs nothing.
func accountDue(account LedgerEmailAccount, now time.Time, interval time.Duration) bool {
	last := account.LastScanAttemptAt
	if last == nil {
		last = account.LastScanAt
	}
	return last == nil || now.Sub(*last) >= interval
}

func (s *EmailScanScheduler) scanAll(ctx context.Context) {
	users, err := s.core.ListUsers(ctx)
	if err != nil {
		s.logger.Error("listing users for email scans", "error", err)
		return
	}
	for _, u := range users {
		if ctx.Err() != nil {
			return
		}
		if err := s.scanUser(ctx, u.ID.String()); err != nil {
			s.logger.Error("scanning ledger email accounts", "userID", u.ID, "error", err)
		}
	}
}

func (s *EmailScanScheduler) scanUser(ctx context.Context, userID string) error {
	enabled, err := s.core.ModuleEnabled(ctx, userID, ModuleID)
	if err != nil {
		return fmt.Errorf("checking module enablement: %w", err)
	}
	if !enabled {
		return nil
	}

	accounts, err := s.store.ListLedgerEmailAccounts(ctx, userID)
	if err != nil {
		return fmt.Errorf("listing email accounts: %w", err)
	}
	now := time.Now().UTC()
	for _, account := range accounts {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if !accountDue(account, now, s.interval) {
			continue
		}
		s.scanAccount(ctx, userID, account)
	}
	return nil
}

func (s *EmailScanScheduler) scanAccount(ctx context.Context, userID string, account LedgerEmailAccount) {
	scanCtx, cancel := context.WithTimeout(ctx, accountScanTimeout)
	defer cancel()

	if account.Folder == "" {
		s.logger.Debug("skipping email account without folder", "userID", userID, "accountID", account.ID)
		s.finishScan(ctx, userID, account, "Background scan skipped: choose a folder to scan", false, false)
		return
	}
	if account.EncryptedPassword == "" {
		s.logger.Warn("skipping email account without stored password", "userID", userID, "accountID", account.ID)
		s.finishScan(ctx, userID, account, "Background scan skipped: no stored password, re-enter the account password", true, false)
		return
	}
	password, err := cryptoutil.DecryptString(account.EncryptedPassword, s.encryptionKey)
	if err != nil {
		s.logger.Error("decrypting email account password", "userID", userID, "accountID", account.ID, "error", err)
		s.finishScan(ctx, userID, account, fmt.Sprintf("Background scan failed: %v", err), true, false)
		return
	}
	result, err := s.service.ScanMailbox(scanCtx, userID, account, password)
	if err != nil {
		s.logger.Error("background mailbox scan failed", "userID", userID, "accountID", account.ID, "error", err)
		s.finishScan(ctx, userID, account, fmt.Sprintf("Background scan failed: %v", err), true, false)
		return
	}
	s.logger.Info("background mailbox scan finished",
		"userID", userID, "accountID", account.ID,
		"emailsScanned", result.EmailsScanned, "ordersNew", result.OrdersNew, "ordersLinked", result.OrdersLinked)
	s.finishScan(ctx, userID, account, scanStatusMessage(account.Folder, result), true, true)
}

func scanStatusMessage(folder string, result LedgerEmailScanResult) string {
	return fmt.Sprintf("Background scan of folder %q: %d emails scanned, %d new orders, %d linked",
		folder, result.EmailsScanned, result.OrdersNew, result.OrdersLinked)
}

// finishScan records the scan outcome on the account. An attempted scan
// schedules the next one after the interval; only a complete one moves
// LastScanAt, the anchor of the next lookback. Skips (neither) stay due and
// only a changed status message is written.
func (s *EmailScanScheduler) finishScan(ctx context.Context, userID string, account LedgerEmailAccount, status string, attempted, complete bool) {
	state := LedgerEmailScanState{Status: status}
	now := time.Now().UTC()
	if attempted {
		state.AttemptedAt = &now
	}
	if complete {
		state.CompletedAt = &now
	}
	if err := s.store.SetLedgerEmailAccountScanState(ctx, userID, account.ID, state); err != nil {
		s.logger.Error("updating email account after scan", "userID", userID, "accountID", account.ID, "error", err)
	}
}
