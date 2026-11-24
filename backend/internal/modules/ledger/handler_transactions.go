package ledger

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/google/uuid"
	"github.com/reusing-code/kontor/backend/internal/httputil"
	"github.com/reusing-code/kontor/backend/internal/middleware"
)

type ledgerTransferCandidatesResponse struct {
	Items []LedgerTransferCandidate `json:"items"`
}

type ledgerTransferLinkResponse struct {
	Transaction       LedgerTransaction  `json:"transaction"`
	PairedTransaction *LedgerTransaction `json:"pairedTransaction,omitempty"`
}

func (h *Handler) GetLedgerTransaction(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("transactionId"))
	if err != nil {
		httputil.Error(h.logger, w, http.StatusBadRequest, "invalid transactionId")
		return
	}
	ledgerTxn, err := h.store.GetLedgerTransaction(r.Context(), middleware.GetUserID(r.Context()), id)
	if err != nil {
		h.storeError(w, err)
		return
	}
	httputil.WriteJSON(h.logger, w, http.StatusOK, ledgerTxn)
}

func (h *Handler) UpdateLedgerTransactionDetails(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("transactionId"))
	if err != nil {
		httputil.Error(h.logger, w, http.StatusBadRequest, "invalid transactionId")
		return
	}
	var input LedgerTransactionDetailsInput
	if err := httputil.ReadJSON(r, &input); err != nil {
		httputil.BodyError(h.logger, w, err, "invalid request body")
		return
	}
	if err := input.Validate(); err != nil {
		httputil.Error(h.logger, w, http.StatusBadRequest, err.Error())
		return
	}
	ledgerTxn, err := h.store.UpdateLedgerTransactionDetails(r.Context(), middleware.GetUserID(r.Context()), id, input)
	if err != nil {
		h.storeError(w, err)
		return
	}
	httputil.WriteJSON(h.logger, w, http.StatusOK, ledgerTxn)
}

// SetLedgerTransactionSplits replaces the parts of a transaction
// (`{splits: [...]}`, an empty list removes the split).
func (h *Handler) SetLedgerTransactionSplits(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("transactionId"))
	if err != nil {
		httputil.Error(h.logger, w, http.StatusBadRequest, "invalid transactionId")
		return
	}
	var input LedgerTransactionSplitsInput
	if err := httputil.ReadJSON(r, &input); err != nil {
		httputil.BodyError(h.logger, w, err, "invalid request body")
		return
	}
	ledgerTxn, err := h.store.SetLedgerTransactionSplits(r.Context(), middleware.GetUserID(r.Context()), id, input.Splits)
	if err != nil {
		h.storeError(w, err)
		return
	}
	httputil.WriteJSON(h.logger, w, http.StatusOK, ledgerTxn)
}

// ErrTargetModuleDisabled means the module that owns a reference target is
// disabled for the user.
var ErrTargetModuleDisabled = errors.New("target module is disabled")

var referenceModules = map[string]string{
	LedgerReferenceContract: "contracts",
	LedgerReferencePurchase: "purchases",
	LedgerReferenceVehicle:  "auto",
	LedgerReferenceCost:     "auto",
}

func writeModuleDisabled(w http.ResponseWriter, err error) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusForbidden)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error(), "code": "module_disabled"})
}

// linkReference adds one reference after checking that the target's module
// is enabled for the user; added is false when it was already there.
func (h *Handler) linkReference(ctx context.Context, userID string, txnID uuid.UUID, ref LedgerTransactionReference) (LedgerTransaction, bool, error) {
	moduleID := referenceModules[ref.Type]
	if h.enabled != nil && moduleID != "" {
		enabled, err := h.enabled.ModuleEnabled(ctx, userID, moduleID)
		if err != nil {
			return LedgerTransaction{}, false, err
		}
		if !enabled {
			return LedgerTransaction{}, false, fmt.Errorf("%w: the %s module is disabled", ErrTargetModuleDisabled, moduleID)
		}
	}
	return h.store.AddLedgerTransactionReference(ctx, userID, txnID, ref)
}

func (h *Handler) AddLedgerTransactionReference(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("transactionId"))
	if err != nil {
		httputil.Error(h.logger, w, http.StatusBadRequest, "invalid transactionId")
		return
	}
	var input LedgerTransactionReferenceInput
	if err := httputil.ReadJSON(r, &input); err != nil {
		httputil.BodyError(h.logger, w, err, "invalid request body")
		return
	}
	if err := input.Validate(); err != nil {
		httputil.Error(h.logger, w, http.StatusBadRequest, err.Error())
		return
	}
	ledgerTxn, _, err := h.linkReference(r.Context(), middleware.GetUserID(r.Context()),
		id, LedgerTransactionReference(input))
	if err != nil {
		h.storeError(w, err)
		return
	}
	httputil.WriteJSON(h.logger, w, http.StatusOK, ledgerTxn)
}

func (h *Handler) ListLedgerTransferCandidates(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("transactionId"))
	if err != nil {
		httputil.Error(h.logger, w, http.StatusBadRequest, "invalid transactionId")
		return
	}
	result, err := h.store.ListLedgerTransferCandidates(r.Context(), middleware.GetUserID(r.Context()), id)
	if err != nil {
		h.storeError(w, err)
		return
	}
	httputil.WriteJSON(h.logger, w, http.StatusOK, ledgerTransferCandidatesResponse(result))
}

func (h *Handler) LinkLedgerTransfer(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("transactionId"))
	if err != nil {
		httputil.Error(h.logger, w, http.StatusBadRequest, "invalid transactionId")
		return
	}
	var input LedgerTransferLinkInput
	if err := httputil.ReadJSON(r, &input); err != nil {
		httputil.BodyError(h.logger, w, err, "invalid request body")
		return
	}
	if err := input.Validate(); err != nil {
		httputil.Error(h.logger, w, http.StatusBadRequest, err.Error())
		return
	}
	result, err := h.store.LinkLedgerTransfer(r.Context(), middleware.GetUserID(r.Context()), id, input)
	if err != nil {
		h.storeError(w, err)
		return
	}
	httputil.WriteJSON(h.logger, w, http.StatusOK, ledgerTransferLinkResponse{Transaction: result.Transaction, PairedTransaction: &result.PairedTransaction})
}

func (h *Handler) UnlinkLedgerTransfer(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("transactionId"))
	if err != nil {
		httputil.Error(h.logger, w, http.StatusBadRequest, "invalid transactionId")
		return
	}
	result, err := h.store.UnlinkLedgerTransfer(r.Context(), middleware.GetUserID(r.Context()), id)
	if err != nil {
		h.storeError(w, err)
		return
	}
	httputil.WriteJSON(h.logger, w, http.StatusOK, ledgerTransferLinkResponse(result))
}
