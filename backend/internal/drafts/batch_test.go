package drafts

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/reusing-code/kontor/backend/internal/middleware"
	"github.com/reusing-code/kontor/backend/internal/storage"
)

const batchBaseURL = "https://kontor.example"

// fakeCreator stands in for a module: it keeps the created entities by ID and
// rejects an input without itemName like a create endpoint would.
type fakeCreator struct {
	entities map[uuid.UUID]json.RawMessage
	calls    []CreateRequest
	err      error
}

func (f *fakeCreator) create(_ context.Context, _ string, req CreateRequest) (string, error) {
	f.calls = append(f.calls, req)
	path := "/purchases/" + req.EntityID.String()
	if _, ok := f.entities[req.EntityID]; ok {
		return path, nil
	}
	if f.err != nil {
		return "", f.err
	}
	if _, err := req.ParentUUID(); err != nil {
		return "", err
	}
	var input struct {
		ItemName string `json:"itemName"`
	}
	if err := DecodeInput(req.Input, &input); err != nil {
		return "", err
	}
	if input.ItemName == "" {
		return "", InvalidInput(errors.New("itemName is required"))
	}
	f.entities[req.EntityID] = req.Input
	return path, nil
}

type batchEnv struct {
	store    *Store
	hooks    *fakeHooks
	creator  *fakeCreator
	disabled map[string]bool
	mux      http.Handler
}

func setupBatch(t *testing.T) *batchEnv {
	t.Helper()
	e, err := storage.Open(t.TempDir(), slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatalf("opening engine: %v", err)
	}
	t.Cleanup(func() { e.Close() })
	env := &batchEnv{
		store:    NewStore(e),
		hooks:    &fakeHooks{},
		creator:  &fakeCreator{entities: map[uuid.UUID]json.RawMessage{}},
		disabled: map[string]bool{},
	}
	h := NewHandler(env.store, slog.New(slog.DiscardHandler))
	h.RegisterSource(SourcePaperless, env.hooks)
	h.RegisterTarget(TargetPurchase, "purchases", env.creator.create)
	h.RegisterTarget(TargetContract, "contracts", env.creator.create)
	h.SetEnabled(func(_ context.Context, _, moduleID string) (bool, error) { return !env.disabled[moduleID], nil })
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/drafts/accept-batch", h.AcceptBatch)
	mux.HandleFunc("POST /api/v1/drafts/{id}/reject", h.Reject)
	env.mux = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mux.ServeHTTP(w, r.WithContext(middleware.SetUserID(r.Context(), testUserID)))
	})
	return env
}

var batchParent = uuid.NewString()

func (env *batchEnv) draft(t *testing.T, ref, target, parent, input string) Draft {
	t.Helper()
	d, err := New(Source{Kind: SourcePaperless, Ref: ref, Title: "Invoice " + ref}, target, parent, json.RawMessage(input), "")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := env.store.Create(context.Background(), testUserID, d); err != nil {
		t.Fatalf("Create: %v", err)
	}
	return d
}

func (env *batchEnv) valid(t *testing.T, ref string) Draft {
	return env.draft(t, ref, TargetPurchase, batchParent, `{"itemName":"Drill"}`)
}

func (env *batchEnv) accept(t *testing.T, ids ...uuid.UUID) []BatchResult {
	t.Helper()
	raw := make([]string, len(ids))
	for i, id := range ids {
		raw[i] = id.String()
	}
	rec := do(t, env.mux, "POST", "/api/v1/drafts/accept-batch", AcceptBatchInput{IDs: raw, BaseURL: batchBaseURL})
	if rec.Code != http.StatusOK {
		t.Fatalf("accept-batch = %d: %s", rec.Code, rec.Body.String())
	}
	return decode[acceptBatchResponse](t, rec).Results
}

func (env *batchEnv) status(t *testing.T, id uuid.UUID) string {
	t.Helper()
	d, err := env.store.Get(context.Background(), testUserID, id)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	return d.Status
}

