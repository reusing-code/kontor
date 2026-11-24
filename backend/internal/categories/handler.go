package categories

import (
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

func NewHandler(store *Store, logger *slog.Logger) *Handler {
	return &Handler{store: store, logger: logger}
}

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	module := r.PathValue("module")
	cats, err := h.store.List(r.Context(), middleware.GetUserID(r.Context()), module)
	if err != nil {
		httputil.StoreError(h.logger, w, err)
		return
	}
	httputil.WriteJSON(h.logger, w, http.StatusOK, cats)
}

func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	module := r.PathValue("module")
	id, ok := httputil.PathUUID(h.logger, w, r, "id", "invalid id")
	if !ok {
		return
	}

	cat, err := h.store.Get(r.Context(), middleware.GetUserID(r.Context()), module, id)
	if err != nil {
		httputil.StoreError(h.logger, w, err)
		return
	}
	httputil.WriteJSON(h.logger, w, http.StatusOK, cat)
}

func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	module := r.PathValue("module")
	input, ok := httputil.Decode[CategoryInput](h.logger, w, r)
	if !ok {
		return
	}

	cat := New(uuid.New(), input, time.Now().UTC())

	if err := h.store.Create(r.Context(), middleware.GetUserID(r.Context()), module, cat); err != nil {
		httputil.StoreError(h.logger, w, err)
		return
	}
	httputil.WriteJSON(h.logger, w, http.StatusCreated, cat)
}

func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	module := r.PathValue("module")
	id, ok := httputil.PathUUID(h.logger, w, r, "id", "invalid id")
	if !ok {
		return
	}

	existing, err := h.store.Get(r.Context(), middleware.GetUserID(r.Context()), module, id)
	if err != nil {
		httputil.StoreError(h.logger, w, err)
		return
	}

	input, ok := httputil.Decode[CategoryInput](h.logger, w, r)
	if !ok {
		return
	}

	existing.Apply(input, time.Now().UTC())

	if err := h.store.Update(r.Context(), middleware.GetUserID(r.Context()), module, existing); err != nil {
		httputil.StoreError(h.logger, w, err)
		return
	}
	httputil.WriteJSON(h.logger, w, http.StatusOK, existing)
}

func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	module := r.PathValue("module")
	id, ok := httputil.PathUUID(h.logger, w, r, "id", "invalid id")
	if !ok {
		return
	}

	if err := h.store.Delete(r.Context(), middleware.GetUserID(r.Context()), module, id); err != nil {
		httputil.StoreError(h.logger, w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
