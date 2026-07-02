package storage

import (
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/dgraph-io/badger/v4"
)

func newTestEngine(t *testing.T) *Engine {
	t.Helper()
	e, err := Open(t.TempDir(), slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatalf("opening engine: %v", err)
	}
	t.Cleanup(func() { e.Close() })
	return e
}

func TestWriteBackup_CreatesSnapshot(t *testing.T) {
	e := newTestEngine(t)
	if err := e.Update(func(txn *badger.Txn) error {
		return txn.Set([]byte("test/key"), []byte("value"))
	}); err != nil {
		t.Fatalf("seeding data: %v", err)
	}
	dir := t.TempDir()

	path, err := e.writeBackup(dir)
	if err != nil {
		t.Fatalf("writeBackup: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat backup: %v", err)
	}
	if info.Size() == 0 {
		t.Error("backup file is empty")
	}
	files, err := listBackupFiles(dir)
	if err != nil {
		t.Fatalf("listBackupFiles: %v", err)
	}
	if len(files) != 1 || files[0] != path {
		t.Errorf("listBackupFiles = %v, want [%s]", files, path)
	}
}

func TestPruneBackups_KeepsNewest(t *testing.T) {
	dir := t.TempDir()
	names := []string{
		"badger-20260601-000000.bak",
		"badger-20260602-000000.bak",
		"badger-20260603-000000.bak",
		"badger-20260604-000000.bak",
	}
	for _, name := range names {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o644); err != nil {
			t.Fatalf("seeding backup file: %v", err)
		}
	}
	// unrelated files must survive pruning
	if err := os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("x"), 0o644); err != nil {
		t.Fatalf("seeding unrelated file: %v", err)
	}

	if err := pruneBackups(dir, 2); err != nil {
		t.Fatalf("pruneBackups: %v", err)
	}

	files, err := listBackupFiles(dir)
	if err != nil {
		t.Fatalf("listBackupFiles: %v", err)
	}
	want := []string{
		filepath.Join(dir, "badger-20260603-000000.bak"),
		filepath.Join(dir, "badger-20260604-000000.bak"),
	}
	if len(files) != 2 || files[0] != want[0] || files[1] != want[1] {
		t.Errorf("remaining = %v, want %v", files, want)
	}
	if _, err := os.Stat(filepath.Join(dir, "notes.txt")); err != nil {
		t.Errorf("unrelated file removed: %v", err)
	}
}

func TestListBackupFiles_MissingDir(t *testing.T) {
	files, err := listBackupFiles(filepath.Join(t.TempDir(), "does-not-exist"))
	if err != nil {
		t.Fatalf("listBackupFiles: %v", err)
	}
	if len(files) != 0 {
		t.Errorf("files = %v, want empty", files)
	}
}
