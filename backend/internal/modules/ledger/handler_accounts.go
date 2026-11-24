package ledger

import (
	"context"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/reusing-code/kontor/backend/internal/httputil"
	"github.com/reusing-code/kontor/backend/internal/middleware"
)

// deleteAccountTimeout bounds a delete that is not cancelled with the client;
// an interrupted delete can be repeated.
const deleteAccountTimeout = 5 * time.Minute

func (h *Handler) accountID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, err := uuid.Parse(r.PathValue("accountId"))
	if err != nil {
		httputil.Error(h.logger, w, http.StatusBadRequest, "invalid accountId")
		return uuid.Nil, false
	}
	return id, true
}

func (h *Handler) UpdateLedgerAccount(w http.ResponseWriter, r *http.Request) {
	id, ok := h.accountID(w, r)
	if !ok {
		return
	}
	var input LedgerAccountUpdateInput
	if err := httputil.ReadJSON(r, &input); err != nil {
		httputil.BodyError(h.logger, w, err, "invalid request body")
		return
	}
	if err := input.Validate(); err != nil {
		httputil.Error(h.logger, w, http.StatusBadRequest, err.Error())
		return
	}
	account, err := h.store.UpdateLedgerAccount(r.Context(), middleware.GetUserID(r.Context()), id, input)
	if err != nil {
		h.storeError(w, err)
		return
	}
	httputil.WriteJSON(h.logger, w, http.StatusOK, account)
}

func (h *Handler) GetLedgerAccountDeleteSummary(w http.ResponseWriter, r *http.Request) {
	id, ok := h.accountID(w, r)
	if !ok {
		return
	}
	summary, err := h.store.LedgerAccountDeleteSummary(r.Context(), middleware.GetUserID(r.Context()), id)
	if err != nil {
		h.storeError(w, err)
		return
	}
	httputil.WriteJSON(h.logger, w, http.StatusOK, summary)
}

func (h *Handler) DeleteLedgerAccount(w http.ResponseWriter, r *http.Request) {
	id, ok := h.accountID(w, r)
	if !ok {
		return
	}
	userID := middleware.GetUserID(r.Context())
	ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), deleteAccountTimeout)
	defer cancel()
	if err := h.store.DeleteLedgerAccount(ctx, userID, id); err != nil {
		h.storeError(w, err)
		return
	}
	h.logger.Info("ledger account deleted", "user_id", userID, "account_id", id)
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) CreateLedgerAccount(w http.ResponseWriter, r *http.Request) {
	var input LedgerAccountInput
	if err := httputil.ReadJSON(r, &input); err != nil {
		httputil.BodyError(h.logger, w, err, "invalid request body")
		return
	}
	if err := input.ValidateForCreate(); err != nil {
		httputil.Error(h.logger, w, http.StatusBadRequest, err.Error())
		return
	}
	account := newLedgerAccount(uuid.New(), input, time.Now().UTC())
	if err := h.store.CreateLedgerAccount(r.Context(), middleware.GetUserID(r.Context()), account); err != nil {
		h.storeError(w, err)
		return
	}
	httputil.WriteJSON(h.logger, w, http.StatusCreated, account)
}

// newLedgerAccount builds the account a create request stores; input must
// have passed ValidateForCreate.
func newLedgerAccount(id uuid.UUID, input LedgerAccountInput, now time.Time) LedgerAccount {
	return LedgerAccount{
		ID:        id,
		Name:      input.Name,
		Bank:      input.Bank,
		IBAN:      input.IBAN,
		Currency:  input.Currency,
		CreatedAt: now,
		UpdatedAt: now,
	}
}

// SetLedgerAccountBalance stores a balance entered by hand and returns the
// account with its balance.
func (h *Handler) SetLedgerAccountBalance(w http.ResponseWriter, r *http.Request) {
	id, ok := h.accountID(w, r)
	if !ok {
		return
	}
	var input LedgerManualBalanceInput
	if err := httputil.ReadJSON(r, &input); err != nil {
		httputil.BodyError(h.logger, w, err, "invalid request body")
		return
	}
	now := time.Now().UTC()
	if err := input.Validate(now.Format(time.DateOnly)); err != nil {
		httputil.Error(h.logger, w, http.StatusBadRequest, err.Error())
		return
	}
	userID := middleware.GetUserID(r.Context())
	if err := h.store.PutLedgerBalanceSnapshot(r.Context(), userID, LedgerBalanceSnapshot{
		AccountID: id, AsOf: input.AsOf, AmountMinor: *input.AmountMinor, Source: LedgerBalanceSourceManual, RecordedAt: now,
	}); err != nil {
		h.storeError(w, err)
		return
	}
	h.writeAccountWithBalance(w, r, userID, id)
}

