package middleware

import (
	"context"
	"slices"
)

type AuthKind string

const (
	AuthJWT AuthKind = "jwt"
	AuthPAT AuthKind = "pat"
)

// Principal is the authenticated caller of a request.
type Principal struct {
	UserID  string
	Kind    AuthKind
	TokenID string
	Scopes  []string
}

const principalKey contextKey = "principal"

// WithPrincipal stores the principal and its user ID, so GetUserID keeps working.
func WithPrincipal(ctx context.Context, p Principal) context.Context {
	return context.WithValue(SetUserID(ctx, p.UserID), principalKey, p)
}

func GetPrincipal(ctx context.Context) (Principal, bool) {
	p, ok := ctx.Value(principalKey).(Principal)
	return p, ok
}

func IsPAT(ctx context.Context) bool {
	p, ok := GetPrincipal(ctx)
	return ok && p.Kind == AuthPAT
}

func GetScopes(ctx context.Context) []string {
	p, _ := GetPrincipal(ctx)
	return p.Scopes
}

func HasScope(ctx context.Context, scope string) bool {
	return slices.Contains(GetScopes(ctx), scope)
}
