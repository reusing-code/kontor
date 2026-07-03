package ledger

import (
	"github.com/reusing-code/kontor/backend/internal/httputil"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/reusing-code/kontor/backend/internal/middleware"
)

func (h *Handler) ListLedgerCategories(w http.ResponseWriter, r *http.Request) {
	items, err := h.store.ListLedgerCategories(r.Context(), middleware.GetUserID(r.Context()))
	if err != nil {
		h.storeError(w, err)
		return
	}
	httputil.WriteJSON(h.logger, w, http.StatusOK, items)
}

func (h *Handler) GetLedgerCategory(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		httputil.Error(h.logger, w, http.StatusBadRequest, "invalid id")
		return
	}
	item, err := h.store.GetLedgerCategory(r.Context(), middleware.GetUserID(r.Context()), id)
	if err != nil {
		h.storeError(w, err)
		return
	}
	httputil.WriteJSON(h.logger, w, http.StatusOK, item)
}

func (h *Handler) CreateLedgerCategory(w http.ResponseWriter, r *http.Request) {
	var input LedgerCategoryInput
	if err := httputil.ReadJSON(r, &input); err != nil {
		httputil.Error(h.logger, w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := input.Validate(); err != nil {
		httputil.Error(h.logger, w, http.StatusBadRequest, err.Error())
		return
	}
	now := time.Now().UTC()
	category := LedgerCategory{
		ID:         uuid.New(),
		Name:       input.Name,
		ParentID:   input.ParentID,
		MatchWords: input.MatchWords,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	if err := h.store.CreateLedgerCategory(r.Context(), middleware.GetUserID(r.Context()), category); err != nil {
		h.storeError(w, err)
		return
	}
	httputil.WriteJSON(h.logger, w, http.StatusCreated, category)
}

func (h *Handler) UpdateLedgerCategory(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		httputil.Error(h.logger, w, http.StatusBadRequest, "invalid id")
		return
	}
	existing, err := h.store.GetLedgerCategory(r.Context(), middleware.GetUserID(r.Context()), id)
	if err != nil {
		h.storeError(w, err)
		return
	}
	var input LedgerCategoryInput
	if err := httputil.ReadJSON(r, &input); err != nil {
		httputil.Error(h.logger, w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := input.Validate(); err != nil {
		httputil.Error(h.logger, w, http.StatusBadRequest, err.Error())
		return
	}
	existing.Name = input.Name
	existing.ParentID = input.ParentID
	existing.MatchWords = input.MatchWords
	existing.UpdatedAt = time.Now().UTC()
	if err := h.store.UpdateLedgerCategory(r.Context(), middleware.GetUserID(r.Context()), existing); err != nil {
		h.storeError(w, err)
		return
	}
	httputil.WriteJSON(h.logger, w, http.StatusOK, existing)
}

func (h *Handler) DeleteLedgerCategory(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		httputil.Error(h.logger, w, http.StatusBadRequest, "invalid id")
		return
	}
	if err := h.store.DeleteLedgerCategory(r.Context(), middleware.GetUserID(r.Context()), id); err != nil {
		h.storeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type ledgerReviewResponse struct {
	Transaction LedgerTransaction `json:"transaction"`
	Category    *LedgerCategory   `json:"category,omitempty"`
}

type ledgerTransferCandidatesResponse struct {
	Items []LedgerTransferCandidate `json:"items"`
}

type ledgerTransferLinkResponse struct {
	Transaction       LedgerTransaction  `json:"transaction"`
	PairedTransaction *LedgerTransaction `json:"pairedTransaction,omitempty"`
}

func (h *Handler) ListLedgerTransactionsReviewQueue(w http.ResponseWriter, r *http.Request) {
	limit := 100
	if rawLimit := r.URL.Query().Get("limit"); rawLimit != "" {
		parsed, err := strconv.Atoi(rawLimit)
		if err != nil || parsed <= 0 {
			httputil.Error(h.logger, w, http.StatusBadRequest, "invalid limit")
			return
		}
		limit = parsed
	}
	page, err := h.store.ListLedgerTransactionsFiltered(r.Context(), middleware.GetUserID(r.Context()), LedgerTransactionListOptions{
		ReviewStatus: LedgerTransactionReviewNeedsReview,
		Limit:        limit,
		Cursor:       r.URL.Query().Get("cursor"),
	})
	if err != nil {
		h.storeError(w, err)
		return
	}
	httputil.WriteJSON(h.logger, w, http.StatusOK, ledgerTransactionPageResponse(page))
}

func (h *Handler) GetLedgerTransaction(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("transactionId"))
	if err != nil {
		httputil.Error(h.logger, w, http.StatusBadRequest, "invalid transactionId")
		return
	}
	ledgerTxn, err := h.store.GetLedgerTransaction(r.Context(), middleware.GetUserID(r.Context()), id)
	if err != nil {
		h.storeError(w, err)
		return
	}
	httputil.WriteJSON(h.logger, w, http.StatusOK, ledgerTxn)
}

func (h *Handler) UpdateLedgerTransactionDetails(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("transactionId"))
	if err != nil {
		httputil.Error(h.logger, w, http.StatusBadRequest, "invalid transactionId")
		return
	}
	var input LedgerTransactionDetailsInput
	if err := httputil.ReadJSON(r, &input); err != nil {
		httputil.Error(h.logger, w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := input.Validate(); err != nil {
		httputil.Error(h.logger, w, http.StatusBadRequest, err.Error())
		return
	}
	ledgerTxn, err := h.store.UpdateLedgerTransactionDetails(r.Context(), middleware.GetUserID(r.Context()), id, input)
	if err != nil {
		h.storeError(w, err)
		return
	}
	httputil.WriteJSON(h.logger, w, http.StatusOK, ledgerTxn)
}

func (h *Handler) ListLedgerTransferCandidates(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("transactionId"))
	if err != nil {
		httputil.Error(h.logger, w, http.StatusBadRequest, "invalid transactionId")
		return
	}
	result, err := h.store.ListLedgerTransferCandidates(r.Context(), middleware.GetUserID(r.Context()), id)
	if err != nil {
		h.storeError(w, err)
		return
	}
	httputil.WriteJSON(h.logger, w, http.StatusOK, ledgerTransferCandidatesResponse(result))
}

func (h *Handler) LinkLedgerTransfer(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("transactionId"))
	if err != nil {
		httputil.Error(h.logger, w, http.StatusBadRequest, "invalid transactionId")
		return
	}
	var input LedgerTransferLinkInput
	if err := httputil.ReadJSON(r, &input); err != nil {
		httputil.Error(h.logger, w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := input.Validate(); err != nil {
		httputil.Error(h.logger, w, http.StatusBadRequest, err.Error())
		return
	}
	result, err := h.store.LinkLedgerTransfer(r.Context(), middleware.GetUserID(r.Context()), id, input)
	if err != nil {
		h.storeError(w, err)
		return
	}
	httputil.WriteJSON(h.logger, w, http.StatusOK, ledgerTransferLinkResponse{Transaction: result.Transaction, PairedTransaction: &result.PairedTransaction})
}

func (h *Handler) UnlinkLedgerTransfer(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("transactionId"))
	if err != nil {
		httputil.Error(h.logger, w, http.StatusBadRequest, "invalid transactionId")
		return
	}
	result, err := h.store.UnlinkLedgerTransfer(r.Context(), middleware.GetUserID(r.Context()), id)
	if err != nil {
		h.storeError(w, err)
		return
	}
	httputil.WriteJSON(h.logger, w, http.StatusOK, ledgerTransferLinkResponse(result))
}

func (h *Handler) ReviewLedgerTransaction(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("transactionId"))
	if err != nil {
		httputil.Error(h.logger, w, http.StatusBadRequest, "invalid transactionId")
		return
	}
	var input LedgerTransactionReviewInput
	if err := httputil.ReadJSON(r, &input); err != nil {
		httputil.Error(h.logger, w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := input.Validate(); err != nil {
		httputil.Error(h.logger, w, http.StatusBadRequest, err.Error())
		return
	}
	result, err := h.store.ReviewLedgerTransaction(r.Context(), middleware.GetUserID(r.Context()), id, input)
	if err != nil {
		h.storeError(w, err)
		return
	}
	httputil.WriteJSON(h.logger, w, http.StatusOK, ledgerReviewResponse(result))
}
