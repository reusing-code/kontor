package handler

import (
	"encoding/json"
	"net/http"

	"github.com/tobi/contracts/backend/internal/ledgerimport"
	"github.com/tobi/contracts/backend/internal/middleware"
	"github.com/tobi/contracts/backend/internal/model"
)

func (h *Handler) LedgerImportPreview(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())

	if err := r.ParseMultipartForm(32 << 20); err != nil {
		h.errorResponse(w, http.StatusBadRequest, "invalid multipart form")
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		h.errorResponse(w, http.StatusBadRequest, "missing file field")
		return
	}
	defer file.Close()

	sourceType := ledgerimport.SourceType(r.FormValue("sourceType"))
	if sourceType == "" {
		h.errorResponse(w, http.StatusBadRequest, "sourceType is required")
		return
	}

	result, err := h.ledgerImport.Preview(r.Context(), ledgerimport.PreviewRequest{
		File:       file,
		Filename:   header.Filename,
		SourceType: sourceType,
		AccountID:  r.FormValue("accountId"),
		UserID:     userID,
	})
	if err != nil {
		h.logger.Error("ledger import preview failed", "error", err)
		h.errorResponse(w, http.StatusBadRequest, err.Error())
		return
	}

	h.writeJSON(w, http.StatusOK, result)
}

type ledgerCommitBody struct {
	AccountID  string                    `json:"accountId,omitempty"`
	NewAccount *model.LedgerAccountInput `json:"newAccount,omitempty"`
}

func (h *Handler) LedgerImportCommit(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	previewID := r.PathValue("previewId")
	if previewID == "" {
		h.errorResponse(w, http.StatusBadRequest, "previewId is required")
		return
	}

	var body ledgerCommitBody
	if r.Body != nil && r.ContentLength > 0 {
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			h.errorResponse(w, http.StatusBadRequest, "invalid JSON body: "+err.Error())
			return
		}
	}

	result, err := h.ledgerImport.Commit(r.Context(), ledgerimport.CommitRequest{
		PreviewID:  previewID,
		AccountID:  body.AccountID,
		NewAccount: body.NewAccount,
		UserID:     userID,
	})
	if err != nil {
		h.logger.Error("ledger import commit failed", "error", err)
		h.errorResponse(w, http.StatusBadRequest, err.Error())
		return
	}

	h.writeJSON(w, http.StatusOK, result)
}

func (h *Handler) ListLedgerAccounts(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())

	accounts, err := h.store.ListLedgerAccounts(r.Context(), userID)
	if err != nil {
		h.handleStoreError(w, err)
		return
	}

	h.writeJSON(w, http.StatusOK, accounts)
}

func (h *Handler) ListLedgerImports(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())

	imports, err := h.store.ListLedgerImports(r.Context(), userID)
	if err != nil {
		h.handleStoreError(w, err)
		return
	}

	h.writeJSON(w, http.StatusOK, imports)
}

func (h *Handler) ListLedgerTransactions(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	accountIDStr := r.PathValue("accountId")
	if accountIDStr == "" {
		h.errorResponse(w, http.StatusBadRequest, "accountId is required")
		return
	}

	accountID, err := parseUUID(accountIDStr)
	if err != nil {
		h.errorResponse(w, http.StatusBadRequest, "invalid accountId")
		return
	}

	txns, err := h.store.ListLedgerTransactions(r.Context(), userID, accountID)
	if err != nil {
		h.handleStoreError(w, err)
		return
	}

	h.writeJSON(w, http.StatusOK, txns)
}
