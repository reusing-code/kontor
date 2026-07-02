package contracts

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

type BillingInterval string

const (
	BillingMonthly BillingInterval = "monthly"
	BillingYearly  BillingInterval = "yearly"
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
	if c.BillingInterval == BillingYearly {
		return *c.Price / 12
	}
	return *c.Price
}

// YearlyPrice returns the price normalized to a yearly amount.
func (c *Contract) YearlyPrice() float64 {
	if c.Price == nil {
		return 0
	}
	if c.BillingInterval == BillingYearly {
		return *c.Price
	}
	return *c.Price * 12
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
	if c.BillingInterval != "" && c.BillingInterval != BillingMonthly && c.BillingInterval != BillingYearly {
		return errors.New("billingInterval must be 'monthly' or 'yearly'")
	}
	return nil
}
