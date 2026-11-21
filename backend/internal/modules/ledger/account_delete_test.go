package ledger

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/dgraph-io/badger/v4"
	"github.com/google/uuid"
	"github.com/reusing-code/kontor/backend/internal/enablebanking"
	"github.com/reusing-code/kontor/backend/internal/modules/purchases"
	"github.com/reusing-code/kontor/backend/internal/storage"
)

const deleteTestIBAN = "DE89370400440532013000"

func addAccountWithIBAN(t *testing.T, s *testStores, name, iban string) uuid.UUID {
	t.Helper()
	now := time.Now().UTC()
	id := uuid.New()
	if err := s.CreateLedgerAccount(t.Context(), testUserID, LedgerAccount{ID: id, Name: name, Bank: "DKB", IBAN: iban, Currency: "EUR", CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatalf("CreateLedgerAccount: %v", err)
	}
	return id
}

// keysWith counts the user's ledger keys whose path contains sub.
func keysWith(t *testing.T, s *testStores, sub string) int {
	t.Helper()
	prefix := []byte("u/" + testUserID + "/mod/ledger/")
	n := 0
	if err := s.e.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.PrefetchValues = false
		it := txn.NewIterator(opts)
		defer it.Close()
		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			if strings.Contains(string(it.Item().Key()), sub) {
				n++
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestDeleteLedgerAccount_RemovesEverythingThatPointsAtIt(t *testing.T) {
	_, s := newTestHandler(t)
	ctx := t.Context()
	main := addAccountWithIBAN(t, s, "Main", deleteTestIBAN)
	other := addAccount(t, s, "Savings")

	referenced, transfer, ordered, plain := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	partner := uuid.New()
	batch := LedgerImportBatch{ID: uuid.New(), AccountID: main, SourceType: "dkb.csv", FileSHA256: "sha-main", Status: ImportStatusCommitted}
	mainTxns := []LedgerTransaction{
		{ID: referenced, AccountID: main, BookingDate: "2026-04-01", AmountMinor: -1000, Currency: "EUR", Fingerprint: "fp-r"},
		{ID: transfer, AccountID: main, BookingDate: "2026-04-02", AmountMinor: -5000, Currency: "EUR", Fingerprint: "fp-t"},
		{ID: ordered, AccountID: main, BookingDate: "2026-04-03", AmountMinor: -2500, Currency: "EUR", Fingerprint: "fp-o"},
		{ID: plain, AccountID: main, BookingDate: "2026-04-04", AmountMinor: -100, Currency: "EUR", Fingerprint: "fp-p"},
	}
	if _, err := s.CommitLedgerImport(ctx, testUserID, batch, mainTxns); err != nil {
		t.Fatalf("CommitLedgerImport: %v", err)
	}
	commitTransactions(t, s, other, LedgerTransaction{ID: partner, AccountID: other, BookingDate: "2026-04-02", AmountMinor: 5000, Currency: "EUR"})

	purchaseID := uuid.New()
	now := time.Now().UTC()
	if err := s.CreatePurchase(ctx, testUserID, purchases.Purchase{ID: purchaseID, CategoryID: uuid.New(), ItemName: "Desk", CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.UpdateLedgerTransactionDetails(ctx, testUserID, referenced, LedgerTransactionDetailsInput{
		References: []LedgerTransactionReference{{Type: LedgerReferencePurchase, TargetID: purchaseID}},
	}); err != nil {
		t.Fatalf("reference: %v", err)
	}
	if _, err := s.LinkLedgerTransfer(ctx, testUserID, transfer, LedgerTransferLinkInput{PairedTransactionID: partner}); err != nil {
		t.Fatalf("LinkLedgerTransfer: %v", err)
	}
	order := LedgerEmailOrder{ID: uuid.New(), EmailAccountID: uuid.New(), ImporterID: "llm", OrderDate: "2026-04-03", TotalMinor: 2500, Currency: "EUR", MatchStatus: LedgerEmailOrderStatusUnmatched, CreatedAt: now, UpdatedAt: now}
	if err := s.CreateLedgerEmailOrder(ctx, testUserID, order); err != nil {
		t.Fatal(err)
	}
	if _, err := s.LinkLedgerEmailOrder(ctx, testUserID, order.ID, LedgerEmailOrderLinkInput{TransactionIDs: []uuid.UUID{ordered}}); err != nil {
		t.Fatalf("LinkLedgerEmailOrder: %v", err)
	}

	summary, err := s.LedgerAccountDeleteSummary(ctx, testUserID, main)
	if err != nil {
		t.Fatal(err)
	}
	if summary.Transactions != 4 || summary.Imports != 1 || summary.References != 1 || summary.Transfers != 1 || summary.EmailOrders != 1 {
		t.Fatalf("summary = %+v", summary)
	}

	if err := s.DeleteLedgerAccount(ctx, testUserID, main); err != nil {
		t.Fatalf("DeleteLedgerAccount: %v", err)
	}

	if _, err := s.GetLedgerAccount(ctx, testUserID, main); !errors.Is(err, storage.ErrNotFound) {
		t.Fatalf("account still there: %v", err)
	}
	for _, id := range []uuid.UUID{referenced, transfer, ordered, plain} {
		if _, err := s.GetLedgerTransaction(ctx, testUserID, id); !errors.Is(err, storage.ErrNotFound) {
			t.Fatalf("transaction %s still there: %v", id, err)
		}
	}
	if p, _ := s.GetPurchase(ctx, testUserID, purchaseID); slices.Contains(p.LinkedTransactionIDs, referenced) {
		t.Fatalf("purchase still links the transaction: %v", p.LinkedTransactionIDs)
	}
	if p, _ := s.GetLedgerTransaction(ctx, testUserID, partner); p.TransferPairTransactionID != nil || p.SpecialCategory != "" || p.ReviewStatus != LedgerTransactionReviewNeedsReview {
		t.Fatalf("partner = %+v", p)
	}
	if o, _ := s.GetLedgerEmailOrder(ctx, testUserID, order.ID); len(o.LinkedTransactionIDs) != 0 || o.MatchStatus != LedgerEmailOrderStatusUnmatched {
		t.Fatalf("order = %+v", o)
	}
	for _, sub := range []string{"/acc/" + main.String(), "acc_txn/" + main.String(), "acc_iban/", "file_hash/sha-main", "imp_txn/" + batch.ID.String(), "/imp/" + batch.ID.String()} {
		if n := keysWith(t, s, sub); n != 0 {
			t.Fatalf("%d keys with %q left", n, sub)
		}
	}
	if n := keysWith(t, s, "/txn_fp/"); n != 1 {
		t.Fatalf("fingerprint keys = %d, want only the other account's", n)
	}

	// the same file and IBAN can be used again
	again := addAccountWithIBAN(t, s, "Main again", deleteTestIBAN)
	batch.AccountID = again
	for i := range mainTxns {
		mainTxns[i].AccountID = again
	}
	res, err := s.CommitLedgerImport(ctx, testUserID, batch, mainTxns)
	if err != nil || res.ImportedRows != 4 {
		t.Fatalf("re-import = %+v, %v", res, err)
	}
}

func TestDeleteLedgerAccount_InChunks(t *testing.T) {
	_, s := newTestHandler(t)
	main := addAccount(t, s, "Main")
	n := 2*deleteAccountChunk + 50
	txns := make([]LedgerTransaction, n)
	for i := range txns {
		txns[i] = LedgerTransaction{ID: uuid.New(), AccountID: main, BookingDate: fmt.Sprintf("2026-%02d-%02d", i%12+1, i%28+1), Currency: "EUR"}
	}
	commitTransactions(t, s, main, txns...)

	if err := s.DeleteLedgerAccount(t.Context(), testUserID, main); err != nil {
		t.Fatalf("DeleteLedgerAccount: %v", err)
	}
	if left := keysWith(t, s, "/txn/"); left != 0 {
		t.Fatalf("%d transactions left", left)
	}
	if err := s.DeleteLedgerAccount(t.Context(), testUserID, main); !errors.Is(err, storage.ErrNotFound) {
		t.Fatalf("second delete: %v", err)
	}
}

func TestDeleteLedgerAccount_ResyncImportsAgain(t *testing.T) {
	f := newSyncFixture(t)
	f.fake.SetTransactions("uid-1", []enablebanking.Transaction{
		apiTxn("a", "2026-04-01", "10.00", "DBIT"),
		apiTxn("b", "2026-04-02", "20.00", "DBIT"),
	})
	if s := f.sync(t); s.Imported != 2 {
		t.Fatalf("first sync = %+v", s)
	}

	if err := f.DeleteLedgerAccount(t.Context(), testUserID, f.ledgerID); err != nil {
		t.Fatalf("DeleteLedgerAccount: %v", err)
	}
	a := f.account(t, "uid-1")
	if a.LedgerAccountID != nil || a.Unmapped || a.LastSyncAttemptAt != nil || a.LastSyncStatus != "" {
		t.Fatalf("bank account after delete = %+v", a)
	}

	// a new ledger account with the IBAN is mapped again by IBAN and gets the full history
	f.ledgerID = f.addLedgerAccount(t, bankTestIBAN1, "EUR")
	if _, err := f.svc.Connections(t.Context(), testUserID); err != nil {
		t.Fatal(err)
	}
	s := f.sync(t)
	if s.Imported != 2 || s.Duplicates != 0 {
		t.Fatalf("sync after delete = %+v", s)
	}
	q := f.fake.TransactionQueries()
	if last := q[len(q)-1]; last.Strategy != "longest" {
		t.Fatalf("last query = %+v", last)
	}
	if got := len(f.stored(t)); got != 2 {
		t.Fatalf("stored %d", got)
	}
}

func TestDeleteLedgerAccount_FreesAbsorbedBankRows(t *testing.T) {
	_, s := newTestHandler(t)
	ctx := t.Context()
	main := addAccount(t, s, "Main")
	csv := LedgerTransaction{ID: uuid.New(), AccountID: main, BookingDate: "2026-04-01", AmountMinor: -1000, Currency: "EUR", SourceType: "dkb.csv"}
	commitTransactions(t, s, main, csv)
	row := func(account uuid.UUID) LedgerTransaction {
		return LedgerTransaction{ID: uuid.New(), AccountID: account, BookingDate: "2026-04-01", AmountMinor: -1000, Currency: "EUR", SourceType: string(SourceEnableBanking), Fingerprint: "eb1:row"}
	}
	res, err := s.CommitLedgerSync(ctx, testUserID, LedgerImportBatch{ID: uuid.New(), AccountID: main}, []LedgerTransaction{row(main)})
	if err != nil || res.CrossSourceDuplicates != 1 || keysWith(t, s, "/xdup/") != 1 {
		t.Fatalf("absorb = %+v, %v", res, err)
	}

	if err := s.DeleteLedgerAccount(ctx, testUserID, main); err != nil {
		t.Fatal(err)
	}
	if keysWith(t, s, "/xdup/")+keysWith(t, s, "/xabs/") != 0 {
		t.Fatal("absorption keys left")
	}
	again := addAccount(t, s, "Main again")
	res, err = s.CommitLedgerSync(ctx, testUserID, LedgerImportBatch{ID: uuid.New(), AccountID: again}, []LedgerTransaction{row(again)})
	if err != nil || res.Imported != 1 {
		t.Fatalf("sync after delete = %+v, %v", res, err)
	}
}

func TestCommitLedgerSync_RefusesDeletedAccount(t *testing.T) {
	_, s := newTestHandler(t)
	gone := uuid.New()
	_, err := s.CommitLedgerSync(t.Context(), testUserID, LedgerImportBatch{ID: uuid.New(), AccountID: gone}, []LedgerTransaction{{ID: uuid.New(), AccountID: gone, BookingDate: "2026-04-01", Currency: "EUR", Fingerprint: "eb1:x"}})
	if !errors.Is(err, storage.ErrNotFound) {
		t.Fatalf("err = %v, want not found", err)
	}
	if n := keysWith(t, s, "/txn/"); n != 0 {
		t.Fatalf("%d transactions stored", n)
	}
}

func TestLedgerAccountHandlers_UpdateSummaryDelete(t *testing.T) {
	h, s := newTestHandler(t)
	mux := newMux(h)
	id := addAccountWithIBAN(t, s, "Main", deleteTestIBAN)
	commitTransactions(t, s, id, LedgerTransaction{ID: uuid.New(), AccountID: id, BookingDate: "2026-04-01", Currency: "EUR"})
	path := "/api/v1/ledger/accounts/" + id.String()

	do := func(method, p string, body any) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		if body == nil {
			mux.ServeHTTP(rec, httptest.NewRequest(method, p, nil))
		} else {
			mux.ServeHTTP(rec, httptest.NewRequest(method, p, jsonBody(body)))
		}
		return rec
	}

	rec := do("PUT", path, map[string]string{"name": "  Giro  ", "bank": "DKB AG"})
	if rec.Code != http.StatusOK {
		t.Fatalf("update = %d %s", rec.Code, rec.Body.String())
	}
	if a := decodeJSON[LedgerAccount](t, rec); a.Name != "Giro" || a.Bank != "DKB AG" || a.IBAN != deleteTestIBAN || a.Currency != "EUR" {
		t.Fatalf("updated = %+v", a)
	}
	for _, body := range []any{map[string]string{"name": "", "bank": "x"}, map[string]string{"name": "x", "bank": " "}, map[string]string{"name": "x", "bank": "y", "iban": "DE00"}} {
		if rec := do("PUT", path, body); rec.Code != http.StatusBadRequest {
			t.Fatalf("update %v = %d", body, rec.Code)
		}
	}
	if rec := do("PUT", "/api/v1/ledger/accounts/"+uuid.NewString(), map[string]string{"name": "x", "bank": "y"}); rec.Code != http.StatusNotFound {
		t.Fatalf("update unknown = %d", rec.Code)
	}

	rec = do("GET", path+"/delete-summary", nil)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"transactions":1`) || !strings.Contains(rec.Body.String(), `"bankAccounts":[]`) {
		t.Fatalf("summary = %d %s", rec.Code, rec.Body.String())
	}

	if rec := do("DELETE", path, nil); rec.Code != http.StatusNoContent {
		t.Fatalf("delete = %d %s", rec.Code, rec.Body.String())
	}
	if rec := do("DELETE", path, nil); rec.Code != http.StatusNotFound {
		t.Fatalf("second delete = %d", rec.Code)
	}
	if rec := do("GET", path+"/delete-summary", nil); rec.Code != http.StatusNotFound {
		t.Fatalf("summary after delete = %d", rec.Code)
	}
}
