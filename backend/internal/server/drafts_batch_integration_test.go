package server

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/reusing-code/kontor/backend/internal/drafts"
	"github.com/reusing-code/kontor/backend/internal/paperless/paperlesstest"
	"github.com/reusing-code/kontor/backend/internal/pat"
)

const batchOrigin = "https://kontor.example"

type batchUser struct {
	app    *testApp
	userID string
	jwt    string
	store  *drafts.Store
}

func newBatchUser(t *testing.T, a *testApp, email string) *batchUser {
	t.Helper()
	userID, jwt := a.register(t, email)
	return &batchUser{app: a, userID: userID, jwt: jwt, store: drafts.NewStore(a.engine)}
}

func (u *batchUser) draftFrom(t *testing.T, source drafts.Source, target, parent, input string) drafts.Draft {
	t.Helper()
	d, err := drafts.New(source, target, parent, json.RawMessage(input), "")
	if err != nil {
		t.Fatal(err)
	}
	if err := u.store.Create(t.Context(), u.userID, d); err != nil {
		t.Fatal(err)
	}
	return d
}

func (u *batchUser) draft(t *testing.T, target, parent, input string) drafts.Draft {
	t.Helper()
	return u.draftFrom(t, drafts.Source{Kind: drafts.SourceMCP, Ref: "r:" + uuid.NewString(), Title: "Draft"}, target, parent, input)
}

func (u *batchUser) accept(t *testing.T, list ...drafts.Draft) []drafts.BatchResult {
	t.Helper()
	ids := make([]string, len(list))
	for i, d := range list {
		ids[i] = d.ID.String()
	}
	body, _ := json.Marshal(map[string]any{"ids": ids, "baseUrl": batchOrigin})
	status, resp := u.app.request(t, "POST", "/api/v1/drafts/accept-batch", u.jwt, string(body))
	if status != http.StatusOK {
		t.Fatalf("accept-batch = %d %s", status, resp)
	}
	var out struct {
		Results []drafts.BatchResult `json:"results"`
	}
	if err := json.Unmarshal([]byte(resp), &out); err != nil {
		t.Fatal(err)
	}
	if len(out.Results) != len(list) {
		t.Fatalf("results = %d, want %d: %s", len(out.Results), len(list), resp)
	}
	return out.Results
}

// firstID returns the ID of the first item of a list endpoint.
func (u *batchUser) firstID(t *testing.T, path string) string {
	t.Helper()
	status, resp := u.app.request(t, "GET", path, u.jwt, "")
	var items []struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal([]byte(resp), &items); status != 200 || err != nil || len(items) == 0 {
		t.Fatalf("GET %s = %d %s", path, status, resp)
	}
	return items[0].ID
}

func (u *batchUser) count(t *testing.T, path string) int {
	t.Helper()
	status, resp := u.app.request(t, "GET", path, u.jwt, "")
	var items []json.RawMessage
	if err := json.Unmarshal([]byte(resp), &items); status != 200 || err != nil {
		t.Fatalf("GET %s = %d %s", path, status, resp)
	}
	return len(items)
}

func (u *batchUser) create(t *testing.T, path, body string) string {
	t.Helper()
	status, resp := u.app.request(t, "POST", path, u.jwt, body)
	var out struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal([]byte(resp), &out); status != http.StatusCreated || err != nil {
		t.Fatalf("POST %s = %d %s", path, status, resp)
	}
	return out.ID
}

func (u *batchUser) pending(t *testing.T, d drafts.Draft) bool {
	t.Helper()
	got, err := u.store.Get(t.Context(), u.userID, d.ID)
	if err != nil {
		t.Fatal(err)
	}
	return got.Status == drafts.StatusPending
}

