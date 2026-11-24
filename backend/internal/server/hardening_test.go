package server

import (
	"net/http"
	"strings"
	"testing"

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
