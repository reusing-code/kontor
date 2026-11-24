package main

import (
	"log/slog"
	"os"

	"github.com/reusing-code/kontor/backend/internal/config"
	"github.com/reusing-code/kontor/backend/internal/server"
	"github.com/reusing-code/kontor/backend/internal/storage"
)

func main() {
	if !run() {
		os.Exit(1)
	}
}

// run returns only after the database is closed, so a serve error does not
// skip the close.
func run() bool {
	cfg, err := config.Load()
	if err != nil {
		slog.Error("loading config", "error", err)
		return false
	}

	var handler slog.Handler
	opts := &slog.HandlerOptions{Level: cfg.SlogLevel()}
	if cfg.LogFormat == "json" {
		handler = slog.NewJSONHandler(os.Stdout, opts)
	} else {
		handler = slog.NewTextHandler(os.Stdout, opts)
	}
	logger := slog.New(handler)
	slog.SetDefault(logger)

	if cfg.UsesDeprecatedEncryptionKey() {
		logger.Warn("EMAIL_ENCRYPTION_KEY is deprecated, rename it to ENCRYPTION_KEY")
	}

	engine, err := storage.Open(cfg.DBPath, logger)
	if err != nil {
		logger.Error("opening database", "error", err)
		return false
	}
	defer func() {
		if err := engine.Close(); err != nil {
			logger.Error("closing database", "error", err)
		}
	}()

	if err := server.New(cfg, logger, engine).Run(); err != nil {
		logger.Error("server error", "error", err)
		return false
	}
	return true
}
