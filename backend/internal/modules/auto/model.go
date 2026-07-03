package auto

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

type Vehicle struct {
	ID                   uuid.UUID   `json:"id"`
	LinkedTransactionIDs []uuid.UUID `json:"linkedTransactionIds,omitempty"`
	Name                 string      `json:"name"`
	Make                 string      `json:"make,omitempty"`
	Model                string      `json:"model,omitempty"`
	Year                 *int        `json:"year,omitempty"`
	LicensePlate         string      `json:"licensePlate,omitempty"`
	PurchaseDate         string      `json:"purchaseDate,omitempty"`
	PurchasePrice        *float64    `json:"purchasePrice,omitempty"`
	PurchaseMileage      *float64    `json:"purchaseMileage,omitempty"`
	TargetMileage        *float64    `json:"targetMileage,omitempty"`
	TargetMonths         *int        `json:"targetMonths,omitempty"`
	AnnualInsurance      *float64    `json:"annualInsurance,omitempty"`
	AnnualTax            *float64    `json:"annualTax,omitempty"`
	MaintenanceFactor    *float64    `json:"maintenanceFactor,omitempty"`
	Comments             string      `json:"comments,omitempty"`
	CreatedAt            time.Time   `json:"createdAt"`
	UpdatedAt            time.Time   `json:"updatedAt"`
}

type VehicleInput struct {
	Name              string   `json:"name"`
	Make              string   `json:"make,omitempty"`
	Model             string   `json:"model,omitempty"`
	Year              *int     `json:"year,omitempty"`
	LicensePlate      string   `json:"licensePlate,omitempty"`
	PurchaseDate      string   `json:"purchaseDate,omitempty"`
	PurchasePrice     *float64 `json:"purchasePrice,omitempty"`
	PurchaseMileage   *float64 `json:"purchaseMileage,omitempty"`
	TargetMileage     *float64 `json:"targetMileage,omitempty"`
	TargetMonths      *int     `json:"targetMonths,omitempty"`
	AnnualInsurance   *float64 `json:"annualInsurance,omitempty"`
	AnnualTax         *float64 `json:"annualTax,omitempty"`
	MaintenanceFactor *float64 `json:"maintenanceFactor,omitempty"`
	Comments          string   `json:"comments,omitempty"`
}

func (v *VehicleInput) Validate() error {
	if v.Name == "" {
		return errors.New("name is required")
	}

	if v.PurchaseDate != "" {
		if _, err := time.Parse("2006-01-02", v.PurchaseDate); err != nil {
			return errors.New("purchaseDate must be in format YYYY-MM-DD")
		}
	}
	return nil
}

// CostEntryType enumerates the predefined cost types.
const (
	CostTypeService    = "service"
	CostTypeFuel       = "fuel"
	CostTypeInsurance  = "insurance"
	CostTypeTax        = "tax"
	CostTypeInspection = "inspection"
	CostTypeTires      = "tires"
	CostTypeMileage    = "mileage"
	CostTypeMisc       = "misc"
)

var validCostTypes = map[string]bool{
	CostTypeService:    true,
	CostTypeFuel:       true,
	CostTypeInsurance:  true,
	CostTypeTax:        true,
	CostTypeInspection: true,
	CostTypeTires:      true,
	CostTypeMileage:    true,
	CostTypeMisc:       true,
}

type CostEntry struct {
	ID          uuid.UUID `json:"id"`
	VehicleID   uuid.UUID `json:"vehicleId"`
	Type        string    `json:"type"`
	Description string    `json:"description,omitempty"`
	Vendor      string    `json:"vendor,omitempty"`
	Amount      *float64  `json:"amount,omitempty"`
	Date        string    `json:"date"`
	Mileage     *float64  `json:"mileage,omitempty"`
	Comments    string    `json:"comments,omitempty"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

type CostEntryInput struct {
	Type        string   `json:"type"`
	Description string   `json:"description,omitempty"`
	Vendor      string   `json:"vendor,omitempty"`
	Amount      *float64 `json:"amount,omitempty"`
	Date        string   `json:"date"`
	Mileage     *float64 `json:"mileage,omitempty"`
	Comments    string   `json:"comments,omitempty"`
}

func (c *CostEntryInput) Validate() error {
	if c.Type == "" {
		return errors.New("type is required")
	}
	if !validCostTypes[c.Type] {
		return errors.New("invalid cost type")
	}
	if c.Date == "" {
		return errors.New("date is required")
	}
	if c.Type == CostTypeMileage && c.Mileage == nil {
		return errors.New("mileage is required for mileage entries")
	}
	return nil
}
