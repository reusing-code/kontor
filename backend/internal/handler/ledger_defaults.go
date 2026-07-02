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

func (h *Handler) seedDefaultLedgerCategoriesIfEmpty(ctx context.Context, userID string) {
	existing, err := h.store.ListLedgerCategories(ctx, userID)
	if err != nil {
		h.logger.Error("listing ledger categories for seed check", "error", err)
		return
	}
	if len(existing) > 0 {
		return
	}
	defaults, err := loadDefaultLedgerCategories(time.Now().UTC())
	if err != nil {
		h.logger.Error("loading default ledger categories", "error", err)
		return
	}
	for _, category := range defaults {
		if err := h.store.CreateLedgerCategory(ctx, userID, category); err != nil {
			h.logger.Error("seeding default ledger category", "name", category.Name, "error", err)
		}
	}
}
