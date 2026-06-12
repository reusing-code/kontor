package handler

import (
	"net/http"

	"github.com/tobi/contracts/backend/internal/middleware"
	"github.com/tobi/contracts/backend/internal/model"
	"golang.org/x/crypto/bcrypt"
)

var validReminderFrequencies = map[string]bool{
	"disabled":  true,
	"weekly":    true,
	"biweekly":  true,
	"monthly":   true,
}

func (h *Handler) GetSettings(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	settings, err := h.store.GetSettings(r.Context(), userID)
	if err != nil {
		h.handleStoreError(w, err)
		return
	}
	h.writeJSON(w, http.StatusOK, model.SettingsResponse{
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
	if err := h.readJSON(r, &req); err != nil {
		h.errorResponse(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.RenewalDays < 1 || req.RenewalDays > 365 {
		h.errorResponse(w, http.StatusBadRequest, "renewalDays must be between 1 and 365")
		return
	}
	if req.ReminderFrequency != "" && !validReminderFrequencies[req.ReminderFrequency] {
		h.errorResponse(w, http.StatusBadRequest, "reminderFrequency must be one of: disabled, weekly, biweekly, monthly")
		return
	}

	userID := middleware.GetUserID(r.Context())
	current, err := h.store.GetSettings(r.Context(), userID)
	if err != nil {
		h.handleStoreError(w, err)
		return
	}

	current.RenewalDays = req.RenewalDays
	if req.ReminderFrequency != "" {
		current.ReminderFrequency = req.ReminderFrequency
	}

	if err := h.store.UpdateSettings(r.Context(), userID, current); err != nil {
		h.handleStoreError(w, err)
		return
	}
	h.writeJSON(w, http.StatusOK, model.SettingsResponse{
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
	if err := h.readJSON(r, &req); err != nil {
		h.errorResponse(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.CurrentPassword == "" || req.NewPassword == "" {
		h.errorResponse(w, http.StatusBadRequest, "currentPassword and newPassword are required")
		return
	}
	if len(req.NewPassword) < minPasswordLength {
		h.errorResponse(w, http.StatusBadRequest, "password must be at least 8 characters")
		return
	}

	userID := middleware.GetUserID(r.Context())
	user, err := h.store.GetUserByID(r.Context(), userID)
	if err != nil {
		h.handleStoreError(w, err)
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.CurrentPassword)); err != nil {
		h.errorResponse(w, http.StatusUnauthorized, "current password is incorrect")
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		h.logger.Error("hashing password", "error", err)
		h.errorResponse(w, http.StatusInternalServerError, "internal error")
		return
	}

	user.PasswordHash = string(hash)
	if err := h.store.UpdateUser(r.Context(), user); err != nil {
		h.handleStoreError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
