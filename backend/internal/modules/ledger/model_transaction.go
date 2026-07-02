package ledger

import (
	"errors"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	LedgerReferencePurchase               = "purchase"
	LedgerReferenceContract               = "contract"
	LedgerReferenceVehicle                = "vehicle"
	LedgerSpecialCategoryInternalTransfer = "internalTransfer"
)

type LedgerTransactionReference struct {
	Type     string    `json:"type"`
	TargetID uuid.UUID `json:"targetId"`
}

type LedgerTransactionDetailsInput struct {
	Note       string                       `json:"note,omitempty"`
	Links      []string                     `json:"links,omitempty"`
	References []LedgerTransactionReference `json:"references,omitempty"`
}

type LedgerTransferCandidate struct {
	Transaction   LedgerTransaction `json:"transaction"`
	AccountName   string            `json:"accountName"`
	DateDeltaDays int               `json:"dateDeltaDays"`
	IBANMatch     bool              `json:"ibanMatch"`
}

type LedgerTransferLinkInput struct {
	PairedTransactionID uuid.UUID `json:"pairedTransactionId"`
}

type LedgerTransferLinkResult struct {
	Transaction       LedgerTransaction `json:"transaction"`
	PairedTransaction LedgerTransaction `json:"pairedTransaction"`
}

func (i *LedgerTransferLinkInput) Validate() error {
	if i.PairedTransactionID == uuid.Nil {
		return errors.New("pairedTransactionId is required")
	}
	return nil
}

type LedgerTransaction struct {
	ID                        uuid.UUID                    `json:"id"`
	AccountID                 uuid.UUID                    `json:"accountId"`
	CategoryID                *uuid.UUID                   `json:"categoryId,omitempty"`
	SpecialCategory           string                       `json:"specialCategory,omitempty"`
	TransferPairTransactionID *uuid.UUID                   `json:"transferPairTransactionId,omitempty"`
	BookingDate               string                       `json:"bookingDate"`
	ValueDate                 string                       `json:"valueDate,omitempty"`
	AmountMinor               int64                        `json:"amountMinor"`
	Currency                  string                       `json:"currency"`
	CounterpartyName          string                       `json:"counterpartyName,omitempty"`
	CounterpartyIBAN          string                       `json:"counterpartyIban,omitempty"`
	Purpose                   string                       `json:"purpose,omitempty"`
	BankReference             string                       `json:"bankReference,omitempty"`
	TransactionType           string                       `json:"transactionType,omitempty"`
	ReviewStatus              string                       `json:"reviewStatus"`
	CategorizationSource      string                       `json:"categorizationSource"`
	Note                      string                       `json:"note,omitempty"`
	Links                     []string                     `json:"links,omitempty"`
	References                []LedgerTransactionReference `json:"references,omitempty"`
	EmailOrderIDs             []uuid.UUID                  `json:"emailOrderIds,omitempty"`
	SourceType                string                       `json:"sourceType"`
	ImportBatchID             uuid.UUID                    `json:"importBatchId"`
	Fingerprint               string                       `json:"fingerprint"`
	CreatedAt                 time.Time                    `json:"createdAt"`
	UpdatedAt                 time.Time                    `json:"updatedAt"`
}

func NormalizeLedgerTransactionLinks(links []string) []string {
	if len(links) == 0 {
		return []string{}
	}
	seen := make(map[string]struct{}, len(links))
	normalized := make([]string, 0, len(links))
	for _, link := range links {
		link = strings.TrimSpace(link)
		if link == "" {
			continue
		}
		if _, ok := seen[link]; ok {
			continue
		}
		seen[link] = struct{}{}
		normalized = append(normalized, link)
	}
	if normalized == nil {
		return []string{}
	}
	return normalized
}

func NormalizeLedgerTransactionReferences(references []LedgerTransactionReference) []LedgerTransactionReference {
	if len(references) == 0 {
		return []LedgerTransactionReference{}
	}
	seen := make(map[string]struct{}, len(references))
	normalized := make([]LedgerTransactionReference, 0, len(references))
	for _, reference := range references {
		reference.Type = strings.TrimSpace(reference.Type)
		if reference.Type == "" || reference.TargetID == uuid.Nil {
			continue
		}
		key := reference.Type + ":" + reference.TargetID.String()
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		normalized = append(normalized, reference)
	}
	if normalized == nil {
		return []LedgerTransactionReference{}
	}
	return normalized
}

func NormalizeLinkedTransactionIDs(ids []uuid.UUID) []uuid.UUID {
	if len(ids) == 0 {
		return []uuid.UUID{}
	}
	seen := make(map[uuid.UUID]struct{}, len(ids))
	normalized := make([]uuid.UUID, 0, len(ids))
	for _, id := range ids {
		if id == uuid.Nil {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		normalized = append(normalized, id)
	}
	if normalized == nil {
		return []uuid.UUID{}
	}
	return normalized
}

func (i *LedgerTransactionDetailsInput) Validate() error {
	i.Note = strings.TrimSpace(i.Note)
	i.Links = NormalizeLedgerTransactionLinks(i.Links)
	i.References = NormalizeLedgerTransactionReferences(i.References)
	for _, link := range i.Links {
		parsed, err := url.ParseRequestURI(link)
		if err != nil || parsed.Scheme == "" || parsed.Host == "" {
			return errors.New("links must contain valid absolute URLs")
		}
	}
	for _, reference := range i.References {
		switch reference.Type {
		case LedgerReferencePurchase, LedgerReferenceContract, LedgerReferenceVehicle:
		default:
			return errors.New("references contain an invalid type")
		}
	}
	return nil
}
