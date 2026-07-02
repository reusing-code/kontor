package handler

import (
	"context"
	_ "embed"
	"time"

	"github.com/google/uuid"
	"github.com/reusing-code/kontor/backend/internal/model"
	"go.yaml.in/yaml/v2"
)

//go:embed default_ledger_categories.yaml
var defaultLedgerCategoriesYAML []byte

type ledgerCategorySeed struct {
	Name       string               `yaml:"name"`
	MatchWords []string             `yaml:"matchWords"`
	Children   []ledgerCategorySeed `yaml:"children"`
}

func loadDefaultLedgerCategories(now time.Time) ([]model.LedgerCategory, error) {
	var seeds []ledgerCategorySeed
	if err := yaml.Unmarshal(defaultLedgerCategoriesYAML, &seeds); err != nil {
		return nil, err
	}
	var categories []model.LedgerCategory
	var walk func(parentID *uuid.UUID, nodes []ledgerCategorySeed)
	walk = func(parentID *uuid.UUID, nodes []ledgerCategorySeed) {
		for _, node := range nodes {
			id := uuid.New()
			categories = append(categories, model.LedgerCategory{
				ID:         id,
				Name:       node.Name,
				ParentID:   parentID,
				MatchWords: node.MatchWords,
				CreatedAt:  now,
				UpdatedAt:  now,
			})
			walk(&id, node.Children)
		}
	}
	walk(nil, seeds)
	return categories, nil
}

// SeedLedgerDefaults creates the default ledger category tree if the user has
// no ledger categories yet.
func (h *Handler) SeedLedgerDefaults(ctx context.Context, userID string) error {
	existing, err := h.store.ListLedgerCategories(ctx, userID)
	if err != nil {
		return err
	}
	if len(existing) > 0 {
		return nil
	}
	defaults, err := loadDefaultLedgerCategories(time.Now().UTC())
	if err != nil {
		return err
	}
	for _, category := range defaults {
		if err := h.store.CreateLedgerCategory(ctx, userID, category); err != nil {
			return err
		}
	}
	return nil
}
