package handler

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/google/uuid"
	"github.com/tobi/contracts/backend/internal/email"
	"github.com/tobi/contracts/backend/internal/ledgerimport"
	"github.com/tobi/contracts/backend/internal/store"
)

type Handler struct {
	store        store.Store
	logger       *slog.Logger
	jwtSecret    []byte
	emailClient  *email.Client
	ledgerImport *ledgerimport.Service
}

func New(s store.Store, logger *slog.Logger, jwtSecret []byte, emailClient *email.Client) *Handler {
	return &Handler{
		store:        s,
		logger:       logger,
		jwtSecret:    jwtSecret,
		emailClient:  emailClient,
		ledgerImport: ledgerimport.NewService(s, logger),
	}
}

func (h *Handler) writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		h.logger.Error("encoding response", "error", err)
	}
}

func (h *Handler) readJSON(r *http.Request, v any) error {
	defer r.Body.Close()
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	return dec.Decode(v)
}

func (h *Handler) errorResponse(w http.ResponseWriter, status int, msg string) {
	h.writeJSON(w, status, map[string]string{"error": msg})
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