func TestAcceptBatch_MixedBatch(t *testing.T) {
	env := setupBatch(t)
	valid := env.valid(t, "1")
	invalid := env.draft(t, "2", TargetPurchase, batchParent, `{"itemName":""}`)
	unknownField := env.draft(t, "3", TargetPurchase, batchParent, `{"itemName":"Saw","colour":"red"}`)
	accepted := env.valid(t, "4")
	rejected := env.valid(t, "5")
	none := env.draft(t, "6", TargetNone, "", `{}`)
	noParent := env.draft(t, "7", TargetPurchase, "", `{"itemName":"Vice"}`)
	disabled := env.draft(t, "8", TargetContract, batchParent, `{"itemName":"Fiber"}`)
	missing := uuid.New()

	env.accept(t, accepted.ID)
	if rec := do(t, env.mux, "POST", "/api/v1/drafts/"+rejected.ID.String()+"/reject", nil); rec.Code != http.StatusOK {
		t.Fatalf("reject = %d", rec.Code)
	}
	env.disabled["contracts"] = true
	entitiesBefore := len(env.creator.entities)

	results := env.accept(t, valid.ID, invalid.ID, unknownField.ID, accepted.ID, rejected.ID, none.ID, noParent.ID, disabled.ID, missing)

	want := []struct {
		id           uuid.UUID
		status, code string
		draftStatus  string
	}{
		{valid.ID, BatchAccepted, "", StatusAccepted},
		{invalid.ID, BatchFailed, CodeInvalidInput, StatusPending},
		{unknownField.ID, BatchFailed, CodeInvalidInput, StatusPending},
		{accepted.ID, BatchAlreadyAccepted, "", StatusAccepted},
		{rejected.ID, BatchFailed, CodeRejected, StatusRejected},
		{none.ID, BatchFailed, CodeNoType, StatusPending},
		{noParent.ID, BatchFailed, CodeParentMissing, StatusPending},
		{disabled.ID, BatchFailed, CodeModuleDisabled, StatusPending},
		{missing, BatchFailed, CodeNotFound, ""},
	}
	if len(results) != len(want) {
		t.Fatalf("results = %d, want %d: %+v", len(results), len(want), results)
	}
	for i, w := range want {
		got := results[i]
		if got.ID != w.id.String() || got.Status != w.status || got.Code != w.code {
			t.Errorf("result %d = %+v, want status %q code %q", i, got, w.status, w.code)
		}
		if (got.Status == BatchFailed) != (got.Error != "") {
			t.Errorf("result %d: error text = %q for status %q", i, got.Error, got.Status)
		}
		if w.draftStatus != "" && env.status(t, w.id) != w.draftStatus {
			t.Errorf("draft %d status = %s, want %s", i, env.status(t, w.id), w.draftStatus)
		}
	}
	if results[1].Error != "itemName is required" {
		t.Errorf("invalid input error = %q, want the create validation message", results[1].Error)
	}

	wantEntity := EntityID(valid.ID).String()
	if results[0].EntityID != wantEntity || results[0].EntityType != TargetPurchase || len(results[0].Warnings) != 1 {
		t.Errorf("accepted result = %+v", results[0])
	}
	if results[3].EntityID != EntityID(accepted.ID).String() || results[3].EntityType != TargetPurchase {
		t.Errorf("already accepted result = %+v", results[3])
	}
	if len(env.creator.entities) != entitiesBefore+1 {
		t.Errorf("entities = %d, want one new", len(env.creator.entities))
	}
	last := env.hooks.accepted[len(env.hooks.accepted)-1]
	if last.EntityID != wantEntity || last.EntityURL != batchBaseURL+"/purchases/"+wantEntity || last.EntityType != TargetPurchase {
		t.Errorf("hook input = %+v", last)
	}
	stored, _ := env.store.Get(context.Background(), testUserID, valid.ID)
	if stored.Result == nil || stored.Result.EntityID != wantEntity || stored.DecidedAt == nil {
		t.Errorf("stored draft = %+v", stored)
	}
}

func TestAcceptBatch_RepeatCreatesNothingNew(t *testing.T) {
	env := setupBatch(t)
	a, b := env.valid(t, "1"), env.valid(t, "2")
	invalid := env.draft(t, "3", TargetPurchase, batchParent, `{}`)

	first := env.accept(t, a.ID, b.ID, invalid.ID)
	second := env.accept(t, a.ID, b.ID, invalid.ID)

	if first[0].Status != BatchAccepted || first[1].Status != BatchAccepted || first[2].Code != CodeInvalidInput {
		t.Fatalf("first = %+v", first)
	}
	if second[0].Status != BatchAlreadyAccepted || second[1].Status != BatchAlreadyAccepted || second[2].Code != CodeInvalidInput {
		t.Fatalf("second = %+v", second)
	}
	if second[0].EntityID != first[0].EntityID || second[1].EntityID != first[1].EntityID {
		t.Errorf("entity IDs changed: %+v vs %+v", first, second)
	}
	if len(env.creator.entities) != 2 || len(env.hooks.accepted) != 2 {
		t.Errorf("entities = %d, hook calls = %d, want 2 and 2", len(env.creator.entities), len(env.hooks.accepted))
	}
}

func TestAcceptBatch_HookFailureThenRepeatReusesEntity(t *testing.T) {
	env := setupBatch(t)
	d := env.valid(t, "1")

	env.hooks.acceptErr = errors.New("paperless down")
	res := env.accept(t, d.ID)
	if res[0].Status != BatchFailed || res[0].Code != CodeSourceLinkFailed {
		t.Fatalf("result = %+v", res[0])
	}
	if env.status(t, d.ID) != StatusPending || len(env.creator.entities) != 1 {
		t.Fatalf("status = %s, entities = %d", env.status(t, d.ID), len(env.creator.entities))
	}

	env.hooks.acceptErr = nil
	res = env.accept(t, d.ID)
	if res[0].Status != BatchAccepted || res[0].EntityID != EntityID(d.ID).String() {
		t.Fatalf("repeat = %+v", res[0])
	}
	if len(env.creator.entities) != 1 {
		t.Errorf("entities = %d, want the one from the first attempt", len(env.creator.entities))
	}
	if len(env.creator.calls) != 2 || env.creator.calls[0].EntityID != env.creator.calls[1].EntityID {
		t.Errorf("create calls = %+v", env.creator.calls)
	}
}

