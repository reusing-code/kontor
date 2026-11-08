package paperless

import (
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/reusing-code/kontor/backend/internal/cryptoutil"
	"github.com/reusing-code/kontor/backend/internal/httputil"
	"github.com/reusing-code/kontor/backend/internal/middleware"
	"github.com/reusing-code/kontor/backend/internal/storage"
)

type Handler struct {
	svc    *Service
	store  *Store
	logger *slog.Logger
}

func NewHandler(svc *Service, logger *slog.Logger) *Handler {
	return &Handler{svc: svc, store: svc.Store(), logger: logger}
}

type configResponse struct {
	Configured bool   `json:"configured"`
	BaseURL    string `json:"baseUrl,omitempty"`
}

func (h *Handler) GetConfig(w http.ResponseWriter, r *http.Request) {
	cfg, err := h.store.GetConfig(r.Context(), middleware.GetUserID(r.Context()))
	if errors.Is(err, storage.ErrNotFound) {
		httputil.WriteJSON(h.logger, w, http.StatusOK, configResponse{Configured: false})
		return
	}
	if err != nil {
		httputil.StoreError(h.logger, w, err)
		return
	}
	httputil.WriteJSON(h.logger, w, http.StatusOK, configResponse{Configured: true, BaseURL: cfg.BaseURL})
}

