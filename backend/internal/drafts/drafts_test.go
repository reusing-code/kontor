package drafts

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/reusing-code/kontor/backend/internal/middleware"
	"github.com/reusing-code/kontor/backend/internal/storage"
)

const testUserID = "00000000-0000-0000-0000-000000000001"

type fakeHooks struct {
	acceptErr error
	accepted  []AcceptInput
	rejected  []Draft
}

func (f *fakeHooks) OnAccept(_ context.Context, _ string, _ Draft, in AcceptInput) ([]string, error) {
	if f.acceptErr != nil {
		return nil, f.acceptErr
	}
	f.accepted = append(f.accepted, in)
	return []string{"accept warning"}, nil
}

func (f *fakeHooks) OnReject(_ context.Context, _ string, d Draft) []string {
	f.rejected = append(f.rejected, d)
	return nil
}

func setup(t *testing.T) (*Store, *fakeHooks, http.Handler) {
	t.Helper()
	e, err := storage.Open(t.TempDir(), slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatalf("opening engine: %v", err)
	}
	t.Cleanup(func() { e.Close() })
	store := NewStore(e)
	hooks := &fakeHooks{}
	h := NewHandler(store, slog.New(slog.DiscardHandler))
	h.RegisterSource(SourcePaperless, hooks)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/drafts", h.List)
	mux.HandleFunc("GET /api/v1/drafts/summary", h.Summary)
	mux.HandleFunc("GET /api/v1/drafts/{id}", h.Get)
	mux.HandleFunc("PUT /api/v1/drafts/{id}", h.Update)
	mux.HandleFunc("POST /api/v1/drafts/{id}/accept", h.Accept)
	mux.HandleFunc("POST /api/v1/drafts/{id}/reject", h.Reject)
	return store, hooks, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mux.ServeHTTP(w, r.WithContext(middleware.SetUserID(r.Context(), testUserID)))
	})
}

func do(t *testing.T, mux http.Handler, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		json.NewEncoder(&buf).Encode(body)
	}
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(method, path, &buf))
	return rec
}

func decode[T any](t *testing.T, rec *httptest.ResponseRecorder) T {
	t.Helper()
	var v T
	if err := json.NewDecoder(rec.Body).Decode(&v); err != nil {
		t.Fatalf("decoding: %v (%s)", err, rec.Body.String())
	}
	return v
}

func createDraft(t *testing.T, store *Store, ref, target string) Draft {
	t.Helper()
	d, err := New(Source{Kind: SourcePaperless, Ref: ref, Title: "Invoice"}, target, "", json.RawMessage(`{"itemName":"Drill"}`), "")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := store.Create(context.Background(), testUserID, d); err != nil {
		t.Fatalf("Create: %v", err)
	}
	return d
}

func acceptBody(entityType string) AcceptInput {
	return AcceptInput{EntityType: entityType, EntityID: uuid.NewString(), EntityURL: "https://kontor.example/purchases/x"}
}

func TestNew_Validation(t *testing.T) {
	src := Source{Kind: SourcePaperless, Ref: "1"}
	cases := []struct {
		name   string
		src    Source
		target string
		parent string
		input  string
	}{
		{"bad kind", Source{Kind: "fax", Ref: "1"}, TargetPurchase, "", `{}`},
		{"empty ref", Source{Kind: SourcePaperless}, TargetPurchase, "", `{}`},
		{"bad target", src, "car", "", `{}`},
		{"bad parent", src, TargetPurchase, "nope", `{}`},
		{"array input", src, TargetPurchase, "", `[]`},
	}
	for _, c := range cases {
		if _, err := New(c.src, c.target, c.parent, json.RawMessage(c.input), ""); err == nil {
			t.Errorf("%s: expected error", c.name)
		}
	}
	d, err := New(src, TargetNone, "", nil, "unclassified")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if string(d.Input) != "{}" || d.Status != StatusPending {
		t.Errorf("draft = %+v", d)
	}
}

