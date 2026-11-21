package ledger

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func putSplits(t *testing.T, mux http.Handler, id uuid.UUID, splits any) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("PUT", "/api/v1/ledger/transactions/"+id.String()+"/splits", jsonBody(map[string]any{"splits": splits})))
	return rec
}

func part(amountMinor int64, categoryID uuid.UUID, note string) map[string]any {
	return map[string]any{"amountMinor": amountMinor, "categoryId": categoryID, "note": note}
}

func TestSetLedgerTransactionSplits(t *testing.T) {
	h, s := newTestHandler(t)
	mux := newMux(h)
	accountID := addAccount(t, s, "Main")
	groceries := addCategory(t, s, "Groceries")
	cash := addCategory(t, s, "Cash")
	txnID := uuid.New()
	commitTransactions(t, s, accountID, LedgerTransaction{
		ID: txnID, AccountID: accountID, BookingDate: "2026-11-02", AmountMinor: -17235, Currency: "EUR",
		CategoryID: &groceries, ReviewStatus: LedgerTransactionReviewConfirmed, CategorizationSource: LedgerCategorizationManual,
	})

	rec := putSplits(t, mux, txnID, []any{part(-15000, cash, "  Auszahlung  ")})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; body: %s", rec.Code, rec.Body.String())
	}
	got := decodeJSON[LedgerTransaction](t, rec)
	if len(got.Splits) != 1 || got.Splits[0].AmountMinor != -15000 || *got.Splits[0].CategoryID != cash || got.Splits[0].Note != "Auszahlung" {
		t.Fatalf("splits = %+v", got.Splits)
	}
	if *got.CategoryID != groceries || got.ReviewStatus != LedgerTransactionReviewConfirmed || got.AmountMinor != -17235 {
		t.Fatalf("transaction changed: %+v", got)
	}
	if rest := ledgerSplitRemainder(got); rest != -2235 {
		t.Fatalf("remainder = %d, want -2235", rest)
	}

	stored, err := s.GetLedgerTransaction(t.Context(), testUserID, txnID)
	if err != nil || len(stored.Splits) != 1 {
		t.Fatalf("stored splits = %+v, err %v", stored.Splits, err)
	}

	rec = putSplits(t, mux, txnID, []any{})
	if rec.Code != http.StatusOK {
		t.Fatalf("remove status = %d; body: %s", rec.Code, rec.Body.String())
	}
	if raw := rec.Body.String(); strings.Contains(raw, `"splits"`) {
		t.Fatalf("splits still in response: %s", raw)
	}
}

func TestSetLedgerTransactionSplits_Invalid(t *testing.T) {
	h, s := newTestHandler(t)
	mux := newMux(h)
	accountID := addAccount(t, s, "Main")
	cash := addCategory(t, s, "Cash")
	txnID := uuid.New()
	commitTransactions(t, s, accountID, LedgerTransaction{ID: txnID, AccountID: accountID, BookingDate: "2026-11-02", AmountMinor: -10000, Currency: "EUR"})

	many := make([]any, maxLedgerSplits+1)
	for i := range many {
		many[i] = part(-1, cash, "")
	}
	cases := map[string]struct {
		splits any
		want   string
	}{
		"wrong sign":     {[]any{part(500, cash, "")}, "sign"},
		"zero":           {[]any{part(0, cash, "")}, "zero"},
		"whole amount":   {[]any{part(-6000, cash, ""), part(-4000, cash, "")}, "less than"},
		"no category":    {[]any{map[string]any{"amountMinor": -100}}, "category"},
		"unknown":        {[]any{part(-100, uuid.New(), "")}, "unknown category"},
		"too many parts": {many, "at most"},
		"note too long":  {[]any{part(-100, cash, strings.Repeat("x", maxLedgerSplitNoteLength+1))}, "note"},
		"unknown field":  {[]any{map[string]any{"amountMinor": -100, "categoryId": cash, "x": 1}}, "invalid request body"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			rec := putSplits(t, mux, txnID, c.splits)
			if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), c.want) {
				t.Fatalf("status = %d, body %s; want 400 containing %q", rec.Code, rec.Body.String(), c.want)
			}
		})
	}

	if rec := putSplits(t, mux, uuid.New(), []any{}); rec.Code != http.StatusNotFound {
		t.Fatalf("unknown transaction: status = %d", rec.Code)
	}
}

