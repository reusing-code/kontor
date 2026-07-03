package categories

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/dgraph-io/badger/v4"
	"github.com/google/uuid"
	"github.com/reusing-code/kontor/backend/internal/storage"
)

// CascadeFunc deletes a module's items belonging to a category, inside the
// same transaction as the category deletion.
type CascadeFunc func(txn *badger.Txn, userID string, categoryID uuid.UUID) error

type Store struct {
	e        *storage.Engine
	cascades map[string]CascadeFunc
}

func NewStore(e *storage.Engine) *Store {
	return &Store{e: e, cascades: map[string]CascadeFunc{}}
}

// RegisterCascade attaches a module's item cleanup to category deletion.
func (s *Store) RegisterCascade(module string, fn CascadeFunc) {
	s.cascades[module] = fn
}

func Key(userID, module string, id uuid.UUID) []byte {
	return []byte(fmt.Sprintf("u/%s/mod/%s/cat/%s", userID, module, id))
}

func Prefix(userID, module string) []byte {
	return []byte(fmt.Sprintf("u/%s/mod/%s/cat/", userID, module))
}

func (s *Store) List(_ context.Context, userID, module string) ([]Category, error) {
	categories := []Category{}
	err := s.e.View(func(txn *badger.Txn) error {
		return storage.IteratePrefix(txn, Prefix(userID, module), func(_, val []byte) error {
			var cat Category
			if err := json.Unmarshal(val, &cat); err != nil {
				return err
			}
			categories = append(categories, cat)
			return nil
		})
	})
	if err != nil {
		return nil, err
	}
	return categories, nil
}

func (s *Store) Get(_ context.Context, userID, module string, id uuid.UUID) (Category, error) {
	var cat Category
	err := s.e.View(func(txn *badger.Txn) error {
		return storage.GetJSON(txn, Key(userID, module, id), &cat)
	})
	return cat, err
}

func (s *Store) Create(_ context.Context, userID, module string, c Category) error {
	return s.e.Update(func(txn *badger.Txn) error {
		return storage.SetJSON(txn, Key(userID, module, c.ID), c)
	})
}

func (s *Store) Update(_ context.Context, userID, module string, c Category) error {
	return s.e.Update(func(txn *badger.Txn) error {
		var existing Category
		if err := storage.GetJSON(txn, Key(userID, module, c.ID), &existing); err != nil {
			return err
		}
		return storage.SetJSON(txn, Key(userID, module, c.ID), c)
	})
}

func (s *Store) Delete(_ context.Context, userID, module string, id uuid.UUID) error {
	return s.e.Update(func(txn *badger.Txn) error {
		var existing Category
		if err := storage.GetJSON(txn, Key(userID, module, id), &existing); err != nil {
			return err
		}
		if err := txn.Delete(Key(userID, module, id)); err != nil {
			return err
		}
		if cascade, ok := s.cascades[module]; ok {
			return cascade(txn, userID, id)
		}
		return nil
	})
}

// SeedDefaults creates the given defaults if the module has no categories yet.
func (s *Store) SeedDefaults(ctx context.Context, userID, module string, defaults []Default) error {
	existing, err := s.List(ctx, userID, module)
	if err != nil {
		return err
	}
	if len(existing) > 0 {
		return nil
	}
	now := time.Now().UTC()
	for _, d := range defaults {
		cat := Category{
			ID:        uuid.New(),
			Name:      d.Name,
			NameKey:   d.NameKey,
			CreatedAt: now,
			UpdatedAt: now,
		}
		if err := s.Create(ctx, userID, module, cat); err != nil {
			return err
		}
	}
	return nil
}
