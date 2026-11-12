// Package mcpserver mounts the MCP endpoint: a stateless streamable HTTP
// handler that builds one server per request, bound to the caller's token.
package mcpserver

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/reusing-code/kontor/backend/internal/mcptool"
	"github.com/reusing-code/kontor/backend/internal/middleware"
	"github.com/reusing-code/kontor/backend/internal/paperless"
)

type ModuleStatus struct {
	ID      string `json:"id"`
	Enabled bool   `json:"enabled"`
}

// EntityRef locates an entity in the UI; Path is relative to PublicURL.
type EntityRef struct{ ModuleID, Path string }

// EntityLookup resolves an entity of the user; storage.ErrNotFound when it
// does not exist.
type EntityLookup func(ctx context.Context, userID, entityType string, id uuid.UUID) (EntityRef, error)

type Deps struct {
	// Registrar is built by the caller with the module tools registered.
	Registrar *mcptool.Registrar
	Paperless *paperless.Service
	Modules   func(ctx context.Context, userID string) ([]ModuleStatus, error)
	Entities  EntityLookup
	// PublicURL is config.PublicURL; empty means unset.
	PublicURL string
}

type serverKey struct{}

// Handler serves /mcp. It must sit behind middleware.Auth(PATOnly()); it
// re-checks the principal as defence in depth.
func Handler(d Deps) http.Handler {
	cache := mcp.NewSchemaCache()
	inner := mcp.NewStreamableHTTPHandler(func(r *http.Request) *mcp.Server {
		srv, _ := r.Context().Value(serverKey{}).(*mcp.Server)
		return srv
	}, &mcp.StreamableHTTPOptions{
		Stateless: true,
		// Requests carry a bearer token, so DNS rebinding is not a concern, and
		// the protection would reject a reverse proxy on the same host that
		// forwards the public Host header.
		DisableLocalhostProtection: true,
	})

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p, ok := middleware.GetPrincipal(r.Context())
		if !ok || p.Kind != middleware.AuthPAT {
			w.Header().Set("WWW-Authenticate", "Bearer")
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "unauthorized"})
			return
		}
		// The SDK asks for the server more than once per request; build it once.
		srv := d.Registrar.Build(mcptool.Call{UserID: p.UserID, TokenID: p.TokenID, Scopes: p.Scopes}, Instructions, cache)
		inner.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), serverKey{}, srv)))
	})
}
