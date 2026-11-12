package mcptool

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/reusing-code/kontor/backend/internal/drafts"
	"github.com/reusing-code/kontor/backend/internal/pat"
	"github.com/reusing-code/kontor/backend/internal/storage"
)

const testUser = "00000000-0000-0000-0000-000000000001"

type fakeEnabled struct {
	mu       sync.Mutex
	disabled map[string]bool
	calls    map[string]int
	err      error
}

func (f *fakeEnabled) enabled(_ context.Context, _, moduleID string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.calls == nil {
		f.calls = map[string]int{}
	}
	f.calls[moduleID]++
	return !f.disabled[moduleID], f.err
}

type fixture struct {
	reg     *Registrar
	enabled *fakeEnabled
	logs    *bytes.Buffer
	drafts  *drafts.Store
}

type echoIn struct {
	Text string `json:"text"`
}
type echoOut struct {
	Echo string `json:"echo"`
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	logs := &bytes.Buffer{}
	logger := slog.New(slog.NewTextHandler(logs, nil))
	engine, err := storage.Open(t.TempDir(), slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { engine.Close() })
	f := &fixture{enabled: &fakeEnabled{disabled: map[string]bool{}}, logs: logs, drafts: drafts.NewStore(engine)}
	f.reg = NewRegistrar(Env{Drafts: f.drafts, Enabled: f.enabled.enabled, Logger: logger})

	Add(f.reg, Spec{Name: "read_tool", Description: "read", Scope: ScopeRead, ReadOnly: true},
		func(_ context.Context, _ Call, in echoIn) (echoOut, error) { return echoOut{Echo: in.Text}, nil })
	Add(f.reg, Spec{Name: "draft_tool", Description: "draft", Scope: ScopeDrafts, Module: "contracts"},
		func(_ context.Context, _ Call, in echoIn) (echoOut, error) { return echoOut{Echo: in.Text}, nil })
	Add(f.reg, Spec{Name: "links_tool", Description: "links", Scope: ScopeLinks, Module: "auto"},
		func(_ context.Context, _ Call, in echoIn) (echoOut, error) { return echoOut{}, nil })
	Add(f.reg, Spec{Name: "tool_error", Description: "e", Scope: ScopeRead},
		func(_ context.Context, _ Call, in echoIn) (echoOut, error) {
			return echoOut{}, Errorf(CodeNotFound, "thing not found")
		})
	Add(f.reg, Spec{Name: "internal_fail", Description: "e", Scope: ScopeRead},
		func(_ context.Context, _ Call, in echoIn) (echoOut, error) {
			return echoOut{}, errors.New("secret database detail " + in.Text)
		})
	Add(f.reg, Spec{Name: "panics", Description: "e", Scope: ScopeRead},
		func(_ context.Context, _ Call, in echoIn) (echoOut, error) { panic("boom " + in.Text) })
	return f
}

func (f *fixture) connect(t *testing.T, scopes ...string) *mcp.ClientSession {
	t.Helper()
	srv := f.reg.Build(Call{UserID: testUser, TokenID: "tok-1", Scopes: scopes}, "instructions", mcp.NewSchemaCache())
	ct, st := mcp.NewInMemoryTransports()
	ss, err := srv.Connect(t.Context(), st, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ss.Close() })
	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "1"}, nil)
	cs, err := client.Connect(t.Context(), ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cs.Close() })
	return cs
}

