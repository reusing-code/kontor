package migration

import (
	"encoding/json"

	"github.com/dgraph-io/badger/v4"
)

var V1RenamePriceField = Migration{
	Version:     1,
	Description: "rename pricePerMonth to price, add billingInterval default",
	Run:         v1RenamePriceField,
}

func v1RenamePriceField(db *badger.DB) error {
	// Collect all contract keys and their transformed values.
	// Contract keys match: u/{userID}/con/{contractID}
	type kv struct {
		key []byte
		val []byte
	}
	var updates []kv

	err := db.View(func(txn *badger.Txn) error {
		it := txn.NewIterator(badger.DefaultIteratorOptions)
		defer it.Close()

		prefix := []byte("u/")
		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			item := it.Item()
			key := item.Key()

			if !isContractKey(key) {
				continue
			}

			err := item.Value(func(val []byte) error {
				transformed, changed, err := transformContract(val)
				if err != nil {
					return err
				}
				if changed {
					keyCopy := make([]byte, len(key))
					copy(keyCopy, key)
					updates = append(updates, kv{key: keyCopy, val: transformed})
				}
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

	if len(updates) == 0 {
		return nil
	}

	return db.Update(func(txn *badger.Txn) error {
		for _, u := range updates {
			if err := txn.Set(u.key, u.val); err != nil {
				return err
			}
		}
		return nil
	})
}

// isContractKey checks if a badger key matches the pattern u/{id}/con/{id}
func isContractKey(key []byte) bool {
	s := string(key)
	// Minimum: "u/x/con/y" = 9 chars
	if len(s) < 9 {
		return false
	}
	if s[:2] != "u/" {
		return false
	}
	// Find /con/ after the user ID segment
	for i := 2; i < len(s)-4; i++ {
		if s[i:i+5] == "/con/" && i > 2 && i+5 < len(s) {
			return true
		}
	}
	return false
}

// transformContract renames pricePerMonth → price and sets billingInterval if missing.
// Returns the new JSON and whether any change was made.
func transformContract(data []byte) ([]byte, bool, error) {
	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, false, err
	}

	changed := false

	if val, ok := doc["pricePerMonth"]; ok {
		doc["price"] = val
		delete(doc, "pricePerMonth")
		changed = true
	}

	if _, ok := doc["billingInterval"]; !ok {
		doc["billingInterval"] = "monthly"
		changed = true
	} else if doc["billingInterval"] == "" {
		doc["billingInterval"] = "monthly"
		changed = true
	}

	if !changed {
		return data, false, nil
	}

	out, err := json.Marshal(doc)
	if err != nil {
		return nil, false, err
	}
	return out, true, nil
}
