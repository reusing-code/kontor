package storage

import (
	"encoding/json"
	"errors"

	"github.com/dgraph-io/badger/v4"
)

// GetJSON reads the value at key into v, mapping badger.ErrKeyNotFound to ErrNotFound.
func GetJSON(txn *badger.Txn, key []byte, v any) error {
	item, err := txn.Get(key)
	if errors.Is(err, badger.ErrKeyNotFound) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	return item.Value(func(val []byte) error {
		return json.Unmarshal(val, v)
	})
}

// SetJSON marshals v and stores it at key.
func SetJSON(txn *badger.Txn, key []byte, v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return txn.Set(key, data)
}

// IteratePrefix calls fn with the raw value of every key under prefix.
func IteratePrefix(txn *badger.Txn, prefix []byte, fn func(key, val []byte) error) error {
	opts := badger.DefaultIteratorOptions
	opts.Prefix = prefix
	it := txn.NewIterator(opts)
	defer it.Close()
	for it.Rewind(); it.Valid(); it.Next() {
		item := it.Item()
		key := item.KeyCopy(nil)
		if err := item.Value(func(val []byte) error {
			return fn(key, val)
		}); err != nil {
			return err
		}
	}
	return nil
}

// HasPrefix reports whether at least one key exists under prefix.
func HasPrefix(txn *badger.Txn, prefix []byte) bool {
	opts := badger.DefaultIteratorOptions
	opts.Prefix = prefix
	opts.PrefetchValues = false
	it := txn.NewIterator(opts)
	defer it.Close()
	it.Rewind()
	return it.Valid()
}

// Get returns the JSON value at key, or ErrNotFound when the key is missing.
func Get[T any](txn *badger.Txn, key []byte) (T, error) {
	var v T
	if err := GetJSON(txn, key, &v); err != nil {
		var zero T
		return zero, err
	}
	return v, nil
}

// List decodes every value under prefix in key order. The result is never
// nil, so an empty list encodes as [] and not as null.
func List[T any](txn *badger.Txn, prefix []byte) ([]T, error) {
	out := []T{}
	opts := badger.DefaultIteratorOptions
	opts.Prefix = prefix
	it := txn.NewIterator(opts)
	defer it.Close()
	for it.Rewind(); it.Valid(); it.Next() {
		var v T
		if err := it.Item().Value(func(val []byte) error {
			return json.Unmarshal(val, &v)
		}); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, nil
}
