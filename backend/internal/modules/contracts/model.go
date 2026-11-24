package contracts

import (
	"errors"
	"github.com/reusing-code/kontor/backend/internal/validate"
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

// Apply sets the fields a create or update request controls; input must be
// validated.
func (c *Contract) Apply(input ContractInput, now time.Time) {
	c.Name = input.Name
	c.ProductName = input.ProductName
	c.Company = input.Company
	c.ContractNumber = input.ContractNumber
	c.CustomerNumber = input.CustomerNumber
	c.Price = input.Price
	c.BillingInterval = input.BillingInterval
	if c.BillingInterval == "" {
		c.BillingInterval = BillingMonthly
	}
	c.StartDate = input.StartDate
	c.EndDate = input.EndDate
	c.MinimumDurationMonths = input.MinimumDurationMonths
	c.ExtensionDurationMonths = input.ExtensionDurationMonths
	c.NoticePeriodMonths = input.NoticePeriodMonths
	c.CustomerPortalURL = input.CustomerPortalURL
	c.PaperlessURL = input.PaperlessURL
	c.Comments = input.Comments
	c.UpdatedAt = now
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

// Validate trims the name and checks the input.
func (c *ContractInput) Validate() error {
	return validate.First(
		validate.Name("name", &c.Name),
		validate.Date("startDate", c.StartDate),
		c.validateBillingInterval(),
		validate.OptionalDate("endDate", c.EndDate),
		validate.NonNegative("price", c.Price),
		validate.Months("minimumDurationMonths", c.MinimumDurationMonths),
		validate.Months("extensionDurationMonths", c.ExtensionDurationMonths),
		validate.Months("noticePeriodMonths", c.NoticePeriodMonths),
		validate.Text("productName", c.ProductName, validate.MaxName),
		validate.Text("company", c.Company, validate.MaxName),
		validate.Text("contractNumber", c.ContractNumber, validate.MaxName),
		validate.Text("customerNumber", c.CustomerNumber, validate.MaxName),
		validate.Text("customerPortalUrl", c.CustomerPortalURL, validate.MaxURL),
		validate.Text("paperlessUrl", c.PaperlessURL, validate.MaxURL),
		validate.Text("comments", c.Comments, validate.MaxText),
	)
}

func (c *ContractInput) validateBillingInterval() error {
	switch c.BillingInterval {
	case "", BillingMonthly, BillingQuarterly, BillingYearly:
		return nil
	}
	return errors.New("billingInterval must be 'monthly', 'quarterly' or 'yearly'")
}
