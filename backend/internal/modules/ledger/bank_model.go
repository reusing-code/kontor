package ledger

import (
	"fmt"
	"strings"
	"time"
	"unicode"

	"github.com/google/uuid"
)

// bankExpiringWindow is how long before its end a consent counts as expiring.
const bankExpiringWindow = 14 * 24 * time.Hour

const (
	BankStatusActive   = "active"
	BankStatusExpiring = "expiring"
	BankStatusExpired  = "expired"
)

// Per-account sync status codes. The UI translates them; statusMessage is the
// English fallback.
const (
	BankSyncOK                   = "ok"
	BankSyncUnmapped             = "unmapped"
	BankSyncConsentExpired       = "consent_expired"
	BankSyncAccessDenied         = "access_denied"
	BankSyncAccountNotAccessible = "account_not_accessible"
	BankSyncRateLimited          = "rate_limited"
	BankSyncBankUnavailable      = "bank_unavailable"
	BankSyncAppError             = "app_error"
	BankSyncTimeout              = "timeout"
	BankSyncError                = "error"
)

// LedgerBankAccount is one account of a bank consent. LedgerAccountID is the
// explicit mapping to a ledger account; only mapped accounts are synced.
type LedgerBankAccount struct {
	UID                string     `json:"uid"`
	IBAN               string     `json:"iban"`
	Currency           string     `json:"currency"`
	Name               string     `json:"name"`
	IdentificationHash string     `json:"identificationHash,omitempty"`
	LedgerAccountID    *uuid.UUID `json:"ledgerAccountId,omitempty"`
	LastSyncAttemptAt  *time.Time `json:"lastSyncAttemptAt,omitempty"`
	LastSyncAt         *time.Time `json:"lastSyncAt,omitempty"`
	LastSyncStatus     string     `json:"lastSyncStatus,omitempty"`
	LastImported       int        `json:"lastImported"`
	LastDuplicates     int        `json:"lastDuplicates"`
}

// LedgerBankConnection is a user's consent at one bank. EncryptedSessionID is
// the AES-GCM encrypted Enable Banking session ID and never leaves the API.
type LedgerBankConnection struct {
	ID                 uuid.UUID           `json:"id"`
	ASPSPName          string              `json:"aspspName"`
	ASPSPCountry       string              `json:"aspspCountry"`
	EncryptedSessionID string              `json:"-"`
	AuthorizedAt       time.Time           `json:"authorizedAt"`
	ValidUntil         time.Time           `json:"validUntil"`
	LostAt             *time.Time          `json:"lostAt,omitempty"`
	NotifiedDays       []int               `json:"notifiedDays,omitempty"`
	Accounts           []LedgerBankAccount `json:"accounts"`
	CreatedAt          time.Time           `json:"createdAt"`
	UpdatedAt          time.Time           `json:"updatedAt"`
}

// storableLedgerBankConnection includes EncryptedSessionID for persistence
// (LedgerBankConnection has json:"-" on it so it never leaves the API).
type storableLedgerBankConnection struct {
	ID                 uuid.UUID           `json:"id"`
	ASPSPName          string              `json:"aspspName"`
	ASPSPCountry       string              `json:"aspspCountry"`
	EncryptedSessionID string              `json:"encryptedSessionId"`
	AuthorizedAt       time.Time           `json:"authorizedAt"`
	ValidUntil         time.Time           `json:"validUntil"`
	LostAt             *time.Time          `json:"lostAt,omitempty"`
	NotifiedDays       []int               `json:"notifiedDays,omitempty"`
	Accounts           []LedgerBankAccount `json:"accounts"`
	CreatedAt          time.Time           `json:"createdAt"`
	UpdatedAt          time.Time           `json:"updatedAt"`
}

// BankConnectionStatus derives active, expiring or expired. A consent that was
// reported lost by the bank counts as expired.
func BankConnectionStatus(c LedgerBankConnection, now time.Time) string {
	remaining := c.ValidUntil.Sub(now)
	switch {
	case c.LostAt != nil || remaining <= 0:
		return BankStatusExpired
	case remaining < bankExpiringWindow:
		return BankStatusExpiring
	default:
		return BankStatusActive
	}
}

