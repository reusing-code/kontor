package middleware

import (
	"context"
	"errors"
	"net/http"

	"github.com/golang-jwt/jwt/v5"
)

// JWTClaims are the claims of a session JWT. TokenVersion is the user's
// token version at issue time; a token without it counts as version 0.
type JWTClaims struct {
	jwt.RegisteredClaims
	TokenVersion int `json:"tv,omitempty"`
}

// ErrUnknownUser is returned by a TokenVersions for a user that does not
// exist.
var ErrUnknownUser = errors.New("unknown user")

// TokenVersions returns a user's current token version. A JWT with another
// version is refused, so bumping it invalidates every issued session.
type TokenVersions interface {
	TokenVersion(ctx context.Context, userID string) (int, error)
}

// WithTokenVersions checks the token version of every JWT.
func WithTokenVersions(v TokenVersions) AuthOption {
	return func(c *authConfig) { c.versions = v }
}

func parseJWT(secret []byte, tokenStr string) (JWTClaims, bool) {
	var claims JWTClaims
	token, err := jwt.ParseWithClaims(tokenStr, &claims, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, jwt.ErrSignatureInvalid
		}
		return secret, nil
	})
	if err != nil || !token.Valid || claims.Subject == "" {
		return JWTClaims{}, false
	}
	return claims, true
}

// authenticateJWT returns the principal and the HTTP status of the check.
func (c authConfig) authenticateJWT(ctx context.Context, secret []byte, tokenStr string) (Principal, int) {
	claims, ok := parseJWT(secret, tokenStr)
	if !ok {
		return Principal{}, http.StatusUnauthorized
	}
	if c.versions != nil {
		current, err := c.versions.TokenVersion(ctx, claims.Subject)
		switch {
		case errors.Is(err, ErrUnknownUser):
			return Principal{}, http.StatusUnauthorized
		case err != nil:
			c.logger.Error("checking token version", "error", err)
			return Principal{}, http.StatusInternalServerError
		case current != claims.TokenVersion:
			return Principal{}, http.StatusUnauthorized
		}
	}
	return Principal{UserID: claims.Subject, Kind: AuthJWT}, http.StatusOK
}
