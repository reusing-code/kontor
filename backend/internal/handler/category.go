package handler

import (
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/reusing-code/kontor/backend/internal/middleware"
	"github.com/reusing-code/kontor/backend/internal/model"
)

func (h *Handler) ListCategories(w http.ResponseWriter, r *http.Request) {
	module := r.PathValue("module")
	categories, err := h.store.ListCategories(r.Context(), middleware.GetUserID(r.Context()), module)
	if err != nil {
		h.handleStoreError(w, err)
		return
	}
	h.writeJSON(w, http.StatusOK, categories)
}

func (h *Handler) GetCategory(w http.ResponseWriter, r *http.Request) {
	module := r.PathValue("module")
	id, err := parseUUID(r.PathValue("id"))
	if err != nil {
		h.errorResponse(w, http.StatusBadRequest, "invalid id")
		return
	}

	cat, err := h.store.GetCategory(r.Context(), middleware.GetUserID(r.Context()), module, id)
	if err != nil {
		h.handleStoreError(w, err)
		return
	}
	h.writeJSON(w, http.StatusOK, cat)
}

func (h *Handler) CreateCategory(w http.ResponseWriter, r *http.Request) {
	module := r.PathValue("module")
	var input model.CategoryInput
	if err := h.readJSON(r, &input); err != nil {
		h.errorResponse(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := input.Validate(); err != nil {
		h.errorResponse(w, http.StatusBadRequest, err.Error())
		return
	}

	now := time.Now().UTC()
	cat := model.Category{
		ID:        uuid.New(),
		Name:      input.Name,
		CreatedAt: now,
		UpdatedAt: now,
	}

	if err := h.store.CreateCategory(r.Context(), middleware.GetUserID(r.Context()), module, cat); err != nil {
		h.handleStoreError(w, err)
		return
	}
	h.writeJSON(w, http.StatusCreated, cat)
}

func (h *Handler) UpdateCategory(w http.ResponseWriter, r *http.Request) {
	module := r.PathValue("module")
	id, err := parseUUID(r.PathValue("id"))
	if err != nil {
		h.errorResponse(w, http.StatusBadRequest, "invalid id")
		return
	}

	existing, err := h.store.GetCategory(r.Context(), middleware.GetUserID(r.Context()), module, id)
	if err != nil {
		h.handleStoreError(w, err)
		return
	}

	var input model.CategoryInput
	if err := h.readJSON(r, &input); err != nil {
		h.errorResponse(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := input.Validate(); err != nil {
		h.errorResponse(w, http.StatusBadRequest, err.Error())
		return
	}

	existing.Name = input.Name
	existing.UpdatedAt = time.Now().UTC()

	if err := h.store.UpdateCategory(r.Context(), middleware.GetUserID(r.Context()), module, existing); err != nil {
		h.handleStoreError(w, err)
		return
	}
	h.writeJSON(w, http.StatusOK, existing)
}

func (h *Handler) DeleteCategory(w http.ResponseWriter, r *http.Request) {
	module := r.PathValue("module")
	id, err := parseUUID(r.PathValue("id"))
	if err != nil {
		h.errorResponse(w, http.StatusBadRequest, "invalid id")
		return
	}

	if err := h.store.DeleteCategory(r.Context(), middleware.GetUserID(r.Context()), module, id); err != nil {
		h.handleStoreError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
