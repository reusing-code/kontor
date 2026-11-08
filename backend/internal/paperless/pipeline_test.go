package paperless

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/reusing-code/kontor/backend/internal/cryptoutil"
	"github.com/reusing-code/kontor/backend/internal/drafts"
	"github.com/reusing-code/kontor/backend/internal/llm"
	"github.com/reusing-code/kontor/backend/internal/llm/llmtest"
	"github.com/reusing-code/kontor/backend/internal/paperless/paperlesstest"
	"github.com/reusing-code/kontor/backend/internal/storage"
)

const (
	purchaseCatID = "11111111-1111-1111-1111-111111111111"
	contractCatID = "22222222-2222-2222-2222-222222222222"
	vehicleID     = "33333333-3333-3333-3333-333333333333"
)

type pipelineEnv struct {
	fake     *paperlesstest.Server
	llm      *llmtest.Fake
	svc      *Service
	drafts   *drafts.Store
	pipeline *Pipeline
	enabled  map[string]bool
	inboxID  int
}

func newPipelineEnv(t *testing.T) *pipelineEnv {
	t.Helper()
	logger := slog.New(slog.DiscardHandler)
	e, err := storage.Open(t.TempDir(), logger)
	if err != nil {
		t.Fatalf("opening engine: %v", err)
	}
	t.Cleanup(func() { e.Close() })

	env := &pipelineEnv{
		fake:    paperlesstest.New(t),
		llm:     llmtest.New(),
		svc:     NewService(NewStore(e), logger, testEncryptionKey),
		drafts:  drafts.NewStore(e),
		enabled: map[string]bool{drafts.TargetPurchase: true, drafts.TargetContract: true, drafts.TargetCost: true},
	}
	key, _ := cryptoutil.NormalizeEncryptionKey(testEncryptionKey)
	token, err := cryptoutil.EncryptString("test-token", key)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := env.svc.store.PutConfig(ctx, testUserID, Config{BaseURL: env.fake.URL, EncryptedToken: token}); err != nil {
		t.Fatal(err)
	}
	settings := DefaultPipelineSettings()
	settings.Enabled = true
	if err := env.svc.store.PutPipelineSettings(ctx, testUserID, settings); err != nil {
		t.Fatal(err)
	}
	env.inboxID = env.fake.AddTag(DefaultInboxTag)

	deps := PipelineDeps{
		UserIDs: func(context.Context) ([]string, error) { return []string{testUserID}, nil },
		TargetEnabled: func(_ context.Context, _ string, target string) (bool, error) {
			return env.enabled[target], nil
		},
		Categories: func(_ context.Context, _ string, target string) ([]Option, error) {
			if target == drafts.TargetPurchase {
				return []Option{{ID: purchaseCatID, Name: "Tools"}}, nil
			}
			return []Option{{ID: contractCatID, Name: "Telecom"}}, nil
		},
		Vehicles: func(context.Context, string) ([]Option, error) {
			return []Option{{ID: vehicleID, Name: "Golf (VW Golf, B-XY 123)"}}, nil
		},
	}
	env.pipeline = NewPipeline(env.svc, env.drafts, env.llm, deps, time.Minute, logger)
	env.pipeline.now = func() time.Time { return time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC) }
	return env
}

func (env *pipelineEnv) addInboxDoc(id int, content string) {
	env.fake.AddDocument(paperlesstest.Document{ID: id, Title: "Doc", Content: content, Tags: []int{env.inboxID}})
}

func (env *pipelineEnv) run(t *testing.T) RunResult {
	t.Helper()
	res, err := env.pipeline.RunUser(context.Background(), testUserID)
	if err != nil {
		t.Fatalf("RunUser: %v", err)
	}
	return res
}

func (env *pipelineEnv) pending(t *testing.T) []drafts.Draft {
	t.Helper()
	list, err := env.drafts.List(context.Background(), testUserID, drafts.StatusPending)
	if err != nil {
		t.Fatal(err)
	}
	return list
}

func price(v float64) *float64 { return &v }

