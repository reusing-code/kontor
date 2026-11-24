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
	EmailSeenUnusable   = "unusable"
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
	ID                uuid.UUID `json:"id"`
	Name              string    `json:"name"`
	IMAPHost          string    `json:"imapHost"`
	IMAPPort          int       `json:"imapPort"`
	Username          string    `json:"username"`
	EncryptedPassword string    `json:"-"`
	UseTLS            bool      `json:"useTls"`
	Folder            string    `json:"folder"`
	// DraftsFolder is the optional folder whose messages are proposed as
	// drafts in the draft inbox; it differs from Folder.
	DraftsFolder string `json:"draftsFolder,omitempty"`
	// ProcessedAction is what happens in the mailbox to a message of Folder
	// once it is processed (EmailActionKeep when empty); ProcessedFolder is
	// the target of EmailActionMove. DraftsProcessed* are the same for
	// DraftsFolder.
	ProcessedAction       string     `json:"processedAction,omitempty"`
	ProcessedFolder       string     `json:"processedFolder,omitempty"`
	DraftsProcessedAction string     `json:"draftsProcessedAction,omitempty"`
	DraftsProcessedFolder string     `json:"draftsProcessedFolder,omitempty"`
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
	DraftsFolder          string     `json:"draftsFolder,omitempty"`
	ProcessedAction       string     `json:"processedAction,omitempty"`
	ProcessedFolder       string     `json:"processedFolder,omitempty"`
	DraftsProcessedAction string     `json:"draftsProcessedAction,omitempty"`
	DraftsProcessedFolder string     `json:"draftsProcessedFolder,omitempty"`
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
	Name         string `json:"name"`
	IMAPHost     string `json:"imapHost"`
	IMAPPort     int    `json:"imapPort"`
	Username     string `json:"username"`
	Password     string `json:"password,omitempty"`
	UseTLS       bool   `json:"useTls"`
	Folder       string `json:"folder"`
	DraftsFolder string `json:"draftsFolder"`
	EmailProcessedActions
	PreClassify bool   `json:"preClassify"`
	ScanSince   string `json:"scanSince"`
}

// LedgerEmailFoldersInput holds the connection as entered in the account
// dialog. Without a password the stored one of AccountID is used.
type LedgerEmailFoldersInput struct {
	AccountID *uuid.UUID `json:"accountId,omitempty"`
	IMAPHost  string     `json:"imapHost"`
	IMAPPort  int        `json:"imapPort"`
	Username  string     `json:"username"`
	Password  string     `json:"password,omitempty"`
	UseTLS    bool       `json:"useTls"`
}

func (a *LedgerEmailFoldersInput) Validate() error {
	a.IMAPHost = strings.TrimSpace(a.IMAPHost)
	a.Username = strings.TrimSpace(a.Username)
	a.Password = strings.TrimSpace(a.Password)
	if a.IMAPHost == "" {
		return errors.New("imapHost is required")
	}
	if a.Username == "" {
		return errors.New("username is required")
	}
	return validateIMAPConnection(a.IMAPHost, &a.IMAPPort, a.Username, a.Password)
}

const defaultIMAPPort = 993

// validateIMAPConnection is the one check of IMAP connection settings: the
// port (0 means the default) must be a TCP port, and host, username and
// password go into IMAP commands, where a line break would start another
// command, so they must not contain control characters.
func validateIMAPConnection(host string, port *int, username, password string) error {
	if *port == 0 {
		*port = defaultIMAPPort
	}
	if *port < 1 || *port > 65535 {
		return errors.New("imapPort is invalid")
	}
	for _, value := range []string{host, username, password} {
		if strings.IndexFunc(value, unicode.IsControl) >= 0 {
			return errors.New("connection settings must not contain control characters")
		}
	}
	return nil
}

