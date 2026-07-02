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
	"github.com/reusing-code/kontor/backend/internal/categories"
	"github.com/reusing-code/kontor/backend/internal/config"
	"github.com/reusing-code/kontor/backend/internal/core"
	"github.com/reusing-code/kontor/backend/internal/email"
	"github.com/reusing-code/kontor/backend/internal/middleware"
	"github.com/reusing-code/kontor/backend/internal/module"
	"github.com/reusing-code/kontor/backend/internal/modules/auto"
	"github.com/reusing-code/kontor/backend/internal/modules/contracts"
	"github.com/reusing-code/kontor/backend/internal/modules/ledger"
	"github.com/reusing-code/kontor/backend/internal/modules/purchases"
	"github.com/reusing-code/kontor/backend/internal/storage"
	"github.com/reusing-code/kontor/backend/internal/storage/link"
	"github.com/reusing-code/kontor/backend/internal/version"
)

type Server struct {
	cfg    config.Config
	logger *slog.Logger
	engine *storage.Engine
}

func New(cfg config.Config, logger *slog.Logger, engine *storage.Engine) *Server {
	return &Server{cfg: cfg, logger: logger, engine: engine}
}

func (s *Server) Run() error {
	jwtSecret := []byte(s.cfg.JWTSecret)
	emailClient := email.NewClient(s.cfg)

	shutdownCtx, shutdownCancel := context.WithCancel(context.Background())
	defer shutdownCancel()

	engine := s.engine

	links := link.NewRegistry()
	catStore := categories.NewStore(engine)
	catHandler := categories.NewHandler(catStore, s.logger)
	coreStore := core.NewStore(engine)

	contractsMod := contracts.New(engine, links, catStore, coreStore, emailClient, s.logger)
	purchasesMod := purchases.New(engine, links, catStore, s.logger)
	autoMod := auto.New(engine, links, s.logger)
	ledgerMod := ledger.New(engine, links, coreStore, ledger.Config{
		EmailScanInterval:  s.cfg.LedgerEmailScanInterval,
		EmailEncryptionKey: s.cfg.EmailEncryptionKey,
	}, s.logger)
	registry := module.NewRegistry(contractsMod, purchasesMod, autoMod, ledgerMod)

	if s.cfg.BackupDir == "" {
		s.logger.Info("backup scheduler disabled", "reason", "BACKUP_DIR is not set")
	} else {
		engine.StartBackups(shutdownCtx, storage.BackupConfig{
			Dir:      s.cfg.BackupDir,
			Interval: s.cfg.BackupInterval,
			Keep:     s.cfg.BackupKeep,
		})
	}

	seeds := make([]core.SeedFunc, 0, len(registry.All()))
	for _, m := range registry.All() {
		seeds = append(seeds, m.Seed)
	}
	coreHandler := core.NewHandler(coreStore, s.logger, jwtSecret, emailClient, seeds, registry)

	// Protected API routes (require auth)
	apiMux := http.NewServeMux()

	// Module routes (gate is a passthrough until enablement lands)
	for _, m := range registry.All() {
		m.RegisterRoutes(module.NewRouter(apiMux, nil))
		m.StartBackground(shutdownCtx)
	}

	// Module-scoped category routes
	apiMux.HandleFunc("GET /api/v1/modules/{module}/categories", catHandler.List)
	apiMux.HandleFunc("POST /api/v1/modules/{module}/categories", catHandler.Create)
	apiMux.HandleFunc("GET /api/v1/modules/{module}/categories/{id}", catHandler.Get)
	apiMux.HandleFunc("PUT /api/v1/modules/{module}/categories/{id}", catHandler.Update)
	apiMux.HandleFunc("DELETE /api/v1/modules/{module}/categories/{id}", catHandler.Delete)

	// Data export / import
	apiMux.HandleFunc("GET /api/v1/export", coreHandler.Export)
	apiMux.HandleFunc("POST /api/v1/import", coreHandler.Import)
	apiMux.HandleFunc("GET /api/v1/modules/{module}/export", coreHandler.ExportModule)
	apiMux.HandleFunc("POST /api/v1/modules/{module}/import", coreHandler.ImportModule)

	// Settings routes
	apiMux.HandleFunc("GET /api/v1/settings", coreHandler.GetSettings)
	apiMux.HandleFunc("PUT /api/v1/settings", coreHandler.UpdateSettings)
	apiMux.HandleFunc("PUT /api/v1/settings/password", coreHandler.ChangePassword)

	protectedAPI := middleware.Auth(jwtSecret)(apiMux)

	mux := http.NewServeMux()

	// Public routes
	mux.HandleFunc("GET /healthz", coreHandler.Healthz)
	mux.HandleFunc("GET /readyz", coreHandler.Readyz)
	mux.Handle("GET /metrics", promhttp.Handler())
	authRateLimit := middleware.RateLimitPerIP(s.cfg.AuthRateLimit, s.cfg.AuthRateWindow, s.cfg.TrustProxy)
	if s.cfg.AuthRateLimit <= 0 {
		s.logger.Info("auth rate limiting disabled", "reason", "AUTH_RATE_LIMIT is 0")
	}
	mux.Handle("POST /api/v1/auth/register", authRateLimit(http.HandlerFunc(coreHandler.Register)))
	mux.Handle("POST /api/v1/auth/login", authRateLimit(http.HandlerFunc(coreHandler.Login)))
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