// BankAccountView is the API shape of a bank account.
type BankAccountView struct {
	UID               string     `json:"uid"`
	IBAN              string     `json:"iban"`
	Currency          string     `json:"currency"`
	Name              string     `json:"name"`
	LedgerAccountID   *uuid.UUID `json:"ledgerAccountId,omitempty"`
	Mapped            bool       `json:"mapped"`
	LastSyncAt        *time.Time `json:"lastSyncAt,omitempty"`
	LastSyncAttemptAt *time.Time `json:"lastSyncAttemptAt,omitempty"`
	LastSyncStatus    string     `json:"lastSyncStatus"`
	StatusMessage     string     `json:"statusMessage"`
	LastImported      int        `json:"lastImported"`
	LastDuplicates    int        `json:"lastDuplicates"`
}

// BankConnectionView is the API shape of a connection; it never carries the session.
type BankConnectionView struct {
	ID           uuid.UUID         `json:"id"`
	ASPSPName    string            `json:"aspspName"`
	ASPSPCountry string            `json:"aspspCountry"`
	AuthorizedAt time.Time         `json:"authorizedAt"`
	ExpiresAt    time.Time         `json:"expiresAt"`
	Status       string            `json:"status"`
	Accounts     []BankAccountView `json:"accounts"`
}

func NewBankConnectionView(c LedgerBankConnection, now time.Time) BankConnectionView {
	view := BankConnectionView{
		ID:           c.ID,
		ASPSPName:    c.ASPSPName,
		ASPSPCountry: c.ASPSPCountry,
		AuthorizedAt: c.AuthorizedAt,
		ExpiresAt:    c.ValidUntil,
		Status:       BankConnectionStatus(c, now),
		Accounts:     make([]BankAccountView, 0, len(c.Accounts)),
	}
	for _, a := range c.Accounts {
		status := a.LastSyncStatus
		if status == "" && a.LedgerAccountID == nil {
			status = BankSyncUnmapped
		}
		view.Accounts = append(view.Accounts, BankAccountView{
			UID:               a.UID,
			IBAN:              a.IBAN,
			Currency:          a.Currency,
			Name:              a.Name,
			LedgerAccountID:   a.LedgerAccountID,
			Mapped:            a.LedgerAccountID != nil,
			LastSyncAt:        a.LastSyncAt,
			LastSyncAttemptAt: a.LastSyncAttemptAt,
			LastSyncStatus:    status,
			StatusMessage:     bankStatusMessage(status, a.LastImported, a.LastDuplicates),
			LastImported:      a.LastImported,
			LastDuplicates:    a.LastDuplicates,
		})
	}
	return view
}

func bankStatusMessage(code string, imported, duplicates int) string {
	switch code {
	case "":
		return "Not synced yet"
	case BankSyncOK:
		return fmt.Sprintf("Synced: %d imported, %d already known", imported, duplicates)
	case BankSyncUnmapped:
		return "Not mapped to a ledger account"
	case BankSyncConsentExpired:
		return "Bank consent expired, authorize again"
	case BankSyncAccessDenied:
		return "The bank denied access to this account"
	case BankSyncAccountNotAccessible:
		return "This account is not accessible with the current Enable Banking app"
	case BankSyncRateLimited:
		return "Rate limit reached, will retry later"
	case BankSyncBankUnavailable:
		return "The bank is unavailable, will retry later"
	case BankSyncAppError:
		return "Enable Banking rejected the app credentials"
	case BankSyncTimeout:
		return "The sync timed out"
	default:
		return "The sync failed"
	}
}

// normalizeIBAN upper-cases an IBAN and removes all whitespace.
func normalizeIBAN(s string) string {
	return strings.ToUpper(strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) {
			return -1
		}
		return r
	}, s))
}
