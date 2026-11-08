package server

import (
	"context"
	"fmt"
	"strings"

	"github.com/reusing-code/kontor/backend/internal/categories"
	"github.com/reusing-code/kontor/backend/internal/core"
	"github.com/reusing-code/kontor/backend/internal/drafts"
	"github.com/reusing-code/kontor/backend/internal/extraction"
	"github.com/reusing-code/kontor/backend/internal/modules/auto"
	"github.com/reusing-code/kontor/backend/internal/modules/contracts"
	"github.com/reusing-code/kontor/backend/internal/modules/purchases"
)

var draftTargetModules = map[string]string{
	drafts.TargetPurchase: purchases.ModuleID,
	drafts.TargetContract: contracts.ModuleID,
	drafts.TargetCost:     auto.ModuleID,
}

// extractionDeps adapts the module stores for the classifier, which must not
// import modules itself.
func extractionDeps(coreStore *core.Store, catStore *categories.Store, autoStore *auto.Store) extraction.Deps {
	return extraction.Deps{
		TargetEnabled: func(ctx context.Context, userID, target string) (bool, error) {
			moduleID, ok := draftTargetModules[target]
			if !ok {
				return false, nil
			}
			return coreStore.ModuleEnabled(ctx, userID, moduleID)
		},
		Categories: func(ctx context.Context, userID, target string) ([]extraction.Option, error) {
			moduleID, ok := draftTargetModules[target]
			if !ok || target == drafts.TargetCost {
				return nil, fmt.Errorf("no categories for target %q", target)
			}
			cats, err := catStore.List(ctx, userID, moduleID)
			if err != nil {
				return nil, err
			}
			opts := make([]extraction.Option, 0, len(cats))
			for _, c := range cats {
				opts = append(opts, extraction.Option{ID: c.ID.String(), Name: c.Name})
			}
			return opts, nil
		},
		Vehicles: func(ctx context.Context, userID string) ([]extraction.Option, error) {
			vehicles, err := autoStore.ListVehicles(ctx, userID)
			if err != nil {
				return nil, err
			}
			opts := make([]extraction.Option, 0, len(vehicles))
			for _, v := range vehicles {
				opts = append(opts, extraction.Option{ID: v.ID.String(), Name: vehicleLabel(v)})
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

func userIDs(coreStore *core.Store) func(context.Context) ([]string, error) {
	return func(ctx context.Context) ([]string, error) {
		users, err := coreStore.ListUsers(ctx)
		if err != nil {
			return nil, err
		}
		ids := make([]string, 0, len(users))
		for _, u := range users {
			ids = append(ids, u.ID.String())
		}
		return ids, nil
	}
}
