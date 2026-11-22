package ledger

import (
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
