package drafts

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/dgraph-io/badger/v4"
	"github.com/google/uuid"
	"github.com/reusing-code/kontor/backend/internal/storage"
)

// ErrNotPending is returned when a decided draft is changed or decided again.
var ErrNotPending = fmt.Errorf("%w: draft is not pending", storage.ErrConflict)

// ErrDuplicateSource is returned when a draft for the same source exists.
var ErrDuplicateSource = fmt.Errorf("%w: a draft for this source exists", storage.ErrConflict)

type Store struct {
	e *storage.Engine
}

func NewStore(e *storage.Engine) *Store {
	return &Store{e: e}
}

func itemKey(userID string, id uuid.UUID) []byte {
	return []byte(fmt.Sprintf("u/%s/drafts/item/%s", userID, id))
}

func itemPrefix(userID string) []byte {
	return []byte(fmt.Sprintf("u/%s/drafts/item/", userID))
}

func sourceKey(userID, kind, ref string) []byte {
	return []byte(fmt.Sprintf("u/%s/drafts/src/%s/%s", userID, kind, ref))
}

// Create stores a draft and its source index entry; ErrDuplicateSource when
// the source already has a draft.
func (s *Store) Create(_ context.Context, userID string, d Draft) error {
	return s.e.Update(func(txn *badger.Txn) error {
		srcKey := sourceKey(userID, d.Source.Kind, d.Source.Ref)
		if _, err := txn.Get(srcKey); err == nil {
			return ErrDuplicateSource
		} else if !errors.Is(err, badger.ErrKeyNotFound) {
			return err
		}
		if err := txn.Set(srcKey, []byte(d.ID.String())); err != nil {
			return err
		}
		return storage.SetJSON(txn, itemKey(userID, d.ID), d)
	})
}

// HasSource reports whether the source currently has a draft. Deciding a
// draft frees its source, so a re-tagged document produces a new draft.
func (s *Store) HasSource(_ context.Context, userID, kind, ref string) (bool, error) {
	found := false
	err := s.e.View(func(txn *badger.Txn) error {
		_, err := txn.Get(sourceKey(userID, kind, ref))
		if errors.Is(err, badger.ErrKeyNotFound) {
			return nil
		}
		found = err == nil
		return err
	})
	return found, err
}

func (s *Store) Get(_ context.Context, userID string, id uuid.UUID) (Draft, error) {
	var d Draft
	err := s.e.View(func(txn *badger.Txn) error {
		return storage.GetJSON(txn, itemKey(userID, id), &d)
	})
	return d, err
}

// List returns drafts newest first; an empty status returns all of them.
func (s *Store) List(_ context.Context, userID, status string) ([]Draft, error) {
	drafts := []Draft{}
	err := s.e.View(func(txn *badger.Txn) error {
		return storage.IteratePrefix(txn, itemPrefix(userID), func(_, val []byte) error {
			var d Draft
			if err := json.Unmarshal(val, &d); err != nil {
				return err
			}
			if status == "" || d.Status == status {
				drafts = append(drafts, d)
			}
			return nil
		})
	})
	if err != nil {
		return nil, err
	}
	slices.SortFunc(drafts, func(a, b Draft) int {
		if c := b.CreatedAt.Compare(a.CreatedAt); c != 0 {
			return c
		}
		return strings.Compare(a.ID.String(), b.ID.String())
	})
	return drafts, nil
}

// Update applies fn to a pending draft and stores the result.
func (s *Store) Update(_ context.Context, userID string, id uuid.UUID, fn func(*Draft)) (Draft, error) {
	var d Draft
	err := s.e.Update(func(txn *badger.Txn) error {
		if err := storage.GetJSON(txn, itemKey(userID, id), &d); err != nil {
			return err
		}
		if d.Status != StatusPending {
			return ErrNotPending
		}
		fn(&d)
		d.UpdatedAt = time.Now().UTC()
		return storage.SetJSON(txn, itemKey(userID, id), d)
	})
	return d, err
}

// Decide moves a pending draft to accepted or rejected and frees its source.
func (s *Store) Decide(_ context.Context, userID string, id uuid.UUID, status string, result *Result) (Draft, error) {
	if status != StatusAccepted && status != StatusRejected {
		return Draft{}, fmt.Errorf("invalid decision %q", status)
	}
	var d Draft
	err := s.e.Update(func(txn *badger.Txn) error {
		if err := storage.GetJSON(txn, itemKey(userID, id), &d); err != nil {
			return err
		}
		if d.Status != StatusPending {
			return ErrNotPending
		}
		now := time.Now().UTC()
		d.Status = status
		d.Result = result
		d.DecidedAt = &now
		d.UpdatedAt = now
		if err := storage.SetJSON(txn, itemKey(userID, id), d); err != nil {
			return err
		}
		err := txn.Delete(sourceKey(userID, d.Source.Kind, d.Source.Ref))
		if errors.Is(err, badger.ErrKeyNotFound) {
			return nil
		}
		return err
	})
	return d, err
}
