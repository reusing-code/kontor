package auto

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/reusing-code/kontor/backend/internal/module"
)

const sectionSchemaVersion = 0

type exportSection struct {
	SchemaVersion uint64      `json:"schemaVersion"`
	Vehicles      []Vehicle   `json:"vehicles"`
	CostEntries   []CostEntry `json:"costEntries"`
}

func (m *Module) IsEmpty(ctx context.Context, userID string) (bool, error) {
	vehicles, err := m.store.ListVehicles(ctx, userID)
	if err != nil {
		return false, err
	}
	return len(vehicles) == 0, nil
}

func (m *Module) ExportSection(ctx context.Context, userID string) (json.RawMessage, error) {
	section := exportSection{SchemaVersion: sectionSchemaVersion, CostEntries: []CostEntry{}}
	var err error
	if section.Vehicles, err = m.store.ListVehicles(ctx, userID); err != nil {
		return nil, err
	}
	for _, vehicle := range section.Vehicles {
		entries, err := m.store.ListCostEntries(ctx, userID, vehicle.ID)
		if err != nil {
			return nil, err
		}
		section.CostEntries = append(section.CostEntries, entries...)
	}
	return json.Marshal(section)
}

func (m *Module) ImportSection(ctx context.Context, userID string, data json.RawMessage, res *module.ImportResult) error {
	var section exportSection
	if err := json.Unmarshal(data, &section); err != nil {
		return fmt.Errorf("%w: invalid auto section: %v", module.ErrInvalidSection, err)
	}
	if section.SchemaVersion != sectionSchemaVersion {
		return fmt.Errorf("%w: auto section has schema version %d, this version supports %d", module.ErrInvalidSection, section.SchemaVersion, sectionSchemaVersion)
	}

	for _, vehicle := range section.Vehicles {
		if err := m.store.CreateVehicle(ctx, userID, vehicle); err != nil {
			return err
		}
	}
	res.Add("vehicles", len(section.Vehicles))
	for _, entry := range section.CostEntries {
		if err := m.store.CreateCostEntry(ctx, userID, entry); err != nil {
			return err
		}
	}
	res.Add("costEntries", len(section.CostEntries))
	return nil
}

func (m *Module) PruneDeadLinks(ctx context.Context, userID string, res *module.ImportResult) error {
	pruned, err := m.store.PruneDeadTransactionLinks(ctx, userID)
	if err != nil {
		return err
	}
	if pruned > 0 {
		res.Warnf("auto: removed %d link(s) to missing ledger transactions", pruned)
	}
	return nil
}