func TestDraftBatchAcceptCreatesEveryTargetType(t *testing.T) {
	a := newTestApp(t)
	u := newBatchUser(t, a, "batch@example.com")
	purchaseCat := u.firstID(t, "/api/v1/modules/purchases/categories")
	contractCat := u.firstID(t, "/api/v1/modules/contracts/categories")
	vehicle := u.create(t, "/api/v1/vehicles", `{"name":"Van"}`)
	ledgerParent := u.create(t, "/api/v1/ledger/categories", `{"name":"Living","matchWords":[]}`)

	cases := []struct {
		target, parent, input string
		// get is the API path of the created entity, %s is its ID
		get  string
		want string
	}{
		{drafts.TargetPurchase, purchaseCat, `{"itemName":"Standing desk","price":499.9,"purchaseDate":"2026-04-02"}`, "/api/v1/purchases/%s", `"categoryId":"` + purchaseCat + `"`},
		{drafts.TargetContract, contractCat, `{"name":"Fiber 500","startDate":"2026-01-01"}`, "/api/v1/contracts/%s", `"billingInterval":"monthly"`},
		{drafts.TargetCost, vehicle, `{"type":"fuel","amount":61.5,"date":"2026-03-03"}`, "/api/v1/costs/%s", `"vehicleId":"` + vehicle + `"`},
		{drafts.TargetVehicle, "", `{"name":"Roadster"}`, "/api/v1/vehicles/%s", `"name":"Roadster"`},
		{drafts.TargetContractCategory, "", `{"name":"Streaming"}`, "/api/v1/modules/contracts/categories/%s", `"name":"Streaming"`},
		{drafts.TargetPurchaseCategory, "", `{"name":"Workshop"}`, "/api/v1/modules/purchases/categories/%s", `"name":"Workshop"`},
		{drafts.TargetLedgerCategory, ledgerParent, `{"name":"Rent","matchWords":["Miete"]}`, "/api/v1/ledger/categories/%s", `"parentId":"` + ledgerParent + `"`},
		{drafts.TargetLedgerAccount, "", `{"name":"Main","bank":"DKB","iban":"DE89 3704 0044 0532 0130 00"}`, "/api/v1/ledger/accounts/%s", `"iban":"DE89370400440532013000"`},
	}
	list := make([]drafts.Draft, len(cases))
	for i, c := range cases {
		list[i] = u.draft(t, c.target, c.parent, c.input)
	}

	counts := func() []int {
		return []int{
			u.count(t, "/api/v1/purchases"), u.count(t, "/api/v1/contracts"), u.count(t, "/api/v1/vehicles"),
			u.count(t, "/api/v1/vehicles/"+vehicle+"/costs"), u.count(t, "/api/v1/modules/contracts/categories"),
			u.count(t, "/api/v1/modules/purchases/categories"), u.count(t, "/api/v1/ledger/categories"), u.count(t, "/api/v1/ledger/accounts"),
		}
	}

	results := u.accept(t, list...)
	for i, c := range cases {
		res := results[i]
		wantID := drafts.EntityID(list[i].ID).String()
		if res.Status != drafts.BatchAccepted || res.EntityType != c.target || res.EntityID != wantID {
			t.Errorf("%s: result = %+v", c.target, res)
			continue
		}
		status, body := a.request(t, "GET", strings.Replace(c.get, "%s", wantID, 1), u.jwt, "")
		if status != 200 || !strings.Contains(body, c.want) {
			t.Errorf("%s: entity = %d %s, want %s", c.target, status, body, c.want)
		}
		if u.pending(t, list[i]) {
			t.Errorf("%s: draft is still pending", c.target)
		}
	}

	after := counts()
	repeat := u.accept(t, list...)
	for i, res := range repeat {
		if res.Status != drafts.BatchAlreadyAccepted || res.EntityID != results[i].EntityID {
			t.Errorf("repeat %s: result = %+v", cases[i].target, res)
		}
	}
	if got := counts(); !equalInts(got, after) {
		t.Errorf("entity counts after repeat = %v, want %v", got, after)
	}
}