func TestPipeline_CreatesPurchaseDraft(t *testing.T) {
	env := newPipelineEnv(t)
	env.addInboxDoc(5, "Rechnung Bohrmaschine 129,99 EUR")
	env.fake.AddDocument(paperlesstest.Document{ID: 6, Title: "Untagged", Content: "x"})
	env.llm.Push(llmtest.JSON(classification{
		Kind: "purchase",
		Purchase: &purchaseExtract{
			CategoryID: purchaseCatID, ItemName: " Drill ", Dealer: "Hornbach",
			Price: price(129.99), PurchaseDate: "2026-10-01",
		},
	}))

	res := env.run(t)
	if res != (RunResult{Found: 1, Created: 1}) {
		t.Fatalf("result = %+v", res)
	}
	list := env.pending(t)
	if len(list) != 1 {
		t.Fatalf("drafts = %+v", list)
	}
	d := list[0]
	if d.TargetType != drafts.TargetPurchase || d.ParentID != purchaseCatID || d.Note != "" {
		t.Errorf("draft = %+v", d)
	}
	if d.Source.Kind != drafts.SourcePaperless || d.Source.Ref != "5" || d.Source.URL != env.fake.URL+"/documents/5/details" {
		t.Errorf("source = %+v", d.Source)
	}
	var input map[string]any
	json.Unmarshal(d.Input, &input)
	if input["itemName"] != "Drill" || input["price"] != 129.99 || input["purchaseDate"] != "2026-10-01" || input["dealer"] != "Hornbach" {
		t.Errorf("input = %s", d.Input)
	}

	reqs := env.llm.Requests()
	if len(reqs) != 1 {
		t.Fatalf("llm requests = %d", len(reqs))
	}
	req := reqs[0]
	if req.Operation != "paperless.classify" || !strings.Contains(req.System, purchaseCatID+": Tools") ||
		!strings.Contains(req.System, vehicleID) || !strings.Contains(req.System, "Today is 2026-11-08") ||
		!strings.Contains(req.Input, "Bohrmaschine") {
		t.Errorf("unexpected request: system=%q input=%q", req.System, req.Input)
	}
	if !env.fake.HasTag(5, DefaultInboxTag) {
		t.Error("draft creation must leave the tags unchanged")
	}

	if res := env.run(t); res != (RunResult{Found: 1, Skipped: 1}) {
		t.Errorf("second run = %+v, want skipped", res)
	}
	if len(env.llm.Requests()) != 1 {
		t.Error("deduped document must not be classified again")
	}
}

func TestPipeline_ValidatesModelOutput(t *testing.T) {
	env := newPipelineEnv(t)
	env.addInboxDoc(1, "contract")
	env.addInboxDoc(2, "cost")
	env.llm.Push(
		llmtest.JSON(classification{Kind: "contract", Contract: &contractExtract{
			CategoryID: "99999999-9999-9999-9999-999999999999", Name: "Phone", BillingInterval: "weekly",
			StartDate: "01.02.2026", NoticePeriodMonths: -1, CustomerPortalURL: "not a url",
		}}),
		llmtest.JSON(classification{Kind: "cost", Cost: &costExtract{VehicleID: vehicleID, Type: "mileage", Date: "2026-09-30", Amount: price(80)}}),
	)
	env.run(t)

	byRef := map[string]drafts.Draft{}
	for _, d := range env.pending(t) {
		byRef[d.Source.Ref] = d
	}
	contract := byRef["1"]
	if contract.TargetType != drafts.TargetContract || contract.ParentID != "" {
		t.Errorf("unknown category must be dropped: %+v", contract)
	}
	if string(contract.Input) != `{"name":"Phone","minimumDurationMonths":0,"extensionDurationMonths":0,"noticePeriodMonths":0}` {
		t.Errorf("contract input = %s", contract.Input)
	}
	cost := byRef["2"]
	if cost.TargetType != drafts.TargetCost || cost.ParentID != vehicleID {
		t.Errorf("cost = %+v", cost)
	}
	if string(cost.Input) != `{"type":"misc","amount":80,"date":"2026-09-30"}` {
		t.Errorf("mileage without mileage must become misc: %s", cost.Input)
	}
}

