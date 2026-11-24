package contracts

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
	path := func(id uuid.UUID) string { return "/contracts/" + id.String() }
	return map[string]drafts.CreateFunc{
		drafts.TargetContract: drafts.NewCreateFunc(drafts.Creator[ContractInput]{
			Existing: func(ctx context.Context, userID string, id uuid.UUID) (string, error) {
				_, err := m.store.Get(ctx, userID, id)
				return path(id), err
			},
			CheckParent: func(ctx context.Context, userID string, catID uuid.UUID) error {
				_, err := m.categories.Get(ctx, userID, ModuleID, catID)
				return err
			},
			Create: func(ctx context.Context, userID string, id, catID uuid.UUID, input ContractInput) (string, error) {
				return path(id), m.store.Create(ctx, userID, newContract(id, catID, input, time.Now().UTC()))
			},
		}),
		drafts.TargetContractCategory: categories.DraftCreator(m.categories, ModuleID),
	}
}