func toolNames(t *testing.T, cs *mcp.ClientSession) []string {
	t.Helper()
	res, err := cs.ListTools(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, tool := range res.Tools {
		names = append(names, tool.Name)
	}
	return names
}

func callTool(t *testing.T, cs *mcp.ClientSession, name string, args any) *mcp.CallToolResult {
	t.Helper()
	res, err := cs.CallTool(t.Context(), &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatalf("CallTool(%s): protocol error %v", name, err)
	}
	return res
}

func text(res *mcp.CallToolResult) string {
	var sb strings.Builder
	for _, c := range res.Content {
		if tc, ok := c.(*mcp.TextContent); ok {
			sb.WriteString(tc.Text)
		}
	}
	return sb.String()
}

func contains(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}

func TestScopeConstantsMatchPAT(t *testing.T) {
	if ScopeRead != pat.ScopeRead || ScopeDrafts != pat.ScopeDrafts || ScopeLinks != pat.ScopeLinks {
		t.Fatal("scope constants differ from the pat package")
	}
}

func TestListFilteredByScope(t *testing.T) {
	f := newFixture(t)
	cases := []struct {
		scopes []string
		want   []string
	}{
		{[]string{ScopeRead}, []string{"read_tool", "tool_error", "internal_fail", "panics"}},
		{[]string{ScopeDrafts}, []string{"draft_tool"}},
		{[]string{ScopeLinks}, []string{"links_tool"}},
		{nil, nil},
	}
	for _, c := range cases {
		got := toolNames(t, f.connect(t, c.scopes...))
		if len(got) != len(c.want) {
			t.Fatalf("scopes %v: tools = %v, want %v", c.scopes, got, c.want)
		}
		for _, w := range c.want {
			if !contains(got, w) {
				t.Errorf("scopes %v: missing %s in %v", c.scopes, w, got)
			}
		}
	}
}

func TestListFilteredByModule(t *testing.T) {
	f := newFixture(t)
	f.enabled.disabled["contracts"] = true
	got := toolNames(t, f.connect(t, ScopeRead, ScopeDrafts, ScopeLinks))
	if contains(got, "draft_tool") || !contains(got, "links_tool") || !contains(got, "read_tool") {
		t.Fatalf("tools = %v", got)
	}
}

func TestModuleLookupOncePerRequest(t *testing.T) {
	f := newFixture(t)
	cs := f.connect(t, ScopeRead, ScopeDrafts, ScopeLinks)
	toolNames(t, cs)
	callTool(t, cs, "draft_tool", map[string]any{"text": "x"})
	if n := f.enabled.calls["contracts"]; n != 1 {
		t.Fatalf("contracts lookups = %d, want 1", n)
	}
}

func TestCallOutsideScope(t *testing.T) {
	f := newFixture(t)
	cs := f.connect(t, ScopeRead)
	res := callTool(t, cs, "draft_tool", map[string]any{"text": "x"})
	if !res.IsError || !strings.HasPrefix(text(res), CodeInsufficientScope) {
		t.Fatalf("result = %v %q", res.IsError, text(res))
	}
}

func TestCallDisabledModule(t *testing.T) {
	f := newFixture(t)
	f.enabled.disabled["contracts"] = true
	cs := f.connect(t, ScopeDrafts)
	res := callTool(t, cs, "draft_tool", map[string]any{"text": "x"})
	if !res.IsError || !strings.HasPrefix(text(res), CodeModuleDisabled) {
		t.Fatalf("result = %v %q", res.IsError, text(res))
	}
}

func TestEnablementErrorIsInternal(t *testing.T) {
	f := newFixture(t)
	f.enabled.err = errors.New("db down")
	cs := f.connect(t, ScopeDrafts)
	res := callTool(t, cs, "draft_tool", map[string]any{"text": "x"})
	if !res.IsError || text(res) != "internal_error: internal error" {
		t.Fatalf("result = %q", text(res))
	}
}

func TestUnknownToolIsProtocolError(t *testing.T) {
	f := newFixture(t)
	cs := f.connect(t, ScopeRead)
	if _, err := cs.CallTool(t.Context(), &mcp.CallToolParams{Name: "nope"}); err == nil {
		t.Fatal("expected a protocol error")
	}
}

func TestToolSuccessAndAnnotations(t *testing.T) {
	f := newFixture(t)
	cs := f.connect(t, ScopeRead)
	res := callTool(t, cs, "read_tool", map[string]any{"text": "hi"})
	if res.IsError {
		t.Fatalf("error: %s", text(res))
	}
	b, _ := json.Marshal(res.StructuredContent)
	if string(b) != `{"echo":"hi"}` {
		t.Fatalf("structured = %s", b)
	}
	list, _ := cs.ListTools(t.Context(), nil)
	for _, tool := range list.Tools {
		if tool.Name == "read_tool" && !tool.Annotations.ReadOnlyHint {
			t.Error("read_tool should be read-only")
		}
	}
}

func TestToolErrorPassesThrough(t *testing.T) {
	f := newFixture(t)
	res := callTool(t, f.connect(t, ScopeRead), "tool_error", map[string]any{"text": "x"})
	if !res.IsError || text(res) != "not_found: thing not found" {
		t.Fatalf("result = %q", text(res))
	}
}

func TestInternalErrorAndPanicAreFixedText(t *testing.T) {
	f := newFixture(t)
	cs := f.connect(t, ScopeRead)
	for _, name := range []string{"internal_fail", "panics"} {
		res := callTool(t, cs, name, map[string]any{"text": "PRIVATE"})
		if !res.IsError || text(res) != "internal_error: internal error" {
			t.Fatalf("%s result = %q", name, text(res))
		}
	}
	if strings.Contains(f.logs.String(), "boom") {
		t.Fatal("panic value must not be logged")
	}
	if !strings.Contains(f.logs.String(), "secret database detail") {
		t.Fatalf("internal error not logged: %s", f.logs)
	}
	if res := callTool(t, cs, "read_tool", map[string]any{"text": "still works"}); res.IsError {
		t.Fatal("server unusable after panic")
	}
}

func TestMetrics(t *testing.T) {
	f := newFixture(t)
	cs := f.connect(t, ScopeRead)
	before := func(tool, outcome string) float64 {
		return testutil.ToFloat64(toolCalls.WithLabelValues(tool, outcome))
	}
	ok, te, in, fb := before("read_tool", "ok"), before("tool_error", "tool_error"), before("internal_fail", "internal_error"), before("draft_tool", "forbidden")
	callTool(t, cs, "read_tool", map[string]any{"text": "x"})
	callTool(t, cs, "tool_error", map[string]any{"text": "x"})
	callTool(t, cs, "internal_fail", map[string]any{"text": "x"})
	callTool(t, cs, "draft_tool", map[string]any{"text": "x"})
	for _, c := range []struct {
		tool, outcome string
		was           float64
	}{{"read_tool", "ok", ok}, {"tool_error", "tool_error", te}, {"internal_fail", "internal_error", in}, {"draft_tool", "forbidden", fb}} {
		if got := before(c.tool, c.outcome); got != c.was+1 {
			t.Errorf("%s/%s = %v, want %v", c.tool, c.outcome, got, c.was+1)
		}
	}
}

func TestLogsHaveToolAndTokenButNoArguments(t *testing.T) {
	f := newFixture(t)
	cs := f.connect(t, ScopeRead)
	callTool(t, cs, "read_tool", map[string]any{"text": "SENSITIVE-ARGUMENT"})
	logs := f.logs.String()
	if !strings.Contains(logs, "tool=read_tool") || !strings.Contains(logs, "tokenId=tok-1") || !strings.Contains(logs, "outcome=ok") {
		t.Fatalf("logs = %s", logs)
	}
	if strings.Contains(logs, "SENSITIVE-ARGUMENT") {
		t.Fatal("arguments logged")
	}
}

func TestCreateDraft(t *testing.T) {
	f := newFixture(t)
	c := Call{UserID: testUser, TokenID: "t"}
	req := DraftRequest{Target: drafts.TargetNone, Input: json.RawMessage(`{"a":1}`), Title: "Invoice", Key: "file-1", Note: "n"}

	first, err := f.reg.CreateDraft(t.Context(), c, req)
	if err != nil || first.Status != drafts.StatusPending {
		t.Fatalf("first = %+v, %v", first, err)
	}
	var te *ToolError
	if _, err := f.reg.CreateDraft(t.Context(), c, req); !errors.As(err, &te) || te.Code != CodeAlreadyPending {
		t.Fatalf("second err = %v", err)
	}
	req.Key = "file-2"
	if _, err := f.reg.CreateDraft(t.Context(), c, req); err != nil {
		t.Fatalf("other key: %v", err)
	}

	req.Key = ""
	a, errA := f.reg.CreateDraft(t.Context(), c, req)
	b, errB := f.reg.CreateDraft(t.Context(), c, req)
	if errA != nil || errB != nil || a.DraftID == b.DraftID {
		t.Fatalf("keyless drafts: %v %v %+v %+v", errA, errB, a, b)
	}

	list, _ := f.drafts.List(t.Context(), testUser, "")
	if len(list) != 4 {
		t.Fatalf("stored %d drafts", len(list))
	}
	for _, d := range list {
		if d.Source.Kind != drafts.SourceMCP || d.Source.Title != "Invoice" || d.Note != "n" {
			t.Errorf("draft = %+v", d)
		}
	}
	if other, _ := f.drafts.List(t.Context(), "someone-else", ""); len(other) != 0 {
		t.Fatal("drafts leaked across users")
	}
}

func TestCreateDraftValidation(t *testing.T) {
	f := newFixture(t)
	c := Call{UserID: testUser}
	base := DraftRequest{Target: drafts.TargetNone, Title: "t"}
	cases := map[string]func(r *DraftRequest){
		"no title":    func(r *DraftRequest) { r.Title = "" },
		"long title":  func(r *DraftRequest) { r.Title = strings.Repeat("x", 201) },
		"long key":    func(r *DraftRequest) { r.Key = strings.Repeat("x", 201) },
		"long note":   func(r *DraftRequest) { r.Note = strings.Repeat("x", 501) },
		"bad target":  func(r *DraftRequest) { r.Target = "car" },
		"bad parent":  func(r *DraftRequest) { r.ParentID = "nope" },
		"array input": func(r *DraftRequest) { r.Input = json.RawMessage(`[]`) },
	}
	for name, mutate := range cases {
		req := base
		mutate(&req)
		var te *ToolError
		if _, err := f.reg.CreateDraft(t.Context(), c, req); !errors.As(err, &te) || te.Code != CodeInvalidInput {
			t.Errorf("%s: err = %v", name, err)
		}
	}
	if list, _ := f.drafts.List(t.Context(), testUser, ""); len(list) != 0 {
		t.Fatal("invalid requests stored drafts")
	}
}

func TestCheckDate(t *testing.T) {
	if CheckDate("d", "2026-02-28") != nil {
		t.Error("valid date rejected")
	}
	for _, bad := range []string{"", "2026-2-3", "2026-02-30", "28.02.2026", "2026-02-28T00:00:00Z"} {
		if CheckDate("d", bad) == nil {
			t.Errorf("%q accepted", bad)
		}
	}
}

func TestAddRejectsInvalidScope(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic")
		}
	}()
	Add(NewRegistrar(Env{}), Spec{Name: "x", Scope: "write"}, func(context.Context, Call, echoIn) (echoOut, error) { return echoOut{}, nil })
}

