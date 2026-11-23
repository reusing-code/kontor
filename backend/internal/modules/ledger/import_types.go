package ledger

import "time"

const ParserVersion = "1"

type SourceType string

const (
	SourceDKBCSV       SourceType = "dkb.csv"
	SourceComdirectCSV SourceType = "comdirect.csv"
	// SourceEnableBanking marks transactions synced through the Enable Banking
	// API. It has no Provider: there is no file to parse.
	SourceEnableBanking SourceType = "enablebanking"
)

type ParsedRow struct {
	BookingDate      string `json:"bookingDate"`
	ValueDate        string `json:"valueDate,omitempty"`
	AmountMinor      int64  `json:"amountMinor"`
	Currency         string `json:"currency"`
	CounterpartyName string `json:"counterpartyName,omitempty"`
	CounterpartyIBAN string `json:"counterpartyIban,omitempty"`
	Purpose          string `json:"purpose,omitempty"`
	BankReference    string `json:"bankReference,omitempty"`
}

// ParsedBalance is the balance a file reports: at the end of day AsOf,
// including every row booked on or before it.
type ParsedBalance struct {
	AmountMinor int64  `json:"amountMinor"`
	AsOf        string `json:"asOf"`
}

type ParseResult struct {
	IBAN     string
	BankName string
	Rows     []ParsedRow
	Warnings []string
	// Balance is nil when the file has none or it could not be read.
	Balance *ParsedBalance
}

type PreviewTransaction struct {
	Row                   ParsedRow `json:"row"`
	Fingerprint           string    `json:"fingerprint"`
	IsDuplicate           bool      `json:"isDuplicate"`
	SuggestedCategoryID   string    `json:"suggestedCategoryId,omitempty"`
	SuggestedCategoryName string    `json:"suggestedCategoryName,omitempty"`
	ReviewStatus          string    `json:"reviewStatus"`
	CategorizationSource  string    `json:"categorizationSource"`
}

type PreviewResult struct {
	PreviewID     string               `json:"previewId"`
	SourceType    SourceType           `json:"sourceType"`
	Filename      string               `json:"filename"`
	FileSHA256    string               `json:"fileSha256"`
	AccountID     string               `json:"accountId,omitempty"`
	IBAN          string               `json:"iban,omitempty"`
	BankName      string               `json:"bankName,omitempty"`
	Transactions  []PreviewTransaction `json:"transactions"`
	TotalRows     int                  `json:"totalRows"`
	NewRows       int                  `json:"newRows"`
	DuplicateRows int                  `json:"duplicateRows"`
	Warnings      []string             `json:"warnings,omitempty"`
	Balance       *ParsedBalance       `json:"balance,omitempty"`
	ExpiresAt     time.Time            `json:"expiresAt"`
}
