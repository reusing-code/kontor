package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/reusing-code/kontor/backend/internal/categories"
	"github.com/reusing-code/kontor/backend/internal/extraction"
	"github.com/reusing-code/kontor/backend/internal/fx"
	"github.com/reusing-code/kontor/backend/internal/llm/llmtest"
	"github.com/reusing-code/kontor/backend/internal/modules/purchases"
	"github.com/reusing-code/kontor/backend/internal/quickadd"
)

// TestIntegration_QuickAdd classifies a typed note with a foreign-currency
// amount and creates the purchase from the proposal through the module route.
func TestIntegration_QuickAdd(t *testing.T) {
	rates := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/2026-11-02" || r.URL.Query().Get("base") != "USD" {
			http.NotFound(w, r)
			return
		}
		w.Write([]byte(`{"amount":1.0,"base":"USD","date":"2026-11-02","rates":{"EUR":0.9}}`))
	}))
	defer rates.Close()

	fakeLLM := llmtest.New()
	srv, _ := setupServerWith(t, fakeLLM, fx.NewFrankfurter(rates.URL+"/v1"))
	defer srv.Close()
	base := srv.URL

	resp := doJSON(t, "POST", base+"/api/v1/modules/purchases/categories", map[string]string{"name": "Electronics"})
	expectStatus(t, resp, 201)
	cat := decode[categories.Category](t, resp)

	fakeLLM.Push(llmtest.JSON(extraction.Classification{Kind: "purchase", Confident: true, Purchase: &extraction.PurchaseExtract{
		CategoryID: cat.ID.String(), ItemName: "Keyboard", Dealer: "Keychron", Price: new(149.0), Currency: "USD", PurchaseDate: "2026-11-02",
	}}))
	resp = doJSON(t, "POST", base+"/api/v1/quick-add", quickadd.Request{Text: "keychron keyboard $149 on Nov 2"})
	expectStatus(t, resp, 200)
	proposal := decode[quickadd.Response](t, resp)
	if proposal.TargetType != "purchase" || proposal.ParentID != cat.ID.String() || proposal.Confident ||
		proposal.Note != extraction.NoteCurrencyConverted || proposal.Currency == nil || !proposal.Currency.Converted {
		t.Fatalf("proposal = %+v", proposal)
	}

	var input map[string]any
	if err := json.Unmarshal(proposal.Input, &input); err != nil {
		t.Fatal(err)
	}
	resp = doJSON(t, "POST", base+"/api/v1/categories/"+proposal.ParentID+"/purchases", input)
	expectStatus(t, resp, 201)
	created := decode[purchases.Purchase](t, resp)
	if created.ItemName != "Keyboard" || created.Price == nil || *created.Price != 134.1 ||
		created.Comments != "USD 149.00 → EUR 134.10 (ECB 2026-11-02, 1 USD = 0.9 EUR)" {
		t.Errorf("created = %+v", created)
	}
}

func TestIntegration_QuickAddNeedsLLM(t *testing.T) {
	srv := setupServer(t)
	defer srv.Close()
	resp := doJSON(t, "POST", srv.URL+"/api/v1/quick-add", quickadd.Request{Text: "x"})
	expectStatus(t, resp, 409)
	resp.Body.Close()
}
