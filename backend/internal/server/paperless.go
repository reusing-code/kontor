package server

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/reusing-code/kontor/backend/internal/categories"
	"github.com/reusing-code/kontor/backend/internal/core"
	"github.com/reusing-code/kontor/backend/internal/drafts"
	"github.com/reusing-code/kontor/backend/internal/modules/auto"
	"github.com/reusing-code/kontor/backend/internal/modules/contracts"
	"github.com/reusing-code/kontor/backend/internal/modules/purchases"
	"github.com/reusing-code/kontor/backend/internal/paperless"
)

var draftTargetModules = map[string]string{
	drafts.TargetPurchase: purchases.ModuleID,
	drafts.TargetContract: contracts.ModuleID,
	drafts.TargetCost:     auto.ModuleID,
}

// pipelineDeps adapts the module stores for the paperless pipeline, which
// must not import modules itself.
func pipelineDeps(coreStore *core.Store, catStore *categories.Store, autoStore *auto.Store) paperless.PipelineDeps {
	return paperless.PipelineDeps{
		UserIDs: func(ctx context.Context) ([]string, error) {
			users, err := coreStore.ListUsers(ctx)
			if err != nil {
				return nil, err
			}
			ids := make([]string, 0, len(users))
			for _, u := range users {
				ids = append(ids, u.ID.String())
			}
			return ids, nil
		},
		TargetEnabled: func(ctx context.Context, userID, target string) (bool, error) {
			moduleID, ok := draftTargetModules[target]
			if !ok {
				return false, nil
			}
			return coreStore.ModuleEnabled(ctx, userID, moduleID)
		},
		Categories: func(ctx context.Context, userID, target string) ([]paperless.Option, error) {
			moduleID, ok := draftTargetModules[target]
			if !ok || target == drafts.TargetCost {
				return nil, fmt.Errorf("no categories for target %q", target)
			}
			cats, err := catStore.List(ctx, userID, moduleID)
			if err != nil {
				return nil, err
			}
			opts := make([]paperless.Option, 0, len(cats))
			for _, c := range cats {
				opts = append(opts, paperless.Option{ID: c.ID.String(), Name: c.Name})
			}
			return opts, nil
		},
		Vehicles: func(ctx context.Context, userID string) ([]paperless.Option, error) {
			vehicles, err := autoStore.ListVehicles(ctx, userID)
			if err != nil {
				return nil, err
			}
			opts := make([]paperless.Option, 0, len(vehicles))
			for _, v := range vehicles {
				opts = append(opts, paperless.Option{ID: v.ID.String(), Name: vehicleLabel(v)})
			}
			return opts, nil
		},
	}
}

func vehicleLabel(v auto.Vehicle) string {
	var details []string
	if makeModel := strings.TrimSpace(v.Make + " " + v.Model); makeModel != "" {
		details = append(details, makeModel)
	}
	if v.LicensePlate != "" {
		details = append(details, v.LicensePlate)
	}
	if len(details) == 0 {
		return v.Name
	}
	return fmt.Sprintf("%s (%s)", v.Name, strings.Join(details, ", "))
}

func registerPaperlessRoutes(mux *http.ServeMux, h *paperless.Handler, pipeline *paperless.Pipeline) {
	mux.HandleFunc("GET /api/v1/paperless/config", h.GetConfig)
	mux.HandleFunc("PUT /api/v1/paperless/config", h.PutConfig)
	mux.HandleFunc("DELETE /api/v1/paperless/config", h.DeleteConfig)
	mux.HandleFunc("POST /api/v1/paperless/config/test", h.TestConfig)
	mux.HandleFunc("GET /api/v1/paperless/search", h.Search)
	mux.HandleFunc("GET /api/v1/paperless/documents/{documentId}/thumb", h.Thumbnail)
	mux.HandleFunc("GET /api/v1/paperless/links/{entityType}/{entityId}", h.ListLinks)
	mux.HandleFunc("POST /api/v1/paperless/links/{entityType}/{entityId}", h.AttachLinks)
	mux.HandleFunc("DELETE /api/v1/paperless/links/{entityType}/{entityId}/{documentId}", h.DetachLink)
	mux.HandleFunc("GET /api/v1/paperless/pipeline", h.GetPipelineSettings)
	mux.HandleFunc("PUT /api/v1/paperless/pipeline", h.PutPipelineSettings)
	mux.HandleFunc("POST /api/v1/paperless/pipeline/run", pipeline.Run)
}

func registerDraftRoutes(mux *http.ServeMux, h *drafts.Handler) {
	mux.HandleFunc("GET /api/v1/drafts", h.List)
	mux.HandleFunc("GET /api/v1/drafts/{id}", h.Get)
	mux.HandleFunc("PUT /api/v1/drafts/{id}", h.Update)
	mux.HandleFunc("POST /api/v1/drafts/{id}/accept", h.Accept)
	mux.HandleFunc("POST /api/v1/drafts/{id}/reject", h.Reject)
}
