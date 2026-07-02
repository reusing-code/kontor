package core

import (
	"net/http"

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
	httputil.WriteJSON(h.logger, w, http.StatusOK, SettingsResponse{
		RenewalDays:       settings.RenewalDays,
		ReminderFrequency: settings.ReminderFrequency,
	})
}

type updateSettingsRequest struct {
	RenewalDays       int    `json:"renewalDays"`
	ReminderFrequency string `json:"reminderFrequency"`
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

	if err := h.store.UpdateSettings(r.Context(), userID, current); err != nil {
		httputil.StoreError(h.logger, w, err)
		return
	}
	httputil.WriteJSON(h.logger, w, http.StatusOK, SettingsResponse{
		RenewalDays:       current.RenewalDays,
		ReminderFrequency: current.ReminderFrequency,
	})
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
