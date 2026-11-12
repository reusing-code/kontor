package mcptool

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/reusing-code/kontor/backend/internal/drafts"
)

const (
	MaxTitleLength = 200
	MaxKeyLength   = 200
	MaxNoteLength  = 500

	// DefaultMaxPendingDrafts caps the pending drafts a user can have from
	// MCP, like the paperless pipeline caps its own, so an agent cannot
	// flood the inbox.
	DefaultMaxPendingDrafts = 500
)

// DraftRequest is what a module's draft tool hands to CreateDraft after it
// validated the input.
type DraftRequest struct {
	Target   string // drafts.Target*
	ParentID string
	Input    json.RawMessage
	Title    string // required, becomes the source title
	Key      string // optional idempotency key
	Note     string // optional free text
}

type DraftResult struct {
	DraftID string `json:"draftId"`
	Status  string `json:"status"`
}

// CreateDraft stores a pending draft with source kind mcp. With a key the
// source ref is derived from it, so repeating the call while the draft is
// pending is reported as already_pending; without a key every call creates a
// new draft.
func (r *Registrar) CreateDraft(ctx context.Context, c Call, req DraftRequest) (DraftResult, error) {
	blankKey := req.Key != "" && strings.TrimSpace(req.Key) == ""
	req.Title = strings.TrimSpace(req.Title)
	req.Key = strings.TrimSpace(req.Key)
	if blankKey {
		// silently dropping the key would lose the idempotency the agent asked for
		return DraftResult{}, Errorf(CodeInvalidInput, "key must not be blank; omit it or give a stable value")
	}
	req.Note = strings.TrimSpace(req.Note)
	if err := CheckLen("title", req.Title, 1, MaxTitleLength); err != nil {
		return DraftResult{}, err
	}
	if err := CheckLen("key", req.Key, 0, MaxKeyLength); err != nil {
		return DraftResult{}, err
	}
	if err := CheckLen("note", req.Note, 0, MaxNoteLength); err != nil {
		return DraftResult{}, err
	}

	ref := "r:" + uuid.NewString()
	if req.Key != "" {
		sum := sha256.Sum256([]byte(req.Key))
		ref = "k:" + hex.EncodeToString(sum[:])
	}
	d, err := drafts.New(drafts.Source{Kind: drafts.SourceMCP, Ref: ref, Title: req.Title},
		req.Target, req.ParentID, req.Input, req.Note)
	if err != nil {
		return DraftResult{}, Errorf(CodeInvalidInput, "%s", err.Error())
	}
	if err := r.env.Drafts.CreateLimited(ctx, c.UserID, d, r.maxPendingDrafts()); err != nil {
		if errors.Is(err, drafts.ErrDuplicateSource) {
			return DraftResult{}, Errorf(CodeAlreadyPending, "a draft for this key is already pending")
		}
		if errors.Is(err, drafts.ErrTooManyPending) {
			return DraftResult{}, Errorf(CodeTooManyPending, "too many drafts are waiting for review; ask the user to accept or reject some first")
		}
		return DraftResult{}, err
	}
	return DraftResult{DraftID: d.ID.String(), Status: d.Status}, nil
}

// CheckLen validates a text field (length in characters).
func CheckLen(field, v string, minLen, maxLen int) error {
	n := utf8.RuneCountInString(v)
	switch {
	case n < minLen && minLen == 1:
		return Errorf(CodeInvalidInput, "%s is required", field)
	case n < minLen:
		return Errorf(CodeInvalidInput, "%s must be at least %d characters", field, minLen)
	case n > maxLen:
		return Errorf(CodeInvalidInput, "%s must be at most %d characters", field, maxLen)
	}
	return nil
}

// CheckDate validates a YYYY-MM-DD date.
func CheckDate(field, v string) error {
	if _, err := time.Parse("2006-01-02", v); err != nil {
		return Errorf(CodeInvalidInput, "%s must be a date in YYYY-MM-DD format", field)
	}
	return nil
}
