package ledger

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"time"

	"github.com/google/uuid"
	"github.com/reusing-code/kontor/backend/internal/cryptoutil"
	"github.com/reusing-code/kontor/backend/internal/enablebanking"
)

const bankMaxPages = 200

var (
	bankCommitChunk        = 500
	bankSyncAccountTimeout = 10 * time.Minute
	// bankAfterChunkHook runs after each committed chunk; tests use it to
	// interrupt a sync.
	bankAfterChunkHook func()
)

type BankSyncOptions struct {
	OnlyDue  bool
	Interval time.Duration
}

type BankAccountSyncResult struct {
	UID              string `json:"uid"`
	Status           string `json:"status"`
	Imported         int    `json:"imported"`
	Duplicates       int    `json:"duplicates"`
	SkippedNotBooked int    `json:"skippedNotBooked"`
	SkippedCurrency  int    `json:"skippedCurrency"`
	SkippedInvalid   int    `json:"skippedInvalid"`
}

type BankSyncSummary struct {
	Imported   int                     `json:"imported"`
	Duplicates int                     `json:"duplicates"`
	Accounts   []BankAccountSyncResult `json:"accounts"`
}

// bankAccountDue schedules by the last attempt, so a failing account keeps the
// normal interval instead of being retried every tick.
func bankAccountDue(a LedgerBankAccount, now time.Time, interval time.Duration) bool {
	return a.LastSyncAttemptAt == nil || now.Sub(*a.LastSyncAttemptAt) >= interval
}

// classifyBankError maps an upstream error to a status code, whether the
// consent is lost and whether the session status must be checked first.
func classifyBankError(err error) (status string, consentLost, checkSession bool) {
	var apiErr *enablebanking.APIError
	hasAPI := errors.As(err, &apiErr)
	switch {
	case enablebanking.IsSessionUnusable(err):
		return BankSyncConsentExpired, true, false
	case enablebanking.IsAppAuth(err):
		return BankSyncAppError, false, false
	case enablebanking.IsAccountNotAccessible(err):
		return BankSyncAccountNotAccessible, false, false
	case hasAPI && (apiErr.Status == http.StatusUnauthorized || apiErr.Status == http.StatusForbidden):
		return BankSyncAccessDenied, false, true
	case enablebanking.IsRateLimited(err):
		return BankSyncRateLimited, false, false
	case errors.Is(err, context.DeadlineExceeded):
		return BankSyncTimeout, false, false
	case enablebanking.IsUnavailable(err):
		return BankSyncBankUnavailable, false, false
	default:
		return BankSyncError, false, false
	}
}

func (s *BankService) lockSync(userID string, id uuid.UUID) (unlock func(), ok bool) {
	key := userID + "/" + id.String()
	s.syncMu.Lock()
	defer s.syncMu.Unlock()
	if s.syncing[key] {
		return nil, false
	}
	s.syncing[key] = true
	return func() {
		s.syncMu.Lock()
		delete(s.syncing, key)
		s.syncMu.Unlock()
	}, true
}

// SyncConnection imports new transactions of the mapped accounts of one
// connection. One sync per connection runs at a time. An expired or lost
// consent is never used (ErrBankConsentExpired, no upstream call). When a
// sync loses the consent, the remaining accounts appear in the summary as
// consent_expired without a call and without a recorded attempt. When the
// context ends mid-run (shutdown, request timeout) the error is returned
// together with the partial summary, and rows already imported still trigger
// the post-import hook.
func (s *BankService) SyncConnection(ctx context.Context, userID string, id uuid.UUID, opt BankSyncOptions) (BankSyncSummary, error) {
	if !s.active() {
		return BankSyncSummary{}, ErrBankImportDisabled
	}
	unlock, ok := s.lockSync(userID, id)
	if !ok {
		return BankSyncSummary{}, ErrBankSyncRunning
	}
	defer unlock()

	conn, err := s.store.GetLedgerBankConnection(ctx, userID, id)
	if err != nil {
		return BankSyncSummary{}, err
	}
	if BankConnectionStatus(conn, s.now()) == BankStatusExpired {
		return BankSyncSummary{}, ErrBankConsentExpired
	}
	if conn, err = s.resolveMappings(ctx, userID, conn); err != nil {
		return BankSyncSummary{}, err
	}

	summary := BankSyncSummary{Accounts: []BankAccountSyncResult{}}
	defer func() {
		if summary.Imported > 0 && s.afterImport != nil {
			s.afterImport(userID)
		}
	}()
	lost := false
	for _, account := range conn.Accounts {
		if opt.OnlyDue && !bankAccountDue(account, s.now(), opt.Interval) {
			continue
		}
		if lost {
			summary.Accounts = append(summary.Accounts, BankAccountSyncResult{UID: account.UID, Status: BankSyncConsentExpired})
			continue
		}
		if account.LedgerAccountID == nil {
			summary.Accounts = append(summary.Accounts, BankAccountSyncResult{UID: account.UID, Status: BankSyncUnmapped})
			continue
		}
		res, consentLost, err := s.syncAccount(ctx, userID, conn, account)
		summary.Imported += res.Imported
		summary.Duplicates += res.Duplicates
		if err != nil {
			return summary, err
		}
		lost = consentLost
		summary.Accounts = append(summary.Accounts, res)
	}
	return summary, nil
}

