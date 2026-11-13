package ledger

import (
	"errors"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/google/uuid"
)

const (
	LedgerEmailOrderStatusUnmatched = "unmatched"
	LedgerEmailOrderStatusMatched   = "matched"
	LedgerEmailOrderStatusRejected  = "rejected"

	// Legacy importer IDs of orders stored before the LLM extraction.
	LedgerEmailImporterAmazonDE = "amazon.de"
	LedgerEmailImporterPayPalDE = "paypal.de"
	LedgerEmailImporterLLM      = "llm"

	PaymentMethodPayPal       = "paypal"
	PaymentMethodCard         = "card"
	PaymentMethodDirectDebit  = "direct_debit"
	PaymentMethodBankTransfer = "bank_transfer"
	PaymentMethodOther        = "other"
	PaymentMethodUnknown      = "unknown"
)

const (
	EmailSeenNotOrder   = "not_order"
	EmailSeenPreClass   = "preclass"
	EmailSeenTooLarge   = "too_large"
	EmailSeenUnreadable = "unreadable"
)

// ledgerEmailSeen marks a message that was processed without producing an
// order, so later scans skip it.
type ledgerEmailSeen struct {
	Reason string    `json:"reason"`
	At     time.Time `json:"at"`
}

var paymentMethods = []string{
	PaymentMethodPayPal,
	PaymentMethodCard,
	PaymentMethodDirectDebit,
	PaymentMethodBankTransfer,
	PaymentMethodOther,
	PaymentMethodUnknown,
}

type LedgerEmailAccount struct {
	ID                    uuid.UUID  `json:"id"`
	Name                  string     `json:"name"`
	IMAPHost              string     `json:"imapHost"`
	IMAPPort              int        `json:"imapPort"`
	Username              string     `json:"username"`
	EncryptedPassword     string     `json:"-"`
	UseTLS                bool       `json:"useTls"`
	Folder                string     `json:"folder"`
	PreClassify           bool       `json:"preClassify"`
	ScanSince             string     `json:"scanSince"`
	LastScanAt            *time.Time `json:"lastScanAt,omitempty"`
	LastScanAttemptAt     *time.Time `json:"lastScanAttemptAt,omitempty"`
	LastScanStatusMessage string     `json:"lastScanStatusMessage,omitempty"`
	// ScanResumePending marks a partial scan that the scheduler continues
	// after emailScanResumeDelay instead of the full interval.
	ScanResumePending bool      `json:"scanResumePending,omitempty"`
	CreatedAt         time.Time `json:"createdAt"`
	UpdatedAt         time.Time `json:"updatedAt"`
}

// storableLedgerEmailAccount includes EncryptedPassword for persistence
// (LedgerEmailAccount has json:"-" on it so it never leaves the API).
type storableLedgerEmailAccount struct {
	ID                    uuid.UUID  `json:"id"`
	Name                  string     `json:"name"`
	IMAPHost              string     `json:"imapHost"`
	IMAPPort              int        `json:"imapPort"`
	Username              string     `json:"username"`
	EncryptedPassword     string     `json:"encryptedPassword"`
	UseTLS                bool       `json:"useTls"`
	Folder                string     `json:"folder"`
	PreClassify           bool       `json:"preClassify"`
	ScanSince             string     `json:"scanSince"`
	LastScanAt            *time.Time `json:"lastScanAt,omitempty"`
	LastScanAttemptAt     *time.Time `json:"lastScanAttemptAt,omitempty"`
	LastScanStatusMessage string     `json:"lastScanStatusMessage,omitempty"`
	ScanResumePending     bool       `json:"scanResumePending,omitempty"`
	CreatedAt             time.Time  `json:"createdAt"`
	UpdatedAt             time.Time  `json:"updatedAt"`
}

// LedgerEmailScanState is the outcome of a scan as recorded on the account.
// CompletedAt moves the lookback anchor (LastScanAt) and must only be set for
// complete IMAP scans; AttemptedAt schedules the next background scan, after
// emailScanResumeDelay when Resume is set and after the interval otherwise.
type LedgerEmailScanState struct {
	Status      string
	AttemptedAt *time.Time
	CompletedAt *time.Time
	Resume      bool
}