func TestStore_DedupesBySourceUntilDecided(t *testing.T) {
	store, _, _ := setup(t)
	ctx := context.Background()
	d := createDraft(t, store, "42", TargetPurchase)

	dup, _ := New(Source{Kind: SourcePaperless, Ref: "42"}, TargetNone, "", nil, "")
	if err := store.Create(ctx, testUserID, dup); !errors.Is(err, ErrDuplicateSource) {
		t.Fatalf("duplicate Create err = %v, want ErrDuplicateSource", err)
	}
	if has, _ := store.HasSource(ctx, testUserID, SourcePaperless, "42"); !has {
		t.Fatal("HasSource = false for pending draft")
	}
	if _, err := store.Decide(ctx, testUserID, d.ID, StatusRejected, nil); err != nil {
		t.Fatalf("Decide: %v", err)
	}
	if has, _ := store.HasSource(ctx, testUserID, SourcePaperless, "42"); has {
		t.Fatal("deciding should free the source")
	}
	if err := store.Create(ctx, testUserID, dup); err != nil {
		t.Fatalf("Create after decision: %v", err)
	}
}

func TestHandler_ListFiltersByStatus(t *testing.T) {
	store, _, mux := setup(t)
	a := createDraft(t, store, "1", TargetPurchase)
	createDraft(t, store, "2", TargetPurchase)
	if _, err := store.Decide(context.Background(), testUserID, a.ID, StatusRejected, nil); err != nil {
		t.Fatal(err)
	}

	if got := decode[[]Draft](t, do(t, mux, "GET", "/api/v1/drafts", nil)); len(got) != 1 || got[0].Source.Ref != "2" {
		t.Errorf("default list = %+v, want only the pending draft", got)
	}
	if got := decode[[]Draft](t, do(t, mux, "GET", "/api/v1/drafts?status=rejected", nil)); len(got) != 1 || got[0].ID != a.ID {
		t.Errorf("rejected list = %+v", got)
	}
	if got := decode[[]Draft](t, do(t, mux, "GET", "/api/v1/drafts?status=all", nil)); len(got) != 2 {
		t.Errorf("all list has %d drafts", len(got))
	}
	if rec := do(t, mux, "GET", "/api/v1/drafts?status=bogus", nil); rec.Code != http.StatusBadRequest {
		t.Errorf("bogus status = %d", rec.Code)
	}
}

