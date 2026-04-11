package config

import (
	"fmt"
	"log/slog"

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
