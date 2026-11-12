package contracts

import (
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/reusing-code/kontor/backend/internal/categories"
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
	cats      *categories.Store
	coreStore *core.Store
}

func newToolEnv(t *testing.T) *toolEnv {
	t.Helper()
	engine, err := storage.Open(t.TempDir(), slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { engine.Close() })
	cats := categories.NewStore(engine)
	coreStore := core.NewStore(engine)
	mod := New(engine, link.NewRegistry(), cats, coreStore, nil, slog.New(slog.DiscardHandler))
	env := mcptooltest.New(t, engine, coreStore.ModuleEnabled)
	mod.RegisterTools(env.Reg)
	return &toolEnv{Env: env, mod: mod, cats: cats, coreStore: coreStore}
}

func (e *toolEnv) category(t *testing.T, user, module, name string) uuid.UUID {
	t.Helper()
	id := uuid.New()
	now := time.Now().UTC()
	if err := e.cats.Create(t.Context(), user, module, categories.Category{ID: id, Name: name, NameKey: "k." + name, CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
	return id
}

func (e *toolEnv) contract(t *testing.T, user string, catID uuid.UUID, name, company string) {
	t.Helper()
	now := time.Now().UTC()
	price := 9.99
	err := e.mod.Store().Create(t.Context(), user, Contract{
		ID: uuid.New(), CategoryID: catID, Name: name, Company: company, Price: &price,
		BillingInterval: BillingMonthly, StartDate: "2026-01-01", CreatedAt: now, UpdatedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
}

func (e *toolEnv) disable(t *testing.T, user string) {
	t.Helper()
	st := core.DefaultUserSettings()
	st.DisabledModules = []string{ModuleID}
	if err := e.coreStore.UpdateSettings(t.Context(), user, st); err != nil {
		t.Fatal(err)
	}
}

func (e *toolEnv) pending(t *testing.T, user string) []drafts.Draft {
	t.Helper()
	list, err := e.Drafts.List(t.Context(), user, "")
	if err != nil {
		t.Fatal(err)
	}
	return list
}

func contractArgs(catID uuid.UUID, mutate func(map[string]any)) map[string]any {
	input := map[string]any{
		"name": "Home insurance", "company": "Allianz", "price": 12.5, "billingInterval": "monthly",
		"startDate": "2026-03-01", "minimumDurationMonths": 12, "extensionDurationMonths": 12, "noticePeriodMonths": 3,
	}
	args := map[string]any{"title": "allianz.pdf", "key": "allianz.pdf", "categoryId": catID.String(), "input": input}
	if mutate != nil {
		mutate(args)
	}
	return args
}

func TestListContractCategories_OwnDataOnly(t *testing.T) {
	e := newToolEnv(t)
	own := e.category(t, userA, ModuleID, "Insurance")
	e.category(t, userB, ModuleID, "Secret")
	e.category(t, userA, "purchases", "Tools")

	res, _ := mcptooltest.Call(t, e.Connect(t, userA, mcptool.ScopeRead), "list_contract_categories", map[string]any{})
	out := mcptooltest.Decode[listCategoriesOut](t, res)
	if len(out.Items) != 1 || out.Items[0].ID != own.String() || out.Items[0].Name != "Insurance" || out.Items[0].NameKey != "k.Insurance" {
		t.Fatalf("items = %+v", out.Items)
	}
}

func TestSearchContracts(t *testing.T) {
	e := newToolEnv(t)
	cat := e.category(t, userA, ModuleID, "Insurance")
	other := e.category(t, userA, ModuleID, "Telecom")
	e.contract(t, userA, cat, "Home insurance", "Allianz")
	e.contract(t, userA, other, "Mobile plan", "Telekom")
	e.contract(t, userB, cat, "Foreign contract", "Allianz")
	cs := e.Connect(t, userA, mcptool.ScopeRead)

	search := func(args map[string]any) searchContractsOut {
		res, _ := mcptooltest.Call(t, cs, "search_contracts", args)
		return mcptooltest.Decode[searchContractsOut](t, res)
	}
	if out := search(map[string]any{}); len(out.Items) != 2 || out.Truncated {
		t.Fatalf("all = %+v", out)
	}
	if out := search(map[string]any{"query": "ALLIANZ"}); len(out.Items) != 1 || out.Items[0].Name != "Home insurance" {
		t.Fatalf("query = %+v", out)
	}
	if out := search(map[string]any{"categoryId": other.String()}); len(out.Items) != 1 || out.Items[0].Name != "Mobile plan" {
		t.Fatalf("category = %+v", out)
	}
	if out := search(map[string]any{"query": "nothing"}); len(out.Items) != 0 || out.Items == nil {
		t.Fatalf("no match = %+v", out)
	}
	if out := search(map[string]any{"limit": 1}); len(out.Items) != 1 || !out.Truncated {
		t.Fatalf("limit = %+v", out)
	}
	for _, bad := range []map[string]any{{"limit": 101}, {"limit": -1}, {"categoryId": "nope"}} {
		if res, text := mcptooltest.Call(t, cs, "search_contracts", bad); !res.IsError || !strings.HasPrefix(text, mcptool.CodeInvalidInput) {
			t.Errorf("%v: %q", bad, text)
		}
	}
}

func TestSearchContracts_MaxLimitAndDefault(t *testing.T) {
	e := newToolEnv(t)
	cat := e.category(t, userA, ModuleID, "Insurance")
	for i := range 120 {
		e.contract(t, userA, cat, "Contract "+string(rune('A'+i%26))+strings.Repeat("x", i/26), "Co")
	}
	cs := e.Connect(t, userA, mcptool.ScopeRead)
	res, _ := mcptooltest.Call(t, cs, "search_contracts", map[string]any{})
	if out := mcptooltest.Decode[searchContractsOut](t, res); len(out.Items) != 25 || !out.Truncated {
		t.Fatalf("default limit: %d truncated=%v", len(out.Items), out.Truncated)
	}
	res, _ = mcptooltest.Call(t, cs, "search_contracts", map[string]any{"limit": 100})
	if out := mcptooltest.Decode[searchContractsOut](t, res); len(out.Items) != 100 || !out.Truncated {
		t.Fatalf("max limit: %d truncated=%v", len(out.Items), out.Truncated)
	}
}

func TestCreateContractDraft(t *testing.T) {
	e := newToolEnv(t)
	cat := e.category(t, userA, ModuleID, "Insurance")
	cs := e.Connect(t, userA, mcptool.ScopeDrafts)

	res, text := mcptooltest.Call(t, cs, "create_contract_draft", contractArgs(cat, nil))
	if res.IsError {
		t.Fatalf("error: %s", text)
	}
	out := mcptooltest.Decode[mcptool.DraftResult](t, res)
	list := e.pending(t, userA)
	if len(list) != 1 || list[0].ID.String() != out.DraftID || out.Status != drafts.StatusPending {
		t.Fatalf("drafts = %+v, out = %+v", list, out)
	}
	d := list[0]
	if d.TargetType != drafts.TargetContract || d.ParentID != cat.String() || d.Source.Kind != drafts.SourceMCP || d.Source.Title != "allianz.pdf" {
		t.Fatalf("draft = %+v", d)
	}
	var input ContractInput
	if err := json.Unmarshal(d.Input, &input); err != nil || input.Name != "Home insurance" || input.StartDate != "2026-03-01" || input.NoticePeriodMonths != 3 {
		t.Fatalf("input = %s (%v)", d.Input, err)
	}
	if contractsStored, _ := e.mod.Store().List(t.Context(), userA); len(contractsStored) != 0 {
		t.Fatal("a draft tool created a contract")
	}

	if res, text := mcptooltest.Call(t, cs, "create_contract_draft", contractArgs(cat, nil)); !res.IsError || !strings.HasPrefix(text, mcptool.CodeAlreadyPending) {
		t.Fatalf("duplicate key: %q", text)
	}
	noKey := contractArgs(cat, func(a map[string]any) { delete(a, "key") })
	for range 2 {
		if res, text := mcptooltest.Call(t, cs, "create_contract_draft", noKey); res.IsError {
			t.Fatalf("keyless: %s", text)
		}
	}
	if n := len(e.pending(t, userA)); n != 3 {
		t.Fatalf("drafts = %d, want 3", n)
	}
	if n := len(e.pending(t, userB)); n != 0 {
		t.Fatalf("user B has %d drafts", n)
	}
}

func TestCreateContractDraft_InvalidInputStoresNothing(t *testing.T) {
	e := newToolEnv(t)
	cat := e.category(t, userA, ModuleID, "Insurance")
	purchaseCat := e.category(t, userA, "purchases", "Tools")
	foreignCat := e.category(t, userB, ModuleID, "Foreign")
	cs := e.Connect(t, userA, mcptool.ScopeDrafts)

	cases := map[string]map[string]any{
		"missing name":      contractArgs(cat, func(a map[string]any) { a["input"].(map[string]any)["name"] = "" }),
		"bad interval":      contractArgs(cat, func(a map[string]any) { a["input"].(map[string]any)["billingInterval"] = "weekly" }),
		"bad start date":    contractArgs(cat, func(a map[string]any) { a["input"].(map[string]any)["startDate"] = "01.03.2026" }),
		"bad end date":      contractArgs(cat, func(a map[string]any) { a["input"].(map[string]any)["endDate"] = "2026-13-01" }),
		"unknown category":  contractArgs(uuid.New(), nil),
		"purchase category": contractArgs(purchaseCat, nil),
		"foreign category":  contractArgs(foreignCat, nil),
		"bad category id":   contractArgs(cat, func(a map[string]any) { a["categoryId"] = "nope" }),
		"empty title":       contractArgs(cat, func(a map[string]any) { a["title"] = "" }),
		"long note":         contractArgs(cat, func(a map[string]any) { a["note"] = strings.Repeat("x", 501) }),
		"missing input":     {"title": "t", "categoryId": cat.String()},
	}
	for name, args := range cases {
		t.Run(name, func(t *testing.T) {
			if res, _ := mcptooltest.Call(t, cs, "create_contract_draft", args); !res.IsError {
				t.Fatal("expected a tool error")
			}
		})
	}
	if n := len(e.pending(t, userA)); n != 0 {
		t.Fatalf("%d drafts stored by invalid calls", n)
	}
}

func TestCreateContractCategoryDraft(t *testing.T) {
	e := newToolEnv(t)
	cs := e.Connect(t, userA, mcptool.ScopeDrafts)

	args := map[string]any{"title": "new category", "key": "cat-1", "input": map[string]any{"name": " Energy "}}
	if res, text := mcptooltest.Call(t, cs, "create_contract_category_draft", args); res.IsError {
		t.Fatalf("error: %s", text)
	}
	list := e.pending(t, userA)
	if len(list) != 1 || list[0].TargetType != drafts.TargetContractCategory || list[0].ParentID != "" || !strings.Contains(string(list[0].Input), `"Energy"`) {
		t.Fatalf("drafts = %+v", list)
	}
	if res, text := mcptooltest.Call(t, cs, "create_contract_category_draft", args); !res.IsError || !strings.HasPrefix(text, mcptool.CodeAlreadyPending) {
		t.Fatalf("duplicate: %q", text)
	}
	bad := map[string]any{"title": "t", "input": map[string]any{"name": "  "}}
	if res, _ := mcptooltest.Call(t, cs, "create_contract_category_draft", bad); !res.IsError {
		t.Fatal("empty name accepted")
	}
	if cats, _ := e.cats.List(t.Context(), userA, ModuleID); len(cats) != 0 {
		t.Fatal("a draft tool created a category")
	}
}

func TestContractToolsHiddenWhenModuleDisabled(t *testing.T) {
	e := newToolEnv(t)
	cat := e.category(t, userA, ModuleID, "Insurance")
	e.disable(t, userA)
	cs := e.Connect(t, userA, mcptool.ScopeRead, mcptool.ScopeDrafts, mcptool.ScopeLinks)

	if names := mcptooltest.ToolNames(t, cs); len(names) != 0 {
		t.Fatalf("tools = %v", names)
	}
	res, text := mcptooltest.Call(t, cs, "create_contract_draft", contractArgs(cat, nil))
	if !res.IsError || !strings.HasPrefix(text, mcptool.CodeModuleDisabled) {
		t.Fatalf("call: %q", text)
	}
	res, text = mcptooltest.Call(t, cs, "search_contracts", map[string]any{})
	if !res.IsError || !strings.HasPrefix(text, mcptool.CodeModuleDisabled) {
		t.Fatalf("search: %q", text)
	}
	if n := len(e.pending(t, userA)); n != 0 {
		t.Fatalf("%d drafts stored", n)
	}
	// userB is unaffected
	if names := mcptooltest.ToolNames(t, e.Connect(t, userB, mcptool.ScopeRead)); len(names) != 2 {
		t.Fatalf("user B tools = %v", names)
	}
}

func TestContractDraftToolNeedsDraftsScope(t *testing.T) {
	e := newToolEnv(t)
	cat := e.category(t, userA, ModuleID, "Insurance")
	cs := e.Connect(t, userA, mcptool.ScopeRead)

	if names := mcptooltest.ToolNames(t, cs); len(names) != 2 {
		t.Fatalf("read tools = %v", names)
	}
	res, text := mcptooltest.Call(t, cs, "create_contract_draft", contractArgs(cat, nil))
	if !res.IsError || !strings.HasPrefix(text, mcptool.CodeInsufficientScope) {
		t.Fatalf("call: %q", text)
	}
}
