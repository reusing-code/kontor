package ledger

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
)

type transferFixture struct {
	s        *testStores
	userID   string
	giro     uuid.UUID
	savings  uuid.UUID
	category uuid.UUID
	now      time.Time
	seq      int
}

func newTransferFixture(t *testing.T) *transferFixture {
	t.Helper()
	f := &transferFixture{s: newTestStore(t), userID: testUserID, giro: uuid.New(), savings: uuid.New(), now: time.Now().UTC()}
	ctx := context.Background()
	for _, a := range []LedgerAccount{
		{ID: f.giro, Name: "Giro", IBAN: "DE89370400440532013000", Bank: "comdirect", Currency: "EUR"},
		{ID: f.savings, Name: "Savings", IBAN: "DE02120300000000202051", Bank: "DKB", Currency: "EUR"},
	} {
		a.CreatedAt, a.UpdatedAt = f.now, f.now
		if err := f.s.CreateLedgerAccount(ctx, f.userID, a); err != nil {
			t.Fatalf("CreateLedgerAccount: %v", err)
		}
	}
	f.category = addCategoryWith(t, f.s, "Savings transfers", nil, "uebertrag")
	return f
}

// add commits the transactions in one import batch, filling in what every
// transaction needs.
func (f *transferFixture) add(t *testing.T, txns ...LedgerTransaction) {
	t.Helper()
	f.seq++
	batch := LedgerImportBatch{ID: uuid.New(), AccountID: f.giro, SourceType: "dkb.csv", ParserVersion: "1", Filename: "test.csv", FileSHA256: fmt.Sprintf("hash-%d", f.seq), Status: ImportStatusCommitted, CreatedAt: f.now, UpdatedAt: f.now}
	for i := range txns {
		txns[i].Currency = "EUR"
		txns[i].ImportBatchID = batch.ID
		txns[i].Fingerprint = fmt.Sprintf("fp-%d-%d", f.seq, i)
		txns[i].CreatedAt, txns[i].UpdatedAt = f.now, f.now
	}
	if _, err := f.s.CommitLedgerImport(context.Background(), f.userID, batch, txns); err != nil {
		t.Fatalf("CommitLedgerImport: %v", err)
	}
}

func (f *transferFixture) get(t *testing.T, id uuid.UUID) LedgerTransaction {
	t.Helper()
	txn, err := f.s.GetLedgerTransaction(context.Background(), f.userID, id)
	if err != nil {
		t.Fatalf("GetLedgerTransaction: %v", err)
	}
	return txn
}

func (f *transferFixture) autoLink(t *testing.T) int {
	t.Helper()
	linked, err := f.s.AutoLinkLedgerTransfers(context.Background(), f.userID)
	if err != nil {
		t.Fatalf("AutoLinkLedgerTransfers: %v", err)
	}
	return linked
}

func TestAutoLinkLedgerTransfers_LinksPairWithSameCounterparty(t *testing.T) {
	f := newTransferFixture(t)
	out, in := uuid.New(), uuid.New()
	f.add(t,
		LedgerTransaction{ID: out, AccountID: f.giro, BookingDate: "2026-11-02", AmountMinor: -250000, CounterpartyName: "Jane  Doe", Purpose: "Uebertrag", CategoryID: &f.category, CategorizationSource: LedgerCategorizationKeyword},
		LedgerTransaction{ID: in, AccountID: f.savings, BookingDate: "2026-11-03", AmountMinor: 250000, CounterpartyName: "jane doe"},
	)

	if got := f.autoLink(t); got != 1 {
		t.Fatalf("linked = %d, want 1", got)
	}
	left, right := f.get(t, out), f.get(t, in)
	for _, txn := range []LedgerTransaction{left, right} {
		if txn.SpecialCategory != LedgerSpecialCategoryInternalTransfer || txn.CategoryID != nil || txn.ReviewStatus != LedgerTransactionReviewConfirmed {
			t.Fatalf("transaction %s: special %q, category %v, review %q", txn.ID, txn.SpecialCategory, txn.CategoryID, txn.ReviewStatus)
		}
	}
	if left.TransferPairTransactionID == nil || *left.TransferPairTransactionID != in || right.TransferPairTransactionID == nil || *right.TransferPairTransactionID != out {
		t.Fatalf("pair ids = %v / %v", left.TransferPairTransactionID, right.TransferPairTransactionID)
	}
	if got := f.autoLink(t); got != 0 {
		t.Fatalf("second run linked = %d, want 0", got)
	}
}

func TestAutoLinkLedgerTransfers_LinksPairWithMatchingIBAN(t *testing.T) {
	f := newTransferFixture(t)
	out, in := uuid.New(), uuid.New()
	f.add(t,
		LedgerTransaction{ID: out, AccountID: f.giro, BookingDate: "2026-11-02", AmountMinor: -5000, CounterpartyName: "DKB AG", CounterpartyIBAN: "de02 1203 0000 0000 2020 51"},
		LedgerTransaction{ID: in, AccountID: f.savings, BookingDate: "2026-11-02", AmountMinor: 5000},
	)

	if got := f.autoLink(t); got != 1 {
		t.Fatalf("linked = %d, want 1", got)
	}
}

