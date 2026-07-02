package handler

import (
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/reusing-code/kontor/backend/internal/middleware"
	"github.com/reusing-code/kontor/backend/internal/model"
)

func (h *Handler) ListPurchases(w http.ResponseWriter, r *http.Request) {
	purchases, err := h.store.ListPurchases(r.Context(), middleware.GetUserID(r.Context()))
	if err != nil {
		h.handleStoreError(w, err)
		return
	}
	h.writeJSON(w, http.StatusOK, purchases)
}

func (h *Handler) ListPurchasesByCategory(w http.ResponseWriter, r *http.Request) {
	catID, err := parseUUID(r.PathValue("id"))
	if err != nil {
		h.errorResponse(w, http.StatusBadRequest, "invalid category id")
		return
	}

	purchases, err := h.store.ListPurchasesByCategory(r.Context(), middleware.GetUserID(r.Context()), catID)
	if err != nil {
		h.handleStoreError(w, err)
		return
	}
	h.writeJSON(w, http.StatusOK, purchases)
}

func (h *Handler) CreatePurchaseInCategory(w http.ResponseWriter, r *http.Request) {
	catID, err := parseUUID(r.PathValue("id"))
	if err != nil {
		h.errorResponse(w, http.StatusBadRequest, "invalid category id")
		return
	}

	if _, err := h.store.GetCategory(r.Context(), middleware.GetUserID(r.Context()), "purchases", catID); err != nil {
		h.handleStoreError(w, err)
		return
	}

	var input model.PurchaseInput
	if err := h.readJSON(r, &input); err != nil {
		h.errorResponse(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := input.Validate(); err != nil {
		h.errorResponse(w, http.StatusBadRequest, err.Error())
		return
	}

	now := time.Now().UTC()
	p := model.Purchase{
		ID:             uuid.New(),
		CategoryID:     catID,
		Type:           input.Type,
		ItemName:       input.ItemName,
		Brand:          input.Brand,
		ArticleNumber:  input.ArticleNumber,
		Dealer:         input.Dealer,
		Price:          input.Price,
		PurchaseDate:   input.PurchaseDate,
		DescriptionURL: input.DescriptionURL,
		InvoiceURL:     input.InvoiceURL,
		HandbookURL:    input.HandbookURL,
		Consumables:    input.Consumables,
		Comments:       input.Comments,
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	if err := h.store.CreatePurchase(r.Context(), middleware.GetUserID(r.Context()), p); err != nil {
		h.handleStoreError(w, err)
		return
	}
	h.writeJSON(w, http.StatusCreated, p)
}

func (h *Handler) GetPurchase(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUID(r.PathValue("id"))
	if err != nil {
		h.errorResponse(w, http.StatusBadRequest, "invalid id")
		return
	}

	p, err := h.store.GetPurchase(r.Context(), middleware.GetUserID(r.Context()), id)
	if err != nil {
		h.handleStoreError(w, err)
		return
	}
	h.writeJSON(w, http.StatusOK, p)
}

func (h *Handler) UpdatePurchase(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUID(r.PathValue("id"))
	if err != nil {
		h.errorResponse(w, http.StatusBadRequest, "invalid id")
		return
	}

	existing, err := h.store.GetPurchase(r.Context(), middleware.GetUserID(r.Context()), id)
	if err != nil {
		h.handleStoreError(w, err)
		return
	}

	var input model.PurchaseInput
	if err := h.readJSON(r, &input); err != nil {
		h.errorResponse(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := input.Validate(); err != nil {
		h.errorResponse(w, http.StatusBadRequest, err.Error())
		return
	}

	existing.Type = input.Type
	existing.ItemName = input.ItemName
	existing.Brand = input.Brand
	existing.ArticleNumber = input.ArticleNumber
	existing.Dealer = input.Dealer
	existing.Price = input.Price
	existing.PurchaseDate = input.PurchaseDate
	existing.DescriptionURL = input.DescriptionURL
	existing.InvoiceURL = input.InvoiceURL
	existing.HandbookURL = input.HandbookURL
	existing.Consumables = input.Consumables
	existing.Comments = input.Comments
	existing.UpdatedAt = time.Now().UTC()

	if err := h.store.UpdatePurchase(r.Context(), middleware.GetUserID(r.Context()), existing); err != nil {
		h.handleStoreError(w, err)
		return
	}
	h.writeJSON(w, http.StatusOK, existing)
}

func (h *Handler) DeletePurchase(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUID(r.PathValue("id"))
	if err != nil {
		h.errorResponse(w, http.StatusBadRequest, "invalid id")
		return
	}

	if err := h.store.DeletePurchase(r.Context(), middleware.GetUserID(r.Context()), id); err != nil {
		h.handleStoreError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