type bankFetchStats struct {
	notBooked, currency, invalid int
}

// syncAccount returns an error only when the parent context ended (shutdown or
// request timeout). Nothing is recorded then, but the result still carries what
// was imported before the interruption.
func (s *BankService) syncAccount(ctx context.Context, userID string, conn LedgerBankConnection, account LedgerBankAccount) (BankAccountSyncResult, bool, error) {
	result := BankAccountSyncResult{UID: account.UID}
	started := s.now()

	accountCtx, cancel := context.WithTimeout(ctx, bankSyncAccountTimeout)
	defer cancel()

	stats, imported, dupes, err := s.fetchAndCommit(accountCtx, userID, conn, account)
	if ctx.Err() != nil {
		result.Imported, result.Duplicates = imported, dupes
		return result, false, ctx.Err()
	}
	result.SkippedNotBooked, result.SkippedCurrency, result.SkippedInvalid = stats.notBooked, stats.currency, stats.invalid

	status := BankSyncOK
	consentLost := false
	if err != nil {
		var checkSession bool
		status, consentLost, checkSession = classifyBankError(err)
		if checkSession {
			consentLost = s.sessionLost(ctx, conn)
			if consentLost {
				status = BankSyncConsentExpired
			}
		}
		s.logger.Warn("bank account sync failed", "user_id", userID, "connection_id", conn.ID,
			"account_uid", account.UID, "status", status, "upstream", bankErrCode(err))
	}
	result.Status, result.Imported, result.Duplicates = status, imported, dupes

	s.recordSyncState(ctx, userID, conn, account.UID, result, started, consentLost)
	if err == nil {
		s.logger.Info("bank account synced", "user_id", userID, "connection_id", conn.ID, "account_uid", account.UID,
			"imported", imported, "duplicates", dupes, "skipped_not_booked", stats.notBooked,
			"skipped_currency", stats.currency, "skipped_invalid", stats.invalid)
	}
	return result, consentLost, nil
}

func (s *BankService) sessionLost(ctx context.Context, conn LedgerBankConnection) bool {
	sessionID, err := cryptoutil.DecryptString(conn.EncryptedSessionID, s.key)
	if err != nil {
		return false
	}
	checkCtx, cancel := context.WithTimeout(ctx, bankSessionTimeout)
	defer cancel()
	status, err := s.client.SessionStatus(checkCtx, sessionID)
	if err != nil {
		return enablebanking.IsSessionUnusable(err)
	}
	return status.Status != "AUTHORIZED"
}

// recordSyncState writes the outcome for one account. It is dropped when the
// connection was re-authorized while the sync ran (the stored session differs
// from the one the sync used): the old session's failure must not mark the new
// consent as lost.
func (s *BankService) recordSyncState(ctx context.Context, userID string, conn LedgerBankConnection, uid string, res BankAccountSyncResult, at time.Time, consentLost bool) {
	id := conn.ID
	stale := false
	_, err := s.store.ModifyLedgerBankConnection(context.WithoutCancel(ctx), userID, id, func(c *LedgerBankConnection) {
		if c.EncryptedSessionID != conn.EncryptedSessionID {
			stale = true
			return
		}
		if consentLost && c.LostAt == nil {
			c.LostAt = &at
		}
		for i := range c.Accounts {
			a := &c.Accounts[i]
			if a.UID != uid {
				continue
			}
			a.LastSyncAttemptAt = &at
			a.LastSyncStatus = res.Status
			if res.Status == BankSyncOK {
				a.LastSyncAt = &at
				a.LastImported = res.Imported
				a.LastDuplicates = res.Duplicates
			}
		}
	})
	if err != nil {
		s.logger.Error("recording bank sync state failed", "user_id", userID, "connection_id", id, "error", err)
	}
	if stale {
		s.logger.Info("bank sync state dropped, connection was re-authorized meanwhile", "user_id", userID, "connection_id", id)
	}
}

