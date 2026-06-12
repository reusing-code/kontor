package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/tobi/contracts/backend/internal/middleware"
	"github.com/tobi/contracts/backend/internal/model"
)

func newExportMux(h *Handler) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/export", h.Export)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := middleware.SetUserID(r.Context(), testUserID)
		mux.ServeHTTP(w, r.WithContext(ctx))
	})
}

func TestExport_IncludesAllUserData(t *testing.T) {
	h, ms := newTestHandler()

	contract := model.Contract{ID: uuid.New(), Name: "Netflix"}
	ms.contracts[contract.ID] = contract
	purchase := model.Purchase{ID: uuid.New(), ItemName: "Monitor"}
	ms.purchases[purchase.ID] = purchase
	vehicle := model.Vehicle{ID: uuid.New(), Name: "Car"}
	ms.vehicles[vehicle.ID] = vehicle
	account := model.LedgerAccount{ID: uuid.New(), Name: "Checking", Currency: "EUR"}
	ms.ledgerAccounts[account.ID] = account
	txn := model.LedgerTransaction{ID: uuid.New(), AccountID: account.ID, BookingDate: "2026-06-01", AmountMinor: -1999}
	ms.ledgerTransactions[account.ID] = []model.LedgerTransaction{txn}
	emailAccount := model.LedgerEmailAccount{ID: uuid.New(), Name: "Mailbox", EncryptedPassword: "super-secret-ciphertext"}
	ms.ledgerEmailAccounts[emailAccount.ID] = emailAccount

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/v1/export", nil)
	newExportMux(h).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (body: %s)", rec.Code, http.StatusOK, rec.Body.String())
	}
	if disposition := rec.Header().Get("Content-Disposition"); !strings.Contains(disposition, "attachment") {
		t.Errorf("Content-Disposition = %q, want attachment", disposition)
	}
	if strings.Contains(rec.Body.String(), "super-secret-ciphertext") {
		t.Error("export must not contain encrypted email passwords")
	}

	payload := decodeJSON[exportPayload](t, rec)
	if payload.ExportedAt.IsZero() {
		t.Error("exportedAt is zero")
	}
	if len(payload.Contracts) != 1 || payload.Contracts[0].Name != "Netflix" {
		t.Errorf("contracts = %+v, want one named Netflix", payload.Contracts)
	}
	if len(payload.Purchases) != 1 || payload.Purchases[0].ItemName != "Monitor" {
		t.Errorf("purchases = %+v, want one named Monitor", payload.Purchases)
	}
	if len(payload.Vehicles) != 1 {
		t.Errorf("vehicles = %+v, want one entry", payload.Vehicles)
	}
	if len(payload.LedgerAccounts) != 1 {
		t.Errorf("ledgerAccounts = %+v, want one entry", payload.LedgerAccounts)
	}
	if len(payload.LedgerTransactions) != 1 || payload.LedgerTransactions[0].AmountMinor != -1999 {
		t.Errorf("ledgerTransactions = %+v, want one entry with amount -1999", payload.LedgerTransactions)
	}
	if len(payload.LedgerEmailAccounts) != 1 {
		t.Errorf("ledgerEmailAccounts = %+v, want one entry", payload.LedgerEmailAccounts)
	}
}

func TestExport_EmptyUserSucceeds(t *testing.T) {
	h, _ := newTestHandler()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/v1/export", nil)
	newExportMux(h).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (body: %s)", rec.Code, http.StatusOK, rec.Body.String())
	}
	payload := decodeJSON[exportPayload](t, rec)
	if payload.Contracts == nil && payload.LedgerTransactions == nil && payload.CostEntries == nil {
		// at minimum the explicitly initialized slices must encode as arrays
		t.Error("expected initialized slices in empty export")
	}
}
