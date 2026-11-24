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
	"sync"
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
	// start launches the background services (backups, pipeline, module jobs)
	// on background; they end with ctx.
	start func(ctx context.Context)
	// background counts every goroutine that may still write to the
	// database: schedulers, jobs they or requests start, the welcome mail.
	background *sync.WaitGroup
}

func (s *Server) buildApp() (*app, error) {
	svc, err := s.buildModules()
	if err != nil {
		return nil, err
	}
	handler, patterns := s.buildRoutes(svc)
	return &app{handler: handler, patterns: patterns, start: s.buildBackground(svc), background: svc.background}, nil
}

// services is everything buildModules wires that the routes and the
// background start need.
type services struct {
	background       *sync.WaitGroup
	jwtSecret        []byte
	registry         *module.Registry
	coreStore        *core.Store
	coreHandler      *core.Handler
	catHandler       *categories.Handler
	ledger           *ledger.Module
	drafts           *drafts.Store
	draftHandler     *drafts.Handler
	paperless        *paperless.Service
	paperlessHandler *paperless.Handler
	pipeline         *paperless.Pipeline
	classifier       *extraction.Classifier
	patStore         *pat.Store
}

// buildModules builds the stores, the modules with their registry (and runs
// their migrations) and the cross-cutting services: core, paperless, the
// draft inbox and personal access tokens.
func (s *Server) buildModules() (*services, error) {
	engine := s.engine
	svc := &services{background: &sync.WaitGroup{}, jwtSecret: []byte(s.cfg.JWTSecret)}
	emailClient := email.NewClient(s.cfg)

	links := link.NewRegistry()
	catStore := categories.NewStore(engine)
	svc.catHandler = categories.NewHandler(catStore, s.logger)
	svc.coreStore = core.NewStore(engine)

	contractsMod := contracts.New(engine, links, catStore, svc.coreStore, emailClient, s.logger)
	purchasesMod := purchases.New(engine, links, catStore, s.logger)
	autoMod := auto.New(engine, links, s.logger)
	extractor := s.newExtractor()
	rates := s.newRates()
	bankClient, err := s.newBankClient()
	if err != nil {
		return nil, err
	}
	svc.drafts = drafts.NewStore(engine)
	targetModules := draftTargetModules(contractsMod, purchasesMod, autoMod)
	svc.classifier = extraction.New(extractor, extractionDeps(svc.coreStore, catStore, autoMod.Store(), targetModules), rates, s.logger)
	svc.ledger = ledger.New(engine, links, svc.coreStore, ledger.Config{
		EmailScanInterval:          s.cfg.LedgerEmailScanInterval,
		EmailMaxExtractionFailures: s.cfg.LedgerEmailMaxExtractionFailures,
		EncryptionKey:              s.cfg.EncryptionKey,
		Extractor:                  extractor,
		Rates:                      rates,
		Suggest: ledger.SuggestConfig{
			BatchSize: s.cfg.LedgerSuggestBatchSize,
			MaxPerRun: s.cfg.LedgerSuggestMaxPerRun,
		},
		Drafts:     svc.drafts,
		Classifier: svc.classifier,
		Bank: ledger.BankConfig{
			Client: bankClient, PublicURL: s.cfg.PublicURL,
			SyncInterval: s.cfg.EnableBankingSyncInterval, Mailer: emailClient,
		},
	}, s.logger)
	svc.registry = module.NewRegistry(contractsMod, purchasesMod, autoMod, svc.ledger)

	for _, m := range svc.registry.All() {
		if err := migration.RunModule(engine.DB(), s.logger, m.ID(), m.Migrations()); err != nil {
			return nil, fmt.Errorf("running %s migrations: %w", m.ID(), err)
		}
	}

	if s.cfg.BackupDir == "" {
		s.logger.Info("backup scheduler disabled", "reason", "BACKUP_DIR is not set")
	}

	seeds := make([]core.SeedFunc, 0, len(svc.registry.All()))
	for _, m := range svc.registry.All() {
		seeds = append(seeds, core.EnabledOnlySeed(svc.coreStore, m.ID(), m.Seed))
	}
	svc.coreHandler = core.NewHandler(svc.coreStore, s.logger, svc.jwtSecret, emailClient, seeds, svc.registry)

	svc.coreHandler.SetRegistration(s.cfg.Registration)
	svc.coreHandler.SetBackground(svc.background)
	svc.coreHandler.SetCapabilities(core.Capabilities{LLM: extractor.Enabled(), LLMVision: s.cfg.LLMVisionEnabled(), BankImport: svc.ledger.BankImportEnabled()})

	paperlessStore := paperless.NewStore(engine)
	svc.paperless = paperless.NewService(paperlessStore, s.logger, s.cfg.EncryptionKey)
	svc.paperlessHandler = paperless.NewHandler(svc.paperless, s.logger)
	svc.coreHandler.SetPaperlessLinks(paperlessStore)

	svc.draftHandler = drafts.NewHandler(svc.drafts, s.logger)
	svc.draftHandler.RegisterSource(drafts.SourcePaperless, paperless.NewDraftHooks(svc.paperless))
	svc.draftHandler.RegisterSource(drafts.SourceLedger, svc.ledger.DraftHooks())
	svc.draftHandler.RegisterSource(drafts.SourceEmail, svc.ledger.EmailDraftHooks())
	svc.draftHandler.SetEnabled(svc.coreStore.ModuleEnabled)
	for _, m := range svc.registry.All() {
		if p, ok := m.(draftCreatorProvider); ok {
			for target, create := range p.DraftCreators() {
				svc.draftHandler.RegisterTarget(target, m.ID(), create)
			}
		}
	}
	svc.pipeline = paperless.NewPipeline(svc.paperless, svc.drafts, svc.classifier,
		userIDs(svc.coreStore), s.cfg.PaperlessPipelineInterval, s.logger)
	svc.patStore = pat.NewStore(engine, s.logger)
	return svc, nil
}

