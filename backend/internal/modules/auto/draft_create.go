package auto

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
		drafts.TargetVehicle: m.createVehicleFromDraft,
		drafts.TargetCost:    m.createCostFromDraft,
	}
}

func (m *Module) createVehicleFromDraft(ctx context.Context, userID string, req drafts.CreateRequest) (string, error) {
	path := "/auto/vehicles/" + req.EntityID.String()
	if _, err := m.store.GetVehicle(ctx, userID, req.EntityID); err == nil {
		return path, nil
	} else if !errors.Is(err, storage.ErrNotFound) {
		return "", err
	}
	var input VehicleInput
	if err := drafts.DecodeInput(req.Input, &input); err != nil {
		return "", err
	}
	if err := input.Validate(); err != nil {
		return "", drafts.InvalidInput(err)
	}
	return path, m.store.CreateVehicle(ctx, userID, newVehicle(req.EntityID, input, time.Now().UTC()))
}

// A cost entry has no page of its own; its path is the vehicle's.
func (m *Module) createCostFromDraft(ctx context.Context, userID string, req drafts.CreateRequest) (string, error) {
	if existing, err := m.store.GetCostEntry(ctx, userID, req.EntityID); err == nil {
		return "/auto/vehicles/" + existing.VehicleID.String(), nil
	} else if !errors.Is(err, storage.ErrNotFound) {
		return "", err
	}
	vehicleID, err := req.ParentUUID()
	if err != nil {
		return "", err
	}
	if _, err := m.store.GetVehicle(ctx, userID, vehicleID); err != nil {
		return "", drafts.ParentError(err)
	}
	var input CostEntryInput
	if err := drafts.DecodeInput(req.Input, &input); err != nil {
		return "", err
	}
	if err := input.Validate(); err != nil {
		return "", drafts.InvalidInput(err)
	}
	path := "/auto/vehicles/" + vehicleID.String()
	return path, m.store.CreateCostEntry(ctx, userID, newCostEntry(req.EntityID, vehicleID, input, time.Now().UTC()))
}
