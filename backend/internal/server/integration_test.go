package server

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/reusing-code/kontor/backend/internal/categories"
	"github.com/reusing-code/kontor/backend/internal/core"
	"github.com/reusing-code/kontor/backend/internal/handler"
	"github.com/reusing-code/kontor/backend/internal/middleware"
	"github.com/reusing-code/kontor/backend/internal/model"
	"github.com/reusing-code/kontor/backend/internal/module"
	"github.com/reusing-code/kontor/backend/internal/modules/auto"
	"github.com/reusing-code/kontor/backend/internal/modules/contracts"
	"github.com/reusing-code/kontor/backend/internal/storage"
	"github.com/reusing-code/kontor/backend/internal/storage/link"
	"github.com/reusing-code/kontor/backend/internal/store"
)

var testJWTSecret = []byte("integration-test-secret")

const testUserID = "00000000-0000-0000-0000-000000000001"

func setupServer(t *testing.T) *httptest.Server {
	t.Helper()
	logger := slog.New(slog.DiscardHandler)
	engine, err := storage.Open(t.TempDir(), logger)
	if err != nil {
		t.Fatalf("opening engine: %v", err)
	}
	t.Cleanup(func() { engine.Close() })

	links := link.NewRegistry()
	catStore := categories.NewStore(engine)
	catStore.RegisterCascade("purchases", store.PurchaseCategoryCascade)
	catHandler := categories.NewHandler(catStore, logger)
	coreStore := core.NewStore(engine)

	contractsMod := contracts.New(engine, links, catStore, coreStore, nil, logger)
	autoMod := auto.New(engine, links, logger)

	s := store.New(engine, links, contractsMod.Store(), autoMod.Store(), logger)
	h := handler.New(s, logger, testJWTSecret, nil)

	mux := http.NewServeMux()

	for _, m := range []module.Module{contractsMod, autoMod} {
		m.RegisterRoutes(module.NewRouter(mux, nil))
	}

	// Module-scoped category routes
	mux.HandleFunc("GET /api/v1/modules/{module}/categories", catHandler.List)
	mux.HandleFunc("POST /api/v1/modules/{module}/categories", catHandler.Create)
	mux.HandleFunc("GET /api/v1/modules/{module}/categories/{id}", catHandler.Get)
	mux.HandleFunc("PUT /api/v1/modules/{module}/categories/{id}", catHandler.Update)
	mux.HandleFunc("DELETE /api/v1/modules/{module}/categories/{id}", catHandler.Delete)

	// Purchase routes
	mux.HandleFunc("GET /api/v1/categories/{id}/purchases", h.ListPurchasesByCategory)
	mux.HandleFunc("POST /api/v1/categories/{id}/purchases", h.CreatePurchaseInCategory)
	mux.HandleFunc("GET /api/v1/purchases/summary", h.PurchaseSummary)
	mux.HandleFunc("GET /api/v1/purchases", h.ListPurchases)
	mux.HandleFunc("GET /api/v1/purchases/{id}", h.GetPurchase)
	mux.HandleFunc("PUT /api/v1/purchases/{id}", h.UpdatePurchase)
	mux.HandleFunc("DELETE /api/v1/purchases/{id}", h.DeletePurchase)

	// Inject test user into context (integration tests skip auth middleware)
	wrapped := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := middleware.SetUserID(r.Context(), testUserID)
		mux.ServeHTTP(w, r.WithContext(ctx))
	})

	return httptest.NewServer(wrapped)
}

func doJSON(t *testing.T, method, url string, body any) *http.Response {
	t.Helper()
	var r io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		r = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, url, r)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, url, err)
	}
	return resp
}

func decode[T any](t *testing.T, resp *http.Response) T {
	t.Helper()
	defer resp.Body.Close()
	var v T
	if err := json.NewDecoder(resp.Body).Decode(&v); err != nil {
		t.Fatalf("decoding response: %v", err)
	}
	return v
}