func TestAutoLinkLedgerTransfers_SkipsUnclearPairs(t *testing.T) {
	tests := []struct {
		name  string
		setup func(f *transferFixture) []LedgerTransaction
	}{
		{name: "no IBAN and other counterparty", setup: func(f *transferFixture) []LedgerTransaction {
			return []LedgerTransaction{
				{ID: uuid.New(), AccountID: f.giro, BookingDate: "2026-11-02", AmountMinor: -1540, CounterpartyName: "REWE Markt"},
				{ID: uuid.New(), AccountID: f.savings, BookingDate: "2026-11-02", AmountMinor: 1540, CounterpartyName: "REWE Gutschrift GmbH"},
			}
		}},
		{name: "one side reviewed", setup: func(f *transferFixture) []LedgerTransaction {
			return []LedgerTransaction{
				{ID: uuid.New(), AccountID: f.giro, BookingDate: "2026-11-02", AmountMinor: -250000, CounterpartyName: "Jane Doe", CategoryID: &f.category, ReviewStatus: LedgerTransactionReviewConfirmed, CategorizationSource: LedgerCategorizationManual},
				{ID: uuid.New(), AccountID: f.savings, BookingDate: "2026-11-02", AmountMinor: 250000, CounterpartyName: "Jane Doe"},
			}
		}},
		{name: "two possible matches", setup: func(f *transferFixture) []LedgerTransaction {
			return []LedgerTransaction{
				{ID: uuid.New(), AccountID: f.giro, BookingDate: "2026-11-02", AmountMinor: -10000, CounterpartyName: "Jane Doe"},
				{ID: uuid.New(), AccountID: f.savings, BookingDate: "2026-11-02", AmountMinor: 10000, CounterpartyName: "Jane Doe"},
				{ID: uuid.New(), AccountID: f.savings, BookingDate: "2026-11-04", AmountMinor: 10000, CounterpartyName: "Jane Doe"},
			}
		}},
		{name: "reviewed look-alike makes it ambiguous", setup: func(f *transferFixture) []LedgerTransaction {
			return []LedgerTransaction{
				{ID: uuid.New(), AccountID: f.savings, BookingDate: "2026-11-02", AmountMinor: 10000, CounterpartyName: "Jane Doe"},
				{ID: uuid.New(), AccountID: f.giro, BookingDate: "2026-11-02", AmountMinor: -10000, CounterpartyName: "Jane Doe"},
				{ID: uuid.New(), AccountID: f.giro, BookingDate: "2026-11-03", AmountMinor: -10000, CounterpartyName: "Jane Doe", ReviewStatus: LedgerTransactionReviewConfirmed, CategorizationSource: LedgerCategorizationManual},
			}
		}},
		{name: "outside the date window", setup: func(f *transferFixture) []LedgerTransaction {
			return []LedgerTransaction{
				{ID: uuid.New(), AccountID: f.giro, BookingDate: "2026-11-02", AmountMinor: -10000, CounterpartyName: "Jane Doe"},
				{ID: uuid.New(), AccountID: f.savings, BookingDate: "2026-11-06", AmountMinor: 10000, CounterpartyName: "Jane Doe"},
			}
		}},
		{name: "same account", setup: func(f *transferFixture) []LedgerTransaction {
			return []LedgerTransaction{
				{ID: uuid.New(), AccountID: f.giro, BookingDate: "2026-11-02", AmountMinor: -10000, CounterpartyName: "Jane Doe"},
				{ID: uuid.New(), AccountID: f.giro, BookingDate: "2026-11-02", AmountMinor: 10000, CounterpartyName: "Jane Doe"},
			}
		}},
		{name: "split transaction", setup: func(f *transferFixture) []LedgerTransaction {
			return []LedgerTransaction{
				{ID: uuid.New(), AccountID: f.giro, BookingDate: "2026-11-02", AmountMinor: -10000, CounterpartyName: "Jane Doe", CategoryID: &f.category, Splits: []LedgerTransactionSplit{{AmountMinor: -2000, CategoryID: &f.category}}},
				{ID: uuid.New(), AccountID: f.savings, BookingDate: "2026-11-02", AmountMinor: 10000, CounterpartyName: "Jane Doe"},
			}
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := newTransferFixture(t)
			txns := tc.setup(f)
			f.add(t, txns...)

			if got := f.autoLink(t); got != 0 {
				t.Fatalf("linked = %d, want 0", got)
			}
			for _, txn := range txns {
				if got := f.get(t, txn.ID); got.TransferPairTransactionID != nil || got.SpecialCategory != "" {
					t.Fatalf("transaction %s was linked", txn.ID)
				}
			}
		})
	}
}

func TestAfterImport_LinksTransfersBeforeSuggestions(t *testing.T) {
	h, s := newTestHandler(t)
	f := &transferFixture{s: s, userID: testUserID, giro: uuid.New(), savings: uuid.New(), now: time.Now().UTC()}
	for _, a := range []LedgerAccount{{ID: f.giro, Name: "Giro", Currency: "EUR"}, {ID: f.savings, Name: "Savings", Currency: "EUR"}} {
		if err := s.CreateLedgerAccount(context.Background(), testUserID, a); err != nil {
			t.Fatalf("CreateLedgerAccount: %v", err)
		}
	}
	out, in := uuid.New(), uuid.New()
	f.add(t,
		LedgerTransaction{ID: out, AccountID: f.giro, BookingDate: "2026-11-02", AmountMinor: -250000, CounterpartyName: "Jane Doe"},
		LedgerTransaction{ID: in, AccountID: f.savings, BookingDate: "2026-11-02", AmountMinor: 250000, CounterpartyName: "Jane Doe"},
	)

	h.afterImport(testUserID)

	if got := f.get(t, out); got.TransferPairTransactionID == nil || *got.TransferPairTransactionID != in {
		t.Fatalf("transfer pair = %v, want %s", got.TransferPairTransactionID, in)
	}
}
