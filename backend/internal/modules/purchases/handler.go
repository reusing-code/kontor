package purchases

import (
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/reusing-code/kontor/backend/internal/httputil"
	"github.com/reusing-code/kontor/backend/internal/middleware"
)

func (h *Handler) ListPurchases(w http.ResponseWriter, r *http.Request) {
	purchases, err := h.store.List(r.Context(), middleware.GetUserID(r.Context()))
	if err != nil {
		httputil.StoreError(h.logger, w, err)
		return
	}
	httputil.WriteJSON(h.logger, w, http.StatusOK, purchases)
}

func (h *Handler) ListPurchasesByCategory(w http.ResponseWriter, r *http.Request) {
	catID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		httputil.Error(h.logger, w, http.StatusBadRequest, "invalid category id")
		return
	}

	purchases, err := h.store.ListByCategory(r.Context(), middleware.GetUserID(r.Context()), catID)
	if err != nil {
		httputil.StoreError(h.logger, w, err)
		return
	}
	httputil.WriteJSON(h.logger, w, http.StatusOK, purchases)
}

func (h *Handler) CreatePurchaseInCategory(w http.ResponseWriter, r *http.Request) {
	catID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		httputil.Error(h.logger, w, http.StatusBadRequest, "invalid category id")
		return
	}

	if _, err := h.categories.Get(r.Context(), middleware.GetUserID(r.Context()), "purchases", catID); err != nil {
		httputil.StoreError(h.logger, w, err)
		return
	}

	var input PurchaseInput
	if err := httputil.ReadJSON(r, &input); err != nil {
		httputil.Error(h.logger, w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := input.Validate(); err != nil {
		httputil.Error(h.logger, w, http.StatusBadRequest, err.Error())
		return
	}

	now := time.Now().UTC()
	p := Purchase{
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

	if err := h.store.Create(r.Context(), middleware.GetUserID(r.Context()), p); err != nil {
		httputil.StoreError(h.logger, w, err)
		return
	}
	httputil.WriteJSON(h.logger, w, http.StatusCreated, p)
}

func (h *Handler) GetPurchase(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		httputil.Error(h.logger, w, http.StatusBadRequest, "invalid id")
		return
	}

	p, err := h.store.Get(r.Context(), middleware.GetUserID(r.Context()), id)
	if err != nil {
		httputil.StoreError(h.logger, w, err)
		return
	}
	httputil.WriteJSON(h.logger, w, http.StatusOK, p)
}

func (h *Handler) UpdatePurchase(w http.ResponseWriter, r *http.Request) {
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

	var input PurchaseInput
	if err := httputil.ReadJSON(r, &input); err != nil {
		httputil.Error(h.logger, w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := input.Validate(); err != nil {
		httputil.Error(h.logger, w, http.StatusBadRequest, err.Error())
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

	if err := h.store.Update(r.Context(), middleware.GetUserID(r.Context()), existing); err != nil {
		httputil.StoreError(h.logger, w, err)
		return
	}
	httputil.WriteJSON(h.logger, w, http.StatusOK, existing)
}

func (h *Handler) DeletePurchase(w http.ResponseWriter, r *http.Request) {
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