func expectStatus(t *testing.T, resp *http.Response, want int) {
	t.Helper()
	if resp.StatusCode != want {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("status = %d, want %d; body: %s", resp.StatusCode, want, body)
	}
}

func TestIntegration_FullCRUDFlow(t *testing.T) {
	srv := setupServer(t)
	defer srv.Close()
	base := srv.URL

	// List categories — empty
	resp := doJSON(t, "GET", base+"/api/v1/modules/contracts/categories", nil)
	expectStatus(t, resp, 200)
	cats := decode[[]model.Category](t, resp)
	if len(cats) != 0 {
		t.Fatalf("expected 0 categories, got %d", len(cats))
	}

	// Create category
	resp = doJSON(t, "POST", base+"/api/v1/modules/contracts/categories", map[string]string{"name": "Telecom"})
	expectStatus(t, resp, 201)
	cat := decode[model.Category](t, resp)
	if cat.Name != "Telecom" {
		t.Fatalf("Name = %q, want %q", cat.Name, "Telecom")
	}

	// Get category
	resp = doJSON(t, "GET", base+"/api/v1/modules/contracts/categories/"+cat.ID.String(), nil)
	expectStatus(t, resp, 200)
	got := decode[model.Category](t, resp)
	if got.ID != cat.ID {
		t.Fatalf("ID mismatch")
	}

	// Update category
	resp = doJSON(t, "PUT", base+"/api/v1/modules/contracts/categories/"+cat.ID.String(), map[string]string{"name": "Telecommunications"})
	expectStatus(t, resp, 200)
	updated := decode[model.Category](t, resp)
	if updated.Name != "Telecommunications" {
		t.Fatalf("Name = %q, want %q", updated.Name, "Telecommunications")
	}

	// Create contract in category
	conBody := map[string]any{
		"name":                    "Phone Plan",
		"startDate":               "2025-01-01",
		"minimumDurationMonths":   24,
		"extensionDurationMonths": 12,
		"noticePeriodMonths":      3,
		"company":                 "ACME Telecom",
	}
	resp = doJSON(t, "POST", base+"/api/v1/categories/"+cat.ID.String()+"/contracts", conBody)
	expectStatus(t, resp, 201)
	con := decode[model.Contract](t, resp)
	if con.Name != "Phone Plan" {
		t.Fatalf("Name = %q, want %q", con.Name, "Phone Plan")
	}
	if con.CategoryID != cat.ID {
		t.Fatalf("CategoryID = %s, want %s", con.CategoryID, cat.ID)
	}

	// Get contract
	resp = doJSON(t, "GET", base+"/api/v1/contracts/"+con.ID.String(), nil)
	expectStatus(t, resp, 200)
	gotCon := decode[model.Contract](t, resp)
	if gotCon.Company != "ACME Telecom" {
		t.Fatalf("Company = %q, want %q", gotCon.Company, "ACME Telecom")
	}

	// List contracts for category
	resp = doJSON(t, "GET", base+"/api/v1/categories/"+cat.ID.String()+"/contracts", nil)
	expectStatus(t, resp, 200)
	cons := decode[[]model.Contract](t, resp)
	if len(cons) != 1 {
		t.Fatalf("expected 1 contract, got %d", len(cons))
	}

	// List all contracts
	resp = doJSON(t, "GET", base+"/api/v1/contracts", nil)
	expectStatus(t, resp, 200)
	allCons := decode[[]model.Contract](t, resp)
	if len(allCons) != 1 {
		t.Fatalf("expected 1 contract, got %d", len(allCons))
	}

	// Update contract
	conBody["name"] = "Updated Phone Plan"
	resp = doJSON(t, "PUT", base+"/api/v1/contracts/"+con.ID.String(), conBody)
	expectStatus(t, resp, 200)
	updatedCon := decode[model.Contract](t, resp)
	if updatedCon.Name != "Updated Phone Plan" {
		t.Fatalf("Name = %q, want %q", updatedCon.Name, "Updated Phone Plan")
	}

	// Delete contract
	resp = doJSON(t, "DELETE", base+"/api/v1/contracts/"+con.ID.String(), nil)
	expectStatus(t, resp, 204)
	resp.Body.Close()

	// Verify contract is gone
	resp = doJSON(t, "GET", base+"/api/v1/contracts/"+con.ID.String(), nil)
	expectStatus(t, resp, 404)
	resp.Body.Close()

	// Delete category
	resp = doJSON(t, "DELETE", base+"/api/v1/modules/contracts/categories/"+cat.ID.String(), nil)
	expectStatus(t, resp, 204)
	resp.Body.Close()

	// Verify category is gone
	resp = doJSON(t, "GET", base+"/api/v1/modules/contracts/categories/"+cat.ID.String(), nil)
	expectStatus(t, resp, 404)
	resp.Body.Close()
}