func TestHandler_UpdatePendingOnly(t *testing.T) {
	store, _, mux := setup(t)
	d := createDraft(t, store, "1", TargetNone)
	parent := uuid.NewString()

	rec := do(t, mux, "PUT", "/api/v1/drafts/"+d.ID.String(), map[string]any{
		"targetType": TargetContract, "parentId": parent, "input": map[string]any{"name": "Phone"},
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT status = %d: %s", rec.Code, rec.Body.String())
	}
	got := decode[Draft](t, rec)
	if got.TargetType != TargetContract || got.ParentID != parent || string(got.Input) != `{"name":"Phone"}` {
		t.Errorf("updated = %+v", got)
	}

	if rec := do(t, mux, "PUT", "/api/v1/drafts/"+d.ID.String(), map[string]any{"targetType": "x"}); rec.Code != http.StatusBadRequest {
		t.Errorf("invalid PUT status = %d", rec.Code)
	}
	if _, err := store.Decide(context.Background(), testUserID, d.ID, StatusRejected, nil); err != nil {
		t.Fatal(err)
	}
	if rec := do(t, mux, "PUT", "/api/v1/drafts/"+d.ID.String(), map[string]any{"targetType": TargetPurchase}); rec.Code != http.StatusConflict {
		t.Errorf("PUT on decided draft = %d, want 409", rec.Code)
	}
}

func TestHandler_AcceptRunsHookAndStoresResult(t *testing.T) {
	store, hooks, mux := setup(t)
	d := createDraft(t, store, "1", TargetPurchase)

	body := acceptBody(TargetPurchase)
	rec := do(t, mux, "POST", "/api/v1/drafts/"+d.ID.String()+"/accept", body)
	if rec.Code != http.StatusOK {
		t.Fatalf("accept status = %d: %s", rec.Code, rec.Body.String())
	}
	resp := decode[decisionResponse](t, rec)
	if resp.Draft.Status != StatusAccepted || resp.Draft.Result == nil || resp.Draft.Result.EntityID != body.EntityID || resp.Draft.DecidedAt == nil {
		t.Errorf("draft = %+v", resp.Draft)
	}
	if len(resp.Warnings) != 1 || len(hooks.accepted) != 1 {
		t.Errorf("warnings = %v, hook calls = %d", resp.Warnings, len(hooks.accepted))
	}

	if rec := do(t, mux, "POST", "/api/v1/drafts/"+d.ID.String()+"/accept", acceptBody(TargetPurchase)); rec.Code != http.StatusConflict {
		t.Errorf("second accept = %d, want 409", rec.Code)
	}
	if rec := do(t, mux, "POST", "/api/v1/drafts/"+d.ID.String()+"/reject", nil); rec.Code != http.StatusConflict {
		t.Errorf("reject after accept = %d, want 409", rec.Code)
	}
}

func TestHandler_AcceptValidation(t *testing.T) {
	store, hooks, mux := setup(t)
	d := createDraft(t, store, "1", TargetPurchase)
	none := createDraft(t, store, "2", TargetNone)

	cases := []struct {
		id   uuid.UUID
		body AcceptInput
	}{
		{d.ID, acceptBody(TargetContract)},
		{d.ID, AcceptInput{EntityType: TargetPurchase, EntityID: "x", EntityURL: "https://k.example/x"}},
		{d.ID, AcceptInput{EntityType: TargetPurchase, EntityID: uuid.NewString(), EntityURL: "/relative"}},
		{none.ID, acceptBody(TargetNone)},
	}
	for i, c := range cases {
		if rec := do(t, mux, "POST", "/api/v1/drafts/"+c.id.String()+"/accept", c.body); rec.Code != http.StatusBadRequest {
			t.Errorf("case %d: status = %d, want 400", i, rec.Code)
		}
	}
	if len(hooks.accepted) != 0 {
		t.Error("hook must not run for invalid input")
	}
}

func TestHandler_AcceptHookErrorKeepsDraftPending(t *testing.T) {
	store, hooks, mux := setup(t)
	hooks.acceptErr = errors.New("db down")
	d := createDraft(t, store, "1", TargetPurchase)

	if rec := do(t, mux, "POST", "/api/v1/drafts/"+d.ID.String()+"/accept", acceptBody(TargetPurchase)); rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
	got, _ := store.Get(context.Background(), testUserID, d.ID)
	if got.Status != StatusPending {
		t.Errorf("status = %s, want pending", got.Status)
	}
}

func TestHandler_Reject(t *testing.T) {
	store, hooks, mux := setup(t)
	d := createDraft(t, store, "1", TargetPurchase)

	rec := do(t, mux, "POST", "/api/v1/drafts/"+d.ID.String()+"/reject", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("reject status = %d: %s", rec.Code, rec.Body.String())
	}
	resp := decode[decisionResponse](t, rec)
	if resp.Draft.Status != StatusRejected || resp.Warnings == nil {
		t.Errorf("resp = %+v", resp)
	}
	if len(hooks.rejected) != 1 {
		t.Errorf("reject hook calls = %d", len(hooks.rejected))
	}
	if rec := do(t, mux, "GET", "/api/v1/drafts/"+uuid.NewString(), nil); rec.Code != http.StatusNotFound {
		t.Errorf("unknown draft = %d, want 404", rec.Code)
	}
}

var newTargets = []string{TargetVehicle, TargetContractCategory, TargetPurchaseCategory, TargetLedgerCategory, TargetLedgerAccount}

func TestNew_NewTargets(t *testing.T) {
	src := Source{Kind: SourceMCP, Ref: "r"}
	for _, target := range newTargets {
		d, err := New(src, target, "", json.RawMessage(`{"name":"x"}`), "")
		if err != nil || d.TargetType != target {
			t.Errorf("%s: %v %+v", target, err, d)
		}
	}
}

func TestNew_ParentRules(t *testing.T) {
	src := Source{Kind: SourceMCP, Ref: "r"}
	parent := uuid.NewString()
	for _, target := range []string{TargetVehicle, TargetContractCategory, TargetPurchaseCategory, TargetLedgerAccount} {
		_, err := New(src, target, parent, nil, "")
		if err == nil || !strings.Contains(err.Error(), "parentId is not allowed for "+target) {
			t.Errorf("%s with parent: err = %v", target, err)
		}
	}
	for _, target := range []string{TargetPurchase, TargetContract, TargetCost, TargetLedgerCategory, TargetNone} {
		d, err := New(src, target, parent, nil, "")
		if err != nil || d.ParentID != parent {
			t.Errorf("%s with parent: %v %+v", target, err, d)
		}
	}
	if _, err := New(src, TargetLedgerCategory, "nope", nil, ""); err == nil {
		t.Error("ledger_category: invalid parent UUID accepted")
	}
	if d, err := New(src, TargetLedgerCategory, "", nil, ""); err != nil || d.ParentID != "" {
		t.Errorf("ledger_category without parent: %v", err)
	}
}

func TestNew_UnknownTargetListsValidValues(t *testing.T) {
	_, err := New(Source{Kind: SourceMCP, Ref: "r"}, "car", "", nil, "")
	if err == nil || !strings.Contains(err.Error(), "ledger_account") || !strings.Contains(err.Error(), "none") {
		t.Fatalf("err = %v", err)
	}
}

func TestHandler_UpdateToNewTargets(t *testing.T) {
	store, _, mux := setup(t)
	d := createDraft(t, store, "u1", TargetPurchase)

	rec := do(t, mux, "PUT", "/api/v1/drafts/"+d.ID.String(), UpdateInput{TargetType: TargetVehicle, Input: json.RawMessage(`{"name":"Golf"}`)})
	if rec.Code != http.StatusOK {
		t.Fatalf("update = %d %s", rec.Code, rec.Body)
	}
	rec = do(t, mux, "PUT", "/api/v1/drafts/"+d.ID.String(), UpdateInput{TargetType: TargetVehicle, ParentID: uuid.NewString()})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("update with parent = %d", rec.Code)
	}
	rec = do(t, mux, "PUT", "/api/v1/drafts/"+d.ID.String(), UpdateInput{TargetType: TargetLedgerCategory, ParentID: uuid.NewString(), Input: json.RawMessage(`{"name":"x"}`)})
	if rec.Code != http.StatusOK {
		t.Fatalf("ledger_category update = %d %s", rec.Code, rec.Body)
	}
}

