package ledger

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
	"time"

	"context"

	"github.com/dgraph-io/badger/v4"
	"github.com/google/uuid"
	"github.com/reusing-code/kontor/backend/internal/modules/purchases"
	"github.com/reusing-code/kontor/backend/internal/storage"
)

type fakeEnablement struct{ disabled map[string]bool }

func (f fakeEnablement) ModuleEnabled(_ context.Context, _, moduleID string) (bool, error) {
	return !f.disabled[moduleID], nil
}

func postReference(mux http.Handler, txnID string, body any) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/v1/ledger/transactions/"+txnID+"/references", jsonBody(body))
	mux.ServeHTTP(rec, req)
	return rec
}

func addPurchase(t *testing.T, s *testStores) uuid.UUID {
	t.Helper()
	now := time.Now().UTC()
	id := uuid.New()
	if err := s.CreatePurchase(t.Context(), testUserID, purchases.Purchase{
		ID: id, CategoryID: uuid.New(), ItemName: "Desk", CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("CreatePurchase: %v", err)
	}
	return id
}

func addTransaction(t *testing.T, s *testStores, accountID uuid.UUID, mutate func(*LedgerTransaction)) uuid.UUID {
	t.Helper()
	txn := LedgerTransaction{
		ID: uuid.New(), AccountID: accountID, BookingDate: "2026-04-01", Currency: "EUR",
		Fingerprint: "fp-" + uuid.NewString(),
	}
	if mutate != nil {
		mutate(&txn)
	}
	commitTransactions(t, s, accountID, txn)
	return txn.ID
}

func TestAddReference_LinksBothSidesAndIsIdempotent(t *testing.T) {
	h, s := newTestHandler(t)
	mux := newMux(h)
	accountID := addAccount(t, s, "Main")
	txnID := addTransaction(t, s, accountID, nil)
	purchaseID := addPurchase(t, s)
	body := map[string]any{"type": LedgerReferencePurchase, "targetId": purchaseID.String()}

	for range 2 {
		rec := postReference(mux, txnID.String(), body)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, body = %s", rec.Code, rec.Body)
		}
		got := decodeJSON[LedgerTransaction](t, rec)
		if len(got.References) != 1 || got.References[0].TargetID != purchaseID {
			t.Fatalf("references = %#v", got.References)
		}
	}

	stored, err := s.GetLedgerTransaction(t.Context(), testUserID, txnID)
	if err != nil || len(stored.References) != 1 {
		t.Fatalf("stored = %#v, %v", stored, err)
	}
	purchase, err := s.GetPurchase(t.Context(), testUserID, purchaseID)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(purchase.LinkedTransactionIDs, []uuid.UUID{txnID}) {
		t.Fatalf("back-links = %v", purchase.LinkedTransactionIDs)
	}
}

func TestAddReference_KeepsExistingReferences(t *testing.T) {
	h, s := newTestHandler(t)
	mux := newMux(h)
	accountID := addAccount(t, s, "Main")
	txnID := addTransaction(t, s, accountID, nil)
	first, second := addPurchase(t, s), addPurchase(t, s)

	for _, id := range []uuid.UUID{first, second} {
		if rec := postReference(mux, txnID.String(), map[string]any{"type": "purchase", "targetId": id.String()}); rec.Code != http.StatusOK {
			t.Fatalf("status = %d, body = %s", rec.Code, rec.Body)
		}
	}
	stored, _ := s.GetLedgerTransaction(t.Context(), testUserID, txnID)
	if len(stored.References) != 2 {
		t.Fatalf("references = %#v", stored.References)
	}
}

