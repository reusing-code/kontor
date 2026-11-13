package ledger

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestBankFingerprints(t *testing.T) {
	acc := uuid.New()
	row := func(ref, purpose string) ParsedRow {
		return ParsedRow{BookingDate: "2026-04-02", AmountMinor: -1250, Currency: "EUR", CounterpartyName: "Shop", Purpose: purpose, BankReference: ref}
	}

	t.Run("entry reference decides, not content", func(t *testing.T) {
		a := BankFingerprints(acc, []ParsedRow{row("ref-1", "first text")})
		b := BankFingerprints(acc, []ParsedRow{row("ref-1", "edited by the bank")})
		if a[0] != b[0] {
			t.Fatal("same reference gave different fingerprints")
		}
		if c := BankFingerprints(acc, []ParsedRow{row("ref-2", "first text")}); c[0] == a[0] {
			t.Fatal("different reference gave the same fingerprint")
		}
	})

	t.Run("namespaced and per account", func(t *testing.T) {
		a := BankFingerprints(acc, []ParsedRow{row("ref-1", "x"), row("", "x")})
		for _, fp := range a {
			if !strings.HasPrefix(fp, "eb1:") {
				t.Fatalf("fingerprint %q lacks the eb1: prefix", fp)
			}
			if len(fp) != len("eb1:")+64 {
				t.Fatalf("fingerprint %q has an unexpected length", fp)
			}
		}
		other := BankFingerprints(uuid.New(), []ParsedRow{row("ref-1", "x"), row("", "x")})
		if other[0] == a[0] || other[1] == a[1] {
			t.Fatal("different account gave the same fingerprint")
		}
	})

	t.Run("identical rows without reference stay distinct and stable", func(t *testing.T) {
		rows := []ParsedRow{row("", "same"), row("", "same"), row("", "other")}
		a := BankFingerprints(acc, rows)
		if a[0] == a[1] || a[0] == a[2] || a[1] == a[2] {
			t.Fatalf("fingerprints collide: %v", a)
		}
		if b := BankFingerprints(acc, rows); strings.Join(a, ",") != strings.Join(b, ",") {
			t.Fatal("fingerprints are not stable across a re-fetch")
		}
		// a window that starts later sees the same first occurrences
		if c := BankFingerprints(acc, rows[:2]); c[0] != a[0] || c[1] != a[1] {
			t.Fatal("prefix of the fetch changed fingerprints")
		}
	})

	t.Run("one reference with different contents", func(t *testing.T) {
		fps := BankFingerprints(acc, []ParsedRow{row("ref-1", "a"), row("ref-1", "b"), row("ref-1", "a")})
		if fps[0] == fps[1] {
			t.Fatal("different contents under one reference collapsed")
		}
		if fps[0] != fps[2] {
			t.Fatal("an exact repeat under one reference must stay a duplicate")
		}
	})
}

