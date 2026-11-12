// Package mcptooltest wires a Registrar to an in-memory MCP client so module
// tool tests can call tools the way an agent does.
package mcptooltest

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/reusing-code/kontor/backend/internal/drafts"
	"github.com/reusing-code/kontor/backend/internal/mcptool"
	"github.com/reusing-code/kontor/backend/internal/storage"
)

type Env struct {
	Reg    *mcptool.Registrar
	Drafts *drafts.Store
}

// New builds a registrar over the engine; enabled is usually core.Store.ModuleEnabled.
func New(t *testing.T, engine *storage.Engine, enabled func(ctx context.Context, userID, moduleID string) (bool, error)) *Env {
	t.Helper()
	ds := drafts.NewStore(engine)
	return &Env{
		Drafts: ds,
		Reg:    mcptool.NewRegistrar(mcptool.Env{Drafts: ds, Enabled: enabled, Logger: slog.New(slog.DiscardHandler)}),
	}
}

// Connect returns a client session bound to a token of the user with the scopes.
func (e *Env) Connect(t *testing.T, userID string, scopes ...string) *mcp.ClientSession {
	t.Helper()
	srv := e.Reg.Build(mcptool.Call{UserID: userID, TokenID: "test-token", Scopes: scopes}, "", mcp.NewSchemaCache())
	ct, st := mcp.NewInMemoryTransports()
	ss, err := srv.Connect(t.Context(), st, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ss.Close() })
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "1"}, nil).Connect(t.Context(), ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cs.Close() })
	return cs
}

// ToolNames lists the tool names the session sees.
func ToolNames(t *testing.T, cs *mcp.ClientSession) []string {
	t.Helper()
	res, err := cs.ListTools(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	names := []string{}
	for _, tool := range res.Tools {
		names = append(names, tool.Name)
	}
	return names
}

// Call invokes a tool and returns the result and its text content.
func Call(t *testing.T, cs *mcp.ClientSession, name string, args any) (*mcp.CallToolResult, string) {
	t.Helper()
	res, err := cs.CallTool(t.Context(), &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatalf("CallTool(%s): protocol error %v", name, err)
	}
	var sb strings.Builder
	for _, c := range res.Content {
		if tc, ok := c.(*mcp.TextContent); ok {
			sb.WriteString(tc.Text)
		}
	}
	return res, sb.String()
}

// Decode converts a successful result's structured content into T.
func Decode[T any](t *testing.T, res *mcp.CallToolResult) T {
	t.Helper()
	if res.IsError {
		t.Fatalf("tool returned an error: %+v", res.Content)
	}
	b, err := json.Marshal(res.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	var v T
	if err := json.Unmarshal(b, &v); err != nil {
		t.Fatal(err)
	}
	return v
}
