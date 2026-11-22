package ledger

import (
	"fmt"
	"math/rand/v2"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/reusing-code/kontor/backend/internal/module"
)

func addBooking(t *testing.T, s *testStores, accountID uuid.UUID, date string, amount int64) {
	t.Helper()
	addTransaction(t, s, accountID, func(txn *LedgerTransaction) {
		txn.BookingDate, txn.AmountMinor = date, amount
	})
}

func putSnapshot(t *testing.T, s *testStores, accountID uuid.UUID, asOf string, amount int64, source string) {
	t.Helper()
	if err := s.PutLedgerBalanceSnapshot(t.Context(), testUserID, LedgerBalanceSnapshot{
		AccountID: accountID, AsOf: asOf, AmountMinor: amount, Source: source, RecordedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("PutLedgerBalanceSnapshot: %v", err)
	}
}

func TestComputeLedgerAccountBalance(t *testing.T) {
	txns := []LedgerTransaction{
		{BookingDate: "2026-03-01", AmountMinor: 500},
		{BookingDate: "2026-03-10", AmountMinor: -200},
		{BookingDate: "2026-03-15", AmountMinor: 1000},
		{BookingDate: "2026-03-20", AmountMinor: -50},
	}
	snap := func(asOf string, amount int64) LedgerBalanceSnapshot {
		return LedgerBalanceSnapshot{AsOf: asOf, AmountMinor: amount, Source: "dkb.csv"}
	}

	if got := computeLedgerAccountBalance(nil, txns); got != nil {
		t.Fatalf("no snapshot: got %+v, want nil", got)
	}

	got := computeLedgerAccountBalance([]LedgerBalanceSnapshot{snap("2026-03-10", 10_000)}, txns)
	if got.CurrentMinor != 10_950 || got.SnapshotMinor != 10_000 || got.AsOf != "2026-03-10" || got.DriftMinor != nil {
		t.Fatalf("single snapshot: %+v", got)
	}

	consistent := computeLedgerAccountBalance([]LedgerBalanceSnapshot{snap("2026-03-01", 10_200), snap("2026-03-15", 11_000)}, txns)
	if consistent.DriftMinor == nil || *consistent.DriftMinor != 0 || consistent.PreviousAsOf != "2026-03-01" {
		t.Fatalf("consistent snapshots: %+v", consistent)
	}
	if consistent.CurrentMinor != 10_950 {
		t.Fatalf("current = %d, want 10950", consistent.CurrentMinor)
	}

	drifting := computeLedgerAccountBalance([]LedgerBalanceSnapshot{snap("2026-03-01", 10_200), snap("2026-03-15", 11_300)}, txns)
	if drifting.DriftMinor == nil || *drifting.DriftMinor != 300 {
		t.Fatalf("drifting snapshots: %+v", drifting)
	}
}

func TestLedgerBalanceSnapshots_StoreAnchorAndReplace(t *testing.T) {
	_, s := newTestHandler(t)
	ctx := t.Context()
	accountID := addAccount(t, s, "Main")
	other := addAccount(t, s, "Other")
	addBooking(t, s, accountID, "2026-04-01", 1000)
	addBooking(t, s, accountID, "2026-04-05", -300)
	addBooking(t, s, other, "2026-04-05", 99_999)

	putSnapshot(t, s, accountID, "2026-04-01", 5000, "dkb.csv")
	putSnapshot(t, s, accountID, "2026-03-01", 1234, LedgerBalanceSourceManual)
	balance, err := s.GetLedgerAccountBalance(ctx, testUserID, accountID)
	if err != nil {
		t.Fatal(err)
	}
	if balance.AsOf != "2026-04-01" || balance.CurrentMinor != 4700 {
		t.Fatalf("an older snapshot must not move the anchor: %+v", balance)
	}

	putSnapshot(t, s, accountID, "2026-04-01", 6000, string(SourceEnableBanking))
	snapshots, err := s.ListLedgerBalanceSnapshots(ctx, testUserID, accountID)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshots) != 2 || snapshots[0].AsOf != "2026-03-01" || snapshots[1].AmountMinor != 6000 || snapshots[1].Source != string(SourceEnableBanking) {
		t.Fatalf("snapshots = %+v, want the same day replaced and sorted by date", snapshots)
	}

	balances, err := s.ListLedgerAccountBalances(ctx, testUserID)
	if err != nil {
		t.Fatal(err)
	}
	if len(balances) != 1 || balances[accountID].CurrentMinor != 5700 {
		t.Fatalf("balances = %+v, want only the account with a snapshot", balances)
	}
	if b, err := s.GetLedgerAccountBalance(ctx, testUserID, other); err != nil || b != nil {
		t.Fatalf("account without snapshot: %+v, %v", b, err)
	}
}

