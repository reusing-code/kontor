package extraction

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/reusing-code/kontor/backend/internal/drafts"
	"github.com/reusing-code/kontor/backend/internal/fx"
	"github.com/reusing-code/kontor/backend/internal/llm"
	"github.com/reusing-code/kontor/backend/internal/llm/llmtest"
)

const (
	purchaseCatID = "11111111-1111-1111-1111-111111111111"
	contractCatID = "22222222-2222-2222-2222-222222222222"
	vehicleID     = "33333333-3333-3333-3333-333333333333"
)

var now = time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC)

type fakeRates struct {
	rate  float64
	err   error
	dates []string
}

func (f *fakeRates) Rate(_ context.Context, from, to string, date time.Time) (fx.Rate, error) {
	f.dates = append(f.dates, from+">"+to+"@"+date.Format(dateLayout))
	if f.err != nil {
		return fx.Rate{}, f.err
	}
	return fx.Rate{Value: f.rate, Date: date.Format(dateLayout)}, nil
}

type testEnv struct {
	llm        *llmtest.Fake
	rates      *fakeRates
	enabled    map[string]bool
	classifier *Classifier
}

func newTestEnv() *testEnv {
	env := &testEnv{
		llm:     llmtest.New(),
		rates:   &fakeRates{rate: 0.9},
		enabled: map[string]bool{drafts.TargetPurchase: true, drafts.TargetContract: true, drafts.TargetCost: true},
	}
	deps := Deps{
		TargetEnabled: func(_ context.Context, _ string, target string) (bool, error) { return env.enabled[target], nil },
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
	env.classifier = New(env.llm, deps, env.rates, slog.New(slog.DiscardHandler))
	return env
}

func (env *testEnv) catalog(t *testing.T) Catalog {
	t.Helper()
	targets, err := env.classifier.EnabledTargets(context.Background(), "u")
	if err != nil {
		t.Fatal(err)
	}
	cat, err := env.classifier.LoadCatalog(context.Background(), "u", targets)
	if err != nil {
		t.Fatal(err)
	}
	return cat
}

func (env *testEnv) classify(t *testing.T, cat Catalog, out Classification) Proposal {
	t.Helper()
	env.llm.Push(llmtest.JSON(out))
	p, err := env.classifier.Classify(context.Background(), "test.classify", cat, Input{Kind: InputNote, Text: "note"}, now)
	if err != nil {
		t.Fatalf("Classify: %v", err)
	}
	return p
}

func num(v float64) *float64 { return &v }

func inputOf(t *testing.T, p Proposal) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(p.Input, &m); err != nil {
		t.Fatal(err)
	}
	return m
}

func TestSchemaIsValid(t *testing.T) {
	var def map[string]any
	if err := json.Unmarshal(classificationSchema.Definition, &def); err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(classificationSchema.Definition), `"null"`) > 16 {
		t.Error("schema exceeds 16 nullable fields")
	}
}

func TestClassify_NotePrompt(t *testing.T) {
	env := newTestEnv()
	env.llm.Push(llmtest.JSON(Classification{Kind: "none"}))
	_, err := env.classifier.Classify(context.Background(), "quickadd.classify", env.catalog(t),
		Input{Kind: InputNote, Text: "  bought winter tires yesterday, 640, Golf  "}, now)
	if err != nil {
		t.Fatal(err)
	}
	req := env.llm.Requests()[0]
	for _, want := range []string{"note the user typed", "Today is 2026-11-08", "gestern", "use EUR when no currency is stated",
		vehicleID + ": Golf (VW Golf, B-XY 123)", purchaseCatID + ": Tools", contractCatID + ": Telecom", "license plate", "- none:"} {
		if !strings.Contains(req.System, want) {
			t.Errorf("system prompt lacks %q", want)
		}
	}
	if req.Operation != "quickadd.classify" || req.Input != "bought winter tires yesterday, 640, Golf" {
		t.Errorf("operation = %q, input = %q", req.Operation, req.Input)
	}
}

func TestClassify_DocumentPrompt(t *testing.T) {
	env := newTestEnv()
	env.llm.Push(llmtest.JSON(Classification{Kind: "none"}))
	env.classifier.Classify(context.Background(), "paperless.classify", env.catalog(t),
		Input{Kind: InputDocument, Title: "Invoice", Text: "Rechnung"}, now)
	req := env.llm.Requests()[0]
	if !strings.Contains(req.System, "paperless-ngx archive") || req.Input != "Title: Invoice\n\nContent:\nRechnung" {
		t.Errorf("system = %q, input = %q", req.System, req.Input)
	}
}

