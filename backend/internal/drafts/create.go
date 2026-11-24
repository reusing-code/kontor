package drafts

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/reusing-code/kontor/backend/internal/storage"
)

// Creator is how a module turns a draft into an entity; NewCreateFunc adds
// the steps every target shares.
type Creator[In any] struct {
	// Existing returns the UI path of the entity with this ID, or
	// storage.ErrNotFound when there is none.
	Existing func(ctx context.Context, userID string, id uuid.UUID) (string, error)
	// CheckParent verifies the draft's parent (category or vehicle); nil for
	// targets without one.
	CheckParent func(ctx context.Context, userID string, parentID uuid.UUID) error
	// Create stores the entity built from the validated input and returns its
	// path; parentID is uuid.Nil without CheckParent.
	Create func(ctx context.Context, userID string, id, parentID uuid.UUID, input In) (string, error)
}

// NewCreateFunc returns the CreateFunc of a target: an existing entity is
// returned as it is; otherwise the parent is checked, the input decoded
// strictly (DecodeInput), validated like the create endpoint and created.
func NewCreateFunc[In any, PIn interface {
	*In
	Validate() error
}](c Creator[In]) CreateFunc {
	return func(ctx context.Context, userID string, req CreateRequest) (string, error) {
		if path, err := c.Existing(ctx, userID, req.EntityID); err == nil {
			return path, nil
		} else if !errors.Is(err, storage.ErrNotFound) {
			return "", err
		}
		var parentID uuid.UUID
		if c.CheckParent != nil {
			id, err := req.ParentUUID()
			if err != nil {
				return "", err
			}
			if err := c.CheckParent(ctx, userID, id); err != nil {
				return "", ParentError(err)
			}
			parentID = id
		}
		var input In
		if err := DecodeInput(req.Input, &input); err != nil {
			return "", err
		}
		if err := PIn(&input).Validate(); err != nil {
			return "", InvalidInput(err)
		}
		return c.Create(ctx, userID, req.EntityID, parentID, input)
	}
}
