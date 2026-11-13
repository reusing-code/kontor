package ledger

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/dgraph-io/badger/v4"
	"github.com/google/uuid"
	"github.com/reusing-code/kontor/backend/internal/module"
)

type syncStoreFixture struct {
	*testStores
	account uuid.UUID
	now     time.Time
	seq     int
}

func newSyncStoreFixture(t *testing.T) *syncStoreFixture {
	t.Helper()
	f := &syncStoreFixture{testStores: newTestStore(t), account: uuid.New(), now: time.Date(2026, 11, 13, 10, 0, 0, 0, time.UTC)}
	if err := f.CreateLedgerAccount(t.Context(), testUserID, LedgerAccount{ID: f.account, Name: "Main", Bank: "DKB", Currency: "EUR", CreatedAt: f.now, UpdatedAt: f.now}); err != nil {
		t.Fatal(err)
	}
	return f
}

func (f *syncStoreFixture) txn(account uuid.UUID, date string, amount int64, currency, source string) LedgerTransaction {
	f.seq++
	return LedgerTransaction{
		ID: uuid.New(), AccountID: account, BookingDate: date, AmountMinor: amount, Currency: currency, SourceType: source,
		Fingerprint: fmt.Sprintf("fp-%s-%d", source, f.seq), ImportBatchID: uuid.New(), CreatedAt: f.now, UpdatedAt: f.now,
	}
}

func (f *syncStoreFixture) csv(t *testing.T, date string, amount int64) LedgerTransaction {
	t.Helper()
	return f.csvIn(t, f.account, date, amount, "EUR")
}

func (f *syncStoreFixture) csvIn(t *testing.T, account uuid.UUID, date string, amount int64, currency string) LedgerTransaction {
	t.Helper()
	tx := f.txn(account, date, amount, currency, string(SourceDKBCSV))
	batch := LedgerImportBatch{ID: tx.ImportBatchID, AccountID: account, SourceType: string(SourceDKBCSV), ParserVersion: "1", Filename: "a.csv",
		FileSHA256: "hash-" + tx.ID.String(), Status: ImportStatusCommitted, CreatedAt: f.now, UpdatedAt: f.now}
	if _, err := f.CommitLedgerImport(t.Context(), testUserID, batch, []LedgerTransaction{tx}); err != nil {
		t.Fatal(err)
	}
	return tx
}

func (f *syncStoreFixture) api(date string, amount int64) LedgerTransaction {
	return f.txn(f.account, date, amount, "EUR", string(SourceEnableBanking))
}

func (f *syncStoreFixture) sync(t *testing.T, batchID uuid.UUID, txns ...LedgerTransaction) LedgerSyncCommitResult {
	t.Helper()
	for i := range txns {
		txns[i].ImportBatchID = batchID
	}
	res, err := f.CommitLedgerSync(t.Context(), testUserID, LedgerImportBatch{
		ID: batchID, AccountID: f.account, SourceType: string(SourceEnableBanking), ParserVersion: ParserVersion,
		Status: ImportStatusCommitted, CreatedAt: f.now, UpdatedAt: f.now,
	}, txns)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func (f *syncStoreFixture) xdup(t *testing.T, fp string) string {
	t.Helper()
	var got string
	err := f.e.View(func(txn *badger.Txn) error {
		item, err := txn.Get(idxLedXDupKey(testUserID, fp))
		if err != nil {
			return err
		}
		return item.Value(func(v []byte) error { got = string(v); return nil })
	})
	if err != nil {
		t.Fatalf("no xdup for %s: %v", fp, err)
	}
	return got
}

func TestCommitLedgerSync_CrossSourceDistance(t *testing.T) {
	tests := []struct {
		name         string
		csvDate      string
		apiDate      string
		csvAmount    int64
		apiAmount    int64
		csvCurrency  string
		sameAccount  bool
		wantImported int
	}{
		{"exact date", "2026-04-10", "2026-04-10", -500, -500, "EUR", true, 0},
		{"API one day later", "2026-04-10", "2026-04-11", -500, -500, "EUR", true, 0},
		{"API one day earlier", "2026-04-10", "2026-04-09", -500, -500, "EUR", true, 0},
		{"two days later", "2026-04-10", "2026-04-12", -500, -500, "EUR", true, 0},
		{"two days earlier", "2026-04-10", "2026-04-08", -500, -500, "EUR", true, 0},
		{"three days apart imports", "2026-04-10", "2026-04-13", -500, -500, "EUR", true, 1},
		{"sign mismatch imports", "2026-04-10", "2026-04-10", 500, -500, "EUR", true, 1},
		{"other amount imports", "2026-04-10", "2026-04-10", -501, -500, "EUR", true, 1},
		{"other account imports", "2026-04-10", "2026-04-10", -500, -500, "EUR", false, 1},
		{"other currency imports", "2026-04-10", "2026-04-10", -500, -500, "USD", true, 1},
		{"month boundary", "2026-03-31", "2026-04-02", -500, -500, "EUR", true, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newSyncStoreFixture(t)
			account := f.account
			if !tt.sameAccount {
				account = uuid.New()
				if err := f.CreateLedgerAccount(t.Context(), testUserID, LedgerAccount{ID: account, Name: "Other", Bank: "DKB", Currency: "EUR", CreatedAt: f.now, UpdatedAt: f.now}); err != nil {
					t.Fatal(err)
				}
			}
			csv := f.csvIn(t, account, tt.csvDate, tt.csvAmount, tt.csvCurrency)
			api := f.api(tt.apiDate, tt.apiAmount)

			res := f.sync(t, uuid.New(), api)
			if res.Imported != tt.wantImported || res.CrossSourceDuplicates != 1-tt.wantImported || res.Duplicates != 0 {
				t.Fatalf("result = %+v, want imported %d", res, tt.wantImported)
			}
			if tt.wantImported == 0 {
				if got := f.xdup(t, api.Fingerprint); got != csv.ID.String() {
					t.Fatalf("xdup = %s, want %s", got, csv.ID)
				}
			}
		})
	}
}

