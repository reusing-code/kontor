package auto

import (
	"context"
	"log/slog"

	"github.com/google/uuid"
	"github.com/reusing-code/kontor/backend/internal/module"
	"github.com/reusing-code/kontor/backend/internal/storage"
	"github.com/reusing-code/kontor/backend/internal/storage/link"
)

const ModuleID = "auto"

type Handler struct {
	store  *Store
	logger *slog.Logger
}

type Module struct {
	module.Base
	store   *Store
	handler *Handler
}

func New(e *storage.Engine, links *link.Registry, logger *slog.Logger) *Module {
	store := NewStore(e, links)
	return &Module{
		store:   store,
		handler: &Handler{store: store, logger: logger},
	}
}

func (m *Module) ID() string { return ModuleID }

func (m *Module) Store() *Store { return m.store }

func (m *Module) RegisterRoutes(r *module.Router) {
	h := m.handler
	r.Handle("GET /api/v1/vehicles", h.ListVehicles)
	r.Handle("POST /api/v1/vehicles", h.CreateVehicle)
	r.Handle("GET /api/v1/vehicles/{id}", h.GetVehicle)
	r.Handle("PUT /api/v1/vehicles/{id}", h.UpdateVehicle)
	r.Handle("DELETE /api/v1/vehicles/{id}", h.DeleteVehicle)
	r.Handle("GET /api/v1/vehicles/{id}/summary", h.VehicleSummary)
	r.Handle("GET /api/v1/vehicles/{id}/costs", h.ListCostEntries)
	r.Handle("POST /api/v1/vehicles/{id}/costs", h.CreateCostEntry)
	r.Handle("GET /api/v1/costs/{id}", h.GetCostEntry)
	r.Handle("PUT /api/v1/costs/{id}", h.UpdateCostEntry)
	r.Handle("DELETE /api/v1/costs/{id}", h.DeleteCostEntry)
}

func (m *Module) Prefix(userID string) []byte {
	return module.Prefix(userID, ModuleID)
}

// LocateEntity implements module.EntityLocator for vehicles and cost
// entries; a cost entry is shown on its vehicle's page.
func (m *Module) LocateEntity(ctx context.Context, userID, entityType string, id uuid.UUID) (string, bool, error) {
	switch entityType {
	case "vehicle":
		if _, err := m.store.GetVehicle(ctx, userID, id); err != nil {
			return "", true, err
		}
		return "/auto/vehicles/" + id.String(), true, nil
	case "cost":
		cost, err := m.store.GetCostEntry(ctx, userID, id)
		if err != nil {
			return "", true, err
		}
		return "/auto/vehicles/" + cost.VehicleID.String(), true, nil
	}
	return "", false, nil
}
