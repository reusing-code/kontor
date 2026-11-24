package auto

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/reusing-code/kontor/backend/internal/drafts"
)

func vehiclePath(id uuid.UUID) string { return "/auto/vehicles/" + id.String() }

// DraftCreators returns the create functions of the module's draft targets;
// they validate and store like the create endpoints. A cost entry has no
// page of its own; its path is the vehicle's.
func (m *Module) DraftCreators() map[string]drafts.CreateFunc {
	return map[string]drafts.CreateFunc{
		drafts.TargetVehicle: drafts.NewCreateFunc(drafts.Creator[VehicleInput]{
			Existing: func(ctx context.Context, userID string, id uuid.UUID) (string, error) {
				_, err := m.store.GetVehicle(ctx, userID, id)
				return vehiclePath(id), err
			},
			Create: func(ctx context.Context, userID string, id, _ uuid.UUID, input VehicleInput) (string, error) {
				return vehiclePath(id), m.store.CreateVehicle(ctx, userID, newVehicle(id, input, time.Now().UTC()))
			},
		}),
		drafts.TargetCost: drafts.NewCreateFunc(drafts.Creator[CostEntryInput]{
			Existing: func(ctx context.Context, userID string, id uuid.UUID) (string, error) {
				existing, err := m.store.GetCostEntry(ctx, userID, id)
				return vehiclePath(existing.VehicleID), err
			},
			CheckParent: func(ctx context.Context, userID string, vehicleID uuid.UUID) error {
				_, err := m.store.GetVehicle(ctx, userID, vehicleID)
				return err
			},
			Create: func(ctx context.Context, userID string, id, vehicleID uuid.UUID, input CostEntryInput) (string, error) {
				return vehiclePath(vehicleID), m.store.CreateCostEntry(ctx, userID, newCostEntry(id, vehicleID, input, time.Now().UTC()))
			},
		}),
	}
}
