package contracts

import (
	"net/http"

	"github.com/google/uuid"
	"github.com/reusing-code/kontor/backend/internal/httputil"
	"github.com/reusing-code/kontor/backend/internal/middleware"
)

type categorySummary struct {
	ID            uuid.UUID `json:"id"`
	Name          string    `json:"name"`
	ContractCount int       `json:"contractCount"`
	MonthlyTotal  float64   `json:"monthlyTotal"`
	YearlyTotal   float64   `json:"yearlyTotal"`
}

type summaryResponse struct {
	TotalContracts     int               `json:"totalContracts"`
	TotalMonthlyAmount float64           `json:"totalMonthlyAmount"`
	TotalYearlyAmount  float64           `json:"totalYearlyAmount"`
	Categories         []categorySummary `json:"categories"`
}

func (h *Handler) Summary(w http.ResponseWriter, r *http.Request) {
	cats, err := h.categories.List(r.Context(), middleware.GetUserID(r.Context()), "contracts")
	if err != nil {
		httputil.StoreError(h.logger, w, err)
		return
	}

	contracts, err := h.store.List(r.Context(), middleware.GetUserID(r.Context()))
	if err != nil {
		httputil.StoreError(h.logger, w, err)
		return
	}

	// Build per-category aggregation
	type agg struct {
		count        int
		monthlyTotal float64
		yearlyTotal  float64
	}
	byCategory := make(map[uuid.UUID]*agg)
	for _, cat := range cats {
		byCategory[cat.ID] = &agg{}
	}

	var totalMonthly, totalYearly float64
	for _, con := range contracts {
		a, ok := byCategory[con.CategoryID]
		if !ok {
			a = &agg{}
			byCategory[con.CategoryID] = a
		}
		a.count++
		a.monthlyTotal += con.MonthlyPrice()
		a.yearlyTotal += con.YearlyPrice()
		totalMonthly += con.MonthlyPrice()
		totalYearly += con.YearlyPrice()
	}

	catSummaries := make([]categorySummary, 0, len(cats))
	for _, cat := range cats {
		a := byCategory[cat.ID]
		catSummaries = append(catSummaries, categorySummary{
			ID:            cat.ID,
			Name:          cat.Name,
			ContractCount: a.count,
			MonthlyTotal:  a.monthlyTotal,
			YearlyTotal:   a.yearlyTotal,
		})
	}

	httputil.WriteJSON(h.logger, w, http.StatusOK, summaryResponse{
		TotalContracts:     len(contracts),
		TotalMonthlyAmount: totalMonthly,
		TotalYearlyAmount:  totalYearly,
		Categories:         catSummaries,
	})
}
