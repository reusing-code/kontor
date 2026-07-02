package handler

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/google/uuid"
	"github.com/reusing-code/kontor/backend/internal/cryptoutil"
	"github.com/reusing-code/kontor/backend/internal/email"
	"github.com/reusing-code/kontor/backend/internal/httputil"
	"github.com/reusing-code/kontor/backend/internal/ledgeremail"
	"github.com/reusing-code/kontor/backend/internal/ledgerimport"
	"github.com/reusing-code/kontor/backend/internal/store"
)

type Handler struct {
	store              store.Store
	logger             *slog.Logger
	jwtSecret          []byte
	emailClient        *email.Client
	ledgerImport       *ledgerimport.Service
	ledgerEmail        *ledgeremail.Service
	emailEncryptionKey []byte
}

func New(s store.Store, logger *slog.Logger, jwtSecret []byte, emailClient *email.Client, rawEmailEncryptionKey ...string) *Handler {
	key := ""
	if len(rawEmailEncryptionKey) > 0 {
		key = rawEmailEncryptionKey[0]
	}
	emailEncryptionKey, err := cryptoutil.NormalizeEncryptionKey(key)
	if err != nil {
		logger.Warn("ledger email encryption key unavailable", "error", err)
	}
	return &Handler{
		store:              s,
		logger:             logger,
		jwtSecret:          jwtSecret,
		emailClient:        emailClient,
		ledgerImport:       ledgerimport.NewService(s, logger),
		ledgerEmail:        ledgeremail.NewService(s, logger),
		emailEncryptionKey: emailEncryptionKey,
	}
}

func (h *Handler) writeJSON(w http.ResponseWriter, status int, v any) {
	httputil.WriteJSON(h.logger, w, status, v)
}

func (h *Handler) readJSON(r *http.Request, v any) error {
	return httputil.ReadJSON(r, v)
}

func (h *Handler) errorResponse(w http.ResponseWriter, status int, msg string) {
	httputil.Error(h.logger, w, status, msg)
}

func (h *Handler) handleStoreError(w http.ResponseWriter, err error) {
	if errors.Is(err, store.ErrNotFound) {
		h.errorResponse(w, http.StatusNotFound, "not found")
		return
	}
	if errors.Is(err, store.ErrConflict) || errors.Is(err, store.ErrLedgerFileImported) {
		h.errorResponse(w, http.StatusConflict, err.Error())
		return
	}
	if errors.Is(err, store.ErrLedgerCategoryHasChild) || errors.Is(err, store.ErrLedgerCategoryHasCycle) {
		h.errorResponse(w, http.StatusBadRequest, err.Error())
		return
	}
	if errors.Is(err, store.ErrLedgerTransferInvalid) {
		h.errorResponse(w, http.StatusBadRequest, err.Error())
		return
	}
	if errors.Is(err, store.ErrLedgerTransferLinked) {
		h.errorResponse(w, http.StatusConflict, err.Error())
		return
	}
	if err != nil && err.Error() == "links must contain valid absolute URLs" {
		h.errorResponse(w, http.StatusBadRequest, err.Error())
		return
	}
	if err != nil && err.Error() == "references contain an invalid type" {
		h.errorResponse(w, http.StatusBadRequest, err.Error())
		return
	}
	if err != nil && (err.Error() == "scanSince must be in YYYY-MM-DD format" || err.Error() == "transactionIds must contain at least one id") {
		h.errorResponse(w, http.StatusBadRequest, err.Error())
		return
	}
	if errors.Is(err, store.ErrLedgerPreviewExpired) {
		h.errorResponse(w, http.StatusGone, err.Error())
		return
	}
	h.logger.Error("store error", "error", err)
	h.errorResponse(w, http.StatusInternalServerError, "internal error")
}

func parseUUID(s string) (uuid.UUID, error) {
	return uuid.Parse(s)
}

func parseInt(s string) (int, error) {
	return strconv.Atoi(s)
}
