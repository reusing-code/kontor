package server

import (
	"context"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/reusing-code/kontor/backend/internal/categories"
	"github.com/reusing-code/kontor/backend/internal/config"
	"github.com/reusing-code/kontor/backend/internal/core"
	"github.com/reusing-code/kontor/backend/internal/drafts"
	"github.com/reusing-code/kontor/backend/internal/email"
	"github.com/reusing-code/kontor/backend/internal/enablebanking"
	"github.com/reusing-code/kontor/backend/internal/extraction"
	"github.com/reusing-code/kontor/backend/internal/fx"
	"github.com/reusing-code/kontor/backend/internal/llm"
	"github.com/reusing-code/kontor/backend/internal/middleware"
	"github.com/reusing-code/kontor/backend/internal/module"
	"github.com/reusing-code/kontor/backend/internal/modules/auto"
	"github.com/reusing-code/kontor/backend/internal/modules/contracts"
	"github.com/reusing-code/kontor/backend/internal/modules/ledger"
	"github.com/reusing-code/kontor/backend/internal/modules/purchases"
	"github.com/reusing-code/kontor/backend/internal/paperless"
	"github.com/reusing-code/kontor/backend/internal/pat"
	"github.com/reusing-code/kontor/backend/internal/quickadd"
	"github.com/reusing-code/kontor/backend/internal/storage"
	"github.com/reusing-code/kontor/backend/internal/storage/link"
	"github.com/reusing-code/kontor/backend/internal/storage/migration"
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

// app is the fully wired server: the handler plus the pieces tests need.
type app struct {
	handler  http.Handler
	patterns []string
	// start launches the background services (backups, pipeline, module jobs).
	start func(ctx context.Context)
}

func (s *Server) buildApp() (*app, error) {
	jwtSecret := []byte(s.cfg.JWTSecret)
	emailClient := email.NewClient(s.cfg)

	engine := s.engine

	links := link.NewRegistry()
	catStore := categories.NewStore(engine)
	catHandler := categories.NewHandler(catStore, s.logger)
	coreStore := core.NewStore(engine)

	contractsMod := contracts.New(engine, links, catStore, coreStore, emailClient, s.logger)
	purchasesMod := purchases.New(engine, links, catStore, s.logger)
	autoMod := auto.New(engine, links, s.logger)
	extractor := s.newExtractor()
	rates := s.newRates()
	bankClient, err := s.newBankClient()
	if err != nil {
		return nil, err
	}
	draftStore := drafts.NewStore(engine)
	ledgerMod := ledger.New(engine, links, coreStore, ledger.Config{
		EmailScanInterval:          s.cfg.LedgerEmailScanInterval,
		EmailMaxExtractionFailures: s.cfg.LedgerEmailMaxExtractionFailures,
		EncryptionKey:              s.cfg.EncryptionKey,
		Extractor:                  extractor,
		Rates:                      rates,
		Suggest: ledger.SuggestConfig{
			BatchSize: s.cfg.LedgerSuggestBatchSize,
			MaxPerRun: s.cfg.LedgerSuggestMaxPerRun,
		},
		Drafts: draftStore,
		Bank: ledger.BankConfig{
			Client: bankClient, PublicURL: s.cfg.PublicURL,
			SyncInterval: s.cfg.EnableBankingSyncInterval, Mailer: emailClient,
		},
	}, s.logger)
	registry := module.NewRegistry(contractsMod, purchasesMod, autoMod, ledgerMod)

	for _, m := range registry.All() {
		if err := migration.RunModule(engine.DB(), s.logger, m.ID(), m.Migrations()); err != nil {
			return nil, fmt.Errorf("running %s migrations: %w", m.ID(), err)
		}
	}

	if s.cfg.BackupDir == "" {
		s.logger.Info("backup scheduler disabled", "reason", "BACKUP_DIR is not set")
	}

	seeds := make([]core.SeedFunc, 0, len(registry.All()))
	for _, m := range registry.All() {
		seeds = append(seeds, core.EnabledOnlySeed(coreStore, m.ID(), m.Seed))
	}
	coreHandler := core.NewHandler(coreStore, s.logger, jwtSecret, emailClient, seeds, registry)

	coreHandler.SetCapabilities(core.Capabilities{LLM: extractor.Enabled(), LLMVision: s.cfg.LLMVisionEnabled(), BankImport: ledgerMod.BankImportEnabled()})
	classifier := extraction.New(extractor, extractionDeps(coreStore, catStore, autoMod.Store()), rates, s.logger)

	paperlessStore := paperless.NewStore(engine)
	paperlessService := paperless.NewService(paperlessStore, s.logger, s.cfg.EncryptionKey)
	paperlessHandler := paperless.NewHandler(paperlessService, s.logger)
	coreHandler.SetPaperlessLinks(paperlessStore)

	draftHandler := drafts.NewHandler(draftStore, s.logger)
	draftHandler.RegisterSource(drafts.SourcePaperless, paperless.NewDraftHooks(paperlessService))
	draftHandler.RegisterSource(drafts.SourceLedger, ledgerMod.DraftHooks())
	pipeline := paperless.NewPipeline(paperlessService, draftStore, classifier,
		userIDs(coreStore), s.cfg.PaperlessPipelineInterval, s.logger)

	// Protected API routes (require auth)
	apiMux := newAPIMux()

	// Module routes, each gated on the user's enabled modules
	for _, m := range registry.All() {
		m.RegisterRoutes(module.NewRouter(apiMux, module.Gate(m.ID(), coreStore)))
	}

	// Module-scoped category routes, gated via the {module} path parameter
	catGate := module.GateParam(coreStore, contracts.ModuleID, purchases.ModuleID)
	apiMux.HandleFunc("GET /api/v1/modules/{module}/categories", catGate(catHandler.List))
	apiMux.HandleFunc("POST /api/v1/modules/{module}/categories", catGate(catHandler.Create))
	apiMux.HandleFunc("GET /api/v1/modules/{module}/categories/{id}", catGate(catHandler.Get))
	apiMux.HandleFunc("PUT /api/v1/modules/{module}/categories/{id}", catGate(catHandler.Update))
	apiMux.HandleFunc("DELETE /api/v1/modules/{module}/categories/{id}", catGate(catHandler.Delete))

	// Module directory and server-wide capabilities
	apiMux.HandleFunc("GET /api/v1/modules", coreHandler.ListModules)
	apiMux.HandleFunc("GET /api/v1/capabilities", coreHandler.GetCapabilities)

	// Data export / import
	apiMux.HandleFunc("GET /api/v1/export", coreHandler.Export)
	apiMux.HandleFunc("POST /api/v1/import", coreHandler.Import)
	apiMux.HandleFunc("GET /api/v1/modules/{module}/export", coreHandler.ExportModule)
	apiMux.HandleFunc("POST /api/v1/modules/{module}/import", coreHandler.ImportModule)

	// Settings routes
	apiMux.HandleFunc("GET /api/v1/settings", coreHandler.GetSettings)
	apiMux.HandleFunc("PUT /api/v1/settings", coreHandler.UpdateSettings)
	apiMux.HandleFunc("PUT /api/v1/settings/password", coreHandler.ChangePassword)

	// Personal access token management
	patStore := pat.NewStore(engine, s.logger)
	patHandler := pat.NewHandler(patStore, s.logger)
	apiMux.HandleFunc("GET /api/v1/tokens", patHandler.List)
	apiMux.HandleFunc("POST /api/v1/tokens", patHandler.Create)
	apiMux.HandleFunc("DELETE /api/v1/tokens/{id}", patHandler.Revoke)

	// Paperless-ngx integration (per-user, hidden until configured), the
	// draft inbox it feeds and free-text quick add
	registerPaperlessRoutes(apiMux, paperlessHandler, pipeline)
	registerDraftRoutes(apiMux, draftHandler)
	apiMux.HandleFunc("POST /api/v1/quick-add", quickadd.NewHandler(classifier, s.cfg.LLMVisionEnabled(), s.logger).Classify)

	mcpHandler := s.newMCPHandler(mcpDeps{
		registry: registry, coreStore: coreStore, drafts: draftStore, paperless: paperlessService,
		contracts: contractsMod, purchases: purchasesMod, auto: autoMod, ledger: ledgerMod,
		patStore: patStore, jwtSecret: jwtSecret,
	})

	protectedAPI := middleware.Auth(jwtSecret,
		middleware.WithPATs(patStore),
		middleware.WithAuthLogger(s.logger),
	)(middleware.ScopeGuard(apiMux.ServeMux, routeRules))

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
	// Redirect target of the bank consent; authenticated by its state only.
	mux.HandleFunc("GET /api/v1/ledger/bank/callback", ledgerMod.BankCallback)

	// Mount protected API routes
	mux.Handle("/api/v1/", protectedAPI)
	mux.Handle("/mcp", mcpHandler)

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

	start := func(ctx context.Context) {
		if s.cfg.BackupDir != "" {
			engine.StartBackups(ctx, storage.BackupConfig{
				Dir:      s.cfg.BackupDir,
				Interval: s.cfg.BackupInterval,
				Keep:     s.cfg.BackupKeep,
			})
		}
		pipeline.Start(ctx)
		for _, m := range registry.All() {
			m.StartBackground(ctx)
		}
	}

	return &app{handler: chain, patterns: apiMux.patterns, start: start}, nil
}

func (s *Server) Run() error {
	shutdownCtx, shutdownCancel := context.WithCancel(context.Background())
	defer shutdownCancel()

	a, err := s.buildApp()
	if err != nil {
		return err
	}
	a.start(shutdownCtx)

	srv := &http.Server{
		Addr:    fmt.Sprintf(":%d", s.cfg.Port),
		Handler: a.handler,
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

func (s *Server) newExtractor() llm.Extractor {
	if !s.cfg.LLMEnabled() {
		s.logger.Info("llm disabled", "reason", "LLM_BASE_URL is not set")
		return llm.Disabled{}
	}
	baseURL := s.cfg.LLMBaseURL
	if u, err := url.Parse(baseURL); err == nil {
		baseURL = u.Redacted()
	}
	s.logger.Info("llm enabled", "base_url", baseURL, "model", s.cfg.LLMModel, "vision_model", s.cfg.LLMVisionModel, "timeout", s.cfg.LLMTimeout)
	return llm.New(llm.Config{
		BaseURL:     s.cfg.LLMBaseURL,
		Model:       s.cfg.LLMModel,
		VisionModel: s.cfg.LLMVisionModel,
		APIKey:      s.cfg.LLMAPIKey,
		Timeout:     s.cfg.LLMTimeout,
	})
}

// newBankClient builds the Enable Banking client, or returns nil while the
// feature is not configured.
func (s *Server) newBankClient() (*enablebanking.Client, error) {
	if !s.cfg.EnableBankingEnabled() {
		s.logger.Info("bank import disabled", "reason", "ENABLE_BANKING_APP_ID or ENABLE_BANKING_KEY_PATH is not set")
		return nil, nil
	}
	key, err := enablebanking.LoadPrivateKey(s.cfg.EnableBankingKeyPath)
	if err != nil {
		return nil, fmt.Errorf("loading Enable Banking key: %w", err)
	}
	client, err := enablebanking.New(enablebanking.Config{
		BaseURL: s.cfg.EnableBankingAPIURL,
		AppID:   s.cfg.EnableBankingAppID,
		Key:     key,
	})
	if err != nil {
		return nil, fmt.Errorf("creating Enable Banking client: %w", err)
	}
	host := s.cfg.EnableBankingAPIURL
	if u, err := url.Parse(host); err == nil {
		host = u.Host
	}
	s.logger.Info("bank import enabled", "api_host", host)
	if s.cfg.PublicURL == "" {
		s.logger.Warn("bank import cannot connect banks", "reason", "PUBLIC_URL is not set")
	}
	return client, nil
}

func (s *Server) newRates() fx.Rates {
	if !s.cfg.FXRatesEnabled() {
		s.logger.Info("currency conversion disabled", "reason", "FX_RATES_URL is off")
		return fx.Disabled{}
	}
	s.logger.Info("currency conversion enabled", "rates_url", s.cfg.FXRatesURL)
	return fx.NewFrankfurter(s.cfg.FXRatesURL)
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
