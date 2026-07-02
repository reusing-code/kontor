package migration

import (
	"encoding/binary"
	"fmt"
	"log/slog"

	"github.com/dgraph-io/badger/v4"
)

var versionKey = []byte("_meta/schema_version")

type Migration struct {
	Version     uint64
	Description string
	Run         func(db *badger.DB) error
}

func getVersion(db *badger.DB) (uint64, error) {
	var version uint64
	err := db.View(func(txn *badger.Txn) error {
		item, err := txn.Get(versionKey)
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
	if err == badger.ErrKeyNotFound {
		return 0, nil
	}
	return version, err
}

func setVersion(txn *badger.Txn, version uint64) error {
	buf := make([]byte, 8)
	binary.BigEndian.PutUint64(buf, version)
	return txn.Set(versionKey, buf)
}

func RunAll(db *badger.DB, logger *slog.Logger, migrations []Migration) error {
	current, err := getVersion(db)
	if err != nil {
		return fmt.Errorf("reading schema version: %w", err)
	}

	logger.Info("migration check", "currentVersion", current, "availableMigrations", len(migrations))

	for _, m := range migrations {
		if m.Version <= current {
			continue
		}
		logger.Info("applying migration", "version", m.Version, "description", m.Description)

		if err := m.Run(db); err != nil {
			return fmt.Errorf("migration %d (%s): %w", m.Version, m.Description, err)
		}

		if err := db.Update(func(txn *badger.Txn) error {
			return setVersion(txn, m.Version)
		}); err != nil {
			return fmt.Errorf("updating schema version to %d: %w", m.Version, err)
		}

		logger.Info("migration applied", "version", m.Version)
	}

	return nil
}