func TestCreateDraftTrimsAndRejectsBlankTitle(t *testing.T) {
	f := newFixture(t)
	c := Call{UserID: testUser}
	var te *ToolError
	for _, title := range []string{"", "   ", "\t\n"} {
		if _, err := f.reg.CreateDraft(t.Context(), c, DraftRequest{Target: drafts.TargetNone, Title: title}); !errors.As(err, &te) || te.Code != CodeInvalidInput {
			t.Errorf("title %q: err = %v", title, err)
		}
	}
	if _, err := f.reg.CreateDraft(t.Context(), c, DraftRequest{Target: drafts.TargetNone, Title: "  Invoice  ", Note: "  n  ", Key: " k "}); err != nil {
		t.Fatal(err)
	}
	list, _ := f.drafts.List(t.Context(), testUser, "")
	if len(list) != 1 || list[0].Source.Title != "Invoice" || list[0].Note != "n" {
		t.Fatalf("stored = %+v", list)
	}
	if _, err := f.reg.CreateDraft(t.Context(), c, DraftRequest{Target: drafts.TargetNone, Title: "x", Key: "   "}); !errors.As(err, &te) || te.Code != CodeInvalidInput {
		t.Fatalf("blank key: %v", err)
	}
	// the key is trimmed too, so the same key with other whitespace is a repeat
	if _, err := f.reg.CreateDraft(t.Context(), c, DraftRequest{Target: drafts.TargetNone, Title: "x", Key: "k"}); !errors.As(err, &te) || te.Code != CodeAlreadyPending {
		t.Fatalf("trimmed key repeat: %v", err)
	}
}

