package ledger

import (
	"context"
	"errors"
	"net/http"

	"github.com/reusing-code/kontor/backend/internal/httputil"
	"github.com/reusing-code/kontor/backend/internal/middleware"
)

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