func (h *Handler) PutConfig(w http.ResponseWriter, r *http.Request) {
	if len(h.svc.encryptionKey) == 0 {
		httputil.Error(h.logger, w, http.StatusInternalServerError, ErrNoEncryptionKey.Error())
		return
	}
	var input ConfigInput
	if err := httputil.ReadJSON(r, &input); err != nil {
		httputil.Error(h.logger, w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := input.Validate(); err != nil {
		httputil.Error(h.logger, w, http.StatusBadRequest, err.Error())
		return
	}
	userID := middleware.GetUserID(r.Context())
	cfg, err := h.store.GetConfig(r.Context(), userID)
	if err != nil && !errors.Is(err, storage.ErrNotFound) {
		httputil.StoreError(h.logger, w, err)
		return
	}
	existed := err == nil
	if !existed && input.Token == "" {
		httputil.Error(h.logger, w, http.StatusBadRequest, "token is required")
		return
	}
	if cfg.BaseURL != input.BaseURL {
		// The cached custom field ID belongs to the old instance.
		cfg.CustomFieldID = 0
	}
	cfg.BaseURL = input.BaseURL
	if input.Token != "" {
		encrypted, err := cryptoutil.EncryptString(input.Token, h.svc.encryptionKey)
		if err != nil {
			httputil.Error(h.logger, w, http.StatusInternalServerError, err.Error())
			return
		}
		cfg.EncryptedToken = encrypted
	}
	cfg.UpdatedAt = time.Now().UTC()
	if err := h.store.PutConfig(r.Context(), userID, cfg); err != nil {
		httputil.StoreError(h.logger, w, err)
		return
	}
	httputil.WriteJSON(h.logger, w, http.StatusOK, configResponse{Configured: true, BaseURL: cfg.BaseURL})
}

func (h *Handler) DeleteConfig(w http.ResponseWriter, r *http.Request) {
	if err := h.store.DeleteConfig(r.Context(), middleware.GetUserID(r.Context())); err != nil {
		httputil.StoreError(h.logger, w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// clientForUser writes the error response itself and returns false when
// the integration is unusable.
func (h *Handler) clientForUser(w http.ResponseWriter, r *http.Request) (*Client, bool) {
	client, _, err := h.svc.Client(r.Context(), middleware.GetUserID(r.Context()))
	switch {
	case errors.Is(err, ErrNotConfigured):
		httputil.Error(h.logger, w, http.StatusConflict, err.Error())
	case errors.Is(err, ErrNoEncryptionKey), errors.Is(err, ErrTokenDecrypt):
		httputil.Error(h.logger, w, http.StatusInternalServerError, err.Error())
	case err != nil:
		httputil.StoreError(h.logger, w, err)
	default:
		return client, true
	}
	return nil, false
}

func (h *Handler) TestConfig(w http.ResponseWriter, r *http.Request) {
	client, ok := h.clientForUser(w, r)
	if !ok {
		return
	}
	if err := client.Ping(r.Context()); err != nil {
		httputil.Error(h.logger, w, http.StatusBadGateway, err.Error())
		return
	}
	httputil.WriteJSON(h.logger, w, http.StatusOK, map[string]any{"ok": true})
}

func (h *Handler) Search(w http.ResponseWriter, r *http.Request) {
	client, ok := h.clientForUser(w, r)
	if !ok {
		return
	}
	page := 1
	if raw := r.URL.Query().Get("page"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 {
			httputil.Error(h.logger, w, http.StatusBadRequest, "invalid page")
			return
		}
		page = parsed
	}
	result, err := client.SearchDocuments(r.Context(), r.URL.Query().Get("query"), page)
	if err != nil {
		httputil.Error(h.logger, w, http.StatusBadGateway, err.Error())
		return
	}
	httputil.WriteJSON(h.logger, w, http.StatusOK, result)
}

func (h *Handler) Thumbnail(w http.ResponseWriter, r *http.Request) {
	documentID, err := strconv.Atoi(r.PathValue("documentId"))
	if err != nil || documentID <= 0 {
		httputil.Error(h.logger, w, http.StatusBadRequest, "invalid documentId")
		return
	}
	client, ok := h.clientForUser(w, r)
	if !ok {
		return
	}
	body, contentType, err := client.Thumbnail(r.Context(), documentID)
	if err != nil {
		httputil.Error(h.logger, w, http.StatusBadGateway, err.Error())
		return
	}
	defer body.Close()
	if contentType != "" {
		w.Header().Set("Content-Type", contentType)
	}
	w.Header().Set("Cache-Control", "private, max-age=3600")
	if _, err := io.Copy(w, body); err != nil {
		h.logger.Warn("streaming paperless thumbnail", "error", err)
	}
}

func (h *Handler) entityParams(w http.ResponseWriter, r *http.Request) (string, string, bool) {
	entityType := r.PathValue("entityType")
	if !ValidEntityType(entityType) {
		httputil.Error(h.logger, w, http.StatusBadRequest, "invalid entityType")
		return "", "", false
	}
	entityID, err := uuid.Parse(r.PathValue("entityId"))
	if err != nil {
		httputil.Error(h.logger, w, http.StatusBadRequest, "invalid entityId")
		return "", "", false
	}
	return entityType, entityID.String(), true
}

func (h *Handler) ListLinks(w http.ResponseWriter, r *http.Request) {
	entityType, entityID, ok := h.entityParams(w, r)
	if !ok {
		return
	}
	links, err := h.store.ListLinks(r.Context(), middleware.GetUserID(r.Context()), entityType, entityID)
	if err != nil {
		httputil.StoreError(h.logger, w, err)
		return
	}
	httputil.WriteJSON(h.logger, w, http.StatusOK, links)
}

type attachResponse struct {
	Links    []Link   `json:"links"`
	Warnings []string `json:"warnings"`
}

func (h *Handler) AttachLinks(w http.ResponseWriter, r *http.Request) {
	entityType, entityID, ok := h.entityParams(w, r)
	if !ok {
		return
	}
	var input AttachInput
	if err := httputil.ReadJSON(r, &input); err != nil {
		httputil.Error(h.logger, w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := input.Validate(); err != nil {
		httputil.Error(h.logger, w, http.StatusBadRequest, err.Error())
		return
	}
	if _, ok := h.clientForUser(w, r); !ok {
		return
	}
	now := time.Now().UTC()
	links := make([]Link, 0, len(input.Documents))
	for _, doc := range input.Documents {
		links = append(links, Link{
			EntityType: entityType,
			EntityID:   entityID,
			DocumentID: doc.ID,
			Title:      doc.Title,
			EntityURL:  input.EntityURL,
			CreatedAt:  now,
		})
	}
	warnings, err := h.svc.Attach(r.Context(), middleware.GetUserID(r.Context()), links, "")
	if err != nil {
		httputil.StoreError(h.logger, w, err)
		return
	}
	httputil.WriteJSON(h.logger, w, http.StatusOK, attachResponse{Links: links, Warnings: warnings})
}

func (h *Handler) DetachLink(w http.ResponseWriter, r *http.Request) {
	entityType, entityID, ok := h.entityParams(w, r)
	if !ok {
		return
	}
	documentID, err := strconv.Atoi(strings.TrimSpace(r.PathValue("documentId")))
	if err != nil || documentID <= 0 {
		httputil.Error(h.logger, w, http.StatusBadRequest, "invalid documentId")
		return
	}
	if err := h.svc.Detach(r.Context(), middleware.GetUserID(r.Context()), entityType, entityID, documentID); err != nil {
		httputil.StoreError(h.logger, w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) GetPipelineSettings(w http.ResponseWriter, r *http.Request) {
	settings, err := h.store.GetPipelineSettings(r.Context(), middleware.GetUserID(r.Context()))
	if err != nil {
		httputil.StoreError(h.logger, w, err)
		return
	}
	httputil.WriteJSON(h.logger, w, http.StatusOK, settings)
}

func (h *Handler) PutPipelineSettings(w http.ResponseWriter, r *http.Request) {
	var input PipelineSettings
	if err := httputil.ReadJSON(r, &input); err != nil {
		httputil.Error(h.logger, w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := input.Validate(); err != nil {
		httputil.Error(h.logger, w, http.StatusBadRequest, err.Error())
		return
	}
	if err := h.store.PutPipelineSettings(r.Context(), middleware.GetUserID(r.Context()), input); err != nil {
		httputil.StoreError(h.logger, w, err)
		return
	}
	httputil.WriteJSON(h.logger, w, http.StatusOK, input)
}
