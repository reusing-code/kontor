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
	// DisabledModules stores the modules the user switched off. Storing the
	// disabled set means the zero value enables everything, including
	// modules added in future versions.
	DisabledModules []string `json:"disabledModules,omitempty"`
}

func DefaultUserSettings() UserSettings {
	return UserSettings{
		RenewalDays:       90,
		ReminderFrequency: "disabled",
	}
}

type SettingsResponse struct {
	RenewalDays       int      `json:"renewalDays"`
	ReminderFrequency string   `json:"reminderFrequency"`
	EnabledModules    []string `json:"enabledModules"`
}
