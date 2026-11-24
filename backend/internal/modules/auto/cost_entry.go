package auto

import (
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/reusing-code/kontor/backend/internal/httputil"
	"github.com/reusing-code/kontor/backend/internal/middleware"
)

func (h *Handler) ListCostEntries(w http.ResponseWriter, r *http.Request) {
	vehicleID, ok := httputil.PathUUID(h.logger, w, r, "id", "invalid vehicle id")
	if !ok {
		return
	}

	entries, err := h.store.ListCostEntries(r.Context(), middleware.GetUserID(r.Context()), vehicleID)
	if err != nil {
		httputil.StoreError(h.logger, w, err)
		return
	}
	httputil.WriteJSON(h.logger, w, http.StatusOK, entries)
}

func (h *Handler) CreateCostEntry(w http.ResponseWriter, r *http.Request) {
	vehicleID, ok := httputil.PathUUID(h.logger, w, r, "id", "invalid vehicle id")
	if !ok {
		return
	}

	if _, err := h.store.GetVehicle(r.Context(), middleware.GetUserID(r.Context()), vehicleID); err != nil {
		httputil.StoreError(h.logger, w, err)
		return
	}

	input, ok := httputil.Decode[CostEntryInput](h.logger, w, r)
	if !ok {
		return
	}

	c := newCostEntry(uuid.New(), vehicleID, input, time.Now().UTC())

	if err := h.store.CreateCostEntry(r.Context(), middleware.GetUserID(r.Context()), c); err != nil {
		httputil.StoreError(h.logger, w, err)
		return
	}
	httputil.WriteJSON(h.logger, w, http.StatusCreated, c)
}

func (h *Handler) GetCostEntry(w http.ResponseWriter, r *http.Request) {
	id, ok := httputil.PathUUID(h.logger, w, r, "id", "invalid id")
	if !ok {
		return
	}

	c, err := h.store.GetCostEntry(r.Context(), middleware.GetUserID(r.Context()), id)
	if err != nil {
		httputil.StoreError(h.logger, w, err)
		return
	}
	httputil.WriteJSON(h.logger, w, http.StatusOK, c)
}

func (h *Handler) UpdateCostEntry(w http.ResponseWriter, r *http.Request) {
	id, ok := httputil.PathUUID(h.logger, w, r, "id", "invalid id")
	if !ok {
		return
	}

	existing, err := h.store.GetCostEntry(r.Context(), middleware.GetUserID(r.Context()), id)
	if err != nil {
		httputil.StoreError(h.logger, w, err)
		return
	}

	input, ok := httputil.Decode[CostEntryInput](h.logger, w, r)
	if !ok {
		return
	}

	existing.Apply(input, time.Now().UTC())

	if err := h.store.UpdateCostEntry(r.Context(), middleware.GetUserID(r.Context()), existing); err != nil {
		httputil.StoreError(h.logger, w, err)
		return
	}
	httputil.WriteJSON(h.logger, w, http.StatusOK, existing)
}

func (h *Handler) DeleteCostEntry(w http.ResponseWriter, r *http.Request) {
	id, ok := httputil.PathUUID(h.logger, w, r, "id", "invalid id")
	if !ok {
		return
	}

	if err := h.store.DeleteCostEntry(r.Context(), middleware.GetUserID(r.Context()), id); err != nil {
		httputil.StoreError(h.logger, w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// newCostEntry builds the cost entry a create request stores; input must be
// validated.
func newCostEntry(id, vehicleID uuid.UUID, input CostEntryInput, now time.Time) CostEntry {
	c := CostEntry{ID: id, VehicleID: vehicleID, CreatedAt: now}
	c.Apply(input, now)
	return c
}
