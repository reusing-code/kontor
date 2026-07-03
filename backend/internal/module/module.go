package module

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/reusing-code/kontor/backend/internal/storage/migration"
)

// ErrInvalidSection marks import failures caused by the uploaded file rather
// than the server; handlers map it to a 400 response.
var ErrInvalidSection = errors.New("invalid export section")

// Module is implemented by each feature module (contracts, purchases, auto,
// ledger). The server wires all registered modules at startup.
type Module interface {
	ID() string
	// Prefix returns the module's DB key prefix for a user; all module data
	// lives under it.
	Prefix(userID string) []byte
	// Migrations returns the module's schema migrations, applied at startup
	// against the module's own version key.
	Migrations() []migration.Migration
	// RegisterRoutes adds the module's routes; every handler passes through
	// the Router's gate, so module routes cannot bypass enablement checks.
	RegisterRoutes(r *Router)
	// Seed initializes default data for a user. Must be idempotent.
	Seed(ctx context.Context, userID string) error
	// StartBackground launches the module's background services, if any.
	// Each module checks its own configuration and may do nothing.
	StartBackground(ctx context.Context)
	// IsEmpty reports whether the user has no data in this module.
	IsEmpty(ctx context.Context, userID string) (bool, error)
	// ExportSection marshals the user's module data for the export envelope.
	ExportSection(ctx context.Context, userID string) (json.RawMessage, error)
	// ImportSection restores a previously exported section. The module must
	// be empty for the user; callers check IsEmpty first.
	ImportSection(ctx context.Context, userID string, data json.RawMessage, res *ImportResult) error
	// PruneDeadLinks removes references to items that do not exist after an
	// import, e.g. transaction links whose ledger data was not imported.
	PruneDeadLinks(ctx context.Context, userID string, res *ImportResult) error
}

// ImportResult aggregates per-entity restore counts and warnings.
type ImportResult struct {
	Restored map[string]int `json:"restored"`
	Warnings []string       `json:"warnings"`
}

func NewImportResult() *ImportResult {
	return &ImportResult{Restored: map[string]int{}, Warnings: []string{}}
}

func (r *ImportResult) Add(key string, n int) {
	if n != 0 {
		r.Restored[key] += n
	}
}

func (r *ImportResult) Warnf(format string, args ...any) {
	r.Warnings = append(r.Warnings, fmt.Sprintf(format, args...))
}

// Prefix builds the canonical per-module key prefix u/{userID}/mod/{moduleID}/.
func Prefix(userID, moduleID string) []byte {
	return []byte("u/" + userID + "/mod/" + moduleID + "/")
}

// Base provides no-op defaults for optional Module methods.
type Base struct{}

func (Base) Seed(context.Context, string) error { return nil }

func (Base) StartBackground(context.Context) {}

func (Base) Migrations() []migration.Migration { return nil }

func (Base) PruneDeadLinks(context.Context, string, *ImportResult) error { return nil }

// Registry holds the modules wired into this server instance, in
// registration order.
type Registry struct {
	modules []Module
	byID    map[string]Module
}

func NewRegistry(modules ...Module) *Registry {
	r := &Registry{byID: make(map[string]Module, len(modules))}
	for _, m := range modules {
		r.modules = append(r.modules, m)
		r.byID[m.ID()] = m
	}
	return r
}

func (r *Registry) All() []Module { return r.modules }

func (r *Registry) Get(id string) (Module, bool) {
	m, ok := r.byID[id]
	return m, ok
}

func (r *Registry) IDs() []string {
	ids := make([]string, 0, len(r.modules))
	for _, m := range r.modules {
		ids = append(ids, m.ID())
	}
	return ids
}

// Router registers module routes on the shared mux, wrapping every handler
// with the module's gate middleware.
type Router struct {
	mux  *http.ServeMux
	gate func(http.Handler) http.Handler
}

func NewRouter(mux *http.ServeMux, gate func(http.Handler) http.Handler) *Router {
	if gate == nil {
		gate = func(next http.Handler) http.Handler { return next }
	}
	return &Router{mux: mux, gate: gate}
}

func (r *Router) Handle(pattern string, h http.HandlerFunc) {
	r.mux.Handle(pattern, r.gate(h))
}