func TestPutLedgerBalanceSnapshot_Validates(t *testing.T) {
	_, s := newTestHandler(t)
	accountID := addAccount(t, s, "Main")
	for name, snapshot := range map[string]LedgerBalanceSnapshot{
		"bad date":        {AccountID: accountID, AsOf: "01.04.2026", Source: "manual"},
		"no source":       {AccountID: accountID, AsOf: "2026-04-01"},
		"unknown account": {AccountID: uuid.New(), AsOf: "2026-04-01", Source: "manual"},
	} {
		if err := s.PutLedgerBalanceSnapshot(t.Context(), testUserID, snapshot); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
}

func TestDeleteLedgerAccount_RemovesBalanceSnapshots(t *testing.T) {
	_, s := newTestHandler(t)
	accountID := addAccount(t, s, "Main")
	other := addAccount(t, s, "Other")
	putSnapshot(t, s, accountID, "2026-04-01", 5000, "dkb.csv")
	putSnapshot(t, s, other, "2026-04-01", 7000, "dkb.csv")

	if err := s.DeleteLedgerAccount(t.Context(), testUserID, accountID); err != nil {
		t.Fatal(err)
	}
	all, err := s.ListAllLedgerBalanceSnapshots(t.Context(), testUserID)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 1 || all[0].AccountID != other {
		t.Fatalf("snapshots after delete = %+v", all)
	}
}

func TestExportImport_RoundTripsBalanceSnapshots(t *testing.T) {
	_, src := newTestHandler(t)
	ctx := t.Context()
	accountID := addAccount(t, src, "Main")
	putSnapshot(t, src, accountID, "2026-03-01", 1000, LedgerBalanceSourceManual)
	putSnapshot(t, src, accountID, "2026-04-01", 2000, "comdirect.csv")

	data, err := (&Module{store: src.Store}).ExportSection(ctx, testUserID)
	if err != nil {
		t.Fatal(err)
	}
	dst := newTestStore(t)
	res := module.NewImportResult()
	if err := (&Module{store: dst.Store}).ImportSection(ctx, "user-2", data, res); err != nil {
		t.Fatal(err)
	}
	got, err := dst.ListLedgerBalanceSnapshots(ctx, "user-2", accountID)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].AmountMinor != 1000 || got[1].Source != "comdirect.csv" {
		t.Fatalf("restored snapshots = %+v", got)
	}
}

func TestImportSection_WithoutBalancesStillWorks(t *testing.T) {
	dst := newTestStore(t)
	accountID := uuid.New()
	data := []byte(`{"schemaVersion":0,"accounts":[{"id":"` + accountID.String() + `","name":"Main","bank":"DKB","currency":"EUR"}],"categories":[],"imports":[],"transactions":[],"emailAccounts":[],"emailOrders":[]}`)
	if err := (&Module{store: dst.Store}).ImportSection(t.Context(), "user-2", data, module.NewImportResult()); err != nil {
		t.Fatal(err)
	}
	orphan := []byte(`{"schemaVersion":0,"accounts":[],"categories":[],"imports":[],"transactions":[],"emailAccounts":[],"emailOrders":[],` +
		`"balances":[{"accountId":"` + uuid.NewString() + `","asOf":"2026-04-01","amountMinor":1,"source":"manual"}]}`)
	res := module.NewImportResult()
	if err := (&Module{store: dst.Store}).ImportSection(t.Context(), "user-3", orphan, res); err != nil {
		t.Fatal(err)
	}
	if all, _ := dst.ListAllLedgerBalanceSnapshots(t.Context(), "user-3"); len(all) != 0 {
		t.Fatalf("orphan snapshot restored: %+v", all)
	}
}

type accountPageBody struct {
	Items          []LedgerTransaction   `json:"items"`
	NextCursor     string                `json:"nextCursor"`
	PageBalance    *LedgerPageBalance    `json:"pageBalance"`
	AccountBalance *LedgerAccountBalance `json:"accountBalance"`
}