type LedgerEmailAccountInput struct {
	Name        string `json:"name"`
	IMAPHost    string `json:"imapHost"`
	IMAPPort    int    `json:"imapPort"`
	Username    string `json:"username"`
	Password    string `json:"password,omitempty"`
	UseTLS      bool   `json:"useTls"`
	Folder      string `json:"folder"`
	PreClassify bool   `json:"preClassify"`
	ScanSince   string `json:"scanSince"`
}

type LedgerEmailAccountUpdateInput struct {
	Name        string  `json:"name"`
	IMAPHost    string  `json:"imapHost"`
	IMAPPort    int     `json:"imapPort"`
	Username    string  `json:"username"`
	Password    *string `json:"password,omitempty"`
	UseTLS      bool    `json:"useTls"`
	Folder      string  `json:"folder"`
	PreClassify bool    `json:"preClassify"`
	ScanSince   string  `json:"scanSince"`
}

type LedgerEmailOrderItem struct {
	Name            string `json:"name"`
	Quantity        int    `json:"quantity"`
	UnitPriceMinor  int64  `json:"unitPriceMinor"`
	TotalPriceMinor int64  `json:"totalPriceMinor"`
}

type LedgerEmailOrderCandidate struct {
	Transaction   LedgerTransaction `json:"transaction"`
	AccountName   string            `json:"accountName"`
	DateDeltaDays *int              `json:"dateDeltaDays,omitempty"`
}

type LedgerEmailOrderCandidatesResult struct {
	Items []LedgerEmailOrderCandidate `json:"items"`
	ChargeWindow
}

type LedgerEmailOrder struct {
	ID                   uuid.UUID              `json:"id"`
	EmailAccountID       uuid.UUID              `json:"emailAccountId"`
	ImporterID           string                 `json:"importerId"`
	ExternalOrderID      string                 `json:"externalOrderId,omitempty"`
	OrderDate            string                 `json:"orderDate"`
	TotalMinor           int64                  `json:"totalMinor"`
	Currency             string                 `json:"currency"`
	Merchant             string                 `json:"merchant,omitempty"`
	PaymentMethod        string                 `json:"paymentMethod,omitempty"`
	Items                []LedgerEmailOrderItem `json:"items,omitempty"`
	EmailMessageID       string                 `json:"emailMessageId,omitempty"`
	EmailSubject         string                 `json:"emailSubject,omitempty"`
	MatchStatus          string                 `json:"matchStatus"`
	LinkedTransactionIDs []uuid.UUID            `json:"linkedTransactionIds,omitempty"`
	CreatedAt            time.Time              `json:"createdAt"`
	UpdatedAt            time.Time              `json:"updatedAt"`
}

type LedgerEmailOrderLinkInput struct {
	TransactionIDs []uuid.UUID `json:"transactionIds"`
}

type LedgerEmailScanResult struct {
	EmailsScanned int                `json:"emailsScanned"`
	OrdersFound   int                `json:"ordersFound"`
	OrdersNew     int                `json:"ordersNew"`
	OrdersLinked  int                `json:"ordersLinked"`
	Warnings      []string           `json:"warnings,omitempty"`
	Orders        []LedgerEmailOrder `json:"orders,omitempty"`
	Partial       bool               `json:"partial,omitempty"`
	// Skipped counts messages that were left unprocessed for a reason that is
	// worth retrying (transient LLM or IMAP failure).
	Skipped int `json:"skipped,omitempty"`
}

// Complete reports whether every message of the scan was dealt with, so the
// next scan may start after it.
func (r LedgerEmailScanResult) Complete() bool {
	return !r.Partial && r.Skipped == 0
}

const maxEmailFolderRunes = 255

