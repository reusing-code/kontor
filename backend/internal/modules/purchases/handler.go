package purchases

import (
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/reusing-code/kontor/backend/internal/httputil"
	"github.com/reusing-code/kontor/backend/internal/middleware"
)

func (h *Handler) ListPurchases(w http.ResponseWriter, r *http.Request) {
	purchases, err := h.store.List(r.Context(), middleware.GetUserID(r.Context()))
	if err != nil {
		httputil.StoreError(h.logger, w, err)
		return
	}
	httputil.WriteJSON(h.logger, w, http.StatusOK, purchases)
}

func (h *Handler) ListPurchasesByCategory(w http.ResponseWriter, r *http.Request) {
	catID, ok := httputil.PathUUID(h.logger, w, r, "id", "invalid category id")
	if !ok {
		return
	}

	purchases, err := h.store.ListByCategory(r.Context(), middleware.GetUserID(r.Context()), catID)
	if err != nil {
		httputil.StoreError(h.logger, w, err)
		return
	}
	httputil.WriteJSON(h.logger, w, http.StatusOK, purchases)
}

func (h *Handler) CreatePurchaseInCategory(w http.ResponseWriter, r *http.Request) {
	catID, ok := httputil.PathUUID(h.logger, w, r, "id", "invalid category id")
	if !ok {
		return
	}

	if _, err := h.categories.Get(r.Context(), middleware.GetUserID(r.Context()), ModuleID, catID); err != nil {
		httputil.StoreError(h.logger, w, err)
		return
	}

	input, ok := httputil.Decode[PurchaseInput](h.logger, w, r)
	if !ok {
		return
	}

	p := newPurchase(uuid.New(), catID, input, time.Now().UTC())

	if err := h.store.Create(r.Context(), middleware.GetUserID(r.Context()), p); err != nil {
		httputil.StoreError(h.logger, w, err)
		return
	}
	httputil.WriteJSON(h.logger, w, http.StatusCreated, p)
}

func (h *Handler) GetPurchase(w http.ResponseWriter, r *http.Request) {
	id, ok := httputil.PathUUID(h.logger, w, r, "id", "invalid id")
	if !ok {
		return
	}

	p, err := h.store.Get(r.Context(), middleware.GetUserID(r.Context()), id)
	if err != nil {
		httputil.StoreError(h.logger, w, err)
		return
	}
	httputil.WriteJSON(h.logger, w, http.StatusOK, p)
}

func (h *Handler) UpdatePurchase(w http.ResponseWriter, r *http.Request) {
	id, ok := httputil.PathUUID(h.logger, w, r, "id", "invalid id")
	if !ok {
		return
	}

	existing, err := h.store.Get(r.Context(), middleware.GetUserID(r.Context()), id)
	if err != nil {
		httputil.StoreError(h.logger, w, err)
		return
	}

	input, ok := httputil.Decode[PurchaseInput](h.logger, w, r)
	if !ok {
		return
	}

	existing.Apply(input, time.Now().UTC())

	if err := h.store.Update(r.Context(), middleware.GetUserID(r.Context()), existing); err != nil {
		httputil.StoreError(h.logger, w, err)
		return
	}
	httputil.WriteJSON(h.logger, w, http.StatusOK, existing)
}

func (h *Handler) DeletePurchase(w http.ResponseWriter, r *http.Request) {
	id, ok := httputil.PathUUID(h.logger, w, r, "id", "invalid id")
	if !ok {
		return
	}

	if err := h.store.Delete(r.Context(), middleware.GetUserID(r.Context()), id); err != nil {
		httputil.StoreError(h.logger, w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// newPurchase builds the purchase a create request stores; input must be
// validated.
func newPurchase(id, catID uuid.UUID, input PurchaseInput, now time.Time) Purchase {
	p := Purchase{ID: id, CategoryID: catID, CreatedAt: now}
	p.Apply(input, now)
	return p
}