// fetchAndCommit fetches every page first (nothing is stored when one fails)
// and commits oldest first in chunks, so an interrupted run can never leave a
// newer stored row with older unfetched ones behind it.
func (s *BankService) fetchAndCommit(ctx context.Context, userID string, conn LedgerBankConnection, account LedgerBankAccount) (bankFetchStats, int, int, error) {
	var stats bankFetchStats
	ledgerAccountID := *account.LedgerAccountID

	newest, any, err := s.store.LedgerAccountBookingRange(ctx, userID, ledgerAccountID, s.now().UTC().Format(time.DateOnly))
	if err != nil {
		return stats, 0, 0, err
	}
	window := BankSyncWindow(newest, any, s.now())
	fetched, err := s.fetchTransactions(ctx, account.UID, window)
	if err != nil {
		return stats, 0, 0, err
	}

	rows := make([]ParsedRow, 0, len(fetched))
	for _, t := range fetched {
		row, skip := RowFromBankTransaction(t)
		switch skip {
		case BankSkipNone:
			rows = append(rows, row)
		case BankSkipNotBooked:
			stats.notBooked++
		case BankSkipCurrency:
			stats.currency++
		default:
			stats.invalid++
		}
	}

	categories, err := s.store.ListLedgerCategories(ctx, userID)
	if err != nil {
		return stats, 0, 0, err
	}
	now := s.now().UTC()
	batchID := uuid.New()
	fingerprints := BankFingerprints(ledgerAccountID, rows)
	txns := make([]LedgerTransaction, len(rows))
	for i, row := range rows {
		txns[i] = newLedgerTransactionFromRow(row, ledgerAccountID, batchID, SourceEnableBanking, fingerprints[i], categories, now)
	}
	sort.SliceStable(txns, func(i, j int) bool { return txns[i].BookingDate < txns[j].BookingDate })

	batch := LedgerImportBatch{
		ID: batchID, AccountID: ledgerAccountID, SourceType: string(SourceEnableBanking), ParserVersion: ParserVersion,
		Status: ImportStatusCommitted, CreatedAt: now, UpdatedAt: now,
	}
	if stats != (bankFetchStats{}) {
		batch.Warnings = []string{fmt.Sprintf("skipped %d not booked, %d other currency, %d invalid", stats.notBooked, stats.currency, stats.invalid)}
	}

	imported, duplicates := 0, 0
	for start := 0; start < len(txns); start += bankCommitChunk {
		if err := ctx.Err(); err != nil {
			return stats, imported, duplicates, err
		}
		end := min(start+bankCommitChunk, len(txns))
		res, err := s.store.CommitLedgerSync(ctx, userID, batch, txns[start:end])
		if err != nil {
			return stats, imported, duplicates, err
		}
		imported += res.Imported
		duplicates += res.Duplicates + res.CrossSourceDuplicates
		if bankAfterChunkHook != nil {
			bankAfterChunkHook()
		}
	}
	return stats, imported, duplicates, nil
}

func (s *BankService) fetchTransactions(ctx context.Context, uid string, w BankWindow) ([]enablebanking.Transaction, error) {
	q := enablebanking.TransactionsQuery{AccountUID: uid, DateFrom: w.DateFrom, DateTo: w.DateTo}
	if w.Longest {
		q.Strategy = "longest"
	}
	all, err := s.fetchPages(ctx, q)
	if err != nil && enablebanking.IsWrongTransactionsPeriod(err) && !w.Longest {
		q.Strategy, q.DateTo = "longest", ""
		all, err = s.fetchPages(ctx, q)
	}
	return all, err
}

func (s *BankService) fetchPages(ctx context.Context, q enablebanking.TransactionsQuery) ([]enablebanking.Transaction, error) {
	return s.fetchPagesWith(ctx, q, s.client.Transactions)
}

// fetchPagesWith follows continuation keys; a repeated key or more than
// bankMaxPages pages aborts.
func (s *BankService) fetchPagesWith(ctx context.Context, q enablebanking.TransactionsQuery, next func(context.Context, enablebanking.TransactionsQuery) (enablebanking.TransactionsPage, error)) ([]enablebanking.Transaction, error) {
	var all []enablebanking.Transaction
	seen := map[string]bool{}
	for range bankMaxPages {
		page, err := next(ctx, q)
		if err != nil {
			return nil, err
		}
		all = append(all, page.Transactions...)
		if page.ContinuationKey == "" {
			return all, nil
		}
		if seen[page.ContinuationKey] {
			return nil, errors.New("repeated continuation key")
		}
		seen[page.ContinuationKey] = true
		q.ContinuationKey = page.ContinuationKey
	}
	return nil, errors.New("too many transaction pages")
}
