package migration

import (
	"strings"

	"github.com/dgraph-io/badger/v4"
)

var V2ModuleCategories = Migration{
	Version:     2,
	Description: "move category keys from u/{userId}/cat/ to u/{userId}/mod/contracts/cat/",
	Run:         v2ModuleCategories,
}

func v2ModuleCategories(db *badger.DB) error {
	type kv struct {
		oldKey []byte
		newKey []byte
		val    []byte
	}
	var moves []kv

	err := db.View(func(txn *badger.Txn) error {
		it := txn.NewIterator(badger.DefaultIteratorOptions)
		defer it.Close()

		prefix := []byte("u/")
		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			item := it.Item()
			key := string(item.Key())

			// Match u/{userId}/cat/{catId} but NOT u/{userId}/mod/*/cat/*
			userEnd := strings.Index(key[2:], "/")
			if userEnd < 0 {
				continue
			}
			userEnd += 2
			rest := key[userEnd:]
			if !strings.HasPrefix(rest, "/cat/") {
				continue
			}
			catID := rest[5:]
			if catID == "" {
				continue
			}

			userID := key[2:userEnd]
			newKey := "u/" + userID + "/mod/contracts/cat/" + catID

			err := item.Value(func(val []byte) error {
				keyCopy := make([]byte, len(item.Key()))
				copy(keyCopy, item.Key())
				valCopy := make([]byte, len(val))
				copy(valCopy, val)
				moves = append(moves, kv{
					oldKey: keyCopy,
					newKey: []byte(newKey),
					val:    valCopy,
				})
				return nil
			})
			if err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return err
	}

	if len(moves) == 0 {
		return nil
	}

	return db.Update(func(txn *badger.Txn) error {
		for _, m := range moves {
			if err := txn.Set(m.newKey, m.val); err != nil {
				return err
			}
			if err := txn.Delete(m.oldKey); err != nil {
				return err
			}
		}
		return nil
	})
}
