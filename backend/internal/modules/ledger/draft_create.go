package ledger

import (
	"context"
	"errors"
	"time"

	"github.com/reusing-code/kontor/backend/internal/drafts"
	"github.com/reusing-code/kontor/backend/internal/storage"
)

// DraftCreators returns the create functions of the module's draft targets;
// they validate and store like the create endpoints.
func (m *Module) DraftCreators() map[string]drafts.CreateFunc {
	return map[string]drafts.CreateFunc{
		drafts.TargetLedgerCategory: m.createCategoryFromDraft,
		drafts.TargetLedgerAccount:  m.createAccountFromDraft,
	}
}

// The parent of a ledger category draft is optional and comes from the
// draft's parentId, not from its input.
func (m *Module) createCategoryFromDraft(ctx context.Context, userID string, req drafts.CreateRequest) (string, error) {
	const path = "/ledger/categories"
	if _, err := m.store.GetLedgerCategory(ctx, userID, req.EntityID); err == nil {
		return path, nil
	} else if !errors.Is(err, storage.ErrNotFound) {
		return "", err
	}
	var input LedgerCategoryInput
	if err := drafts.DecodeInput(req.Input, &input); err != nil {
		return "", err
	}
	if req.ParentID != "" {
		parentID, err := req.ParentUUID()
		if err != nil {
			return "", err
		}
		if _, err := m.store.GetLedgerCategory(ctx, userID, parentID); err != nil {
			return "", drafts.ParentError(err)
		}
		input.ParentID = &parentID
	}
	if err := input.Validate(); err != nil {
		return "", drafts.InvalidInput(err)
	}
	return path, m.store.CreateLedgerCategory(ctx, userID, newLedgerCategory(req.EntityID, input, time.Now().UTC()))
}

func (m *Module) createAccountFromDraft(ctx context.Context, userID string, req drafts.CreateRequest) (string, error) {
	path := "/ledger/accounts/" + req.EntityID.String()
	if _, err := m.store.GetLedgerAccount(ctx, userID, req.EntityID); err == nil {
		return path, nil
	} else if !errors.Is(err, storage.ErrNotFound) {
		return "", err
	}
	var input LedgerAccountInput
	if err := drafts.DecodeInput(req.Input, &input); err != nil {
		return "", err
	}
	if err := input.ValidateForCreate(); err != nil {
		return "", drafts.InvalidInput(err)
	}
	err := m.store.CreateLedgerAccount(ctx, userID, newLedgerAccount(req.EntityID, input, time.Now().UTC()))
	if errors.Is(err, storage.ErrConflict) {
		return "", &drafts.CreateError{Code: drafts.CodeConflict, Message: "an account with this IBAN exists"}
	}
	return path, err
}
