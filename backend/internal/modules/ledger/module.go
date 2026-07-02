package ledger

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/reusing-code/kontor/backend/internal/core"
	"github.com/reusing-code/kontor/backend/internal/cryptoutil"
	"github.com/reusing-code/kontor/backend/internal/httputil"
	"github.com/reusing-code/kontor/backend/internal/module"
	"github.com/reusing-code/kontor/backend/internal/storage"
	"github.com/reusing-code/kontor/backend/internal/storage/link"
)

const ModuleID = "ledger"

type Handler struct {
	store              *Store
	logger             *slog.Logger
	ledgerImport       *ImportService
	ledgerEmail        *EmailService
	emailEncryptionKey []byte
}

type Module struct {
	store            *Store
	coreStore        *core.Store
	handler          *Handler
	logger           *slog.Logger
	emailScanEnabled bool
	scanInterval     time.Duration
	encryptionKey    []byte
}

// Config carries the ledger module's background scan settings.
type Config struct {
	EmailScanInterval  time.Duration
	EmailEncryptionKey string
}

func New(e *storage.Engine, links *link.Registry, coreStore *core.Store, cfg Config, logger *slog.Logger) *Module {
	store := NewStore(e, links)

	encryptionKey, err := cryptoutil.NormalizeEncryptionKey(cfg.EmailEncryptionKey)
	if err != nil {
		logger.Warn("ledger email encryption key unavailable", "error", err)
	}

	handler := &Handler{
		store:              store,
		logger:             logger,
		ledgerImport:       NewImportService(store, logger),
		ledgerEmail:        NewEmailService(store, logger),
		emailEncryptionKey: encryptionKey,
	}

	scanEnabled := true
	if cfg.EmailScanInterval <= 0 {
		logger.Info("ledger email scan scheduler disabled", "reason", "LEDGER_EMAIL_SCAN_INTERVAL is 0")
		scanEnabled = false
	} else if len(encryptionKey) == 0 {
		logger.Info("ledger email scan scheduler disabled", "reason", "no valid email encryption key")
		scanEnabled = false
	}

	return &Module{
		store:            store,
		coreStore:        coreStore,
		handler:          handler,
		logger:           logger,
		emailScanEnabled: scanEnabled,
		scanInterval:     cfg.EmailScanInterval,
		encryptionKey:    encryptionKey,
	}
}

func (m *Module) ID() string { return ModuleID }

func (m *Module) Store() *Store { return m.store }

