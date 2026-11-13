package quickadd

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/reusing-code/kontor/backend/internal/drafts"
	"github.com/reusing-code/kontor/backend/internal/extraction"
	"github.com/reusing-code/kontor/backend/internal/fx"
	"github.com/reusing-code/kontor/backend/internal/llm"
	"github.com/reusing-code/kontor/backend/internal/llm/llmtest"
	"github.com/reusing-code/kontor/backend/internal/middleware"
)

const vehicleID = "33333333-3333-3333-3333-333333333333"

type testEnv struct {
	llm     *llmtest.Fake
	enabled map[string]bool
	logs    *bytes.Buffer
	handler *Handler
}

func newTestEnv() *testEnv {
	env := &testEnv{
		llm:     llmtest.New(),
		enabled: map[string]bool{drafts.TargetPurchase: true, drafts.TargetContract: true, drafts.TargetCost: true},
		logs:    &bytes.Buffer{},
	}
	deps := extraction.Deps{
		TargetEnabled: func(_ context.Context, _ string, target string) (bool, error) { return env.enabled[target], nil },
		Categories:    func(context.Context, string, string) ([]extraction.Option, error) { return nil, nil },
		Vehicles: func(context.Context, string) ([]extraction.Option, error) {
			return []extraction.Option{{ID: vehicleID, Name: "Golf"}}, nil
		},
	}
	logger := slog.New(slog.NewTextHandler(env.logs, nil))
	env.handler = NewHandler(extraction.New(env.llm, deps, fx.Disabled{}, logger), true, logger)
	env.handler.now = func() time.Time { return time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC) }
	return env
}

func (env *testEnv) post(t *testing.T, body any) *httptest.ResponseRecorder {
	t.Helper()
	data, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/quick-add", bytes.NewReader(data))
	req = req.WithContext(middleware.SetUserID(req.Context(), "u1"))
	rec := httptest.NewRecorder()
	env.handler.Classify(rec, req)
	return rec
}

func decodeResponse(t *testing.T, rec *httptest.ResponseRecorder) Response {
	t.Helper()
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body)
	}
	var resp Response
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	return resp
}

func amount(v float64) *float64 { return &v }

func TestClassify_ConfidentCost(t *testing.T) {
	env := newTestEnv()
	env.llm.Push(llmtest.JSON(extraction.Classification{Kind: "cost", Confident: true, Cost: &extraction.CostExtract{
		VehicleID: vehicleID, Type: "tires", Description: "Winter tires", Vendor: "Reifen Müller",
		Amount: amount(640), Currency: "EUR", Date: "2026-11-02",
	}}))

	resp := decodeResponse(t, env.post(t, Request{Text: "bought winter tires 2026-11-02 at Reifen Müller, €640, Golf"}))
	if resp.TargetType != drafts.TargetCost || resp.ParentID != vehicleID || !resp.Confident || resp.Note != "" || resp.Currency != nil {
		t.Errorf("response = %+v", resp)
	}
	if string(resp.Input) != `{"type":"tires","description":"Winter tires","vendor":"Reifen Müller","amount":640,"date":"2026-11-02"}` {
		t.Errorf("input = %s", resp.Input)
	}
	req := env.llm.Requests()[0]
	if req.Operation != "quickadd.classify" || !strings.Contains(req.System, "Today is 2026-11-08") {
		t.Errorf("request = %+v", req)
	}
	if strings.Contains(env.logs.String(), "Müller") {
		t.Error("the typed text must not be logged")
	}
}

func TestClassify_UnclearAndForcedType(t *testing.T) {
	env := newTestEnv()
	env.llm.Push(llmtest.JSON(extraction.Classification{Kind: "none"}))
	resp := decodeResponse(t, env.post(t, Request{Text: "something"}))
	if resp.TargetType != drafts.TargetNone || resp.Confident || resp.Note != extraction.NoteUnclassified || string(resp.Input) != "{}" {
		t.Errorf("none response = %+v", resp)
	}

	env.llm.Push(llmtest.JSON(extraction.Classification{Kind: "purchase", Confident: true, Purchase: &extraction.PurchaseExtract{ItemName: "Something"}}))
	resp = decodeResponse(t, env.post(t, Request{Text: "something", TargetType: drafts.TargetPurchase}))
	if resp.TargetType != drafts.TargetPurchase || resp.Confident {
		t.Errorf("forced response = %+v (no category, so not confident)", resp)
	}
	if system := env.llm.Requests()[1].System; !strings.Contains(system, `chosen the kind "purchase"`) || strings.Contains(system, "Vehicles") {
		t.Errorf("forced prompt = %q", system)
	}
}

func TestClassify_NonRetryableLLMErrorIsNone(t *testing.T) {
	env := newTestEnv()
	env.llm.Push(llmtest.Error(llm.ErrRefused))
	resp := decodeResponse(t, env.post(t, Request{Text: "x"}))
	if resp.TargetType != drafts.TargetNone || resp.Note != extraction.NoteLLMRefused {
		t.Errorf("response = %+v", resp)
	}
}

func TestClassify_Errors(t *testing.T) {
	tests := []struct {
		name    string
		setup   func(*testEnv)
		body    any
		status  int
		noCalls bool
	}{
		{name: "empty text", body: Request{Text: "  "}, status: http.StatusBadRequest, noCalls: true},
		{name: "too long", body: Request{Text: strings.Repeat("ä", MaxTextRunes+1)}, status: http.StatusBadRequest, noCalls: true},
		{name: "unknown field", body: map[string]string{"text": "x", "foo": "y"}, status: http.StatusBadRequest, noCalls: true},
		{name: "invalid target", body: Request{Text: "x", TargetType: "none"}, status: http.StatusBadRequest, noCalls: true},
		{
			name:  "disabled target",
			setup: func(env *testEnv) { env.enabled[drafts.TargetCost] = false },
			body:  Request{Text: "x", TargetType: drafts.TargetCost}, status: http.StatusBadRequest, noCalls: true,
		},
		{
			name:  "no targets",
			setup: func(env *testEnv) { clear(env.enabled) },
			body:  Request{Text: "x"}, status: http.StatusConflict, noCalls: true,
		},
		{
			name:  "llm disabled",
			setup: func(env *testEnv) { env.llm.Disabled = true },
			body:  Request{Text: "x"}, status: http.StatusConflict, noCalls: true,
		},
		{
			name:  "llm unavailable",
			setup: func(env *testEnv) { env.llm.Push(llmtest.Error(&llm.APIError{StatusCode: 503})) },
			body:  Request{Text: "x"}, status: http.StatusBadGateway,
		},
		{
			name:  "llm timeout",
			setup: func(env *testEnv) { env.llm.Push(llmtest.Error(context.DeadlineExceeded)) },
			body:  Request{Text: "x"}, status: http.StatusGatewayTimeout,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := newTestEnv()
			if tt.setup != nil {
				tt.setup(env)
			}
			rec := env.post(t, tt.body)
			if rec.Code != tt.status {
				t.Errorf("status = %d, want %d; body %s", rec.Code, tt.status, rec.Body)
			}
			if tt.noCalls && len(env.llm.Requests()) != 0 {
				t.Errorf("LLM must not be called, got %d requests", len(env.llm.Requests()))
			}
		})
	}
}
