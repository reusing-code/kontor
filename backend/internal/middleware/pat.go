package middleware

import (
	"context"
	"errors"
)

// PATPrefix starts every personal access token.
const PATPrefix = "kpat_"

// ErrInvalidPAT is returned by a PATVerifier for unknown, revoked and expired
// tokens alike.
var ErrInvalidPAT = errors.New("invalid personal access token")

// PATIdentity is what a verified personal access token stands for.
type PATIdentity struct {
	UserID  string
	TokenID string
	Scopes  []string
}

// PATVerifier resolves a raw personal access token.
type PATVerifier interface {
	Authenticate(ctx context.Context, raw string) (PATIdentity, error)
}
