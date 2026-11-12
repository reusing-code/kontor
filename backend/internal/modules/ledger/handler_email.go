package ledger

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/reusing-code/kontor/backend/internal/httputil"

	"github.com/google/uuid"
	"github.com/reusing-code/kontor/backend/internal/cryptoutil"
	"github.com/reusing-code/kontor/backend/internal/middleware"
)

func (h *Handler) ListLedgerEmailAccounts(w http.ResponseWriter, r *http.Request) {
	items, err := h.store.ListLedgerEmailAccounts(r.Context(), middleware.GetUserID(r.Context()))
	if err != nil {
		h.storeError(w, err)
		return
	}
	httputil.WriteJSON(h.logger, w, http.StatusOK, items)
}

func (h *Handler) CreateLedgerEmailAccount(w http.ResponseWriter, r *http.Request) {
	if len(h.emailEncryptionKey) == 0 {
		httputil.Error(h.logger, w, http.StatusInternalServerError, "EMAIL_ENCRYPTION_KEY is not configured")
		return
	}
	var input LedgerEmailAccountInput
	if err := httputil.ReadJSON(r, &input); err != nil {
		httputil.Error(h.logger, w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := input.Validate(); err != nil {
		httputil.Error(h.logger, w, http.StatusBadRequest, err.Error())
		return
	}
	encryptedPassword, err := cryptoutil.EncryptString(input.Password, h.emailEncryptionKey)
	if err != nil {
		httputil.Error(h.logger, w, http.StatusInternalServerError, err.Error())
		return
	}
	now := time.Now().UTC()
	account := LedgerEmailAccount{
		ID:                uuid.New(),
		Name:              input.Name,
		IMAPHost:          input.IMAPHost,
		IMAPPort:          input.IMAPPort,
		Username:          input.Username,
		EncryptedPassword: encryptedPassword,
		UseTLS:            input.UseTLS,
		Folder:            input.Folder,
		PreClassify:       input.PreClassify,
		ScanSince:         input.ScanSince,
		CreatedAt:         now,
		UpdatedAt:         now,
	}
	if err := h.store.CreateLedgerEmailAccount(r.Context(), middleware.GetUserID(r.Context()), account); err != nil {
		h.storeError(w, err)
		return
	}
	httputil.WriteJSON(h.logger, w, http.StatusCreated, account)
}

func (h *Handler) GetLedgerEmailAccount(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("emailAccountId"))
	if err != nil {
		httputil.Error(h.logger, w, http.StatusBadRequest, "invalid emailAccountId")
		return
	}
	item, err := h.store.GetLedgerEmailAccount(r.Context(), middleware.GetUserID(r.Context()), id)
	if err != nil {
		h.storeError(w, err)
		return
	}
	httputil.WriteJSON(h.logger, w, http.StatusOK, item)
}

func (h *Handler) UpdateLedgerEmailAccount(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("emailAccountId"))
	if err != nil {
		httputil.Error(h.logger, w, http.StatusBadRequest, "invalid emailAccountId")
		return
	}
	var input LedgerEmailAccountUpdateInput
	if err := httputil.ReadJSON(r, &input); err != nil {
		httputil.Error(h.logger, w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := input.Validate(); err != nil {
		httputil.Error(h.logger, w, http.StatusBadRequest, err.Error())
		return
	}
	encryptedPassword := ""
	if input.Password != nil && *input.Password != "" {
		if len(h.emailEncryptionKey) == 0 {
			httputil.Error(h.logger, w, http.StatusInternalServerError, "EMAIL_ENCRYPTION_KEY is not configured")
			return
		}
		encrypted, err := cryptoutil.EncryptString(*input.Password, h.emailEncryptionKey)
		if err != nil {
			httputil.Error(h.logger, w, http.StatusInternalServerError, err.Error())
			return
		}
		encryptedPassword = encrypted
	}
	existing, err := h.store.ModifyLedgerEmailAccount(r.Context(), middleware.GetUserID(r.Context()), id, func(a *LedgerEmailAccount) {
		// Changed connection settings (or a new password) may fix a failed
		// scan: make the account due on the next scheduler tick.
		if a.IMAPHost != input.IMAPHost || a.IMAPPort != input.IMAPPort || a.Username != input.Username ||
			a.UseTLS != input.UseTLS || a.Folder != input.Folder || encryptedPassword != "" {
			a.LastScanAttemptAt = nil
		}
		a.Name = input.Name
		a.IMAPHost = input.IMAPHost
		a.IMAPPort = input.IMAPPort
		a.Username = input.Username
		a.UseTLS = input.UseTLS
		a.Folder = input.Folder
		a.PreClassify = input.PreClassify
		a.ScanSince = input.ScanSince
		if encryptedPassword != "" {
			a.EncryptedPassword = encryptedPassword
		}
		a.UpdatedAt = time.Now().UTC()
	})
	if err != nil {
		h.storeError(w, err)
		return
	}
	httputil.WriteJSON(h.logger, w, http.StatusOK, existing)
}

func (h *Handler) DeleteLedgerEmailAccount(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("emailAccountId"))
	if err != nil {
		httputil.Error(h.logger, w, http.StatusBadRequest, "invalid emailAccountId")
		return
	}
	if err := h.store.DeleteLedgerEmailAccount(r.Context(), middleware.GetUserID(r.Context()), id); err != nil {
		h.storeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) TestLedgerEmailAccount(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("emailAccountId"))
	if err != nil {
		httputil.Error(h.logger, w, http.StatusBadRequest, "invalid emailAccountId")
		return
	}
	if len(h.emailEncryptionKey) == 0 {
		httputil.Error(h.logger, w, http.StatusInternalServerError, "EMAIL_ENCRYPTION_KEY is not configured")
		return
	}
	account, err := h.store.GetLedgerEmailAccount(r.Context(), middleware.GetUserID(r.Context()), id)
	if err != nil {
		h.storeError(w, err)
		return
	}
	if account.EncryptedPassword == "" {
		httputil.Error(h.logger, w, http.StatusConflict, "no stored email password; edit the account and re-enter the password")
		return
	}
	password, err := cryptoutil.DecryptString(account.EncryptedPassword, h.emailEncryptionKey)
	if err != nil {
		httputil.Error(h.logger, w, http.StatusInternalServerError, "could not decrypt stored email password")
		return
	}
	if err := h.ledgerEmail.TestConnection(account, password); err != nil {
		httputil.Error(h.logger, w, http.StatusBadGateway, err.Error())
		return
	}
	httputil.WriteJSON(h.logger, w, http.StatusOK, map[string]any{"ok": true})
}

func (h *Handler) ListLedgerEmailImporters(w http.ResponseWriter, r *http.Request) {
	httputil.WriteJSON(h.logger, w, http.StatusOK, h.ledgerEmail.ListImporters())
}

func (h *Handler) ListLedgerEmailOrders(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	accountIDStr := r.URL.Query().Get("emailAccountId")
	status := r.URL.Query().Get("status")
	var (
		items []LedgerEmailOrder
		err   error
	)
	if accountIDStr != "" {
		accountID, parseErr := uuid.Parse(accountIDStr)
		if parseErr != nil {
			httputil.Error(h.logger, w, http.StatusBadRequest, "invalid emailAccountId")
			return
		}
		items, err = h.store.ListLedgerEmailOrdersByAccount(r.Context(), userID, accountID)
	} else {
		items, err = h.store.ListLedgerEmailOrders(r.Context(), userID)
	}
	if err != nil {
		h.storeError(w, err)
		return
	}
	if status != "" {
		filtered := make([]LedgerEmailOrder, 0, len(items))
		for _, item := range items {
			if item.MatchStatus == status {
				filtered = append(filtered, item)
			}
		}
		items = filtered
	}
	httputil.WriteJSON(h.logger, w, http.StatusOK, items)
}

func (h *Handler) GetLedgerEmailOrder(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("emailOrderId"))
	if err != nil {
		httputil.Error(h.logger, w, http.StatusBadRequest, "invalid emailOrderId")
		return
	}
	item, err := h.store.GetLedgerEmailOrder(r.Context(), middleware.GetUserID(r.Context()), id)
	if err != nil {
		h.storeError(w, err)
		return
	}
	httputil.WriteJSON(h.logger, w, http.StatusOK, item)
}

