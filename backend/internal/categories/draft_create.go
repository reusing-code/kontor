package categories

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/reusing-code/kontor/backend/internal/drafts"
)

// DraftCreator returns the create function for category drafts of a module;
// it validates and stores like the create endpoint.
func DraftCreator(store *Store, module string) drafts.CreateFunc {
	path := func(id uuid.UUID) string { return "/" + module + "/categories/" + id.String() }
	return drafts.NewCreateFunc(drafts.Creator[CategoryInput]{
		Existing: func(ctx context.Context, userID string, id uuid.UUID) (string, error) {
			_, err := store.Get(ctx, userID, module, id)
			return path(id), err
		},
		Create: func(ctx context.Context, userID string, id, _ uuid.UUID, input CategoryInput) (string, error) {
			return path(id), store.Create(ctx, userID, module, New(id, input, time.Now().UTC()))
		},
	})
}
