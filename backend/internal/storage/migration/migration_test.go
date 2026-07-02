package migration

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
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

func readVersion(t *testing.T, db *badger.DB) uint64 {
	t.Helper()
	v, err := getVersion(db)
	if err != nil {
		t.Fatalf("getVersion: %v", err)
	}
	return v
}

func putJSON(t *testing.T, db *badger.DB, key string, doc map[string]any) {
	t.Helper()
	data, err := json.Marshal(doc)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	err = db.Update(func(txn *badger.Txn) error {
		return txn.Set([]byte(key), data)
	})
	if err != nil {
		t.Fatalf("put: %v", err)
	}
}

func getJSON(t *testing.T, db *badger.DB, key string) map[string]any {
	t.Helper()
	var doc map[string]any
	err := db.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte(key))
		if err != nil {
			return err
		}
		return item.Value(func(val []byte) error {
			return json.Unmarshal(val, &doc)
		})
	})
	if err != nil {
		t.Fatalf("get %s: %v", key, err)
	}
	return doc
}

// Runner tests

func TestRunAll_EmptyDB(t *testing.T) {
	db := openTestDB(t)

	called := false
	migrations := []Migration{{
		Version:     1,
		Description: "test",
		Run: func(db *badger.DB) error {
			called = true
			return nil
		},
	}}

	if err := RunAll(db, slog.Default(), migrations); err != nil {
		t.Fatalf("RunAll: %v", err)
	}
	if !called {
		t.Error("migration was not called")
	}
	if v := readVersion(t, db); v != 1 {
		t.Errorf("version = %d, want 1", v)
	}
}

func TestRunAll_SkipsAlreadyApplied(t *testing.T) {
	db := openTestDB(t)

	// Set version to 1 manually
	err := db.Update(func(txn *badger.Txn) error {
		buf := make([]byte, 8)
		binary.BigEndian.PutUint64(buf, 1)
		return txn.Set(versionKey, buf)
	})
	if err != nil {
		t.Fatal(err)
	}

	called := false
	migrations := []Migration{{
		Version:     1,
		Description: "should be skipped",
		Run: func(db *badger.DB) error {
			called = true
			return nil
		},
	}}

	if err := RunAll(db, slog.Default(), migrations); err != nil {
		t.Fatalf("RunAll: %v", err)
	}
	if called {
		t.Error("migration should have been skipped")
	}
}

func TestRunAll_AppliesMultipleInOrder(t *testing.T) {
	db := openTestDB(t)

	var order []uint64
	migrations := []Migration{
		{Version: 1, Description: "first", Run: func(db *badger.DB) error {
			order = append(order, 1)
			return nil
		}},
		{Version: 2, Description: "second", Run: func(db *badger.DB) error {
			order = append(order, 2)
			return nil
		}},
		{Version: 3, Description: "third", Run: func(db *badger.DB) error {
			order = append(order, 3)
			return nil
		}},
	}

	if err := RunAll(db, slog.Default(), migrations); err != nil {
		t.Fatalf("RunAll: %v", err)
	}
	if len(order) != 3 {
		t.Fatalf("expected 3 migrations, ran %d", len(order))
	}
	for i, v := range order {
		if v != uint64(i+1) {
			t.Errorf("order[%d] = %d, want %d", i, v, i+1)
		}
	}
	if v := readVersion(t, db); v != 3 {
		t.Errorf("version = %d, want 3", v)
	}
}

func TestRunAll_StopsOnError(t *testing.T) {
	db := openTestDB(t)

	migrations := []Migration{
		{Version: 1, Description: "ok", Run: func(db *badger.DB) error { return nil }},
		{Version: 2, Description: "fails", Run: func(db *badger.DB) error {
			return fmt.Errorf("boom")
		}},
		{Version: 3, Description: "never", Run: func(db *badger.DB) error {
			t.Error("should not be reached")
			return nil
		}},
	}

	err := RunAll(db, slog.Default(), migrations)
	if err == nil {
		t.Fatal("expected error")
	}
	if v := readVersion(t, db); v != 1 {
		t.Errorf("version = %d, want 1 (should stop at failed migration)", v)
	}
}

// V1 migration tests

func TestV1_RenamesPricePerMonth(t *testing.T) {
	db := openTestDB(t)

	key := "u/user1/con/abc-123"
	putJSON(t, db, key, map[string]any{
		"id":            "abc-123",
		"categoryId":    "cat-1",
		"name":          "Netflix",
		"pricePerMonth": 12.99,
		"startDate":     "2025-01-01",
	})

	if err := v1RenamePriceField(db); err != nil {
		t.Fatalf("v1: %v", err)
	}

	doc := getJSON(t, db, key)
	if _, ok := doc["pricePerMonth"]; ok {
		t.Error("pricePerMonth should have been removed")
	}
	if price, ok := doc["price"]; !ok {
		t.Error("price field missing")
	} else if price.(float64) != 12.99 {
		t.Errorf("price = %v, want 12.99", price)
	}
	if bi, ok := doc["billingInterval"]; !ok {
		t.Error("billingInterval field missing")
	} else if bi != "monthly" {
		t.Errorf("billingInterval = %v, want monthly", bi)
	}
}

