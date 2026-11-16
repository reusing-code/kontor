package purchases

import (
	"context"
	"errors"
	"time"

	"github.com/reusing-code/kontor/backend/internal/categories"
	"github.com/reusing-code/kontor/backend/internal/drafts"
	"github.com/reusing-code/kontor/backend/internal/storage"
)

// DraftCreators returns the create functions of the module's draft targets;
// they validate and store like the create endpoints.
func (m *Module) DraftCreators() map[string]drafts.CreateFunc {
	return map[string]drafts.CreateFunc{
		drafts.TargetPurchase:         m.createPurchaseFromDraft,
		drafts.TargetPurchaseCategory: categories.DraftCreator(m.categories, ModuleID),
	}
}

func (m *Module) createPurchaseFromDraft(ctx context.Context, userID string, req drafts.CreateRequest) (string, error) {
	path := "/purchases/" + req.EntityID.String()
	if _, err := m.store.Get(ctx, userID, req.EntityID); err == nil {
		return path, nil
	} else if !errors.Is(err, storage.ErrNotFound) {
		return "", err
	}
	catID, err := req.ParentUUID()
	if err != nil {
		return "", err
	}
	if _, err := m.categories.Get(ctx, userID, ModuleID, catID); err != nil {
		return "", drafts.ParentError(err)
	}
	var input PurchaseInput
	if err := drafts.DecodeInput(req.Input, &input); err != nil {
		return "", err
	}
	if err := input.Validate(); err != nil {
		return "", drafts.InvalidInput(err)
	}
	return path, m.store.Create(ctx, userID, newPurchase(req.EntityID, catID, input, time.Now().UTC()))
}