func TestAddReference_Errors(t *testing.T) {
	h, s := newTestHandler(t)
	mux := newMux(h)
	accountID := addAccount(t, s, "Main")
	txnID := addTransaction(t, s, accountID, nil)
	purchaseID := addPurchase(t, s)

	tests := []struct {
		name string
		txn  string
		body any
		want int
	}{
		{"unknown transaction", uuid.NewString(), map[string]any{"type": "purchase", "targetId": purchaseID.String()}, 404},
		{"unknown target", txnID.String(), map[string]any{"type": "purchase", "targetId": uuid.NewString()}, 404},
		{"invalid type", txnID.String(), map[string]any{"type": "cost", "targetId": purchaseID.String()}, 400},
		{"missing target id", txnID.String(), map[string]any{"type": "purchase"}, 400},
		{"bad transaction id", "nope", map[string]any{"type": "purchase", "targetId": purchaseID.String()}, 400},
		{"unknown field", txnID.String(), map[string]any{"type": "purchase", "targetId": purchaseID.String(), "note": "x"}, 400},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if rec := postReference(mux, tt.txn, tt.body); rec.Code != tt.want {
				t.Fatalf("status = %d, want %d, body = %s", rec.Code, tt.want, rec.Body)
			}
		})
	}

	stored, _ := s.GetLedgerTransaction(t.Context(), testUserID, txnID)
	if len(stored.References) != 0 {
		t.Fatalf("failed requests stored references: %#v", stored.References)
	}
}

func TestAddReference_NotFoundCausesAreDistinguishable(t *testing.T) {
	_, s := newTestHandler(t)
	accountID := addAccount(t, s, "Main")
	txnID := addTransaction(t, s, accountID, nil)

	_, _, err := s.AddLedgerTransactionReference(t.Context(), testUserID, uuid.New(), LedgerTransactionReference{Type: "purchase", TargetID: uuid.New()})
	if err == nil || err.Error() != "not found: transaction" {
		t.Fatalf("unknown transaction err = %v", err)
	}
	_, _, err = s.AddLedgerTransactionReference(t.Context(), testUserID, txnID, LedgerTransactionReference{Type: "purchase", TargetID: uuid.New()})
	if err == nil || err.Error() != "not found: target" {
		t.Fatalf("unknown target err = %v", err)
	}
}

func TestAddReference_DisabledTargetModule(t *testing.T) {
	h, s := newTestHandler(t)
	h.enabled = fakeEnablement{disabled: map[string]bool{"purchases": true}}
	mux := newMux(h)
	accountID := addAccount(t, s, "Main")
	txnID := addTransaction(t, s, accountID, nil)
	purchaseID := addPurchase(t, s)

	rec := postReference(mux, txnID.String(), map[string]any{"type": "purchase", "targetId": purchaseID.String()})
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body)
	}
	if got := decodeJSON[map[string]string](t, rec); got["code"] != "module_disabled" {
		t.Fatalf("body = %v", got)
	}
	stored, _ := s.GetLedgerTransaction(t.Context(), testUserID, txnID)
	if len(stored.References) != 0 {
		t.Fatal("reference stored despite disabled module")
	}

	h.enabled = fakeEnablement{disabled: map[string]bool{"contracts": true}}
	if rec := postReference(mux, txnID.String(), map[string]any{"type": "purchase", "targetId": purchaseID.String()}); rec.Code != http.StatusOK {
		t.Fatalf("other module disabled: status = %d", rec.Code)
	}
}

