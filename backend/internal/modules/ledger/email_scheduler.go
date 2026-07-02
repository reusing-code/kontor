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

func accountDue(account LedgerEmailAccount, now time.Time, interval time.Duration) bool {
	return account.LastScanAt == nil || now.Sub(*account.LastScanAt) >= interval
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

	password, err := cryptoutil.DecryptString(account.EncryptedPassword, s.encryptionKey)
	if err != nil {
		s.logger.Error("decrypting email account password", "userID", userID, "accountID", account.ID, "error", err)
		s.finishScan(ctx, userID, account, fmt.Sprintf("Background scan failed: %v", err))
		return
	}
	result, err := s.service.ScanMailbox(scanCtx, userID, account, password)
	if err != nil {
		s.logger.Error("background mailbox scan failed", "userID", userID, "accountID", account.ID, "error", err)
		s.finishScan(ctx, userID, account, fmt.Sprintf("Background scan failed: %v", err))
		return
	}
	s.logger.Info("background mailbox scan finished",
		"userID", userID, "accountID", account.ID,
		"emailsScanned", result.EmailsScanned, "ordersNew", result.OrdersNew, "ordersLinked", result.OrdersLinked)
	s.finishScan(ctx, userID, account, scanStatusMessage(result))
}

func scanStatusMessage(result LedgerEmailScanResult) string {
	return fmt.Sprintf("Background scan: %d emails scanned, %d new orders, %d linked",
		result.EmailsScanned, result.OrdersNew, result.OrdersLinked)
}

// finishScan stamps LastScanAt even on failure so a broken account is retried
// once per interval instead of on every tick.
func (s *EmailScanScheduler) finishScan(ctx context.Context, userID string, account LedgerEmailAccount, status string) {
	now := time.Now().UTC()
	account.LastScanAt = &now
	account.LastScanStatusMessage = status
	account.UpdatedAt = now
	if err := s.store.UpdateLedgerEmailAccount(ctx, userID, account); err != nil {
		s.logger.Error("updating email account after scan", "userID", userID, "accountID", account.ID, "error", err)
	}
}
