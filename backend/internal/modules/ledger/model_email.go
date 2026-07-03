package ledger

import (
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	LedgerEmailOrderStatusUnmatched = "unmatched"
	LedgerEmailOrderStatusMatched   = "matched"
	LedgerEmailOrderStatusRejected  = "rejected"

	LedgerEmailImporterAmazonDE = "amazon.de"
	LedgerEmailImporterPayPalDE = "paypal.de"
)

type LedgerEmailAccount struct {
	ID                    uuid.UUID  `json:"id"`
	Name                  string     `json:"name"`
	IMAPHost              string     `json:"imapHost"`
	IMAPPort              int        `json:"imapPort"`
	Username              string     `json:"username"`
	EncryptedPassword     string     `json:"-"`
	UseTLS                bool       `json:"useTls"`
	ScanSince             string     `json:"scanSince"`
	LastScanAt            *time.Time `json:"lastScanAt,omitempty"`
	LastScanStatusMessage string     `json:"lastScanStatusMessage,omitempty"`
	CreatedAt             time.Time  `json:"createdAt"`
	UpdatedAt             time.Time  `json:"updatedAt"`
}

type LedgerEmailAccountInput struct {
	Name      string `json:"name"`
	IMAPHost  string `json:"imapHost"`
	IMAPPort  int    `json:"imapPort"`
	Username  string `json:"username"`
	Password  string `json:"password,omitempty"`
	UseTLS    bool   `json:"useTls"`
	ScanSince string `json:"scanSince"`
}

type LedgerEmailAccountUpdateInput struct {
	Name      string  `json:"name"`
	IMAPHost  string  `json:"imapHost"`
	IMAPPort  int     `json:"imapPort"`
	Username  string  `json:"username"`
	Password  *string `json:"password,omitempty"`
	UseTLS    bool    `json:"useTls"`
	ScanSince string  `json:"scanSince"`
}

type LedgerEmailOrderItem struct {
	Name            string `json:"name"`
	Quantity        int    `json:"quantity"`
	UnitPriceMinor  int64  `json:"unitPriceMinor"`
	TotalPriceMinor int64  `json:"totalPriceMinor"`
}

type LedgerEmailOrder struct {
	ID                   uuid.UUID              `json:"id"`
	EmailAccountID       uuid.UUID              `json:"emailAccountId"`
	ImporterID           string                 `json:"importerId"`
	ExternalOrderID      string                 `json:"externalOrderId,omitempty"`
	OrderDate            string                 `json:"orderDate"`
	TotalMinor           int64                  `json:"totalMinor"`
	Currency             string                 `json:"currency"`
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

type LedgerEmailImporterInfo struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Senders     []string `json:"senders"`
	Subjects    []string `json:"subjects"`
}

type LedgerEmailScanResult struct {
	EmailsScanned int                `json:"emailsScanned"`
	OrdersFound   int                `json:"ordersFound"`
	OrdersNew     int                `json:"ordersNew"`
	OrdersLinked  int                `json:"ordersLinked"`
	Warnings      []string           `json:"warnings,omitempty"`
	Orders        []LedgerEmailOrder `json:"orders,omitempty"`
}

func (a *LedgerEmailAccountInput) Validate() error {
	a.Name = strings.TrimSpace(a.Name)
	a.IMAPHost = strings.TrimSpace(a.IMAPHost)
	a.Username = strings.TrimSpace(a.Username)
	a.Password = strings.TrimSpace(a.Password)
	a.ScanSince = strings.TrimSpace(a.ScanSince)
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
