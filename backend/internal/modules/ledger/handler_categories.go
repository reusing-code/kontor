package ledger

import (
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/reusing-code/kontor/backend/internal/httputil"
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
		httputil.BodyError(h.logger, w, err, "invalid request body")
		return
	}
	if err := input.Validate(); err != nil {
		httputil.Error(h.logger, w, http.StatusBadRequest, err.Error())
		return
	}
	category := newLedgerCategory(uuid.New(), input, time.Now().UTC())
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
		httputil.BodyError(h.logger, w, err, "invalid request body")
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

// newLedgerCategory builds the category a create request stores; input must
// be validated.
func newLedgerCategory(id uuid.UUID, input LedgerCategoryInput, now time.Time) LedgerCategory {
	return LedgerCategory{
		ID:         id,
		Name:       input.Name,
		ParentID:   input.ParentID,
		MatchWords: input.MatchWords,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
}
