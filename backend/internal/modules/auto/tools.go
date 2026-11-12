package auto

import (
	"context"
	"errors"
	"slices"
	"strings"

	"github.com/google/uuid"
	"github.com/reusing-code/kontor/backend/internal/drafts"
	"github.com/reusing-code/kontor/backend/internal/mcptool"
	"github.com/reusing-code/kontor/backend/internal/storage"
)

type vehicleItem struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Make         string `json:"make,omitempty"`
	Model        string `json:"model,omitempty"`
	Year         *int   `json:"year,omitempty"`
	LicensePlate string `json:"licensePlate,omitempty"`
}

type listVehiclesOut struct {
	Items []vehicleItem `json:"items"`
}

type searchCostsIn struct {
	VehicleID string `json:"vehicleId,omitempty" jsonschema:"only costs of this vehicle (UUID from list_vehicles)"`
	Query     string `json:"query,omitempty" jsonschema:"case-insensitive text in description or vendor"`
	Type      string `json:"type,omitempty" jsonschema:"service, fuel, insurance, tax, tires, mileage or misc"`
	DateFrom  string `json:"dateFrom,omitempty" jsonschema:"earliest date, YYYY-MM-DD"`
	DateTo    string `json:"dateTo,omitempty" jsonschema:"latest date, YYYY-MM-DD"`
	Limit     int    `json:"limit,omitempty" jsonschema:"maximum results, default 25, at most 100"`
}

type costItem struct {
	ID          string   `json:"id"`
	VehicleID   string   `json:"vehicleId"`
	Type        string   `json:"type"`
	Description string   `json:"description,omitempty"`
	Vendor      string   `json:"vendor,omitempty"`
	Amount      *float64 `json:"amount,omitempty"`
	Date        string   `json:"date"`
	Mileage     *float64 `json:"mileage,omitempty"`
}

type searchCostsOut struct {
	Items     []costItem `json:"items"`
	Truncated bool       `json:"truncated"`
}

type createCostDraftIn struct {
	Title     string         `json:"title" jsonschema:"short description shown in the inbox, e.g. the document name"`
	Key       string         `json:"key,omitempty" jsonschema:"stable idempotency key, e.g. file name or hash; repeating it while the draft is pending is rejected"`
	Note      string         `json:"note,omitempty" jsonschema:"free text for the reviewer (at most 500 characters)"`
	VehicleID string         `json:"vehicleId" jsonschema:"UUID of the vehicle (list_vehicles)"`
	Input     CostEntryInput `json:"input" jsonschema:"the cost entry; type is service, fuel, insurance, tax, tires, mileage or misc, date is YYYY-MM-DD, amount is EUR, mileage entries need mileage"`
}

type createVehicleDraftIn struct {
	Title string       `json:"title"`
	Key   string       `json:"key,omitempty"`
	Note  string       `json:"note,omitempty"`
	Input VehicleInput `json:"input" jsonschema:"the vehicle; purchaseDate is YYYY-MM-DD, amounts are EUR"`
}

