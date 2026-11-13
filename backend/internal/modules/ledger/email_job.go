package ledger

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/reusing-code/kontor/backend/internal/cryptoutil"
)

const (
	EmailScanStateIdle    = "idle"
	EmailScanStateRunning = "running"
	EmailScanStateDone    = "done"
	EmailScanStatePartial = "partial"
	EmailScanStateFailed  = "failed"

	EmailScanTriggerManual    = "manual"
	EmailScanTriggerScheduled = "scheduled"
	EmailScanTriggerUpload    = "upload"

	scanTimeLimitWarning = "scan stopped at the time limit"
)

// emailScanResumeDelay is how long after a partial scan (time limit, repeated
// LLM failures) the scheduler continues it. A variable so tests can shorten it.
var emailScanResumeDelay = 5 * time.Minute

// LedgerEmailScanStatus is the scan of an account as kept in memory: the
// running one, or the last finished one since the server started.
type LedgerEmailScanStatus struct {
	State         string     `json:"state"`
	Trigger       string     `json:"trigger,omitempty"`
	StartedAt     *time.Time `json:"startedAt,omitempty"`
	FinishedAt    *time.Time `json:"finishedAt,omitempty"`
	EmailsScanned int        `json:"emailsScanned"`
	OrdersNew     int        `json:"ordersNew"`
	OrdersLinked  int        `json:"ordersLinked"`
	Skipped       int        `json:"skipped"`
	Warnings      []string   `json:"warnings"`
	Error         string     `json:"error,omitempty"`
	// Resumes is set when the scheduler continues a partial scan shortly.
	Resumes bool `json:"resumes,omitempty"`
}

func (st *LedgerEmailScanStatus) setCounts(result LedgerEmailScanResult) {
	st.EmailsScanned = result.EmailsScanned
	st.OrdersNew = result.OrdersNew
	st.OrdersLinked = result.OrdersLinked
	st.Skipped = result.Skipped
	st.Warnings = append([]string{}, result.Warnings...)
}

// SetBackgroundContext bounds manual scan jobs by the server's lifetime.
func (s *EmailService) SetBackgroundContext(ctx context.Context) {
	s.scanMu.Lock()
	defer s.scanMu.Unlock()
	s.baseCtx = ctx
}

// ScanStatus returns the running or last finished scan of an account.
func (s *EmailService) ScanStatus(id uuid.UUID) LedgerEmailScanStatus {
	s.scanMu.Lock()
	defer s.scanMu.Unlock()
	if st, ok := s.scans[id]; ok {
		st.Warnings = slices.Clone(st.Warnings)
		return st
	}
	return LedgerEmailScanStatus{State: EmailScanStateIdle, Warnings: []string{}}
}

// beginScan takes the account's scan slot. One scan per account at a time, so
// a manual and a scheduled scan cannot extract the same messages twice.
func (s *EmailService) beginScan(id uuid.UUID, trigger string) (LedgerEmailScanStatus, bool) {
	s.scanMu.Lock()
	defer s.scanMu.Unlock()
	if s.scans[id].State == EmailScanStateRunning {
		return LedgerEmailScanStatus{}, false
	}
	now := time.Now().UTC()
	st := LedgerEmailScanStatus{State: EmailScanStateRunning, Trigger: trigger, StartedAt: &now, Warnings: []string{}}
	s.scans[id] = st
	return st, true
}

// publishScan makes the progress of a running scan visible.
func (s *EmailService) publishScan(id uuid.UUID, result *LedgerEmailScanResult) {
	s.scanMu.Lock()
	defer s.scanMu.Unlock()
	st, ok := s.scans[id]
	if !ok || st.State != EmailScanStateRunning {
		return
	}
	st.setCounts(*result)
	s.scans[id] = st
}

// endScan records the outcome and frees the account's scan slot.
func (s *EmailService) endScan(ctx context.Context, id uuid.UUID, result LedgerEmailScanResult, err error) {
	timedOut := scanTimedOut(ctx, err)
	s.scanMu.Lock()
	defer s.scanMu.Unlock()
	st := s.scans[id]
	now := time.Now().UTC()
	st.FinishedAt = &now
	st.setCounts(result)
	switch {
	case timedOut:
		st.State = EmailScanStatePartial
		if !slices.Contains(st.Warnings, scanTimeLimitWarning) {
			st.Warnings = append(st.Warnings, scanTimeLimitWarning)
		}
	case err != nil:
		st.State = EmailScanStateFailed
		st.Error = scanErrorText(err)
	case !result.Complete():
		st.State = EmailScanStatePartial
	default:
		st.State = EmailScanStateDone
	}
	st.Resumes = s.autoResume && st.Trigger != EmailScanTriggerUpload && scanResumesSoon(result, err, timedOut)
	s.scans[id] = st
}

