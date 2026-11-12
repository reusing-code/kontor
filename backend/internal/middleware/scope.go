package middleware

import (
	"encoding/json"
	"net/http"
	"slices"
)

// Rule says what a personal access token needs to reach a route.
type Rule string

const (
	RuleRead    Rule = "read"
	RuleDrafts  Rule = "drafts"
	RuleLinks   Rule = "links"
	RuleDenied  Rule = "denied"
	RuleJWTOnly Rule = "jwt-only"
)

func allowed(scopes []string, rule Rule) bool {
	switch rule {
	case RuleRead, RuleDrafts, RuleLinks:
		return slices.Contains(scopes, string(rule))
	default:
		return false
	}
}

// ScopeGuard classifies requests by the pattern the mux matched. JWT requests
// pass untouched; for personal access tokens a pattern missing from the table
// is denied. A request without a principal or of an unknown kind is rejected (401): the guard sits
// behind Auth and must fail closed if it is ever mounted without it. Requests
// that match no pattern (404, 405, redirects) go to the mux, which answers
// them as usual.
func ScopeGuard(mux *http.ServeMux, table map[string]Rule) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p, ok := GetPrincipal(r.Context())
		if !ok {
			writeUnauthorized(w)
			return
		}
		switch p.Kind {
		case AuthJWT:
			mux.ServeHTTP(w, r)
			return
		case AuthPAT:
		default:
			writeUnauthorized(w)
			return
		}
		_, pattern := mux.Handler(r)
		if pattern == "" {
			mux.ServeHTTP(w, r)
			return
		}
		if !allowed(p.Scopes, table[pattern]) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusForbidden)
			_ = json.NewEncoder(w).Encode(map[string]string{
				"error": "insufficient scope",
				"code":  "insufficient_scope",
			})
			return
		}
		mux.ServeHTTP(w, r)
	})
}
