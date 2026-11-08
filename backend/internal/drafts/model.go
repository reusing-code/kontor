// Package drafts is the source-agnostic draft inbox: proposed entities that
// the user accepts (after creating the entity in the UI) or rejects. Sources
// such as the paperless pipeline create drafts and register hooks that run on
// a decision; the package never imports modules or sources.
package drafts

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	StatusPending  = "pending"
	StatusAccepted = "accepted"
	StatusRejected = "rejected"
)

const (
	TargetPurchase = "purchase"
	TargetContract = "contract"
	TargetCost     = "cost"
	TargetNone     = "none"
)

const (
	SourcePaperless = "paperless"
	SourceEmail     = "email"
	SourceText      = "text"
	SourceMCP       = "mcp"
)

func ValidTarget(t string) bool {
	switch t {
	case TargetPurchase, TargetContract, TargetCost, TargetNone:
		return true
	}
	return false
}

func validSource(kind string) bool {
	switch kind {
	case SourcePaperless, SourceEmail, SourceText, SourceMCP:
		return true
	}
	return false
}

// Source identifies where a draft came from. Ref is unique per kind and
// user (e.g. the paperless document ID) and dedupes drafts.
type Source struct {
	Kind  string `json:"kind"`
	Ref   string `json:"ref"`
	Title string `json:"title,omitempty"`
	URL   string `json:"url,omitempty"`
}

type Result struct {
	EntityType string `json:"entityType"`
	EntityID   string `json:"entityId"`
}

// Draft is a proposed entity. Input has the shape of the target module's
// create input; ParentID is the category (purchase, contract) or vehicle
// (cost) the entity belongs to. Note is a short machine-readable code that
// the UI translates (e.g. "unclassified").
type Draft struct {
	ID         uuid.UUID       `json:"id"`
	Status     string          `json:"status"`
	TargetType string          `json:"targetType"`
	ParentID   string          `json:"parentId,omitempty"`
	Input      json.RawMessage `json:"input"`
	Source     Source          `json:"source"`
	Note       string          `json:"note,omitempty"`
	Result     *Result         `json:"result,omitempty"`
	CreatedAt  time.Time       `json:"createdAt"`
	UpdatedAt  time.Time       `json:"updatedAt"`
	DecidedAt  *time.Time      `json:"decidedAt,omitempty"`
}

// New builds a pending draft; callers fill the content fields.
func New(source Source, targetType, parentID string, input json.RawMessage, note string) (Draft, error) {
	now := time.Now().UTC()
	d := Draft{
		ID:         uuid.New(),
		Status:     StatusPending,
		TargetType: targetType,
		ParentID:   parentID,
		Input:      input,
		Source:     source,
		Note:       note,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	if !validSource(source.Kind) || strings.TrimSpace(source.Ref) == "" {
		return Draft{}, errors.New("draft source needs a valid kind and a ref")
	}
	return d, normalizeContent(&d.TargetType, &d.ParentID, &d.Input)
}

func normalizeContent(targetType, parentID *string, input *json.RawMessage) error {
	if !ValidTarget(*targetType) {
		return errors.New("targetType must be purchase, contract, cost or none")
	}
	*parentID = strings.TrimSpace(*parentID)
	if *parentID != "" {
		id, err := uuid.Parse(*parentID)
		if err != nil {
			return errors.New("parentId must be a UUID")
		}
		*parentID = id.String()
	}
	trimmed := bytes.TrimSpace(*input)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		*input = json.RawMessage("{}")
		return nil
	}
	if trimmed[0] != '{' || !json.Valid(trimmed) {
		return errors.New("input must be a JSON object")
	}
	*input = json.RawMessage(trimmed)
	return nil
}

type UpdateInput struct {
	TargetType string          `json:"targetType"`
	ParentID   string          `json:"parentId"`
	Input      json.RawMessage `json:"input"`
}

func (in *UpdateInput) Validate() error {
	return normalizeContent(&in.TargetType, &in.ParentID, &in.Input)
}

// AcceptInput reports the entity the frontend created from the draft.
type AcceptInput struct {
	EntityType string `json:"entityType"`
	EntityID   string `json:"entityId"`
	EntityURL  string `json:"entityUrl"`
}

func (in *AcceptInput) Validate(d Draft) error {
	if in.EntityType == TargetNone || !ValidTarget(in.EntityType) {
		return errors.New("entityType must be purchase, contract or cost")
	}
	if in.EntityType != d.TargetType {
		return errors.New("entityType must match the draft's targetType")
	}
	id, err := uuid.Parse(in.EntityID)
	if err != nil {
		return errors.New("entityId must be a UUID")
	}
	in.EntityID = id.String()
	in.EntityURL = strings.TrimSpace(in.EntityURL)
	parsed, err := url.ParseRequestURI(in.EntityURL)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return errors.New("entityUrl must be a valid absolute http(s) URL")
	}
	return nil
}