func TestV1_AddsBillingIntervalToExistingPrice(t *testing.T) {
	db := openTestDB(t)

	// Already has "price" but no billingInterval
	key := "u/user1/con/def-456"
	putJSON(t, db, key, map[string]any{
		"id":         "def-456",
		"categoryId": "cat-1",
		"name":       "Spotify",
		"price":      9.99,
		"startDate":  "2025-01-01",
	})

	if err := v1RenamePriceField(db); err != nil {
		t.Fatalf("v1: %v", err)
	}

	doc := getJSON(t, db, key)
	if doc["price"].(float64) != 9.99 {
		t.Errorf("price = %v, want 9.99", doc["price"])
	}
	if doc["billingInterval"] != "monthly" {
		t.Errorf("billingInterval = %v, want monthly", doc["billingInterval"])
	}
}

func TestV1_LeavesAlreadyMigratedAlone(t *testing.T) {
	db := openTestDB(t)

	key := "u/user1/con/ghi-789"
	putJSON(t, db, key, map[string]any{
		"id":              "ghi-789",
		"categoryId":      "cat-1",
		"name":            "AWS",
		"price":           100.0,
		"billingInterval": "yearly",
		"startDate":       "2025-01-01",
	})

	if err := v1RenamePriceField(db); err != nil {
		t.Fatalf("v1: %v", err)
	}

	doc := getJSON(t, db, key)
	if doc["price"].(float64) != 100.0 {
		t.Errorf("price = %v, want 100.0", doc["price"])
	}
	if doc["billingInterval"] != "yearly" {
		t.Errorf("billingInterval = %v, want yearly", doc["billingInterval"])
	}
}

func TestV1_IgnoresNonContractKeys(t *testing.T) {
	db := openTestDB(t)

	catKey := "u/user1/cat/cat-1"
	putJSON(t, db, catKey, map[string]any{
		"id":   "cat-1",
		"name": "Insurance",
	})

	conKey := "u/user1/con/abc-123"
	putJSON(t, db, conKey, map[string]any{
		"id":            "abc-123",
		"name":          "Policy",
		"pricePerMonth": 50.0,
	})

	if err := v1RenamePriceField(db); err != nil {
		t.Fatalf("v1: %v", err)
	}

	// Category should be untouched
	catDoc := getJSON(t, db, catKey)
	if catDoc["name"] != "Insurance" {
		t.Errorf("category was modified")
	}
	if _, ok := catDoc["billingInterval"]; ok {
		t.Error("billingInterval should not be added to categories")
	}

	// Contract should be transformed
	conDoc := getJSON(t, db, conKey)
	if _, ok := conDoc["pricePerMonth"]; ok {
		t.Error("pricePerMonth should be removed")
	}
	if conDoc["price"].(float64) != 50.0 {
		t.Errorf("price = %v, want 50.0", conDoc["price"])
	}
}

func TestV1_MultipleUsersMultipleContracts(t *testing.T) {
	db := openTestDB(t)

	contracts := map[string]map[string]any{
		"u/alice/con/c1": {"id": "c1", "name": "C1", "pricePerMonth": 10.0},
		"u/alice/con/c2": {"id": "c2", "name": "C2", "pricePerMonth": 20.0},
		"u/bob/con/c3":   {"id": "c3", "name": "C3", "pricePerMonth": 30.0},
	}
	for k, v := range contracts {
		putJSON(t, db, k, v)
	}

	if err := v1RenamePriceField(db); err != nil {
		t.Fatalf("v1: %v", err)
	}

	for k := range contracts {
		doc := getJSON(t, db, k)
		if _, ok := doc["pricePerMonth"]; ok {
			t.Errorf("%s: pricePerMonth still present", k)
		}
		if _, ok := doc["price"]; !ok {
			t.Errorf("%s: price field missing", k)
		}
		if doc["billingInterval"] != "monthly" {
			t.Errorf("%s: billingInterval = %v, want monthly", k, doc["billingInterval"])
		}
	}
}

func TestIsContractKey(t *testing.T) {
	tests := []struct {
		key  string
		want bool
	}{
		{"u/user1/con/abc", true},
		{"u/alice/con/some-uuid-here", true},
		{"u/user1/cat/abc", false},
		{"u/user1/idx/cat_con/a/b", false},
		{"usr/user1", false},
		{"u/user1/settings", false},
		{"_meta/schema_version", false},
	}
	for _, tt := range tests {
		got := isContractKey([]byte(tt.key))
		if got != tt.want {
			t.Errorf("isContractKey(%q) = %v, want %v", tt.key, got, tt.want)
		}
	}
}