func TestPipeline_UnclassifiableBecomesNoneDraft(t *testing.T) {
	env := newPipelineEnv(t)
	env.addInboxDoc(1, "letter")
	env.addInboxDoc(2, "garbage")
	env.addInboxDoc(3, "   ")
	env.addInboxDoc(4, "purchase without object")
	env.llm.Push(
		llmtest.JSON(classification{Kind: "none"}),
		llmtest.Error(llm.ErrInvalidOutput),
		llmtest.JSON(classification{Kind: "purchase"}),
	)
	res := env.run(t)
	if res.Created != 4 {
		t.Fatalf("result = %+v", res)
	}
	notes := map[string]string{}
	for _, d := range env.pending(t) {
		if d.TargetType != drafts.TargetNone {
			t.Errorf("draft %s target = %s, want none", d.Source.Ref, d.TargetType)
		}
		notes[d.Source.Ref] = d.Note
	}
	want := map[string]string{"1": NoteUnclassified, "2": NoteLLMInvalidOutput, "3": NoteEmptyContent, "4": NoteLLMInvalidOutput}
	for ref, note := range want {
		if notes[ref] != note {
			t.Errorf("doc %s note = %q, want %q", ref, notes[ref], note)
		}
	}
	if len(env.llm.Requests()) != 3 {
		t.Errorf("empty content must not reach the LLM; requests = %d", len(env.llm.Requests()))
	}
}

func TestPipeline_TransientErrorStopsPassWithoutDraft(t *testing.T) {
	env := newPipelineEnv(t)
	env.addInboxDoc(1, "a")
	env.addInboxDoc(2, "b")
	env.llm.Push(llmtest.Error(&llm.APIError{StatusCode: 503}))

	res := env.run(t)
	if res != (RunResult{Found: 2, Failed: 1}) {
		t.Fatalf("result = %+v", res)
	}
	if len(env.pending(t)) != 0 || len(env.llm.Requests()) != 1 {
		t.Fatalf("no draft and one attempt expected")
	}

	env.llm.Push(llmtest.JSON(classification{Kind: "none"}), llmtest.JSON(classification{Kind: "none"}))
	if res := env.run(t); res.Created != 2 {
		t.Errorf("retry run = %+v", res)
	}
}

func TestPipeline_CapsDraftsPerRun(t *testing.T) {
	env := newPipelineEnv(t)
	for i := 1; i <= 12; i++ {
		env.addInboxDoc(i, "doc")
	}
	env.llm.Func = func(llm.Request) (json.RawMessage, error) {
		return json.RawMessage(`{"kind":"none","purchase":null,"contract":null,"cost":null}`), nil
	}
	if res := env.run(t); res.Created != maxDraftsPerRun || res.Found != 12 {
		t.Fatalf("first run = %+v", res)
	}
	if res := env.run(t); res.Created != 2 || res.Skipped != maxDraftsPerRun {
		t.Fatalf("second run = %+v", res)
	}
}

func TestPipeline_OnlyOffersEnabledModules(t *testing.T) {
	env := newPipelineEnv(t)
	env.enabled[drafts.TargetPurchase] = false
	env.enabled[drafts.TargetCost] = false
	env.addInboxDoc(1, "doc")
	env.llm.Push(llmtest.JSON(classification{Kind: "purchase", Purchase: &purchaseExtract{ItemName: "Drill"}}))

	env.run(t)
	req := env.llm.Requests()[0]
	if strings.Contains(req.System, "Purchase categories") || strings.Contains(req.System, "Vehicles") || !strings.Contains(req.System, "Contract categories") {
		t.Errorf("system prompt offers disabled modules: %q", req.System)
	}
	d := env.pending(t)[0]
	if d.TargetType != drafts.TargetNone || d.Note != NoteUnclassified {
		t.Errorf("disabled kind must become none: %+v", d)
	}
}

func TestPipeline_Preconditions(t *testing.T) {
	env := newPipelineEnv(t)
	ctx := context.Background()

	for _, target := range []string{drafts.TargetPurchase, drafts.TargetContract, drafts.TargetCost} {
		env.enabled[target] = false
	}
	if _, err := env.pipeline.RunUser(ctx, testUserID); !errors.Is(err, ErrNoTargets) {
		t.Errorf("no targets: err = %v", err)
	}
	env.enabled[drafts.TargetPurchase] = true

	if err := env.svc.store.PutPipelineSettings(ctx, testUserID, DefaultPipelineSettings()); err != nil {
		t.Fatal(err)
	}
	if _, err := env.pipeline.RunUser(ctx, testUserID); !errors.Is(err, ErrPipelineDisabled) {
		t.Errorf("disabled: err = %v", err)
	}

	if _, err := env.pipeline.RunUser(ctx, "other-user"); !errors.Is(err, ErrPipelineDisabled) {
		t.Errorf("other user without settings: err = %v", err)
	}

	settings := DefaultPipelineSettings()
	settings.Enabled = true
	settings.InboxTag = "missing-tag"
	if err := env.svc.store.PutPipelineSettings(ctx, testUserID, settings); err != nil {
		t.Fatal(err)
	}
	env.addInboxDoc(1, "doc")
	if res := env.run(t); res != (RunResult{}) {
		t.Errorf("missing inbox tag: result = %+v", res)
	}
	if _, found, _ := NewClient(env.fake.URL, "t").FindTag(ctx, "missing-tag"); found {
		t.Error("the pipeline must not create the inbox tag")
	}
}

