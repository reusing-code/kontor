// Package mcptool is the framework for MCP tools: the registrar that modules
// add typed tools to, the per-request server with its scope and module guard,
// error mapping, metrics and the shared draft helper. It is a leaf package
// (SDK, drafts, storage); modules import it, never the other way around.
package mcptool

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/reusing-code/kontor/backend/internal/drafts"
	"github.com/reusing-code/kontor/backend/internal/version"
)

// Scope names; a test pins them to the pat package.
const (
	ScopeRead   = "read"
	ScopeDrafts = "drafts"
	ScopeLinks  = "links"
)

// Spec describes one tool.
type Spec struct {
	Name        string
	Description string
	// Scope is the one personal access token scope the tool needs.
	Scope string
	// Module is the owning module's ID, "" for core tools. A disabled module
	// hides the tool and blocks calls.
	Module   string
	ReadOnly bool
	// Idempotent marks tools that can be repeated without further effect
	// (read-only tools always are).
	Idempotent bool
}

// Call identifies the caller. It comes from the authenticated request, never
// from tool arguments.
type Call struct {
	UserID  string
	TokenID string
	Scopes  []string
}

type Env struct {
	// MaxPendingDrafts caps pending MCP drafts per user; 0 = DefaultMaxPendingDrafts.
	MaxPendingDrafts int
	Drafts           *drafts.Store
	Enabled          func(ctx context.Context, userID, moduleID string) (bool, error)
	Logger           *slog.Logger
}

type registered struct {
	spec    Spec
	install func(srv *mcp.Server, call Call)
}

type Registrar struct {
	env   Env
	tools []registered
}

func NewRegistrar(env Env) *Registrar {
	if env.Logger == nil {
		env.Logger = slog.Default()
	}
	return &Registrar{env: env}
}

func (r *Registrar) Specs() []Spec {
	specs := make([]Spec, len(r.tools))
	for i, t := range r.tools {
		specs[i] = t.spec
	}
	return specs
}

func (r *Registrar) maxPendingDrafts() int {
	if r.env.MaxPendingDrafts > 0 {
		return r.env.MaxPendingDrafts
	}
	return DefaultMaxPendingDrafts
}

func (r *Registrar) Logger() *slog.Logger { return r.env.Logger }

func (r *Registrar) Drafts() *drafts.Store { return r.env.Drafts }

func (r *Registrar) ModuleEnabled(ctx context.Context, userID, moduleID string) (bool, error) {
	if moduleID == "" {
		return true, nil
	}
	return r.env.Enabled(ctx, userID, moduleID)
}

// Add registers a typed tool. The handler returns the output or a *ToolError;
// any other error (and any panic) is logged and reaches the agent as the
// fixed internal_error.
func Add[In, Out any](r *Registrar, spec Spec, h func(ctx context.Context, c Call, in In) (Out, error)) {
	if !slices.Contains([]string{ScopeRead, ScopeDrafts, ScopeLinks}, spec.Scope) {
		panic(fmt.Sprintf("mcptool: tool %q has invalid scope %q", spec.Name, spec.Scope))
	}
	install := func(srv *mcp.Server, call Call) {
		tool := &mcp.Tool{
			Name:        spec.Name,
			Description: spec.Description,
			// Without explicit hints clients assume a non-read-only tool is
			// destructive and open-world; no tool here deletes or leaves Kontor.
			Annotations: &mcp.ToolAnnotations{
				ReadOnlyHint:    spec.ReadOnly,
				IdempotentHint:  spec.ReadOnly || spec.Idempotent,
				DestructiveHint: new(false),
				OpenWorldHint:   new(false),
			},
		}
		mcp.AddTool(srv, tool, func(ctx context.Context, _ *mcp.CallToolRequest, in In) (*mcp.CallToolResult, Out, error) {
			var zero Out
			out, err := run(r, spec, call, func() (Out, error) { return h(ctx, call, in) })
			if err != nil {
				return nil, zero, err
			}
			return nil, out, nil
		})
	}
	r.tools = append(r.tools, registered{spec: spec, install: install})
}

func run[Out any](r *Registrar, spec Spec, call Call, fn func() (Out, error)) (out Out, err error) {
	start := time.Now()
	outcome := outcomeOK
	defer func() {
		if rec := recover(); rec != nil {
			r.env.Logger.Error("mcp tool panicked", "tool", spec.Name, "tokenId", call.TokenID)
			err = internalError()
			outcome = outcomeInternal
			var zero Out
			out = zero
		}
		r.record(spec.Name, call.TokenID, outcome, time.Since(start))
	}()

	out, err = fn()
	if err == nil {
		return out, nil
	}
	var te *ToolError
	if errors.As(err, &te) {
		outcome = outcomeToolErr
		return out, te
	}
	r.env.Logger.Error("mcp tool failed", "tool", spec.Name, "tokenId", call.TokenID, "error", err)
	outcome = outcomeInternal
	return out, internalError()
}

func (r *Registrar) record(tool, tokenID, outcome string, d time.Duration) {
	toolCalls.WithLabelValues(tool, outcome).Inc()
	toolDuration.WithLabelValues(tool).Observe(d.Seconds())
	r.env.Logger.Info("mcp tool call", "tool", tool, "tokenId", tokenID, "outcome", outcome, "duration", d.String())
}

// Build returns a server for one request: every tool installed, the guard
// attached. Scope and module state are evaluated per request, so revoking a
// token or toggling a module applies on the next request.
func (r *Registrar) Build(call Call, instructions string, cache *mcp.SchemaCache) *mcp.Server {
	srv := mcp.NewServer(
		&mcp.Implementation{Name: "kontor", Version: version.Get().Version},
		&mcp.ServerOptions{Instructions: instructions, SchemaCache: cache},
	)
	for _, t := range r.tools {
		t.install(srv, call)
	}
	srv.AddReceivingMiddleware(r.guard(call))
	return srv
}