func normalizeEmailFolder(folder string) (string, error) {
	folder = strings.TrimSpace(folder)
	if folder == "" {
		return "", errors.New("folder is required")
	}
	if !utf8.ValidString(folder) || utf8.RuneCountInString(folder) > maxEmailFolderRunes || strings.IndexFunc(folder, unicode.IsControl) >= 0 {
		return "", errors.New("folder is invalid")
	}
	return folder, nil
}

func (a *LedgerEmailAccountInput) Validate() error {
	a.Name = strings.TrimSpace(a.Name)
	a.IMAPHost = strings.TrimSpace(a.IMAPHost)
	a.Username = strings.TrimSpace(a.Username)
	a.Password = strings.TrimSpace(a.Password)
	a.ScanSince = strings.TrimSpace(a.ScanSince)
	folder, err := normalizeEmailFolder(a.Folder)
	if err != nil {
		return err
	}
	a.Folder = folder
	if a.Name == "" {
		return errors.New("name is required")
	}
	if a.IMAPHost == "" {
		return errors.New("imapHost is required")
	}
	if a.IMAPPort <= 0 {
		a.IMAPPort = 993
	}
	if a.Username == "" {
		return errors.New("username is required")
	}
	if a.Password == "" {
		return errors.New("password is required")
	}
	if a.ScanSince == "" {
		return errors.New("scanSince is required")
	}
	if _, err := time.Parse("2006-01-02", a.ScanSince); err != nil {
		return errors.New("scanSince must be in YYYY-MM-DD format")
	}
	return nil
}

func (a *LedgerEmailAccountUpdateInput) Validate() error {
	a.Name = strings.TrimSpace(a.Name)
	a.IMAPHost = strings.TrimSpace(a.IMAPHost)
	a.Username = strings.TrimSpace(a.Username)
	a.ScanSince = strings.TrimSpace(a.ScanSince)
	folder, err := normalizeEmailFolder(a.Folder)
	if err != nil {
		return err
	}
	a.Folder = folder
	if a.Password != nil {
		trimmed := strings.TrimSpace(*a.Password)
		a.Password = &trimmed
	}
	if a.Name == "" {
		return errors.New("name is required")
	}
	if a.IMAPHost == "" {
		return errors.New("imapHost is required")
	}
	if a.IMAPPort <= 0 {
		a.IMAPPort = 993
	}
	if a.Username == "" {
		return errors.New("username is required")
	}
	if a.ScanSince == "" {
		return errors.New("scanSince is required")
	}
	if _, err := time.Parse("2006-01-02", a.ScanSince); err != nil {
		return errors.New("scanSince must be in YYYY-MM-DD format")
	}
	return nil
}

func (i *LedgerEmailOrderLinkInput) Validate() error {
	i.TransactionIDs = NormalizeLinkedTransactionIDs(i.TransactionIDs)
	if len(i.TransactionIDs) == 0 {
		return errors.New("transactionIds must contain at least one id")
	}
	return nil
}

func NormalizeLedgerEmailOrderItems(items []LedgerEmailOrderItem) []LedgerEmailOrderItem {
	if len(items) == 0 {
		return []LedgerEmailOrderItem{}
	}
	normalized := make([]LedgerEmailOrderItem, 0, len(items))
	for _, item := range items {
		item.Name = strings.TrimSpace(item.Name)
		if item.Name == "" {
			continue
		}
		if item.Quantity <= 0 {
			item.Quantity = 1
		}
		normalized = append(normalized, item)
	}
	if normalized == nil {
		return []LedgerEmailOrderItem{}
	}
	return normalized
}

func NormalizeLedgerEmailOrder(order LedgerEmailOrder) LedgerEmailOrder {
	order.Items = NormalizeLedgerEmailOrderItems(order.Items)
	order.LinkedTransactionIDs = NormalizeLinkedTransactionIDs(order.LinkedTransactionIDs)
	if order.Currency == "" {
		order.Currency = "EUR"
	}
	if order.MatchStatus == "" {
		if len(order.LinkedTransactionIDs) > 0 {
			order.MatchStatus = LedgerEmailOrderStatusMatched
		} else {
			order.MatchStatus = LedgerEmailOrderStatusUnmatched
		}
	}
	return order
}