const maxLedgerEmailOrderCandidateAfterDays = 365

func (h *Handler) ListLedgerEmailOrderCandidates(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("emailOrderId"))
	if err != nil {
		httputil.Error(h.logger, w, http.StatusBadRequest, "invalid emailOrderId")
		return
	}
	afterDays := 0
	if raw := r.URL.Query().Get("afterDays"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 || parsed > maxLedgerEmailOrderCandidateAfterDays {
			httputil.Error(h.logger, w, http.StatusBadRequest, "invalid afterDays")
			return
		}
		afterDays = parsed
	}
	result, err := h.ledgerEmail.ListOrderCandidates(r.Context(), middleware.GetUserID(r.Context()), id, afterDays)
	if err != nil {
		h.storeError(w, err)
		return
	}
	httputil.WriteJSON(h.logger, w, http.StatusOK, result)
}

func (h *Handler) LinkLedgerEmailOrder(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("emailOrderId"))
	if err != nil {
		httputil.Error(h.logger, w, http.StatusBadRequest, "invalid emailOrderId")
		return
	}
	var input LedgerEmailOrderLinkInput
	if err := httputil.ReadJSON(r, &input); err != nil {
		httputil.Error(h.logger, w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := input.Validate(); err != nil {
		httputil.Error(h.logger, w, http.StatusBadRequest, err.Error())
		return
	}
	item, err := h.store.LinkLedgerEmailOrder(r.Context(), middleware.GetUserID(r.Context()), id, input)
	if err != nil {
		h.storeError(w, err)
		return
	}
	httputil.WriteJSON(h.logger, w, http.StatusOK, item)
}

func (h *Handler) RejectLedgerEmailOrder(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("emailOrderId"))
	if err != nil {
		httputil.Error(h.logger, w, http.StatusBadRequest, "invalid emailOrderId")
		return
	}
	item, err := h.store.RejectLedgerEmailOrder(r.Context(), middleware.GetUserID(r.Context()), id)
	if err != nil {
		h.storeError(w, err)
		return
	}
	httputil.WriteJSON(h.logger, w, http.StatusOK, item)
}

