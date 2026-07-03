package core

import (
	"net/http"
	"slices"

	"github.com/reusing-code/kontor/backend/internal/httputil"
	"github.com/reusing-code/kontor/backend/internal/middleware"
	"golang.org/x/crypto/bcrypt"
)

var validReminderFrequencies = map[string]bool{
	"disabled": true,
	"weekly":   true,
	"biweekly": true,
	"monthly":  true,
}

func (h *Handler) GetSettings(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	settings, err := h.store.GetSettings(r.Context(), userID)
	if err != nil {
		httputil.StoreError(h.logger, w, err)
		return
	}
	httputil.WriteJSON(h.logger, w, http.StatusOK, h.settingsResponse(settings))
}

func (h *Handler) settingsResponse(settings UserSettings) SettingsResponse {
	enabled := []string{}
	for _, id := range h.registry.IDs() {
		if !slices.Contains(settings.DisabledModules, id) {
			enabled = append(enabled, id)
		}
	}
	return SettingsResponse{
		RenewalDays:       settings.RenewalDays,
		ReminderFrequency: settings.ReminderFrequency,
		EnabledModules:    enabled,
	}
}

type moduleStatus struct {
	ID      string `json:"id"`
	Enabled bool   `json:"enabled"`
}

// ListModules reports every available module and whether the user has it
// enabled.
func (h *Handler) ListModules(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	settings, err := h.store.GetSettings(r.Context(), userID)
	if err != nil {
		httputil.StoreError(h.logger, w, err)
		return
	}
	statuses := []moduleStatus{}
	for _, id := range h.registry.IDs() {
		statuses = append(statuses, moduleStatus{ID: id, Enabled: !slices.Contains(settings.DisabledModules, id)})
	}
	httputil.WriteJSON(h.logger, w, http.StatusOK, statuses)
}

type updateSettingsRequest struct {
	RenewalDays       int       `json:"renewalDays"`
	ReminderFrequency string    `json:"reminderFrequency"`
	EnabledModules    *[]string `json:"enabledModules,omitempty"`
}

func (h *Handler) UpdateSettings(w http.ResponseWriter, r *http.Request) {
	var req updateSettingsRequest
	if err := httputil.ReadJSON(r, &req); err != nil {
		httputil.Error(h.logger, w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.RenewalDays < 1 || req.RenewalDays > 365 {
		httputil.Error(h.logger, w, http.StatusBadRequest, "renewalDays must be between 1 and 365")
		return
	}
	if req.ReminderFrequency != "" && !validReminderFrequencies[req.ReminderFrequency] {
		httputil.Error(h.logger, w, http.StatusBadRequest, "reminderFrequency must be one of: disabled, weekly, biweekly, monthly")
		return
	}

	userID := middleware.GetUserID(r.Context())
	current, err := h.store.GetSettings(r.Context(), userID)
	if err != nil {
		httputil.StoreError(h.logger, w, err)
		return
	}

	current.RenewalDays = req.RenewalDays
	if req.ReminderFrequency != "" {
		current.ReminderFrequency = req.ReminderFrequency
	}

	var newlyEnabled []string
	if req.EnabledModules != nil {
		for _, id := range *req.EnabledModules {
			if _, ok := h.registry.Get(id); !ok {
				httputil.Error(h.logger, w, http.StatusBadRequest, "unknown module: "+id)
				return
			}
		}
		disabled := []string{}
		for _, id := range h.registry.IDs() {
			if !slices.Contains(*req.EnabledModules, id) {
				disabled = append(disabled, id)
			} else if slices.Contains(current.DisabledModules, id) {
				newlyEnabled = append(newlyEnabled, id)
			}
		}
		current.DisabledModules = disabled
	}

	if err := h.store.UpdateSettings(r.Context(), userID, current); err != nil {
		httputil.StoreError(h.logger, w, err)
		return
	}

	// Re-enabled modules get their defaults seeded again (idempotent).
	for _, id := range newlyEnabled {
		if m, ok := h.registry.Get(id); ok {
			if err := m.Seed(r.Context(), userID); err != nil {
				h.logger.Error("seeding re-enabled module", "module", id, "error", err)
			}
		}
	}

	httputil.WriteJSON(h.logger, w, http.StatusOK, h.settingsResponse(current))
}

type changePasswordRequest struct {
	CurrentPassword string `json:"currentPassword"`
	NewPassword     string `json:"newPassword"`
}

func (h *Handler) ChangePassword(w http.ResponseWriter, r *http.Request) {
	var req changePasswordRequest
	if err := httputil.ReadJSON(r, &req); err != nil {
		httputil.Error(h.logger, w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.CurrentPassword == "" || req.NewPassword == "" {
		httputil.Error(h.logger, w, http.StatusBadRequest, "currentPassword and newPassword are required")
		return
	}
	if len(req.NewPassword) < minPasswordLength {
		httputil.Error(h.logger, w, http.StatusBadRequest, "password must be at least 8 characters")
		return
	}

	userID := middleware.GetUserID(r.Context())
	user, err := h.store.GetUserByID(r.Context(), userID)
	if err != nil {
		httputil.StoreError(h.logger, w, err)
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.CurrentPassword)); err != nil {
		httputil.Error(h.logger, w, http.StatusUnauthorized, "current password is incorrect")
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		h.logger.Error("hashing password", "error", err)
		httputil.Error(h.logger, w, http.StatusInternalServerError, "internal error")
		return
	}

	user.PasswordHash = string(hash)
	if err := h.store.UpdateUser(r.Context(), user); err != nil {
		httputil.StoreError(h.logger, w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