func TestCommitLedgerSync_SameDayIdenticalPaymentsAreCountAware(t *testing.T) {
	f := newSyncStoreFixture(t)
	f.csv(t, "2026-04-10", -450)
	f.csv(t, "2026-04-10", -450)
	res := f.sync(t, uuid.New(), f.api("2026-04-10", -450), f.api("2026-04-10", -450), f.api("2026-04-10", -450), f.api("2026-04-10", -450))
	if res.Imported != 2 || res.CrossSourceDuplicates != 2 {
		t.Fatalf("result = %+v, want 2 imported and 2 absorbed (k=4 API, m=2 CSV)", res)
	}
}

func TestCommitLedgerSync_ExactDateFirst(t *testing.T) {
	f := newSyncStoreFixture(t)
	csvEarly := f.csv(t, "2026-04-10", -500)
	csvLate := f.csv(t, "2026-04-11", -500)
	// the later API row comes first in the input; it must still take the exact-date CSV row
	lateAPI := f.api("2026-04-11", -500)
	earlyAPI := f.api("2026-04-10", -500)
	res := f.sync(t, uuid.New(), lateAPI, earlyAPI)
	if res.Imported != 0 || res.CrossSourceDuplicates != 2 {
		t.Fatalf("result = %+v", res)
	}
	if got := f.xdup(t, lateAPI.Fingerprint); got != csvLate.ID.String() {
		t.Fatalf("late API row absorbed into %s, want the exact-date row %s", got, csvLate.ID)
	}
	if got := f.xdup(t, earlyAPI.Fingerprint); got != csvEarly.ID.String() {
		t.Fatalf("early API row absorbed into %s, want %s", got, csvEarly.ID)
	}
}

func TestCommitLedgerSync_RerunUsesAbsorptionKeys(t *testing.T) {
	f := newSyncStoreFixture(t)
	f.csv(t, "2026-04-10", -500)
	api := f.api("2026-04-11", -500)
	batch := uuid.New()
	if res := f.sync(t, batch, api); res.CrossSourceDuplicates != 1 {
		t.Fatalf("first run = %+v", res)
	}
	res := f.sync(t, uuid.New(), api)
	if res.Duplicates != 1 || res.CrossSourceDuplicates != 0 || res.Imported != 0 {
		t.Fatalf("second run = %+v, want a plain duplicate via xdup", res)
	}
}

func TestCommitLedgerSync_AbsorbedCSVRowIsNotFreeAgain(t *testing.T) {
	f := newSyncStoreFixture(t)
	f.csv(t, "2026-04-10", -500)
	first := f.api("2026-04-10", -500)
	if res := f.sync(t, uuid.New(), first); res.CrossSourceDuplicates != 1 {
		t.Fatalf("first run = %+v", res)
	}
	// a shifted window no longer returns the first row; an identical new payment arrives
	second := f.api("2026-04-10", -500)
	res := f.sync(t, uuid.New(), second)
	if res.Imported != 1 || res.CrossSourceDuplicates != 0 {
		t.Fatalf("second row = %+v, want it imported because the CSV row is already absorbed", res)
	}
}

