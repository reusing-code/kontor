package storage

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	backupFilePrefix = "badger-"
	backupFileSuffix = ".bak"
)

type BackupConfig struct {
	Dir      string
	Interval time.Duration
	Keep     int
}

// Backup streams a full snapshot of the database to w in Badger's backup format.
// Restore with `badger restore` or badger.DB.Load.
func (e *Engine) Backup(w io.Writer) error {
	_, err := e.db.Backup(w, 0)
	return err
}

// StartBackups periodically writes full snapshots to cfg.Dir, keeping the
// cfg.Keep most recent files. The age of the newest existing snapshot decides
// whether a backup is due, so restarts do not trigger redundant backups.
func (e *Engine) StartBackups(ctx context.Context, cfg BackupConfig) {
	go func() {
		logger := e.logger.With("component", "backup")
		logger.Info("backup scheduler started", "dir", cfg.Dir, "interval", cfg.Interval, "keep", cfg.Keep)
		e.backupIfDue(cfg, logger)

		tick := cfg.Interval
		if tick > time.Hour {
			tick = time.Hour
		}
		ticker := time.NewTicker(tick)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				logger.Info("backup scheduler stopped")
				return
			case <-ticker.C:
				e.backupIfDue(cfg, logger)
			}
		}
	}()
}

func (e *Engine) backupIfDue(cfg BackupConfig, logger *slog.Logger) {
	files, err := listBackupFiles(cfg.Dir)
	if err != nil {
		logger.Error("listing backup dir", "error", err)
		return
	}
	if len(files) > 0 {
		newest, err := os.Stat(files[len(files)-1])
		if err == nil && time.Since(newest.ModTime()) < cfg.Interval {
			return
		}
	}
	path, err := e.writeBackup(cfg.Dir)
	if err != nil {
		logger.Error("writing backup", "error", err)
		return
	}
	logger.Info("backup written", "path", path)
	if err := pruneBackups(cfg.Dir, cfg.Keep); err != nil {
		logger.Error("pruning backups", "error", err)
	}
}

func (e *Engine) writeBackup(dir string) (string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("creating backup dir: %w", err)
	}
	name := fmt.Sprintf("%s%s%s", backupFilePrefix, time.Now().UTC().Format("20060102-150405"), backupFileSuffix)
	finalPath := filepath.Join(dir, name)
	tmp, err := os.CreateTemp(dir, name+".tmp-*")
	if err != nil {
		return "", fmt.Errorf("creating temp backup file: %w", err)
	}
	defer os.Remove(tmp.Name())
	if err := e.Backup(tmp); err != nil {
		tmp.Close()
		return "", fmt.Errorf("streaming backup: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return "", fmt.Errorf("closing backup file: %w", err)
	}
	if err := os.Rename(tmp.Name(), finalPath); err != nil {
		return "", fmt.Errorf("finalizing backup file: %w", err)
	}
	return finalPath, nil
}

// listBackupFiles returns full paths of snapshot files in dir, oldest first.
// The timestamp filename format makes lexical order chronological.
func listBackupFiles(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	files := make([]string, 0, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasPrefix(name, backupFilePrefix) || !strings.HasSuffix(name, backupFileSuffix) {
			continue
		}
		files = append(files, filepath.Join(dir, name))
	}
	sort.Strings(files)
	return files, nil
}

func pruneBackups(dir string, keep int) error {
	if keep <= 0 {
		return nil
	}
	files, err := listBackupFiles(dir)
	if err != nil {
		return err
	}
	for len(files) > keep {
		if err := os.Remove(files[0]); err != nil {
			return err
		}
		files = files[1:]
	}
	return nil
}