func (m *Module) RegisterRoutes(r *module.Router) {
	h := m.handler
	r.Handle("POST /api/v1/ledger/imports/preview", h.LedgerImportPreview)
	r.Handle("POST /api/v1/ledger/imports/{previewId}/commit", h.LedgerImportCommit)
	r.Handle("GET /api/v1/ledger/categories", h.ListLedgerCategories)
	r.Handle("POST /api/v1/ledger/categories", h.CreateLedgerCategory)
	r.Handle("GET /api/v1/ledger/categories/{id}", h.GetLedgerCategory)
	r.Handle("PUT /api/v1/ledger/categories/{id}", h.UpdateLedgerCategory)
	r.Handle("DELETE /api/v1/ledger/categories/{id}", h.DeleteLedgerCategory)
	r.Handle("GET /api/v1/ledger/accounts", h.ListLedgerAccounts)
	r.Handle("GET /api/v1/ledger/accounts/{accountId}", h.GetLedgerAccount)
	r.Handle("GET /api/v1/ledger/accounts/{accountId}/transactions", h.ListLedgerTransactions)
	r.Handle("GET /api/v1/ledger/email-accounts", h.ListLedgerEmailAccounts)
	r.Handle("POST /api/v1/ledger/email-accounts", h.CreateLedgerEmailAccount)
	r.Handle("GET /api/v1/ledger/email-accounts/{emailAccountId}", h.GetLedgerEmailAccount)
	r.Handle("PUT /api/v1/ledger/email-accounts/{emailAccountId}", h.UpdateLedgerEmailAccount)
	r.Handle("DELETE /api/v1/ledger/email-accounts/{emailAccountId}", h.DeleteLedgerEmailAccount)
	r.Handle("POST /api/v1/ledger/email-accounts/{emailAccountId}/test", h.TestLedgerEmailAccount)
	r.Handle("POST /api/v1/ledger/email-accounts/{emailAccountId}/scan", h.ScanLedgerEmailAccount)
	r.Handle("GET /api/v1/ledger/email-orders", h.ListLedgerEmailOrders)
	r.Handle("GET /api/v1/ledger/email-orders/{emailOrderId}", h.GetLedgerEmailOrder)
	r.Handle("POST /api/v1/ledger/email-orders/{emailOrderId}/link", h.LinkLedgerEmailOrder)
	r.Handle("POST /api/v1/ledger/email-orders/{emailOrderId}/reject", h.RejectLedgerEmailOrder)
	r.Handle("GET /api/v1/ledger/email-importers", h.ListLedgerEmailImporters)
	r.Handle("GET /api/v1/ledger/imports", h.ListLedgerImports)
	r.Handle("GET /api/v1/ledger/transactions", h.ListLedgerTransactionsReviewQueue)
	r.Handle("GET /api/v1/ledger/transactions/{transactionId}", h.GetLedgerTransaction)
	r.Handle("PUT /api/v1/ledger/transactions/{transactionId}", h.UpdateLedgerTransactionDetails)
	r.Handle("GET /api/v1/ledger/transactions/{transactionId}/transfer-candidates", h.ListLedgerTransferCandidates)
	r.Handle("POST /api/v1/ledger/transactions/{transactionId}/transfer-link", h.LinkLedgerTransfer)
	r.Handle("DELETE /api/v1/ledger/transactions/{transactionId}/transfer-link", h.UnlinkLedgerTransfer)
	r.Handle("POST /api/v1/ledger/transactions/{transactionId}/review", h.ReviewLedgerTransaction)
}

func (m *Module) Seed(ctx context.Context, userID string) error {
	return m.handler.SeedLedgerDefaults(ctx, userID)
}

func (m *Module) StartBackground(ctx context.Context) {
	if !m.emailScanEnabled {
		return
	}
	sched := NewEmailScanScheduler(m.store, m.coreStore, m.handler.ledgerEmail, m.encryptionKey, m.scanInterval, m.logger)
	sched.Start(ctx)
}

// storeError maps ledger store errors, including module-specific sentinels,
// to HTTP responses.
func (h *Handler) storeError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, storage.ErrNotFound):
		httputil.Error(h.logger, w, http.StatusNotFound, "not found")
	case errors.Is(err, storage.ErrConflict), errors.Is(err, ErrLedgerFileImported), errors.Is(err, ErrLedgerTransferLinked):
		httputil.Error(h.logger, w, http.StatusConflict, err.Error())
	case errors.Is(err, ErrLedgerCategoryHasChild), errors.Is(err, ErrLedgerCategoryHasCycle), errors.Is(err, ErrLedgerTransferInvalid):
		httputil.Error(h.logger, w, http.StatusBadRequest, err.Error())
	case err != nil && (err.Error() == "links must contain valid absolute URLs" ||
		err.Error() == "references contain an invalid type" ||
		err.Error() == "scanSince must be in YYYY-MM-DD format" ||
		err.Error() == "transactionIds must contain at least one id"):
		httputil.Error(h.logger, w, http.StatusBadRequest, err.Error())
	case errors.Is(err, ErrLedgerPreviewExpired):
		httputil.Error(h.logger, w, http.StatusGone, err.Error())
	default:
		h.logger.Error("store error", "error", err)
		httputil.Error(h.logger, w, http.StatusInternalServerError, "internal error")
	}
}
