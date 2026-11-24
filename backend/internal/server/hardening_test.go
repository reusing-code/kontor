package server

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/reusing-code/kontor/backend/internal/config"
	"github.com/reusing-code/kontor/backend/internal/pat"
)

func TestOversizedJSONBodyIsRefused(t *testing.T) {
	a := newTestApp(t)
	_, jwt := a.register(t, "limits@example.com")
	big := `{"name":"` + strings.Repeat("a", 1<<20) + `"}`
	for _, path := range []string{"/api/v1/modules/contracts/categories", "/api/v1/import"} {
		status, body := a.request(t, "POST", path, jwt, big)
		want := http.StatusRequestEntityTooLarge
		if path == "/api/v1/import" {
			// the export import has its own, larger limit
			want = http.StatusBadRequest
		}
		if status != want {
			t.Errorf("%s = %d %s, want %d", path, status, body, want)
		}
	}
	status, body := a.request(t, "POST", "/api/v1/auth/login", "", big)
	if status != http.StatusRequestEntityTooLarge || !strings.Contains(body, "body_too_large") {
		t.Errorf("login = %d %s", status, body)
	}
}

func TestOversizedMCPBodyIsRefused(t *testing.T) {
	a := newTestApp(t)
	userID, _ := a.register(t, "mcp-limit@example.com")
	_, raw := a.newPAT(t, userID, nil, pat.ScopeRead)
	big := `{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{"x":"` + strings.Repeat("a", maxMCPBodyBytes) + `"}}`
	req, _ := http.NewRequest("POST", a.srv.URL+"/mcp", strings.NewReader(big))
	req.Header.Set("Authorization", "Bearer "+raw)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest && resp.StatusCode != http.StatusRequestEntityTooLarge {
		t.Errorf("status = %d, want 400 or 413", resp.StatusCode)
	}
}

func TestRegistrationClosedAfterFirstUserByDefault(t *testing.T) {
	a := newTestApp(t, func(c *config.Config) { c.Registration = config.RegistrationFirstUser })
	status, body := a.request(t, "GET", "/api/v1/auth/config", "", "")
	if status != http.StatusOK || !strings.Contains(body, `"registrationOpen":true`) {
		t.Fatalf("auth config = %d %s", status, body)
	}
	a.register(t, "first@example.com")
	status, body = a.request(t, "POST", "/api/v1/auth/register", "", `{"email":"second@example.com","password":"password123"}`)
	if status != http.StatusForbidden || !strings.Contains(body, "registration_closed") {
		t.Fatalf("second register = %d %s", status, body)
	}
	if status, body = a.request(t, "GET", "/api/v1/auth/config", "", ""); !strings.Contains(body, `"registrationOpen":false`) {
		t.Fatalf("auth config = %d %s", status, body)
	}
}

func TestOldJWTIsRefusedAfterPasswordChange(t *testing.T) {
	a := newTestApp(t)
	userID, oldJWT := a.register(t, "pw@example.com")
	_, raw := a.newPAT(t, userID, nil, pat.ScopeRead)

	// a second session of the same user, e.g. another browser
	status, body := a.request(t, "POST", "/api/v1/auth/login", "", `{"email":"pw@example.com","password":"password123"}`)
	if status != http.StatusOK {
		t.Fatalf("login = %d %s", status, body)
	}
	var login struct{ Token string }
	_ = json.Unmarshal([]byte(body), &login)

	status, body = a.request(t, "PUT", "/api/v1/settings/password", oldJWT, `{"currentPassword":"password123","newPassword":"another-password"}`)
	if status != http.StatusOK {
		t.Fatalf("change = %d %s", status, body)
	}
	var changed struct{ Token string }
	if err := json.Unmarshal([]byte(body), &changed); err != nil || changed.Token == "" {
		t.Fatalf("change body = %s", body)
	}

	for name, tok := range map[string]string{"changing session": oldJWT, "other session": login.Token} {
		if status, _ := a.request(t, "GET", "/api/v1/settings", tok, ""); status != http.StatusUnauthorized {
			t.Errorf("%s after the change = %d, want 401", name, status)
		}
	}
	if status, body := a.request(t, "GET", "/api/v1/settings", changed.Token, ""); status != http.StatusOK {
		t.Errorf("new token = %d %s", status, body)
	}
	if status, body := a.request(t, "GET", "/api/v1/settings", raw, ""); status != http.StatusOK {
		t.Errorf("personal access token = %d %s", status, body)
	}
	status, body = a.request(t, "POST", "/api/v1/auth/login", "", `{"email":"pw@example.com","password":"another-password"}`)
	_ = json.Unmarshal([]byte(body), &login)
	if status != http.StatusOK {
		t.Fatalf("login with new password = %d %s", status, body)
	}
	if status, _ := a.request(t, "GET", "/api/v1/settings", login.Token, ""); status != http.StatusOK {
		t.Errorf("fresh login token = %d", status)
	}
}

func TestJWTWithoutVersionStillWorksForUnchangedUsers(t *testing.T) {
	a := newTestApp(t)
	userID, _ := a.register(t, "legacy@example.com")
	legacy := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.RegisteredClaims{
		Subject:   userID,
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
	})
	signed, err := legacy.SignedString(appJWTSecret)
	if err != nil {
		t.Fatal(err)
	}
	if status, body := a.request(t, "GET", "/api/v1/settings", signed, ""); status != http.StatusOK {
		t.Fatalf("token issued before token versions = %d %s", status, body)
	}

	unknown := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.RegisteredClaims{Subject: "00000000-0000-0000-0000-00000000dead"})
	signed, _ = unknown.SignedString(appJWTSecret)
	if status, _ := a.request(t, "GET", "/api/v1/settings", signed, ""); status != http.StatusUnauthorized {
		t.Errorf("token of an unknown user = %d, want 401", status)
	}
}
