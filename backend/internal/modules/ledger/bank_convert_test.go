package ledger

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/reusing-code/kontor/backend/internal/enablebanking"
)

func TestParseBankAmount(t *testing.T) {
	tests := []struct {
		in      string
		want    int64
		wantErr bool
	}{
		{"1.23", 123, false},
		{"1.2", 120, false},
		{"100", 10000, false},
		{"0.05", 5, false},
		{"0", 0, false},
		{"0.00", 0, false},
		{"1.230", 123, false},
		{"1.2300", 123, false},
		{"007.10", 710, false},
		{"1.234", 0, true},
		{"-1.00", 0, true},
		{"+1.00", 0, true},
		{"1e3", 0, true},
		{"", 0, true},
		{".5", 0, true},
		{"1.", 0, true},
		{"1,23", 0, true},
		{"1.2.3", 0, true},
		{" 1.00", 0, true},
		{"99999999999999999999", 0, true},
		{"92233720368547758.08", 0, true},
		{"92233720368547758.07", 9223372036854775807, false},
	}
	for _, tt := range tests {
		got, err := ParseBankAmount(tt.in)
		if (err != nil) != tt.wantErr || got != tt.want {
			t.Errorf("ParseBankAmount(%q) = %d, %v; want %d, err=%v", tt.in, got, err, tt.want, tt.wantErr)
		}
	}
}

func bankTxn(mutate func(*enablebanking.Transaction)) enablebanking.Transaction {
	t := enablebanking.Transaction{
		EntryReference:        "ref-1",
		TransactionAmount:     enablebanking.Amount{Currency: "EUR", Amount: "12.50"},
		CreditDebitIndicator:  "DBIT",
		Status:                "BOOK",
		BookingDate:           "2026-04-02",
		ValueDate:             "2026-04-01",
		TransactionDate:       "2026-03-31",
		Creditor:              &enablebanking.Party{Name: " Shop GmbH "},
		CreditorAccount:       &enablebanking.AccountRef{IBAN: "de44 5001 0517 5407 3249 31"},
		Debtor:                &enablebanking.Party{Name: "Me"},
		DebtorAccount:         &enablebanking.AccountRef{IBAN: "DE02120300000000202051"},
		RemittanceInformation: []string{" Order 1 ", "", "thanks"},
		BankTransactionCode:   &enablebanking.BankTransactionCode{Description: "Kartenzahlung"},
	}
	if mutate != nil {
		mutate(&t)
	}
	return t
}

