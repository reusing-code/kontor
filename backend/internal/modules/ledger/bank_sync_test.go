package ledger

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/reusing-code/kontor/backend/internal/enablebanking"
	"github.com/reusing-code/kontor/backend/internal/storage"
)

type syncFixture struct {
	*bankFixture
	ledgerID   uuid.UUID
	conn       LedgerBankConnection
	afterCalls int
}

// newSyncFixture connects DKB with one account (uid-1) mapped to a ledger account.
func newSyncFixture(t *testing.T) *syncFixture {
	t.Helper()
	f := &syncFixture{bankFixture: newBankFixture(t)}
	f.ledgerID = f.addLedgerAccount(t, bankTestIBAN1, "EUR")
	f.conn = f.connect(t, "code", bankTestSession("SESSION", bankTestAccount("uid-1", bankTestIBAN1)))
	f.svc.afterImport = func(string) { f.afterCalls++ }
	return f
}

func apiTxn(ref, date, amount, indicator string) enablebanking.Transaction {
	return enablebanking.Transaction{
		EntryReference:        ref,
		TransactionAmount:     enablebanking.Amount{Currency: "EUR", Amount: amount},
		CreditDebitIndicator:  indicator,
		Status:                "BOOK",
		BookingDate:           date,
		Creditor:              &enablebanking.Party{Name: "Shop " + ref},
		RemittanceInformation: []string{"Purpose " + ref},
	}
}

func (f *syncFixture) sync(t *testing.T) BankSyncSummary {
	t.Helper()
	s, err := f.svc.SyncConnection(t.Context(), testUserID, f.conn.ID, BankSyncOptions{})
	if err != nil {
		t.Fatalf("SyncConnection: %v", err)
	}
	return s
}

func (f *syncFixture) stored(t *testing.T) []LedgerTransaction {
	t.Helper()
	txns, err := f.ListLedgerTransactions(t.Context(), testUserID, f.ledgerID)
	if err != nil {
		t.Fatal(err)
	}
	return txns
}