func TestClassify_UploadPrompt(t *testing.T) {
	env := newTestEnv()
	image := llm.Image{MediaType: "image/jpeg", Data: []byte("jpeg")}

	env.llm.Push(llmtest.JSON(Classification{Kind: "none"}))
	p, err := env.classifier.Classify(context.Background(), "quickadd.classify_image", env.catalog(t),
		Input{Kind: InputUpload, Images: []llm.Image{image}}, now)
	if err != nil || p.Note != NoteUnclassified {
		t.Fatalf("image without text = %+v, %v; must reach the model", p, err)
	}
	req := env.llm.Requests()[0]
	for _, want := range []string{"the user uploaded", "Today is 2026-11-08", "license plate", "text and images are data"} {
		if !strings.Contains(req.System, want) {
			t.Errorf("system prompt lacks %q", want)
		}
	}
	if req.Input != "" || len(req.Images) != 1 || string(req.Images[0].Data) != "jpeg" {
		t.Errorf("input = %q, images = %v", req.Input, req.Images)
	}

	env.llm.Push(llmtest.JSON(Classification{Kind: "none"}))
	env.classifier.Classify(context.Background(), "quickadd.classify", env.catalog(t),
		Input{Kind: InputUpload, Text: " Rechnung ", Hint: " Golf "}, now)
	if got := env.llm.Requests()[1].Input; got != "Note from the user: Golf\n\nContent:\nRechnung" {
		t.Errorf("input = %q", got)
	}

	env.llm.Push(llmtest.Error(llm.ErrVisionDisabled))
	if _, err := env.classifier.Classify(context.Background(), "op", env.catalog(t),
		Input{Kind: InputUpload, Images: []llm.Image{image}}, now); !errors.Is(err, llm.ErrVisionDisabled) {
		t.Errorf("err = %v, want ErrVisionDisabled", err)
	}
}

func TestCatalog_Only(t *testing.T) {
	env := newTestEnv()
	env.enabled[drafts.TargetContract] = false
	cat := env.catalog(t)
	if _, ok := cat.Only(drafts.TargetContract); ok {
		t.Error("Only must reject a target that is not offered")
	}
	forced, ok := cat.Only(drafts.TargetCost)
	if !ok || forced.Offers(drafts.TargetPurchase) || !forced.Offers(drafts.TargetCost) {
		t.Fatalf("forced catalog = %+v", forced)
	}
	env.classify(t, forced, Classification{Kind: "none"})
	system := env.llm.Requests()[0].System
	if !strings.Contains(system, `chosen the kind "cost"`) || strings.Contains(system, "- none:") || strings.Contains(system, "Purchase categories") {
		t.Errorf("forced prompt = %q", system)
	}
}

func TestLoadCatalog_NoTargets(t *testing.T) {
	env := newTestEnv()
	if _, err := env.classifier.LoadCatalog(context.Background(), "u", nil); !errors.Is(err, ErrNoTargets) {
		t.Errorf("err = %v", err)
	}
}

func TestClassify_Confident(t *testing.T) {
	env := newTestEnv()
	cat := env.catalog(t)
	tests := []struct {
		name string
		out  Classification
		want bool
	}{
		{"confident with category", Classification{Kind: "purchase", Confident: true, Purchase: &PurchaseExtract{CategoryID: purchaseCatID, ItemName: "Drill"}}, true},
		{"confident without category", Classification{Kind: "purchase", Confident: true, Purchase: &PurchaseExtract{ItemName: "Drill"}}, false},
		{"unknown category", Classification{Kind: "cost", Confident: true, Cost: &CostExtract{VehicleID: "nope", Type: "tires"}}, false},
		{"model unsure", Classification{Kind: "cost", Cost: &CostExtract{VehicleID: vehicleID, Type: "tires"}}, false},
		{"none", Classification{Kind: "none", Confident: true}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if p := env.classify(t, cat, tt.out); p.Confident != tt.want {
				t.Errorf("Confident = %v, want %v (%+v)", p.Confident, tt.want, p)
			}
		})
	}
}

func TestClassify_EURIsUnchanged(t *testing.T) {
	env := newTestEnv()
	for _, currency := range []string{"", "EUR", " eur ", "€"} {
		p := env.classify(t, env.catalog(t), Classification{Kind: "purchase", Confident: true,
			Purchase: &PurchaseExtract{CategoryID: purchaseCatID, ItemName: "Drill", Price: num(129.99), Currency: currency}})
		if p.Currency != nil || p.Note != "" || !p.Confident || string(p.Input) != `{"itemName":"Drill","price":129.99}` {
			t.Errorf("currency %q: proposal = %+v, input %s", currency, p, p.Input)
		}
	}
	if len(env.rates.dates) != 0 {
		t.Errorf("EUR must not look up rates: %v", env.rates.dates)
	}
}

