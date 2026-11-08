package server

import (
	"encoding/json"
	"testing"

	"github.com/reusing-code/kontor/backend/internal/categories"
	"github.com/reusing-code/kontor/backend/internal/drafts"
	"github.com/reusing-code/kontor/backend/internal/extraction"
	"github.com/reusing-code/kontor/backend/internal/llm/llmtest"
	"github.com/reusing-code/kontor/backend/internal/modules/purchases"
	"github.com/reusing-code/kontor/backend/internal/paperless"
	"github.com/reusing-code/kontor/backend/internal/paperless/paperlesstest"
)

type decision struct {
	Draft    drafts.Draft `json:"draft"`
	Warnings []string     `json:"warnings"`
}

// TestIntegration_PaperlessPipeline runs the whole flow against a fake
// paperless and a fake LLM: tagged document -> draft -> entity created from
// the draft -> accept links the document and swaps the tags; a second
// document is rejected and leaves the inbox.
func TestIntegration_PaperlessPipeline(t *testing.T) {
	fake := paperlesstest.New(t)
	inbox := fake.AddTag("kontor-inbox")
	fake.AddTag("Steuer")
	fake.AddDocument(paperlesstest.Document{ID: 5, Title: "Rechnung Bohrmaschine", Content: "Hornbach Rechnung Bohrmaschine 129,99 EUR 01.10.2026", Tags: []int{inbox, 2}})
	fake.AddDocument(paperlesstest.Document{ID: 6, Title: "Brief", Content: "Sehr geehrte Damen und Herren", Tags: []int{inbox}})

	fakeLLM := llmtest.New()
	srv, _ := setupServerWithExtractor(t, fakeLLM)
	defer srv.Close()
	base := srv.URL

	resp := doJSON(t, "POST", base+"/api/v1/modules/purchases/categories", map[string]string{"name": "Tools"})
	expectStatus(t, resp, 201)
	cat := decode[categories.Category](t, resp)

	resp = doJSON(t, "POST", base+"/api/v1/paperless/pipeline/run", nil)
	expectStatus(t, resp, 409) // paperless not configured
	resp.Body.Close()

	resp = doJSON(t, "PUT", base+"/api/v1/paperless/config", map[string]string{"baseUrl": fake.URL, "token": "secret"})
	expectStatus(t, resp, 200)
	resp.Body.Close()

	resp = doJSON(t, "POST", base+"/api/v1/paperless/pipeline/run", nil)
	expectStatus(t, resp, 409) // pipeline off by default
	resp.Body.Close()

	resp = doJSON(t, "PUT", base+"/api/v1/paperless/pipeline", paperless.PipelineSettings{Enabled: true, InboxTag: "kontor-inbox", DoneTag: "kontor"})
	expectStatus(t, resp, 200)
	resp.Body.Close()

	fakeLLM.Push(
		llmtest.JSON(map[string]any{
			"kind": "purchase",
			"purchase": map[string]any{
				"categoryId": cat.ID.String(), "itemName": "Bohrmaschine", "brand": "", "articleNumber": "",
				"dealer": "Hornbach", "price": 129.99, "purchaseDate": "2026-10-01",
			},
			"contract": nil, "cost": nil,
		}),
		llmtest.JSON(map[string]any{"kind": "none", "purchase": nil, "contract": nil, "cost": nil}),
	)
	resp = doJSON(t, "POST", base+"/api/v1/paperless/pipeline/run", nil)
	expectStatus(t, resp, 200)
	run := decode[paperless.RunResult](t, resp)
	if run != (paperless.RunResult{Found: 2, Created: 2}) {
		t.Fatalf("run = %+v", run)
	}

	resp = doJSON(t, "GET", base+"/api/v1/drafts", nil)
	expectStatus(t, resp, 200)
	pending := decode[[]drafts.Draft](t, resp)
	if len(pending) != 2 {
		t.Fatalf("pending drafts = %+v", pending)
	}
	byRef := map[string]drafts.Draft{}
	for _, d := range pending {
		byRef[d.Source.Ref] = d
	}
	purchaseDraft, letterDraft := byRef["5"], byRef["6"]
	if purchaseDraft.TargetType != drafts.TargetPurchase || purchaseDraft.ParentID != cat.ID.String() {
		t.Fatalf("purchase draft = %+v", purchaseDraft)
	}
	if letterDraft.TargetType != drafts.TargetNone || letterDraft.Note != extraction.NoteUnclassified {
		t.Fatalf("letter draft = %+v", letterDraft)
	}
	if !fake.HasTag(5, "kontor-inbox") {
		t.Fatal("creating a draft must not touch the tags")
	}

	// The frontend creates the entity from the draft input through the
	// regular module endpoint, then accepts the draft.
	var input map[string]any
	if err := json.Unmarshal(purchaseDraft.Input, &input); err != nil {
		t.Fatal(err)
	}
	resp = doJSON(t, "POST", base+"/api/v1/categories/"+purchaseDraft.ParentID+"/purchases", input)
	expectStatus(t, resp, 201)
	purchase := decode[purchases.Purchase](t, resp)
	if purchase.ItemName != "Bohrmaschine" || purchase.Price == nil || *purchase.Price != 129.99 {
		t.Fatalf("purchase = %+v", purchase)
	}

	entityURL := "https://kontor.example/purchases/categories/" + cat.ID.String()
	resp = doJSON(t, "POST", base+"/api/v1/drafts/"+purchaseDraft.ID.String()+"/accept", drafts.AcceptInput{
		EntityType: drafts.TargetPurchase, EntityID: purchase.ID.String(), EntityURL: entityURL,
	})
	expectStatus(t, resp, 200)
	accepted := decode[decision](t, resp)
	if accepted.Draft.Status != drafts.StatusAccepted || len(accepted.Warnings) != 0 {
		t.Fatalf("accept = %+v", accepted)
	}

	resp = doJSON(t, "GET", base+"/api/v1/paperless/links/purchase/"+purchase.ID.String(), nil)
	expectStatus(t, resp, 200)
	links := decode[[]paperless.Link](t, resp)
	if len(links) != 1 || links[0].DocumentID != 5 || links[0].Title != "Rechnung Bohrmaschine" {
		t.Fatalf("links = %+v", links)
	}
	doc, _ := fake.Document(5)
	if len(doc.CustomFields) != 1 || doc.CustomFields[0].Value != entityURL {
		t.Errorf("back-link = %+v", doc.CustomFields)
	}
	if fake.HasTag(5, "kontor-inbox") || !fake.HasTag(5, "kontor") || !fake.HasTag(5, "Steuer") {
		t.Errorf("tags = %v, want inbox tag swapped for kontor and others kept", doc.Tags)
	}

	resp = doJSON(t, "POST", base+"/api/v1/drafts/"+letterDraft.ID.String()+"/reject", nil)
	expectStatus(t, resp, 200)
	rejected := decode[decision](t, resp)
	if rejected.Draft.Status != drafts.StatusRejected || len(rejected.Warnings) != 0 {
		t.Fatalf("reject = %+v", rejected)
	}
	if fake.HasTag(6, "kontor-inbox") || fake.HasTag(6, "kontor") {
		t.Error("reject must only remove the inbox tag")
	}

	resp = doJSON(t, "GET", base+"/api/v1/drafts", nil)
	expectStatus(t, resp, 200)
	if left := decode[[]drafts.Draft](t, resp); len(left) != 0 {
		t.Fatalf("pending after decisions = %+v", left)
	}

	// Nothing carries the inbox tag anymore, so another pass is a no-op.
	resp = doJSON(t, "POST", base+"/api/v1/paperless/pipeline/run", nil)
	expectStatus(t, resp, 200)
	if run := decode[paperless.RunResult](t, resp); run != (paperless.RunResult{}) {
		t.Errorf("idle run = %+v", run)
	}
	if n := len(fakeLLM.Requests()); n != 2 {
		t.Errorf("llm requests = %d, want 2", n)
	}
}

func TestIntegration_PaperlessPipelineNeedsLLM(t *testing.T) {
	srv := setupServer(t)
	defer srv.Close()
	resp := doJSON(t, "POST", srv.URL+"/api/v1/paperless/pipeline/run", nil)
	expectStatus(t, resp, 409)
	resp.Body.Close()
}