// TestBankCSVCrossSource is the D1 check: for DKB and comdirect the CSV row
// and the equivalent API row never share a fingerprint, and syncing the API
// fixture after the CSV import stores nothing and reports every row as a
// cross-source duplicate.
func TestBankCSVCrossSource(t *testing.T) {
	tests := []struct {
		bank     string
		csvFile  string
		fixture  string
		provider Provider
		source   SourceType
	}{
		{"DKB", "DKB-csv-example.csv", "dkb-transactions.json", &DKBCSVProvider{}, SourceDKBCSV},
		{"comdirect", "comdirect-csv-example.csv", "comdirect-transactions.json", &ComdirectCSVProvider{}, SourceComdirectCSV},
	}
	for _, tt := range tests {
		t.Run(tt.bank, func(t *testing.T) {
			f := newBankFixture(t)
			ctx := t.Context()
			accountID := f.addLedgerAccount(t, bankTestIBAN1, "EUR")

			csvFile, err := os.Open(filepath.Join("..", "..", "..", "..", "docs", "design", "csv", tt.csvFile))
			if err != nil {
				t.Fatal(err)
			}
			defer csvFile.Close()
			parsed, err := tt.provider.Parse(csvFile)
			if err != nil {
				t.Fatal(err)
			}
			api := loadBankFixture(t, tt.fixture).CSVEquivalent
			if len(parsed.Rows) != len(api) {
				t.Fatalf("CSV has %d rows, fixture %d", len(parsed.Rows), len(api))
			}

			apiRows := make([]ParsedRow, len(api))
			for i, tx := range api {
				row, skip := RowFromBankTransaction(tx)
				if skip != BankSkipNone {
					t.Fatalf("fixture row %d skipped: %s", i, skip)
				}
				apiRows[i] = row
			}
			apiFps := BankFingerprints(accountID, apiRows)

			batchID := uuid.New()
			var csvTxns []LedgerTransaction
			for i, row := range parsed.Rows {
				csvFp := Fingerprint(accountID.String(), row)
				if csvFp == apiFps[i] {
					t.Fatalf("row %d: CSV and API fingerprints are equal", i)
				}
				csvTxns = append(csvTxns, newLedgerTransactionFromRow(row, accountID, batchID, tt.source, csvFp, nil, f.now))
			}
			if _, err := f.CommitLedgerImport(ctx, testUserID, LedgerImportBatch{
				ID: batchID, AccountID: accountID, SourceType: string(tt.source), ParserVersion: ParserVersion,
				Filename: tt.csvFile, FileSHA256: "csv-" + tt.bank, Status: ImportStatusCommitted, CreatedAt: f.now, UpdatedAt: f.now,
			}, csvTxns); err != nil {
				t.Fatal(err)
			}

			conn := f.connect(t, "code", bankTestSession("SESSION", bankTestAccount("uid-1", bankTestIBAN1)))
			f.fake.SetTransactions("uid-1", api)
			summary, err := f.svc.SyncConnection(ctx, testUserID, conn.ID, BankSyncOptions{})
			if err != nil {
				t.Fatal(err)
			}
			if summary.Imported != 0 || summary.Duplicates != len(api) {
				t.Fatalf("summary = %+v, want 0 imported and %d duplicates", summary, len(api))
			}
			all, err := f.ListLedgerTransactions(ctx, testUserID, accountID)
			if err != nil || len(all) != len(csvTxns) {
				t.Fatalf("transactions = %d, %v; want only the %d CSV rows", len(all), err, len(csvTxns))
			}
			for _, tx := range all {
				if tx.SourceType == string(SourceEnableBanking) {
					t.Fatalf("API row stored: %+v", tx)
				}
			}
			if n := f.countKeys(t, "xdup/"); n != len(api) {
				t.Fatalf("xdup keys = %d, want %d", n, len(api))
			}

			again, err := f.svc.SyncConnection(ctx, testUserID, conn.ID, BankSyncOptions{})
			if err != nil || again.Imported != 0 || again.Duplicates != len(api) {
				t.Fatalf("second run = %+v, %v", again, err)
			}
		})
	}
}

func TestEncodeFieldsIsUnambiguous(t *testing.T) {
	seen := map[string][]string{}
	for _, fields := range [][]string{
		{"a", "bc"}, {"ab", "c"}, {"abc"}, {"abc", ""}, {"", "abc"}, {"1:a", "b"}, {"1", "a:b"}, {"1:", "ab"}, {"a|b", "c"}, {"a", "b|c"},
	} {
		key := encodeFields(fields...)
		if prev, ok := seen[key]; ok {
			t.Fatalf("%q and %q encode to the same string %q", prev, fields, key)
		}
		seen[key] = fields
	}
}

func TestBankFingerprints_DelimiterInjection(t *testing.T) {
	acc := uuid.New()
	base := ParsedRow{BookingDate: "2026-04-02", AmountMinor: -100}

	t.Run("content fields containing the separator", func(t *testing.T) {
		a, b := base, base
		a.CounterpartyName, a.Purpose = "A|B", "C"
		b.CounterpartyName, b.Purpose = "A", "B|C"
		fps := BankFingerprints(acc, []ParsedRow{a})
		if other := BankFingerprints(acc, []ParsedRow{b}); fps[0] == other[0] {
			t.Fatal("distinct payments share a fingerprint")
		}
	})

	t.Run("reference that looks like a suffixed reference", func(t *testing.T) {
		r1, r2, r3 := base, base, base
		r1.BankReference, r1.Purpose = "X", "p"
		r2.BankReference, r2.Purpose = "X", "q"
		r3.BankReference, r3.Purpose = "X|1", "p"
		fps := BankFingerprints(acc, []ParsedRow{r1, r2, r3})
		if fps[1] == fps[2] {
			t.Fatal("distinct references share a fingerprint")
		}
	})
}
