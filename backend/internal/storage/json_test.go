package storage

import (
	"errors"
	"testing"

	"github.com/dgraph-io/badger/v4"
)

type jsonItem struct {
	Name string `json:"name"`
}

func openTestDB(t *testing.T) *badger.DB {
	t.Helper()
	db, err := badger.Open(badger.DefaultOptions("").WithInMemory(true).WithLogger(nil))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func TestGetAndList(t *testing.T) {
	db := openTestDB(t)
	if err := db.Update(func(txn *badger.Txn) error {
		for key, name := range map[string]string{"p/b": "b", "p/a": "a", "q/c": "c"} {
			if err := SetJSON(txn, []byte(key), jsonItem{Name: name}); err != nil {
				return err
			}
		}
		return txn.Set([]byte("bad/x"), []byte("{"))
	}); err != nil {
		t.Fatal(err)
	}

	err := db.View(func(txn *badger.Txn) error {
		got, err := Get[jsonItem](txn, []byte("p/a"))
		if err != nil || got.Name != "a" {
			t.Errorf("Get = %+v, %v; want a", got, err)
		}
		if _, err := Get[jsonItem](txn, []byte("p/missing")); !errors.Is(err, ErrNotFound) {
			t.Errorf("Get missing: err = %v, want ErrNotFound", err)
		}
		if got, err := Get[jsonItem](txn, []byte("bad/x")); err == nil || got != (jsonItem{}) {
			t.Errorf("Get invalid JSON = %+v, %v; want zero value and an error", got, err)
		}

		items, err := List[jsonItem](txn, []byte("p/"))
		if err != nil || len(items) != 2 || items[0].Name != "a" || items[1].Name != "b" {
			t.Errorf("List = %+v, %v; want [a b]", items, err)
		}
		empty, err := List[jsonItem](txn, []byte("none/"))
		if err != nil || empty == nil || len(empty) != 0 {
			t.Errorf("List empty = %#v, %v; want a non-nil empty slice", empty, err)
		}
		if _, err := List[jsonItem](txn, []byte("bad/")); err == nil {
			t.Error("List invalid JSON: want an error")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
