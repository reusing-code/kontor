package model

import (
	"time"

	"github.com/google/uuid"
)

type LedgerTransaction struct {
	ID                   uuid.UUID  `json:"id"`
	AccountID            uuid.UUID  `json:"accountId"`
	CategoryID           *uuid.UUID `json:"categoryId,omitempty"`
	BookingDate          string     `json:"bookingDate"`
	ValueDate            string     `json:"valueDate,omitempty"`
	AmountMinor          int64      `json:"amountMinor"`
	Currency             string     `json:"currency"`
	CounterpartyName     string     `json:"counterpartyName,omitempty"`
	CounterpartyIBAN     string     `json:"counterpartyIban,omitempty"`
	Purpose              string     `json:"purpose,omitempty"`
	BankReference        string     `json:"bankReference,omitempty"`
	TransactionType      string     `json:"transactionType,omitempty"`
	ReviewStatus         string     `json:"reviewStatus"`
	CategorizationSource string     `json:"categorizationSource"`
	SourceType           string     `json:"sourceType"`
	ImportBatchID        uuid.UUID  `json:"importBatchId"`
	Fingerprint          string     `json:"fingerprint"`
	CreatedAt            time.Time  `json:"createdAt"`
	UpdatedAt            time.Time  `json:"updatedAt"`
}