// validImportedEmailAccount checks what an export carries into IMAP
// commands; exports have no password.
func validImportedEmailAccount(a LedgerEmailAccount) bool {
	port := a.IMAPPort
	if validateIMAPConnection(a.IMAPHost, &port, a.Username, "") != nil {
		return false
	}
	for _, folder := range []string{a.Folder, a.DraftsFolder, a.ProcessedFolder, a.DraftsProcessedFolder} {
		if strings.IndexFunc(folder, unicode.IsControl) >= 0 {
			return false
		}
	}
	return true
}

type LedgerEmailAccountUpdateInput struct {
	Name         string  `json:"name"`
	IMAPHost     string  `json:"imapHost"`
	IMAPPort     int     `json:"imapPort"`
	Username     string  `json:"username"`
	Password     *string `json:"password,omitempty"`
	UseTLS       bool    `json:"useTls"`
	Folder       string  `json:"folder"`
	DraftsFolder string  `json:"draftsFolder"`
	EmailProcessedActions
	PreClassify bool   `json:"preClassify"`
	ScanSince   string `json:"scanSince"`
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
	// AmountComparable is false when the transaction's currency differs from
	// the order's and no exchange rate was available.
	AmountComparable bool `json:"amountComparable"`
}

// LedgerEmailOrderConversion is the order total in another currency at the
// ECB reference rate published on RateDate.
type LedgerEmailOrderConversion struct {
	Currency   string  `json:"currency"`
	TotalMinor int64   `json:"totalMinor"`
	Rate       float64 `json:"rate"`
	RateDate   string  `json:"rateDate"`
}

