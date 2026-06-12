package config

import (
	"fmt"
	"log/slog"
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
}

func Load() (Config, error) {
	var cfg Config
	if err := env.Parse(&cfg); err != nil {
		return cfg, fmt.Errorf("parsing config: %w", err)
	}
	return cfg, nil
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