func TestRowFromBankTransaction(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*enablebanking.Transaction)
		skip   BankSkip
		check  func(t *testing.T, r ParsedRow)
	}{
		{"debit uses creditor", nil, BankSkipNone, func(t *testing.T, r ParsedRow) {
			if r.AmountMinor != -1250 || r.CounterpartyName != "Shop GmbH" || r.CounterpartyIBAN != "DE44500105175407324931" ||
				r.Purpose != "Order 1 thanks" || r.BankReference != "ref-1" || r.TransactionType != "Kartenzahlung" ||
				r.BookingDate != "2026-04-02" || r.ValueDate != "2026-04-01" || r.Currency != "EUR" {
				t.Fatalf("row = %+v", r)
			}
		}},
		{"credit uses debtor", func(x *enablebanking.Transaction) { x.CreditDebitIndicator = "CRDT" }, BankSkipNone, func(t *testing.T, r ParsedRow) {
			if r.AmountMinor != 1250 || r.CounterpartyName != "Me" || r.CounterpartyIBAN != "DE02120300000000202051" {
				t.Fatalf("row = %+v", r)
			}
		}},
		{"credit without debtor", func(x *enablebanking.Transaction) {
			x.CreditDebitIndicator = "CRDT"
			x.Debtor, x.DebtorAccount = nil, nil
		}, BankSkipNone, func(t *testing.T, r ParsedRow) {
			if r.CounterpartyName != "" || r.CounterpartyIBAN != "" {
				t.Fatalf("row = %+v", r)
			}
		}},
		{"booking date falls back to value date", func(x *enablebanking.Transaction) { x.BookingDate = "" }, BankSkipNone, func(t *testing.T, r ParsedRow) {
			if r.BookingDate != "2026-04-01" {
				t.Fatalf("row = %+v", r)
			}
		}},
		{"then to transaction date", func(x *enablebanking.Transaction) { x.BookingDate, x.ValueDate = "", "" }, BankSkipNone, func(t *testing.T, r ParsedRow) {
			if r.BookingDate != "2026-03-31" || r.ValueDate != "" {
				t.Fatalf("row = %+v", r)
			}
		}},
		{"invalid booking date is skipped over", func(x *enablebanking.Transaction) { x.BookingDate = "02.04.2026" }, BankSkipNone, func(t *testing.T, r ParsedRow) {
			if r.BookingDate != "2026-04-01" {
				t.Fatalf("row = %+v", r)
			}
		}},
		{"no date", func(x *enablebanking.Transaction) { x.BookingDate, x.ValueDate, x.TransactionDate = "", "", "" }, BankSkipNoDate, nil},
		{"pending", func(x *enablebanking.Transaction) { x.Status = "PDNG" }, BankSkipNotBooked, nil},
		{"hold", func(x *enablebanking.Transaction) { x.Status = "HOLD" }, BankSkipNotBooked, nil},
		{"scheduled", func(x *enablebanking.Transaction) { x.Status = "SCHD" }, BankSkipNotBooked, nil},
		{"cancelled", func(x *enablebanking.Transaction) { x.Status = "CNCL" }, BankSkipNotBooked, nil},
		{"rejected", func(x *enablebanking.Transaction) { x.Status = "RJCT" }, BankSkipNotBooked, nil},
		{"other status", func(x *enablebanking.Transaction) { x.Status = "OTHR" }, BankSkipNotBooked, nil},
		{"empty status", func(x *enablebanking.Transaction) { x.Status = "" }, BankSkipNotBooked, nil},
		{"lower-case status and currency", func(x *enablebanking.Transaction) { x.Status = "book"; x.TransactionAmount.Currency = "eur" }, BankSkipNone, nil},
		{"foreign currency", func(x *enablebanking.Transaction) { x.TransactionAmount.Currency = "USD" }, BankSkipCurrency, nil},
		{"invalid amount", func(x *enablebanking.Transaction) { x.TransactionAmount.Amount = "1,5" }, BankSkipInvalidAmount, nil},
		{"negative amount", func(x *enablebanking.Transaction) { x.TransactionAmount.Amount = "-1.00" }, BankSkipInvalidAmount, nil},
		{"unknown indicator", func(x *enablebanking.Transaction) { x.CreditDebitIndicator = "" }, BankSkipInvalidAmount, nil},
		{"lower-case indicator", func(x *enablebanking.Transaction) { x.CreditDebitIndicator = "dbit" }, BankSkipNone, func(t *testing.T, r ParsedRow) {
			if r.AmountMinor != -1250 {
				t.Fatalf("row = %+v", r)
			}
		}},
		{"non-ASCII digits in the amount", func(x *enablebanking.Transaction) { x.TransactionAmount.Amount = "١٢.٥٠" }, BankSkipInvalidAmount, nil},
		{"amount with trailing text", func(x *enablebanking.Transaction) { x.TransactionAmount.Amount = "1.00 EUR" }, BankSkipInvalidAmount, nil},
		{"impossible calendar dates are never used", func(x *enablebanking.Transaction) {
			x.BookingDate, x.ValueDate, x.TransactionDate = "2026-13-45", "20260402", " 2026-04-02"
		}, BankSkipNoDate, nil},
		{"date with time part is not a date", func(x *enablebanking.Transaction) {
			x.BookingDate, x.ValueDate, x.TransactionDate = "2026-04-02T10:00:00Z", "", ""
		}, BankSkipNoDate, nil},
		{"leap day", func(x *enablebanking.Transaction) { x.BookingDate = "2026-02-29"; x.ValueDate = "" }, BankSkipNone, func(t *testing.T, r ParsedRow) {
			if r.BookingDate != "2026-03-31" {
				t.Fatalf("2026-02-29 does not exist, expected the transaction date fallback, got %+v", r)
			}
		}},
		{"blank reference and names", func(x *enablebanking.Transaction) {
			x.EntryReference = "  \t"
			x.Creditor = &enablebanking.Party{Name: "   "}
			x.RemittanceInformation = []string{" ", "\t"}
		}, BankSkipNone, func(t *testing.T, r ParsedRow) {
			if r.BankReference != "" || r.CounterpartyName != "" || r.Purpose != "" {
				t.Fatalf("row = %+v", r)
			}
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			row, skip := RowFromBankTransaction(bankTxn(tt.mutate))
			if skip != tt.skip {
				t.Fatalf("skip = %q, want %q", skip, tt.skip)
			}
			if tt.check != nil {
				tt.check(t, row)
			}
		})
	}
}

type bankFixtureFile struct {
	CSVEquivalent []enablebanking.Transaction `json:"csvEquivalent"`
	Other         []enablebanking.Transaction `json:"other"`
}

func loadBankFixture(t *testing.T, name string) bankFixtureFile {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "docs", "design", "enablebanking", name))
	if err != nil {
		t.Fatal(err)
	}
	var f bankFixtureFile
	if err := json.Unmarshal(raw, &f); err != nil {
		t.Fatal(err)
	}
	return f
}

func TestRowFromBankTransaction_Fixtures(t *testing.T) {
	for _, name := range []string{"dkb-transactions.json", "comdirect-transactions.json"} {
		f := loadBankFixture(t, name)
		skips := map[BankSkip]int{}
		for _, tx := range append(append([]enablebanking.Transaction{}, f.CSVEquivalent...), f.Other...) {
			row, skip := RowFromBankTransaction(tx)
			skips[skip]++
			if skip == BankSkipNone && (row.BookingDate == "" || strings.TrimSpace(row.Currency) == "") {
				t.Errorf("%s: incomplete row %+v", name, row)
			}
		}
		if skips[BankSkipNotBooked] != 1 || skips[BankSkipCurrency] != 1 || skips[BankSkipNone] != len(f.CSVEquivalent)+len(f.Other)-2 {
			t.Errorf("%s: skips = %v", name, skips)
		}
	}
}