type LedgerEmailOrderCandidatesResult struct {
	Items []LedgerEmailOrderCandidate `json:"items"`
	ChargeWindow
	// Conversions holds the order total in each ledger account currency that
	// differs from the order's; UnconvertedCurrencies lists those without a rate.
	Conversions           []LedgerEmailOrderConversion `json:"conversions,omitempty"`
	UnconvertedCurrencies []string                     `json:"unconvertedCurrencies,omitempty"`
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

// normalizeEmailDraftsFolder validates the optional drafts folder, which
// must not be the folder scanned for orders.
func normalizeEmailDraftsFolder(draftsFolder, folder string) (string, error) {
	if strings.TrimSpace(draftsFolder) == "" {
		return "", nil
	}
	draftsFolder, err := normalizeEmailFolder(draftsFolder)
	if err != nil {
		return "", errors.New("draftsFolder is invalid")
	}
	if draftsFolder == folder {
		return "", errors.New("draftsFolder must differ from folder")
	}
	return draftsFolder, nil
}

// Actions for processed messages of a folder.
const (
	EmailActionKeep   = "keep"
	EmailActionMove   = "move"
	EmailActionDelete = "delete"
)

// EmailProcessedActions are the account fields that say what happens in the
// mailbox to processed messages, per scanned folder.
type EmailProcessedActions struct {
	ProcessedAction       string `json:"processedAction"`
	ProcessedFolder       string `json:"processedFolder"`
	DraftsProcessedAction string `json:"draftsProcessedAction"`
	DraftsProcessedFolder string `json:"draftsProcessedFolder"`
}

// normalize validates the actions against the (already normalised) scanned
// folders. A move target must be neither of them: a processed message moved
// into the other scanned folder would be picked up by the other pipeline.
func (p *EmailProcessedActions) normalize(folder, draftsFolder string) error {
	var err error
	if p.ProcessedAction, p.ProcessedFolder, err = normalizeEmailProcessedAction("processed", p.ProcessedAction, p.ProcessedFolder, folder, draftsFolder); err != nil {
		return err
	}
	if draftsFolder == "" {
		p.DraftsProcessedAction, p.DraftsProcessedFolder = EmailActionKeep, ""
		return nil
	}
	p.DraftsProcessedAction, p.DraftsProcessedFolder, err = normalizeEmailProcessedAction("draftsProcessed", p.DraftsProcessedAction, p.DraftsProcessedFolder, folder, draftsFolder)
	return err
}

// normalizeEmailProcessedAction names the fields in its errors as
// prefix+"Action" and prefix+"Folder".
func normalizeEmailProcessedAction(prefix, action, target, folder, draftsFolder string) (string, string, error) {
	switch action = strings.TrimSpace(action); action {
	case "", EmailActionKeep:
		return EmailActionKeep, "", nil
	case EmailActionDelete:
		return EmailActionDelete, "", nil
	case EmailActionMove:
	default:
		return "", "", errors.New(prefix + "Action is invalid")
	}
	if strings.TrimSpace(target) == "" {
		return "", "", errors.New(prefix + "Folder is required")
	}
	target, err := normalizeEmailFolder(target)
	if err != nil {
		return "", "", errors.New(prefix + "Folder is invalid")
	}
	if target == folder || target == draftsFolder {
		return "", "", errors.New(prefix + "Folder must differ from folder and draftsFolder")
	}
	return EmailActionMove, target, nil
}

// apply copies the validated actions to the account.
func (p EmailProcessedActions) apply(a *LedgerEmailAccount) {
	a.ProcessedAction, a.ProcessedFolder = p.ProcessedAction, p.ProcessedFolder
	a.DraftsProcessedAction, a.DraftsProcessedFolder = p.DraftsProcessedAction, p.DraftsProcessedFolder
}

// Email sources of an account: the folder scanned for orders and the folder
// of the Kontor inbox.
const (
	emailSourceOrders = "o"
	emailSourceDrafts = "d"
)

// processedAction returns the action for processed messages of a source and
// its move target.
func (a LedgerEmailAccount) processedAction(source string) (action, target string) {
	action, target = a.ProcessedAction, a.ProcessedFolder
	if source == emailSourceDrafts {
		action, target = a.DraftsProcessedAction, a.DraftsProcessedFolder
	}
	if action == "" {
		action = EmailActionKeep
	}
	return action, target
}

// sourceFolder is the scanned folder of a source.
func (a LedgerEmailAccount) sourceFolder(source string) string {
	if source == emailSourceDrafts {
		return a.DraftsFolder
	}
	return a.Folder
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
	if a.DraftsFolder, err = normalizeEmailDraftsFolder(a.DraftsFolder, folder); err != nil {
		return err
	}
	if err := a.normalize(folder, a.DraftsFolder); err != nil {
		return err
	}
	if a.Name == "" {
		return errors.New("name is required")
	}
	if a.IMAPHost == "" {
		return errors.New("imapHost is required")
	}
	if a.Username == "" {
		return errors.New("username is required")
	}
	if a.Password == "" {
		return errors.New("password is required")
	}
	if err := validateIMAPConnection(a.IMAPHost, &a.IMAPPort, a.Username, a.Password); err != nil {
		return err
	}
	if a.ScanSince == "" {
		return errors.New("scanSince is required")
	}
	if _, err := time.Parse("2006-01-02", a.ScanSince); err != nil {
		return ErrLedgerScanSinceInvalid
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
	if a.DraftsFolder, err = normalizeEmailDraftsFolder(a.DraftsFolder, folder); err != nil {
		return err
	}
	if err := a.normalize(folder, a.DraftsFolder); err != nil {
		return err
	}
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
	if a.Username == "" {
		return errors.New("username is required")
	}
	password := ""
	if a.Password != nil {
		password = *a.Password
	}
	if err := validateIMAPConnection(a.IMAPHost, &a.IMAPPort, a.Username, password); err != nil {
		return err
	}
	if a.ScanSince == "" {
		return errors.New("scanSince is required")
	}
	if _, err := time.Parse("2006-01-02", a.ScanSince); err != nil {
		return ErrLedgerScanSinceInvalid
	}
	return nil
}

// Validation errors of email accounts and order links, answered with 400.
var (
	ErrLedgerScanSinceInvalid    = errors.New("scanSince must be in YYYY-MM-DD format")
	ErrLedgerTransactionIDsEmpty = errors.New("transactionIds must contain at least one id")
)

func (i *LedgerEmailOrderLinkInput) Validate() error {
	i.TransactionIDs = NormalizeLinkedTransactionIDs(i.TransactionIDs)
	if len(i.TransactionIDs) == 0 {
		return ErrLedgerTransactionIDsEmpty
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