func TestHandler_AcceptNewTargets(t *testing.T) {
	store, hooks, mux := setup(t)
	for i, target := range newTargets {
		d, err := New(Source{Kind: SourcePaperless, Ref: strings.Repeat("a", i+1)}, target, "", json.RawMessage(`{"name":"x"}`), "")
		if err != nil {
			t.Fatal(err)
		}
		if err := store.Create(t.Context(), testUserID, d); err != nil {
			t.Fatal(err)
		}
		rec := do(t, mux, "POST", "/api/v1/drafts/"+d.ID.String()+"/accept", acceptBody(target))
		if rec.Code != http.StatusOK {
			t.Fatalf("%s accept = %d %s", target, rec.Code, rec.Body)
		}
		decided := decode[decisionResponse](t, rec)
		if decided.Draft.Result == nil || decided.Draft.Result.EntityType != target {
			t.Errorf("%s result = %+v", target, decided.Draft.Result)
		}
	}
	if len(hooks.accepted) != len(newTargets) {
		t.Fatalf("hook calls = %d", len(hooks.accepted))
	}
}

func TestHandler_AcceptEntityTypeMismatchAndNone(t *testing.T) {
	store, _, mux := setup(t)
	d, _ := New(Source{Kind: SourceMCP, Ref: "m"}, TargetVehicle, "", nil, "")
	if err := store.Create(t.Context(), testUserID, d); err != nil {
		t.Fatal(err)
	}
	for _, entity := range []string{TargetCost, TargetLedgerAccount, TargetNone, "car"} {
		rec := do(t, mux, "POST", "/api/v1/drafts/"+d.ID.String()+"/accept", acceptBody(entity))
		if rec.Code != http.StatusBadRequest {
			t.Errorf("accept as %s = %d", entity, rec.Code)
		}
	}
}

func TestStore_Counts(t *testing.T) {
	store, _, _ := setup(t)
	ctx := t.Context()
	if p, n, err := store.Counts(ctx, testUserID); err != nil || p != 0 || n != 0 {
		t.Fatalf("empty = %d %d %v", p, n, err)
	}
	a := createDraft(t, store, "a", TargetPurchase)
	createDraft(t, store, "b", TargetContract)
	c := createDraft(t, store, "c", TargetCost)
	store.Decide(ctx, testUserID, a.ID, StatusRejected, nil)
	store.Decide(ctx, testUserID, c.ID, StatusAccepted, &Result{EntityType: TargetCost, EntityID: uuid.NewString()})
	if p, n, err := store.Counts(ctx, testUserID); err != nil || p != 1 || n != 3 {
		t.Fatalf("counts = %d %d %v", p, n, err)
	}
	if p, n, _ := store.Counts(ctx, "other-user"); p != 0 || n != 0 {
		t.Fatalf("other user sees %d %d", p, n)
	}
}