func equalInts(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestDraftBatchAcceptFailuresMatchTheCreateEndpoints(t *testing.T) {
	a := newTestApp(t)
	u := newBatchUser(t, a, "fail@example.com")
	purchaseCat := u.firstID(t, "/api/v1/modules/purchases/categories")
	contractCat := u.firstID(t, "/api/v1/modules/contracts/categories")
	vehicle := u.create(t, "/api/v1/vehicles", `{"name":"Van"}`)
	u.create(t, "/api/v1/ledger/accounts", `{"name":"Main","bank":"DKB","iban":"DE89370400440532013000"}`)

	// The same body must fail with the same message on the create endpoint.
	invalid := []struct {
		target, parent, input, endpoint string
	}{
		{drafts.TargetPurchase, purchaseCat, `{"dealer":"Deskhaus"}`, "/api/v1/categories/" + purchaseCat + "/purchases"},
		{drafts.TargetContract, contractCat, `{"name":"Fiber","startDate":"2026-01-01","billingInterval":"weekly"}`, "/api/v1/categories/" + contractCat + "/contracts"},
		{drafts.TargetCost, vehicle, `{"type":"mileage","date":"2026-03-03"}`, "/api/v1/vehicles/" + vehicle + "/costs"},
		{drafts.TargetVehicle, "", `{"name":"Roadster","purchaseDate":"03.03.2026"}`, "/api/v1/vehicles"},
		{drafts.TargetContractCategory, "", `{"name":""}`, "/api/v1/modules/contracts/categories"},
		{drafts.TargetLedgerCategory, "", `{"name":" ","matchWords":[]}`, "/api/v1/ledger/categories"},
		{drafts.TargetLedgerAccount, "", `{"name":"Second","bank":"DKB","iban":"DE00"}`, "/api/v1/ledger/accounts"},
	}
	list := make([]drafts.Draft, len(invalid))
	for i, c := range invalid {
		list[i] = u.draft(t, c.target, c.parent, c.input)
	}
	results := u.accept(t, list...)
	for i, c := range invalid {
		status, body := a.request(t, "POST", c.endpoint, u.jwt, c.input)
		var apiErr struct {
			Error string `json:"error"`
		}
		_ = json.Unmarshal([]byte(body), &apiErr)
		if status != http.StatusBadRequest || apiErr.Error == "" {
			t.Fatalf("%s: create endpoint = %d %s, want 400", c.target, status, body)
		}
		res := results[i]
		if res.Status != drafts.BatchFailed || res.Code != drafts.CodeInvalidInput || res.Error != apiErr.Error {
			t.Errorf("%s: result = %+v, want invalid_input %q", c.target, res, apiErr.Error)
		}
		if !u.pending(t, list[i]) {
			t.Errorf("%s: draft must stay pending", c.target)
		}
	}

	other := []struct {
		name, target, parent, input, code string
	}{
		{"unknown field", drafts.TargetPurchase, purchaseCat, `{"itemName":"Desk","colour":"oak"}`, drafts.CodeInvalidInput},
		{"unknown category", drafts.TargetPurchase, uuid.NewString(), `{"itemName":"Desk"}`, drafts.CodeParentNotFound},
		{"category of another module", drafts.TargetContract, purchaseCat, `{"name":"Fiber","startDate":"2026-01-01"}`, drafts.CodeParentNotFound},
		{"unknown vehicle", drafts.TargetCost, uuid.NewString(), `{"type":"fuel","date":"2026-03-03"}`, drafts.CodeParentNotFound},
		{"unknown ledger parent", drafts.TargetLedgerCategory, uuid.NewString(), `{"name":"Rent","matchWords":[]}`, drafts.CodeParentNotFound},
		{"no category", drafts.TargetPurchase, "", `{"itemName":"Desk"}`, drafts.CodeParentMissing},
		{"duplicate iban", drafts.TargetLedgerAccount, "", `{"name":"Copy","bank":"DKB","iban":"DE89370400440532013000"}`, drafts.CodeConflict},
		{"no type", drafts.TargetNone, "", `{}`, drafts.CodeNoType},
	}
	list = list[:0]
	for _, c := range other {
		list = append(list, u.draft(t, c.target, c.parent, c.input))
	}
	for i, res := range u.accept(t, list...) {
		if res.Status != drafts.BatchFailed || res.Code != other[i].code || res.Error == "" {
			t.Errorf("%s: result = %+v, want %s", other[i].name, res, other[i].code)
		}
		if !u.pending(t, list[i]) {
			t.Errorf("%s: draft must stay pending", other[i].name)
		}
	}
	if n := u.count(t, "/api/v1/purchases") + u.count(t, "/api/v1/contracts") + u.count(t, "/api/v1/vehicles/"+vehicle+"/costs"); n != 0 {
		t.Errorf("%d entities were created from failing drafts", n)
	}
	if n := u.count(t, "/api/v1/ledger/accounts"); n != 1 {
		t.Errorf("ledger accounts = %d, want 1", n)
	}
}

func TestDraftBatchAcceptDisabledModuleAndIsolation(t *testing.T) {
	a := newTestApp(t)
	u := newBatchUser(t, a, "gate@example.com")
	stranger := newBatchUser(t, a, "stranger@example.com")
	purchaseCat := u.firstID(t, "/api/v1/modules/purchases/categories")
	vehicle := u.draft(t, drafts.TargetVehicle, "", `{"name":"Roadster"}`)
	purchase := u.draft(t, drafts.TargetPurchase, purchaseCat, `{"itemName":"Desk"}`)

	if res := stranger.accept(t, purchase); res[0].Code != drafts.CodeNotFound {
		t.Fatalf("other user's draft = %+v, want not_found", res[0])
	}

	status, resp := a.request(t, "PUT", "/api/v1/settings", u.jwt, `{"renewalDays":90,"reminderFrequency":"disabled","enabledModules":["contracts","purchases","ledger"]}`)
	if status != 200 {
		t.Fatalf("settings = %d %s", status, resp)
	}
	res := u.accept(t, vehicle, purchase)
	if res[0].Code != drafts.CodeModuleDisabled || !u.pending(t, vehicle) {
		t.Errorf("vehicle with auto disabled = %+v", res[0])
	}
	if res[1].Status != drafts.BatchAccepted {
		t.Errorf("purchase = %+v, a failing draft must not stop the batch", res[1])
	}
}

func TestDraftBatchAcceptIsJWTOnly(t *testing.T) {
	a := newTestApp(t)
	u := newBatchUser(t, a, "pat@example.com")
	_, tok := a.newPAT(t, u.userID, nil, pat.ScopeRead, pat.ScopeDrafts, pat.ScopeLinks)
	d := u.draft(t, drafts.TargetVehicle, "", `{"name":"Roadster"}`)

	body := `{"ids":["` + d.ID.String() + `"],"baseUrl":"` + batchOrigin + `"}`
	status, resp := a.request(t, "POST", "/api/v1/drafts/accept-batch", tok, body)
	if status != http.StatusForbidden || !strings.Contains(resp, "insufficient_scope") {
		t.Fatalf("pat accept-batch = %d %s, want 403", status, resp)
	}
	if !u.pending(t, d) || u.count(t, "/api/v1/vehicles") != 0 {
		t.Error("a token must not create entities from drafts")
	}
}

func TestDraftBatchAcceptLinksPaperlessSource(t *testing.T) {
	fake := paperlesstest.New(t)
	inbox := fake.AddTag("kontor-inbox")
	fake.AddDocument(paperlesstest.Document{ID: 7, Title: "Invoice", Tags: []int{inbox}})

	a := newTestApp(t)
	u := newBatchUser(t, a, "paperless@example.com")
	status, body := a.request(t, "PUT", "/api/v1/paperless/config", u.jwt, `{"baseUrl":"`+fake.URL+`","token":"secret"}`)
	if status != 200 {
		t.Fatalf("paperless config = %d %s", status, body)
	}
	purchaseCat := u.firstID(t, "/api/v1/modules/purchases/categories")
	d := u.draftFrom(t, drafts.Source{Kind: drafts.SourcePaperless, Ref: "7", Title: "Invoice"},
		drafts.TargetPurchase, purchaseCat, `{"itemName":"Standing desk"}`)

	res := u.accept(t, d)[0]
	if res.Status != drafts.BatchAccepted || len(res.Warnings) != 0 {
		t.Fatalf("result = %+v", res)
	}
	status, body = a.request(t, "GET", "/api/v1/paperless/links/purchase/"+res.EntityID, u.jwt, "")
	if status != 200 || !strings.Contains(body, `"documentId":7`) {
		t.Fatalf("links = %d %s", status, body)
	}
	doc, _ := fake.Document(7)
	if len(doc.CustomFields) != 1 || doc.CustomFields[0].Value != batchOrigin+"/purchases/"+res.EntityID {
		t.Errorf("back-link = %+v", doc.CustomFields)
	}
	if fake.HasTag(7, "kontor-inbox") || !fake.HasTag(7, "kontor") {
		t.Errorf("tags = %v, want the done tag instead of the inbox tag", doc.Tags)
	}
}

// A draft whose source hook fails stays pending although its entity exists;
// repeating must reuse that entity, also where a second create would be
// refused (the account's IBAN).
func TestDraftBatchAcceptRepeatAfterHookFailureReusesEntity(t *testing.T) {
	a := newTestApp(t)
	u := newBatchUser(t, a, "retry@example.com")
	purchaseCat := u.firstID(t, "/api/v1/modules/purchases/categories")
	// "x" is not a paperless document ID, so the accept hook fails.
	broken := func(ref string) drafts.Source {
		return drafts.Source{Kind: drafts.SourcePaperless, Ref: ref, Title: "Broken"}
	}
	purchase := u.draftFrom(t, broken("x"), drafts.TargetPurchase, purchaseCat, `{"itemName":"Desk"}`)
	account := u.draftFrom(t, broken("y"), drafts.TargetLedgerAccount, "", `{"name":"Main","bank":"DKB","iban":"DE89370400440532013000"}`)

	for attempt := range 2 {
		for i, res := range u.accept(t, purchase, account) {
			if res.Status != drafts.BatchFailed || res.Code != drafts.CodeSourceLinkFailed {
				t.Fatalf("attempt %d, draft %d = %+v, want source_link_failed", attempt, i, res)
			}
		}
		if !u.pending(t, purchase) || !u.pending(t, account) {
			t.Fatalf("attempt %d: drafts must stay pending", attempt)
		}
		if p, acc := u.count(t, "/api/v1/purchases"), u.count(t, "/api/v1/ledger/accounts"); p != 1 || acc != 1 {
			t.Fatalf("attempt %d: purchases = %d, accounts = %d, want 1 and 1", attempt, p, acc)
		}
	}
}
