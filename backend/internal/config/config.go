package config

import (
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"time"

	"github.com/caarlos0/env/v11"
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

	EmailEncryptionKey string `env:"EMAIL_ENCRYPTION_KEY"`

	// Interval between background IMAP scans of ledger email accounts. 0 disables.
	LedgerEmailScanInterval time.Duration `env:"LEDGER_EMAIL_SCAN_INTERVAL" envDefault:"6h"`

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
}

func Load() (Config, error) {
	var cfg Config
	if err := env.Parse(&cfg); err != nil {
		return cfg, fmt.Errorf("parsing config: %w", err)
	}
	if err := cfg.validateLLM(); err != nil {
		return cfg, err
	}
	return cfg, nil
}

func (c Config) LLMEnabled() bool {
	return c.LLMBaseURL != ""
}

func (c Config) validateLLM() error {
	switch {
	case !c.LLMEnabled() && (c.LLMModel != "" || c.LLMAPIKey != ""):
		return errors.New("LLM_MODEL or LLM_API_KEY is set but LLM_BASE_URL is not")
	case c.LLMEnabled() && c.LLMModel == "":
		return errors.New("LLM_BASE_URL is set but LLM_MODEL is not")
	case c.LLMEnabled() && c.LLMTimeout <= 0:
		return errors.New("LLM_TIMEOUT must be positive")
	}
	if c.LLMEnabled() {
		u, err := url.Parse(c.LLMBaseURL)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return errors.New("LLM_BASE_URL must be an absolute http(s) URL")
		}
	}
	return nil
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
