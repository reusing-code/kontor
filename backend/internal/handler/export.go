package handler

import (
	"fmt"
	"net/http"
	"time"

	"github.com/tobi/contracts/backend/internal/middleware"
	"github.com/tobi/contracts/backend/internal/model"
	"github.com/tobi/contracts/backend/internal/version"
)

type exportPayload struct {
	ExportedAt          time.Time                 `json:"exportedAt"`
	AppVersion          string                    `json:"appVersion"`
	Settings            model.UserSettings        `json:"settings"`
	ContractCategories  []model.Category          `json:"contractCategories"`
	Contracts           []model.Contract          `json:"contracts"`
	PurchaseCategories  []model.Category          `json:"purchaseCategories"`
	Purchases           []model.Purchase          `json:"purchases"`
	Vehicles            []model.Vehicle           `json:"vehicles"`
	CostEntries         []model.CostEntry         `json:"costEntries"`
	LedgerAccounts      []model.LedgerAccount     `json:"ledgerAccounts"`
	LedgerCategories    []model.LedgerCategory    `json:"ledgerCategories"`
	LedgerImports       []model.LedgerImportBatch `json:"ledgerImports"`
	LedgerTransactions  []model.LedgerTransaction `json:"ledgerTransactions"`
	LedgerEmailAccounts []model.LedgerEmailAccount `json:"ledgerEmailAccounts"`
	LedgerEmailOrders   []model.LedgerEmailOrder  `json:"ledgerEmailOrders"`
}

// Export streams all data belonging to the authenticated user as a JSON download.
// Encrypted email passwords are excluded via their json:"-" tag.
func (h *Handler) Export(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := middleware.GetUserID(ctx)

	payload := exportPayload{
		ExportedAt: time.Now().UTC(),
		AppVersion: version.Get().Version,
	}

	var err error
	if payload.Settings, err = h.store.GetSettings(ctx, userID); err != nil {
		h.handleStoreError(w, err)
		return
	}
	if payload.ContractCategories, err = h.store.ListCategories(ctx, userID, "contracts"); err != nil {
		h.handleStoreError(w, err)
		return
	}
	if payload.Contracts, err = h.store.ListContracts(ctx, userID); err != nil {
		h.handleStoreError(w, err)
		return
	}
	if payload.PurchaseCategories, err = h.store.ListCategories(ctx, userID, "purchases"); err != nil {
		h.handleStoreError(w, err)
		return
	}
	if payload.Purchases, err = h.store.ListPurchases(ctx, userID); err != nil {
		h.handleStoreError(w, err)
		return
	}
	if payload.Vehicles, err = h.store.ListVehicles(ctx, userID); err != nil {
		h.handleStoreError(w, err)
		return
	}
	payload.CostEntries = make([]model.CostEntry, 0)
	for _, vehicle := range payload.Vehicles {
		entries, err := h.store.ListCostEntries(ctx, userID, vehicle.ID)
		if err != nil {
			h.handleStoreError(w, err)
			return
		}
		payload.CostEntries = append(payload.CostEntries, entries...)
	}
	if payload.LedgerAccounts, err = h.store.ListLedgerAccounts(ctx, userID); err != nil {
		h.handleStoreError(w, err)
		return
	}
	if payload.LedgerCategories, err = h.store.ListLedgerCategories(ctx, userID); err != nil {
		h.handleStoreError(w, err)
		return
	}
	if payload.LedgerImports, err = h.store.ListLedgerImports(ctx, userID); err != nil {
		h.handleStoreError(w, err)
		return
	}
	payload.LedgerTransactions = make([]model.LedgerTransaction, 0)
	for _, account := range payload.LedgerAccounts {
		txns, err := h.store.ListLedgerTransactions(ctx, userID, account.ID)
		if err != nil {
			h.handleStoreError(w, err)
			return
		}
		payload.LedgerTransactions = append(payload.LedgerTransactions, txns...)
	}
	if payload.LedgerEmailAccounts, err = h.store.ListLedgerEmailAccounts(ctx, userID); err != nil {
		h.handleStoreError(w, err)
		return
	}
	if payload.LedgerEmailOrders, err = h.store.ListLedgerEmailOrders(ctx, userID); err != nil {
		h.handleStoreError(w, err)
		return
	}

	filename := fmt.Sprintf("contracts-export-%s.json", payload.ExportedAt.Format("20060102-150405"))
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
	h.writeJSON(w, http.StatusOK, payload)
}
