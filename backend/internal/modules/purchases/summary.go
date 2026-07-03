package purchases

import (
	"net/http"

	"github.com/google/uuid"
	"github.com/reusing-code/kontor/backend/internal/httputil"
	"github.com/reusing-code/kontor/backend/internal/middleware"
)

type purchaseCategorySummary struct {
	ID            uuid.UUID `json:"id"`
	Name          string    `json:"name"`
	PurchaseCount int       `json:"purchaseCount"`
	TotalSpent    float64   `json:"totalSpent"`
}

type purchaseSummaryResponse struct {
	TotalPurchases int                       `json:"totalPurchases"`
	TotalSpent     float64                   `json:"totalSpent"`
	Categories     []purchaseCategorySummary `json:"categories"`
}

func (h *Handler) PurchaseSummary(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())

	cats, err := h.categories.List(r.Context(), userID, "purchases")
	if err != nil {
		httputil.StoreError(h.logger, w, err)
		return
	}

	purchases, err := h.store.List(r.Context(), userID)
	if err != nil {
		httputil.StoreError(h.logger, w, err)
		return
	}

	type agg struct {
		count int
		total float64
	}
	byCategory := make(map[uuid.UUID]*agg)
	for _, cat := range cats {
		byCategory[cat.ID] = &agg{}
	}

	var totalSpent float64
	for _, p := range purchases {
		a, ok := byCategory[p.CategoryID]
		if !ok {
			a = &agg{}
			byCategory[p.CategoryID] = a
		}
		a.count++
		if p.Price != nil {
			a.total += *p.Price
			totalSpent += *p.Price
		}
	}

	catSummaries := make([]purchaseCategorySummary, 0, len(cats))
	for _, cat := range cats {
		a := byCategory[cat.ID]
		catSummaries = append(catSummaries, purchaseCategorySummary{
			ID:            cat.ID,
			Name:          cat.Name,
			PurchaseCount: a.count,
			TotalSpent:    a.total,
		})
	}

	httputil.WriteJSON(h.logger, w, http.StatusOK, purchaseSummaryResponse{
		TotalPurchases: len(purchases),
		TotalSpent:     totalSpent,
		Categories:     catSummaries,
	})
}
