package ledger

import (
	"errors"
	"net/http"

	"github.com/reusing-code/kontor/backend/internal/httputil"
	"github.com/reusing-code/kontor/backend/internal/middleware"
)

// maxLedgerImportBytes caps the whole multipart request of a CSV preview.
const maxLedgerImportBytes = 10 << 20

func (h *Handler) LedgerImportPreview(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())

	httputil.LimitBody(w, r, maxLedgerImportBytes)
	if err := r.ParseMultipartForm(maxLedgerImportBytes); err != nil {
		httputil.BodyError(h.logger, w, err, "invalid multipart form")
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
		h.importError(w, err)
		return
	}

	httputil.WriteJSON(h.logger, w, http.StatusOK, result)
}

type ledgerCommitBody struct {
	AccountID  string              `json:"accountId,omitempty"`
	NewAccount *LedgerAccountInput `json:"newAccount,omitempty"`
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
			if httputil.IsTooLarge(err) {
				httputil.TooLarge(h.logger, w)
				return
			}
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
		h.importError(w, err)
		return
	}

	h.afterImport(userID)
	httputil.WriteJSON(h.logger, w, http.StatusOK, result)
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

// importError answers a failed preview or commit: problems with the request
// or the file get 400 with their message, everything else (missing preview
// or account, duplicate file, store failures) goes through storeError.
func (h *Handler) importError(w http.ResponseWriter, err error) {
	if _, ok := errors.AsType[importInputError](err); ok {
		httputil.Error(h.logger, w, http.StatusBadRequest, err.Error())
		return
	}
	h.storeError(w, err)
}
