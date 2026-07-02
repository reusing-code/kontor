package storage

import (
	"fmt"
	"log/slog"
	"time"

	"github.com/dgraph-io/badger/v4"
	"github.com/reusing-code/kontor/backend/internal/storage/migration"
)

// Engine owns the badger database shared by all module stores.
type Engine struct {
	db     *badger.DB
	logger *slog.Logger
	done   chan struct{}
}

type badgerLogger struct {
	logger *slog.Logger
}

func (l *badgerLogger) Errorf(f string, v ...interface{})   { l.logger.Error(fmt.Sprintf(f, v...)) }
func (l *badgerLogger) Warningf(f string, v ...interface{}) { l.logger.Warn(fmt.Sprintf(f, v...)) }
func (l *badgerLogger) Infof(f string, v ...interface{})    { l.logger.Info(fmt.Sprintf(f, v...)) }
func (l *badgerLogger) Debugf(f string, v ...interface{})   { l.logger.Debug(fmt.Sprintf(f, v...)) }

func Open(path string, logger *slog.Logger) (*Engine, error) {
	opts := badger.DefaultOptions(path).
		WithLogger(&badgerLogger{logger: logger.With("component", "badger")})

	db, err := badger.Open(opts)
	if err != nil {
		return nil, fmt.Errorf("opening badger db: %w", err)
	}

	if err := migration.RunAll(db, logger, migration.All); err != nil {
		db.Close()
		return nil, fmt.Errorf("running migrations: %w", err)
	}

	e := &Engine{
		db:     db,
		logger: logger,
		done:   make(chan struct{}),
	}
	go e.runGC()
	return e, nil
}

func (e *Engine) DB() *badger.DB { return e.db }

func (e *Engine) Logger() *slog.Logger { return e.logger }

func (e *Engine) View(fn func(txn *badger.Txn) error) error { return e.db.View(fn) }

func (e *Engine) Update(fn func(txn *badger.Txn) error) error { return e.db.Update(fn) }

func (e *Engine) runGC() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-e.done:
			return
		case <-ticker.C:
			for e.db.RunValueLogGC(0.5) == nil {
				// keep running until no more GC needed
			}
		}
	}
}

func (e *Engine) Close() error {
	close(e.done)
	return e.db.Close()
}

func (e *Engine) Healthy() error {
	return e.db.View(func(txn *badger.Txn) error {
		return nil
	})
}
