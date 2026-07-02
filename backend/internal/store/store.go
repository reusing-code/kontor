package store

import (
	"context"

	"github.com/google/uuid"
	"github.com/reusing-code/kontor/backend/internal/model"
)

type LedgerTransactionPage struct {
	Items      []model.LedgerTransaction
	NextCursor string
}

type LedgerTransactionListOptions struct {
	AccountID    *uuid.UUID
	CategoryID   *uuid.UUID
	ReviewStatus string
	Limit        int
	Cursor       string
}

type LedgerReviewResult struct {
	Transaction model.LedgerTransaction
	Category    *model.LedgerCategory
}

type LedgerTransferCandidatesResult struct {
	Items []model.LedgerTransferCandidate
}

type LedgerTransferLinkResult struct {
	Transaction       model.LedgerTransaction
	PairedTransaction *model.LedgerTransaction
}

type LedgerImportCommitResult struct {
	ImportedRows  int
	DuplicateRows int
}

type Store interface {
	CreateUser(ctx context.Context, u model.User) error
	GetUserByEmail(ctx context.Context, email string) (model.User, error)
	GetUserByID(ctx context.Context, id string) (model.User, error)
	UpdateUser(ctx context.Context, u model.User) error
	ListUsers(ctx context.Context) ([]model.User, error)

	GetSettings(ctx context.Context, userID string) (model.UserSettings, error)
	UpdateSettings(ctx context.Context, userID string, s model.UserSettings) error

	ListCategories(ctx context.Context, userID string, module string) ([]model.Category, error)
	GetCategory(ctx context.Context, userID string, module string, id uuid.UUID) (model.Category, error)
	CreateCategory(ctx context.Context, userID string, module string, c model.Category) error
	UpdateCategory(ctx context.Context, userID string, module string, c model.Category) error
	DeleteCategory(ctx context.Context, userID string, module string, id uuid.UUID) error

	ListContracts(ctx context.Context, userID string) ([]model.Contract, error)
	ListContractsByCategory(ctx context.Context, userID string, categoryID uuid.UUID) ([]model.Contract, error)
	GetContract(ctx context.Context, userID string, id uuid.UUID) (model.Contract, error)
	CreateContract(ctx context.Context, userID string, c model.Contract) error
	UpdateContract(ctx context.Context, userID string, c model.Contract) error
	DeleteContract(ctx context.Context, userID string, id uuid.UUID) error

	ListPurchases(ctx context.Context, userID string) ([]model.Purchase, error)
	ListPurchasesByCategory(ctx context.Context, userID string, categoryID uuid.UUID) ([]model.Purchase, error)
	GetPurchase(ctx context.Context, userID string, id uuid.UUID) (model.Purchase, error)
	CreatePurchase(ctx context.Context, userID string, p model.Purchase) error
	UpdatePurchase(ctx context.Context, userID string, p model.Purchase) error
	DeletePurchase(ctx context.Context, userID string, id uuid.UUID) error

	ListVehicles(ctx context.Context, userID string) ([]model.Vehicle, error)
	GetVehicle(ctx context.Context, userID string, id uuid.UUID) (model.Vehicle, error)
	CreateVehicle(ctx context.Context, userID string, v model.Vehicle) error
	UpdateVehicle(ctx context.Context, userID string, v model.Vehicle) error
	DeleteVehicle(ctx context.Context, userID string, id uuid.UUID) error

	ListCostEntries(ctx context.Context, userID string, vehicleID uuid.UUID) ([]model.CostEntry, error)
	GetCostEntry(ctx context.Context, userID string, id uuid.UUID) (model.CostEntry, error)
	CreateCostEntry(ctx context.Context, userID string, c model.CostEntry) error
	UpdateCostEntry(ctx context.Context, userID string, c model.CostEntry) error
	DeleteCostEntry(ctx context.Context, userID string, id uuid.UUID) error

	ListLedgerAccounts(ctx context.Context, userID string) ([]model.LedgerAccount, error)
	GetLedgerAccount(ctx context.Context, userID string, id uuid.UUID) (model.LedgerAccount, error)
	FindLedgerAccountByIBAN(ctx context.Context, userID string, iban string) (model.LedgerAccount, error)
	CreateLedgerAccount(ctx context.Context, userID string, a model.LedgerAccount) error

	ListLedgerCategories(ctx context.Context, userID string) ([]model.LedgerCategory, error)
	GetLedgerCategory(ctx context.Context, userID string, id uuid.UUID) (model.LedgerCategory, error)
	CreateLedgerCategory(ctx context.Context, userID string, c model.LedgerCategory) error
	UpdateLedgerCategory(ctx context.Context, userID string, c model.LedgerCategory) error
	DeleteLedgerCategory(ctx context.Context, userID string, id uuid.UUID) error

	GetLedgerImportByFileHash(ctx context.Context, userID string, sha256 string) (model.LedgerImportBatch, error)
	LedgerTransactionFingerprintExists(ctx context.Context, userID string, fingerprint string) (bool, error)
	CommitLedgerImport(ctx context.Context, userID string, batch model.LedgerImportBatch, txns []model.LedgerTransaction) (LedgerImportCommitResult, error)
	ListLedgerImports(ctx context.Context, userID string) ([]model.LedgerImportBatch, error)

	ListLedgerTransactions(ctx context.Context, userID string, accountID uuid.UUID) ([]model.LedgerTransaction, error)
	ListLedgerTransactionsPage(ctx context.Context, userID string, accountID uuid.UUID, limit int, cursor string) (LedgerTransactionPage, error)
	ListLedgerTransactionsFiltered(ctx context.Context, userID string, options LedgerTransactionListOptions) (LedgerTransactionPage, error)
	GetLedgerTransaction(ctx context.Context, userID string, id uuid.UUID) (model.LedgerTransaction, error)
	ListLedgerTransferCandidates(ctx context.Context, userID string, id uuid.UUID) (LedgerTransferCandidatesResult, error)
	LinkLedgerTransfer(ctx context.Context, userID string, id uuid.UUID, input model.LedgerTransferLinkInput) (model.LedgerTransferLinkResult, error)
	UnlinkLedgerTransfer(ctx context.Context, userID string, id uuid.UUID) (LedgerTransferLinkResult, error)
	UpdateLedgerTransactionDetails(ctx context.Context, userID string, id uuid.UUID, input model.LedgerTransactionDetailsInput) (model.LedgerTransaction, error)
	ReviewLedgerTransaction(ctx context.Context, userID string, id uuid.UUID, input model.LedgerTransactionReviewInput) (LedgerReviewResult, error)

	ListLedgerEmailAccounts(ctx context.Context, userID string) ([]model.LedgerEmailAccount, error)
	GetLedgerEmailAccount(ctx context.Context, userID string, id uuid.UUID) (model.LedgerEmailAccount, error)
	CreateLedgerEmailAccount(ctx context.Context, userID string, account model.LedgerEmailAccount) error
	UpdateLedgerEmailAccount(ctx context.Context, userID string, account model.LedgerEmailAccount) error
	DeleteLedgerEmailAccount(ctx context.Context, userID string, id uuid.UUID) error
	ListLedgerEmailOrders(ctx context.Context, userID string) ([]model.LedgerEmailOrder, error)
	ListLedgerEmailOrdersByAccount(ctx context.Context, userID string, accountID uuid.UUID) ([]model.LedgerEmailOrder, error)
	ListLedgerEmailOrdersByTransaction(ctx context.Context, userID string, transactionID uuid.UUID) ([]model.LedgerEmailOrder, error)
	GetLedgerEmailOrder(ctx context.Context, userID string, id uuid.UUID) (model.LedgerEmailOrder, error)
	GetLedgerEmailOrderByMessageID(ctx context.Context, userID string, messageID string) (model.LedgerEmailOrder, error)
	CreateLedgerEmailOrder(ctx context.Context, userID string, order model.LedgerEmailOrder) error
	LinkLedgerEmailOrder(ctx context.Context, userID string, id uuid.UUID, input model.LedgerEmailOrderLinkInput) (model.LedgerEmailOrder, error)
	RejectLedgerEmailOrder(ctx context.Context, userID string, id uuid.UUID) (model.LedgerEmailOrder, error)

	Close() error
}
