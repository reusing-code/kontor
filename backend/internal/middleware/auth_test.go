package middleware

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

var testSecret = []byte("test-secret")

type fakeVerifier struct {
	identities map[string]PATIdentity
	err        error
}

func (f fakeVerifier) Authenticate(_ context.Context, raw string) (PATIdentity, error) {
	if f.err != nil {
		return PATIdentity{}, f.err
	}
	id, ok := f.identities[raw]
	if !ok {
		return PATIdentity{}, ErrInvalidPAT
	}
	return id, nil
}

func signJWT(t *testing.T, sub string) string {
	t.Helper()
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.RegisteredClaims{
		Subject:   sub,
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
	})
	s, err := tok.SignedString(testSecret)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func probe(t *testing.T, mw Middleware, auth string) (*httptest.ResponseRecorder, *Principal) {
	t.Helper()
	var got *Principal
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p, ok := GetPrincipal(r.Context())
		if ok {
			got = &p
		}
		if GetUserID(r.Context()) == "" {
			t.Error("GetUserID empty after auth")
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	req := httptest.NewRequest("GET", "/x", nil)
	if auth != "" {
		req.Header.Set("Authorization", auth)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec, got
}

func patVerifier() fakeVerifier {
	return fakeVerifier{identities: map[string]PATIdentity{
		"kpat_good": {UserID: "u1", TokenID: "t1", Scopes: []string{"read", "drafts"}},
	}}
}

func TestAuthJWT(t *testing.T) {
	mw := Auth(testSecret)

	rec, p := probe(t, mw, "Bearer "+signJWT(t, "u1"))
	if rec.Code != http.StatusNoContent || p == nil || p.UserID != "u1" || p.Kind != AuthJWT || p.Scopes != nil {
		t.Fatalf("valid jwt: %d %+v", rec.Code, p)
	}

	for name, header := range map[string]string{
		"missing":    "",
		"not bearer": "Basic abc",
		"garbage":    "Bearer nope",
		"wrong secret": "Bearer " + func() string {
			s, _ := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.RegisteredClaims{Subject: "u1"}).SignedString([]byte("other"))
			return s
		}(),
		"empty subject": "Bearer " + signJWT(t, ""),
	} {
		t.Run(name, func(t *testing.T) {
			rec, _ := probe(t, mw, header)
			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("status = %d", rec.Code)
			}
			if rec.Header().Get("WWW-Authenticate") != "" {
				t.Fatal("unexpected challenge")
			}
		})
	}
}

func TestAuthPAT(t *testing.T) {
	mw := Auth(testSecret, WithPATs(patVerifier()))

	rec, p := probe(t, mw, "Bearer kpat_good")
	if rec.Code != http.StatusNoContent || p == nil {
		t.Fatalf("valid pat: %d", rec.Code)
	}
	if p.Kind != AuthPAT || p.UserID != "u1" || p.TokenID != "t1" || len(p.Scopes) != 2 {
		t.Fatalf("principal = %+v", p)
	}

	h := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !IsPAT(r.Context()) || !HasScope(r.Context(), "read") || HasScope(r.Context(), "links") || len(GetScopes(r.Context())) != 2 {
			t.Error("context helpers wrong")
		}
	}))
	req := httptest.NewRequest("GET", "/x", nil)
	req.Header.Set("Authorization", "Bearer kpat_good")
	h.ServeHTTP(httptest.NewRecorder(), req)

	for name, header := range map[string]string{
		"unknown":   "Bearer kpat_unknown",
		"empty":     "Bearer kpat_",
		"wrong pfx": "Bearer xpat_good",
	} {
		t.Run(name, func(t *testing.T) {
			if rec, _ := probe(t, mw, header); rec.Code != http.StatusUnauthorized {
				t.Fatalf("status = %d", rec.Code)
			}
		})
	}
}

