package ledger

import (
	"context"
	"errors"
	"net/http"

	"github.com/reusing-code/kontor/backend/internal/httputil"
	"github.com/reusing-code/kontor/backend/internal/middleware"
	"github.com/reusing-code/kontor/backend/internal/storage"
)

type ledgerReviewQueueItem struct {
	LedgerTransaction
	Suggestion *LedgerSuggestionView `json:"suggestion,omitempty"`
}

type ledgerReviewQueueResponse struct {
	Items      []ledgerReviewQueueItem `json:"items"`
	NextCursor string                  `json:"nextCursor,omitempty"`
}

type ledgerSuggestionStatusResponse struct {
	Running bool `json:"running"`
}

func (h *Handler) RunLedgerCategorySuggestions(w http.ResponseWriter, r *http.Request) {
	// Like the email scan, a run is not tied to the request so it finishes and
	// stores its suggestions even when the client gives up waiting.
	ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), suggestRunTimeout)
	defer cancel()
	result, err := h.suggester.Run(ctx, middleware.GetUserID(r.Context()))
	switch {
	case errors.Is(err, ErrSuggestLLMDisabled), errors.Is(err, ErrSuggestRunning):
		httputil.Error(h.logger, w, http.StatusConflict, err.Error())
	case err != nil:
		h.storeError(w, err)
	default:
		httputil.WriteJSON(h.logger, w, http.StatusOK, result)
	}
}

func (h *Handler) GetLedgerCategorySuggestionStatus(w http.ResponseWriter, r *http.Request) {
	httputil.WriteJSON(h.logger, w, http.StatusOK, ledgerSuggestionStatusResponse{
		Running: h.suggester.Running(middleware.GetUserID(r.Context())),
	})
}

func (h *Handler) ReviewLedgerTransactionsBatch(w http.ResponseWriter, r *http.Request) {
	var input LedgerBatchReviewInput
	if err := httputil.ReadJSON(r, &input); err != nil {
		httputil.Error(h.logger, w, http.StatusBadRequest, "invalid request body")
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
