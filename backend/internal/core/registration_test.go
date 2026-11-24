package core

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/reusing-code/kontor/backend/internal/config"
)

func registerStatus(t *testing.T, h *Handler, email string) (int, string) {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/v1/auth/register", jsonBody(map[string]string{"email": email, "password": "password123"}))
	h.Register(rec, req)
	return rec.Code, rec.Body.String()
}

func registrationOpenFlag(t *testing.T, h *Handler) bool {
	t.Helper()
	rec := httptest.NewRecorder()
	h.AuthConfig(rec, httptest.NewRequest("GET", "/api/v1/auth/config", nil))
	var resp authConfigResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); rec.Code != http.StatusOK || err != nil {
		t.Fatalf("auth config = %d, %v", rec.Code, err)
	}
	return resp.RegistrationOpen
}

func TestRegister_Policies(t *testing.T) {
	tests := []struct {
		name   string
		policy config.Registration
		// statuses of three registrations in a row
		want     [3]int
		openThen bool
	}{
		{"first user only", config.RegistrationFirstUser, [3]int{http.StatusCreated, http.StatusForbidden, http.StatusForbidden}, false},
		{"open", config.RegistrationOpen, [3]int{http.StatusCreated, http.StatusCreated, http.StatusCreated}, true},
		{"closed", config.RegistrationClosed, [3]int{http.StatusForbidden, http.StatusForbidden, http.StatusForbidden}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, s := newTestHandler(t)
			h.SetRegistration(tt.policy)
			if got := registrationOpenFlag(t, h); got != (tt.policy != config.RegistrationClosed) {
				t.Errorf("registrationOpen before any user = %v", got)
			}
			for i, want := range tt.want {
				status, body := registerStatus(t, h, fmt.Sprintf("u%d@example.com", i))
				if status != want {
					t.Fatalf("register %d = %d %s, want %d", i, status, body, want)
				}
				if status == http.StatusForbidden && body != `{"code":"registration_closed","error":"registration is closed"}`+"\n" {
					t.Errorf("closed body = %s", body)
				}
			}
			if got := registrationOpenFlag(t, h); got != tt.openThen {
				t.Errorf("registrationOpen afterwards = %v, want %v", got, tt.openThen)
			}
			users, err := s.ListUsers(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			created := 0
			for _, w := range tt.want {
				if w == http.StatusCreated {
					created++
				}
			}
			if len(users) != created {
				t.Errorf("users = %d, want %d", len(users), created)
			}
		})
	}
}

func TestHasUsers(t *testing.T) {
	h, s := newTestHandler(t)
	if has, err := s.HasUsers(t.Context()); has || err != nil {
		t.Fatalf("empty store: %v %v", has, err)
	}
	h.SetRegistration(config.RegistrationOpen)
	if status, body := registerStatus(t, h, "a@example.com"); status != http.StatusCreated {
		t.Fatalf("register = %d %s", status, body)
	}
	if has, err := s.HasUsers(t.Context()); !has || err != nil {
		t.Fatalf("after register: %v %v", has, err)
	}
}
