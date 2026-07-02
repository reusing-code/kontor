package storage

import "errors"

var (
	ErrNotFound = errors.New("not found")
	ErrConflict = errors.New("conflict")

	ErrLedgerPreviewExpired   = errors.New("ledger preview expired")
	ErrLedgerFileImported     = errors.New("ledger file already imported")
	ErrLedgerCategoryHasChild = errors.New("ledger category has children")
	ErrLedgerCategoryHasCycle = errors.New("ledger category cycle")
	ErrLedgerTransferInvalid  = errors.New("invalid transfer pair")
	ErrLedgerTransferLinked   = errors.New("linked internal transfers must be unlinked explicitly before assigning a category")
)
