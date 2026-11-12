package auto

import (
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/reusing-code/kontor/backend/internal/core"
	"github.com/reusing-code/kontor/backend/internal/drafts"
	"github.com/reusing-code/kontor/backend/internal/mcptool"
	"github.com/reusing-code/kontor/backend/internal/mcptool/mcptooltest"
	"github.com/reusing-code/kontor/backend/internal/storage"
	"github.com/reusing-code/kontor/backend/internal/storage/link"
)

const (
	userA = "00000000-0000-0000-0000-00000000000a"
	userB = "00000000-0000-0000-0000-00000000000b"
)

type toolEnv struct {
	*mcptooltest.Env
	mod       *Module
	coreStore *core.Store
}

func newToolEnv(t *testing.T) *toolEnv {
	t.Helper()
	engine, err := storage.Open(t.TempDir(), slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { engine.Close() })
	coreStore := core.NewStore(engine)
	mod := New(engine, link.NewRegistry(), slog.New(slog.DiscardHandler))
	env := mcptooltest.New(t, engine, coreStore.ModuleEnabled)
	mod.RegisterTools(env.Reg)
	return &toolEnv{Env: env, mod: mod, coreStore: coreStore}
}

func (e *toolEnv) vehicle(t *testing.T, user, name string) uuid.UUID {
	t.Helper()
	id := uuid.New()
	now := time.Now().UTC()
	year := 2019
	if err := e.mod.Store().CreateVehicle(t.Context(), user, Vehicle{ID: id, Name: name, Make: "VW", Model: "Golf", Year: &year, LicensePlate: "AB-123", CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
	return id
}

func (e *toolEnv) cost(t *testing.T, user string, vehicleID uuid.UUID, typ, desc, vendor, date string) {
	t.Helper()
	now := time.Now().UTC()
	amount := 100.0
	if err := e.mod.Store().CreateCostEntry(t.Context(), user, CostEntry{
		ID: uuid.New(), VehicleID: vehicleID, Type: typ, Description: desc, Vendor: vendor, Amount: &amount, Date: date, CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
}

func (e *toolEnv) drafts(t *testing.T, user string) []drafts.Draft {
	t.Helper()
	list, err := e.Drafts.List(t.Context(), user, "")
	if err != nil {
		t.Fatal(err)
	}
	return list
}

func costArgs(vehicleID uuid.UUID, mutate func(map[string]any)) map[string]any {
	args := map[string]any{
		"title": "inspection.pdf", "key": "inspection.pdf", "vehicleId": vehicleID.String(),
		"input": map[string]any{"type": "service", "description": "Inspection", "vendor": "Garage", "amount": 250.0, "date": "2026-09-15"},
	}
	if mutate != nil {
		mutate(args)
	}
	return args
}

func TestListVehicles_OwnDataOnly(t *testing.T) {
	e := newToolEnv(t)
	own := e.vehicle(t, userA, "Golf")
	e.vehicle(t, userB, "Secret car")

	res, _ := mcptooltest.Call(t, e.Connect(t, userA, mcptool.ScopeRead), "list_vehicles", map[string]any{})
	out := mcptooltest.Decode[listVehiclesOut](t, res)
	if len(out.Items) != 1 || out.Items[0].ID != own.String() || out.Items[0].LicensePlate != "AB-123" {
		t.Fatalf("items = %+v", out.Items)
	}
}

func TestSearchVehicleCosts(t *testing.T) {
	e := newToolEnv(t)
	golf := e.vehicle(t, userA, "Golf")
	polo := e.vehicle(t, userA, "Polo")
	foreign := e.vehicle(t, userB, "Foreign")
	e.cost(t, userA, golf, "service", "Inspection", "Garage Müller", "2026-09-15")
	e.cost(t, userA, golf, "fuel", "Diesel", "Aral", "2026-10-02")
	e.cost(t, userA, polo, "fuel", "Petrol", "Shell", "2026-08-01")
	e.cost(t, userB, foreign, "fuel", "Foreign", "Aral", "2026-10-02")
	cs := e.Connect(t, userA, mcptool.ScopeRead)

	search := func(args map[string]any) searchCostsOut {
		res, _ := mcptooltest.Call(t, cs, "search_vehicle_costs", args)
		return mcptooltest.Decode[searchCostsOut](t, res)
	}
	if out := search(map[string]any{}); len(out.Items) != 3 || out.Truncated || out.Items[0].Date != "2026-10-02" {
		t.Fatalf("all = %+v", out)
	}
	if out := search(map[string]any{"vehicleId": polo.String()}); len(out.Items) != 1 || out.Items[0].Description != "Petrol" {
		t.Fatalf("vehicle = %+v", out)
	}
	if out := search(map[string]any{"query": "aral"}); len(out.Items) != 1 {
		t.Fatalf("query = %+v", out)
	}
	if out := search(map[string]any{"type": "fuel"}); len(out.Items) != 2 {
		t.Fatalf("type = %+v", out)
	}
	if out := search(map[string]any{"dateFrom": "2026-09-01", "dateTo": "2026-09-30"}); len(out.Items) != 1 || out.Items[0].Type != "service" {
		t.Fatalf("range = %+v", out)
	}
	if out := search(map[string]any{"limit": 2}); len(out.Items) != 2 || !out.Truncated {
		t.Fatalf("limit = %+v", out)
	}
	if res, text := mcptooltest.Call(t, cs, "search_vehicle_costs", map[string]any{"vehicleId": foreign.String()}); !res.IsError || !strings.HasPrefix(text, mcptool.CodeNotFound) {
		t.Fatalf("foreign vehicle: %q", text)
	}
	for _, bad := range []map[string]any{{"type": "wash"}, {"limit": 500}, {"vehicleId": "x"}, {"dateTo": "no"}} {
		if res, text := mcptooltest.Call(t, cs, "search_vehicle_costs", bad); !res.IsError || !strings.HasPrefix(text, mcptool.CodeInvalidInput) {
			t.Errorf("%v: %q", bad, text)
		}
	}
}

func TestCreateVehicleCostDraft(t *testing.T) {
	e := newToolEnv(t)
	golf := e.vehicle(t, userA, "Golf")
	cs := e.Connect(t, userA, mcptool.ScopeDrafts)

	res, text := mcptooltest.Call(t, cs, "create_vehicle_cost_draft", costArgs(golf, nil))
	if res.IsError {
		t.Fatalf("error: %s", text)
	}
	list := e.drafts(t, userA)
	if len(list) != 1 {
		t.Fatalf("drafts = %+v", list)
	}
	d := list[0]
	var input CostEntryInput
	if err := json.Unmarshal(d.Input, &input); err != nil || input.Type != "service" || input.Date != "2026-09-15" {
		t.Fatalf("input = %s", d.Input)
	}
	if d.TargetType != drafts.TargetCost || d.ParentID != golf.String() || d.Source.Kind != drafts.SourceMCP {
		t.Fatalf("draft = %+v", d)
	}
	if costs, _ := e.mod.Store().ListCostEntries(t.Context(), userA, golf); len(costs) != 0 {
		t.Fatal("a draft tool created a cost entry")
	}
	if res, text := mcptooltest.Call(t, cs, "create_vehicle_cost_draft", costArgs(golf, nil)); !res.IsError || !strings.HasPrefix(text, mcptool.CodeAlreadyPending) {
		t.Fatalf("duplicate: %q", text)
	}
}

func TestCreateVehicleCostDraft_InvalidInputStoresNothing(t *testing.T) {
	e := newToolEnv(t)
	golf := e.vehicle(t, userA, "Golf")
	foreign := e.vehicle(t, userB, "Foreign")
	cs := e.Connect(t, userA, mcptool.ScopeDrafts)

	cases := map[string]map[string]any{
		"missing type":      costArgs(golf, func(a map[string]any) { delete(a["input"].(map[string]any), "type") }),
		"bad type":          costArgs(golf, func(a map[string]any) { a["input"].(map[string]any)["type"] = "wash" }),
		"bad date":          costArgs(golf, func(a map[string]any) { a["input"].(map[string]any)["date"] = "15.09.2026" }),
		"mileage w/o value": costArgs(golf, func(a map[string]any) { a["input"].(map[string]any)["type"] = "mileage" }),
		"unknown vehicle":   costArgs(uuid.New(), nil),
		"foreign vehicle":   costArgs(foreign, nil),
		"bad vehicle id":    costArgs(golf, func(a map[string]any) { a["vehicleId"] = "x" }),
		"empty title":       costArgs(golf, func(a map[string]any) { a["title"] = "" }),
	}
	for name, args := range cases {
		t.Run(name, func(t *testing.T) {
			if res, _ := mcptooltest.Call(t, cs, "create_vehicle_cost_draft", args); !res.IsError {
				t.Fatal("expected a tool error")
			}
		})
	}
	if n := len(e.drafts(t, userA)); n != 0 {
		t.Fatalf("%d drafts stored", n)
	}
}

func TestCreateVehicleDraft(t *testing.T) {
	e := newToolEnv(t)
	cs := e.Connect(t, userA, mcptool.ScopeDrafts)
	args := map[string]any{"title": "registration.pdf", "key": "reg", "input": map[string]any{"name": "Golf", "make": "VW", "year": 2019, "purchaseDate": "2024-02-01", "purchasePrice": 18000.0}}

	if res, text := mcptooltest.Call(t, cs, "create_vehicle_draft", args); res.IsError {
		t.Fatalf("error: %s", text)
	}
	list := e.drafts(t, userA)
	if len(list) != 1 || list[0].TargetType != drafts.TargetVehicle || list[0].ParentID != "" {
		t.Fatalf("drafts = %+v", list)
	}
	var input VehicleInput
	if err := json.Unmarshal(list[0].Input, &input); err != nil || input.Name != "Golf" || *input.Year != 2019 {
		t.Fatalf("input = %s", list[0].Input)
	}
	if vs, _ := e.mod.Store().ListVehicles(t.Context(), userA); len(vs) != 0 {
		t.Fatal("a draft tool created a vehicle")
	}
	if res, text := mcptooltest.Call(t, cs, "create_vehicle_draft", args); !res.IsError || !strings.HasPrefix(text, mcptool.CodeAlreadyPending) {
		t.Fatalf("duplicate: %q", text)
	}
	for name, input := range map[string]map[string]any{
		"no name":  {"name": ""},
		"bad date": {"name": "x", "purchaseDate": "soon"},
	} {
		if res, _ := mcptooltest.Call(t, cs, "create_vehicle_draft", map[string]any{"title": "t", "input": input}); !res.IsError {
			t.Errorf("%s accepted", name)
		}
	}
	if n := len(e.drafts(t, userA)); n != 1 {
		t.Fatalf("%d drafts, want 1", n)
	}
}

func TestAutoToolsHiddenWhenModuleDisabled(t *testing.T) {
	e := newToolEnv(t)
	golf := e.vehicle(t, userA, "Golf")
	st := core.DefaultUserSettings()
	st.DisabledModules = []string{ModuleID}
	if err := e.coreStore.UpdateSettings(t.Context(), userA, st); err != nil {
		t.Fatal(err)
	}
	cs := e.Connect(t, userA, mcptool.ScopeRead, mcptool.ScopeDrafts, mcptool.ScopeLinks)
	if names := mcptooltest.ToolNames(t, cs); len(names) != 0 {
		t.Fatalf("tools = %v", names)
	}
	if res, text := mcptooltest.Call(t, cs, "create_vehicle_cost_draft", costArgs(golf, nil)); !res.IsError || !strings.HasPrefix(text, mcptool.CodeModuleDisabled) {
		t.Fatalf("call: %q", text)
	}
	if n := len(e.drafts(t, userA)); n != 0 {
		t.Fatalf("%d drafts stored", n)
	}
}

func TestAutoDraftToolsNeedDraftsScope(t *testing.T) {
	e := newToolEnv(t)
	golf := e.vehicle(t, userA, "Golf")
	cs := e.Connect(t, userA, mcptool.ScopeRead)
	for name, args := range map[string]map[string]any{
		"create_vehicle_cost_draft": costArgs(golf, nil),
		"create_vehicle_draft":      {"title": "t", "input": map[string]any{"name": "x"}},
	} {
		if res, text := mcptooltest.Call(t, cs, name, args); !res.IsError || !strings.HasPrefix(text, mcptool.CodeInsufficientScope) {
			t.Errorf("%s: %q", name, text)
		}
	}
}
