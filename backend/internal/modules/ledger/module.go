package ledger

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/reusing-code/kontor/backend/internal/core"
	"github.com/reusing-code/kontor/backend/internal/cryptoutil"
	"github.com/reusing-code/kontor/backend/internal/drafts"
	"github.com/reusing-code/kontor/backend/internal/enablebanking"
	"github.com/reusing-code/kontor/backend/internal/httputil"
	"github.com/reusing-code/kontor/backend/internal/llm"
	"github.com/reusing-code/kontor/backend/internal/module"
	"github.com/reusing-code/kontor/backend/internal/storage"
	"github.com/reusing-code/kontor/backend/internal/storage/link"
	"github.com/reusing-code/kontor/backend/internal/storage/migration"
)

const ModuleID = "ledger"

type Handler struct {
	store              *Store
	logger             *slog.Logger
	ledgerImport       *ImportService
	ledgerEmail        *EmailService
	suggester          *CategorySuggester
	recurring          *RecurringService
	bank               *BankService
	emailEncryptionKey []byte
	// enabled checks the module of a reference target; nil skips the check.
	enabled module.EnablementSource
}

// afterImport starts the background runs that follow new transactions.
func (h *Handler) afterImport(userID string) {
	h.suggester.RunInBackground(userID)
	h.recurring.RunInBackground(userID)
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
	// Extractor reads order data from emails and suggests categories; nil
	// behaves like llm.Disabled.
	Extractor llm.Extractor
	// Suggest sizes the LLM category suggestion runs; zero values use the
	// defaults.
	Suggest SuggestConfig
	// Drafts receives recurring payments proposed as contracts; nil disables
	// the detection.
	Drafts *drafts.Store
	// Bank enables the Enable Banking import; a nil client disables it.
	Bank BankConfig
}

// BankConfig wires the Enable Banking client into the module.
type BankConfig struct {
	Client    *enablebanking.Client
	PublicURL string
}

func New(e *storage.Engine, links *link.Registry, coreStore *core.Store, cfg Config, logger *slog.Logger) *Module {
	store := NewStore(e, links)

	encryptionKey, err := cryptoutil.NormalizeEncryptionKey(cfg.EmailEncryptionKey)
	if err != nil {
		logger.Warn("ledger email encryption key unavailable", "error", err)
	}

	extractor := cfg.Extractor
	if extractor == nil {
		extractor = llm.Disabled{}
	}

	handler := &Handler{
		store:              store,
		logger:             logger,
		ledgerImport:       NewImportService(store, logger),
		ledgerEmail:        NewEmailService(store, extractor, logger),
		suggester:          NewCategorySuggester(store, extractor, cfg.Suggest, logger),
		emailEncryptionKey: encryptionKey,
		enabled:            coreStore,
	}
	if cfg.Bank.Client != nil {
		if len(encryptionKey) == 0 {
			logger.Warn("bank import disabled", "reason", "no valid email encryption key")
		} else {
			handler.bank = NewBankService(BankServiceConfig{
				Client:        cfg.Bank.Client,
				Store:         store,
				Enabled:       enablement(coreStore),
				EncryptionKey: encryptionKey,
				PublicURL:     cfg.Bank.PublicURL,
				AfterImport:   handler.afterImport,
				Logger:        logger,
			})
		}
	}
	if cfg.Drafts != nil {
		handler.recurring = NewRecurringService(store, cfg.Drafts, coreStore, logger)
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

// enablement returns nil for a nil store, so the typed nil never reaches an interface.
func enablement(s *core.Store) module.EnablementSource {
	if s == nil {
		return nil
	}
	return s
}

// BankImportEnabled reports whether the Enable Banking import is usable.
func (m *Module) BankImportEnabled() bool { return m.handler.bank != nil }

// BankCallback is the public redirect target of the bank consent; the server
// mounts it outside the authenticated API.
func (m *Module) BankCallback(w http.ResponseWriter, r *http.Request) {
	m.handler.BankCallback(w, r)
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
	r.Handle("POST /api/v1/ledger/accounts", h.CreateLedgerAccount)
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
	r.Handle("GET /api/v1/ledger/email-orders/{emailOrderId}/candidates", h.ListLedgerEmailOrderCandidates)
	r.Handle("POST /api/v1/ledger/email-orders/{emailOrderId}/link", h.LinkLedgerEmailOrder)
	r.Handle("POST /api/v1/ledger/email-orders/{emailOrderId}/reject", h.RejectLedgerEmailOrder)
	r.Handle("GET /api/v1/ledger/imports", h.ListLedgerImports)
	r.Handle("GET /api/v1/ledger/transactions", h.ListLedgerTransactionsReviewQueue)
	r.Handle("GET /api/v1/ledger/transactions/{transactionId}", h.GetLedgerTransaction)
	r.Handle("PUT /api/v1/ledger/transactions/{transactionId}", h.UpdateLedgerTransactionDetails)
	r.Handle("POST /api/v1/ledger/transactions/{transactionId}/references", h.AddLedgerTransactionReference)
	r.Handle("GET /api/v1/ledger/transactions/{transactionId}/transfer-candidates", h.ListLedgerTransferCandidates)
	r.Handle("POST /api/v1/ledger/transactions/{transactionId}/transfer-link", h.LinkLedgerTransfer)
	r.Handle("DELETE /api/v1/ledger/transactions/{transactionId}/transfer-link", h.UnlinkLedgerTransfer)
	r.Handle("POST /api/v1/ledger/transactions/{transactionId}/review", h.ReviewLedgerTransaction)
	r.Handle("POST /api/v1/ledger/transactions/review-batch", h.ReviewLedgerTransactionsBatch)
	r.Handle("POST /api/v1/ledger/category-suggestions/run", h.RunLedgerCategorySuggestions)
	r.Handle("GET /api/v1/ledger/category-suggestions/status", h.GetLedgerCategorySuggestionStatus)
	r.Handle("POST /api/v1/ledger/recurring/run", h.RunLedgerRecurringDetection)
	r.Handle("GET /api/v1/ledger/bank/aspsps", h.ListBankASPSPs)
	r.Handle("GET /api/v1/ledger/bank/connections", h.ListBankConnections)
	r.Handle("POST /api/v1/ledger/bank/connections/authorize", h.AuthorizeBank)
	r.Handle("DELETE /api/v1/ledger/bank/connections/{id}", h.DisconnectBank)
	r.Handle("PUT /api/v1/ledger/bank/connections/{id}/accounts/{uid}", h.MapBankAccount)
}

// DraftHooks returns the decision hooks of the ledger draft source
// (recurring payments proposed as contracts).
func (m *Module) DraftHooks() drafts.SourceHooks {
	return RecurringDraftHooks{store: m.store, enabled: m.coreStore, logger: m.logger}
}

func (m *Module) Seed(ctx context.Context, userID string) error {
	return m.handler.SeedLedgerDefaults(ctx, userID)
}

func (m *Module) StartBackground(ctx context.Context) {
	m.handler.suggester.SetBackgroundContext(ctx)
	m.handler.recurring.SetBackgroundContext(ctx)
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
	case errors.Is(err, ErrTargetModuleDisabled):
		writeModuleDisabled(w, err)
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

func (m *Module) Prefix(userID string) []byte {
	return module.Prefix(userID, ModuleID)
}

func (m *Module) Migrations() []migration.Migration { return nil }