func (f *syncFixture) account(t *testing.T, uid string) LedgerBankAccount {
	t.Helper()
	c, err := f.GetLedgerBankConnection(t.Context(), testUserID, f.conn.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range c.Accounts {
		if a.UID == uid {
			return a
		}
	}
	t.Fatalf("no account %s", uid)
	return LedgerBankAccount{}
}

func TestSync_FirstSyncImportsLongestHistory(t *testing.T) {
	f := newSyncFixture(t)
	pending := apiTxn("p", "2026-04-04", "1.00", "DBIT")
	pending.Status = "PDNG"
	usd := apiTxn("u", "2026-04-04", "2.00", "DBIT")
	usd.TransactionAmount.Currency = "USD"
	bad := apiTxn("b", "2026-04-04", "x", "DBIT")
	f.fake.SetTransactions("uid-1", []enablebanking.Transaction{
		apiTxn("a", "2026-04-01", "10.00", "DBIT"),
		apiTxn("b1", "2026-04-02", "20.50", "CRDT"),
		apiTxn("c", "2026-04-03", "5.00", "DBIT"),
		pending, usd, bad,
	})
	cat := f.addCategory(t, "Shops", "Shop a")

	summary := f.sync(t)
	if summary.Imported != 3 || summary.Duplicates != 0 || len(summary.Accounts) != 1 {
		t.Fatalf("summary = %+v", summary)
	}
	r := summary.Accounts[0]
	if r.Status != BankSyncOK || r.SkippedNotBooked != 1 || r.SkippedCurrency != 1 || r.SkippedInvalid != 1 {
		t.Fatalf("account result = %+v", r)
	}
	q := f.fake.TransactionQueries()
	if len(q) != 1 || q[0].Strategy != "longest" || q[0].From != "" || q[0].To != "" {
		t.Fatalf("queries = %+v", q)
	}

	txns := f.stored(t)
	if len(txns) != 3 {
		t.Fatalf("stored %d", len(txns))
	}
	categorized := 0
	for _, tx := range txns {
		if tx.SourceType != "enablebanking" || tx.ReviewStatus != LedgerTransactionReviewNeedsReview || !strings.HasPrefix(tx.Fingerprint, "eb1:") {
			t.Fatalf("txn = %+v", tx)
		}
		if tx.CategoryID != nil && *tx.CategoryID == cat && tx.CategorizationSource == LedgerCategorizationKeyword {
			categorized++
		}
	}
	if categorized != 1 {
		t.Fatalf("keyword categorized = %d, want 1", categorized)
	}

	a := f.account(t, "uid-1")
	if a.LastSyncStatus != BankSyncOK || a.LastSyncAt == nil || a.LastSyncAttemptAt == nil || a.LastImported != 3 {
		t.Fatalf("account state = %+v", a)
	}
	imports, _ := f.ListLedgerImports(t.Context(), testUserID)
	if len(imports) != 1 || imports[0].SourceType != "enablebanking" || imports[0].Filename != "" || len(imports[0].Warnings) != 1 {
		t.Fatalf("imports = %+v", imports)
	}
	if f.countKeys(t, "file_hash/") != 0 {
		t.Fatal("sync wrote a file hash")
	}
	if f.afterCalls != 1 {
		t.Fatalf("afterImport calls = %d, want 1", f.afterCalls)
	}
}

func (f *syncFixture) addCategory(t *testing.T, name, word string) uuid.UUID {
	t.Helper()
	id := uuid.New()
	if err := f.CreateLedgerCategory(t.Context(), testUserID, LedgerCategory{ID: id, Name: name, MatchWords: []string{word}, CreatedAt: f.now, UpdatedAt: f.now}); err != nil {
		t.Fatal(err)
	}
	return id
}

func TestSync_SecondRunUsesOverlapWindow(t *testing.T) {
	f := newSyncFixture(t)
	f.fake.SetTransactions("uid-1", []enablebanking.Transaction{
		apiTxn("a", "2026-04-01", "10.00", "DBIT"),
		apiTxn("b", "2026-04-07", "20.00", "DBIT"),
	})
	f.sync(t)

	second := f.sync(t)
	if second.Imported != 0 || second.Duplicates != 2 {
		t.Fatalf("second = %+v", second)
	}
	q := f.fake.TransactionQueries()
	if len(q) != 2 || q[1].Strategy != "" || q[1].From != "2026-03-24" || q[1].To != "2026-11-13" {
		t.Fatalf("queries = %+v", q)
	}
	if f.afterCalls != 1 {
		t.Fatalf("afterImport calls = %d, want 1 (none for the empty run)", f.afterCalls)
	}

	f.fake.SetTransactions("uid-1", []enablebanking.Transaction{
		apiTxn("a", "2026-04-01", "10.00", "DBIT"),
		apiTxn("b", "2026-04-07", "20.00", "DBIT"),
		apiTxn("c", "2026-04-09", "30.00", "DBIT"),
	})
	third := f.sync(t)
	if third.Imported != 1 || third.Duplicates != 2 || len(f.stored(t)) != 3 {
		t.Fatalf("third = %+v (stored %d)", third, len(f.stored(t)))
	}
	if f.afterCalls != 2 {
		t.Fatalf("afterImport calls = %d, want 2", f.afterCalls)
	}
	if a := f.account(t, "uid-1"); a.LastImported != 1 || a.LastDuplicates != 2 {
		t.Fatalf("account = %+v", a)
	}
}

func TestSync_FailureOnLaterPageStoresNothing(t *testing.T) {
	f := newSyncFixture(t)
	var all []enablebanking.Transaction
	for i := 1; i <= 5; i++ {
		all = append(all, apiTxn(fmt.Sprintf("r%d", i), fmt.Sprintf("2026-04-%02d", i), "1.00", "DBIT"))
	}
	f.fake.SetTransactions("uid-1", all)
	f.fake.SetPageSize(2)
	f.fake.FailTransactionRequest(2, 500, "")

	summary := f.sync(t)
	if summary.Imported != 0 || summary.Accounts[0].Status != BankSyncBankUnavailable {
		t.Fatalf("summary = %+v", summary)
	}
	if len(f.stored(t)) != 0 {
		t.Fatal("rows stored although a page failed")
	}
	a := f.account(t, "uid-1")
	if a.LastSyncAttemptAt == nil || a.LastSyncAt != nil || a.LastSyncStatus != BankSyncBankUnavailable {
		t.Fatalf("account = %+v", a)
	}
	if f.afterCalls != 0 {
		t.Fatal("afterImport after a failed run")
	}

	retry := f.sync(t)
	if retry.Imported != 5 || retry.Duplicates != 0 {
		t.Fatalf("retry = %+v", retry)
	}
	q := f.fake.TransactionQueries()
	if q[0].Strategy != "longest" || q[len(q)-1].Strategy != "longest" {
		t.Fatalf("anchor moved after the failed run: %+v", q)
	}
}

func TestSync_InterruptionBetweenChunksLosesNothing(t *testing.T) {
	f := newSyncFixture(t)
	var all []enablebanking.Transaction
	for i := 5; i >= 1; i-- { // newest first, as banks return them
		all = append(all, apiTxn(fmt.Sprintf("r%d", i), fmt.Sprintf("2026-04-%02d", i), fmt.Sprintf("%d.00", i), "DBIT"))
	}
	f.fake.SetTransactions("uid-1", all)

	oldChunk := bankCommitChunk
	bankCommitChunk = 2
	t.Cleanup(func() { bankCommitChunk = oldChunk; bankAfterChunkHook = nil })
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	bankAfterChunkHook = cancel

	_, err := f.svc.SyncConnection(ctx, testUserID, f.conn.ID, BankSyncOptions{})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	stored := f.stored(t)
	if len(stored) != 2 {
		t.Fatalf("stored %d rows after the interruption, want the 2 oldest", len(stored))
	}
	for _, tx := range stored {
		if tx.BookingDate > "2026-04-02" {
			t.Fatalf("a newer row was stored first: %s", tx.BookingDate)
		}
	}
	if a := f.account(t, "uid-1"); a.LastSyncAttemptAt != nil || a.LastSyncStatus != "" {
		t.Fatalf("an interrupted run recorded state: %+v", a)
	}
	if f.afterCalls != 1 {
		t.Fatalf("afterImport calls = %d, want 1: rows imported before the interruption need the post-import hook", f.afterCalls)
	}

	bankAfterChunkHook = nil
	done := f.sync(t)
	if done.Imported != 3 || done.Duplicates != 2 || len(f.stored(t)) != 5 {
		t.Fatalf("completion = %+v (stored %d)", done, len(f.stored(t)))
	}
}

func TestSync_FutureDatedRowDoesNotMoveTheAnchor(t *testing.T) {
	f := newSyncFixture(t)
	// bad bank data: a booked row far in the future
	if res, err := f.CommitLedgerSync(t.Context(), testUserID, LedgerImportBatch{ID: uuid.New(), AccountID: f.ledgerID, SourceType: "enablebanking", ParserVersion: ParserVersion,
		Status: ImportStatusCommitted, CreatedAt: f.now, UpdatedAt: f.now}, []LedgerTransaction{{
		ID: uuid.New(), AccountID: f.ledgerID, BookingDate: "2027-01-01", Currency: "EUR", AmountMinor: -100, SourceType: "enablebanking",
		Fingerprint: "eb1:future", CreatedAt: f.now, UpdatedAt: f.now}}); err != nil || res.Imported != 1 {
		t.Fatalf("seeding the future row: %+v, %v", res, err)
	}
	f.fake.SetTransactions("uid-1", []enablebanking.Transaction{apiTxn("a", "2026-04-01", "10.00", "DBIT")})
	f.sync(t) // only a future row exists: no usable anchor, so the longest history
	f.sync(t)

	q := f.fake.TransactionQueries()
	if len(q) != 2 || q[0].Strategy != "longest" {
		t.Fatalf("queries = %+v", q)
	}
	if q[1].Strategy != "" || q[1].From != "2026-03-18" {
		t.Fatalf("second window = %+v, want it anchored on the newest row not after today (2026-04-01 - 14 d), not on the future row", q[1])
	}
}

func TestClassifyBankError(t *testing.T) {
	api := func(status int, code string) error { return &enablebanking.APIError{Status: status, Code: code} }
	tests := []struct {
		name   string
		err    error
		status string
		lost   bool
		check  bool
	}{
		{"expired session", api(401, "EXPIRED_SESSION"), BankSyncConsentExpired, true, false},
		{"revoked", api(403, "REVOKED_SESSION"), BankSyncConsentExpired, true, false},
		{"closed", api(403, "CLOSED_SESSION"), BankSyncConsentExpired, true, false},
		{"does not exist", api(404, "SESSION_DOES_NOT_EXIST"), BankSyncConsentExpired, true, false},
		{"plain 403 checks the session", api(403, ""), BankSyncAccessDenied, false, true},
		{"plain 401 checks the session", api(401, ""), BankSyncAccessDenied, false, true},
		{"app credentials", api(401, "UNAUTHORIZED_ACCESS"), BankSyncAppError, false, false},
		{"unauthorized ip", api(403, "UNAUTHORIZED_IP"), BankSyncAppError, false, false},
		{"restricted account", api(403, "ASPSP_ACCOUNT_NOT_ACCESSIBLE"), BankSyncAccountNotAccessible, false, false},
		{"access denied code", api(403, "ACCESS_DENIED"), BankSyncAccountNotAccessible, false, false},
		{"429", api(429, ""), BankSyncRateLimited, false, false},
		{"rate limit code", api(400, "ASPSP_RATE_LIMIT_EXCEEDED"), BankSyncRateLimited, false, false},
		{"502", api(502, ""), BankSyncBankUnavailable, false, false},
		{"bank error", api(400, "ASPSP_ERROR"), BankSyncBankUnavailable, false, false},
		{"transport", &enablebanking.TransportError{Err: errors.New("boom")}, BankSyncBankUnavailable, false, false},
		{"deadline", &enablebanking.TransportError{Err: context.DeadlineExceeded}, BankSyncTimeout, false, false},
		{"unknown 400", api(400, "WHATEVER"), BankSyncError, false, false},
		{"plain error", errors.New("x"), BankSyncError, false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, lost, check := classifyBankError(tt.err)
			if status != tt.status || lost != tt.lost || check != tt.check {
				t.Fatalf("= %q %v %v, want %q %v %v", status, lost, check, tt.status, tt.lost, tt.check)
			}
		})
	}
}

