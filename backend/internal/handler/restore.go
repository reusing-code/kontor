package handler

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/google/uuid"
	"github.com/reusing-code/kontor/backend/internal/middleware"
	"github.com/reusing-code/kontor/backend/internal/model"
	"github.com/reusing-code/kontor/backend/internal/store"
)

type restoreResult struct {
	Restored map[string]int `json:"restored"`
	Warnings []string       `json:"warnings"`
}

// Restore recreates a full data export for the authenticated user. It only
// runs against an account without data, so original IDs (and with them all
// cross-references) can be preserved verbatim. Seeded default categories are
// replaced by the exported ones.
func (h *Handler) Restore(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := middleware.GetUserID(ctx)

	var payload exportPayload
	if err := h.readJSON(r, &payload); err != nil {
		h.errorResponse(w, http.StatusBadRequest, "invalid export file")
		return
	}

	empty, err := h.accountIsEmpty(ctx, userID)
	if err != nil {
		h.handleStoreError(w, err)
		return
	}
	if !empty {
		h.errorResponse(w, http.StatusConflict, "restore requires an account without data")
		return
	}

	result := restoreResult{Restored: map[string]int{}, Warnings: []string{}}

	if err := h.store.UpdateSettings(ctx, userID, payload.Settings); err != nil {
		h.handleStoreError(w, err)
		return
	}

	if err := h.restoreModuleCategories(ctx, userID, "contracts", payload.ContractCategories, &result); err != nil {
		h.handleStoreError(w, err)
		return
	}
	if err := h.restoreModuleCategories(ctx, userID, "purchases", payload.PurchaseCategories, &result); err != nil {
		h.handleStoreError(w, err)
		return
	}
	if err := h.restoreLedgerCategories(ctx, userID, payload.LedgerCategories, &result); err != nil {
		h.handleStoreError(w, err)
		return
	}

	for _, contract := range payload.Contracts {
		if err := h.store.CreateContract(ctx, userID, contract); err != nil {
			h.handleStoreError(w, err)
			return
		}
		result.Restored["contracts"]++
	}
	for _, purchase := range payload.Purchases {
		if err := h.store.CreatePurchase(ctx, userID, purchase); err != nil {
			h.handleStoreError(w, err)
			return
		}
		result.Restored["purchases"]++
	}
	for _, vehicle := range payload.Vehicles {
		if err := h.store.CreateVehicle(ctx, userID, vehicle); err != nil {
			h.handleStoreError(w, err)
			return
		}
		result.Restored["vehicles"]++
	}
	for _, entry := range payload.CostEntries {
		if err := h.store.CreateCostEntry(ctx, userID, entry); err != nil {
			h.handleStoreError(w, err)
			return
		}
		result.Restored["costEntries"]++
	}
	for _, account := range payload.LedgerAccounts {
		if err := h.store.CreateLedgerAccount(ctx, userID, account); err != nil {
			h.handleStoreError(w, err)
			return
		}
		result.Restored["ledgerAccounts"]++
	}

	if err := h.restoreLedgerTransactions(ctx, userID, payload, &result); err != nil {
		h.handleStoreError(w, err)
		return
	}

	for _, account := range payload.LedgerEmailAccounts {
		if err := h.store.CreateLedgerEmailAccount(ctx, userID, account); err != nil {
			h.handleStoreError(w, err)
			return
		}
		result.Restored["ledgerEmailAccounts"]++
	}
	if len(payload.LedgerEmailAccounts) > 0 {
		result.Warnings = append(result.Warnings, "email account passwords are not part of exports and must be re-entered")
	}
	for _, order := range payload.LedgerEmailOrders {
		if err := h.store.CreateLedgerEmailOrder(ctx, userID, order); err != nil {
			h.handleStoreError(w, err)
			return
		}
		result.Restored["ledgerEmailOrders"]++
	}

	h.writeJSON(w, http.StatusOK, result)
}

