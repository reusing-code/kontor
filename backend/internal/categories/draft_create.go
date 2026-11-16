package categories

import (
	"context"
	"errors"
	"time"

	"github.com/reusing-code/kontor/backend/internal/drafts"
	"github.com/reusing-code/kontor/backend/internal/storage"
)

// DraftCreator returns the create function for category drafts of a module;
// it validates and stores like the create endpoint.
func DraftCreator(store *Store, module string) drafts.CreateFunc {
	return func(ctx context.Context, userID string, req drafts.CreateRequest) (string, error) {
		path := "/" + module + "/categories/" + req.EntityID.String()
		if _, err := store.Get(ctx, userID, module, req.EntityID); err == nil {
			return path, nil
		} else if !errors.Is(err, storage.ErrNotFound) {
			return "", err
		}
		var input CategoryInput
		if err := drafts.DecodeInput(req.Input, &input); err != nil {
			return "", err
		}
		if err := input.Validate(); err != nil {
			return "", drafts.InvalidInput(err)
		}
		return path, store.Create(ctx, userID, module, New(req.EntityID, input, time.Now().UTC()))
	}
}
