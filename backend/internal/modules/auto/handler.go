package auto

import (
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/reusing-code/kontor/backend/internal/httputil"
	"github.com/reusing-code/kontor/backend/internal/middleware"
)

func (h *Handler) ListVehicles(w http.ResponseWriter, r *http.Request) {
	vehicles, err := h.store.ListVehicles(r.Context(), middleware.GetUserID(r.Context()))
	if err != nil {
		httputil.StoreError(h.logger, w, err)
		return
	}
	httputil.WriteJSON(h.logger, w, http.StatusOK, vehicles)
}

func (h *Handler) GetVehicle(w http.ResponseWriter, r *http.Request) {
	id, ok := httputil.PathUUID(h.logger, w, r, "id", "invalid id")
	if !ok {
		return
	}

	v, err := h.store.GetVehicle(r.Context(), middleware.GetUserID(r.Context()), id)
	if err != nil {
		httputil.StoreError(h.logger, w, err)
		return
	}
	httputil.WriteJSON(h.logger, w, http.StatusOK, v)
}

func (h *Handler) CreateVehicle(w http.ResponseWriter, r *http.Request) {
	input, ok := httputil.Decode[VehicleInput](h.logger, w, r)
	if !ok {
		return
	}

	v := newVehicle(uuid.New(), input, time.Now().UTC())

	if err := h.store.CreateVehicle(r.Context(), middleware.GetUserID(r.Context()), v); err != nil {
		httputil.StoreError(h.logger, w, err)
		return
	}
	httputil.WriteJSON(h.logger, w, http.StatusCreated, v)
}

func (h *Handler) UpdateVehicle(w http.ResponseWriter, r *http.Request) {
	id, ok := httputil.PathUUID(h.logger, w, r, "id", "invalid id")
	if !ok {
		return
	}

	existing, err := h.store.GetVehicle(r.Context(), middleware.GetUserID(r.Context()), id)
	if err != nil {
		httputil.StoreError(h.logger, w, err)
		return
	}

	input, ok := httputil.Decode[VehicleInput](h.logger, w, r)
	if !ok {
		return
	}

	existing.Apply(input, time.Now().UTC())

	if err := h.store.UpdateVehicle(r.Context(), middleware.GetUserID(r.Context()), existing); err != nil {
		httputil.StoreError(h.logger, w, err)
		return
	}
	httputil.WriteJSON(h.logger, w, http.StatusOK, existing)
}

func (h *Handler) DeleteVehicle(w http.ResponseWriter, r *http.Request) {
	id, ok := httputil.PathUUID(h.logger, w, r, "id", "invalid id")
	if !ok {
		return
	}

	if err := h.store.DeleteVehicle(r.Context(), middleware.GetUserID(r.Context()), id); err != nil {
		httputil.StoreError(h.logger, w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// newVehicle builds the vehicle a create request stores; input must be
// validated.
func newVehicle(id uuid.UUID, input VehicleInput, now time.Time) Vehicle {
	v := Vehicle{ID: id, CreatedAt: now}
	v.Apply(input, now)
	return v
}
