package core

import (
	"time"

	"github.com/google/uuid"
)

type User struct {
	ID           uuid.UUID `json:"id"`
	Email        string    `json:"email"`
	PasswordHash string    `json:"-"`
	CreatedAt    time.Time `json:"createdAt"`
}

type UserSettings struct {
	RenewalDays       int       `json:"renewalDays"`
	ReminderFrequency string    `json:"reminderFrequency"`
	LastReminderSent  time.Time `json:"lastReminderSent,omitempty"`
}

func DefaultUserSettings() UserSettings {
	return UserSettings{
		RenewalDays:       90,
		ReminderFrequency: "disabled",
	}
}

type SettingsResponse struct {
	RenewalDays       int    `json:"renewalDays"`
	ReminderFrequency string `json:"reminderFrequency"`
}
