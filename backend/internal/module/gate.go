package module

import (
	"context"
	"encoding/json"
	"net/http"
	"slices"

	"github.com/reusing-code/kontor/backend/internal/middleware"
)

// EnablementSource reports whether a user has a module enabled; implemented
// by the core settings store.
type EnablementSource interface {
	ModuleEnabled(ctx context.Context, userID, moduleID string) (bool, error)
}

func writeGateError(w http.ResponseWriter, status int, code, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg, "code": code})
}

func checkEnabled(src EnablementSource, moduleID string, w http.ResponseWriter, r *http.Request) bool {
	userID := middleware.GetUserID(r.Context())
	enabled, err := src.ModuleEnabled(r.Context(), userID, moduleID)
	if err != nil {
		writeGateError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return false
	}
	if !enabled {
		writeGateError(w, http.StatusForbidden, "module_disabled", "the "+moduleID+" module is disabled")
		return false
	}
	return true
}

// Gate rejects requests from users who disabled the module. It runs after
// the auth middleware, which put the user ID into the request context.
func Gate(moduleID string, src EnablementSource) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !checkEnabled(src, moduleID, w, r) {
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// GateParam gates routes whose module is a {module} path parameter, such as
// the shared category routes. Unknown modules yield 404.
func GateParam(src EnablementSource, allowed ...string) func(http.HandlerFunc) http.HandlerFunc {
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			moduleID := r.PathValue("module")
			if !slices.Contains(allowed, moduleID) {
				writeGateError(w, http.StatusNotFound, "unknown_module", "unknown module")
				return
			}
			if !checkEnabled(src, moduleID, w, r) {
				return
			}
			next(w, r)
		}
	}
}