func TestAcceptBatch_CreateErrors(t *testing.T) {
	cases := []struct {
		err  error
		code string
	}{
		{&CreateError{Code: CodeParentNotFound, Message: "gone"}, CodeParentNotFound},
		{&CreateError{Code: CodeConflict, Message: "an account with this IBAN exists"}, CodeConflict},
		{fmt.Errorf("%w: duplicate", storage.ErrConflict), CodeConflict},
		{errors.New("disk full"), CodeInternalError},
	}
	for _, c := range cases {
		env := setupBatch(t)
		d := env.valid(t, "1")
		env.creator.err = c.err
		res := env.accept(t, d.ID)
		if res[0].Status != BatchFailed || res[0].Code != c.code {
			t.Errorf("%v: result = %+v, want code %s", c.err, res[0], c.code)
		}
		if c.code == CodeInternalError && res[0].Error != "internal error" {
			t.Errorf("internal error text = %q, must not leak the cause", res[0].Error)
		}
		if env.status(t, d.ID) != StatusPending || len(env.hooks.accepted) != 0 {
			t.Errorf("%v: draft must stay pending without a hook call", c.err)
		}
	}
}

func TestAcceptBatch_OtherUsersDraftIsNotFound(t *testing.T) {
	env := setupBatch(t)
	d, err := New(Source{Kind: SourceMCP, Ref: "r:1", Title: "x"}, TargetPurchase, batchParent, json.RawMessage(`{"itemName":"Drill"}`), "")
	if err != nil {
		t.Fatal(err)
	}
	if err := env.store.Create(context.Background(), "00000000-0000-0000-0000-000000000002", d); err != nil {
		t.Fatal(err)
	}
	res := env.accept(t, d.ID)
	if res[0].Code != CodeNotFound || len(env.creator.calls) != 0 {
		t.Errorf("result = %+v, create calls = %d", res[0], len(env.creator.calls))
	}
}

func TestAcceptBatch_RequestValidation(t *testing.T) {
	env := setupBatch(t)
	d := env.valid(t, "1")
	id := d.ID.String()
	tooMany := make([]string, MaxBatchSize+1)
	for i := range tooMany {
		tooMany[i] = uuid.NewString()
	}
	cases := map[string]AcceptBatchInput{
		"no ids":         {IDs: nil, BaseURL: batchBaseURL},
		"too many":       {IDs: tooMany, BaseURL: batchBaseURL},
		"duplicate":      {IDs: []string{id, id}, BaseURL: batchBaseURL},
		"bad uuid":       {IDs: []string{"x"}, BaseURL: batchBaseURL},
		"no base url":    {IDs: []string{id}},
		"relative url":   {IDs: []string{id}, BaseURL: "/inbox"},
		"url with path":  {IDs: []string{id}, BaseURL: batchBaseURL + "/app"},
		"url with query": {IDs: []string{id}, BaseURL: batchBaseURL + "?x=1"},
		"other scheme":   {IDs: []string{id}, BaseURL: "javascript://kontor.example"},
	}
	for name, body := range cases {
		if rec := do(t, env.mux, "POST", "/api/v1/drafts/accept-batch", body); rec.Code != http.StatusBadRequest {
			t.Errorf("%s: status = %d, want 400", name, rec.Code)
		}
	}
	if len(env.creator.calls) != 0 || env.status(t, d.ID) != StatusPending {
		t.Error("an invalid request must not touch any draft")
	}

	full := make([]uuid.UUID, MaxBatchSize)
	for i := range full {
		full[i] = uuid.New()
	}
	if res := env.accept(t, full...); len(res) != MaxBatchSize {
		t.Errorf("results = %d, want %d", len(res), MaxBatchSize)
	}
	rec := do(t, env.mux, "POST", "/api/v1/drafts/accept-batch", AcceptBatchInput{IDs: []string{id}, BaseURL: batchBaseURL + "/"})
	if rec.Code != http.StatusOK || env.hooks.accepted[0].EntityURL != batchBaseURL+"/purchases/"+EntityID(d.ID).String() {
		t.Errorf("trailing slash: status = %d, hook = %+v", rec.Code, env.hooks.accepted)
	}
}

func TestEntityID_PinnedPerDraft(t *testing.T) {
	a, b := uuid.New(), uuid.New()
	if EntityID(a) == EntityID(b) || EntityID(a) == a {
		t.Error("EntityID must be distinct between drafts and differ from the draft ID")
	}
	// pinned: a changed namespace would duplicate entities on a repeated accept
	if got := EntityID(uuid.MustParse("00000000-0000-0000-0000-000000000001")).String(); got != pinnedEntityID {
		t.Errorf("EntityID = %s, want %s", got, pinnedEntityID)
	}
}

const pinnedEntityID = "fce830f4-d6cd-5e59-a59f-8af22e124856"
