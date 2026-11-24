package ledger

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/reusing-code/kontor/backend/internal/httputil"
	"github.com/reusing-code/kontor/backend/internal/middleware"
)

// today is the UTC date the analyses treat as today.
func (h *Handler) today() string {
	now := time.Now
	if h.now != nil {
		now = h.now
	}
	return now().UTC().Format(time.DateOnly)
}

// analysisRangeParams reads from and to; either may be empty. A range given
// in full is checked here, before anything is loaded. It writes the 400
// itself.
func (h *Handler) analysisRangeParams(w http.ResponseWriter, r *http.Request) (from, to string, ok bool) {
	query := r.URL.Query()
	from, to = query.Get("from"), query.Get("to")
	if from != "" {
		if _, err := time.Parse(time.DateOnly, from); err != nil {
			httputil.Error(h.logger, w, http.StatusBadRequest, "from must be a date (YYYY-MM-DD)")
			return "", "", false
		}
	}
	if to != "" {
		if _, err := time.Parse(time.DateOnly, to); err != nil {
			httputil.Error(h.logger, w, http.StatusBadRequest, "to must be a date (YYYY-MM-DD)")
			return "", "", false
		}
	}
	if from != "" && to != "" {
		if _, err := resolveLedgerAnalysisRange(from, to, h.today(), ""); err != nil {
			httputil.Error(h.logger, w, http.StatusBadRequest, err.Error())
			return "", "", false
		}
	}
	return from, to, true
}

// analysisIDParam reads an optional UUID query parameter and writes the 400
// itself.
func (h *Handler) analysisIDParam(w http.ResponseWriter, r *http.Request, name string) (*uuid.UUID, bool) {
	raw := r.URL.Query().Get(name)
	if raw == "" {
		return nil, true
	}
	id, err := uuid.Parse(raw)
	if err != nil {
		httputil.Error(h.logger, w, http.StatusBadRequest, "invalid "+name)
		return nil, false
	}
	return &id, true
}

// writeAnalysisError maps ErrLedgerAnalysisInvalid to 400 and leaves the rest to storeError.
func (h *Handler) writeAnalysisError(w http.ResponseWriter, err error) {
	if errors.Is(err, ErrLedgerAnalysisInvalid) {
		httputil.Error(h.logger, w, http.StatusBadRequest, err.Error())
		return
	}
	h.storeError(w, err)
}

func (h *Handler) GetLedgerSpendingAnalysis(w http.ResponseWriter, r *http.Request) {
	from, to, ok := h.analysisRangeParams(w, r)
	if !ok {
		return
	}
	accountID, ok := h.analysisIDParam(w, r, "accountId")
	if !ok {
		return
	}
	parentID, ok := h.analysisIDParam(w, r, "parentCategoryId")
	if !ok {
		return
	}
	in, err := h.store.LoadLedgerAnalysisData(r.Context(), middleware.GetUserID(r.Context()))
	if err != nil {
		h.storeError(w, err)
		return
	}
	analysis, err := AnalyzeLedgerSpending(in, from, to, h.today(), LedgerSpendingOptions{AccountID: accountID, ParentCategoryID: parentID})
	if err != nil {
		h.writeAnalysisError(w, err)
		return
	}
	httputil.WriteJSON(h.logger, w, http.StatusOK, analysis)
}

// serveLedgerRangeAnalysis answers an analysis that only takes from and to.
func serveLedgerRangeAnalysis[In, Out any](h *Handler, w http.ResponseWriter, r *http.Request,
	load func(ctx context.Context, userID string) (In, error),
	analyze func(in In, from, to, today string) (Out, error)) {
	from, to, ok := h.analysisRangeParams(w, r)
	if !ok {
		return
	}
	in, err := load(r.Context(), middleware.GetUserID(r.Context()))
	if err != nil {
		h.storeError(w, err)
		return
	}
	analysis, err := analyze(in, from, to, h.today())
	if err != nil {
		h.writeAnalysisError(w, err)
		return
	}
	httputil.WriteJSON(h.logger, w, http.StatusOK, analysis)
}

func (h *Handler) GetLedgerCashFlowAnalysis(w http.ResponseWriter, r *http.Request) {
	serveLedgerRangeAnalysis(h, w, r, h.store.LoadLedgerAnalysisData, AnalyzeLedgerCashFlow)
}

func (h *Handler) GetLedgerRankingAnalysis(w http.ResponseWriter, r *http.Request) {
	serveLedgerRangeAnalysis(h, w, r, h.store.LoadLedgerAnalysisData, AnalyzeLedgerRanking)
}

func (h *Handler) GetLedgerNetWorthAnalysis(w http.ResponseWriter, r *http.Request) {
	serveLedgerRangeAnalysis(h, w, r, h.store.LoadLedgerNetWorthData, AnalyzeLedgerNetWorth)
}

func (h *Handler) GetLedgerCategoryBreakdownAnalysis(w http.ResponseWriter, r *http.Request) {
	serveLedgerRangeAnalysis(h, w, r, h.store.LoadLedgerAnalysisData, AnalyzeLedgerCategoryBreakdown)
}