func getAccountPage(t *testing.T, mux http.Handler, accountID uuid.UUID, limit int, cursor string) accountPageBody {
	t.Helper()
	url := fmt.Sprintf("/api/v1/ledger/accounts/%s/transactions?limit=%d&cursor=%s", accountID, limit, cursor)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("GET", url, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body)
	}
	return decodeJSON[accountPageBody](t, rec)
}

func TestAccountTransactionPages_BalancesMatchARunningSum(t *testing.T) {
	h, s := newTestHandler(t)
	mux := newMux(h)
	accountID := addAccount(t, s, "Main")
	other := addAccount(t, s, "Other")
	rng := rand.New(rand.NewPCG(71, 1))
	var txns []LedgerTransaction
	for i := range 23 {
		txns = append(txns, LedgerTransaction{
			ID: uuid.New(), AccountID: accountID, Currency: "EUR",
			BookingDate: fmt.Sprintf("2026-03-%02d", 1+i%12),
			AmountMinor: rng.Int64N(20_000) - 10_000,
		})
	}
	commitTransactions(t, s, accountID, txns...)
	addBooking(t, s, other, "2026-03-05", 1_000_000)
	putSnapshot(t, s, accountID, "2026-03-06", 50_000, "dkb.csv")

	listed, err := s.ListLedgerTransactionsPage(t.Context(), testUserID, accountID, 500, "")
	if err != nil {
		t.Fatal(err)
	}
	current := int64(50_000) + sumBookedBetween(listed.Items, "2026-03-06", "")
	// balanceAfter[i] is the balance right after the i-th listed transaction
	balanceAfter := make([]int64, len(listed.Items))
	running := current
	for i, txn := range listed.Items {
		balanceAfter[i] = running
		running -= txn.AmountMinor
	}
	oldestOpening := running

	cursor, offset, pages := "", 0, 0
	var previousOpening *int64
	for {
		page := getAccountPage(t, mux, accountID, 5, cursor)
		pages++
		if page.PageBalance == nil || page.AccountBalance == nil {
			t.Fatalf("page %d has no balances", pages)
		}
		if page.AccountBalance.CurrentMinor != current {
			t.Fatalf("current = %d, want %d", page.AccountBalance.CurrentMinor, current)
		}
		if page.PageBalance.ClosingMinor != balanceAfter[offset] {
			t.Fatalf("page %d closing = %d, want %d", pages, page.PageBalance.ClosingMinor, balanceAfter[offset])
		}
		if previousOpening != nil && *previousOpening != page.PageBalance.ClosingMinor {
			t.Fatalf("page %d closing %d != previous opening %d", pages, page.PageBalance.ClosingMinor, *previousOpening)
		}
		opening := page.PageBalance.OpeningMinor
		previousOpening = &opening
		offset += len(page.Items)
		if page.NextCursor == "" {
			break
		}
		cursor = page.NextCursor
	}
	if offset != len(txns) || pages != 5 {
		t.Fatalf("paged %d transactions in %d pages", offset, pages)
	}
	if *previousOpening != oldestOpening {
		t.Fatalf("last opening = %d, want %d", *previousOpening, oldestOpening)
	}
}

func TestAccountTransactionPages_NoSnapshotNoBalances(t *testing.T) {
	h, s := newTestHandler(t)
	mux := newMux(h)
	accountID := addAccount(t, s, "Main")
	addBooking(t, s, accountID, "2026-03-01", 100)

	page := getAccountPage(t, mux, accountID, 10, "")
	if len(page.Items) != 1 || page.PageBalance != nil || page.AccountBalance != nil {
		t.Fatalf("page = %+v", page)
	}

	empty := addAccount(t, s, "Empty")
	putSnapshot(t, s, empty, "2026-03-01", 4200, LedgerBalanceSourceManual)
	page = getAccountPage(t, mux, empty, 10, "")
	if page.PageBalance == nil || page.PageBalance.ClosingMinor != 4200 || page.PageBalance.OpeningMinor != 4200 {
		t.Fatalf("empty account page balance = %+v", page.PageBalance)
	}
}