// scanTimedOut reports whether the scan ended at its own time limit.
func scanTimedOut(ctx context.Context, err error) bool {
	return err != nil && errors.Is(err, context.DeadlineExceeded) && ctx.Err() != nil
}

// scanResumesSoon reports whether a scan stopped with work left that a prompt
// rerun can pick up: at the time limit or after repeated LLM failures.
func scanResumesSoon(result LedgerEmailScanResult, err error, timedOut bool) bool {
	return timedOut || (err == nil && result.Partial)
}

// scanErrorText is the failure as shown to the user; unexpected errors stay in
// the log.
func scanErrorText(err error) string {
	switch {
	case errors.Is(err, ErrIMAP), errors.Is(err, ErrEmailLLMDisabled), errors.Is(err, ErrEmailFolderMissing):
		return err.Error()
	case errors.Is(err, context.Canceled):
		return "scan interrupted"
	default:
		return "internal error"
	}
}

// StartScan starts an IMAP scan of the account as a background job and
// returns its initial status. The job is bounded by emailScanTimeout and the
// server's lifetime, not by the request that started it.
func (s *EmailService) StartScan(userID string, account LedgerEmailAccount, encryptionKey []byte) (LedgerEmailScanStatus, error) {
	if err := s.CanScan(account, true); err != nil {
		return LedgerEmailScanStatus{}, err
	}
	if account.EncryptedPassword == "" {
		return LedgerEmailScanStatus{}, ErrEmailPasswordMissing
	}
	password, err := cryptoutil.DecryptString(account.EncryptedPassword, encryptionKey)
	if err != nil {
		return LedgerEmailScanStatus{}, fmt.Errorf("decrypting email password: %w", err)
	}
	status, ok := s.beginScan(account.ID, EmailScanTriggerManual)
	if !ok {
		return LedgerEmailScanStatus{}, ErrEmailScanRunning
	}
	s.scanMu.Lock()
	base := s.baseCtx
	s.scanMu.Unlock()
	go func() {
		ctx, cancel := context.WithTimeout(base, emailScanTimeout)
		defer cancel()
		_, _ = s.runScan(ctx, userID, account, password, func(result LedgerEmailScanResult, err error) {
			s.recordManualScan(ctx, userID, account, result, err)
		})
	}()
	return status, nil
}

// runScan scans the folder while the caller holds the account's scan slot.
// record runs before the slot is freed, so whoever sees the scan finished
// also sees what record wrote.
func (s *EmailService) runScan(ctx context.Context, userID string, account LedgerEmailAccount, password string, record func(LedgerEmailScanResult, error)) (LedgerEmailScanResult, error) {
	result, err := s.scanFolder(ctx, userID, account, password)
	if record != nil {
		record(result, err)
	}
	s.endScan(ctx, account.ID, result, err)
	return result, err
}

func (s *EmailService) recordManualScan(ctx context.Context, userID string, account LedgerEmailAccount, result LedgerEmailScanResult, err error) {
	timedOut := scanTimedOut(ctx, err)
	attrs := []any{
		"userID", userID, "accountID", account.ID,
		"emailsScanned", result.EmailsScanned, "ordersNew", result.OrdersNew, "ordersLinked", result.OrdersLinked,
	}
	now := time.Now().UTC()
	state := LedgerEmailScanState{AttemptedAt: &now, Resume: scanResumesSoon(result, err, timedOut)}
	switch {
	case timedOut:
		result.Partial = true
		state.Status = manualScanStatus(account.Folder, false, result)
		s.logger.Info("manual mailbox scan timed out", attrs...)
	case err != nil && errors.Is(ctx.Err(), context.Canceled):
		s.logger.Debug("manual mailbox scan interrupted by shutdown", attrs...)
		return
	case err != nil:
		state.Status = fmt.Sprintf("Scan of folder %q failed: %s", account.Folder, scanErrorText(err))
		s.logger.Error("manual mailbox scan failed", append(attrs, "error", err)...)
	default:
		state.Status = manualScanStatus(account.Folder, false, result)
		if result.Complete() {
			state.CompletedAt = &now
		}
		s.logger.Info("manual mailbox scan finished", append(attrs, "partial", result.Partial, "skipped", result.Skipped)...)
	}
	if err := s.store.SetLedgerEmailAccountScanState(context.WithoutCancel(ctx), userID, account.ID, state); err != nil {
		s.logger.Error("updating email account after scan", "userID", userID, "accountID", account.ID, "error", err)
	}
}

func manualScanStatus(folder string, isUpload bool, result LedgerEmailScanResult) string {
	status := "Processed uploaded email messages"
	if !isUpload {
		status = fmt.Sprintf("Scanned folder %q via IMAP", folder)
	}
	if !result.Complete() {
		status += fmt.Sprintf(" (incomplete, %d warnings)", len(result.Warnings))
	}
	return status
}
