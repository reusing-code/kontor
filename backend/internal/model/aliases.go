package model

import (
	"github.com/reusing-code/kontor/backend/internal/categories"
	"github.com/reusing-code/kontor/backend/internal/core"
)

// Transitional aliases while the module split is in progress; the model
// package disappears once all modules own their types.

type User = core.User

type UserSettings = core.UserSettings

type SettingsResponse = core.SettingsResponse

var DefaultUserSettings = core.DefaultUserSettings

type Category = categories.Category

type CategoryInput = categories.CategoryInput
