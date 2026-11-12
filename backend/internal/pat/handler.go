package pat

import (
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/reusing-code/kontor/backend/internal/httputil"
	"github.com/reusing-code/kontor/backend/internal/middleware"
)

type Handler struct {
	store  *Store
	logger *slog.Logger
}

func NewHandler(s *Store, l *slog.Logger) *Handler {
	return &Handler{store: s, logger: l}
}

type createResponse struct {
	Token
	Raw string `json:"token"`
}

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	tokens, err := h.store.List(r.Context(), middleware.GetUserID(r.Context()))
	if err != nil {
		httputil.StoreError(h.logger, w, err)
		return
	}
	httputil.WriteJSON(h.logger, w, http.StatusOK, tokens)
}

func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	var in CreateInput
	if err := httputil.ReadJSON(r, &in); err != nil {
		httputil.Error(h.logger, w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := in.Validate(time.Now()); err != nil {
		httputil.Error(h.logger, w, http.StatusBadRequest, err.Error())
		return
	}
	tok, raw, err := h.store.Create(r.Context(), middleware.GetUserID(r.Context()), in)
	if err != nil {
		if errors.Is(err, ErrLimitReached) {
			httputil.Error(h.logger, w, http.StatusConflict, "token limit reached")
			return
		}
		httputil.StoreError(h.logger, w, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	httputil.WriteJSON(h.logger, w, http.StatusCreated, createResponse{Token: tok, Raw: raw})
}

func (h *Handler) Revoke(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		httputil.Error(h.logger, w, http.StatusBadRequest, "invalid id")
		return
	}
	if err := h.store.Revoke(r.Context(), middleware.GetUserID(r.Context()), id); err != nil {
		httputil.StoreError(h.logger, w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
