package purchases

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/reusing-code/kontor/backend/internal/categories"
	"github.com/reusing-code/kontor/backend/internal/drafts"
)

// DraftCreators returns the create functions of the module's draft targets;
// they validate and store like the create endpoints.
func (m *Module) DraftCreators() map[string]drafts.CreateFunc {
	path := func(id uuid.UUID) string { return "/purchases/" + id.String() }
	return map[string]drafts.CreateFunc{
		drafts.TargetPurchase: drafts.NewCreateFunc(drafts.Creator[PurchaseInput]{
			Existing: func(ctx context.Context, userID string, id uuid.UUID) (string, error) {
				_, err := m.store.Get(ctx, userID, id)
				return path(id), err
			},
			CheckParent: func(ctx context.Context, userID string, catID uuid.UUID) error {
				_, err := m.categories.Get(ctx, userID, ModuleID, catID)
				return err
			},
			Create: func(ctx context.Context, userID string, id, catID uuid.UUID, input PurchaseInput) (string, error) {
				return path(id), m.store.Create(ctx, userID, newPurchase(id, catID, input, time.Now().UTC()))
			},
		}),
		drafts.TargetPurchaseCategory: categories.DraftCreator(m.categories, ModuleID),
	}
}
