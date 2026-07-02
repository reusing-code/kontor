package ledger

import (
	"time"

	"github.com/google/uuid"
)

const (
	ImportStatusParsed    = "parsed"
	ImportStatusCommitted = "committed"
	ImportStatusFailed    = "failed"
)

type LedgerImportBatch struct {
	ID            uuid.UUID `json:"id"`
	AccountID     uuid.UUID `json:"accountId"`
	SourceType    string    `json:"sourceType"`
	ParserVersion string    `json:"parserVersion"`
	Filename      string    `json:"filename"`
	FileSHA256    string    `json:"fileSha256"`
	Status        string    `json:"status"`
	TotalRows     int       `json:"totalRows"`
	ImportedRows  int       `json:"importedRows"`
	DuplicateRows int       `json:"duplicateRows"`
	ErrorRows     int       `json:"errorRows"`
	Warnings      []string  `json:"warnings,omitempty"`
	CreatedAt     time.Time `json:"createdAt"`
	UpdatedAt     time.Time `json:"updatedAt"`
}
