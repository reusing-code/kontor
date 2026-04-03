package model

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

type LedgerAccount struct {
	ID        uuid.UUID `json:"id"`
	Name      string    `json:"name"`
	Bank      string    `json:"bank"`
	IBAN      string    `json:"iban,omitempty"`
	Currency  string    `json:"currency"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type LedgerAccountInput struct {
	Name     string `json:"name"`
	Bank     string `json:"bank"`
	IBAN     string `json:"iban,omitempty"`
	Currency string `json:"currency"`
}

func (a *LedgerAccountInput) Validate() error {
	if a.Name == "" {
		return errors.New("name is required")
	}
	if a.Bank == "" {
		return errors.New("bank is required")
	}
	if a.Currency == "" {
		a.Currency = "EUR"
	}
	return nil
}
