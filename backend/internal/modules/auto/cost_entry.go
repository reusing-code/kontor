package auto

import (
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/reusing-code/kontor/backend/internal/httputil"
	"github.com/reusing-code/kontor/backend/internal/middleware"
	)

func (h *Handler) ListCostEntries(w http.ResponseWriter, r *http.Request) {
	vehicleID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		httputil.Error(h.logger, w, http.StatusBadRequest, "invalid vehicle id")
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
	vehicleID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		httputil.Error(h.logger, w, http.StatusBadRequest, "invalid vehicle id")
		return
	}

	if _, err := h.store.GetVehicle(r.Context(), middleware.GetUserID(r.Context()), vehicleID); err != nil {
		httputil.StoreError(h.logger, w, err)
		return
	}

	var input CostEntryInput
	if err := httputil.ReadJSON(r, &input); err != nil {
		httputil.Error(h.logger, w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := input.Validate(); err != nil {
		httputil.Error(h.logger, w, http.StatusBadRequest, err.Error())
		return
	}

	now := time.Now().UTC()
	c := CostEntry{
		ID:          uuid.New(),
		VehicleID:   vehicleID,
		Type:        input.Type,
		Description: input.Description,
		Vendor:      input.Vendor,
		Amount:      input.Amount,
		Date:        input.Date,
		Mileage:     input.Mileage,
		Comments:    input.Comments,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	if err := h.store.CreateCostEntry(r.Context(), middleware.GetUserID(r.Context()), c); err != nil {
		httputil.StoreError(h.logger, w, err)
		return
	}
	httputil.WriteJSON(h.logger, w, http.StatusCreated, c)
}

func (h *Handler) GetCostEntry(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		httputil.Error(h.logger, w, http.StatusBadRequest, "invalid id")
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
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		httputil.Error(h.logger, w, http.StatusBadRequest, "invalid id")
		return
	}

	existing, err := h.store.GetCostEntry(r.Context(), middleware.GetUserID(r.Context()), id)
	if err != nil {
		httputil.StoreError(h.logger, w, err)
		return
	}

	var input CostEntryInput
	if err := httputil.ReadJSON(r, &input); err != nil {
		httputil.Error(h.logger, w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := input.Validate(); err != nil {
		httputil.Error(h.logger, w, http.StatusBadRequest, err.Error())
		return
	}

	existing.Type = input.Type
	existing.Description = input.Description
	existing.Vendor = input.Vendor
	existing.Amount = input.Amount
	existing.Date = input.Date
	existing.Mileage = input.Mileage
	existing.Comments = input.Comments
	existing.UpdatedAt = time.Now().UTC()

	if err := h.store.UpdateCostEntry(r.Context(), middleware.GetUserID(r.Context()), existing); err != nil {
		httputil.StoreError(h.logger, w, err)
		return
	}
	httputil.WriteJSON(h.logger, w, http.StatusOK, existing)
}

func (h *Handler) DeleteCostEntry(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		httputil.Error(h.logger, w, http.StatusBadRequest, "invalid id")
		return
	}

	if err := h.store.DeleteCostEntry(r.Context(), middleware.GetUserID(r.Context()), id); err != nil {
		httputil.StoreError(h.logger, w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
