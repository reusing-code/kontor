package llm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"
)

const (
	secretInput  = "Rechnung Nr. 4711 GEHEIM-INPUT"
	secretOutput = "GEHEIM-OUTPUT"
)

type invoice struct {
	Number string   `json:"number"`
	Total  *float64 `json:"total"`
}

// fakeEndpoint answers chat completions with scripted replies and records
// the requests it receives.
type fakeEndpoint struct {
	t       *testing.T
	server  *httptest.Server
	mu      sync.Mutex
	replies []func(w http.ResponseWriter, r *http.Request)
	bodies  []map[string]any
	auth    []string
	paths   []string
}

func newFakeEndpoint(t *testing.T, replies ...func(w http.ResponseWriter, r *http.Request)) *fakeEndpoint {
	t.Helper()
	f := &fakeEndpoint{t: t, replies: replies}
	f.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decoding request body: %v", err)
		}
		f.mu.Lock()
		f.bodies = append(f.bodies, body)
		f.auth = append(f.auth, r.Header.Get("Authorization"))
		f.paths = append(f.paths, r.Method+" "+r.URL.Path)
		if len(f.replies) == 0 {
			f.mu.Unlock()
			t.Errorf("unexpected request %d", len(f.bodies))
			http.Error(w, "no reply", http.StatusInternalServerError)
			return
		}
		reply := f.replies[0]
		f.replies = f.replies[1:]
		f.mu.Unlock()
		reply(w, r)
	}))
	t.Cleanup(f.server.Close)
	return f
}

func (f *fakeEndpoint) client(apiKey string) *Client {
	return NewClient(Config{BaseURL: f.server.URL + "/v1/", Model: "kontor", APIKey: apiKey, Timeout: 5 * time.Second})
}

func (f *fakeEndpoint) calls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.bodies)
}

type completion struct {
	content      *string
	refusal      string
	finishReason string
	prompt       int
	completion   int
}

func reply(c completion) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, _ *http.Request) {
		finish := c.finishReason
		if finish == "" {
			finish = "stop"
		}
		msg := map[string]any{"role": "assistant", "content": c.content}
		if c.refusal != "" {
			msg["refusal"] = c.refusal
		}
		resp := map[string]any{
			"id":      "cmpl-1",
			"choices": []any{map[string]any{"index": 0, "message": msg, "finish_reason": finish}},
			"usage":   map[string]any{"prompt_tokens": c.prompt, "completion_tokens": c.completion, "total_tokens": c.prompt + c.completion},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}
}

func content(s string) func(w http.ResponseWriter, r *http.Request) {
	return reply(completion{content: &s, prompt: 10, completion: 5})
}

func status(code int, body string) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(code)
		w.Write([]byte(body))
	}
}

func request(op string) Request {
	return Request{Operation: op, System: "Extract the invoice.", Input: secretInput, Schema: MustSchemaFor[invoice]()}
}

func assertNoContent(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		return
	}
	for _, s := range []string{secretInput, "GEHEIM", secretOutput} {
		if strings.Contains(err.Error(), s) {
			t.Errorf("error %q leaks content %q", err, s)
		}
	}
}

var opSeq atomic.Int64

// uniqueOp keeps metric assertions independent of earlier runs (-count=N),
// as the collectors are process-global.
func uniqueOp(t *testing.T) string {
	return fmt.Sprintf("%s#%d", t.Name(), opSeq.Add(1))
}

func requestCount(op, outcome string) float64 {
	return testutil.ToFloat64(requestsTotal.WithLabelValues(op, outcome))
}

func TestExtractRequestShape(t *testing.T) {
	f := newFakeEndpoint(t, content(`{"number":"4711","total":12.5}`))
	out, err := f.client("secret-key").Extract(t.Context(), request("test.shape"))
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if string(out) != `{"number":"4711","total":12.5}` {
		t.Errorf("output = %s", out)
	}
	if f.paths[0] != "POST /v1/chat/completions" {
		t.Errorf("path = %q", f.paths[0])
	}
	if f.auth[0] != "Bearer secret-key" {
		t.Errorf("Authorization = %q", f.auth[0])
	}

	body := f.bodies[0]
	if body["model"] != "kontor" {
		t.Errorf("model = %v", body["model"])
	}
	messages, _ := body["messages"].([]any)
	want := []map[string]any{{"role": "system", "content": "Extract the invoice."}, {"role": "user", "content": secretInput}}
	if len(messages) != len(want) {
		t.Fatalf("messages = %v", messages)
	}
	for i, m := range messages {
		msg := m.(map[string]any)
		if msg["role"] != want[i]["role"] || msg["content"] != want[i]["content"] {
			t.Errorf("message %d = %v, want %v", i, msg, want[i])
		}
	}

	rf := body["response_format"].(map[string]any)
	if rf["type"] != "json_schema" {
		t.Errorf("response_format.type = %v", rf["type"])
	}
	js := rf["json_schema"].(map[string]any)
	if js["name"] != "invoice" || js["strict"] != true {
		t.Errorf("json_schema name/strict = %v/%v", js["name"], js["strict"])
	}
	schema, _ := json.Marshal(js["schema"])
	wantSchema := `{"additionalProperties":false,"properties":{"number":{"type":"string"},"total":{"type":["number","null"]}},"required":["number","total"],"type":"object"}`
	if string(schema) != wantSchema {
		t.Errorf("schema = %s\nwant     %s", schema, wantSchema)
	}
}