// RegisterTools adds the auto module's MCP tools.
func (m *Module) RegisterTools(r *mcptool.Registrar) {
	spec := func(name, desc, scope string, readOnly bool) mcptool.Spec {
		return mcptool.Spec{Name: name, Description: desc, Scope: scope, Module: ModuleID, ReadOnly: readOnly}
	}

	mcptool.Add(r, spec("list_vehicles",
		"List the user's vehicles with their IDs; needed for create_vehicle_cost_draft.",
		mcptool.ScopeRead, true),
		func(ctx context.Context, c mcptool.Call, _ struct{}) (listVehiclesOut, error) {
			vehicles, err := m.store.ListVehicles(ctx, c.UserID)
			if err != nil {
				return listVehiclesOut{}, err
			}
			slices.SortFunc(vehicles, func(a, b Vehicle) int {
				return strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name))
			})
			out := listVehiclesOut{Items: make([]vehicleItem, 0, len(vehicles))}
			for _, v := range vehicles {
				out.Items = append(out.Items, vehicleItem{ID: v.ID.String(), Name: v.Name, Make: v.Make, Model: v.Model, Year: v.Year, LicensePlate: v.LicensePlate})
			}
			return out, nil
		})

	mcptool.Add(r, spec("search_vehicle_costs",
		"Search the user's existing vehicle cost entries to avoid creating duplicates. Results are bounded; truncated=true means more exist.",
		mcptool.ScopeRead, true),
		func(ctx context.Context, c mcptool.Call, in searchCostsIn) (searchCostsOut, error) {
			limit, err := mcptool.Limit(in.Limit)
			if err != nil {
				return searchCostsOut{}, err
			}
			if err := mcptool.CheckDateRange(in.DateFrom, in.DateTo); err != nil {
				return searchCostsOut{}, err
			}
			if in.Type != "" && !validCostTypes[in.Type] {
				return searchCostsOut{}, mcptool.Errorf(mcptool.CodeInvalidInput, "type must be service, fuel, insurance, tax, tires, mileage or misc")
			}
			vehicleIDs, err := m.vehicleScope(ctx, c.UserID, in.VehicleID)
			if err != nil {
				return searchCostsOut{}, err
			}
			var entries []CostEntry
			for _, id := range vehicleIDs {
				costs, err := m.store.ListCostEntries(ctx, c.UserID, id)
				if err != nil {
					return searchCostsOut{}, err
				}
				entries = append(entries, costs...)
			}
			slices.SortFunc(entries, func(a, b CostEntry) int {
				if x := strings.Compare(b.Date, a.Date); x != 0 {
					return x
				}
				return strings.Compare(a.ID.String(), b.ID.String())
			})
			var items []costItem
			for _, e := range entries {
				if in.Type != "" && e.Type != in.Type {
					continue
				}
				if !mcptool.ContainsAny(in.Query, e.Description, e.Vendor) || !mcptool.InDateRange(e.Date, in.DateFrom, in.DateTo) {
					continue
				}
				items = append(items, costItem{
					ID: e.ID.String(), VehicleID: e.VehicleID.String(), Type: e.Type, Description: e.Description,
					Vendor: e.Vendor, Amount: e.Amount, Date: e.Date, Mileage: e.Mileage,
				})
				if len(items) > limit {
					break
				}
			}
			items, truncated := mcptool.Truncate(items, limit)
			return searchCostsOut{Items: items, Truncated: truncated}, nil
		})

	mcptool.Add(r, spec("create_vehicle_cost_draft",
		"Propose a new cost entry for a vehicle (service, fuel, insurance, tax, tires, mileage, misc). Creates a draft in the user's inbox; nothing is created until the user accepts it. "+
			"Amounts are EUR, dates YYYY-MM-DD. Use list_vehicles for vehicleId and search_vehicle_costs to avoid duplicates. "+
			"Give a short title and a stable key (file name or hash) so re-runs do not duplicate the draft while it is pending.",
		mcptool.ScopeDrafts, false),
		func(ctx context.Context, c mcptool.Call, in createCostDraftIn) (mcptool.DraftResult, error) {
			input := in.Input
			if err := input.Validate(); err != nil {
				return mcptool.DraftResult{}, mcptool.Errorf(mcptool.CodeInvalidInput, "%s", err.Error())
			}
			if err := mcptool.CheckDate("date", input.Date); err != nil {
				return mcptool.DraftResult{}, err
			}
			id, err := uuid.Parse(in.VehicleID)
			if err != nil {
				return mcptool.DraftResult{}, mcptool.Errorf(mcptool.CodeInvalidInput, "vehicleId must be a UUID")
			}
			if _, err := m.store.GetVehicle(ctx, c.UserID, id); errors.Is(err, storage.ErrNotFound) {
				return mcptool.DraftResult{}, mcptool.Errorf(mcptool.CodeInvalidInput, "vehicleId does not match a vehicle")
			} else if err != nil {
				return mcptool.DraftResult{}, err
			}
			return r.CreateDraftFrom(ctx, c, mcptool.DraftSpec{Target: drafts.TargetCost, ParentID: in.VehicleID, Input: input, Title: in.Title, Key: in.Key, Note: in.Note})
		})

	mcptool.Add(r, spec("create_vehicle_draft",
		"Propose a new vehicle. Creates a draft in the user's inbox; nothing is created until the user accepts it. "+
			"Check list_vehicles first. Amounts are EUR, dates YYYY-MM-DD. Give a short title and a stable key.",
		mcptool.ScopeDrafts, false),
		func(ctx context.Context, c mcptool.Call, in createVehicleDraftIn) (mcptool.DraftResult, error) {
			input := in.Input
			if err := input.Validate(); err != nil {
				return mcptool.DraftResult{}, mcptool.Errorf(mcptool.CodeInvalidInput, "%s", err.Error())
			}
			return r.CreateDraftFrom(ctx, c, mcptool.DraftSpec{Target: drafts.TargetVehicle, Input: input, Title: in.Title, Key: in.Key, Note: in.Note})
		})
}

// vehicleScope returns the vehicle IDs to search: the given one (which must
// exist for the user) or all of them.
func (m *Module) vehicleScope(ctx context.Context, userID, vehicleID string) ([]uuid.UUID, error) {
	if vehicleID != "" {
		id, err := uuid.Parse(vehicleID)
		if err != nil {
			return nil, mcptool.Errorf(mcptool.CodeInvalidInput, "vehicleId must be a UUID")
		}
		if _, err := m.store.GetVehicle(ctx, userID, id); errors.Is(err, storage.ErrNotFound) {
			return nil, mcptool.Errorf(mcptool.CodeNotFound, "no vehicle with this id")
		} else if err != nil {
			return nil, err
		}
		return []uuid.UUID{id}, nil
	}
	vehicles, err := m.store.ListVehicles(ctx, userID)
	if err != nil {
		return nil, err
	}
	ids := make([]uuid.UUID, 0, len(vehicles))
	for _, v := range vehicles {
		ids = append(ids, v.ID)
	}
	return ids, nil
}