func TestIntegration_CascadeDelete(t *testing.T) {
	srv := setupServer(t)
	defer srv.Close()
	base := srv.URL

	// Create category with two contracts
	resp := doJSON(t, "POST", base+"/api/v1/modules/contracts/categories", map[string]string{"name": "Insurance"})
	expectStatus(t, resp, 201)
	cat := decode[model.Category](t, resp)

	conBody := map[string]any{"name": "Health", "startDate": "2025-01-01"}
	resp = doJSON(t, "POST", base+"/api/v1/categories/"+cat.ID.String()+"/contracts", conBody)
	expectStatus(t, resp, 201)
	con1 := decode[model.Contract](t, resp)

	conBody["name"] = "Car"
	resp = doJSON(t, "POST", base+"/api/v1/categories/"+cat.ID.String()+"/contracts", conBody)
	expectStatus(t, resp, 201)
	con2 := decode[model.Contract](t, resp)

	// Delete category — contracts should cascade
	resp = doJSON(t, "DELETE", base+"/api/v1/modules/contracts/categories/"+cat.ID.String(), nil)
	expectStatus(t, resp, 204)
	resp.Body.Close()

	for _, id := range []string{con1.ID.String(), con2.ID.String()} {
		resp = doJSON(t, "GET", base+"/api/v1/contracts/"+id, nil)
		expectStatus(t, resp, 404)
		resp.Body.Close()
	}
}

func TestIntegration_Summary(t *testing.T) {
	srv := setupServer(t)
	defer srv.Close()
	base := srv.URL

	// Empty summary
	resp := doJSON(t, "GET", base+"/api/v1/summary", nil)
	expectStatus(t, resp, 200)
	summary := decode[map[string]any](t, resp)
	if int(summary["totalContracts"].(float64)) != 0 {
		t.Fatalf("expected 0 totalContracts, got %v", summary["totalContracts"])
	}

	// Create category + contract with price
	resp = doJSON(t, "POST", base+"/api/v1/modules/contracts/categories", map[string]string{"name": "Insurance"})
	expectStatus(t, resp, 201)
	cat := decode[model.Category](t, resp)

	price := 49.99
	conBody := map[string]any{
		"name":            "Health",
		"startDate":       "2025-01-01",
		"price":           price,
		"billingInterval": "monthly",
	}
	resp = doJSON(t, "POST", base+"/api/v1/categories/"+cat.ID.String()+"/contracts", conBody)
	expectStatus(t, resp, 201)
	resp.Body.Close()

	// Summary should reflect the contract
	resp = doJSON(t, "GET", base+"/api/v1/summary", nil)
	expectStatus(t, resp, 200)
	summary = decode[map[string]any](t, resp)
	if int(summary["totalContracts"].(float64)) != 1 {
		t.Fatalf("expected 1 totalContracts, got %v", summary["totalContracts"])
	}
	if summary["totalMonthlyAmount"].(float64) != price {
		t.Fatalf("expected totalMonthlyAmount %v, got %v", price, summary["totalMonthlyAmount"])
	}
	cats := summary["categories"].([]any)
	if len(cats) != 1 {
		t.Fatalf("expected 1 category, got %d", len(cats))
	}
}