func TestSync_ErrorsAreRecorded(t *testing.T) {
	tests := []struct {
		name   string
		status int
		code   string
		want   string
		lost   bool
	}{
		{"rate limited", 429, "", BankSyncRateLimited, false},
		{"bank unavailable", 502, "", BankSyncBankUnavailable, false},
		{"restricted", 403, "ASPSP_ACCOUNT_NOT_ACCESSIBLE", BankSyncAccountNotAccessible, false},
		{"app error", 401, "UNAUTHORIZED_ACCESS", BankSyncAppError, false},
		{"expired", 401, "EXPIRED_SESSION", BankSyncConsentExpired, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newSyncFixture(t)
			f.fake.FailNext("/accounts", tt.status, tt.code)
			summary := f.sync(t)
			if summary.Accounts[0].Status != tt.want {
				t.Fatalf("status = %q", summary.Accounts[0].Status)
			}
			c, _ := f.GetLedgerBankConnection(t.Context(), testUserID, f.conn.ID)
			if (c.LostAt != nil) != tt.lost || c.Accounts[0].LastSyncStatus != tt.want || c.Accounts[0].LastSyncAttemptAt == nil {
				t.Fatalf("connection = %+v", c)
			}
			if tt.lost {
				before := len(f.fake.TransactionQueries())
				if _, err := f.svc.SyncConnection(t.Context(), testUserID, f.conn.ID, BankSyncOptions{}); !errors.Is(err, ErrBankConsentExpired) {
					t.Fatalf("after loss: %v", err)
				}
				if len(f.fake.TransactionQueries()) != before {
					t.Fatal("upstream call with a lost consent")
				}
			}
		})
	}
}

