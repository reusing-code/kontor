package ledger

import (
	"context"
	"errors"
	"net/http"
	"strconv"

	"github.com/google/uuid"
	"github.com/reusing-code/kontor/backend/internal/httputil"
	"github.com/reusing-code/kontor/backend/internal/middleware"
	"github.com/reusing-code/kontor/backend/internal/storage"
)

type ledgerReviewResponse struct {
	Transaction LedgerTransaction `json:"transaction"`
	Category    *LedgerCategory   `json:"category,omitempty"`
}

// ListLedgerTransactionsReviewQueue pages the review queue; limit=all returns
// the whole queue in one response (the review page groups and pages it).
func (h *Handler) ListLedgerTransactionsReviewQueue(w http.ResponseWriter, r *http.Request) {
	options := LedgerTransactionListOptions{
		ReviewStatus: LedgerTransactionReviewNeedsReview,
		Limit:        100,
		Cursor:       r.URL.Query().Get("cursor"),
	}
	switch rawLimit := r.URL.Query().Get("limit"); rawLimit {
	case "":
	case "all":
		options.All = true
		options.Cursor = ""
	default:
		parsed, err := strconv.Atoi(rawLimit)
		if err != nil || parsed <= 0 {
			httputil.Error(h.logger, w, http.StatusBadRequest, "invalid limit")
			return
		}
		options.Limit = parsed
	}
	page, err := h.store.ListLedgerTransactionsFiltered(r.Context(), middleware.GetUserID(r.Context()), options)
	if err != nil {
		h.storeError(w, err)
		return
	}
	resp, err := h.reviewQueueResponse(r.Context(), middleware.GetUserID(r.Context()), page)
	if err != nil {
		h.storeError(w, err)
		return
	}
	httputil.WriteJSON(h.logger, w, http.StatusOK, resp)
}

func (h *Handler) ReviewLedgerTransaction(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("transactionId"))
	if err != nil {
		httputil.Error(h.logger, w, http.StatusBadRequest, "invalid transactionId")
		return
	}
	var input LedgerTransactionReviewInput
	if err := httputil.ReadJSON(r, &input); err != nil {
		httputil.BodyError(h.logger, w, err, "invalid request body")
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

type ledgerReviewQueueItem struct {
	LedgerTransaction
	Suggestion *LedgerSuggestionView `json:"suggestion,omitempty"`
}

type ledgerReviewQueueResponse struct {
	Items      []ledgerReviewQueueItem `json:"items"`
	NextCursor string                  `json:"nextCursor,omitempty"`
}

func (h *Handler) ReviewLedgerTransactionsBatch(w http.ResponseWriter, r *http.Request) {
	var input LedgerBatchReviewInput
	if err := httputil.ReadJSON(r, &input); err != nil {
		httputil.BodyError(h.logger, w, err, "invalid request body")
		return
	}
	if err := input.Validate(); err != nil {
		httputil.Error(h.logger, w, http.StatusBadRequest, err.Error())
		return
	}
	userID := middleware.GetUserID(r.Context())
	result := LedgerBatchReviewResult{Failed: []LedgerBatchReviewFailure{}}
	for _, item := range input.Items {
		_, err := h.store.ReviewLedgerTransaction(r.Context(), userID, item.TransactionID, LedgerTransactionReviewInput{
			CategoryID:    item.CategoryID,
			AddMatchWords: item.AddMatchWords,
		})
		switch {
		case err == nil:
			result.Reviewed++
			continue
		case errors.Is(err, storage.ErrNotFound):
			result.Failed = append(result.Failed, LedgerBatchReviewFailure{TransactionID: item.TransactionID, Code: "not_found"})
		case errors.Is(err, ErrLedgerTransferLinked):
			result.Failed = append(result.Failed, LedgerBatchReviewFailure{TransactionID: item.TransactionID, Code: "transfer_linked"})
		default:
			h.logger.Error("batch review failed", "transactionId", item.TransactionID, "error", err)
			result.Failed = append(result.Failed, LedgerBatchReviewFailure{TransactionID: item.TransactionID, Code: "internal_error"})
		}
	}
	httputil.WriteJSON(h.logger, w, http.StatusOK, result)
}

func (h *Handler) reviewQueueResponse(ctx context.Context, userID string, page LedgerTransactionPage) (ledgerReviewQueueResponse, error) {
	views, err := h.store.LedgerSuggestionViews(ctx, userID, page.Items)
	if err != nil {
		return ledgerReviewQueueResponse{}, err
	}
	items := make([]ledgerReviewQueueItem, len(page.Items))
	for i, t := range page.Items {
		items[i] = ledgerReviewQueueItem{LedgerTransaction: t}
		if v, ok := views[t.ID]; ok {
			items[i].Suggestion = &v
		}
	}
	return ledgerReviewQueueResponse{Items: items, NextCursor: page.NextCursor}, nil
}
