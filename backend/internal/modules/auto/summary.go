package auto

import (
	"net/http"
	"time"

	"github.com/reusing-code/kontor/backend/internal/httputil"
	"github.com/reusing-code/kontor/backend/internal/middleware"
)

func (h *Handler) VehicleSummary(w http.ResponseWriter, r *http.Request) {
	vehicleID, ok := httputil.PathUUID(h.logger, w, r, "id", "invalid vehicle id")
	if !ok {
		return
	}

	userID := middleware.GetUserID(r.Context())

	vehicle, err := h.store.GetVehicle(r.Context(), userID, vehicleID)
	if err != nil {
		httputil.StoreError(h.logger, w, err)
		return
	}

	entries, err := h.store.ListCostEntries(r.Context(), userID, vehicleID)
	if err != nil {
		httputil.StoreError(h.logger, w, err)
		return
	}

	summary := CalculateVehicleSummary(vehicle, entries, time.Now().UTC())
	httputil.WriteJSON(h.logger, w, http.StatusOK, summary)
}
