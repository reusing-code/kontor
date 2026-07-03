package ledger

import (
	"errors"
	"github.com/google/uuid"
	"github.com/reusing-code/kontor/backend/internal/httputil"
	"github.com/reusing-code/kontor/backend/internal/storage"
	"net/http"
	"strconv"

	"github.com/reusing-code/kontor/backend/internal/middleware"
)

func (h *Handler) LedgerImportPreview(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())

	if err := r.ParseMultipartForm(32 << 20); err != nil {
		httputil.Error(h.logger, w, http.StatusBadRequest, "invalid multipart form")
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		httputil.Error(h.logger, w, http.StatusBadRequest, "missing file field")
		return
	}
	defer file.Close()

	sourceType := SourceType(r.FormValue("sourceType"))
	if sourceType == "" {
		httputil.Error(h.logger, w, http.StatusBadRequest, "sourceType is required")
		return
	}

	result, err := h.ledgerImport.Preview(r.Context(), PreviewRequest{
		File:       file,
		Filename:   header.Filename,
		SourceType: sourceType,
		AccountID:  r.FormValue("accountId"),
		UserID:     userID,
	})
	if err != nil {
		h.logger.Error("ledger import preview failed", "error", err)
		httputil.Error(h.logger, w, http.StatusBadRequest, err.Error())
		return
	}

	httputil.WriteJSON(h.logger, w, http.StatusOK, result)
}

type ledgerCommitBody struct {
	AccountID  string              `json:"accountId,omitempty"`
	NewAccount *LedgerAccountInput `json:"newAccount,omitempty"`
}

type ledgerTransactionPageResponse struct {
	Items      []LedgerTransaction `json:"items"`
	NextCursor string              `json:"nextCursor,omitempty"`
}

func (h *Handler) LedgerImportCommit(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	previewID := r.PathValue("previewId")
	if previewID == "" {
		httputil.Error(h.logger, w, http.StatusBadRequest, "previewId is required")
		return
	}

	var body ledgerCommitBody
	if r.Body != nil && r.ContentLength > 0 {
		if err := httputil.ReadJSON(r, &body); err != nil {
			httputil.Error(h.logger, w, http.StatusBadRequest, "invalid JSON body: "+err.Error())
			return
		}
	}
	if body.AccountID != "" && body.NewAccount != nil {
		httputil.Error(h.logger, w, http.StatusBadRequest, "accountId and newAccount are mutually exclusive")
		return
	}

	result, err := h.ledgerImport.Commit(r.Context(), CommitRequest{
		PreviewID:  previewID,
		AccountID:  body.AccountID,
		NewAccount: body.NewAccount,
		UserID:     userID,
	})
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) || errors.Is(err, storage.ErrConflict) || errors.Is(err, ErrLedgerFileImported) || errors.Is(err, ErrLedgerPreviewExpired) {
			h.storeError(w, err)
			return
		}
		h.logger.Error("ledger import commit failed", "error", err)
		httputil.Error(h.logger, w, http.StatusBadRequest, err.Error())
		return
	}

	httputil.WriteJSON(h.logger, w, http.StatusOK, result)
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

	account, err := h.store.GetLedgerAccount(r.Context(), userID, accountID)
	if err != nil {
		h.storeError(w, err)
		return
	}

	httputil.WriteJSON(h.logger, w, http.StatusOK, account)
}

func (h *Handler) ListLedgerAccounts(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())

	accounts, err := h.store.ListLedgerAccounts(r.Context(), userID)
	if err != nil {
		h.storeError(w, err)
		return
	}

	httputil.WriteJSON(h.logger, w, http.StatusOK, accounts)
}

func (h *Handler) ListLedgerImports(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())

	imports, err := h.store.ListLedgerImports(r.Context(), userID)
	if err != nil {
		h.storeError(w, err)
		return
	}

	httputil.WriteJSON(h.logger, w, http.StatusOK, imports)
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

	limit := 100
	if rawLimit := r.URL.Query().Get("limit"); rawLimit != "" {
		parsedLimit, err := strconv.Atoi(rawLimit)
		if err != nil || parsedLimit <= 0 {
			httputil.Error(h.logger, w, http.StatusBadRequest, "invalid limit")
			return
		}
		limit = parsedLimit
	}

	page, err := h.store.ListLedgerTransactionsPage(r.Context(), userID, accountID, limit, r.URL.Query().Get("cursor"))
	if err != nil {
		h.storeError(w, err)
		return
	}

	httputil.WriteJSON(h.logger, w, http.StatusOK, ledgerTransactionPageResponse(page))
}