func TestSync_AccessDeniedChecksSession(t *testing.T) {
	t.Run("session still authorized", func(t *testing.T) {
		f := newSyncFixture(t)
		f.fake.FailNext("/accounts", 403, "")
		summary := f.sync(t)
		c, _ := f.GetLedgerBankConnection(t.Context(), testUserID, f.conn.ID)
		if summary.Accounts[0].Status != BankSyncAccessDenied || c.LostAt != nil {
			t.Fatalf("status = %q, lost = %v", summary.Accounts[0].Status, c.LostAt)
		}
		if calls := f.fake.SessionCalls(); calls[len(calls)-1].Method != "GET" || calls[len(calls)-1].SessionID != "SESSION" {
			t.Fatalf("session calls = %+v", calls)
		}
	})
	t.Run("session no longer authorized", func(t *testing.T) {
		f := newSyncFixture(t)
		f.fake.SetSessionStatus("SESSION", "CLOSED")
		f.fake.FailNext("/accounts", 403, "")
		summary := f.sync(t)
		c, _ := f.GetLedgerBankConnection(t.Context(), testUserID, f.conn.ID)
		if summary.Accounts[0].Status != BankSyncConsentExpired || c.LostAt == nil {
			t.Fatalf("status = %q, lost = %v", summary.Accounts[0].Status, c.LostAt)
		}
	})
}