func TestAuthPATWithoutVerifier(t *testing.T) {
	if rec, _ := probe(t, Auth(testSecret), "Bearer kpat_good"); rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestAuthPATVerifierError(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, nil))
	mw := Auth(testSecret, WithAuthLogger(logger), WithPATs(fakeVerifier{err: errors.New("db down")}))
	rec, _ := probe(t, mw, "Bearer kpat_secretvalue")
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d", rec.Code)
	}
	if strings.Contains(buf.String(), "kpat_secretvalue") || strings.Contains(rec.Body.String(), "db down") {
		t.Fatalf("leak: log=%q body=%q", buf.String(), rec.Body)
	}
}

func TestAuthPATOnly(t *testing.T) {
	mw := Auth(testSecret, WithPATs(patVerifier()), PATOnly(), WithChallenge("Bearer"))

	rec, _ := probe(t, mw, "Bearer "+signJWT(t, "u1"))
	if rec.Code != http.StatusUnauthorized || rec.Header().Get("WWW-Authenticate") != "Bearer" {
		t.Fatalf("jwt on pat-only: %d %q", rec.Code, rec.Header().Get("WWW-Authenticate"))
	}
	rec, _ = probe(t, mw, "")
	if rec.Code != http.StatusUnauthorized || rec.Header().Get("WWW-Authenticate") != "Bearer" {
		t.Fatalf("missing: %d", rec.Code)
	}
	rec, _ = probe(t, mw, "Bearer kpat_unknown")
	if rec.Code != http.StatusUnauthorized || rec.Header().Get("WWW-Authenticate") != "Bearer" {
		t.Fatalf("unknown: %d", rec.Code)
	}
	if rec, p := probe(t, mw, "Bearer kpat_good"); rec.Code != http.StatusNoContent || p == nil {
		t.Fatalf("valid pat: %d", rec.Code)
	}
}

func TestAuthUnauthorizedBodyIsConstant(t *testing.T) {
	mw := Auth(testSecret, WithPATs(patVerifier()))
	a, _ := probe(t, mw, "Bearer kpat_unknown")
	b, _ := probe(t, mw, "")
	if a.Body.String() != b.Body.String() {
		t.Fatalf("bodies differ: %q vs %q", a.Body, b.Body)
	}
}

func TestAuthLogsNeverContainToken(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, nil))
	mw := Chain(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}), Logging(logger), Auth(testSecret, WithPATs(patVerifier())))

	for _, token := range []string{"kpat_good", "kpat_bad"} {
		req := httptest.NewRequest("GET", "/api/v1/x", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		mw.ServeHTTP(httptest.NewRecorder(), req)
	}
	if strings.Contains(buf.String(), "kpat_") {
		t.Fatalf("log contains token: %s", buf.String())
	}
	if buf.Len() == 0 {
		t.Fatal("expected request logs")
	}
}

func TestAuthHostileHeaders(t *testing.T) {
	mw := Auth(testSecret, WithPATs(patVerifier()))
	algNone := "eyJhbGciOiJub25lIiwidHlwIjoiSldUIn0.eyJzdWIiOiJ1MSJ9."
	for name, header := range map[string]string{
		"lowercase scheme":     "bearer kpat_good",
		"double space":         "Bearer  kpat_good",
		"trailing space":       "Bearer kpat_good ",
		"trailing newline":     "Bearer kpat_good\n",
		"uppercase prefix":     "Bearer KPAT_good",
		"prefix only, no body": "Bearer kpat_",
		"alg none jwt":         "Bearer " + algNone,
		"bare scheme":          "Bearer",
		"bare scheme + space":  "Bearer ",
		"basic":                "Basic a3BhdF9nb29k",
	} {
		t.Run(name, func(t *testing.T) {
			rec, p := probe(t, mw, header)
			if rec.Code != http.StatusUnauthorized || p != nil {
				t.Fatalf("status = %d, principal = %+v", rec.Code, p)
			}
		})
	}
}

func TestAuthPATTokenNeverFallsBackToJWT(t *testing.T) {
	// A kpat_ token is never parsed as a JWT, even when the verifier rejects it.
	mw := Auth(testSecret, WithPATs(fakeVerifier{}))
	if rec, _ := probe(t, mw, "Bearer "+PATPrefix+signJWT(t, "u1")); rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d", rec.Code)
	}
}
