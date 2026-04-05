package store

import (
	"context"

	"github.com/google/uuid"
	"github.com/tobi/contracts/backend/internal/model"
)

type LedgerTransactionPage struct {
	Items      []model.LedgerTransaction
	NextCursor string
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

	GetLedgerImportByFileHash(ctx context.Context, userID string, sha256 string) (model.LedgerImportBatch, error)
	LedgerTransactionFingerprintExists(ctx context.Context, userID string, fingerprint string) (bool, error)
	CommitLedgerImport(ctx context.Context, userID string, batch model.LedgerImportBatch, txns []model.LedgerTransaction) (LedgerImportCommitResult, error)
	ListLedgerImports(ctx context.Context, userID string) ([]model.LedgerImportBatch, error)

	ListLedgerTransactions(ctx context.Context, userID string, accountID uuid.UUID) ([]model.LedgerTransaction, error)
	ListLedgerTransactionsPage(ctx context.Context, userID string, accountID uuid.UUID, limit int, cursor string) (LedgerTransactionPage, error)

	Close() error
}