func TestSync_WrongPeriodRetriesWithLongest(t *testing.T) {
	f := newSyncFixture(t)
	f.fake.SetTransactions("uid-1", []enablebanking.Transaction{apiTxn("a", "2026-04-01", "10.00", "DBIT")})
	f.sync(t)

	f.fake.SetTransactions("uid-1", []enablebanking.Transaction{apiTxn("a", "2026-04-01", "10.00", "DBIT"), apiTxn("b", "2026-04-05", "5.00", "DBIT")})
	f.fake.FailNext("/accounts", 422, "WRONG_TRANSACTIONS_PERIOD")
	summary := f.sync(t)
	if summary.Accounts[0].Status != BankSyncOK || summary.Imported != 1 {
		t.Fatalf("summary = %+v", summary)
	}
	q := f.fake.TransactionQueries()
	if len(q) != 3 || q[1].Strategy != "" || q[2].Strategy != "longest" || q[2].From != q[1].From {
		t.Fatalf("queries = %+v", q)
	}
}

func TestSync_FailingAccountDoesNotStopTheOther(t *testing.T) {
	f := newBankFixture(t)
	first := f.addLedgerAccount(t, bankTestIBAN1, "EUR")
	second := f.addLedgerAccount(t, bankTestIBAN2, "EUR")
	conn := f.connect(t, "code", bankTestSession("SESSION", bankTestAccount("uid-1", bankTestIBAN1), bankTestAccount("uid-2", bankTestIBAN2)))
	calls := 0
	f.svc.afterImport = func(string) { calls++ }
	f.fake.SetTransactions("uid-1", []enablebanking.Transaction{apiTxn("a", "2026-04-01", "1.00", "DBIT")})
	f.fake.SetTransactions("uid-2", []enablebanking.Transaction{apiTxn("b", "2026-04-02", "2.00", "DBIT")})
	f.fake.FailNext("/accounts/uid-1", 500, "")

	summary, err := f.svc.SyncConnection(t.Context(), testUserID, conn.ID, BankSyncOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if summary.Accounts[0].Status != BankSyncBankUnavailable || summary.Accounts[1].Status != BankSyncOK || summary.Imported != 1 {
		t.Fatalf("summary = %+v", summary)
	}
	if a, _ := f.ListLedgerTransactions(t.Context(), testUserID, first); len(a) != 0 {
		t.Fatal("failed account stored rows")
	}
	if b, _ := f.ListLedgerTransactions(t.Context(), testUserID, second); len(b) != 1 {
		t.Fatal("healthy account did not import")
	}
	if calls != 1 {
		t.Fatalf("afterImport calls = %d", calls)
	}
}

func TestSync_UnmappedAccountIsNotFetched(t *testing.T) {
	f := newBankFixture(t)
	f.addLedgerAccount(t, bankTestIBAN1, "EUR")
	conn := f.connect(t, "code", bankTestSession("SESSION", bankTestAccount("uid-1", bankTestIBAN1), bankTestAccount("uid-2", bankTestIBAN2)))
	summary, err := f.svc.SyncConnection(t.Context(), testUserID, conn.ID, BankSyncOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if summary.Accounts[1].UID != "uid-2" || summary.Accounts[1].Status != BankSyncUnmapped {
		t.Fatalf("summary = %+v", summary)
	}
	for _, q := range f.fake.TransactionQueries() {
		if q.Account == "uid-2" {
			t.Fatal("unmapped account fetched")
		}
	}
	c, _ := f.GetLedgerBankConnection(t.Context(), testUserID, conn.ID)
	if c.Accounts[1].LastSyncAttemptAt != nil {
		t.Fatal("attempt recorded for an unmapped account")
	}
}

func TestSync_ExpiredConsentMakesNoUpstreamCall(t *testing.T) {
	f := newSyncFixture(t)
	f.now = f.conn.ValidUntil.Add(time.Second)
	_, err := f.svc.SyncConnection(t.Context(), testUserID, f.conn.ID, BankSyncOptions{})
	if !errors.Is(err, ErrBankConsentExpired) {
		t.Fatalf("err = %v", err)
	}
	if len(f.fake.TransactionQueries()) != 0 {
		t.Fatal("upstream call with an expired consent")
	}
}

func TestSync_OnlyDueSkipsRecentAttempts(t *testing.T) {
	f := newSyncFixture(t)
	f.fake.SetTransactions("uid-1", []enablebanking.Transaction{apiTxn("a", "2026-04-01", "1.00", "DBIT")})
	opt := BankSyncOptions{OnlyDue: true, Interval: 24 * time.Hour}
	if s, err := f.svc.SyncConnection(t.Context(), testUserID, f.conn.ID, opt); err != nil || len(s.Accounts) != 1 {
		t.Fatalf("first = %+v, %v", s, err)
	}
	if s, err := f.svc.SyncConnection(t.Context(), testUserID, f.conn.ID, opt); err != nil || len(s.Accounts) != 0 {
		t.Fatalf("second = %+v, %v", s, err)
	}
	f.now = f.now.Add(25 * time.Hour)
	if s, err := f.svc.SyncConnection(t.Context(), testUserID, f.conn.ID, opt); err != nil || len(s.Accounts) != 1 {
		t.Fatalf("third = %+v, %v", s, err)
	}
}

func TestBankAccountDue(t *testing.T) {
	now := time.Date(2026, 11, 13, 10, 0, 0, 0, time.UTC)
	at := func(d time.Duration) *time.Time { v := now.Add(-d); return &v }
	for name, tt := range map[string]struct {
		a    LedgerBankAccount
		want bool
	}{
		"never attempted":       {LedgerBankAccount{}, true},
		"just attempted":        {LedgerBankAccount{LastSyncAttemptAt: at(time.Minute)}, false},
		"exactly the interval":  {LedgerBankAccount{LastSyncAttemptAt: at(24 * time.Hour)}, true},
		"failed attempt counts": {LedgerBankAccount{LastSyncAttemptAt: at(time.Hour), LastSyncAt: at(48 * time.Hour)}, false},
	} {
		if got := bankAccountDue(tt.a, now, 24*time.Hour); got != tt.want {
			t.Errorf("%s: due = %v, want %v", name, got, tt.want)
		}
	}
}

func TestSync_OneRunPerConnection(t *testing.T) {
	f := newSyncFixture(t)
	unlock, ok := f.svc.lockSync(testUserID, f.conn.ID)
	if !ok {
		t.Fatal("lock not free")
	}
	if _, err := f.svc.SyncConnection(t.Context(), testUserID, f.conn.ID, BankSyncOptions{}); !errors.Is(err, ErrBankSyncRunning) {
		t.Fatalf("err = %v", err)
	}
	unlock()
	if _, err := f.svc.SyncConnection(t.Context(), testUserID, f.conn.ID, BankSyncOptions{}); err != nil {
		t.Fatalf("after unlock: %v", err)
	}
	if _, err := f.svc.SyncConnection(t.Context(), testUserID, uuid.New(), BankSyncOptions{}); !errors.Is(err, storage.ErrNotFound) {
		t.Fatalf("unknown connection: %v", err)
	}
}

func TestSync_RepeatedContinuationKeyAborts(t *testing.T) {
	f := newSyncFixture(t)
	pages := 0
	_, err := f.svc.fetchPagesWith(t.Context(), enablebanking.TransactionsQuery{AccountUID: "uid-1"},
		func(context.Context, enablebanking.TransactionsQuery) (enablebanking.TransactionsPage, error) {
			pages++
			return enablebanking.TransactionsPage{ContinuationKey: "same"}, nil
		})
	if err == nil || pages != 2 {
		t.Fatalf("pages = %d, err = %v", pages, err)
	}
	pages = 0
	_, err = f.svc.fetchPagesWith(t.Context(), enablebanking.TransactionsQuery{AccountUID: "uid-1"},
		func(context.Context, enablebanking.TransactionsQuery) (enablebanking.TransactionsPage, error) {
			pages++
			return enablebanking.TransactionsPage{ContinuationKey: fmt.Sprint(pages)}, nil
		})
	if err == nil || pages != bankMaxPages {
		t.Fatalf("pages = %d, err = %v", pages, err)
	}
}

func TestSync_StateWriteIsDroppedAfterReauthorization(t *testing.T) {
	f := newSyncFixture(t)
	started := f.conn // the record the running sync read, with the old session
	renewed := f.connect(t, "code-2", bankTestSession("SESSION-2", bankTestAccount("uid-1", bankTestIBAN1)))

	// the old session fails after the user re-authorized
	f.svc.recordSyncState(t.Context(), testUserID, started, "uid-1",
		BankAccountSyncResult{UID: "uid-1", Status: BankSyncConsentExpired}, f.now, true)

	c, err := f.GetLedgerBankConnection(t.Context(), testUserID, renewed.ID)
	if err != nil {
		t.Fatal(err)
	}
	if c.LostAt != nil || BankConnectionStatus(c, f.now) != BankStatusActive {
		t.Fatalf("the new consent was marked lost: %+v", c)
	}
	if c.Accounts[0].LastSyncStatus != "" || c.Accounts[0].LastSyncAttemptAt != nil {
		t.Fatalf("a stale sync wrote state into the new record: %+v", c.Accounts[0])
	}

	// a write for the current session still lands
	f.svc.recordSyncState(t.Context(), testUserID, renewed, "uid-1",
		BankAccountSyncResult{UID: "uid-1", Status: BankSyncRateLimited}, f.now, false)
	if a := f.account(t, "uid-1"); a.LastSyncStatus != BankSyncRateLimited {
		t.Fatalf("current-session write lost: %+v", a)
	}
}

func TestSync_AccessDeniedWithUnusableOrBrokenSessionCheck(t *testing.T) {
	t.Run("session check says the session does not exist", func(t *testing.T) {
		f := newSyncFixture(t)
		f.fake.FailNext("/accounts", 403, "")
		f.fake.FailNext("/sessions", 404, "SESSION_DOES_NOT_EXIST")
		summary := f.sync(t)
		c, _ := f.GetLedgerBankConnection(t.Context(), testUserID, f.conn.ID)
		if summary.Accounts[0].Status != BankSyncConsentExpired || c.LostAt == nil {
			t.Fatalf("status = %q, lost = %v", summary.Accounts[0].Status, c.LostAt)
		}
	})
	t.Run("session check itself fails with a server error", func(t *testing.T) {
		f := newSyncFixture(t)
		f.fake.FailNext("/accounts", 403, "")
		f.fake.FailNext("/sessions", 500, "")
		summary := f.sync(t)
		c, _ := f.GetLedgerBankConnection(t.Context(), testUserID, f.conn.ID)
		if summary.Accounts[0].Status != BankSyncAccessDenied || c.LostAt != nil {
			t.Fatalf("a failing session check must not cost the consent: status = %q, lost = %v", summary.Accounts[0].Status, c.LostAt)
		}
	})
}

func TestSync_IdenticalPaymentsWithoutReferenceAcrossRuns(t *testing.T) {
	f := newSyncFixture(t)
	twin := func() enablebanking.Transaction { return apiTxn("", "2026-04-01", "9.99", "DBIT") }
	f.fake.SetTransactions("uid-1", []enablebanking.Transaction{twin(), twin()})
	if s := f.sync(t); s.Imported != 2 {
		t.Fatalf("first run imported %d, want 2", s.Imported)
	}
	if s := f.sync(t); s.Imported != 0 || s.Duplicates != 2 {
		t.Fatalf("repeat = %+v, want 0 imported / 2 duplicates", s)
	}
	f.fake.SetTransactions("uid-1", []enablebanking.Transaction{twin(), twin(), twin()})
	if s := f.sync(t); s.Imported != 1 || s.Duplicates != 2 {
		t.Fatalf("third identical payment = %+v, want exactly 1 imported", s)
	}
	if got := len(f.stored(t)); got != 3 {
		t.Fatalf("stored %d transactions, want 3", got)
	}
}

func TestSync_HostileRowsAreSkippedAndCounted(t *testing.T) {
	f := newSyncFixture(t)
	badDate := apiTxn("d", "2026-13-45", "1.00", "DBIT")
	badIndicator := apiTxn("i", "2026-04-02", "1.00", "")
	huge := apiTxn("h", "2026-04-02", "99999999999999999999.00", "CRDT")
	f.fake.SetTransactions("uid-1", []enablebanking.Transaction{
		apiTxn("ok", "2026-04-02", "1.00", "DBIT"), badDate, badIndicator, huge,
	})
	s := f.sync(t)
	if s.Imported != 1 {
		t.Fatalf("summary = %+v", s)
	}
	if a := s.Accounts[0]; a.SkippedInvalid != 3 || a.Status != BankSyncOK {
		t.Fatalf("account result = %+v, want 3 invalid rows counted", a)
	}
}