// buildRoutes builds the protected API (JWT or PAT, scope guarded), the MCP
// endpoint, the public routes and the middleware chain. It returns the
// handler and the API patterns.
func (s *Server) buildRoutes(svc *services) (http.Handler, []string) {
	coreHandler := svc.coreHandler

	// Protected API routes (require auth)
	apiMux := newAPIMux()

	// Module routes, each gated on the user's enabled modules
	for _, m := range svc.registry.All() {
		m.RegisterRoutes(module.NewRouter(apiMux, module.Gate(m.ID(), svc.coreStore)))
	}

	// Module-scoped category routes, gated via the {module} path parameter
	catGate := module.GateParam(svc.coreStore, contracts.ModuleID, purchases.ModuleID)
	apiMux.HandleFunc("GET /api/v1/modules/{module}/categories", catGate(svc.catHandler.List))
	apiMux.HandleFunc("POST /api/v1/modules/{module}/categories", catGate(svc.catHandler.Create))
	apiMux.HandleFunc("GET /api/v1/modules/{module}/categories/{id}", catGate(svc.catHandler.Get))
	apiMux.HandleFunc("PUT /api/v1/modules/{module}/categories/{id}", catGate(svc.catHandler.Update))
	apiMux.HandleFunc("DELETE /api/v1/modules/{module}/categories/{id}", catGate(svc.catHandler.Delete))

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
	patHandler := pat.NewHandler(svc.patStore, s.logger)
	apiMux.HandleFunc("GET /api/v1/tokens", patHandler.List)
	apiMux.HandleFunc("POST /api/v1/tokens", patHandler.Create)
	apiMux.HandleFunc("DELETE /api/v1/tokens/{id}", patHandler.Revoke)

	// Paperless-ngx integration (per-user, hidden until configured), the
	// draft inbox it feeds and free-text quick add
	registerPaperlessRoutes(apiMux, svc.paperlessHandler, svc.pipeline)
	registerDraftRoutes(apiMux, svc.draftHandler)
	apiMux.HandleFunc("POST /api/v1/quick-add", quickadd.NewHandler(svc.classifier, s.cfg.LLMVisionEnabled(), s.logger).Classify)

	mcpHandler := s.newMCPHandler(mcpDeps{
		registry: svc.registry, coreStore: svc.coreStore, drafts: svc.drafts, paperless: svc.paperless,
		patStore: svc.patStore, jwtSecret: svc.jwtSecret,
	})

	protectedAPI := middleware.Auth(svc.jwtSecret,
		middleware.WithPATs(svc.patStore),
		middleware.WithTokenVersions(svc.coreStore),
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
	mux.HandleFunc("GET /api/v1/auth/config", coreHandler.AuthConfig)
	mux.HandleFunc("GET /api/version", version.Handler)
	// Redirect target of the bank consent; authenticated by its state only.
	mux.HandleFunc("GET /api/v1/ledger/bank/callback", svc.ledger.BankCallback)

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
	return chain, apiMux.patterns
}

// buildBackground returns the start of the background services: backups,
// the paperless pipeline and the module jobs, all on svc.background.
func (s *Server) buildBackground(svc *services) func(ctx context.Context) {
	return func(ctx context.Context) {
		if s.cfg.BackupDir != "" {
			s.engine.StartBackups(ctx, svc.background, storage.BackupConfig{
				Dir:      s.cfg.BackupDir,
				Interval: s.cfg.BackupInterval,
				Keep:     s.cfg.BackupKeep,
			})
		}
		svc.pipeline.Start(ctx, svc.background)
		for _, m := range svc.registry.All() {
			m.StartBackground(ctx, svc.background)
		}
	}
}

// Shutdown budget: requests first, then the background work. Together they
// stay below Kubernetes' default grace period of 30s.
const (
	httpShutdownTimeout       = 10 * time.Second
	backgroundShutdownTimeout = 15 * time.Second
)

// Run serves until SIGINT/SIGTERM or a serve error. Either way it stops
// the background work and waits for it, so the caller can close the
// database afterwards.
func (s *Server) Run() error {
	a, err := s.buildApp()
	if err != nil {
		return err
	}
	srv := newHTTPServer(s.cfg.Port, a.handler)
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(quit)
	return s.serve(a, srv, quit)
}

func (s *Server) serve(a *app, srv *http.Server, quit <-chan os.Signal) error {
	bgCtx, stopBackground := context.WithCancel(context.Background())
	defer stopBackground()
	a.start(bgCtx)

	errCh := make(chan error, 1)
	go func() {
		v := version.Get()
		s.logger.Info("server starting", "port", s.cfg.Port, "environment", s.cfg.Environment, "version", v.Version, "commit", v.Commit)
		errCh <- srv.ListenAndServe()
	}()

	var runErr error
	select {
	case err := <-errCh:
		runErr = err
	case sig := <-quit:
		s.logger.Info("shutting down", "signal", sig)
		ctx, cancel := context.WithTimeout(context.Background(), httpShutdownTimeout)
		runErr = srv.Shutdown(ctx)
		cancel()
	}

	stopBackground()
	if !waitTimeout(a.background, backgroundShutdownTimeout) {
		s.logger.Warn("background work still running at shutdown", "timeout", backgroundShutdownTimeout)
	}
	return runErr
}

// waitTimeout waits for wg and reports whether it finished in time.
func waitTimeout(wg *sync.WaitGroup, timeout time.Duration) bool {
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		return true
	case <-time.After(timeout):
		return false
	}
}

// No WriteTimeout: the manual bank sync answers after up to 15 minutes and
// /mcp may stream. ReadTimeout leaves room for the 64 MiB uploads.
const (
	readHeaderTimeout = 10 * time.Second
	readTimeout       = 5 * time.Minute
	idleTimeout       = 2 * time.Minute
)

func newHTTPServer(port int, handler http.Handler) *http.Server {
	return &http.Server{
		Addr:              fmt.Sprintf(":%d", port),
		Handler:           handler,
		ReadHeaderTimeout: readHeaderTimeout,
		ReadTimeout:       readTimeout,
		IdleTimeout:       idleTimeout,
	}
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
