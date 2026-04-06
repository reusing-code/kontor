package ledgerimport

import "time"

const ParserVersion = "1"

type SourceType string

const (
	SourceDKBCSV       SourceType = "dkb.csv"
	SourceComdirectCSV SourceType = "comdirect.csv"
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
	TransactionType  string `json:"transactionType,omitempty"`
}

type ParseResult struct {
	IBAN     string
	BankName string
	Rows     []ParsedRow
	Warnings []string
}

type PreviewTransaction struct {
	Row         ParsedRow `json:"row"`
	Fingerprint string    `json:"fingerprint"`
	IsDuplicate bool      `json:"isDuplicate"`
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
	ExpiresAt     time.Time            `json:"expiresAt"`
}
