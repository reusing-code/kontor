package purchases

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

type Purchase struct {
	ID                   uuid.UUID   `json:"id"`
	CategoryID           uuid.UUID   `json:"categoryId"`
	LinkedTransactionIDs []uuid.UUID `json:"linkedTransactionIds,omitempty"`
	Type                 string      `json:"type,omitempty"`
	ItemName             string      `json:"itemName"`
	Brand                string      `json:"brand,omitempty"`
	ArticleNumber        string      `json:"articleNumber,omitempty"`
	Dealer               string      `json:"dealer,omitempty"`
	Price                *float64    `json:"price,omitempty"`
	PurchaseDate         string      `json:"purchaseDate,omitempty"`
	DescriptionURL       string      `json:"descriptionUrl,omitempty"`
	InvoiceURL           string      `json:"invoiceUrl,omitempty"`
	HandbookURL          string      `json:"handbookUrl,omitempty"`
	Consumables          string      `json:"consumables,omitempty"`
	Comments             string      `json:"comments,omitempty"`
	CreatedAt            time.Time   `json:"createdAt"`
	UpdatedAt            time.Time   `json:"updatedAt"`
}

// Apply sets the fields a create or update request controls; input must be
// validated.
func (p *Purchase) Apply(input PurchaseInput, now time.Time) {
	p.Type = input.Type
	p.ItemName = input.ItemName
	p.Brand = input.Brand
	p.ArticleNumber = input.ArticleNumber
	p.Dealer = input.Dealer
	p.Price = input.Price
	p.PurchaseDate = input.PurchaseDate
	p.DescriptionURL = input.DescriptionURL
	p.InvoiceURL = input.InvoiceURL
	p.HandbookURL = input.HandbookURL
	p.Consumables = input.Consumables
	p.Comments = input.Comments
	p.UpdatedAt = now
}

type PurchaseInput struct {
	Type           string   `json:"type,omitempty"`
	ItemName       string   `json:"itemName"`
	Brand          string   `json:"brand,omitempty"`
	ArticleNumber  string   `json:"articleNumber,omitempty"`
	Dealer         string   `json:"dealer,omitempty"`
	Price          *float64 `json:"price,omitempty"`
	PurchaseDate   string   `json:"purchaseDate,omitempty"`
	DescriptionURL string   `json:"descriptionUrl,omitempty"`
	InvoiceURL     string   `json:"invoiceUrl,omitempty"`
	HandbookURL    string   `json:"handbookUrl,omitempty"`
	Consumables    string   `json:"consumables,omitempty"`
	Comments       string   `json:"comments,omitempty"`
}

func (p *PurchaseInput) Validate() error {
	if p.ItemName == "" {
		return errors.New("itemName is required")
	}
	return nil
}
