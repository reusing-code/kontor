package ledger

import (
	"bytes"
	"github.com/reusing-code/kontor/backend/internal/httputil"
	"io"
	"net/http"
	"strings"
	"time"

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
	existing, err := h.store.GetLedgerEmailAccount(r.Context(), middleware.GetUserID(r.Context()), id)
	if err != nil {
		h.storeError(w, err)
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
	existing.Name = input.Name
	existing.IMAPHost = input.IMAPHost
	existing.IMAPPort = input.IMAPPort
	existing.Username = input.Username
	existing.UseTLS = input.UseTLS
	existing.ScanSince = input.ScanSince
	existing.UpdatedAt = time.Now().UTC()
	if input.Password != nil && *input.Password != "" {
		if len(h.emailEncryptionKey) == 0 {
			httputil.Error(h.logger, w, http.StatusInternalServerError, "EMAIL_ENCRYPTION_KEY is not configured")
			return
		}
		encryptedPassword, err := cryptoutil.EncryptString(*input.Password, h.emailEncryptionKey)
		if err != nil {
			httputil.Error(h.logger, w, http.StatusInternalServerError, err.Error())
			return
		}
		existing.EncryptedPassword = encryptedPassword
	}
	if err := h.store.UpdateLedgerEmailAccount(r.Context(), middleware.GetUserID(r.Context()), existing); err != nil {
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
	if strings.HasPrefix(contentType, "multipart/form-data") {
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
	} else {
		if len(h.emailEncryptionKey) == 0 {
			httputil.Error(h.logger, w, http.StatusInternalServerError, "EMAIL_ENCRYPTION_KEY is not configured")
			return
		}
		password, err := cryptoutil.DecryptString(account.EncryptedPassword, h.emailEncryptionKey)
		if err != nil {
			httputil.Error(h.logger, w, http.StatusInternalServerError, "could not decrypt stored email password")
			return
		}
		mailboxResult, scanErr := h.ledgerEmail.ScanMailbox(r.Context(), userID, account, password)
		if scanErr != nil {
			h.storeError(w, scanErr)
			return
		}
		result = mailboxResult
	}
	if err != nil {
		h.storeError(w, err)
		return
	}
	now := time.Now().UTC()
	account.LastScanAt = &now
	if strings.HasPrefix(contentType, "multipart/form-data") {
		account.LastScanStatusMessage = "Processed uploaded email messages"
	} else {
		account.LastScanStatusMessage = "Scanned inbox via IMAP"
	}
	account.UpdatedAt = now
	if err := h.store.UpdateLedgerEmailAccount(r.Context(), userID, account); err != nil {
		h.storeError(w, err)
		return
	}
	httputil.WriteJSON(h.logger, w, http.StatusOK, result)
}
