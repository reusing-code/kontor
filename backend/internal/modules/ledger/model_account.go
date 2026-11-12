package ledger

import (
	"errors"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

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

const maxAccountFieldLength = 200

var (
	currencyPattern = regexp.MustCompile(`^[A-Z]{3}$`)
	ibanPattern     = regexp.MustCompile(`^[A-Z]{2}[0-9]{2}[A-Z0-9]{11,30}$`)
)

// ValidateForCreate is the stricter validation for standalone account
// creation (REST and MCP): it trims and normalizes the fields and checks the
// currency code and the IBAN including its mod-97 checksum. The import path
// keeps using Validate.
func (a *LedgerAccountInput) ValidateForCreate() error {
	a.Name = strings.TrimSpace(a.Name)
	a.Bank = strings.TrimSpace(a.Bank)
	a.Currency = strings.ToUpper(strings.TrimSpace(a.Currency))
	a.IBAN = strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(a.IBAN), " ", ""))
	if err := a.Validate(); err != nil {
		return err
	}
	if utf8.RuneCountInString(a.Name) > maxAccountFieldLength || utf8.RuneCountInString(a.Bank) > maxAccountFieldLength {
		return errors.New("name and bank must be at most 200 characters")
	}
	if !currencyPattern.MatchString(a.Currency) {
		return errors.New("currency must be a three-letter code")
	}
	if a.IBAN != "" && !validIBAN(a.IBAN) {
		return errors.New("iban is invalid")
	}
	return nil
}

func validIBAN(iban string) bool {
	if !ibanPattern.MatchString(iban) {
		return false
	}
	rearranged := iban[4:] + iban[:4]
	rem := 0
	for _, c := range rearranged {
		if c >= 'A' && c <= 'Z' {
			rem = (rem*100 + int(c-'A') + 10) % 97
		} else {
			rem = (rem*10 + int(c-'0')) % 97
		}
	}
	return rem == 1
}