func TestCommitLedgerSync_APIRowsOnlyMatchByFingerprint(t *testing.T) {
	f := newSyncStoreFixture(t)
	first := f.api("2026-04-10", -500)
	if res := f.sync(t, uuid.New(), first); res.Imported != 1 {
		t.Fatalf("first = %+v", res)
	}
	second := f.api("2026-04-11", -500)
	if res := f.sync(t, uuid.New(), second); res.Imported != 1 || res.CrossSourceDuplicates != 0 {
		t.Fatalf("second = %+v, API rows must not absorb each other", res)
	}
	if res := f.sync(t, uuid.New(), first); res.Duplicates != 1 || res.Imported != 0 {
		t.Fatalf("same fingerprint = %+v", res)
	}
}

func TestCommitLedgerSync_DuplicateWithinOneCall(t *testing.T) {
	f := newSyncStoreFixture(t)
	a := f.api("2026-04-10", -500)
	b := a
	b.ID = uuid.New()
	res := f.sync(t, uuid.New(), a, b)
	if res.Imported != 1 || res.Duplicates != 1 {
		t.Fatalf("result = %+v", res)
	}
}

func TestCommitLedgerSync_BatchAccumulatesAndStaysHashless(t *testing.T) {
	f := newSyncStoreFixture(t)
	batchID := uuid.New()
	f.sync(t, batchID, f.api("2026-04-10", -100), f.api("2026-04-11", -200))
	dup := f.api("2026-04-12", -300)
	f.sync(t, batchID, dup)
	f.sync(t, batchID, dup, f.api("2026-04-13", -400))

	imports, err := f.ListLedgerImports(t.Context(), testUserID)
	if err != nil || len(imports) != 1 {
		t.Fatalf("imports = %+v, %v", imports, err)
	}
	b := imports[0]
	if b.ID != batchID || b.TotalRows != 5 || b.ImportedRows != 4 || b.DuplicateRows != 1 {
		t.Fatalf("batch = %+v", b)
	}
	if b.SourceType != "enablebanking" || b.Filename != "" || b.FileSHA256 != "" || b.Status != ImportStatusCommitted {
		t.Fatalf("batch = %+v", b)
	}
	if n := f.countKeysFor(t, "file_hash/"); n != 0 {
		t.Fatalf("file_hash keys = %d", n)
	}
}