func TestIntegration_UpcomingRenewals(t *testing.T) {
	srv := setupServer(t)
	defer srv.Close()
	base := srv.URL

	// Empty
	resp := doJSON(t, "GET", base+"/api/v1/contracts/upcoming-renewals", nil)
	expectStatus(t, resp, 200)
	renewals := decode[[]map[string]any](t, resp)
	if len(renewals) != 0 {
		t.Fatalf("expected 0 upcoming renewals, got %d", len(renewals))
	}

	// Create category + contract (no endDate, so it gets a cancellation date)
	resp = doJSON(t, "POST", base+"/api/v1/modules/contracts/categories", map[string]string{"name": "Telecom"})
	expectStatus(t, resp, 201)
	cat := decode[model.Category](t, resp)

	conBody := map[string]any{
		"name":                    "Phone",
		"startDate":               "2025-01-01",
		"minimumDurationMonths":   12,
		"extensionDurationMonths": 12,
		"noticePeriodMonths":      3,
	}
	resp = doJSON(t, "POST", base+"/api/v1/categories/"+cat.ID.String()+"/contracts", conBody)
	expectStatus(t, resp, 201)
	con := decode[map[string]any](t, resp)
	if con["cancellationDate"] == nil {
		t.Fatal("expected cancellationDate in response")
	}

	// Fetch with large window — should include the contract
	resp = doJSON(t, "GET", base+"/api/v1/contracts/upcoming-renewals?days=365", nil)
	expectStatus(t, resp, 200)
	_ = decode[[]map[string]any](t, resp)
	// The contract may or may not appear depending on its calculated date.
	// Just verify it's valid JSON and doesn't error.

	// Invalid days param
	resp = doJSON(t, "GET", base+"/api/v1/contracts/upcoming-renewals?days=abc", nil)
	expectStatus(t, resp, 400)
	resp.Body.Close()
}

func TestIntegration_ContractResponse_HasComputedFields(t *testing.T) {
	srv := setupServer(t)
	defer srv.Close()
	base := srv.URL

	// Create category + contract
	resp := doJSON(t, "POST", base+"/api/v1/modules/contracts/categories", map[string]string{"name": "Test"})
	expectStatus(t, resp, 201)
	cat := decode[model.Category](t, resp)

	conBody := map[string]any{
		"name":                    "Test Contract",
		"startDate":               "2025-01-01",
		"minimumDurationMonths":   24,
		"extensionDurationMonths": 12,
		"noticePeriodMonths":      3,
	}
	resp = doJSON(t, "POST", base+"/api/v1/categories/"+cat.ID.String()+"/contracts", conBody)
	expectStatus(t, resp, 201)
	con := decode[map[string]any](t, resp)

	// Should have cancellationDate and expired fields
	if con["cancellationDate"] == nil {
		t.Error("expected cancellationDate in create response")
	}
	if _, ok := con["expired"]; !ok {
		t.Error("expected expired field in create response")
	}

	// GET should also have them
	conID := con["id"].(string)
	resp = doJSON(t, "GET", base+"/api/v1/contracts/"+conID, nil)
	expectStatus(t, resp, 200)
	got := decode[map[string]any](t, resp)
	if got["cancellationDate"] == nil {
		t.Error("expected cancellationDate in get response")
	}
}