func TestDraftHooks_AcceptLinksAndSwapsTags(t *testing.T) {
	env := newPipelineEnv(t)
	ctx := context.Background()
	env.fake.AddDocument(paperlesstest.Document{ID: 9, Title: "Invoice", Tags: []int{env.inboxID, 77}})
	d, err := drafts.New(drafts.Source{Kind: drafts.SourcePaperless, Ref: "9", Title: "Invoice"}, drafts.TargetPurchase, "", nil, "")
	if err != nil {
		t.Fatal(err)
	}
	hooks := NewDraftHooks(env.svc)
	entityID := "44444444-4444-4444-4444-444444444444"
	entityURL := "https://kontor.example/purchases/categories/x"

	warnings, err := hooks.OnAccept(ctx, testUserID, d, drafts.AcceptInput{EntityType: drafts.TargetPurchase, EntityID: entityID, EntityURL: entityURL})
	if err != nil || len(warnings) != 0 {
		t.Fatalf("OnAccept = %v, %v", warnings, err)
	}
	links, _ := env.svc.store.ListLinks(ctx, testUserID, EntityPurchase, entityID)
	if len(links) != 1 || links[0].DocumentID != 9 || links[0].Title != "Invoice" {
		t.Errorf("links = %+v", links)
	}
	doc, _ := env.fake.Document(9)
	if len(doc.CustomFields) != 1 || doc.CustomFields[0].Value != entityURL {
		t.Errorf("back-link = %+v", doc.CustomFields)
	}
	if env.fake.HasTag(9, DefaultInboxTag) || !env.fake.HasTag(9, DefaultDoneTag) || len(doc.Tags) != 2 {
		t.Errorf("tags = %v, want inbox removed, done added, others kept", doc.Tags)
	}
}

func TestDraftHooks_AcceptStoresLinkWhenPaperlessDown(t *testing.T) {
	env := newPipelineEnv(t)
	ctx := context.Background()
	env.fake.FailAll(500)
	d, _ := drafts.New(drafts.Source{Kind: drafts.SourcePaperless, Ref: "9"}, drafts.TargetCost, "", nil, "")
	entityID := "44444444-4444-4444-4444-444444444444"

	warnings, err := NewDraftHooks(env.svc).OnAccept(ctx, testUserID, d, drafts.AcceptInput{EntityType: drafts.TargetCost, EntityID: entityID, EntityURL: "https://k.example/x"})
	if err != nil {
		t.Fatalf("OnAccept: %v", err)
	}
	if len(warnings) == 0 {
		t.Error("expected warnings when paperless is down")
	}
	if links, _ := env.svc.store.ListLinks(ctx, testUserID, EntityCost, entityID); len(links) != 1 {
		t.Errorf("link must be stored: %+v", links)
	}
}

func TestDraftHooks_RejectRemovesInboxTag(t *testing.T) {
	env := newPipelineEnv(t)
	env.fake.AddDocument(paperlesstest.Document{ID: 9, Tags: []int{env.inboxID}})
	d, _ := drafts.New(drafts.Source{Kind: drafts.SourcePaperless, Ref: "9"}, drafts.TargetNone, "", nil, "")

	if warnings := NewDraftHooks(env.svc).OnReject(context.Background(), testUserID, d); len(warnings) != 0 {
		t.Fatalf("warnings = %v", warnings)
	}
	if env.fake.HasTag(9, DefaultInboxTag) || env.fake.HasTag(9, DefaultDoneTag) {
		t.Error("reject must remove the inbox tag and not add the done tag")
	}
}

func TestClassificationSchemaIsValid(t *testing.T) {
	var def map[string]any
	if err := json.Unmarshal(classificationSchema.Definition, &def); err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(classificationSchema.Definition), `"null"`) > 16 {
		t.Error("schema exceeds 16 nullable fields")
	}
}
