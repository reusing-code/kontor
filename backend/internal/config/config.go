package config

import (
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"strings"
	"time"

	"github.com/caarlos0/env/v11"
	"github.com/reusing-code/kontor/backend/internal/cryptoutil"
)

type Config struct {
	Port        int    `env:"PORT"        envDefault:"8080"`
	DBPath      string `env:"DB_PATH"     envDefault:"./data"`
	LogFormat   string `env:"LOG_FORMAT"  envDefault:"text"`
	LogLevel    string `env:"LOG_LEVEL"   envDefault:"info"`
	CORSOrigin  string `env:"CORS_ORIGIN"`
	StaticDir   string `env:"STATIC_DIR"`
	Environment string `env:"ENVIRONMENT" envDefault:"development"`
	JWTSecret   string `env:"JWT_SECRET,required"`

	// SMTP Configuration
	SMTPHost     string `env:"SMTP_HOST"`
	SMTPPort     int    `env:"SMTP_PORT"     envDefault:"587"`
	SMTPProtocol string `env:"SMTP_PROTOCOL" envDefault:"starttls"` // "tls", "starttls", "none"
	SMTPUser     string `env:"SMTP_USER"`
	SMTPPassword string `env:"SMTP_PASSWORD"`
	SMTPFrom     string `env:"SMTP_FROM"`

	// AES-GCM key for stored secrets (email passwords, paperless tokens, bank sessions).
	EncryptionKey string `env:"ENCRYPTION_KEY"`
	// Deprecated name of ENCRYPTION_KEY, used only when that is unset.
	LegacyEncryptionKey string `env:"EMAIL_ENCRYPTION_KEY"`

	// Interval between background IMAP scans of ledger email accounts. 0 disables.
	LedgerEmailScanInterval time.Duration `env:"LEDGER_EMAIL_SCAN_INTERVAL" envDefault:"6h"`

	// LLM category suggestions: transactions per LLM call and per run.
	LedgerSuggestBatchSize int `env:"LEDGER_SUGGEST_BATCH_SIZE"  envDefault:"25"`
	LedgerSuggestMaxPerRun int `env:"LEDGER_SUGGEST_MAX_PER_RUN" envDefault:"100"`

	// Interval between paperless pipeline passes (tagged documents to drafts). 0 disables.
	PaperlessPipelineInterval time.Duration `env:"PAPERLESS_PIPELINE_INTERVAL" envDefault:"15m"`

	// Periodic Badger snapshots are written here; empty disables backups.
	BackupDir      string        `env:"BACKUP_DIR"`
	BackupInterval time.Duration `env:"BACKUP_INTERVAL" envDefault:"24h"`
	BackupKeep     int           `env:"BACKUP_KEEP"     envDefault:"7"`

	// Per-IP request budget for /auth/login and /auth/register. 0 disables.
	AuthRateLimit  int           `env:"AUTH_RATE_LIMIT"  envDefault:"10"`
	AuthRateWindow time.Duration `env:"AUTH_RATE_WINDOW" envDefault:"1m"`
	// Take client IPs from X-Real-IP / X-Forwarded-For; only enable behind a
	// reverse proxy that sets these headers, as clients can spoof them otherwise.
	TrustProxy bool `env:"TRUST_PROXY" envDefault:"false"`

	// Server-wide OpenAI-compatible LLM endpoint; empty base URL disables LLM features.
	LLMBaseURL string        `env:"LLM_BASE_URL"`
	LLMModel   string        `env:"LLM_MODEL"`
	LLMAPIKey  string        `env:"LLM_API_KEY"`
	LLMTimeout time.Duration `env:"LLM_TIMEOUT"  envDefault:"60s"`
	// Model for requests with images, on the same endpoint; empty disables image input.
	LLMVisionModel string `env:"LLM_VISION_MODEL"`

	// Frankfurter-compatible API with ECB reference rates, used to convert
	// extracted non-EUR amounts. "off" disables conversion (an empty value
	// falls back to the default).
	FXRatesURL string `env:"FX_RATES_URL" envDefault:"https://api.frankfurter.dev/v1"`

	// Public base URL of this Kontor (no trailing slash); used for the
	// back-links that MCP tools write into paperless. Empty = unset.
	PublicURL string `env:"PUBLIC_URL"`

	// Enable Banking (automatic bank import). Enabled when both the app ID
	// and the path of the app's RSA private key (PEM) are set.
	EnableBankingAppID        string        `env:"ENABLE_BANKING_APP_ID"`
	EnableBankingKeyPath      string        `env:"ENABLE_BANKING_KEY_PATH"`
	EnableBankingAPIURL       string        `env:"ENABLE_BANKING_API_URL"       envDefault:"https://api.enablebanking.com"`
	EnableBankingSyncInterval time.Duration `env:"ENABLE_BANKING_SYNC_INTERVAL" envDefault:"24h"`
}

