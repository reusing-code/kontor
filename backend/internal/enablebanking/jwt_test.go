package enablebanking_test

import (
	"crypto/rand"
	"crypto/rsa"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/reusing-code/kontor/backend/internal/enablebanking"
)

type captureTransport struct {
	mu     sync.Mutex
	tokens []string
}

func (c *captureTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	c.mu.Lock()
	c.tokens = append(c.tokens, strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
	c.mu.Unlock()
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": {"application/json"}},
		Body:       io.NopCloser(strings.NewReader(`{"aspsps":[]}`)),
		Request:    r,
	}, nil
}

func TestJWTClaimsAndSignature(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 11, 13, 10, 0, 0, 0, time.UTC)
	capture := &captureTransport{}
	client, err := enablebanking.New(enablebanking.Config{
		BaseURL: "http://eb.test", AppID: "app-1", Key: key,
		HTTP: &http.Client{Transport: capture}, Now: func() time.Time { return now },
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.ASPSPs(t.Context(), "DE"); err != nil {
		t.Fatal(err)
	}

	claims := jwt.RegisteredClaims{}
	tok, err := jwt.ParseWithClaims(capture.tokens[0], &claims, func(*jwt.Token) (any, error) { return &key.PublicKey, nil },
		jwt.WithValidMethods([]string{"RS256"}), jwt.WithTimeFunc(func() time.Time { return now }))
	if err != nil {
		t.Fatalf("parsing token: %v", err)
	}
	if tok.Header["alg"] != "RS256" || tok.Header["typ"] != "JWT" || tok.Header["kid"] != "app-1" {
		t.Fatalf("header = %v", tok.Header)
	}
	if claims.Issuer != "enablebanking.com" {
		t.Fatalf("iss = %q", claims.Issuer)
	}
	if len(claims.Audience) != 1 || claims.Audience[0] != "api.enablebanking.com" {
		t.Fatalf("aud = %v", claims.Audience)
	}
	if !claims.IssuedAt.Equal(now) {
		t.Fatalf("iat = %v, want %v", claims.IssuedAt, now)
	}
	if lifetime := claims.ExpiresAt.Sub(claims.IssuedAt.Time); lifetime <= 0 || lifetime > 24*time.Hour {
		t.Fatalf("exp - iat = %v", lifetime)
	}
}

func TestJWTCachedAndRefreshedNearExpiry(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 11, 13, 10, 0, 0, 0, time.UTC)
	capture := &captureTransport{}
	client, err := enablebanking.New(enablebanking.Config{
		BaseURL: "http://eb.test", AppID: "app-1", Key: key,
		HTTP: &http.Client{Transport: capture}, Now: func() time.Time { return now },
	})
	if err != nil {
		t.Fatal(err)
	}
	call := func() string {
		t.Helper()
		if _, err := client.ASPSPs(t.Context(), "DE"); err != nil {
			t.Fatal(err)
		}
		return capture.tokens[len(capture.tokens)-1]
	}

	first := call()
	now = now.Add(30 * time.Minute)
	if second := call(); second != first {
		t.Fatal("token was not reused within its lifetime")
	}
	now = now.Add(26 * time.Minute) // 4 minutes before exp: inside the refresh margin
	if third := call(); third == first {
		t.Fatal("token was not refreshed shortly before expiry")
	}
}
