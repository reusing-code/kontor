package migration

import (
	"errors"
	"log/slog"
	"testing"

	"github.com/dgraph-io/badger/v4"
)

func openTestDB(t *testing.T) *badger.DB {
	t.Helper()
	opts := badger.DefaultOptions("").WithInMemory(true).WithLogger(nil)
	db, err := badger.Open(opts)
	if err != nil {
		t.Fatalf("open badger: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func readVersion(t *testing.T, db *badger.DB, moduleID string) uint64 {
	t.Helper()
	v, err := getVersion(db, moduleID)
	if err != nil {
		t.Fatalf("getVersion: %v", err)
	}
	return v
}

func testLogger() *slog.Logger { return slog.New(slog.DiscardHandler) }

func TestRunModule_FreshDBIsVersionZero(t *testing.T) {
	db := openTestDB(t)
	if err := RunModule(db, testLogger(), "contracts", nil); err != nil {
		t.Fatalf("RunModule: %v", err)
	}
	if v := readVersion(t, db, "contracts"); v != 0 {
		t.Fatalf("version = %d, want 0", v)
	}
}

func TestRunModule_AppliesPendingInOrder(t *testing.T) {
	db := openTestDB(t)
	var applied []uint64
	migs := []Migration{
		{Version: 1, Description: "one", Run: func(*badger.DB) error { applied = append(applied, 1); return nil }},
		{Version: 2, Description: "two", Run: func(*badger.DB) error { applied = append(applied, 2); return nil }},
	}
	if err := RunModule(db, testLogger(), "contracts", migs); err != nil {
		t.Fatalf("RunModule: %v", err)
	}
	if len(applied) != 2 || applied[0] != 1 || applied[1] != 2 {
		t.Fatalf("applied = %v, want [1 2]", applied)
	}
	if v := readVersion(t, db, "contracts"); v != 2 {
		t.Fatalf("version = %d, want 2", v)
	}
}

func TestRunModule_IdempotentOnRerun(t *testing.T) {
	db := openTestDB(t)
	runs := 0
	migs := []Migration{
		{Version: 1, Description: "one", Run: func(*badger.DB) error { runs++; return nil }},
	}
	for range 2 {
		if err := RunModule(db, testLogger(), "ledger", migs); err != nil {
			t.Fatalf("RunModule: %v", err)
		}
	}
	if runs != 1 {
		t.Fatalf("migration ran %d times, want 1", runs)
	}
}

func TestRunModule_VersionsAreIndependentPerModule(t *testing.T) {
	db := openTestDB(t)
	migs := []Migration{
		{Version: 3, Description: "three", Run: func(*badger.DB) error { return nil }},
	}
	if err := RunModule(db, testLogger(), "contracts", migs); err != nil {
		t.Fatalf("RunModule: %v", err)
	}
	if v := readVersion(t, db, "contracts"); v != 3 {
		t.Fatalf("contracts version = %d, want 3", v)
	}
	if v := readVersion(t, db, "ledger"); v != 0 {
		t.Fatalf("ledger version = %d, want 0", v)
	}
}

func TestRunModule_StopsOnError(t *testing.T) {
	db := openTestDB(t)
	boom := errors.New("boom")
	migs := []Migration{
		{Version: 1, Description: "one", Run: func(*badger.DB) error { return nil }},
		{Version: 2, Description: "two", Run: func(*badger.DB) error { return boom }},
		{Version: 3, Description: "three", Run: func(*badger.DB) error { t.Error("must not run"); return nil }},
	}
	if err := RunModule(db, testLogger(), "auto", migs); !errors.Is(err, boom) {
		t.Fatalf("err = %v, want %v", err, boom)
	}
	if v := readVersion(t, db, "auto"); v != 1 {
		t.Fatalf("version = %d, want 1", v)
	}
}

func TestHead(t *testing.T) {
	if h := Head(nil); h != 0 {
		t.Fatalf("Head(nil) = %d, want 0", h)
	}
	if h := Head([]Migration{{Version: 2}, {Version: 5}, {Version: 3}}); h != 5 {
		t.Fatalf("Head = %d, want 5", h)
	}
}
