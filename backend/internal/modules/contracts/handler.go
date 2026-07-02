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
	catID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		httputil.Error(h.logger, w, http.StatusBadRequest, "invalid category id")
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
	catID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		httputil.Error(h.logger, w, http.StatusBadRequest, "invalid category id")
		return
	}

	// Verify category exists (contracts module)
	if _, err := h.categories.Get(r.Context(), middleware.GetUserID(r.Context()), "contracts", catID); err != nil {
		httputil.StoreError(h.logger, w, err)
		return
	}

	var input ContractInput
	if err := httputil.ReadJSON(r, &input); err != nil {
		httputil.Error(h.logger, w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := input.Validate(); err != nil {
		httputil.Error(h.logger, w, http.StatusBadRequest, err.Error())
		return
	}

	now := time.Now().UTC()
	bi := input.BillingInterval
	if bi == "" {
		bi = BillingMonthly
	}
	con := Contract{
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

	if err := h.store.Create(r.Context(), middleware.GetUserID(r.Context()), con); err != nil {
		httputil.StoreError(h.logger, w, err)
		return
	}
	httputil.WriteJSON(h.logger, w, http.StatusCreated, newContractView(con))
}

func (h *Handler) GetContract(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		httputil.Error(h.logger, w, http.StatusBadRequest, "invalid id")
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
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		httputil.Error(h.logger, w, http.StatusBadRequest, "invalid id")
		return
	}

	existing, err := h.store.Get(r.Context(), middleware.GetUserID(r.Context()), id)
	if err != nil {
		httputil.StoreError(h.logger, w, err)
		return
	}

	var input ContractInput
	if err := httputil.ReadJSON(r, &input); err != nil {
		httputil.Error(h.logger, w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := input.Validate(); err != nil {
		httputil.Error(h.logger, w, http.StatusBadRequest, err.Error())
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
		bi = BillingMonthly
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

	if err := h.store.Update(r.Context(), middleware.GetUserID(r.Context()), existing); err != nil {
		httputil.StoreError(h.logger, w, err)
		return
	}
	httputil.WriteJSON(h.logger, w, http.StatusOK, newContractView(existing))
}

func (h *Handler) DeleteContract(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		httputil.Error(h.logger, w, http.StatusBadRequest, "invalid id")
		return
	}

	if err := h.store.Delete(r.Context(), middleware.GetUserID(r.Context()), id); err != nil {
		httputil.StoreError(h.logger, w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
