package contracts

import (
	"net/http"
	"sort"
	"strconv"
	"time"

	"github.com/reusing-code/kontor/backend/internal/httputil"
	"github.com/reusing-code/kontor/backend/internal/middleware"
)

type contractView struct {
	Contract
	CancellationDate *string `json:"cancellationDate,omitempty"`
	Expired          bool    `json:"expired"`
}

func newContractView(c Contract) contractView {
	return contractView{
		Contract:         c,
		CancellationDate: c.CancellationDate(),
		Expired:          c.IsExpired(),
	}
}

func newContractViews(cs []Contract) []contractView {
	out := make([]contractView, len(cs))
	for i, c := range cs {
		out[i] = newContractView(c)
	}
	return out
}

func (h *Handler) UpcomingRenewals(w http.ResponseWriter, r *http.Request) {
	days := 90
	if v := r.URL.Query().Get("days"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 {
			httputil.Error(h.logger, w, http.StatusBadRequest, "days must be a positive integer")
			return
		}
		if n > 365 {
			n = 365
		}
		days = n
	}

	contracts, err := h.store.List(r.Context(), middleware.GetUserID(r.Context()))
	if err != nil {
		httputil.StoreError(h.logger, w, err)
		return
	}

	today := time.Now().UTC().Truncate(24 * time.Hour)
	deadline := today.AddDate(0, 0, days)

	var upcoming []contractView
	for _, c := range contracts {
		cv := newContractView(c)
		if cv.CancellationDate == nil {
			continue
		}
		d, err := time.Parse("2006-01-02", *cv.CancellationDate)
		if err != nil {
			continue
		}
		if !d.Before(today) && !d.After(deadline) {
			upcoming = append(upcoming, cv)
		}
	}

	sort.Slice(upcoming, func(i, j int) bool {
		return *upcoming[i].CancellationDate < *upcoming[j].CancellationDate
	})

	if upcoming == nil {
		upcoming = []contractView{}
	}

	httputil.WriteJSON(h.logger, w, http.StatusOK, upcoming)
}
