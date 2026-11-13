package ledger

import (
	"context"
	"errors"
	"net/http"

	"github.com/reusing-code/kontor/backend/internal/httputil"
	"github.com/reusing-code/kontor/backend/internal/middleware"
)

func (h *Handler) RunLedgerRecurringDetection(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), recurringRunTimeout)
	defer cancel()
	result, err := h.recurring.RunUser(ctx, middleware.GetUserID(r.Context()))
	switch {
	case errors.Is(err, ErrRecurringRunning), errors.Is(err, ErrRecurringUnavailable):
		httputil.Error(h.logger, w, http.StatusConflict, err.Error())
	case err != nil:
		h.storeError(w, err)
	default:
		httputil.WriteJSON(h.logger, w, http.StatusOK, result)
	}
}
