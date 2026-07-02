package handler

import (
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/reusing-code/kontor/backend/internal/middleware"
	"github.com/reusing-code/kontor/backend/internal/model"
)

func (h *Handler) ListVehicles(w http.ResponseWriter, r *http.Request) {
	vehicles, err := h.store.ListVehicles(r.Context(), middleware.GetUserID(r.Context()))
	if err != nil {
		h.handleStoreError(w, err)
		return
	}
	h.writeJSON(w, http.StatusOK, vehicles)
}

func (h *Handler) GetVehicle(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUID(r.PathValue("id"))
	if err != nil {
		h.errorResponse(w, http.StatusBadRequest, "invalid id")
		return
	}

	v, err := h.store.GetVehicle(r.Context(), middleware.GetUserID(r.Context()), id)
	if err != nil {
		h.handleStoreError(w, err)
		return
	}
	h.writeJSON(w, http.StatusOK, v)
}

func (h *Handler) CreateVehicle(w http.ResponseWriter, r *http.Request) {
	var input model.VehicleInput
	if err := h.readJSON(r, &input); err != nil {
		h.errorResponse(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := input.Validate(); err != nil {
		h.errorResponse(w, http.StatusBadRequest, err.Error())
		return
	}

	now := time.Now().UTC()
	v := model.Vehicle{
		ID:                uuid.New(),
		Name:              input.Name,
		Make:              input.Make,
		Model:             input.Model,
		Year:              input.Year,
		LicensePlate:      input.LicensePlate,
		PurchaseDate:      input.PurchaseDate,
		PurchasePrice:     input.PurchasePrice,
		PurchaseMileage:   input.PurchaseMileage,
		TargetMileage:     input.TargetMileage,
		TargetMonths:      input.TargetMonths,
		AnnualInsurance:   input.AnnualInsurance,
		AnnualTax:         input.AnnualTax,
		MaintenanceFactor: input.MaintenanceFactor,
		Comments:          input.Comments,
		CreatedAt:         now,
		UpdatedAt:         now,
	}

	if err := h.store.CreateVehicle(r.Context(), middleware.GetUserID(r.Context()), v); err != nil {
		h.handleStoreError(w, err)
		return
	}
	h.writeJSON(w, http.StatusCreated, v)
}

func (h *Handler) UpdateVehicle(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUID(r.PathValue("id"))
	if err != nil {
		h.errorResponse(w, http.StatusBadRequest, "invalid id")
		return
	}

	existing, err := h.store.GetVehicle(r.Context(), middleware.GetUserID(r.Context()), id)
	if err != nil {
		h.handleStoreError(w, err)
		return
	}

	var input model.VehicleInput
	if err := h.readJSON(r, &input); err != nil {
		h.errorResponse(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := input.Validate(); err != nil {
		h.errorResponse(w, http.StatusBadRequest, err.Error())
		return
	}

	existing.Name = input.Name
	existing.Make = input.Make
	existing.Model = input.Model
	existing.Year = input.Year
	existing.LicensePlate = input.LicensePlate
	existing.PurchaseDate = input.PurchaseDate
	existing.PurchasePrice = input.PurchasePrice
	existing.PurchaseMileage = input.PurchaseMileage
	existing.TargetMileage = input.TargetMileage
	existing.TargetMonths = input.TargetMonths
	existing.AnnualInsurance = input.AnnualInsurance
	existing.AnnualTax = input.AnnualTax
	existing.MaintenanceFactor = input.MaintenanceFactor
	existing.Comments = input.Comments
	existing.UpdatedAt = time.Now().UTC()

	if err := h.store.UpdateVehicle(r.Context(), middleware.GetUserID(r.Context()), existing); err != nil {
		h.handleStoreError(w, err)
		return
	}
	h.writeJSON(w, http.StatusOK, existing)
}

func (h *Handler) DeleteVehicle(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUID(r.PathValue("id"))
	if err != nil {
		h.errorResponse(w, http.StatusBadRequest, "invalid id")
		return
	}

	if err := h.store.DeleteVehicle(r.Context(), middleware.GetUserID(r.Context()), id); err != nil {
		h.handleStoreError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