func TestAddReference_LinkedTransferKeepsProtections(t *testing.T) {
	h, s := newTestHandler(t)
	mux := newMux(h)
	accountA, accountB := addAccount(t, s, "Checking"), addAccount(t, s, "Savings")
	leftID := addTransaction(t, s, accountA, func(x *LedgerTransaction) {
		x.AmountMinor = -1000
		x.Note = "keep me"
		x.Links = []string{"https://example.com/a"}
	})
	rightID := addTransaction(t, s, accountB, func(x *LedgerTransaction) { x.AmountMinor = 1000 })
	if _, err := s.LinkLedgerTransfer(t.Context(), testUserID, leftID, LedgerTransferLinkInput{PairedTransactionID: rightID}); err != nil {
		t.Fatalf("LinkLedgerTransfer: %v", err)
	}
	before, _ := s.GetLedgerTransaction(t.Context(), testUserID, leftID)

	purchaseID := addPurchase(t, s)
	if rec := postReference(mux, leftID.String(), map[string]any{"type": "purchase", "targetId": purchaseID.String()}); rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body)
	}

	after, _ := s.GetLedgerTransaction(t.Context(), testUserID, leftID)
	if after.TransferPairTransactionID == nil || *after.TransferPairTransactionID != rightID {
		t.Fatalf("transfer pair lost: %v", after.TransferPairTransactionID)
	}
	if after.SpecialCategory != LedgerSpecialCategoryInternalTransfer || after.SpecialCategory != before.SpecialCategory {
		t.Fatalf("special category = %q", after.SpecialCategory)
	}
	if after.CategoryID != before.CategoryID || after.Note != "keep me" || len(after.Links) != 1 {
		t.Fatalf("other fields changed: %#v", after)
	}
}

func TestUpdateDetails_StillReplacesReferences(t *testing.T) {
	h, s := newTestHandler(t)
	mux := newMux(h)
	accountID := addAccount(t, s, "Main")
	txnID := addTransaction(t, s, accountID, nil)
	purchaseID := addPurchase(t, s)
	if rec := postReference(mux, txnID.String(), map[string]any{"type": "purchase", "targetId": purchaseID.String()}); rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("PUT", "/api/v1/ledger/transactions/"+txnID.String(), jsonBody(map[string]any{"note": "n"})))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if got := decodeJSON[LedgerTransaction](t, rec); len(got.References) != 0 {
		t.Fatalf("PUT should drop references as before: %#v", got.References)
	}
	purchase, _ := s.GetPurchase(t.Context(), testUserID, purchaseID)
	if len(purchase.LinkedTransactionIDs) != 0 {
		t.Fatalf("back-link left behind: %v", purchase.LinkedTransactionIDs)
	}
}

func TestAddReference_ReportsWhetherItAdded(t *testing.T) {
	_, s := newTestHandler(t)
	accountID := addAccount(t, s, "Main")
	txnID := addTransaction(t, s, accountID, nil)
	ref := LedgerTransactionReference{Type: "purchase", TargetID: addPurchase(t, s)}

	if _, added, err := s.AddLedgerTransactionReference(t.Context(), testUserID, txnID, ref); err != nil || !added {
		t.Fatalf("first: added=%v err=%v", added, err)
	}
	if _, added, err := s.AddLedgerTransactionReference(t.Context(), testUserID, txnID, ref); err != nil || added {
		t.Fatalf("repeat: added=%v err=%v", added, err)
	}
}

func TestAddReference_ConcurrentCallsAddExactlyOnce(t *testing.T) {
	_, s := newTestHandler(t)
	accountID := addAccount(t, s, "Main")
	txnID := addTransaction(t, s, accountID, nil)
	ref := LedgerTransactionReference{Type: "purchase", TargetID: addPurchase(t, s)}

	const workers = 8
	results := make(chan bool, workers)
	for range workers {
		go func() {
			_, added, err := s.AddLedgerTransactionReference(t.Context(), testUserID, txnID, ref)
			if err != nil {
				t.Errorf("add: %v", err)
			}
			results <- added
		}()
	}
	added := 0
	for range workers {
		if <-results {
			added++
		}
	}
	if added != 1 {
		t.Fatalf("%d callers reported added=true, want exactly 1", added)
	}
}

func TestAddReference_RetryExhaustionIsA409(t *testing.T) {
	h := &Handler{logger: slog.New(slog.DiscardHandler)}
	rec := httptest.NewRecorder()
	h.storeError(rec, storage.RetryConflicts(t.Context(), 1, func() error { return badger.ErrConflict }))
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409", rec.Code)
	}
}
