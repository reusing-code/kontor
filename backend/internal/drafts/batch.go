package drafts

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/google/uuid"
	"github.com/reusing-code/kontor/backend/internal/httputil"
	"github.com/reusing-code/kontor/backend/internal/middleware"
	"github.com/reusing-code/kontor/backend/internal/storage"
)

// MaxBatchSize is the number of drafts one batch accept may carry.
const MaxBatchSize = 100

const (
	BatchAccepted        = "accepted"
	BatchAlreadyAccepted = "already_accepted"
	BatchFailed          = "failed"
)

// Failure codes of a batch accept result.
const (
	CodeNotFound         = "not_found"
	CodeRejected         = "rejected"
	CodeNoType           = "no_type"
	CodeModuleDisabled   = "module_disabled"
	CodeParentMissing    = "parent_missing"
	CodeParentNotFound   = "parent_not_found"
	CodeInvalidInput     = "invalid_input"
	CodeConflict         = "conflict"
	CodeSourceLinkFailed = "source_link_failed"
	CodeInternalError    = "internal_error"
)

// entityNamespace derives entity IDs from draft IDs; changing it would let a
// repeated batch create a second entity for a draft.
var entityNamespace = uuid.MustParse("ff7ebf04-f6ba-4c93-8824-902bd9267149")

// EntityID is the ID of the entity a batch accept creates for a draft. It is
// derived from the draft ID, so repeating an accept whose later steps failed
// finds the entity instead of creating another one.
func EntityID(draftID uuid.UUID) uuid.UUID {
	return uuid.NewSHA1(entityNamespace, draftID[:])
}

// CreateRequest is what a draft hands to its target's create function.
type CreateRequest struct {
	EntityID uuid.UUID
	// ParentID is the draft's parentId; empty when the draft has none.
	ParentID string
	Input    json.RawMessage
}

// CreateFunc turns a draft into an entity with the module's create code and
// returns the entity's path in the UI. It must not create anything when an
// entity with EntityID exists. Failures the user can fix are a *CreateError.
type CreateFunc func(ctx context.Context, userID string, req CreateRequest) (path string, err error)

// EnabledFunc reports whether a user has a module enabled.
type EnabledFunc func(ctx context.Context, userID, moduleID string) (bool, error)

// CreateError is a create failure that is reported to the user as it is.
type CreateError struct {
	Code    string
	Message string
}

func (e *CreateError) Error() string { return e.Code + ": " + e.Message }

// InvalidInput reports a validation error of the module's create input.
func InvalidInput(err error) error {
	return &CreateError{Code: CodeInvalidInput, Message: err.Error()}
}

// DecodeInput decodes a draft input like the create endpoints decode their
// body: unknown fields are an error.
func DecodeInput(raw json.RawMessage, v any) error {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return &CreateError{Code: CodeInvalidInput, Message: "the draft input does not fit this type: " + err.Error()}
	}
	return nil
}

// ParentUUID parses the parent a create function needs.
func (r CreateRequest) ParentUUID() (uuid.UUID, error) {
	if r.ParentID == "" {
		return uuid.UUID{}, &CreateError{Code: CodeParentMissing, Message: "the draft has no category or vehicle"}
	}
	id, err := uuid.Parse(r.ParentID)
	if err != nil {
		return uuid.UUID{}, &CreateError{Code: CodeParentNotFound, Message: "the draft's category or vehicle does not exist"}
	}
	return id, nil
}

// ParentError maps the lookup error of a draft's parent.
func ParentError(err error) error {
	if errors.Is(err, storage.ErrNotFound) {
		return &CreateError{Code: CodeParentNotFound, Message: "the draft's category or vehicle does not exist"}
	}
	return err
}

type target struct {
	moduleID string
	create   CreateFunc
}

// RegisterTarget attaches the create function of a target type and the
// module that must be enabled for it.
func (h *Handler) RegisterTarget(targetType, moduleID string, create CreateFunc) {
	h.targets[targetType] = target{moduleID: moduleID, create: create}
}

// SetEnabled sets the module check of the batch accept; without one every
// module counts as enabled.
func (h *Handler) SetEnabled(enabled EnabledFunc) {
	h.enabled = enabled
}

type AcceptBatchInput struct {
	IDs     []string `json:"ids"`
	BaseURL string   `json:"baseUrl"`

	ids []uuid.UUID
}

func (in *AcceptBatchInput) Validate() error {
	if len(in.IDs) == 0 {
		return errors.New("ids must not be empty")
	}
	if len(in.IDs) > MaxBatchSize {
		return fmt.Errorf("at most %d drafts per request", MaxBatchSize)
	}
	seen := make(map[uuid.UUID]struct{}, len(in.IDs))
	in.ids = make([]uuid.UUID, 0, len(in.IDs))
	for _, raw := range in.IDs {
		id, err := uuid.Parse(raw)
		if err != nil {
			return errors.New("ids must be UUIDs")
		}
		if _, dup := seen[id]; dup {
			return errors.New("ids must not contain duplicates")
		}
		seen[id] = struct{}{}
		in.ids = append(in.ids, id)
	}
	in.BaseURL = strings.TrimRight(strings.TrimSpace(in.BaseURL), "/")
	parsed, err := url.Parse(in.BaseURL)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") ||
		parsed.Path != "" || parsed.RawQuery != "" || parsed.Fragment != "" || parsed.User != nil {
		return errors.New("baseUrl must be an http(s) origin such as https://kontor.example")
	}
	return nil
}

