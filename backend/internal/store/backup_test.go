package store

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWriteBackup_CreatesSnapshot(t *testing.T) {
	s := newTestStore(t)
	dir := t.TempDir()

	path, err := s.writeBackup(dir)
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