func (h *Handler) accountIsEmpty(ctx context.Context, userID string) (bool, error) {
	contracts, err := h.store.ListContracts(ctx, userID)
	if err != nil {
		return false, err
	}
	purchases, err := h.store.ListPurchases(ctx, userID)
	if err != nil {
		return false, err
	}
	vehicles, err := h.store.ListVehicles(ctx, userID)
	if err != nil {
		return false, err
	}
	accounts, err := h.store.ListLedgerAccounts(ctx, userID)
	if err != nil {
		return false, err
	}
	imports, err := h.store.ListLedgerImports(ctx, userID)
	if err != nil {
		return false, err
	}
	emailAccounts, err := h.store.ListLedgerEmailAccounts(ctx, userID)
	if err != nil {
		return false, err
	}
	emailOrders, err := h.store.ListLedgerEmailOrders(ctx, userID)
	if err != nil {
		return false, err
	}
	return len(contracts) == 0 && len(purchases) == 0 && len(vehicles) == 0 &&
		len(accounts) == 0 && len(imports) == 0 && len(emailAccounts) == 0 && len(emailOrders) == 0, nil
}

// restoreModuleCategories replaces the seeded default categories with the
// exported ones so item category references resolve again.
func (h *Handler) restoreModuleCategories(ctx context.Context, userID string, module string, categories []model.Category, result *restoreResult) error {
	existing, err := h.store.ListCategories(ctx, userID, module)
	if err != nil {
		return err
	}
	for _, category := range existing {
		if err := h.store.DeleteCategory(ctx, userID, module, category.ID); err != nil {
			return err
		}
	}
	for _, category := range categories {
		if err := h.store.CreateCategory(ctx, userID, module, category); err != nil {
			return err
		}
		result.Restored[module+"Categories"]++
	}
	return nil
}

func (h *Handler) restoreLedgerCategories(ctx context.Context, userID string, categories []model.LedgerCategory, result *restoreResult) error {
	existing, err := h.store.ListLedgerCategories(ctx, userID)
	if err != nil {
		return err
	}
	// children-first deletion: retry until the tree is gone
	for len(existing) > 0 {
		remaining := make([]model.LedgerCategory, 0, len(existing))
		for _, category := range existing {
			err := h.store.DeleteLedgerCategory(ctx, userID, category.ID)
			if errors.Is(err, store.ErrLedgerCategoryHasChild) {
				remaining = append(remaining, category)
				continue
			}
			if err != nil {
				return err
			}
		}
		if len(remaining) == len(existing) {
			return errors.New("could not delete existing ledger categories")
		}
		existing = remaining
	}

	// parents-first creation
	created := make(map[uuid.UUID]bool, len(categories))
	pending := categories
	for len(pending) > 0 {
		remaining := make([]model.LedgerCategory, 0, len(pending))
		for _, category := range pending {
			if category.ParentID != nil && !created[*category.ParentID] {
				remaining = append(remaining, category)
				continue
			}
			if err := h.store.CreateLedgerCategory(ctx, userID, category); err != nil {
				return err
			}
			created[category.ID] = true
			result.Restored["ledgerCategories"]++
		}
		if len(remaining) == len(pending) {
			return errors.New("ledger categories contain an unresolvable parent reference")
		}
		pending = remaining
	}
	return nil
}

func (h *Handler) restoreLedgerTransactions(ctx context.Context, userID string, payload exportPayload, result *restoreResult) error {
	byBatch := make(map[uuid.UUID][]model.LedgerTransaction)
	for _, txn := range payload.LedgerTransactions {
		byBatch[txn.ImportBatchID] = append(byBatch[txn.ImportBatchID], txn)
	}
	for _, batch := range payload.LedgerImports {
		commit, err := h.store.CommitLedgerImport(ctx, userID, batch, byBatch[batch.ID])
		if err != nil {
			return err
		}
		result.Restored["ledgerImports"]++
		result.Restored["ledgerTransactions"] += commit.ImportedRows
		delete(byBatch, batch.ID)
	}
	// transactions whose batch is missing from the export get a synthetic batch
	for batchID, txns := range byBatch {
		if len(txns) == 0 {
			continue
		}
		synthetic := model.LedgerImportBatch{
			ID:         batchID,
			AccountID:  txns[0].AccountID,
			SourceType: "restore",
			Filename:   "restored-orphan-transactions",
			FileSHA256: fmt.Sprintf("restore-%s", batchID),
			Status:     "committed",
			TotalRows:  len(txns),
		}
		commit, err := h.store.CommitLedgerImport(ctx, userID, synthetic, txns)
		if err != nil {
			return err
		}
		result.Restored["ledgerTransactions"] += commit.ImportedRows
		result.Warnings = append(result.Warnings, fmt.Sprintf("%d transaction(s) referenced missing import batch %s", len(txns), batchID))
	}
	return nil
}