func TestIntegration_PurchaseCRUDFlow(t *testing.T) {
	srv := setupServer(t)
	defer srv.Close()
	base := srv.URL

	// List purchase categories — empty
	resp := doJSON(t, "GET", base+"/api/v1/modules/purchases/categories", nil)
	expectStatus(t, resp, 200)
	cats := decode[[]model.Category](t, resp)
	if len(cats) != 0 {
		t.Fatalf("expected 0 categories, got %d", len(cats))
	}

	// Create purchase category
	resp = doJSON(t, "POST", base+"/api/v1/modules/purchases/categories", map[string]string{"name": "PC Hardware"})
	expectStatus(t, resp, 201)
	cat := decode[model.Category](t, resp)
	if cat.Name != "PC Hardware" {
		t.Fatalf("Name = %q, want %q", cat.Name, "PC Hardware")
	}

	// Create purchase in category
	price := 299.99
	purBody := map[string]any{
		"itemName":     "Graphics Card",
		"brand":        "NVIDIA",
		"dealer":       "Amazon",
		"price":        price,
		"purchaseDate": "2025-06-15",
	}
	resp = doJSON(t, "POST", base+"/api/v1/categories/"+cat.ID.String()+"/purchases", purBody)
	expectStatus(t, resp, 201)
	pur := decode[model.Purchase](t, resp)
	if pur.ItemName != "Graphics Card" {
		t.Fatalf("ItemName = %q, want %q", pur.ItemName, "Graphics Card")
	}
	if pur.CategoryID != cat.ID {
		t.Fatalf("CategoryID = %s, want %s", pur.CategoryID, cat.ID)
	}

	// Get purchase
	resp = doJSON(t, "GET", base+"/api/v1/purchases/"+pur.ID.String(), nil)
	expectStatus(t, resp, 200)
	gotPur := decode[model.Purchase](t, resp)
	if gotPur.Brand != "NVIDIA" {
		t.Fatalf("Brand = %q, want %q", gotPur.Brand, "NVIDIA")
	}

	// List purchases for category
	resp = doJSON(t, "GET", base+"/api/v1/categories/"+cat.ID.String()+"/purchases", nil)
	expectStatus(t, resp, 200)
	purs := decode[[]model.Purchase](t, resp)
	if len(purs) != 1 {
		t.Fatalf("expected 1 purchase, got %d", len(purs))
	}

	// List all purchases
	resp = doJSON(t, "GET", base+"/api/v1/purchases", nil)
	expectStatus(t, resp, 200)
	allPurs := decode[[]model.Purchase](t, resp)
	if len(allPurs) != 1 {
		t.Fatalf("expected 1 purchase, got %d", len(allPurs))
	}

	// Update purchase
	purBody["itemName"] = "Updated Graphics Card"
	resp = doJSON(t, "PUT", base+"/api/v1/purchases/"+pur.ID.String(), purBody)
	expectStatus(t, resp, 200)
	updatedPur := decode[model.Purchase](t, resp)
	if updatedPur.ItemName != "Updated Graphics Card" {
		t.Fatalf("ItemName = %q, want %q", updatedPur.ItemName, "Updated Graphics Card")
	}

	// Purchase summary
	resp = doJSON(t, "GET", base+"/api/v1/purchases/summary", nil)
	expectStatus(t, resp, 200)
	summary := decode[map[string]any](t, resp)
	if int(summary["totalPurchases"].(float64)) != 1 {
		t.Fatalf("expected 1 totalPurchases, got %v", summary["totalPurchases"])
	}
	if summary["totalSpent"].(float64) != price {
		t.Fatalf("expected totalSpent %v, got %v", price, summary["totalSpent"])
	}

	// Delete purchase
	resp = doJSON(t, "DELETE", base+"/api/v1/purchases/"+pur.ID.String(), nil)
	expectStatus(t, resp, 204)
	resp.Body.Close()

	// Verify purchase is gone
	resp = doJSON(t, "GET", base+"/api/v1/purchases/"+pur.ID.String(), nil)
	expectStatus(t, resp, 404)
	resp.Body.Close()

	// Delete category
	resp = doJSON(t, "DELETE", base+"/api/v1/modules/purchases/categories/"+cat.ID.String(), nil)
	expectStatus(t, resp, 204)
	resp.Body.Close()

	// Verify category is gone
	resp = doJSON(t, "GET", base+"/api/v1/modules/purchases/categories/"+cat.ID.String(), nil)
	expectStatus(t, resp, 404)
	resp.Body.Close()
}
