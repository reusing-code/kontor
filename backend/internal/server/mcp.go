package server

import (
	"context"
	"fmt"
	"net/http"
	"slices"

	"github.com/google/uuid"
	"github.com/reusing-code/kontor/backend/internal/core"
	"github.com/reusing-code/kontor/backend/internal/drafts"
	"github.com/reusing-code/kontor/backend/internal/mcpserver"
	"github.com/reusing-code/kontor/backend/internal/mcptool"
	"github.com/reusing-code/kontor/backend/internal/middleware"
	"github.com/reusing-code/kontor/backend/internal/module"
	"github.com/reusing-code/kontor/backend/internal/modules/auto"
	"github.com/reusing-code/kontor/backend/internal/modules/contracts"
	"github.com/reusing-code/kontor/backend/internal/modules/ledger"
	"github.com/reusing-code/kontor/backend/internal/modules/purchases"
	"github.com/reusing-code/kontor/backend/internal/paperless"
)

type mcpDeps struct {
	registry  *module.Registry
	coreStore *core.Store
	drafts    *drafts.Store
	paperless *paperless.Service
	contracts *contracts.Module
	purchases *purchases.Module
	auto      *auto.Module
	ledger    *ledger.Module
	patStore  middleware.PATVerifier
	jwtSecret []byte
}

// newMCPHandler builds the /mcp endpoint: PAT-only authentication around the
// core tools plus the tools every module registers.
func (s *Server) newMCPHandler(d mcpDeps) http.Handler {
	reg := mcptool.NewRegistrar(mcptool.Env{
		Drafts:  d.drafts,
		Enabled: d.coreStore.ModuleEnabled,
		Logger:  s.logger,
	})
	deps := mcpserver.Deps{
		Registrar: reg,
		Paperless: d.paperless,
		Modules:   moduleStatuses(d.registry, d.coreStore),
		Entities:  entityLookup(d),
		PublicURL: s.cfg.PublicURL,
	}
	mcpserver.RegisterCoreTools(deps)
	d.registry.RegisterTools(reg)

	return middleware.Auth(d.jwtSecret,
		middleware.WithPATs(d.patStore),
		middleware.PATOnly(),
		middleware.WithChallenge("Bearer"),
		middleware.WithAuthLogger(s.logger),
	)(mcpserver.Handler(deps))
}

func moduleStatuses(registry *module.Registry, coreStore *core.Store) func(context.Context, string) ([]mcpserver.ModuleStatus, error) {
	return func(ctx context.Context, userID string) ([]mcpserver.ModuleStatus, error) {
		settings, err := coreStore.GetSettings(ctx, userID)
		if err != nil {
			return nil, err
		}
		ids := registry.IDs()
		statuses := make([]mcpserver.ModuleStatus, 0, len(ids))
		for _, id := range ids {
			statuses = append(statuses, mcpserver.ModuleStatus{ID: id, Enabled: !slices.Contains(settings.DisabledModules, id)})
		}
		return statuses, nil
	}
}

// entityLookup resolves the entities a paperless document can be linked to.
func entityLookup(d mcpDeps) mcpserver.EntityLookup {
	return func(ctx context.Context, userID, entityType string, id uuid.UUID) (mcpserver.EntityRef, error) {
		switch entityType {
		case "contract":
			if _, err := d.contracts.Store().Get(ctx, userID, id); err != nil {
				return mcpserver.EntityRef{}, err
			}
			return mcpserver.EntityRef{ModuleID: contracts.ModuleID, Path: "/contracts/" + id.String()}, nil
		case "purchase":
			if _, err := d.purchases.Store().Get(ctx, userID, id); err != nil {
				return mcpserver.EntityRef{}, err
			}
			return mcpserver.EntityRef{ModuleID: purchases.ModuleID, Path: "/purchases/" + id.String()}, nil
		case "vehicle":
			if _, err := d.auto.Store().GetVehicle(ctx, userID, id); err != nil {
				return mcpserver.EntityRef{}, err
			}
			return mcpserver.EntityRef{ModuleID: auto.ModuleID, Path: "/auto/vehicles/" + id.String()}, nil
		case "cost":
			cost, err := d.auto.Store().GetCostEntry(ctx, userID, id)
			if err != nil {
				return mcpserver.EntityRef{}, err
			}
			return mcpserver.EntityRef{ModuleID: auto.ModuleID, Path: "/auto/vehicles/" + cost.VehicleID.String()}, nil
		case "transaction":
			if _, err := d.ledger.Store().GetLedgerTransaction(ctx, userID, id); err != nil {
				return mcpserver.EntityRef{}, err
			}
			return mcpserver.EntityRef{ModuleID: ledger.ModuleID, Path: "/ledger/transactions/" + id.String()}, nil
		}
		// attach_paperless_documents validates the type before it looks anything up
		return mcpserver.EntityRef{}, fmt.Errorf("unsupported entity type %q", entityType)
	}
}
