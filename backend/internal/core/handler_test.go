package core

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/reusing-code/kontor/backend/internal/middleware"
	"github.com/reusing-code/kontor/backend/internal/storage"
	"golang.org/x/crypto/bcrypt"
)

var testJWTSecret = []byte("test-secret-key")

const testUserID = "00000000-0000-0000-0000-000000000001"

func newTestStore(t *testing.T) *Store {
	t.Helper()
	e, err := storage.Open(t.TempDir(), slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatalf("opening engine: %v", err)
	}
	t.Cleanup(func() { e.Close() })
	return NewStore(e)
}

func newTestHandler(t *testing.T, seeds ...SeedFunc) (*Handler, *Store) {
	s := newTestStore(t)
	h := NewHandler(s, slog.New(slog.DiscardHandler), testJWTSecret, nil, seeds)
	return h, s
}

func newAuthMux(h *Handler) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/auth/register", h.Register)
	mux.HandleFunc("POST /api/v1/auth/login", h.Login)
	return mux
}

func newSettingsMux(h *Handler) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/settings", h.GetSettings)
	mux.HandleFunc("PUT /api/v1/settings", h.UpdateSettings)
	mux.HandleFunc("PUT /api/v1/settings/password", h.ChangePassword)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := middleware.SetUserID(r.Context(), testUserID)
		mux.ServeHTTP(w, r.WithContext(ctx))
	})
}

func jsonBody(v any) *bytes.Buffer {
	b, _ := json.Marshal(v)
	return bytes.NewBuffer(b)
}

func decodeJSON[T any](t *testing.T, rec *httptest.ResponseRecorder) T {
	t.Helper()
	var v T
	if err := json.NewDecoder(rec.Body).Decode(&v); err != nil {
		t.Fatalf("decoding response: %v", err)
	}
	return v
}

func seedTestUser(t *testing.T, s *Store, password string) User {
	t.Helper()
	uid, _ := uuid.Parse(testUserID)
	hash, _ := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	user := User{ID: uid, Email: "test@test.com", PasswordHash: string(hash), CreatedAt: time.Now().UTC()}
	if err := s.CreateUser(t.Context(), user); err != nil {
		t.Fatalf("creating user: %v", err)
	}
	return user
}

// Auth handler tests

func TestRegister_RunsSeeds(t *testing.T) {
	seededUsers := []string{}
	seed := func(_ context.Context, userID string) error {
		seededUsers = append(seededUsers, userID)
		return nil
	}
	h, _ := newTestHandler(t, seed, seed)
	mux := newAuthMux(h)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/v1/auth/register", jsonBody(map[string]string{"email": "seed@test.com", "password": "password1"}))
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("register: status = %d, want %d", rec.Code, http.StatusCreated)
	}

	if len(seededUsers) != 2 {
		t.Fatalf("expected 2 seed calls, got %d", len(seededUsers))
	}
}