func TestExtractImagesUseVisionModel(t *testing.T) {
	f := newFakeEndpoint(t, content(`{"number":"1","total":null}`), content(`{"number":"2","total":null}`))
	c := NewClient(Config{BaseURL: f.server.URL + "/v1", Model: "kontor", VisionModel: "kontor-vision", Timeout: 5 * time.Second})

	req := request("test.vision")
	req.Images = []Image{{MediaType: "image/jpeg", Data: []byte("jpeg-bytes")}, {MediaType: "image/png", Data: []byte("png")}}
	if _, err := c.Extract(t.Context(), req); err != nil {
		t.Fatalf("Extract: %v", err)
	}
	body := f.bodies[0]
	if body["model"] != "kontor-vision" {
		t.Errorf("model = %v, want the vision model", body["model"])
	}
	user := body["messages"].([]any)[1].(map[string]any)
	got, _ := json.Marshal(user["content"])
	want := `[{"text":"` + secretInput + `","type":"text"},` +
		`{"image_url":{"url":"data:image/jpeg;base64,anBlZy1ieXRlcw=="},"type":"image_url"},` +
		`{"image_url":{"url":"data:image/png;base64,cG5n"},"type":"image_url"}]`
	if string(got) != want {
		t.Errorf("content = %s\nwant      %s", got, want)
	}

	req.Input = ""
	req.Images = req.Images[:1]
	if _, err := c.Extract(t.Context(), req); err != nil {
		t.Fatalf("Extract without text: %v", err)
	}
	parts := f.bodies[1]["messages"].([]any)[1].(map[string]any)["content"].([]any)
	if len(parts) != 1 || parts[0].(map[string]any)["type"] != "image_url" {
		t.Errorf("content without text = %v, want only the image part", parts)
	}
}

func TestExtractImagesWithoutVisionModel(t *testing.T) {
	f := newFakeEndpoint(t)
	req := request(uniqueOp(t))
	req.Images = []Image{{MediaType: "image/jpeg", Data: []byte("x")}}
	_, err := f.client("").Extract(t.Context(), req)
	if !errors.Is(err, ErrVisionDisabled) {
		t.Fatalf("err = %v, want ErrVisionDisabled", err)
	}
	if f.calls() != 0 {
		t.Errorf("calls = %d, want none", f.calls())
	}
}

func TestExtractOmitsAuthAndEmptySystem(t *testing.T) {
	f := newFakeEndpoint(t, content(`{"number":"1","total":null}`))
	req := request("test.noauth")
	req.System = ""
	if _, err := f.client("").Extract(t.Context(), req); err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if f.auth[0] != "" {
		t.Errorf("Authorization = %q, want none", f.auth[0])
	}
	if messages := f.bodies[0]["messages"].([]any); len(messages) != 1 {
		t.Errorf("messages = %v, want only the user message", messages)
	}
}

func TestExtractExtraBody(t *testing.T) {
	f := newFakeEndpoint(t, content(`{"number":"1","total":null}`))
	c := f.client("")
	c.extraBody = map[string]any{"provider": map[string]any{"require_parameters": true}}
	if _, err := c.Extract(t.Context(), request("test.extra")); err != nil {
		t.Fatalf("Extract: %v", err)
	}
	provider, _ := f.bodies[0]["provider"].(map[string]any)
	if provider["require_parameters"] != true {
		t.Errorf("provider = %v", f.bodies[0]["provider"])
	}
	if f.bodies[0]["model"] != "kontor" {
		t.Errorf("model = %v", f.bodies[0]["model"])
	}
}

func TestExtractAs(t *testing.T) {
	f := newFakeEndpoint(t, content(" {\"number\":\"4711\",\"total\":99.95}\n"))
	got, err := ExtractAs[invoice](t.Context(), f.client(""), Request{Operation: "test.as", Input: secretInput})
	if err != nil {
		t.Fatalf("ExtractAs: %v", err)
	}
	if got.Number != "4711" || got.Total == nil || *got.Total != 99.95 {
		t.Errorf("got %+v", got)
	}
	js := f.bodies[0]["response_format"].(map[string]any)["json_schema"].(map[string]any)
	if js["name"] != "invoice" {
		t.Errorf("schema name = %v, want derived from type", js["name"])
	}
}

