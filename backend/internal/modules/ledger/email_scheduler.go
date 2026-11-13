package ledger

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/reusing-code/kontor/backend/internal/core"
	"github.com/reusing-code/kontor/backend/internal/cryptoutil"
)

// emailScanTimeout bounds one account scan, scheduled or manual. A variable so
// tests can shorten it.
var emailScanTimeout = 10 * time.Minute

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

		ticker := time.NewTicker(emailSchedulerTick(s.interval))
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

// emailSchedulerTick also bounds how late a partial scan is resumed.
func emailSchedulerTick(interval time.Duration) time.Duration {
	return min(schedulerTickInterval(interval), emailScanResumeDelay)
}

// accountDue is based on the last scan attempt, so a failing account keeps the
// normal interval instead of being retried every tick. Accounts from before the
// attempt timestamp existed fall back to their last complete scan. Skipped
// accounts (no LLM, no folder) record no attempt and stay due, which costs nothing.
// A partial scan is continued after emailScanResumeDelay.
func accountDue(account LedgerEmailAccount, now time.Time, interval time.Duration) bool {
	last := account.LastScanAttemptAt
	if last == nil {
		last = account.LastScanAt
	}
	if account.ScanResumePending {
		interval = min(interval, emailScanResumeDelay)
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
	scanCtx, cancel := context.WithTimeout(ctx, emailScanTimeout)
	defer cancel()

	if err := s.service.CanScan(account, true); err != nil {
		s.skipScan(ctx, userID, account, err)
		return
	}
	if account.EncryptedPassword == "" {
		s.logger.Warn("skipping email account without stored password", "userID", userID, "accountID", account.ID)
		s.finishScan(ctx, userID, account, "Background scan skipped: no stored password, re-enter the account password", true, false, false)
		return
	}
	password, err := cryptoutil.DecryptString(account.EncryptedPassword, s.encryptionKey)
	if err != nil {
		s.logger.Error("decrypting email account password", "userID", userID, "accountID", account.ID, "error", err)
		s.finishScan(ctx, userID, account, fmt.Sprintf("Background scan failed: %v", err), true, false, false)
		return
	}
	// The outcome is recorded while the scan slot is held, so a client that
	// sees the scan finished reads the updated account.
	_, err = s.service.scanMailbox(scanCtx, userID, account, password, func(result LedgerEmailScanResult, err error) {
		switch {
		case errors.Is(err, ErrEmailLLMDisabled):
			s.skipScan(ctx, userID, account, err)
			return
		case err != nil && ctx.Err() != nil:
			s.logger.Debug("email scan interrupted by shutdown", "userID", userID, "accountID", account.ID)
			return
		}
		status, complete := scanOutcome(account.Folder, result, err, scanCtx.Err() != nil)
		s.logScanOutcome(userID, account, result, err, complete)
		s.finishScan(ctx, userID, account, status, true, complete, scanResumesSoon(result, err, scanTimedOut(scanCtx, err)))
	})
	if errors.Is(err, ErrEmailScanRunning) {
		s.logger.Debug("email account is already being scanned", "userID", userID, "accountID", account.ID)
	}
}

// scanOutcome decides the status message and whether the scan was complete.
// Only a complete scan advances LastScanAt: the next scan searches from there
// (minus a 48 hour overlap), so advancing after an incomplete one would
// skip mail.
func scanOutcome(folder string, result LedgerEmailScanResult, err error, timedOut bool) (status string, complete bool) {
	switch {
	case err != nil && timedOut && errors.Is(err, context.DeadlineExceeded):
		return fmt.Sprintf("Background scan of folder %q timed out; it continues shortly", folder), false
	case err != nil:
		return fmt.Sprintf("Background scan failed: %v", err), false
	case result.Partial:
		return fmt.Sprintf("Background scan of folder %q stopped after repeated LLM failures; it is retried shortly", folder), false
	case result.Skipped > 0:
		return fmt.Sprintf("Background scan of folder %q: %d emails scanned, %d new orders, %d linked; %d skipped and retried on the next run",
			folder, result.EmailsScanned, result.OrdersNew, result.OrdersLinked, result.Skipped), false
	default:
		return scanStatusMessage(folder, result), true
	}
}

func (s *EmailScanScheduler) logScanOutcome(userID string, account LedgerEmailAccount, result LedgerEmailScanResult, err error, complete bool) {
	attrs := []any{
		"userID", userID, "accountID", account.ID,
		"emailsScanned", result.EmailsScanned, "ordersNew", result.OrdersNew, "ordersLinked", result.OrdersLinked,
	}
	switch {
	case complete:
		s.logger.Info("background mailbox scan finished", attrs...)
	case err != nil && errors.Is(err, context.DeadlineExceeded):
		s.logger.Info("background mailbox scan timed out", attrs...)
	case err != nil:
		s.logger.Error("background mailbox scan failed", append(attrs, "error", err)...)
	default:
		s.logger.Warn("background mailbox scan incomplete", append(attrs, "partial", result.Partial, "skipped", result.Skipped)...)
	}
}

// skipScan records why an account was not scanned. The account stays due, so it
// is retried on the next tick once the cause is fixed.
func (s *EmailScanScheduler) skipScan(ctx context.Context, userID string, account LedgerEmailAccount, cause error) {
	s.logger.Debug("skipping email account scan", "userID", userID, "accountID", account.ID, "reason", cause.Error())
	s.finishScan(ctx, userID, account, "Background scan skipped: "+cause.Error(), false, false, false)
}

func scanStatusMessage(folder string, result LedgerEmailScanResult) string {
	return fmt.Sprintf("Background scan of folder %q: %d emails scanned, %d new orders, %d linked",
		folder, result.EmailsScanned, result.OrdersNew, result.OrdersLinked)
}

// finishScan records the scan outcome on the account. An attempted scan
// schedules the next one after the interval; only a complete one moves
// LastScanAt, the anchor of the next lookback. Skips (neither) stay due and
// only a changed status message is written. resume shortens the wait for the
// next scan to emailScanResumeDelay.
func (s *EmailScanScheduler) finishScan(ctx context.Context, userID string, account LedgerEmailAccount, status string, attempted, complete, resume bool) {
	state := LedgerEmailScanState{Status: status, Resume: resume}
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
