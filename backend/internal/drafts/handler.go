package drafts

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/google/uuid"
	"github.com/reusing-code/kontor/backend/internal/httputil"
	"github.com/reusing-code/kontor/backend/internal/middleware"
)

// SourceHooks run source-specific steps when a draft is decided. OnAccept
// runs before the draft is marked accepted: an error aborts the accept and
// leaves the draft pending; warnings report best-effort steps that failed.
type SourceHooks interface {
	OnAccept(ctx context.Context, userID string, d Draft, in AcceptInput) (warnings []string, err error)
	OnReject(ctx context.Context, userID string, d Draft) (warnings []string)
}

type Handler struct {
	store  *Store
	logger *slog.Logger
	hooks  map[string]SourceHooks
	// batch accept: create function per target type and the module check
	targets map[string]target
	enabled EnabledFunc
}

func NewHandler(store *Store, logger *slog.Logger) *Handler {
	return &Handler{store: store, logger: logger, hooks: map[string]SourceHooks{}, targets: map[string]target{}}
}

// RegisterSource attaches decision hooks for a source kind.
func (h *Handler) RegisterSource(kind string, hooks SourceHooks) {
	h.hooks[kind] = hooks
}

type decisionResponse struct {
	Draft    Draft    `json:"draft"`
	Warnings []string `json:"warnings"`
}

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	status := r.URL.Query().Get("status")
	switch status {
	case "":
		status = StatusPending
	case "all":
		status = ""
	case StatusPending, StatusAccepted, StatusRejected:
	default:
		httputil.Error(h.logger, w, http.StatusBadRequest, "invalid status")
		return
	}
	drafts, err := h.store.List(r.Context(), middleware.GetUserID(r.Context()), status)
	if err != nil {
		httputil.StoreError(h.logger, w, err)
		return
	}
	httputil.WriteJSON(h.logger, w, http.StatusOK, drafts)
}

func (h *Handler) Summary(w http.ResponseWriter, r *http.Request) {
	pending, total, err := h.store.Counts(r.Context(), middleware.GetUserID(r.Context()))
	if err != nil {
		httputil.StoreError(h.logger, w, err)
		return
	}
	httputil.WriteJSON(h.logger, w, http.StatusOK, map[string]int{"pending": pending, "total": total})
}

func (h *Handler) draftID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		httputil.Error(h.logger, w, http.StatusBadRequest, "invalid id")
		return uuid.UUID{}, false
	}
	return id, true
}

func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	id, ok := h.draftID(w, r)
	if !ok {
		return
	}
	d, err := h.store.Get(r.Context(), middleware.GetUserID(r.Context()), id)
	if err != nil {
		httputil.StoreError(h.logger, w, err)
		return
	}
	httputil.WriteJSON(h.logger, w, http.StatusOK, d)
}

func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	id, ok := h.draftID(w, r)
	if !ok {
		return
	}
	var input UpdateInput
	if err := httputil.ReadJSON(r, &input); err != nil {
		httputil.Error(h.logger, w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := input.Validate(); err != nil {
		httputil.Error(h.logger, w, http.StatusBadRequest, err.Error())
		return
	}
	d, err := h.store.Update(r.Context(), middleware.GetUserID(r.Context()), id, func(d *Draft) {
		d.TargetType = input.TargetType
		d.ParentID = input.ParentID
		d.Input = input.Input
	})
	if err != nil {
		httputil.StoreError(h.logger, w, err)
		return
	}
	httputil.WriteJSON(h.logger, w, http.StatusOK, d)
}

func (h *Handler) pending(w http.ResponseWriter, r *http.Request) (Draft, bool) {
	id, ok := h.draftID(w, r)
	if !ok {
		return Draft{}, false
	}
	d, err := h.store.Get(r.Context(), middleware.GetUserID(r.Context()), id)
	if err != nil {
		httputil.StoreError(h.logger, w, err)
		return Draft{}, false
	}
	if d.Status != StatusPending {
		httputil.StoreError(h.logger, w, ErrNotPending)
		return Draft{}, false
	}
	return d, true
}

func (h *Handler) Accept(w http.ResponseWriter, r *http.Request) {
	d, ok := h.pending(w, r)
	if !ok {
		return
	}
	var input AcceptInput
	if err := httputil.ReadJSON(r, &input); err != nil {
		httputil.Error(h.logger, w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := input.Validate(d); err != nil {
		httputil.Error(h.logger, w, http.StatusBadRequest, err.Error())
		return
	}
	decided, warnings, err := h.accept(r.Context(), middleware.GetUserID(r.Context()), d, input)
	if errors.Is(err, errSourceHook) {
		httputil.Error(h.logger, w, http.StatusInternalServerError, "could not link the draft source to the entity")
		return
	}
	if err != nil {
		httputil.StoreError(h.logger, w, err)
		return
	}
	httputil.WriteJSON(h.logger, w, http.StatusOK, decisionResponse{Draft: decided, Warnings: warnings})
}

var errSourceHook = errors.New("draft accept hook failed")

// accept runs the source's accept hook and marks the draft accepted with the
// entity. A hook error is logged and returned as errSourceHook; the draft
// stays pending.
func (h *Handler) accept(ctx context.Context, userID string, d Draft, in AcceptInput) (Draft, []string, error) {
	warnings := []string{}
	if hooks, ok := h.hooks[d.Source.Kind]; ok {
		w, err := hooks.OnAccept(ctx, userID, d, in)
		if err != nil {
			h.logger.Error("draft accept hook failed", "draftId", d.ID, "source", d.Source.Kind, "error", err)
			return Draft{}, nil, errSourceHook
		}
		warnings = append(warnings, w...)
	}
	decided, err := h.store.Decide(ctx, userID, d.ID, StatusAccepted, &Result{EntityType: in.EntityType, EntityID: in.EntityID})
	return decided, warnings, err
}

func (h *Handler) Reject(w http.ResponseWriter, r *http.Request) {
	d, ok := h.pending(w, r)
	if !ok {
		return
	}
	userID := middleware.GetUserID(r.Context())
	decided, err := h.store.Decide(r.Context(), userID, d.ID, StatusRejected, nil)
	if err != nil {
		httputil.StoreError(h.logger, w, err)
		return
	}
	warnings := []string{}
	if hooks, ok := h.hooks[d.Source.Kind]; ok {
		warnings = append(warnings, hooks.OnReject(r.Context(), userID, decided)...)
	}
	httputil.WriteJSON(h.logger, w, http.StatusOK, decisionResponse{Draft: decided, Warnings: warnings})
}