func TestCheckLenHonoursMinimum(t *testing.T) {
	if CheckLen("f", "ab", 3, 10) == nil {
		t.Error("below the minimum accepted")
	}
	if CheckLen("f", "abc", 3, 10) != nil {
		t.Error("at the minimum rejected")
	}
	if CheckLen("f", "", 1, 10) == nil || CheckLen("f", "", 0, 10) != nil {
		t.Error("required/optional handling changed")
	}
}

func TestPendingDraftCap(t *testing.T) {
	f := newFixture(t)
	f.reg.env.MaxPendingDrafts = 3
	c := Call{UserID: testUser}
	req := DraftRequest{Target: drafts.TargetNone, Title: "t"}
	for i := range 3 {
		if _, err := f.reg.CreateDraft(t.Context(), c, req); err != nil {
			t.Fatalf("draft %d: %v", i, err)
		}
	}
	var te *ToolError
	if _, err := f.reg.CreateDraft(t.Context(), c, req); !errors.As(err, &te) || te.Code != CodeTooManyPending {
		t.Fatalf("over the cap: %v", err)
	}
	// with a key the duplicate answer wins, and another user is unaffected
	req.Key = "same"
	if _, err := f.reg.CreateDraft(t.Context(), Call{UserID: "someone-else"}, req); err != nil {
		t.Fatalf("other user: %v", err)
	}
	if n, _ := f.drafts.List(t.Context(), testUser, ""); len(n) != 3 {
		t.Fatalf("%d drafts stored, want 3", len(n))
	}
	// deciding one frees a slot
	list, _ := f.drafts.List(t.Context(), testUser, drafts.StatusPending)
	if _, err := f.drafts.Decide(t.Context(), testUser, list[0].ID, drafts.StatusRejected, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := f.reg.CreateDraft(t.Context(), c, DraftRequest{Target: drafts.TargetNone, Title: "t"}); err != nil {
		t.Fatalf("after a decision: %v", err)
	}
}

func TestDefaultPendingCap(t *testing.T) {
	if got := NewRegistrar(Env{}).maxPendingDrafts(); got != 500 {
		t.Fatalf("default cap = %d", got)
	}
}

func TestToolAnnotations(t *testing.T) {
	f := newFixture(t)
	Add(f.reg, Spec{Name: "idem_tool", Description: "i", Scope: ScopeLinks, Idempotent: true},
		func(context.Context, Call, echoIn) (echoOut, error) { return echoOut{}, nil })
	cs := f.connect(t, ScopeRead, ScopeDrafts, ScopeLinks)
	list, err := cs.ListTools(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]struct{ readOnly, idempotent bool }{
		"read_tool": {true, true}, "draft_tool": {false, false}, "links_tool": {false, false}, "idem_tool": {false, true},
	}
	for _, tool := range list.Tools {
		a := tool.Annotations
		if a == nil || a.DestructiveHint == nil || *a.DestructiveHint || a.OpenWorldHint == nil || *a.OpenWorldHint {
			t.Errorf("%s: destructive/open-world hints not explicitly false: %+v", tool.Name, a)
		}
		if w, ok := want[tool.Name]; ok && (a.ReadOnlyHint != w.readOnly || a.IdempotentHint != w.idempotent) {
			t.Errorf("%s: readOnly=%v idempotent=%v, want %+v", tool.Name, a.ReadOnlyHint, a.IdempotentHint, w)
		}
	}
}
