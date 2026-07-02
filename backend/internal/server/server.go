package server

import (
	"context"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/reusing-code/kontor/backend/internal/config"
	"github.com/reusing-code/kontor/backend/internal/cryptoutil"
	"github.com/reusing-code/kontor/backend/internal/email"
	"github.com/reusing-code/kontor/backend/internal/handler"
	"github.com/reusing-code/kontor/backend/internal/ledgeremail"
	"github.com/reusing-code/kontor/backend/internal/middleware"
	"github.com/reusing-code/kontor/backend/internal/reminder"
	"github.com/reusing-code/kontor/backend/internal/store"
	"github.com/reusing-code/kontor/backend/internal/version"
)

type Server struct {
	cfg    config.Config
	logger *slog.Logger
	store  store.Store
}

func New(cfg config.Config, logger *slog.Logger, s store.Store) *Server {
	return &Server{cfg: cfg, logger: logger, store: s}
}

func (s *Server) Run() error {
	jwtSecret := []byte(s.cfg.JWTSecret)
	emailClient := email.NewClient(s.cfg)

	shutdownCtx, shutdownCancel := context.WithCancel(context.Background())
	defer shutdownCancel()

	if emailClient.IsConfigured() {
		sched := reminder.New(s.store, emailClient, s.logger)
		sched.Start(shutdownCtx)
	} else {
		s.logger.Info("SMTP not configured, reminder scheduler disabled")
	}

	if s.cfg.BackupDir == "" {
		s.logger.Info("backup scheduler disabled", "reason", "BACKUP_DIR is not set")
	} else if bs, ok := s.store.(*store.BadgerStore); !ok {
		s.logger.Info("backup scheduler disabled", "reason", "store does not support backups")
	} else {
		bs.StartBackups(shutdownCtx, store.BackupConfig{
			Dir:      s.cfg.BackupDir,
			Interval: s.cfg.BackupInterval,
			Keep:     s.cfg.BackupKeep,
		})
	}

	if s.cfg.LedgerEmailScanInterval <= 0 {
		s.logger.Info("ledger email scan scheduler disabled", "reason", "LEDGER_EMAIL_SCAN_INTERVAL is 0")
	} else if encryptionKey, err := cryptoutil.NormalizeEncryptionKey(s.cfg.EmailEncryptionKey); err != nil {
		s.logger.Info("ledger email scan scheduler disabled", "reason", err.Error())
	} else {
		sched := ledgeremail.NewScheduler(s.store, ledgeremail.NewService(s.store, s.logger), encryptionKey, s.cfg.LedgerEmailScanInterval, s.logger)
		sched.Start(shutdownCtx)
	}

	h := handler.New(s.store, s.logger, jwtSecret, emailClient, s.cfg.EmailEncryptionKey)

	// Protected API routes (require auth)
	apiMux := http.NewServeMux()

	// Module-scoped category routes
	apiMux.HandleFunc("GET /api/v1/modules/{module}/categories", h.ListCategories)
	apiMux.HandleFunc("POST /api/v1/modules/{module}/categories", h.CreateCategory)
	apiMux.HandleFunc("GET /api/v1/modules/{module}/categories/{id}", h.GetCategory)
	apiMux.HandleFunc("PUT /api/v1/modules/{module}/categories/{id}", h.UpdateCategory)
	apiMux.HandleFunc("DELETE /api/v1/modules/{module}/categories/{id}", h.DeleteCategory)

	// Contract routes
	apiMux.HandleFunc("GET /api/v1/categories/{id}/contracts", h.ListContractsByCategory)
	apiMux.HandleFunc("POST /api/v1/categories/{id}/contracts", h.CreateContractInCategory)
	apiMux.HandleFunc("POST /api/v1/contracts/import", h.ImportContracts)
	apiMux.HandleFunc("GET /api/v1/contracts/upcoming-renewals", h.UpcomingRenewals)
	apiMux.HandleFunc("GET /api/v1/contracts", h.ListContracts)
	apiMux.HandleFunc("GET /api/v1/contracts/{id}", h.GetContract)
	apiMux.HandleFunc("PUT /api/v1/contracts/{id}", h.UpdateContract)
	apiMux.HandleFunc("DELETE /api/v1/contracts/{id}", h.DeleteContract)
	apiMux.HandleFunc("GET /api/v1/summary", h.Summary)

	// Purchase routes
	apiMux.HandleFunc("GET /api/v1/categories/{id}/purchases", h.ListPurchasesByCategory)
	apiMux.HandleFunc("POST /api/v1/categories/{id}/purchases", h.CreatePurchaseInCategory)
	apiMux.HandleFunc("GET /api/v1/purchases/summary", h.PurchaseSummary)
	apiMux.HandleFunc("GET /api/v1/purchases", h.ListPurchases)
	apiMux.HandleFunc("GET /api/v1/purchases/{id}", h.GetPurchase)
	apiMux.HandleFunc("PUT /api/v1/purchases/{id}", h.UpdatePurchase)
	apiMux.HandleFunc("DELETE /api/v1/purchases/{id}", h.DeletePurchase)

	// Vehicle routes
	apiMux.HandleFunc("GET /api/v1/vehicles", h.ListVehicles)
	apiMux.HandleFunc("POST /api/v1/vehicles", h.CreateVehicle)
	apiMux.HandleFunc("GET /api/v1/vehicles/{id}", h.GetVehicle)
	apiMux.HandleFunc("PUT /api/v1/vehicles/{id}", h.UpdateVehicle)
	apiMux.HandleFunc("DELETE /api/v1/vehicles/{id}", h.DeleteVehicle)
	apiMux.HandleFunc("GET /api/v1/vehicles/{id}/summary", h.VehicleSummary)
	apiMux.HandleFunc("GET /api/v1/vehicles/{id}/costs", h.ListCostEntries)
	apiMux.HandleFunc("POST /api/v1/vehicles/{id}/costs", h.CreateCostEntry)
	apiMux.HandleFunc("GET /api/v1/costs/{id}", h.GetCostEntry)
	apiMux.HandleFunc("PUT /api/v1/costs/{id}", h.UpdateCostEntry)
	apiMux.HandleFunc("DELETE /api/v1/costs/{id}", h.DeleteCostEntry)

	// Data export / restore
	apiMux.HandleFunc("GET /api/v1/export", h.Export)
	apiMux.HandleFunc("POST /api/v1/restore", h.Restore)

	// Settings routes
	apiMux.HandleFunc("GET /api/v1/settings", h.GetSettings)
	apiMux.HandleFunc("PUT /api/v1/settings", h.UpdateSettings)
	apiMux.HandleFunc("PUT /api/v1/settings/password", h.ChangePassword)

	// Ledger routes
	apiMux.HandleFunc("POST /api/v1/ledger/imports/preview", h.LedgerImportPreview)
	apiMux.HandleFunc("POST /api/v1/ledger/imports/{previewId}/commit", h.LedgerImportCommit)
	apiMux.HandleFunc("GET /api/v1/ledger/categories", h.ListLedgerCategories)
	apiMux.HandleFunc("POST /api/v1/ledger/categories", h.CreateLedgerCategory)
	apiMux.HandleFunc("GET /api/v1/ledger/categories/{id}", h.GetLedgerCategory)
	apiMux.HandleFunc("PUT /api/v1/ledger/categories/{id}", h.UpdateLedgerCategory)
	apiMux.HandleFunc("DELETE /api/v1/ledger/categories/{id}", h.DeleteLedgerCategory)
	apiMux.HandleFunc("GET /api/v1/ledger/accounts", h.ListLedgerAccounts)
	apiMux.HandleFunc("GET /api/v1/ledger/accounts/{accountId}", h.GetLedgerAccount)
	apiMux.HandleFunc("GET /api/v1/ledger/accounts/{accountId}/transactions", h.ListLedgerTransactions)
	apiMux.HandleFunc("GET /api/v1/ledger/email-accounts", h.ListLedgerEmailAccounts)
	apiMux.HandleFunc("POST /api/v1/ledger/email-accounts", h.CreateLedgerEmailAccount)
	apiMux.HandleFunc("GET /api/v1/ledger/email-accounts/{emailAccountId}", h.GetLedgerEmailAccount)
	apiMux.HandleFunc("PUT /api/v1/ledger/email-accounts/{emailAccountId}", h.UpdateLedgerEmailAccount)
	apiMux.HandleFunc("DELETE /api/v1/ledger/email-accounts/{emailAccountId}", h.DeleteLedgerEmailAccount)
	apiMux.HandleFunc("POST /api/v1/ledger/email-accounts/{emailAccountId}/test", h.TestLedgerEmailAccount)
	apiMux.HandleFunc("POST /api/v1/ledger/email-accounts/{emailAccountId}/scan", h.ScanLedgerEmailAccount)
	apiMux.HandleFunc("GET /api/v1/ledger/email-orders", h.ListLedgerEmailOrders)
	apiMux.HandleFunc("GET /api/v1/ledger/email-orders/{emailOrderId}", h.GetLedgerEmailOrder)
	apiMux.HandleFunc("POST /api/v1/ledger/email-orders/{emailOrderId}/link", h.LinkLedgerEmailOrder)
	apiMux.HandleFunc("POST /api/v1/ledger/email-orders/{emailOrderId}/reject", h.RejectLedgerEmailOrder)
	apiMux.HandleFunc("GET /api/v1/ledger/email-importers", h.ListLedgerEmailImporters)
	apiMux.HandleFunc("GET /api/v1/ledger/imports", h.ListLedgerImports)
	apiMux.HandleFunc("GET /api/v1/ledger/transactions", h.ListLedgerTransactionsReviewQueue)
	apiMux.HandleFunc("GET /api/v1/ledger/transactions/{transactionId}", h.GetLedgerTransaction)
	apiMux.HandleFunc("PUT /api/v1/ledger/transactions/{transactionId}", h.UpdateLedgerTransactionDetails)
	apiMux.HandleFunc("GET /api/v1/ledger/transactions/{transactionId}/transfer-candidates", h.ListLedgerTransferCandidates)
	apiMux.HandleFunc("POST /api/v1/ledger/transactions/{transactionId}/transfer-link", h.LinkLedgerTransfer)
	apiMux.HandleFunc("DELETE /api/v1/ledger/transactions/{transactionId}/transfer-link", h.UnlinkLedgerTransfer)
	apiMux.HandleFunc("POST /api/v1/ledger/transactions/{transactionId}/review", h.ReviewLedgerTransaction)

	protectedAPI := middleware.Auth(jwtSecret)(apiMux)

	mux := http.NewServeMux()

	// Public routes
	mux.HandleFunc("GET /healthz", h.Healthz)
	mux.HandleFunc("GET /readyz", h.Readyz)
	mux.Handle("GET /metrics", promhttp.Handler())
	authRateLimit := middleware.RateLimitPerIP(s.cfg.AuthRateLimit, s.cfg.AuthRateWindow, s.cfg.TrustProxy)
	if s.cfg.AuthRateLimit <= 0 {
		s.logger.Info("auth rate limiting disabled", "reason", "AUTH_RATE_LIMIT is 0")
	}
	mux.Handle("POST /api/v1/auth/register", authRateLimit(http.HandlerFunc(h.Register)))
	mux.Handle("POST /api/v1/auth/login", authRateLimit(http.HandlerFunc(h.Login)))
	mux.HandleFunc("GET /api/version", version.Handler)

	// Mount protected API routes
	mux.Handle("/api/v1/", protectedAPI)

	// SPA static files
	if s.cfg.StaticDir != "" {
		mux.Handle("/", spaHandler(s.cfg.StaticDir))
	}

	chain := middleware.Chain(mux,
		middleware.RequestID,
		middleware.Recovery(s.logger),
		middleware.Metrics,
		middleware.Logging(s.logger),
		middleware.CORS(s.cfg.CORSOrigin),
	)

	srv := &http.Server{
		Addr:    fmt.Sprintf(":%d", s.cfg.Port),
		Handler: chain,
	}

	errCh := make(chan error, 1)
	go func() {
		v := version.Get()
		s.logger.Info("server starting", "port", s.cfg.Port, "environment", s.cfg.Environment, "version", v.Version, "commit", v.Commit)
		errCh <- srv.ListenAndServe()
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	select {
	case err := <-errCh:
		return err
	case sig := <-quit:
		s.logger.Info("shutting down", "signal", sig)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return srv.Shutdown(ctx)
}

func spaHandler(dir string) http.Handler {
	fsys := os.DirFS(dir)
	fileServer := http.FileServer(http.FS(fsys))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/")
		if path == "" {
			path = "index.html"
		}

		// Try to serve the file directly
		if _, err := fs.Stat(fsys, path); err == nil {
			fileServer.ServeHTTP(w, r)
			return
		}

		// Fall back to index.html for SPA routing
		r.URL.Path = "/"
		fileServer.ServeHTTP(w, r)
	})
}
