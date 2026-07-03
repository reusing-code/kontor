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
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		httputil.Error(h.logger, w, http.StatusBadRequest, "invalid id")
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
	var input CategoryInput
	if err := httputil.ReadJSON(r, &input); err != nil {
		httputil.Error(h.logger, w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := input.Validate(); err != nil {
		httputil.Error(h.logger, w, http.StatusBadRequest, err.Error())
		return
	}

	now := time.Now().UTC()
	cat := Category{
		ID:        uuid.New(),
		Name:      input.Name,
		CreatedAt: now,
		UpdatedAt: now,
	}

	if err := h.store.Create(r.Context(), middleware.GetUserID(r.Context()), module, cat); err != nil {
		httputil.StoreError(h.logger, w, err)
		return
	}
	httputil.WriteJSON(h.logger, w, http.StatusCreated, cat)
}

func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	module := r.PathValue("module")
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		httputil.Error(h.logger, w, http.StatusBadRequest, "invalid id")
		return
	}

	existing, err := h.store.Get(r.Context(), middleware.GetUserID(r.Context()), module, id)
	if err != nil {
		httputil.StoreError(h.logger, w, err)
		return
	}

	var input CategoryInput
	if err := httputil.ReadJSON(r, &input); err != nil {
		httputil.Error(h.logger, w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := input.Validate(); err != nil {
		httputil.Error(h.logger, w, http.StatusBadRequest, err.Error())
		return
	}

	existing.Name = input.Name
	existing.UpdatedAt = time.Now().UTC()

	if err := h.store.Update(r.Context(), middleware.GetUserID(r.Context()), module, existing); err != nil {
		httputil.StoreError(h.logger, w, err)
		return
	}
	httputil.WriteJSON(h.logger, w, http.StatusOK, existing)
}

func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	module := r.PathValue("module")
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		httputil.Error(h.logger, w, http.StatusBadRequest, "invalid id")
		return
	}

	if err := h.store.Delete(r.Context(), middleware.GetUserID(r.Context()), module, id); err != nil {
		httputil.StoreError(h.logger, w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