func (f *syncStoreFixture) countKeysFor(t *testing.T, sub string) int {
	t.Helper()
	prefix := []byte("u/" + testUserID + "/mod/ledger/idx/" + sub)
	n := 0
	if err := f.e.View(func(txn *badger.Txn) error {
		it := txn.NewIterator(badger.DefaultIteratorOptions)
		defer it.Close()
		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			n++
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestCommitLedgerSync_NoBatchWithoutImports(t *testing.T) {
	f := newSyncStoreFixture(t)
	f.csv(t, "2026-04-10", -500)
	before, _ := f.ListLedgerImports(t.Context(), testUserID)
	f.sync(t, uuid.New(), f.api("2026-04-10", -500))
	after, _ := f.ListLedgerImports(t.Context(), testUserID)
	if len(after) != len(before) {
		t.Fatalf("a run without imports left a batch: %d -> %d", len(before), len(after))
	}
}

func TestLedgerAccountBookingRange(t *testing.T) {
	f := newSyncStoreFixture(t)
	if newest, any, err := f.LedgerAccountBookingRange(t.Context(), testUserID, f.account, "9999-12-31"); err != nil || any || newest != "" {
		t.Fatalf("empty = %q %v %v", newest, any, err)
	}
	other := uuid.New()
	f.csvIn(t, other, "2030-01-01", -1, "EUR")
	f.csv(t, "2026-04-10", -1)
	f.csv(t, "2026-04-12", -2)
	f.csv(t, "2026-03-01", -3)
	if newest, any, err := f.LedgerAccountBookingRange(t.Context(), testUserID, f.account, "9999-12-31"); err != nil || !any || newest != "2026-04-12" {
		t.Fatalf("range = %q %v %v", newest, any, err)
	}
	f.csv(t, "2027-01-01", -4)
	for notAfter, want := range map[string]string{"2026-11-13": "2026-04-12", "2026-04-12": "2026-04-12", "2026-04-11": "2026-04-10", "2026-02-28": ""} {
		newest, any, err := f.LedgerAccountBookingRange(t.Context(), testUserID, f.account, notAfter)
		if err != nil || newest != want || any != (want != "") {
			t.Fatalf("notAfter %s: %q %v %v, want %q", notAfter, newest, any, err, want)
		}
	}
}

func TestSyncBatchListedWithEmptyFilename(t *testing.T) {
	h, s := newTestHandler(t)
	accountID := addAccount(t, s, "Main")
	now := time.Now().UTC()
	batch := LedgerImportBatch{ID: uuid.New(), AccountID: accountID, SourceType: "enablebanking", ParserVersion: "1", Status: ImportStatusCommitted, CreatedAt: now, UpdatedAt: now}
	tx := LedgerTransaction{ID: uuid.New(), AccountID: accountID, BookingDate: "2026-04-10", Currency: "EUR", SourceType: "enablebanking",
		Fingerprint: "eb1:x", ImportBatchID: batch.ID, CreatedAt: now, UpdatedAt: now}
	if _, err := s.CommitLedgerSync(context.Background(), testUserID, batch, []LedgerTransaction{tx}); err != nil {
		t.Fatal(err)
	}
	rec := bankRequest(t, newMux(h), http.MethodGet, "/api/v1/ledger/imports", "")
	got := decodeJSON[[]LedgerImportBatch](t, rec)
	if rec.Code != 200 || len(got) != 1 || got[0].Filename != "" || got[0].SourceType != "enablebanking" || got[0].ImportedRows != 1 {
		t.Fatalf("imports = %d %+v", rec.Code, got)
	}
}

// TestExportRestoreRecomputesAbsorptions covers the keys that are not exported:
// after a restore the next sync must reach the same decisions by the matching
// rule, without importing anything again.
func TestExportRestoreRecomputesAbsorptions(t *testing.T) {
	src := newSyncStoreFixture(t)
	ctx := t.Context()
	src.csv(t, "2026-04-10", -500)
	src.csv(t, "2026-04-12", -700)
	a1, a2, a3 := src.api("2026-04-10", -500), src.api("2026-04-12", -700), src.api("2026-04-11", -300)
	if res := src.sync(t, uuid.New(), a1, a2, a3); res.CrossSourceDuplicates != 2 || res.Imported != 1 {
		t.Fatalf("first sync = %+v", res)
	}
	if src.countKeysFor(t, "xdup/") != 2 || src.countKeysFor(t, "xabs/") != 2 {
		t.Fatal("absorption keys missing before the export")
	}

	data, err := (&Module{store: src.Store}).ExportSection(ctx, testUserID)
	if err != nil {
		t.Fatal(err)
	}
	const restored = "user-restored"
	if err := (&Module{store: src.Store}).ImportSection(ctx, restored, data, module.NewImportResult()); err != nil {
		t.Fatal(err)
	}
	countRestored := func(sub string) int {
		n := 0
		prefix := []byte("u/" + restored + "/mod/ledger/idx/" + sub)
		if err := src.e.View(func(txn *badger.Txn) error {
			it := txn.NewIterator(badger.DefaultIteratorOptions)
			defer it.Close()
			for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
				n++
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		return n
	}
	if countRestored("xdup/") != 0 || countRestored("xabs/") != 0 {
		t.Fatal("absorption keys must not be part of the export")
	}

	batch := LedgerImportBatch{ID: uuid.New(), AccountID: src.account, SourceType: string(SourceEnableBanking), ParserVersion: ParserVersion,
		Status: ImportStatusCommitted, CreatedAt: src.now, UpdatedAt: src.now}
	res, err := src.CommitLedgerSync(ctx, restored, batch, []LedgerTransaction{a1, a2, a3})
	if err != nil {
		t.Fatal(err)
	}
	if res.Imported != 0 || res.CrossSourceDuplicates != 2 || res.Duplicates != 1 {
		t.Fatalf("sync after the restore = %+v, want the same decisions and nothing imported", res)
	}
	if countRestored("xdup/") != 2 || countRestored("xabs/") != 2 {
		t.Fatalf("absorptions not recreated: xdup %d, xabs %d", countRestored("xdup/"), countRestored("xabs/"))
	}
	txns, err := src.ListLedgerTransactions(ctx, restored, src.account)
	if err != nil || len(txns) != 3 {
		t.Fatalf("restored account has %d transactions, %v; want 3 (2 CSV + 1 stored API row)", len(txns), err)
	}
}
