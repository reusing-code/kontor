package middleware

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/golang-jwt/jwt/v5"
)

type contextKey string

const userIDKey contextKey = "userID"

func SetUserID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, userIDKey, id)
}

func GetUserID(ctx context.Context) string {
	if id, ok := ctx.Value(userIDKey).(string); ok {
		return id
	}
	return ""
}

type authConfig struct {
	pats      PATVerifier
	patOnly   bool
	challenge string
	logger    *slog.Logger
}

type AuthOption func(*authConfig)

// WithPATs lets the middleware accept personal access tokens.
func WithPATs(v PATVerifier) AuthOption { return func(c *authConfig) { c.pats = v } }

// PATOnly rejects JWTs; used for endpoints that only agents call.
func PATOnly() AuthOption { return func(c *authConfig) { c.patOnly = true } }

// WithChallenge sets WWW-Authenticate on 401 responses.
func WithChallenge(scheme string) AuthOption { return func(c *authConfig) { c.challenge = scheme } }

// WithAuthLogger sets the logger for internal verification errors.
func WithAuthLogger(l *slog.Logger) AuthOption { return func(c *authConfig) { c.logger = l } }

func Auth(secret []byte, opts ...AuthOption) Middleware {
	cfg := authConfig{logger: slog.Default()}
	for _, o := range opts {
		o(&cfg)
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			header := r.Header.Get("Authorization")
			if !strings.HasPrefix(header, "Bearer ") {
				cfg.unauthorized(w)
				return
			}
			tokenStr := strings.TrimPrefix(header, "Bearer ")

			var (
				p  Principal
				ok bool
			)
			if strings.HasPrefix(tokenStr, PATPrefix) {
				var status int
				p, status = cfg.authenticatePAT(r.Context(), tokenStr)
				switch status {
				case http.StatusUnauthorized:
					cfg.unauthorized(w)
					return
				case http.StatusInternalServerError:
					writeJSONError(w, http.StatusInternalServerError, "internal error")
					return
				}
			} else {
				if cfg.patOnly {
					cfg.unauthorized(w)
					return
				}
				if p, ok = authenticateJWT(secret, tokenStr); !ok {
					cfg.unauthorized(w)
					return
				}
			}

			next.ServeHTTP(w, r.WithContext(WithPrincipal(r.Context(), p)))
		})
	}
}

func (c authConfig) authenticatePAT(ctx context.Context, raw string) (Principal, int) {
	if c.pats == nil {
		return Principal{}, http.StatusUnauthorized
	}
	id, err := c.pats.Authenticate(ctx, raw)
	switch {
	case errors.Is(err, ErrInvalidPAT):
		return Principal{}, http.StatusUnauthorized
	case err != nil:
		c.logger.Error("verifying personal access token", "error", err)
		return Principal{}, http.StatusInternalServerError
	}
	return Principal{UserID: id.UserID, Kind: AuthPAT, TokenID: id.TokenID, Scopes: id.Scopes}, http.StatusOK
}

func authenticateJWT(secret []byte, tokenStr string) (Principal, bool) {
	token, err := jwt.Parse(tokenStr, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, jwt.ErrSignatureInvalid
		}
		return secret, nil
	})
	if err != nil || !token.Valid {
		return Principal{}, false
	}
	sub, err := token.Claims.GetSubject()
	if err != nil || sub == "" {
		return Principal{}, false
	}
	return Principal{UserID: sub, Kind: AuthJWT}, true
}

func (c authConfig) unauthorized(w http.ResponseWriter) {
	if c.challenge != "" {
		w.Header().Set("WWW-Authenticate", c.challenge)
	}
	writeUnauthorized(w)
}

func writeJSONError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

func writeUnauthorized(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnauthorized)
	json.NewEncoder(w).Encode(map[string]string{"error": "unauthorized"})
}
