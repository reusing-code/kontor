package contracts

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/reusing-code/kontor/backend/internal/categories"
	"github.com/reusing-code/kontor/backend/internal/module"
)

const sectionSchemaVersion = 0

type exportSection struct {
	SchemaVersion uint64                `json:"schemaVersion"`
	Categories    []categories.Category `json:"categories"`
	Contracts     []Contract            `json:"contracts"`
}

func (m *Module) IsEmpty(ctx context.Context, userID string) (bool, error) {
	items, err := m.store.List(ctx, userID)
	if err != nil {
		return false, err
	}
	return len(items) == 0, nil
}

func (m *Module) ExportSection(ctx context.Context, userID string) (json.RawMessage, error) {
	section := exportSection{SchemaVersion: sectionSchemaVersion}
	var err error
	if section.Categories, err = m.categories.List(ctx, userID, ModuleID); err != nil {
		return nil, err
	}
	if section.Contracts, err = m.store.List(ctx, userID); err != nil {
		return nil, err
	}
	return json.Marshal(section)
}

func (m *Module) ImportSection(ctx context.Context, userID string, data json.RawMessage, res *module.ImportResult) error {
	var section exportSection
	if err := json.Unmarshal(data, &section); err != nil {
		return fmt.Errorf("%w: invalid contracts section: %v", module.ErrInvalidSection, err)
	}
	if section.SchemaVersion != sectionSchemaVersion {
		return fmt.Errorf("%w: contracts section has schema version %d, this version supports %d", module.ErrInvalidSection, section.SchemaVersion, sectionSchemaVersion)
	}

	if err := m.categories.Replace(ctx, userID, ModuleID, section.Categories); err != nil {
		return err
	}
	res.Add(ModuleID+"Categories", len(section.Categories))
	for _, contract := range section.Contracts {
		if err := m.store.Create(ctx, userID, contract); err != nil {
			return err
		}
	}
	res.Add("contracts", len(section.Contracts))
	return nil
}

func (m *Module) PruneDeadLinks(ctx context.Context, userID string, res *module.ImportResult) error {
	pruned, err := m.store.PruneDeadTransactionLinks(ctx, userID)
	if err != nil {
		return err
	}
	if pruned > 0 {
		res.Warnf("contracts: removed %d link(s) to missing ledger transactions", pruned)
	}
	return nil
}