func (h *Handler) scanError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrEmailFolderMissing):
		httputil.Error(h.logger, w, http.StatusConflict, "no folder to scan; edit the account and choose a folder")
	case errors.Is(err, ErrIMAP):
		httputil.Error(h.logger, w, http.StatusBadGateway, err.Error())
	default:
		h.storeError(w, err)
	}
}

func (h *Handler) ScanLedgerEmailAccount(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("emailAccountId"))
	if err != nil {
		httputil.Error(h.logger, w, http.StatusBadRequest, "invalid emailAccountId")
		return
	}
	account, err := h.store.GetLedgerEmailAccount(r.Context(), middleware.GetUserID(r.Context()), id)
	if err != nil {
		h.storeError(w, err)
		return
	}
	userID := middleware.GetUserID(r.Context())
	var result LedgerEmailScanResult
	contentType := strings.TrimSpace(r.Header.Get("Content-Type"))
	isUpload := strings.HasPrefix(contentType, "multipart/form-data")
	if isUpload {
		if err := r.ParseMultipartForm(32 << 20); err != nil {
			httputil.Error(h.logger, w, http.StatusBadRequest, "invalid multipart form")
			return
		}
		files := r.MultipartForm.File["files"]
		if len(files) == 0 {
			httputil.Error(h.logger, w, http.StatusBadRequest, "missing files field")
			return
		}
		uploads := make([]UploadedMessage, 0, len(files))
		for _, header := range files {
			file, err := header.Open()
			if err != nil {
				httputil.Error(h.logger, w, http.StatusBadRequest, "could not open uploaded file")
				return
			}
			data, err := io.ReadAll(file)
			file.Close()
			if err != nil {
				httputil.Error(h.logger, w, http.StatusBadRequest, "could not read uploaded file")
				return
			}
			uploads = append(uploads, UploadedMessage{Filename: header.Filename, Reader: bytes.NewReader(data)})
		}
		result, err = h.ledgerEmail.ScanUploadedMessages(r.Context(), userID, account, uploads)
		if err != nil {
			h.scanError(w, err)
			return
		}
	} else {
		if account.Folder == "" {
			h.scanError(w, ErrEmailFolderMissing)
			return
		}
		if len(h.emailEncryptionKey) == 0 {
			httputil.Error(h.logger, w, http.StatusInternalServerError, "EMAIL_ENCRYPTION_KEY is not configured")
			return
		}
		if account.EncryptedPassword == "" {
			httputil.Error(h.logger, w, http.StatusConflict, "no stored email password; edit the account and re-enter the password")
			return
		}
		password, err := cryptoutil.DecryptString(account.EncryptedPassword, h.emailEncryptionKey)
		if err != nil {
			httputil.Error(h.logger, w, http.StatusInternalServerError, "could not decrypt stored email password")
			return
		}
		result, err = h.ledgerEmail.ScanMailbox(r.Context(), userID, account, password)
		if err != nil {
			h.scanError(w, err)
			return
		}
	}
	// Only an IMAP scan moves the lookback start; uploads scan no mailbox.
	state := LedgerEmailScanState{Status: "Processed uploaded email messages"}
	if !isUpload {
		now := time.Now().UTC()
		state = LedgerEmailScanState{Status: fmt.Sprintf("Scanned folder %q via IMAP", account.Folder), AttemptedAt: &now, CompletedAt: &now}
	}
	if err := h.store.SetLedgerEmailAccountScanState(r.Context(), userID, account.ID, state); err != nil {
		h.storeError(w, err)
		return
	}
	httputil.WriteJSON(h.logger, w, http.StatusOK, result)
}