func TestExtractRetriesInvalidOutput(t *testing.T) {
	op := uniqueOp(t)
	f := newFakeEndpoint(t, content("not json "+secretOutput), content(`{"number":"1","total":2}`))
	out, err := f.client("").Extract(t.Context(), request(op))
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if string(out) != `{"number":"1","total":2}` {
		t.Errorf("output = %s", out)
	}
	if f.calls() != 2 {
		t.Errorf("calls = %d, want 2", f.calls())
	}
	if got := testutil.ToFloat64(retriesTotal.WithLabelValues(op)); got != 1 {
		t.Errorf("retries = %v, want 1", got)
	}
	if got := requestCount(op, outcomeOK); got != 1 {
		t.Errorf("ok requests = %v, want 1", got)
	}
}

func TestExtractAsRetriesDecodeFailure(t *testing.T) {
	f := newFakeEndpoint(t, content(`{"number":"1","total":"12,50"}`), content(`{"number":"1","total":12.5}`))
	got, err := ExtractAs[invoice](t.Context(), f.client(""), request("test.decode"))
	if err != nil {
		t.Fatalf("ExtractAs: %v", err)
	}
	if got.Total == nil || *got.Total != 12.5 {
		t.Errorf("got %+v", got)
	}
	if f.calls() != 2 {
		t.Errorf("calls = %d, want 2", f.calls())
	}
}

func TestExtractInvalidTwice(t *testing.T) {
	op := uniqueOp(t)
	f := newFakeEndpoint(t, content(secretOutput), content(`["`+secretOutput+`"]`))
	_, err := f.client("").Extract(t.Context(), request(op))
	if !errors.Is(err, ErrInvalidOutput) {
		t.Fatalf("err = %v, want ErrInvalidOutput", err)
	}
	assertNoContent(t, err)
	if f.calls() != 2 {
		t.Errorf("calls = %d, want 2", f.calls())
	}
	if got := requestCount(op, outcomeInvalidOutput); got != 1 {
		t.Errorf("invalid_output requests = %v, want 1", got)
	}
}

func TestExtractAsDecodeErrorHidesContent(t *testing.T) {
	bad := `{"number":"` + secretOutput + `","total":"` + secretOutput + `"}`
	f := newFakeEndpoint(t, content(bad), content(bad))
	_, err := ExtractAs[invoice](t.Context(), f.client(""), request("test.decodeerr"))
	if !errors.Is(err, ErrInvalidOutput) {
		t.Fatalf("err = %v, want ErrInvalidOutput", err)
	}
	assertNoContent(t, err)
}

func TestExtractAPIError(t *testing.T) {
	op := uniqueOp(t)
	body := `{"error":{"message":"model not allowed","type":"invalid_request_error"},"echo":"` + secretInput + `"}`
	f := newFakeEndpoint(t, status(http.StatusForbidden, body))
	_, err := f.client("").Extract(t.Context(), request(op))
	apiErr, ok := errors.AsType[*APIError](err)
	if !ok {
		t.Fatalf("err = %v, want *APIError", err)
	}
	if apiErr.StatusCode != http.StatusForbidden || apiErr.Message != "model not allowed" {
		t.Errorf("APIError = %+v", apiErr)
	}
	assertNoContent(t, err)
	if f.calls() != 1 {
		t.Errorf("calls = %d, want no retry", f.calls())
	}
	if got := requestCount(op, outcomeHTTPError); got != 1 {
		t.Errorf("http_error requests = %v, want 1", got)
	}
}

func TestExtractAPIErrorTruncatesMessage(t *testing.T) {
	long := strings.Repeat("x", 1000)
	f := newFakeEndpoint(t, status(http.StatusBadGateway, `{"error":{"message":"`+long+`"}}`))
	_, err := f.client("").Extract(t.Context(), request("test.apierrlong"))
	apiErr, ok := errors.AsType[*APIError](err)
	if !ok {
		t.Fatalf("err = %v, want *APIError", err)
	}
	if n := len([]rune(apiErr.Message)); n != maxErrorMessageLen+1 {
		t.Errorf("message length = %d, want %d", n, maxErrorMessageLen+1)
	}
}

