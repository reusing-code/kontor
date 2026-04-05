package handler

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/tobi/contracts/backend/internal/ledgerimport"
	"github.com/tobi/contracts/backend/internal/middleware"
	"github.com/tobi/contracts/backend/internal/model"
	"github.com/tobi/contracts/backend/internal/store"
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

type ledgerTransactionPageResponse struct {
	Items      []model.LedgerTransaction `json:"items"`
	NextCursor string                    `json:"nextCursor,omitempty"`
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
		if err := h.readJSON(r, &body); err != nil {
			h.errorResponse(w, http.StatusBadRequest, "invalid JSON body: "+err.Error())
			return
		}
	}
	if body.AccountID != "" && body.NewAccount != nil {
		h.errorResponse(w, http.StatusBadRequest, "accountId and newAccount are mutually exclusive")
		return
	}

	result, err := h.ledgerImport.Commit(r.Context(), ledgerimport.CommitRequest{
		PreviewID:  previewID,
		AccountID:  body.AccountID,
		NewAccount: body.NewAccount,
		UserID:     userID,
	})
	if err != nil {
		if errors.Is(err, store.ErrNotFound) || errors.Is(err, store.ErrConflict) || errors.Is(err, store.ErrLedgerFileImported) || errors.Is(err, store.ErrLedgerPreviewExpired) {
			h.handleStoreError(w, err)
			return
		}
		h.logger.Error("ledger import commit failed", "error", err)
		h.errorResponse(w, http.StatusBadRequest, err.Error())
		return
	}

	h.writeJSON(w, http.StatusOK, result)
}

func (h *Handler) GetLedgerAccount(w http.ResponseWriter, r *http.Request) {
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

	account, err := h.store.GetLedgerAccount(r.Context(), userID, accountID)
	if err != nil {
		h.handleStoreError(w, err)
		return
	}

	h.writeJSON(w, http.StatusOK, account)
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

	if _, err := h.store.GetLedgerAccount(r.Context(), userID, accountID); err != nil {
		h.handleStoreError(w, err)
		return
	}

	limit := 100
	if rawLimit := r.URL.Query().Get("limit"); rawLimit != "" {
		parsedLimit, err := strconv.Atoi(rawLimit)
		if err != nil || parsedLimit <= 0 {
			h.errorResponse(w, http.StatusBadRequest, "invalid limit")
			return
		}
		limit = parsedLimit
	}

	page, err := h.store.ListLedgerTransactionsPage(r.Context(), userID, accountID, limit, r.URL.Query().Get("cursor"))
	if err != nil {
		h.handleStoreError(w, err)
		return
	}

	h.writeJSON(w, http.StatusOK, ledgerTransactionPageResponse{
		Items:      page.Items,
		NextCursor: page.NextCursor,
	})
}
