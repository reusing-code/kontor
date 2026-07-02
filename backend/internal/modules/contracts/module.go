package contracts

import (
	"context"
	"log/slog"

	"github.com/reusing-code/kontor/backend/internal/categories"
	"github.com/reusing-code/kontor/backend/internal/core"
	"github.com/reusing-code/kontor/backend/internal/email"
	"github.com/reusing-code/kontor/backend/internal/module"
	"github.com/reusing-code/kontor/backend/internal/storage"
	"github.com/reusing-code/kontor/backend/internal/storage/link"
)

const ModuleID = "contracts"

var defaultCategories = []categories.Default{
	{Name: "Insurance", NameKey: "categoryNames.insurance"},
	{Name: "Banking / Portfolios", NameKey: "categoryNames.banking"},
	{Name: "Telecommunications", NameKey: "categoryNames.telecommunications"},
}

type Handler struct {
	store      *Store
	categories *categories.Store
	logger     *slog.Logger
}

type Module struct {
	store       *Store
	categories  *categories.Store
	coreStore   *core.Store
	handler     *Handler
	emailClient *email.Client
	logger      *slog.Logger
}

func New(e *storage.Engine, links *link.Registry, catStore *categories.Store, coreStore *core.Store, emailClient *email.Client, logger *slog.Logger) *Module {
	store := NewStore(e, links)
	catStore.RegisterCascade(ModuleID, store.CategoryCascade)
	return &Module{
		store:       store,
		categories:  catStore,
		coreStore:   coreStore,
		handler:     &Handler{store: store, categories: catStore, logger: logger},
		emailClient: emailClient,
		logger:      logger,
	}
}

func (m *Module) ID() string { return ModuleID }

func (m *Module) Store() *Store { return m.store }

func (m *Module) RegisterRoutes(r *module.Router) {
	h := m.handler
	r.Handle("GET /api/v1/categories/{id}/contracts", h.ListContractsByCategory)
	r.Handle("POST /api/v1/categories/{id}/contracts", h.CreateContractInCategory)
	r.Handle("POST /api/v1/contracts/import", h.ImportContracts)
	r.Handle("GET /api/v1/contracts/upcoming-renewals", h.UpcomingRenewals)
	r.Handle("GET /api/v1/contracts", h.ListContracts)
	r.Handle("GET /api/v1/contracts/{id}", h.GetContract)
	r.Handle("PUT /api/v1/contracts/{id}", h.UpdateContract)
	r.Handle("DELETE /api/v1/contracts/{id}", h.DeleteContract)
	r.Handle("GET /api/v1/summary", h.Summary)
}

func (m *Module) Seed(ctx context.Context, userID string) error {
	return m.categories.SeedDefaults(ctx, userID, ModuleID, defaultCategories)
}

func (m *Module) StartBackground(ctx context.Context) {
	if !m.emailClient.IsConfigured() {
		m.logger.Info("SMTP not configured, reminder scheduler disabled")
		return
	}
	NewScheduler(m.coreStore, m.store, m.emailClient, m.logger).Start(ctx)
}