func TestExtractAPIErrorNonJSONBody(t *testing.T) {
	f := newFakeEndpoint(t, status(http.StatusInternalServerError, "upstream said: "+secretInput))
	_, err := f.client("").Extract(t.Context(), request("test.apierrraw"))
	apiErr, ok := errors.AsType[*APIError](err)
	if !ok {
		t.Fatalf("err = %v, want *APIError", err)
	}
	if apiErr.Message != "" {
		t.Errorf("Message = %q, want empty", apiErr.Message)
	}
	assertNoContent(t, err)
}

func TestExtractRefusal(t *testing.T) {
	op := uniqueOp(t)
	f := newFakeEndpoint(t, reply(completion{refusal: "I can't help with " + secretInput}))
	_, err := f.client("").Extract(t.Context(), request(op))
	if !errors.Is(err, ErrRefused) {
		t.Fatalf("err = %v, want ErrRefused", err)
	}
	assertNoContent(t, err)
	if got := requestCount(op, outcomeRefused); got != 1 {
		t.Errorf("refused requests = %v, want 1", got)
	}
}

func TestExtractTruncated(t *testing.T) {
	op := uniqueOp(t)
	partial := `{"number":"` + secretOutput
	f := newFakeEndpoint(t, reply(completion{content: &partial, finishReason: "length"}))
	_, err := f.client("").Extract(t.Context(), request(op))
	if !errors.Is(err, ErrTruncated) {
		t.Fatalf("err = %v, want ErrTruncated", err)
	}
	assertNoContent(t, err)
	if f.calls() != 1 {
		t.Errorf("calls = %d, want no retry", f.calls())
	}
	if got := requestCount(op, outcomeTruncated); got != 1 {
		t.Errorf("truncated requests = %v, want 1", got)
	}
}

func TestExtractTimeout(t *testing.T) {
	op := uniqueOp(t)
	f := newFakeEndpoint(t, func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(5 * time.Second):
		}
	})
	c := f.client("")
	c.timeout = 50 * time.Millisecond
	_, err := c.Extract(t.Context(), request(op))
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v, want DeadlineExceeded", err)
	}
	if got := requestCount(op, outcomeTimeout); got != 1 {
		t.Errorf("timeout requests = %v, want 1", got)
	}
}

func TestExtractCanceled(t *testing.T) {
	op := uniqueOp(t)
	f := newFakeEndpoint(t)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err := f.client("").Extract(ctx, request(op))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want Canceled", err)
	}
	if got := requestCount(op, outcomeCanceled); got != 1 {
		t.Errorf("canceled requests = %v, want 1", got)
	}
}

func TestExtractRequiresSchema(t *testing.T) {
	f := newFakeEndpoint(t)
	_, err := f.client("").Extract(t.Context(), Request{Operation: "test.noschema", Input: secretInput})
	if err == nil {
		t.Fatal("expected error for missing schema")
	}
	if f.calls() != 0 {
		t.Errorf("calls = %d, want 0", f.calls())
	}
}

func TestExtractMetrics(t *testing.T) {
	op := uniqueOp(t)
	f := newFakeEndpoint(t,
		reply(completion{content: new("nope"), prompt: 100, completion: 7}),
		reply(completion{content: new(`{"number":"1","total":1}`), prompt: 120, completion: 9}),
	)
	if _, err := f.client("").Extract(t.Context(), request(op)); err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if got := testutil.ToFloat64(tokensTotal.WithLabelValues(op, "prompt")); got != 220 {
		t.Errorf("prompt tokens = %v, want 220", got)
	}
	if got := testutil.ToFloat64(tokensTotal.WithLabelValues(op, "completion")); got != 16 {
		t.Errorf("completion tokens = %v, want 16", got)
	}
	if got := testutil.ToFloat64(retriesTotal.WithLabelValues(op)); got != 1 {
		t.Errorf("retries = %v, want 1", got)
	}
	if got := requestCount(op, outcomeOK); got != 1 {
		t.Errorf("ok requests = %v, want 1", got)
	}
	if n := testutil.CollectAndCount(requestDuration, "llm_request_duration_seconds"); n == 0 {
		t.Error("no duration observations collected")
	}
}

func TestNewDisabled(t *testing.T) {
	ex := New(Config{})
	if ex.Enabled() {
		t.Error("Enabled() = true for empty base URL")
	}
	if _, err := ex.Extract(t.Context(), request("test.disabled")); !errors.Is(err, ErrDisabled) {
		t.Errorf("Extract err = %v, want ErrDisabled", err)
	}
	if _, err := ExtractAs[invoice](t.Context(), ex, Request{Input: secretInput}); !errors.Is(err, ErrDisabled) {
		t.Errorf("ExtractAs err = %v, want ErrDisabled", err)
	}
	if !New(Config{BaseURL: "http://gw/v1", Model: "m", Timeout: time.Second}).Enabled() {
		t.Error("Enabled() = false with base URL")
	}
}