func TestRegisterThenLogin(t *testing.T) {
	h, _ := newTestHandler(t)
	mux := newAuthMux(h)

	creds := map[string]string{"email": "test@example.com", "password": "secret123"}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/v1/auth/register", jsonBody(creds))
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("register: status = %d, want %d; body: %s", rec.Code, http.StatusCreated, rec.Body.String())
	}
	regResp := decodeJSON[authResponse](t, rec)
	if regResp.Token == "" {
		t.Fatal("register: expected token")
	}
	if regResp.User.Email != "test@example.com" {
		t.Fatalf("register: email = %q, want %q", regResp.User.Email, "test@example.com")
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest("POST", "/api/v1/auth/login", jsonBody(creds))
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("login: status = %d, want %d; body: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	loginResp := decodeJSON[authResponse](t, rec)
	if loginResp.Token == "" {
		t.Fatal("login: expected token")
	}
	if loginResp.User.Email != "test@example.com" {
		t.Fatalf("login: email = %q, want %q", loginResp.User.Email, "test@example.com")
	}
}

func TestLogin_WrongPassword(t *testing.T) {
	h, _ := newTestHandler(t)
	mux := newAuthMux(h)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/v1/auth/register", jsonBody(map[string]string{"email": "a@b.com", "password": "correct-horse"}))
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("register: status = %d, want %d", rec.Code, http.StatusCreated)
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest("POST", "/api/v1/auth/login", jsonBody(map[string]string{"email": "a@b.com", "password": "wrong"}))
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("login wrong pw: status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestRegister_DuplicateEmail(t *testing.T) {
	h, _ := newTestHandler(t)
	mux := newAuthMux(h)

	creds := map[string]string{"email": "dup@test.com", "password": "password1"}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/v1/auth/register", jsonBody(creds))
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("first register: status = %d, want %d", rec.Code, http.StatusCreated)
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest("POST", "/api/v1/auth/register", jsonBody(creds))
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusConflict {
		t.Fatalf("duplicate register: status = %d, want %d", rec.Code, http.StatusConflict)
	}
}

func TestRegister_ShortPassword(t *testing.T) {
	h, s := newTestHandler(t)
	mux := newAuthMux(h)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/v1/auth/register", jsonBody(map[string]string{"email": "short@test.com", "password": "1234567"}))
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
	if _, err := s.GetUserByEmail(t.Context(), "short@test.com"); err == nil {
		t.Error("user should not be created with a short password")
	}
}

// Settings handler tests

func TestGetSettings_ReturnsDefaults(t *testing.T) {
	h, _ := newTestHandler(t)
	mux := newSettingsMux(h)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/v1/settings", nil)
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	s := decodeJSON[SettingsResponse](t, rec)
	if s.RenewalDays != 90 {
		t.Errorf("RenewalDays = %d, want 90", s.RenewalDays)
	}
	if s.ReminderFrequency != "disabled" {
		t.Errorf("ReminderFrequency = %q, want %q", s.ReminderFrequency, "disabled")
	}
}

func TestGetSettings_OmitsLastReminderSent(t *testing.T) {
	h, s := newTestHandler(t)
	mux := newSettingsMux(h)

	if err := s.UpdateSettings(t.Context(), testUserID, UserSettings{
		RenewalDays:       90,
		ReminderFrequency: "weekly",
		LastReminderSent:  time.Now(),
	}); err != nil {
		t.Fatalf("updating settings: %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/v1/settings", nil)
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var raw map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&raw); err != nil {
		t.Fatalf("decoding response: %v", err)
	}
	if _, ok := raw["lastReminderSent"]; ok {
		t.Error("response should not contain lastReminderSent")
	}
}

func TestUpdateSettings_Success(t *testing.T) {
	h, _ := newTestHandler(t)
	mux := newSettingsMux(h)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("PUT", "/api/v1/settings", jsonBody(map[string]any{"renewalDays": 30, "reminderFrequency": "weekly"}))
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	s := decodeJSON[SettingsResponse](t, rec)
	if s.RenewalDays != 30 {
		t.Errorf("RenewalDays = %d, want 30", s.RenewalDays)
	}
	if s.ReminderFrequency != "weekly" {
		t.Errorf("ReminderFrequency = %q, want %q", s.ReminderFrequency, "weekly")
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest("GET", "/api/v1/settings", nil)
	mux.ServeHTTP(rec, req)

	s = decodeJSON[SettingsResponse](t, rec)
	if s.RenewalDays != 30 {
		t.Errorf("persisted RenewalDays = %d, want 30", s.RenewalDays)
	}
}

func TestUpdateSettings_InvalidRange(t *testing.T) {
	h, _ := newTestHandler(t)
	mux := newSettingsMux(h)

	for _, days := range []int{0, -1, 366} {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest("PUT", "/api/v1/settings", jsonBody(map[string]int{"renewalDays": days}))
		mux.ServeHTTP(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Errorf("days=%d: status = %d, want %d", days, rec.Code, http.StatusBadRequest)
		}
	}
}

func TestUpdateSettings_InvalidReminderFrequency(t *testing.T) {
	h, _ := newTestHandler(t)
	mux := newSettingsMux(h)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("PUT", "/api/v1/settings", jsonBody(map[string]any{
		"renewalDays":       30,
		"reminderFrequency": "daily",
	}))
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestUpdateSettings_PreservesLastReminderSent(t *testing.T) {
	h, s := newTestHandler(t)
	mux := newSettingsMux(h)

	sent := time.Date(2025, 6, 15, 10, 0, 0, 0, time.UTC)
	if err := s.UpdateSettings(t.Context(), testUserID, UserSettings{
		RenewalDays:       90,
		ReminderFrequency: "weekly",
		LastReminderSent:  sent,
	}); err != nil {
		t.Fatalf("updating settings: %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("PUT", "/api/v1/settings", jsonBody(map[string]any{
		"renewalDays":       60,
		"reminderFrequency": "monthly",
	}))
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	persisted, err := s.GetSettings(t.Context(), testUserID)
	if err != nil {
		t.Fatalf("getting settings: %v", err)
	}
	if !persisted.LastReminderSent.Equal(sent) {
		t.Errorf("LastReminderSent = %v, want %v", persisted.LastReminderSent, sent)
	}
	if persisted.RenewalDays != 60 {
		t.Errorf("RenewalDays = %d, want 60", persisted.RenewalDays)
	}
	if persisted.ReminderFrequency != "monthly" {
		t.Errorf("ReminderFrequency = %q, want %q", persisted.ReminderFrequency, "monthly")
	}
}

// Password change tests

func TestChangePassword_Success(t *testing.T) {
	h, s := newTestHandler(t)
	mux := newSettingsMux(h)
	seedTestUser(t, s, "oldpass")

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("PUT", "/api/v1/settings/password", jsonBody(map[string]string{
		"currentPassword": "oldpass",
		"newPassword":     "newpass123",
	}))
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d; body: %s", rec.Code, http.StatusNoContent, rec.Body.String())
	}

	updated, err := s.GetUserByID(t.Context(), testUserID)
	if err != nil {
		t.Fatalf("getting user: %v", err)
	}
	if err := bcrypt.CompareHashAndPassword([]byte(updated.PasswordHash), []byte("newpass123")); err != nil {
		t.Error("new password should be valid after change")
	}
}

func TestChangePassword_WrongCurrent(t *testing.T) {
	h, s := newTestHandler(t)
	mux := newSettingsMux(h)
	seedTestUser(t, s, "correct")

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("PUT", "/api/v1/settings/password", jsonBody(map[string]string{
		"currentPassword": "wrong",
		"newPassword":     "newpass123",
	}))
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestChangePassword_MissingFields(t *testing.T) {
	h, _ := newTestHandler(t)
	mux := newSettingsMux(h)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("PUT", "/api/v1/settings/password", jsonBody(map[string]string{
		"currentPassword": "old",
	}))
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestChangePassword_ShortNewPassword(t *testing.T) {
	h, s := newTestHandler(t)
	mux := newSettingsMux(h)
	seedTestUser(t, s, "oldpass123")

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("PUT", "/api/v1/settings/password", jsonBody(map[string]string{
		"currentPassword": "oldpass123",
		"newPassword":     "1234567",
	}))
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}
