package ledger

import (
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/reusing-code/kontor/backend/internal/httputil"
	"github.com/reusing-code/kontor/backend/internal/middleware"
)

func (h *Handler) CreateLedgerAccount(w http.ResponseWriter, r *http.Request) {
	var input LedgerAccountInput
	if err := httputil.ReadJSON(r, &input); err != nil {
		httputil.Error(h.logger, w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := input.ValidateForCreate(); err != nil {
		httputil.Error(h.logger, w, http.StatusBadRequest, err.Error())
		return
	}
	now := time.Now().UTC()
	account := LedgerAccount{
		ID:        uuid.New(),
		Name:      input.Name,
		Bank:      input.Bank,
		IBAN:      input.IBAN,
		Currency:  input.Currency,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := h.store.CreateLedgerAccount(r.Context(), middleware.GetUserID(r.Context()), account); err != nil {
		h.storeError(w, err)
		return
	}
	httputil.WriteJSON(h.logger, w, http.StatusCreated, account)
}
