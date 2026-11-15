package ledger

import (
	"context"
	"errors"
	"net/http"

	"github.com/google/uuid"
	"github.com/reusing-code/kontor/backend/internal/httputil"
	"github.com/reusing-code/kontor/backend/internal/middleware"
)

// RunLedgerEmailDrafts checks the drafts folders of the user's accounts now.
func (h *Handler) RunLedgerEmailDrafts(w http.ResponseWriter, r *http.Request) {
	if h.emailDrafts == nil {
		httputil.Error(h.logger, w, http.StatusConflict, ErrEmailDraftsUnavailable.Error())
		return
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), emailDraftRunTimeout)
	defer cancel()
	result, err := h.emailDrafts.RunUser(ctx, middleware.GetUserID(r.Context()))
	switch {
	case errors.Is(err, ErrEmailLLMDisabled), errors.Is(err, ErrEmailDraftsNoTargets), errors.Is(err, ErrEmailDraftsBusy):
		httputil.Error(h.logger, w, http.StatusConflict, err.Error())
	case err != nil:
		h.storeError(w, err)
	default:
		httputil.WriteJSON(h.logger, w, http.StatusOK, result)
	}
}

// ListLedgerEmailLinks returns the emails an entity was created from.
func (h *Handler) ListLedgerEmailLinks(w http.ResponseWriter, r *http.Request) {
	entityType := r.PathValue("entityType")
	if !ValidEmailLinkEntityType(entityType) {
		httputil.Error(h.logger, w, http.StatusBadRequest, "invalid entityType")
		return
	}
	entityID, err := uuid.Parse(r.PathValue("entityId"))
	if err != nil {
		httputil.Error(h.logger, w, http.StatusBadRequest, "invalid entityId")
		return
	}
	links, err := h.store.ListLedgerEmailLinks(r.Context(), middleware.GetUserID(r.Context()), entityType, entityID.String())
	if err != nil {
		h.storeError(w, err)
		return
	}
	httputil.WriteJSON(h.logger, w, http.StatusOK, links)
}