func TestLedgerAccountEndpoints_CarryBalance(t *testing.T) {
	h, s := newTestHandler(t)
	mux := newMux(h)
	withBalance := addAccount(t, s, "A")
	addAccount(t, s, "B")
	addBooking(t, s, withBalance, "2026-03-02", -500)
	putSnapshot(t, s, withBalance, "2026-03-01", 10_000, "comdirect.csv")

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("GET", "/api/v1/ledger/accounts", nil))
	type accountBody struct {
		ID      uuid.UUID             `json:"id"`
		Balance *LedgerAccountBalance `json:"balance"`
	}
	list := decodeJSON[[]accountBody](t, rec)
	if len(list) != 2 {
		t.Fatalf("accounts = %+v", list)
	}
	for _, account := range list {
		if (account.ID == withBalance) != (account.Balance != nil) {
			t.Fatalf("account %s balance = %+v", account.ID, account.Balance)
		}
		if account.Balance != nil && account.Balance.CurrentMinor != 9_500 {
			t.Fatalf("current = %d, want 9500", account.Balance.CurrentMinor)
		}
	}

	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("GET", "/api/v1/ledger/accounts/"+withBalance.String(), nil))
	single := decodeJSON[accountBody](t, rec)
	if single.Balance == nil || single.Balance.CurrentMinor != 9_500 || single.Balance.Source != "comdirect.csv" {
		t.Fatalf("account balance = %+v", single.Balance)
	}
}

func TestManualBalance_SetAndDelete(t *testing.T) {
	h, s := newTestHandler(t)
	mux := newMux(h)
	accountID := addAccount(t, s, "Main")
	addBooking(t, s, accountID, "2026-03-02", 250)
	base := "/api/v1/ledger/accounts/" + accountID.String() + "/balance"
	do := func(method, url string, body any) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(method, url, nil)
		if body != nil {
			req = httptest.NewRequest(method, url, jsonBody(body))
		}
		mux.ServeHTTP(rec, req)
		return rec
	}
	type accountBody struct {
		Balance *LedgerAccountBalance `json:"balance"`
	}

	rec := do("POST", base, map[string]any{"amountMinor": 1000, "asOf": "2026-03-01"})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body)
	}
	if got := decodeJSON[accountBody](t, rec); got.Balance == nil || got.Balance.CurrentMinor != 1250 || got.Balance.Source != LedgerBalanceSourceManual {
		t.Fatalf("balance = %+v", got.Balance)
	}

	rec = do("POST", base, map[string]any{"amountMinor": -5})
	today := time.Now().UTC().Format(time.DateOnly)
	if got := decodeJSON[accountBody](t, rec); rec.Code != http.StatusOK || got.Balance.AsOf != today || got.Balance.SnapshotMinor != -5 {
		t.Fatalf("default date: %d %+v", rec.Code, got.Balance)
	}

	for name, body := range map[string]any{
		"no amount":    map[string]any{"asOf": "2026-03-01"},
		"future":       map[string]any{"amountMinor": 1, "asOf": "2999-01-01"},
		"bad date":     map[string]any{"amountMinor": 1, "asOf": "01.03.2026"},
		"out of range": map[string]any{"amountMinor": int64(maxManualBalanceMinor) + 1},
	} {
		if rec := do("POST", base, body); rec.Code != http.StatusBadRequest {
			t.Errorf("%s: status = %d", name, rec.Code)
		}
	}
	if rec := do("POST", "/api/v1/ledger/accounts/"+uuid.NewString()+"/balance", map[string]any{"amountMinor": 1}); rec.Code != http.StatusNotFound {
		t.Errorf("unknown account: status = %d", rec.Code)
	}

	rec = do("DELETE", base+"/"+today, nil)
	if got := decodeJSON[accountBody](t, rec); rec.Code != http.StatusOK || got.Balance == nil || got.Balance.AsOf != "2026-03-01" {
		t.Fatalf("after delete: %d %+v", rec.Code, got.Balance)
	}
	putSnapshot(t, s, accountID, "2026-03-05", 9, "dkb.csv")
	if rec := do("DELETE", base+"/2026-03-05", nil); rec.Code != http.StatusConflict {
		t.Fatalf("deleting an imported balance: status = %d", rec.Code)
	}
	if rec := do("DELETE", base+"/2026-01-01", nil); rec.Code != http.StatusNotFound {
		t.Fatalf("deleting a missing balance: status = %d", rec.Code)
	}
	if rec := do("DELETE", base+"/yesterday", nil); rec.Code != http.StatusBadRequest {
		t.Fatalf("bad date: status = %d", rec.Code)
	}
}
