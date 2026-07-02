package model

import (
	"github.com/reusing-code/kontor/backend/internal/categories"
	"github.com/reusing-code/kontor/backend/internal/core"
	"github.com/reusing-code/kontor/backend/internal/modules/auto"
	"github.com/reusing-code/kontor/backend/internal/modules/contracts"
)

// Transitional aliases while the module split is in progress; the model
// package disappears once all modules own their types.

type User = core.User

type UserSettings = core.UserSettings

type SettingsResponse = core.SettingsResponse

var DefaultUserSettings = core.DefaultUserSettings

type Category = categories.Category

type CategoryInput = categories.CategoryInput

type Contract = contracts.Contract

type ContractInput = contracts.ContractInput

type BillingInterval = contracts.BillingInterval

const (
	BillingMonthly = contracts.BillingMonthly
	BillingYearly  = contracts.BillingYearly
)

type Vehicle = auto.Vehicle

type VehicleInput = auto.VehicleInput

type CostEntry = auto.CostEntry

type CostEntryInput = auto.CostEntryInput
