package ledger

import (
	"errors"
	"net/http"
	"net/url"
	"strings"

	"github.com/google/uuid"
	"github.com/reusing-code/kontor/backend/internal/httputil"
	"github.com/reusing-code/kontor/backend/internal/middleware"
	"github.com/reusing-code/kontor/backend/internal/storage"
)

func writeBankError(h *Handler, w http.ResponseWriter, status int, code, msg string) {
	httputil.WriteJSON(h.logger, w, status, map[string]string{"error": msg, "code": code})
}

// bankError maps service errors to fixed responses; upstream text never
// reaches the client.
func (h *Handler) bankError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrBankImportDisabled):
		writeBankError(h, w, http.StatusConflict, "bank_import_disabled", "bank import is not enabled")
	case errors.Is(err, ErrBankPublicURLMissing):
		writeBankError(h, w, http.StatusConflict, "public_url_not_configured", "PUBLIC_URL is not configured")
	case errors.Is(err, ErrBankUnknownASPSP):
		writeBankError(h, w, http.StatusBadRequest, "unknown_bank", "unknown bank")
	case errors.Is(err, ErrBankInvalidCountry):
		writeBankError(h, w, http.StatusBadRequest, "invalid_country", "country must be a two-letter code")
	case errors.Is(err, ErrBankRestricted):
		writeBankError(h, w, http.StatusUnprocessableEntity, "bank_restricted", "the Enable Banking app can only connect its owner's accounts")
	case errors.Is(err, ErrBankUnavailable):
		writeBankError(h, w, http.StatusBadGateway, "bank_unavailable", "the bank service is unavailable")
	case errors.Is(err, ErrBankRateLimited):
		writeBankError(h, w, http.StatusTooManyRequests, "bank_rate_limited", "the bank service rate limit was reached")
	case errors.Is(err, ErrBankSyncRunning):
		writeBankError(h, w, http.StatusConflict, "sync_running", "a sync is already running")
	case errors.Is(err, ErrBankConsentExpired):
		writeBankError(h, w, http.StatusConflict, "consent_expired", "the bank consent expired")
	case errors.Is(err, ErrBankInvalidMapping):
		writeBankError(h, w, http.StatusBadRequest, "invalid_mapping", "the ledger account cannot be mapped to this bank account")
	case errors.Is(err, ErrBankCurrencyMismatch):
		writeBankError(h, w, http.StatusBadRequest, "currency_mismatch", "the currencies differ")
	case errors.Is(err, storage.ErrNotFound):
		writeBankError(h, w, http.StatusNotFound, "not_found", "not found")
	default:
		h.logger.Error("bank request failed", "error", err)
		writeBankError(h, w, http.StatusInternalServerError, "internal_error", "internal error")
	}
}

func (h *Handler) ListBankASPSPs(w http.ResponseWriter, r *http.Request) {
	banks, err := h.bank.ListASPSPs(r.Context(), r.URL.Query().Get("country"))
	if err != nil {
		h.bankError(w, err)
		return
	}
	httputil.WriteJSON(h.logger, w, http.StatusOK, banks)
}

func (h *Handler) ListBankConnections(w http.ResponseWriter, r *http.Request) {
	views, err := h.bank.Connections(r.Context(), middleware.GetUserID(r.Context()))
	if err != nil {
		h.bankError(w, err)
		return
	}
	httputil.WriteJSON(h.logger, w, http.StatusOK, views)
}

func (h *Handler) AuthorizeBank(w http.ResponseWriter, r *http.Request) {
	if !h.bank.active() {
		h.bankError(w, ErrBankImportDisabled)
		return
	}
	var in BankAuthorizeInput
	if err := httputil.ReadJSON(r, &in); err != nil || strings.TrimSpace(in.ASPSPName) == "" {
		writeBankError(h, w, http.StatusBadRequest, "invalid_request", "invalid request body")
		return
	}
	authURL, err := h.bank.StartAuthorization(r.Context(), middleware.GetUserID(r.Context()), in)
	if err != nil {
		h.bankError(w, err)
		return
	}
	httputil.WriteJSON(h.logger, w, http.StatusOK, map[string]string{"url": authURL})
}

func (h *Handler) DisconnectBank(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeBankError(h, w, http.StatusNotFound, "not_found", "not found")
		return
	}
	remoteClosed, err := h.bank.Disconnect(r.Context(), middleware.GetUserID(r.Context()), id)
	if err != nil {
		h.bankError(w, err)
		return
	}
	httputil.WriteJSON(h.logger, w, http.StatusOK, map[string]bool{"remoteClosed": remoteClosed})
}

func (h *Handler) MapBankAccount(w http.ResponseWriter, r *http.Request) {
	if !h.bank.active() {
		h.bankError(w, ErrBankImportDisabled)
		return
	}
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeBankError(h, w, http.StatusNotFound, "not_found", "not found")
		return
	}
	var body struct {
		LedgerAccountID uuid.UUID `json:"ledgerAccountId"`
	}
	if err := httputil.ReadJSON(r, &body); err != nil || body.LedgerAccountID == uuid.Nil {
		writeBankError(h, w, http.StatusBadRequest, "invalid_request", "invalid request body")
		return
	}
	view, err := h.bank.MapAccount(r.Context(), middleware.GetUserID(r.Context()), id, r.PathValue("uid"), body.LedgerAccountID)
	if err != nil {
		h.bankError(w, err)
		return
	}
	httputil.WriteJSON(h.logger, w, http.StatusOK, view)
}

// BankCallback is the public redirect target of the bank consent. The state
// in the query is its only credential; Authorization is never read. The
// answer is a relative 303 carrying only a fixed result code.
func (h *Handler) BankCallback(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	result := h.bank.CompleteAuthorization(r.Context(), q.Get("code"), q.Get("state"), q.Get("error"))

	location := "/ledger/bank-connections/result?bank=connected"
	if !result.Connected {
		location = "/ledger/bank-connections/result?bank=failed&reason=" + url.QueryEscape(result.Reason)
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("Location", location)
	w.WriteHeader(http.StatusSeeOther)
}
