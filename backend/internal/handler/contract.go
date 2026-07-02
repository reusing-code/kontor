package handler

import (
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/reusing-code/kontor/backend/internal/middleware"
	"github.com/reusing-code/kontor/backend/internal/model"
)

func (h *Handler) ListContracts(w http.ResponseWriter, r *http.Request) {
	contracts, err := h.store.ListContracts(r.Context(), middleware.GetUserID(r.Context()))
	if err != nil {
		h.handleStoreError(w, err)
		return
	}
	h.writeJSON(w, http.StatusOK, newContractViews(contracts))
}

func (h *Handler) ListContractsByCategory(w http.ResponseWriter, r *http.Request) {
	catID, err := parseUUID(r.PathValue("id"))
	if err != nil {
		h.errorResponse(w, http.StatusBadRequest, "invalid category id")
		return
	}

	contracts, err := h.store.ListContractsByCategory(r.Context(), middleware.GetUserID(r.Context()), catID)
	if err != nil {
		h.handleStoreError(w, err)
		return
	}
	h.writeJSON(w, http.StatusOK, newContractViews(contracts))
}

func (h *Handler) CreateContractInCategory(w http.ResponseWriter, r *http.Request) {
	catID, err := parseUUID(r.PathValue("id"))
	if err != nil {
		h.errorResponse(w, http.StatusBadRequest, "invalid category id")
		return
	}

	// Verify category exists (contracts module)
	if _, err := h.store.GetCategory(r.Context(), middleware.GetUserID(r.Context()), "contracts", catID); err != nil {
		h.handleStoreError(w, err)
		return
	}

	var input model.ContractInput
	if err := h.readJSON(r, &input); err != nil {
		h.errorResponse(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := input.Validate(); err != nil {
		h.errorResponse(w, http.StatusBadRequest, err.Error())
		return
	}

	now := time.Now().UTC()
	bi := input.BillingInterval
	if bi == "" {
		bi = model.BillingMonthly
	}
	con := model.Contract{
		ID:                      uuid.New(),
		CategoryID:              catID,
		Name:                    input.Name,
		ProductName:             input.ProductName,
		Company:                 input.Company,
		ContractNumber:          input.ContractNumber,
		CustomerNumber:          input.CustomerNumber,
		Price:                   input.Price,
		BillingInterval:         bi,
		StartDate:               input.StartDate,
		EndDate:                 input.EndDate,
		MinimumDurationMonths:   input.MinimumDurationMonths,
		ExtensionDurationMonths: input.ExtensionDurationMonths,
		NoticePeriodMonths:      input.NoticePeriodMonths,
		CustomerPortalURL:       input.CustomerPortalURL,
		PaperlessURL:            input.PaperlessURL,
		Comments:                input.Comments,
		CreatedAt:               now,
		UpdatedAt:               now,
	}

	if err := h.store.CreateContract(r.Context(), middleware.GetUserID(r.Context()), con); err != nil {
		h.handleStoreError(w, err)
		return
	}
	h.writeJSON(w, http.StatusCreated, newContractView(con))
}

func (h *Handler) GetContract(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUID(r.PathValue("id"))
	if err != nil {
		h.errorResponse(w, http.StatusBadRequest, "invalid id")
		return
	}

	con, err := h.store.GetContract(r.Context(), middleware.GetUserID(r.Context()), id)
	if err != nil {
		h.handleStoreError(w, err)
		return
	}
	h.writeJSON(w, http.StatusOK, newContractView(con))
}

func (h *Handler) UpdateContract(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUID(r.PathValue("id"))
	if err != nil {
		h.errorResponse(w, http.StatusBadRequest, "invalid id")
		return
	}

	existing, err := h.store.GetContract(r.Context(), middleware.GetUserID(r.Context()), id)
	if err != nil {
		h.handleStoreError(w, err)
		return
	}

	var input model.ContractInput
	if err := h.readJSON(r, &input); err != nil {
		h.errorResponse(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := input.Validate(); err != nil {
		h.errorResponse(w, http.StatusBadRequest, err.Error())
		return
	}

	existing.Name = input.Name
	existing.ProductName = input.ProductName
	existing.Company = input.Company
	existing.ContractNumber = input.ContractNumber
	existing.CustomerNumber = input.CustomerNumber
	existing.Price = input.Price
	bi := input.BillingInterval
	if bi == "" {
		bi = model.BillingMonthly
	}
	existing.BillingInterval = bi
	existing.StartDate = input.StartDate
	existing.EndDate = input.EndDate
	existing.MinimumDurationMonths = input.MinimumDurationMonths
	existing.ExtensionDurationMonths = input.ExtensionDurationMonths
	existing.NoticePeriodMonths = input.NoticePeriodMonths
	existing.CustomerPortalURL = input.CustomerPortalURL
	existing.PaperlessURL = input.PaperlessURL
	existing.Comments = input.Comments
	existing.UpdatedAt = time.Now().UTC()

	if err := h.store.UpdateContract(r.Context(), middleware.GetUserID(r.Context()), existing); err != nil {
		h.handleStoreError(w, err)
		return
	}
	h.writeJSON(w, http.StatusOK, newContractView(existing))
}

func (h *Handler) DeleteContract(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUID(r.PathValue("id"))
	if err != nil {
		h.errorResponse(w, http.StatusBadRequest, "invalid id")
		return
	}

	if err := h.store.DeleteContract(r.Context(), middleware.GetUserID(r.Context()), id); err != nil {
		h.handleStoreError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
