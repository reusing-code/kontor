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
	"github.com/reusing-code/kontor/backend/internal/paperless"
)

// maxMCPBodyBytes caps a JSON-RPC request to /mcp.
const maxMCPBodyBytes = 1 << 20

type mcpDeps struct {
	registry  *module.Registry
	coreStore *core.Store
	drafts    *drafts.Store
	paperless *paperless.Service
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
		Entities:  entityLookup(d.registry),
		PublicURL: s.cfg.PublicURL,
	}
	mcpserver.RegisterCoreTools(deps)
	d.registry.RegisterTools(reg)

	return middleware.Auth(d.jwtSecret,
		middleware.WithPATs(d.patStore),
		middleware.PATOnly(),
		middleware.WithChallenge("Bearer"),
		middleware.WithAuthLogger(s.logger),
	)(middleware.MaxBody(maxMCPBodyBytes)(mcpserver.Handler(deps)))
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

// entityLookup resolves the entities a paperless document can be linked to
// through the modules that own them.
func entityLookup(registry *module.Registry) mcpserver.EntityLookup {
	return func(ctx context.Context, userID, entityType string, id uuid.UUID) (mcpserver.EntityRef, error) {
		for _, m := range registry.All() {
			locator, ok := m.(module.EntityLocator)
			if !ok {
				continue
			}
			path, owned, err := locator.LocateEntity(ctx, userID, entityType, id)
			if !owned {
				continue
			}
			if err != nil {
				return mcpserver.EntityRef{}, err
			}
			return mcpserver.EntityRef{ModuleID: m.ID(), Path: path}, nil
		}
		// attach_paperless_documents validates the type before it looks anything up
		return mcpserver.EntityRef{}, fmt.Errorf("unsupported entity type %q", entityType)
	}
}
