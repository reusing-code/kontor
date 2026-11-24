package middleware

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type fakeVersions map[string]int

func (f fakeVersions) TokenVersion(_ context.Context, userID string) (int, error) {
	if userID == "broken" {
		return 0, errors.New("db down")
	}
	v, ok := f[userID]
	if !ok {
		return 0, ErrUnknownUser
	}
	return v, nil
}

func signVersioned(t *testing.T, sub string, version int) string {
	t.Helper()
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, JWTClaims{
		RegisteredClaims: jwt.RegisteredClaims{Subject: sub, ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour))},
		TokenVersion:     version,
	})
	s, err := tok.SignedString(testSecret)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestAuth_TokenVersions(t *testing.T) {
	mw := Auth(testSecret, WithTokenVersions(fakeVersions{"u0": 0, "u2": 2}))
	tests := []struct {
		name  string
		token string
		want  int
	}{
		{"current version", signVersioned(t, "u2", 2), http.StatusNoContent},
		{"old version", signVersioned(t, "u2", 1), http.StatusUnauthorized},
		{"no version claim counts as 0", signJWT(t, "u0"), http.StatusNoContent},
		{"no version claim after a bump", signJWT(t, "u2"), http.StatusUnauthorized},
		{"unknown user", signVersioned(t, "gone", 0), http.StatusUnauthorized},
		{"store error", signVersioned(t, "broken", 0), http.StatusInternalServerError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if rec, _ := probe(t, mw, "Bearer "+tt.token); rec.Code != tt.want {
				t.Errorf("status = %d, want %d", rec.Code, tt.want)
			}
		})
	}
}