// DeleteLedgerAccountBalance deletes a manual balance (409 for one from an
// import or a sync) and returns the account with its remaining balance.
func (h *Handler) DeleteLedgerAccountBalance(w http.ResponseWriter, r *http.Request) {
	id, ok := h.accountID(w, r)
	if !ok {
		return
	}
	asOf := r.PathValue("asOf")
	if _, err := time.Parse(time.DateOnly, asOf); err != nil {
		httputil.Error(h.logger, w, http.StatusBadRequest, "asOf must be a date (YYYY-MM-DD)")
		return
	}
	userID := middleware.GetUserID(r.Context())
	if err := h.store.DeleteManualLedgerBalanceSnapshot(r.Context(), userID, id, asOf); err != nil {
		h.storeError(w, err)
		return
	}
	h.writeAccountWithBalance(w, r, userID, id)
}

func (h *Handler) writeAccountWithBalance(w http.ResponseWriter, r *http.Request, userID string, id uuid.UUID) {
	account, err := h.store.GetLedgerAccount(r.Context(), userID, id)
	if err != nil {
		h.storeError(w, err)
		return
	}
	balance, err := h.store.GetLedgerAccountBalance(r.Context(), userID, id)
	if err != nil {
		h.storeError(w, err)
		return
	}
	httputil.WriteJSON(h.logger, w, http.StatusOK, ledgerAccountResponse{LedgerAccount: account, Balance: balance})
}

type ledgerAccountTransactionPageResponse struct {
	Items          []LedgerTransaction   `json:"items"`
	Offset         int                   `json:"offset"`
	Total          int                   `json:"total"`
	PageBalance    *LedgerPageBalance    `json:"pageBalance,omitempty"`
	AccountBalance *LedgerAccountBalance `json:"accountBalance,omitempty"`
}

// ledgerAccountResponse is an account with its balance (omitted without a
// balance snapshot).
type ledgerAccountResponse struct {
	LedgerAccount
	Balance *LedgerAccountBalance `json:"balance,omitempty"`
}

func (h *Handler) GetLedgerAccount(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	accountIDStr := r.PathValue("accountId")
	if accountIDStr == "" {
		httputil.Error(h.logger, w, http.StatusBadRequest, "accountId is required")
		return
	}

	accountID, err := uuid.Parse(accountIDStr)
	if err != nil {
		httputil.Error(h.logger, w, http.StatusBadRequest, "invalid accountId")
		return
	}

	h.writeAccountWithBalance(w, r, userID, accountID)
}

func (h *Handler) ListLedgerAccounts(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())

	accounts, err := h.store.ListLedgerAccounts(r.Context(), userID)
	if err != nil {
		h.storeError(w, err)
		return
	}
	balances, err := h.store.ListLedgerAccountBalances(r.Context(), userID)
	if err != nil {
		h.storeError(w, err)
		return
	}
	items := make([]ledgerAccountResponse, len(accounts))
	for i, account := range accounts {
		items[i] = ledgerAccountResponse{LedgerAccount: account}
		if balance, ok := balances[account.ID]; ok {
			items[i].Balance = &balance
		}
	}

	httputil.WriteJSON(h.logger, w, http.StatusOK, items)
}

func (h *Handler) ListLedgerTransactions(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	accountIDStr := r.PathValue("accountId")
	if accountIDStr == "" {
		httputil.Error(h.logger, w, http.StatusBadRequest, "accountId is required")
		return
	}

	accountID, err := uuid.Parse(accountIDStr)
	if err != nil {
		httputil.Error(h.logger, w, http.StatusBadRequest, "invalid accountId")
		return
	}

	if _, err := h.store.GetLedgerAccount(r.Context(), userID, accountID); err != nil {
		h.storeError(w, err)
		return
	}

	limit, offset, err := pageQuery(r.URL.Query())
	if err != nil {
		httputil.Error(h.logger, w, http.StatusBadRequest, err.Error())
		return
	}

	page, err := h.store.ListLedgerAccountTransactionsPage(r.Context(), userID, accountID, limit, offset)
	if err != nil {
		h.storeError(w, err)
		return
	}

	httputil.WriteJSON(h.logger, w, http.StatusOK, ledgerAccountTransactionPageResponse{
		Items: page.Items, Offset: page.Offset, Total: page.Total, PageBalance: page.PageBalance, AccountBalance: page.Balance,
	})
}