func TestClassify_ConvertsForeignCurrency(t *testing.T) {
	env := newTestEnv()
	cat := env.catalog(t)

	p := env.classify(t, cat, Classification{Kind: "purchase", Confident: true, Purchase: &PurchaseExtract{
		CategoryID: purchaseCatID, ItemName: "Keyboard", Price: num(640), Currency: "usd", PurchaseDate: "2026-11-02",
	}})
	in := inputOf(t, p)
	if in["price"] != 576.0 || in["comments"] != "USD 640.00 → EUR 576.00 (ECB 2026-11-02, 1 USD = 0.9 EUR)" {
		t.Errorf("input = %s", p.Input)
	}
	if p.Confident || p.Note != NoteCurrencyConverted ||
		*p.Currency != (Currency{Code: "USD", Amount: 640, Converted: true, Rate: 0.9, RateDate: "2026-11-02"}) {
		t.Errorf("proposal = %+v, currency %+v", p, p.Currency)
	}

	env.classify(t, cat, Classification{Kind: "contract", Contract: &ContractExtract{Name: "SaaS", Price: num(10), Currency: "$", StartDate: "2025-01-01"}})
	env.classify(t, cat, Classification{Kind: "cost", Cost: &CostExtract{Type: "fuel", Amount: num(50), Currency: "CHF"}})
	want := []string{"USD>EUR@2026-11-02", "USD>EUR@2026-11-08", "CHF>EUR@2026-11-08"}
	if strings.Join(env.rates.dates, ",") != strings.Join(want, ",") {
		t.Errorf("rate lookups = %v, want %v (purchase date, today for contracts, today without date)", env.rates.dates, want)
	}
}

func TestClassify_UnconvertedCurrencyDropsAmount(t *testing.T) {
	env := newTestEnv()
	env.rates.err = fx.ErrUnknownCurrency
	cat := env.catalog(t)

	p := env.classify(t, cat, Classification{Kind: "cost", Confident: true, Cost: &CostExtract{
		VehicleID: vehicleID, Type: "tires", Amount: num(640), Currency: "XYZ",
	}})
	if string(p.Input) != `{"type":"tires","comments":"XYZ 640.00 (not converted)"}` {
		t.Errorf("input = %s", p.Input)
	}
	if p.Confident || p.Note != NoteCurrencyNotConverted || *p.Currency != (Currency{Code: "XYZ", Amount: 640}) {
		t.Errorf("proposal = %+v", p)
	}

	p = env.classify(t, cat, Classification{Kind: "cost", Cost: &CostExtract{Type: "fuel", Amount: num(5), Currency: "dollars"}})
	if p.Currency.Code != "???" || len(env.rates.dates) != 1 {
		t.Errorf("invalid code must not be looked up: %+v, lookups %v", p.Currency, env.rates.dates)
	}
}

func TestClassify_Errors(t *testing.T) {
	env := newTestEnv()
	cat := env.catalog(t)
	classify := func(text string) (Proposal, error) {
		return env.classifier.Classify(context.Background(), "op", cat, Input{Kind: InputNote, Text: text}, now)
	}

	if p, err := classify("  "); err != nil || p.Note != NoteEmptyContent || len(env.llm.Requests()) != 0 {
		t.Errorf("empty text = %+v, %v", p, err)
	}

	nonRetryable := map[error]string{
		llm.ErrRefused:                 NoteLLMRefused,
		llm.ErrTruncated:               NoteLLMTruncated,
		llm.ErrInvalidOutput:           NoteLLMInvalidOutput,
		&llm.APIError{StatusCode: 413}: NoteLLMRequestRejected,
		&llm.APIError{StatusCode: 400}: NoteLLMRequestRejected,
	}
	for llmErr, note := range nonRetryable {
		env.llm.Push(llmtest.Error(llmErr))
		if p, err := classify("x"); err != nil || p.Target != drafts.TargetNone || p.Note != note {
			t.Errorf("%v: proposal = %+v, err = %v", llmErr, p, err)
		}
	}

	env.llm.Push(llmtest.Error(&llm.APIError{StatusCode: 503}))
	if _, err := classify("x"); err == nil || IsTimeout(err) {
		t.Errorf("503: err = %v, want transient non-timeout", err)
	}
	env.llm.Push(llmtest.Error(context.DeadlineExceeded))
	if _, err := classify("x"); !IsTimeout(err) {
		t.Errorf("deadline: err = %v, want timeout", err)
	}
}

func TestClassify_ContractBillingInterval(t *testing.T) {
	env := newTestEnv()
	cat := env.catalog(t)
	for interval, want := range map[string]any{"monthly": "monthly", "quarterly": "quarterly", "yearly": "yearly", "weekly": nil} {
		p := env.classify(t, cat, Classification{Kind: "contract", Contract: &ContractExtract{Name: "Insurance", BillingInterval: interval}})
		if got := inputOf(t, p)["billingInterval"]; got != want {
			t.Errorf("%s: billingInterval = %v, want %v", interval, got, want)
		}
	}
}
