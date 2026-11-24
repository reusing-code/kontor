package contracts

import (
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/reusing-code/kontor/backend/internal/httputil"
	"github.com/reusing-code/kontor/backend/internal/middleware"
)

func (h *Handler) ListContracts(w http.ResponseWriter, r *http.Request) {
	contracts, err := h.store.List(r.Context(), middleware.GetUserID(r.Context()))
	if err != nil {
		httputil.StoreError(h.logger, w, err)
		return
	}
	httputil.WriteJSON(h.logger, w, http.StatusOK, newContractViews(contracts))
}

func (h *Handler) ListContractsByCategory(w http.ResponseWriter, r *http.Request) {
	catID, ok := httputil.PathUUID(h.logger, w, r, "id", "invalid category id")
	if !ok {
		return
	}

	contracts, err := h.store.ListByCategory(r.Context(), middleware.GetUserID(r.Context()), catID)
	if err != nil {
		httputil.StoreError(h.logger, w, err)
		return
	}
	httputil.WriteJSON(h.logger, w, http.StatusOK, newContractViews(contracts))
}

func (h *Handler) CreateContractInCategory(w http.ResponseWriter, r *http.Request) {
	catID, ok := httputil.PathUUID(h.logger, w, r, "id", "invalid category id")
	if !ok {
		return
	}

	// Verify category exists (contracts module)
	if _, err := h.categories.Get(r.Context(), middleware.GetUserID(r.Context()), "contracts", catID); err != nil {
		httputil.StoreError(h.logger, w, err)
		return
	}

	input, ok := httputil.Decode[ContractInput](h.logger, w, r)
	if !ok {
		return
	}

	con := newContract(uuid.New(), catID, input, time.Now().UTC())

	if err := h.store.Create(r.Context(), middleware.GetUserID(r.Context()), con); err != nil {
		httputil.StoreError(h.logger, w, err)
		return
	}
	httputil.WriteJSON(h.logger, w, http.StatusCreated, newContractView(con))
}

func (h *Handler) GetContract(w http.ResponseWriter, r *http.Request) {
	id, ok := httputil.PathUUID(h.logger, w, r, "id", "invalid id")
	if !ok {
		return
	}

	con, err := h.store.Get(r.Context(), middleware.GetUserID(r.Context()), id)
	if err != nil {
		httputil.StoreError(h.logger, w, err)
		return
	}
	httputil.WriteJSON(h.logger, w, http.StatusOK, newContractView(con))
}

func (h *Handler) UpdateContract(w http.ResponseWriter, r *http.Request) {
	id, ok := httputil.PathUUID(h.logger, w, r, "id", "invalid id")
	if !ok {
		return
	}

	existing, err := h.store.Get(r.Context(), middleware.GetUserID(r.Context()), id)
	if err != nil {
		httputil.StoreError(h.logger, w, err)
		return
	}

	input, ok := httputil.Decode[ContractInput](h.logger, w, r)
	if !ok {
		return
	}

	existing.Apply(input, time.Now().UTC())

	if err := h.store.Update(r.Context(), middleware.GetUserID(r.Context()), existing); err != nil {
		httputil.StoreError(h.logger, w, err)
		return
	}
	httputil.WriteJSON(h.logger, w, http.StatusOK, newContractView(existing))
}

func (h *Handler) DeleteContract(w http.ResponseWriter, r *http.Request) {
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

// newContract builds the contract a create request stores; input must be
// validated.
func newContract(id, catID uuid.UUID, input ContractInput, now time.Time) Contract {
	c := Contract{ID: id, CategoryID: catID, CreatedAt: now}
	c.Apply(input, now)
	return c
}
