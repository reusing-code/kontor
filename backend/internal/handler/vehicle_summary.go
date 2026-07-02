package handler

import (
	"net/http"
	"time"

	"github.com/reusing-code/kontor/backend/internal/middleware"
	"github.com/reusing-code/kontor/backend/internal/model"
)

func (h *Handler) VehicleSummary(w http.ResponseWriter, r *http.Request) {
	vehicleID, err := parseUUID(r.PathValue("id"))
	if err != nil {
		h.errorResponse(w, http.StatusBadRequest, "invalid vehicle id")
		return
	}

	userID := middleware.GetUserID(r.Context())

	vehicle, err := h.store.GetVehicle(r.Context(), userID, vehicleID)
	if err != nil {
		h.handleStoreError(w, err)
		return
	}

	entries, err := h.store.ListCostEntries(r.Context(), userID, vehicleID)
	if err != nil {
		h.handleStoreError(w, err)
		return
	}

	summary := model.CalculateVehicleSummary(vehicle, entries, time.Now().UTC())
	h.writeJSON(w, http.StatusOK, summary)
}
