package handler

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/tobi/contracts/backend/internal/middleware"
	"github.com/tobi/contracts/backend/internal/model"
)

func newExportRestoreMux(h *Handler) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/export", h.Export)
	mux.HandleFunc("POST /api/v1/restore", h.Restore)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := middleware.SetUserID(r.Context(), testUserID)
		mux.ServeHTTP(w, r.WithContext(ctx))
	})
}

func TestRestore_RoundTrip(t *testing.T) {
	source, sourceStore := newTestHandler()

	category := model.Category{ID: uuid.New(), Name: "Streaming"}
	sourceStore.addCategory("contracts", category)
	contract := model.Contract{ID: uuid.New(), CategoryID: category.ID, Name: "Netflix"}
	sourceStore.contracts[contract.ID] = contract

	parentCategory := model.LedgerCategory{ID: uuid.New(), Name: "Living"}
	childCategory := model.LedgerCategory{ID: uuid.New(), Name: "Rent", ParentID: &parentCategory.ID}
	sourceStore.ledgerCategories[parentCategory.ID] = parentCategory
	sourceStore.ledgerCategories[childCategory.ID] = childCategory

	account := model.LedgerAccount{ID: uuid.New(), Name: "Checking", Currency: "EUR"}
	sourceStore.ledgerAccounts[account.ID] = account
	batch := model.LedgerImportBatch{ID: uuid.New(), AccountID: account.ID, SourceType: "dkb.csv", FileSHA256: "abc"}
	sourceStore.ledgerImports = append(sourceStore.ledgerImports, batch)
	txn := model.LedgerTransaction{ID: uuid.New(), AccountID: account.ID, ImportBatchID: batch.ID, BookingDate: "2026-06-01", AmountMinor: -4200}
	sourceStore.ledgerTransactions[account.ID] = []model.LedgerTransaction{txn}

	emailAccount := model.LedgerEmailAccount{ID: uuid.New(), Name: "Mailbox", EncryptedPassword: "secret"}
	sourceStore.ledgerEmailAccounts[emailAccount.ID] = emailAccount
	order := model.LedgerEmailOrder{ID: uuid.New(), EmailAccountID: emailAccount.ID, OrderDate: "2026-06-01", TotalMinor: 4200, LinkedTransactionIDs: []uuid.UUID{txn.ID}}
	sourceStore.ledgerEmailOrders[order.ID] = order

	rec := httptest.NewRecorder()
	newExportRestoreMux(source).ServeHTTP(rec, httptest.NewRequest("GET", "/api/v1/export", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("export status = %d", rec.Code)
	}
	exported := rec.Body.Bytes()

	target, targetStore := newTestHandler()
	// fresh accounts come with seeded defaults; restore must replace them
	targetStore.addCategory("contracts", model.Category{ID: uuid.New(), NameKey: "insurance"})

	rec = httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/v1/restore", bytes.NewReader(exported))
	newExportRestoreMux(target).ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("restore status = %d (body: %s)", rec.Code, rec.Body.String())
	}
	result := decodeJSON[restoreResult](t, rec)

	if got := targetStore.contracts[contract.ID]; got.Name != "Netflix" || got.CategoryID != category.ID {
		t.Errorf("restored contract = %+v", got)
	}
	if len(targetStore.categories["contracts"]) != 1 {
		t.Errorf("contract categories = %v, want seeded defaults replaced by 1 exported", targetStore.categories["contracts"])
	}
	if len(targetStore.ledgerCategories) != 2 {
		t.Errorf("ledger categories = %d, want 2", len(targetStore.ledgerCategories))
	}
	if _, ok := targetStore.ledgerAccounts[account.ID]; !ok {
		t.Error("ledger account missing after restore")
	}
	restoredTxns := targetStore.ledgerTransactions[account.ID]
	if len(restoredTxns) != 1 || restoredTxns[0].ID != txn.ID || restoredTxns[0].AmountMinor != -4200 {
		t.Errorf("restored transactions = %+v", restoredTxns)
	}
	if got := targetStore.ledgerEmailAccounts[emailAccount.ID]; got.EncryptedPassword != "" {
		t.Error("restored email account must not regain a password")
	}
	if got := targetStore.ledgerEmailOrders[order.ID]; len(got.LinkedTransactionIDs) != 1 || got.LinkedTransactionIDs[0] != txn.ID {
		t.Errorf("restored email order links = %+v", got.LinkedTransactionIDs)
	}
	if len(result.Warnings) == 0 {
		t.Error("expected password warning for restored email accounts")
	}
	if result.Restored["ledgerTransactions"] != 1 || result.Restored["contracts"] != 1 {
		t.Errorf("restored counts = %v", result.Restored)
	}
}

func TestRestore_RejectsNonEmptyAccount(t *testing.T) {
	h, ms := newTestHandler()
	contract := model.Contract{ID: uuid.New(), Name: "Existing"}
	ms.contracts[contract.ID] = contract

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/v1/restore", bytes.NewReader([]byte("{}")))
	newExportRestoreMux(h).ServeHTTP(rec, req)
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusConflict)
	}
}
