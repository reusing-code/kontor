package purchases

import (
	"context"
	"log/slog"

	"github.com/reusing-code/kontor/backend/internal/categories"
	"github.com/reusing-code/kontor/backend/internal/module"
	"github.com/reusing-code/kontor/backend/internal/storage"
	"github.com/reusing-code/kontor/backend/internal/storage/link"
)

const ModuleID = "purchases"

var defaultCategories = []categories.Default{
	{Name: "PC Hardware", NameKey: "categoryNames.pcHardware"},
	{Name: "Entertainment", NameKey: "categoryNames.entertainment"},
	{Name: "Kitchen", NameKey: "categoryNames.kitchen"},
	{Name: "Tools", NameKey: "categoryNames.tools"},
	{Name: "Household", NameKey: "categoryNames.household"},
}

type Handler struct {
	store      *Store
	categories *categories.Store
	logger     *slog.Logger
}

type Module struct {
	module.Base
	store      *Store
	categories *categories.Store
	handler    *Handler
}

func New(e *storage.Engine, links *link.Registry, catStore *categories.Store, logger *slog.Logger) *Module {
	store := NewStore(e, links)
	catStore.RegisterCascade(ModuleID, store.CategoryCascade)
	return &Module{
		store:      store,
		categories: catStore,
		handler:    &Handler{store: store, categories: catStore, logger: logger},
	}
}

func (m *Module) ID() string { return ModuleID }

func (m *Module) Store() *Store { return m.store }

func (m *Module) RegisterRoutes(r *module.Router) {
	h := m.handler
	r.Handle("GET /api/v1/categories/{id}/purchases", h.ListPurchasesByCategory)
	r.Handle("POST /api/v1/categories/{id}/purchases", h.CreatePurchaseInCategory)
	r.Handle("GET /api/v1/purchases/summary", h.PurchaseSummary)
	r.Handle("GET /api/v1/purchases", h.ListPurchases)
	r.Handle("GET /api/v1/purchases/{id}", h.GetPurchase)
	r.Handle("PUT /api/v1/purchases/{id}", h.UpdatePurchase)
	r.Handle("DELETE /api/v1/purchases/{id}", h.DeletePurchase)
}

func (m *Module) Seed(ctx context.Context, userID string) error {
	return m.categories.SeedDefaults(ctx, userID, ModuleID, defaultCategories)
}

func (m *Module) Prefix(userID string) []byte {
	return module.Prefix(userID, ModuleID)
}
