package purchases

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
	mod := New(engine, link.NewRegistry(), cats, slog.New(slog.DiscardHandler))
	env := mcptooltest.New(t, engine, coreStore.ModuleEnabled)
	mod.RegisterTools(env.Reg)
	return &toolEnv{Env: env, mod: mod, cats: cats, coreStore: coreStore}
}

func (e *toolEnv) category(t *testing.T, user, module, name string) uuid.UUID {
	t.Helper()
	id := uuid.New()
	now := time.Now().UTC()
	if err := e.cats.Create(t.Context(), user, module, categories.Category{ID: id, Name: name, CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
	return id
}

func (e *toolEnv) purchase(t *testing.T, user string, catID uuid.UUID, item, dealer, date string) {
	t.Helper()
	now := time.Now().UTC()
	price := 20.0
	if err := e.mod.Store().Create(t.Context(), user, Purchase{
		ID: uuid.New(), CategoryID: catID, ItemName: item, Dealer: dealer, PurchaseDate: date, Price: &price, CreatedAt: now, UpdatedAt: now,
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

func purchaseArgs(catID uuid.UUID, mutate func(map[string]any)) map[string]any {
	args := map[string]any{
		"title": "hornbach.pdf", "key": "hornbach.pdf", "categoryId": catID.String(),
		"input": map[string]any{"itemName": "Drill", "dealer": "Hornbach", "price": 129.99, "purchaseDate": "2026-10-01"},
	}
	if mutate != nil {
		mutate(args)
	}
	return args
}

func TestListPurchaseCategories_OwnDataOnly(t *testing.T) {
	e := newToolEnv(t)
	own := e.category(t, userA, ModuleID, "Tools")
	e.category(t, userB, ModuleID, "Secret")
	e.category(t, userA, "contracts", "Insurance")

	res, _ := mcptooltest.Call(t, e.Connect(t, userA, mcptool.ScopeRead), "list_purchase_categories", map[string]any{})
	out := mcptooltest.Decode[listCategoriesOut](t, res)
	if len(out.Items) != 1 || out.Items[0].ID != own.String() {
		t.Fatalf("items = %+v", out.Items)
	}
}

func TestSearchPurchases(t *testing.T) {
	e := newToolEnv(t)
	tools := e.category(t, userA, ModuleID, "Tools")
	kitchen := e.category(t, userA, ModuleID, "Kitchen")
	e.purchase(t, userA, tools, "Drill", "Hornbach", "2026-10-01")
	e.purchase(t, userA, kitchen, "Blender", "Amazon", "2026-05-02")
	e.purchase(t, userA, kitchen, "Undated thing", "Shop", "")
	e.purchase(t, userB, tools, "Foreign drill", "Hornbach", "2026-10-01")
	cs := e.Connect(t, userA, mcptool.ScopeRead)

	search := func(args map[string]any) searchPurchasesOut {
		res, _ := mcptooltest.Call(t, cs, "search_purchases", args)
		return mcptooltest.Decode[searchPurchasesOut](t, res)
	}
	if out := search(map[string]any{}); len(out.Items) != 3 || out.Truncated {
		t.Fatalf("all = %+v", out)
	}
	if out := search(map[string]any{"query": "hornbach"}); len(out.Items) != 1 || out.Items[0].ItemName != "Drill" {
		t.Fatalf("query = %+v", out)
	}
	if out := search(map[string]any{"categoryId": kitchen.String()}); len(out.Items) != 2 {
		t.Fatalf("category = %+v", out)
	}
	if out := search(map[string]any{"dateFrom": "2026-09-01"}); len(out.Items) != 1 || out.Items[0].ItemName != "Drill" {
		t.Fatalf("dateFrom = %+v", out)
	}
	if out := search(map[string]any{"dateFrom": "2026-05-01", "dateTo": "2026-05-31"}); len(out.Items) != 1 || out.Items[0].ItemName != "Blender" {
		t.Fatalf("range = %+v", out)
	}
	if out := search(map[string]any{"limit": 2}); len(out.Items) != 2 || !out.Truncated {
		t.Fatalf("limit = %+v", out)
	}
	for _, bad := range []map[string]any{{"limit": 101}, {"dateFrom": "x"}, {"dateFrom": "2026-02-01", "dateTo": "2026-01-01"}} {
		if res, text := mcptooltest.Call(t, cs, "search_purchases", bad); !res.IsError || !strings.HasPrefix(text, mcptool.CodeInvalidInput) {
			t.Errorf("%v: %q", bad, text)
		}
	}
}

func TestCreatePurchaseDraft(t *testing.T) {
	e := newToolEnv(t)
	cat := e.category(t, userA, ModuleID, "Tools")
	cs := e.Connect(t, userA, mcptool.ScopeDrafts)

	res, text := mcptooltest.Call(t, cs, "create_purchase_draft", purchaseArgs(cat, nil))
	if res.IsError {
		t.Fatalf("error: %s", text)
	}
	list := e.drafts(t, userA)
	if len(list) != 1 {
		t.Fatalf("drafts = %+v", list)
	}
	d := list[0]
	var input PurchaseInput
	if err := json.Unmarshal(d.Input, &input); err != nil || input.ItemName != "Drill" || input.PurchaseDate != "2026-10-01" {
		t.Fatalf("input = %s", d.Input)
	}
	if d.TargetType != drafts.TargetPurchase || d.ParentID != cat.String() || d.Source.Kind != drafts.SourceMCP {
		t.Fatalf("draft = %+v", d)
	}
	if stored, _ := e.mod.Store().List(t.Context(), userA); len(stored) != 0 {
		t.Fatal("a draft tool created a purchase")
	}
	if res, text := mcptooltest.Call(t, cs, "create_purchase_draft", purchaseArgs(cat, nil)); !res.IsError || !strings.HasPrefix(text, mcptool.CodeAlreadyPending) {
		t.Fatalf("duplicate: %q", text)
	}
}

func TestCreatePurchaseDraft_InvalidInputStoresNothing(t *testing.T) {
	e := newToolEnv(t)
	cat := e.category(t, userA, ModuleID, "Tools")
	contractCat := e.category(t, userA, "contracts", "Insurance")
	cs := e.Connect(t, userA, mcptool.ScopeDrafts)

	cases := map[string]map[string]any{
		"missing item name": purchaseArgs(cat, func(a map[string]any) { a["input"].(map[string]any)["itemName"] = "" }),
		"bad date":          purchaseArgs(cat, func(a map[string]any) { a["input"].(map[string]any)["purchaseDate"] = "yesterday" }),
		"unknown category":  purchaseArgs(uuid.New(), nil),
		"contract category": purchaseArgs(contractCat, nil),
		"bad category id":   purchaseArgs(cat, func(a map[string]any) { a["categoryId"] = "x" }),
		"empty title":       purchaseArgs(cat, func(a map[string]any) { a["title"] = "" }),
	}
	for name, args := range cases {
		t.Run(name, func(t *testing.T) {
			if res, _ := mcptooltest.Call(t, cs, "create_purchase_draft", args); !res.IsError {
				t.Fatal("expected a tool error")
			}
		})
	}
	if n := len(e.drafts(t, userA)); n != 0 {
		t.Fatalf("%d drafts stored", n)
	}
}

func TestCreatePurchaseCategoryDraft(t *testing.T) {
	e := newToolEnv(t)
	cs := e.Connect(t, userA, mcptool.ScopeDrafts)
	args := map[string]any{"title": "cat", "key": "k", "input": map[string]any{"name": "Garden"}}
	if res, text := mcptooltest.Call(t, cs, "create_purchase_category_draft", args); res.IsError {
		t.Fatalf("error: %s", text)
	}
	list := e.drafts(t, userA)
	if len(list) != 1 || list[0].TargetType != drafts.TargetPurchaseCategory || list[0].ParentID != "" {
		t.Fatalf("drafts = %+v", list)
	}
	if res, text := mcptooltest.Call(t, cs, "create_purchase_category_draft", args); !res.IsError || !strings.HasPrefix(text, mcptool.CodeAlreadyPending) {
		t.Fatalf("duplicate: %q", text)
	}
	if res, _ := mcptooltest.Call(t, cs, "create_purchase_category_draft", map[string]any{"title": "t", "input": map[string]any{"name": ""}}); !res.IsError {
		t.Fatal("empty name accepted")
	}
	if cats, _ := e.cats.List(t.Context(), userA, ModuleID); len(cats) != 0 {
		t.Fatal("a draft tool created a category")
	}
}

func TestPurchaseToolsHiddenWhenModuleDisabled(t *testing.T) {
	e := newToolEnv(t)
	cat := e.category(t, userA, ModuleID, "Tools")
	st := core.DefaultUserSettings()
	st.DisabledModules = []string{ModuleID}
	if err := e.coreStore.UpdateSettings(t.Context(), userA, st); err != nil {
		t.Fatal(err)
	}
	cs := e.Connect(t, userA, mcptool.ScopeRead, mcptool.ScopeDrafts, mcptool.ScopeLinks)
	if names := mcptooltest.ToolNames(t, cs); len(names) != 0 {
		t.Fatalf("tools = %v", names)
	}
	if res, text := mcptooltest.Call(t, cs, "create_purchase_draft", purchaseArgs(cat, nil)); !res.IsError || !strings.HasPrefix(text, mcptool.CodeModuleDisabled) {
		t.Fatalf("call: %q", text)
	}
	if n := len(e.drafts(t, userA)); n != 0 {
		t.Fatalf("%d drafts stored", n)
	}
}

func TestPurchaseDraftToolNeedsDraftsScope(t *testing.T) {
	e := newToolEnv(t)
	cat := e.category(t, userA, ModuleID, "Tools")
	cs := e.Connect(t, userA, mcptool.ScopeRead)
	if res, text := mcptooltest.Call(t, cs, "create_purchase_draft", purchaseArgs(cat, nil)); !res.IsError || !strings.HasPrefix(text, mcptool.CodeInsufficientScope) {
		t.Fatalf("call: %q", text)
	}
}
