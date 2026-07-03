package migration

import (
	"encoding/binary"
	"errors"
	"fmt"
	"log/slog"

	"github.com/dgraph-io/badger/v4"
)

// Each module tracks its own schema version under _meta/schema/{moduleID}.
func versionKey(moduleID string) []byte {
	return []byte("_meta/schema/" + moduleID)
}

type Migration struct {
	Version     uint64
	Description string
	Run         func(db *badger.DB) error
}

// Head returns the version a database is at after running all migrations.
func Head(migrations []Migration) uint64 {
	var head uint64
	for _, m := range migrations {
		if m.Version > head {
			head = m.Version
		}
	}
	return head
}

func getVersion(db *badger.DB, moduleID string) (uint64, error) {
	var version uint64
	err := db.View(func(txn *badger.Txn) error {
		item, err := txn.Get(versionKey(moduleID))
		if err != nil {
			return err
		}
		return item.Value(func(val []byte) error {
			if len(val) != 8 {
				return fmt.Errorf("invalid schema version: expected 8 bytes, got %d", len(val))
			}
			version = binary.BigEndian.Uint64(val)
			return nil
		})
	})
	if errors.Is(err, badger.ErrKeyNotFound) {
		return 0, nil
	}
	return version, err
}

func setVersion(txn *badger.Txn, moduleID string, version uint64) error {
	buf := make([]byte, 8)
	binary.BigEndian.PutUint64(buf, version)
	return txn.Set(versionKey(moduleID), buf)
}

// RunModule applies a module's pending migrations, tracking progress under
// the module's own schema version key.
func RunModule(db *badger.DB, logger *slog.Logger, moduleID string, migrations []Migration) error {
	current, err := getVersion(db, moduleID)
	if err != nil {
		return fmt.Errorf("reading %s schema version: %w", moduleID, err)
	}

	logger.Info("migration check", "module", moduleID, "currentVersion", current, "availableMigrations", len(migrations))

	for _, m := range migrations {
		if m.Version <= current {
			continue
		}
		logger.Info("applying migration", "module", moduleID, "version", m.Version, "description", m.Description)

		if err := m.Run(db); err != nil {
			return fmt.Errorf("%s migration %d (%s): %w", moduleID, m.Version, m.Description, err)
		}

		if err := db.Update(func(txn *badger.Txn) error {
			return setVersion(txn, moduleID, m.Version)
		}); err != nil {
			return fmt.Errorf("updating %s schema version to %d: %w", moduleID, m.Version, err)
		}

		logger.Info("migration applied", "module", moduleID, "version", m.Version)
	}

	return nil
}