func TestLedgerSplits_TransferLink(t *testing.T) {
	h, s := newTestHandler(t)
	mux := newMux(h)
	left, right := addAccount(t, s, "Main"), addAccount(t, s, "Savings")
	cash := addCategory(t, s, "Cash")
	leftID, rightID := uuid.New(), uuid.New()
	commitTransactions(t, s, left, LedgerTransaction{ID: leftID, AccountID: left, BookingDate: "2026-11-02", AmountMinor: -10000, Currency: "EUR"})
	commitTransactions(t, s, right, LedgerTransaction{ID: rightID, AccountID: right, BookingDate: "2026-11-02", AmountMinor: 10000, Currency: "EUR"})

	if rec := putSplits(t, mux, leftID, []any{part(-100, cash, "")}); rec.Code != http.StatusOK {
		t.Fatalf("split status = %d", rec.Code)
	}
	if _, err := s.LinkLedgerTransfer(t.Context(), testUserID, leftID, LedgerTransferLinkInput{PairedTransactionID: rightID}); err != nil {
		t.Fatalf("LinkLedgerTransfer: %v", err)
	}
	linked, _ := s.GetLedgerTransaction(t.Context(), testUserID, leftID)
	if len(linked.Splits) != 0 {
		t.Fatalf("linking a transfer kept the split: %+v", linked.Splits)
	}
	if rec := putSplits(t, mux, leftID, []any{part(-100, cash, "")}); rec.Code != http.StatusConflict {
		t.Fatalf("split of a linked transfer: status = %d, want 409", rec.Code)
	}
}

func TestLedgerSplits_DeleteCategory(t *testing.T) {
	_, s := newTestHandler(t)
	accountID := addAccount(t, s, "Main")
	groceries := addCategory(t, s, "Groceries")
	cash := addCategory(t, s, "Cash")
	txnID := uuid.New()
	commitTransactions(t, s, accountID, LedgerTransaction{
		ID: txnID, AccountID: accountID, BookingDate: "2026-11-02", AmountMinor: -17235, Currency: "EUR",
		CategoryID: &groceries, ReviewStatus: LedgerTransactionReviewConfirmed, CategorizationSource: LedgerCategorizationManual,
	})
	if _, err := s.SetLedgerTransactionSplits(t.Context(), testUserID, txnID, []LedgerTransactionSplit{{AmountMinor: -15000, CategoryID: &cash}}); err != nil {
		t.Fatalf("SetLedgerTransactionSplits: %v", err)
	}

	page, err := s.ListLedgerTransactionsFiltered(t.Context(), testUserID, LedgerTransactionListOptions{CategoryID: &cash})
	if err != nil || len(page.Items) != 1 {
		t.Fatalf("filter by part category: %d items, err %v", len(page.Items), err)
	}

	if err := s.DeleteLedgerCategory(t.Context(), testUserID, cash); err != nil {
		t.Fatalf("DeleteLedgerCategory: %v", err)
	}
	got, _ := s.GetLedgerTransaction(t.Context(), testUserID, txnID)
	if len(got.Splits) != 1 || got.Splits[0].CategoryID != nil || got.Splits[0].AmountMinor != -15000 {
		t.Fatalf("splits = %+v", got.Splits)
	}
	if got.ReviewStatus != LedgerTransactionReviewNeedsReview || got.CategoryID == nil || *got.CategoryID != groceries {
		t.Fatalf("transaction = %+v", got)
	}
}

func TestSanitizeImportedSplits(t *testing.T) {
	cat := LedgerCategory{ID: uuid.New()}
	unknown := uuid.New()
	pair := uuid.New()
	txns := []LedgerTransaction{
		{AmountMinor: -1000, Splits: []LedgerTransactionSplit{{AmountMinor: -100, CategoryID: &cat.ID}}},
		{AmountMinor: -1000, Splits: []LedgerTransactionSplit{{AmountMinor: -100}}},
		{AmountMinor: -1000, Splits: []LedgerTransactionSplit{{AmountMinor: -100, CategoryID: &unknown}}},
		{AmountMinor: -1000, Splits: []LedgerTransactionSplit{{AmountMinor: -1000, CategoryID: &cat.ID}}},
		{AmountMinor: -1000, TransferPairTransactionID: &pair, Splits: []LedgerTransactionSplit{{AmountMinor: -100, CategoryID: &cat.ID}}},
		{AmountMinor: -1000},
	}
	if dropped := sanitizeImportedSplits(txns, []LedgerCategory{cat}); dropped != 3 {
		t.Fatalf("dropped = %d, want 3", dropped)
	}
	kept := []int{len(txns[0].Splits), len(txns[1].Splits), len(txns[2].Splits), len(txns[3].Splits), len(txns[4].Splits)}
	if kept[0] != 1 || kept[1] != 1 || kept[2] != 0 || kept[3] != 0 || kept[4] != 0 {
		t.Fatalf("kept parts = %v", kept)
	}
}

func TestTransactionResult_Splits(t *testing.T) {
	cat, cash := uuid.New(), uuid.New()
	item := transactionResult(LedgerTransaction{
		AmountMinor: -17235, CategoryID: &cat,
		Splits: []LedgerTransactionSplit{{AmountMinor: -15000, CategoryID: &cash, Note: "Auszahlung"}},
	})
	raw, _ := json.Marshal(item)
	want := `"splits":[{"amount":-150,"categoryId":"` + cash.String() + `","note":"Auszahlung"}],"categoryAmount":-22.35`
	if !strings.Contains(string(raw), want) {
		t.Fatalf("tool item = %s, want it to contain %s", raw, want)
	}
}