// BatchResult is the outcome of one draft of a batch accept.
type BatchResult struct {
	ID         string   `json:"id"`
	Status     string   `json:"status"`
	Code       string   `json:"code,omitempty"`
	Error      string   `json:"error,omitempty"`
	EntityType string   `json:"entityType,omitempty"`
	EntityID   string   `json:"entityId,omitempty"`
	Warnings   []string `json:"warnings,omitempty"`
}

type acceptBatchResponse struct {
	Results []BatchResult `json:"results"`
}

func failed(id uuid.UUID, code, msg string) BatchResult {
	return BatchResult{ID: id.String(), Status: BatchFailed, Code: code, Error: msg}
}

func decided(d Draft) BatchResult {
	if d.Status == StatusRejected {
		return failed(d.ID, CodeRejected, "the draft was rejected")
	}
	res := BatchResult{ID: d.ID.String(), Status: BatchAlreadyAccepted}
	if d.Result != nil {
		res.EntityType = d.Result.EntityType
		res.EntityID = d.Result.EntityID
	}
	return res
}

// AcceptBatch creates the entities of several pending drafts and accepts
// them. Every draft is handled on its own; the response carries one result
// per draft in request order. When the request ends early the remaining
// drafts are left out and stay pending.
func (h *Handler) AcceptBatch(w http.ResponseWriter, r *http.Request) {
	var input AcceptBatchInput
	if err := httputil.ReadJSON(r, &input); err != nil {
		httputil.BodyError(h.logger, w, err, "invalid request body")
		return
	}
	if err := input.Validate(); err != nil {
		httputil.Error(h.logger, w, http.StatusBadRequest, err.Error())
		return
	}
	ctx := r.Context()
	userID := middleware.GetUserID(ctx)
	results := make([]BatchResult, 0, len(input.ids))
	for _, id := range input.ids {
		if ctx.Err() != nil {
			break
		}
		results = append(results, h.acceptOne(ctx, userID, id, input.BaseURL))
	}
	httputil.WriteJSON(h.logger, w, http.StatusOK, acceptBatchResponse{Results: results})
}

func (h *Handler) internalError(id uuid.UUID, step string, err error) BatchResult {
	h.logger.Error("batch accept failed", "draftId", id, "step", step, "error", err)
	return failed(id, CodeInternalError, "internal error")
}

func (h *Handler) acceptOne(ctx context.Context, userID string, id uuid.UUID, baseURL string) BatchResult {
	d, err := h.store.Get(ctx, userID, id)
	if errors.Is(err, storage.ErrNotFound) {
		return failed(id, CodeNotFound, "the draft does not exist")
	} else if err != nil {
		return h.internalError(id, "load", err)
	}
	if d.Status != StatusPending {
		return decided(d)
	}
	if d.TargetType == TargetNone {
		return failed(id, CodeNoType, "the draft has no type")
	}
	t, ok := h.targets[d.TargetType]
	if !ok {
		return h.internalError(id, "target", fmt.Errorf("no create function for %q", d.TargetType))
	}
	if h.enabled != nil {
		on, err := h.enabled(ctx, userID, t.moduleID)
		if err != nil {
			return h.internalError(id, "module", err)
		}
		if !on {
			return failed(id, CodeModuleDisabled, "the "+t.moduleID+" module is disabled")
		}
	}

	entityID := EntityID(d.ID)
	path, err := t.create(ctx, userID, CreateRequest{EntityID: entityID, ParentID: d.ParentID, Input: d.Input})
	var createErr *CreateError
	switch {
	case errors.As(err, &createErr):
		return failed(id, createErr.Code, createErr.Message)
	case errors.Is(err, storage.ErrConflict):
		return failed(id, CodeConflict, err.Error())
	case err != nil:
		return h.internalError(id, "create", err)
	}

	in := AcceptInput{EntityType: d.TargetType, EntityID: entityID.String(), EntityURL: baseURL + path}
	accepted, warnings, err := h.accept(ctx, userID, d, in)
	switch {
	case errors.Is(err, errSourceHook):
		return failed(id, CodeSourceLinkFailed, "could not link the draft source to the entity")
	case errors.Is(err, ErrNotPending):
		// decided by a parallel request after the load above
		if current, getErr := h.store.Get(ctx, userID, id); getErr == nil {
			return decided(current)
		}
		return h.internalError(id, "decide", err)
	case err != nil:
		return h.internalError(id, "decide", err)
	}
	return BatchResult{
		ID:         id.String(),
		Status:     BatchAccepted,
		EntityType: accepted.Result.EntityType,
		EntityID:   accepted.Result.EntityID,
		Warnings:   warnings,
	}
}
