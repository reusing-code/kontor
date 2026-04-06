package handler

import (
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/tobi/contracts/backend/internal/middleware"
	"github.com/tobi/contracts/backend/internal/model"
	"github.com/tobi/contracts/backend/internal/store"
)

func (h *Handler) ListLedgerCategories(w http.ResponseWriter, r *http.Request) {
	items, err := h.store.ListLedgerCategories(r.Context(), middleware.GetUserID(r.Context()))
	if err != nil {
		h.handleStoreError(w, err)
		return
	}
	h.writeJSON(w, http.StatusOK, items)
}

func (h *Handler) GetLedgerCategory(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUID(r.PathValue("id"))
	if err != nil {
		h.errorResponse(w, http.StatusBadRequest, "invalid id")
		return
	}
	item, err := h.store.GetLedgerCategory(r.Context(), middleware.GetUserID(r.Context()), id)
	if err != nil {
		h.handleStoreError(w, err)
		return
	}
	h.writeJSON(w, http.StatusOK, item)
}

func (h *Handler) CreateLedgerCategory(w http.ResponseWriter, r *http.Request) {
	var input model.LedgerCategoryInput
	if err := h.readJSON(r, &input); err != nil {
		h.errorResponse(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := input.Validate(); err != nil {
		h.errorResponse(w, http.StatusBadRequest, err.Error())
		return
	}
	now := time.Now().UTC()
	category := model.LedgerCategory{
		ID:         uuid.New(),
		Name:       input.Name,
		ParentID:   input.ParentID,
		MatchWords: input.MatchWords,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	if err := h.store.CreateLedgerCategory(r.Context(), middleware.GetUserID(r.Context()), category); err != nil {
		h.handleStoreError(w, err)
		return
	}
	h.writeJSON(w, http.StatusCreated, category)
}

func (h *Handler) UpdateLedgerCategory(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUID(r.PathValue("id"))
	if err != nil {
		h.errorResponse(w, http.StatusBadRequest, "invalid id")
		return
	}
	existing, err := h.store.GetLedgerCategory(r.Context(), middleware.GetUserID(r.Context()), id)
	if err != nil {
		h.handleStoreError(w, err)
		return
	}
	var input model.LedgerCategoryInput
	if err := h.readJSON(r, &input); err != nil {
		h.errorResponse(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := input.Validate(); err != nil {
		h.errorResponse(w, http.StatusBadRequest, err.Error())
		return
	}
	existing.Name = input.Name
	existing.ParentID = input.ParentID
	existing.MatchWords = input.MatchWords
	existing.UpdatedAt = time.Now().UTC()
	if err := h.store.UpdateLedgerCategory(r.Context(), middleware.GetUserID(r.Context()), existing); err != nil {
		h.handleStoreError(w, err)
		return
	}
	h.writeJSON(w, http.StatusOK, existing)
}

func (h *Handler) DeleteLedgerCategory(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUID(r.PathValue("id"))
	if err != nil {
		h.errorResponse(w, http.StatusBadRequest, "invalid id")
		return
	}
	if err := h.store.DeleteLedgerCategory(r.Context(), middleware.GetUserID(r.Context()), id); err != nil {
		h.handleStoreError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type ledgerReviewResponse struct {
	Transaction model.LedgerTransaction `json:"transaction"`
	Category    *model.LedgerCategory   `json:"category,omitempty"`
}

func (h *Handler) ListLedgerTransactionsReviewQueue(w http.ResponseWriter, r *http.Request) {
	limit := 100
	if rawLimit := r.URL.Query().Get("limit"); rawLimit != "" {
		parsed, err := parseInt(rawLimit)
		if err != nil || parsed <= 0 {
			h.errorResponse(w, http.StatusBadRequest, "invalid limit")
			return
		}
		limit = parsed
	}
	page, err := h.store.ListLedgerTransactionsFiltered(r.Context(), middleware.GetUserID(r.Context()), store.LedgerTransactionListOptions{
		ReviewStatus: model.LedgerTransactionReviewNeedsReview,
		Limit:        limit,
		Cursor:       r.URL.Query().Get("cursor"),
	})
	if err != nil {
		h.handleStoreError(w, err)
		return
	}
	h.writeJSON(w, http.StatusOK, ledgerTransactionPageResponse{Items: page.Items, NextCursor: page.NextCursor})
}

func (h *Handler) GetLedgerTransaction(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUID(r.PathValue("transactionId"))
	if err != nil {
		h.errorResponse(w, http.StatusBadRequest, "invalid transactionId")
		return
	}
	ledgerTxn, err := h.store.GetLedgerTransaction(r.Context(), middleware.GetUserID(r.Context()), id)
	if err != nil {
		h.handleStoreError(w, err)
		return
	}
	h.writeJSON(w, http.StatusOK, ledgerTxn)
}

func (h *Handler) UpdateLedgerTransactionDetails(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUID(r.PathValue("transactionId"))
	if err != nil {
		h.errorResponse(w, http.StatusBadRequest, "invalid transactionId")
		return
	}
	var input model.LedgerTransactionDetailsInput
	if err := h.readJSON(r, &input); err != nil {
		h.errorResponse(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := input.Validate(); err != nil {
		h.errorResponse(w, http.StatusBadRequest, err.Error())
		return
	}
	ledgerTxn, err := h.store.UpdateLedgerTransactionDetails(r.Context(), middleware.GetUserID(r.Context()), id, input)
	if err != nil {
		h.handleStoreError(w, err)
		return
	}
	h.writeJSON(w, http.StatusOK, ledgerTxn)
}

func (h *Handler) ReviewLedgerTransaction(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUID(r.PathValue("transactionId"))
	if err != nil {
		h.errorResponse(w, http.StatusBadRequest, "invalid transactionId")
		return
	}
	var input model.LedgerTransactionReviewInput
	if err := h.readJSON(r, &input); err != nil {
		h.errorResponse(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := input.Validate(); err != nil {
		h.errorResponse(w, http.StatusBadRequest, err.Error())
		return
	}
	result, err := h.store.ReviewLedgerTransaction(r.Context(), middleware.GetUserID(r.Context()), id, input)
	if err != nil {
		h.handleStoreError(w, err)
		return
	}
	h.writeJSON(w, http.StatusOK, ledgerReviewResponse{Transaction: result.Transaction, Category: result.Category})
}
