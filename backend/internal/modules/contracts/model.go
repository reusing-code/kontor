package contracts

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

type BillingInterval string

const (
	BillingMonthly   BillingInterval = "monthly"
	BillingQuarterly BillingInterval = "quarterly"
	BillingYearly    BillingInterval = "yearly"
)

type Contract struct {
	ID                      uuid.UUID       `json:"id"`
	CategoryID              uuid.UUID       `json:"categoryId"`
	LinkedTransactionIDs    []uuid.UUID     `json:"linkedTransactionIds,omitempty"`
	Name                    string          `json:"name"`
	ProductName             string          `json:"productName,omitempty"`
	Company                 string          `json:"company,omitempty"`
	ContractNumber          string          `json:"contractNumber,omitempty"`
	CustomerNumber          string          `json:"customerNumber,omitempty"`
	Price                   *float64        `json:"price,omitempty"`
	BillingInterval         BillingInterval `json:"billingInterval"`
	StartDate               string          `json:"startDate"`
	EndDate                 string          `json:"endDate,omitempty"`
	MinimumDurationMonths   int             `json:"minimumDurationMonths"`
	ExtensionDurationMonths int             `json:"extensionDurationMonths"`
	NoticePeriodMonths      int             `json:"noticePeriodMonths"`
	CustomerPortalURL       string          `json:"customerPortalUrl,omitempty"`
	PaperlessURL            string          `json:"paperlessUrl,omitempty"`
	Comments                string          `json:"comments,omitempty"`
	CreatedAt               time.Time       `json:"createdAt"`
	UpdatedAt               time.Time       `json:"updatedAt"`
}

// MonthlyPrice returns the price normalized to a monthly amount.
func (c *Contract) MonthlyPrice() float64 {
	if c.Price == nil {
		return 0
	}
	switch c.BillingInterval {
	case BillingQuarterly:
		return *c.Price / 3
	case BillingYearly:
		return *c.Price / 12
	default:
		return *c.Price
	}
}

// YearlyPrice returns the price normalized to a yearly amount.
func (c *Contract) YearlyPrice() float64 {
	if c.Price == nil {
		return 0
	}
	switch c.BillingInterval {
	case BillingQuarterly:
		return *c.Price * 4
	case BillingYearly:
		return *c.Price
	default:
		return *c.Price * 12
	}
}

type ContractInput struct {
	Name                    string          `json:"name"`
	ProductName             string          `json:"productName,omitempty"`
	Company                 string          `json:"company,omitempty"`
	ContractNumber          string          `json:"contractNumber,omitempty"`
	CustomerNumber          string          `json:"customerNumber,omitempty"`
	Price                   *float64        `json:"price,omitempty"`
	BillingInterval         BillingInterval `json:"billingInterval"`
	StartDate               string          `json:"startDate"`
	EndDate                 string          `json:"endDate,omitempty"`
	MinimumDurationMonths   int             `json:"minimumDurationMonths"`
	ExtensionDurationMonths int             `json:"extensionDurationMonths"`
	NoticePeriodMonths      int             `json:"noticePeriodMonths"`
	CustomerPortalURL       string          `json:"customerPortalUrl,omitempty"`
	PaperlessURL            string          `json:"paperlessUrl,omitempty"`
	Comments                string          `json:"comments,omitempty"`
}

func (c *ContractInput) Validate() error {
	if c.Name == "" {
		return errors.New("name is required")
	}
	if c.StartDate == "" {
		return errors.New("startDate is required")
	}
	switch c.BillingInterval {
	case "", BillingMonthly, BillingQuarterly, BillingYearly:
	default:
		return errors.New("billingInterval must be 'monthly', 'quarterly' or 'yearly'")
	}
	return nil
}
