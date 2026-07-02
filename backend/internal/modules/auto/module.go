package auto

import (
	"log/slog"

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