func TestHandler_Summary(t *testing.T) {
	store, _, mux := setup(t)
	createDraft(t, store, "a", TargetPurchase)
	b := createDraft(t, store, "b", TargetNone)
	store.Decide(t.Context(), testUserID, b.ID, StatusRejected, nil)

	rec := do(t, mux, "GET", "/api/v1/drafts/summary", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("summary = %d %s", rec.Code, rec.Body)
	}
	got := decode[map[string]int](t, rec)
	if got["pending"] != 1 || got["total"] != 2 {
		t.Fatalf("summary = %v (not shadowed by {id}?)", got)
	}
}

func TestStore_CreateLimited(t *testing.T) {
	store, _, _ := setup(t)
	ctx := t.Context()
	mk := func(user, kind, ref string) Draft {
		d, err := New(Source{Kind: kind, Ref: ref}, TargetNone, "", nil, "")
		if err != nil {
			t.Fatal(err)
		}
		return d
	}
	for i := range 3 {
		if err := store.CreateLimited(ctx, testUserID, mk(testUserID, SourceMCP, string(rune('a'+i))), 3); err != nil {
			t.Fatalf("draft %d: %v", i, err)
		}
	}
	over := mk(testUserID, SourceMCP, "z")
	if err := store.CreateLimited(ctx, testUserID, over, 3); !errors.Is(err, ErrTooManyPending) {
		t.Fatalf("over the cap: err = %v", err)
	}
	if has, _ := store.HasSource(ctx, testUserID, SourceMCP, "z"); has {
		t.Fatal("the rejected draft was stored")
	}
	// duplicates are reported as such even at the cap
	if err := store.CreateLimited(ctx, testUserID, mk(testUserID, SourceMCP, "a"), 3); !errors.Is(err, ErrDuplicateSource) {
		t.Fatalf("duplicate at the cap: err = %v", err)
	}
	// the cap is per user and per source kind
	if err := store.CreateLimited(ctx, "other-user", mk("", SourceMCP, "a"), 3); err != nil {
		t.Fatalf("other user: %v", err)
	}
	if err := store.CreateLimited(ctx, testUserID, mk(testUserID, SourcePaperless, "p"), 3); err != nil {
		t.Fatalf("other source kind: %v", err)
	}
	// deciding a draft frees a slot
	list, _ := store.List(ctx, testUserID, StatusPending)
	var first Draft
	for _, d := range list {
		if d.Source.Kind == SourceMCP {
			first = d
		}
	}
	if _, err := store.Decide(ctx, testUserID, first.ID, StatusRejected, nil); err != nil {
		t.Fatal(err)
	}
	if err := store.CreateLimited(ctx, testUserID, over, 3); err != nil {
		t.Fatalf("after a decision: %v", err)
	}
	// max 0 means unlimited (Create)
	for i := range 5 {
		if err := store.Create(ctx, testUserID, mk(testUserID, SourceText, string(rune('a'+i)))); err != nil {
			t.Fatal(err)
		}
	}
}

func TestStore_ParallelSameSourceReportsDuplicate(t *testing.T) {
	store, _, _ := setup(t)
	ctx := t.Context()
	const workers = 16
	results := make(chan error, workers)
	for range workers {
		go func() {
			d, err := New(Source{Kind: SourceMCP, Ref: "k:same"}, TargetNone, "", nil, "")
			if err != nil {
				results <- err
				return
			}
			results <- store.CreateLimited(ctx, testUserID, d, 500)
		}()
	}
	created, duplicates := 0, 0
	for range workers {
		switch err := <-results; {
		case err == nil:
			created++
		case errors.Is(err, ErrDuplicateSource):
			duplicates++
		default:
			t.Errorf("unexpected error: %v", err)
		}
	}
	if created != 1 || duplicates != workers-1 {
		t.Fatalf("created %d, duplicates %d; want 1 and %d", created, duplicates, workers-1)
	}
	if list, _ := store.List(ctx, testUserID, ""); len(list) != 1 {
		t.Fatalf("%d drafts stored, want 1", len(list))
	}
}