func Load() (Config, error) {
	var cfg Config
	if err := env.Parse(&cfg); err != nil {
		return cfg, fmt.Errorf("parsing config: %w", err)
	}
	if err := cfg.resolveEncryptionKey(); err != nil {
		return cfg, err
	}
	if err := cfg.validateLLM(); err != nil {
		return cfg, err
	}
	if err := cfg.validateLedgerSuggest(); err != nil {
		return cfg, err
	}
	if cfg.FXRatesEnabled() && !isHTTPURL(cfg.FXRatesURL) {
		return cfg, errors.New(`FX_RATES_URL must be an absolute http(s) URL or "off"`)
	}
	if err := cfg.validateEnableBanking(); err != nil {
		return cfg, err
	}
	cfg.PublicURL = strings.TrimRight(cfg.PublicURL, "/")
	if cfg.PublicURL != "" && !validPublicURL(cfg.PublicURL) {
		return cfg, errors.New("PUBLIC_URL must be an absolute http(s) URL")
	}
	return cfg, nil
}

func validPublicURL(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != "" &&
		u.RawQuery == "" && u.Fragment == "" && !strings.Contains(raw, "?") && !strings.Contains(raw, "#")
}

func (c *Config) resolveEncryptionKey() error {
	switch {
	case c.LegacyEncryptionKey == "":
	case c.EncryptionKey == "":
		c.EncryptionKey = c.LegacyEncryptionKey
	case strings.TrimSpace(c.EncryptionKey) != strings.TrimSpace(c.LegacyEncryptionKey):
		return errors.New("ENCRYPTION_KEY and deprecated EMAIL_ENCRYPTION_KEY are both set with different values")
	}
	return nil
}

// UsesDeprecatedEncryptionKey reports whether EMAIL_ENCRYPTION_KEY is set.
func (c Config) UsesDeprecatedEncryptionKey() bool {
	return c.LegacyEncryptionKey != ""
}

func (c Config) LLMEnabled() bool {
	return c.LLMBaseURL != ""
}

func (c Config) LLMVisionEnabled() bool {
	return c.LLMEnabled() && c.LLMVisionModel != ""
}

func (c Config) FXRatesEnabled() bool {
	return c.FXRatesURL != "off"
}

func (c Config) validateLLM() error {
	switch {
	case !c.LLMEnabled() && (c.LLMModel != "" || c.LLMAPIKey != "" || c.LLMVisionModel != ""):
		return errors.New("LLM_MODEL, LLM_VISION_MODEL or LLM_API_KEY is set but LLM_BASE_URL is not")
	case c.LLMEnabled() && c.LLMModel == "":
		return errors.New("LLM_BASE_URL is set but LLM_MODEL is not")
	case c.LLMEnabled() && c.LLMTimeout <= 0:
		return errors.New("LLM_TIMEOUT must be positive")
	}
	if c.LLMEnabled() && !isHTTPURL(c.LLMBaseURL) {
		return errors.New("LLM_BASE_URL must be an absolute http(s) URL")
	}
	return nil
}

func (c Config) EnableBankingEnabled() bool {
	return c.EnableBankingAppID != "" && c.EnableBankingKeyPath != ""
}

func (c *Config) validateEnableBanking() error {
	c.EnableBankingAPIURL = strings.TrimRight(c.EnableBankingAPIURL, "/")
	switch {
	case (c.EnableBankingAppID == "") != (c.EnableBankingKeyPath == ""):
		return errors.New("ENABLE_BANKING_APP_ID and ENABLE_BANKING_KEY_PATH must be set together")
	case !isHTTPURL(c.EnableBankingAPIURL):
		return errors.New("ENABLE_BANKING_API_URL must be an absolute http(s) URL")
	case c.EnableBankingSyncInterval < 0:
		return errors.New("ENABLE_BANKING_SYNC_INTERVAL must not be negative")
	case c.EnableBankingEnabled():
		if _, err := cryptoutil.NormalizeEncryptionKey(c.EncryptionKey); err != nil {
			return errors.New("bank import needs a valid ENCRYPTION_KEY to store bank sessions")
		}
	}
	return nil
}

func (c Config) validateLedgerSuggest() error {
	switch {
	case c.LedgerSuggestBatchSize < 1 || c.LedgerSuggestBatchSize > 100:
		return errors.New("LEDGER_SUGGEST_BATCH_SIZE must be between 1 and 100")
	case c.LedgerSuggestMaxPerRun < c.LedgerSuggestBatchSize:
		return errors.New("LEDGER_SUGGEST_MAX_PER_RUN must be at least LEDGER_SUGGEST_BATCH_SIZE")
	}
	return nil
}

func isHTTPURL(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != ""
}

func (c Config) SlogLevel() slog.Level {
	switch c.LogLevel {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
