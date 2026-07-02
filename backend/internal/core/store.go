package core

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/dgraph-io/badger/v4"
	"github.com/google/uuid"
	"github.com/reusing-code/kontor/backend/internal/storage"
)

type Store struct {
	e *storage.Engine
}

func NewStore(e *storage.Engine) *Store {
	return &Store{e: e}
}

func (s *Store) Healthy() error { return s.e.Healthy() }

func usrKey(id uuid.UUID) []byte {
	return []byte(fmt.Sprintf("usr/%s", id))
}

func usrEmailKey(email string) []byte {
	return []byte(fmt.Sprintf("usr_email/%s", email))
}

func settingsKey(userID string) []byte {
	return []byte(fmt.Sprintf("u/%s/settings", userID))
}

// storableUser includes PasswordHash for persistence (User has json:"-" on it).
type storableUser struct {
	ID           uuid.UUID `json:"id"`
	Email        string    `json:"email"`
	PasswordHash string    `json:"passwordHash"`
	CreatedAt    time.Time `json:"createdAt"`
}

func toStorableUser(u User) storableUser {
	return storableUser(u)
}

func (su storableUser) toModel() User {
	return User(su)
}

func (s *Store) CreateUser(_ context.Context, u User) error {
	return s.e.Update(func(txn *badger.Txn) error {
		if _, err := txn.Get(usrEmailKey(u.Email)); err == nil {
			return storage.ErrConflict
		}
		if err := storage.SetJSON(txn, usrKey(u.ID), toStorableUser(u)); err != nil {
			return err
		}
		return txn.Set(usrEmailKey(u.Email), []byte(u.ID.String()))
	})
}

func (s *Store) GetUserByID(_ context.Context, id string) (User, error) {
	uid, err := uuid.Parse(id)
	if err != nil {
		return User{}, storage.ErrNotFound
	}
	var su storableUser
	err = s.e.View(func(txn *badger.Txn) error {
		return storage.GetJSON(txn, usrKey(uid), &su)
	})
	return su.toModel(), err
}

func (s *Store) GetUserByEmail(_ context.Context, email string) (User, error) {
	var su storableUser
	err := s.e.View(func(txn *badger.Txn) error {
		item, err := txn.Get(usrEmailKey(email))
		if err != nil {
			return storage.ErrNotFound
		}
		var idStr string
		if err := item.Value(func(val []byte) error {
			idStr = string(val)
			return nil
		}); err != nil {
			return err
		}
		id, err := uuid.Parse(idStr)
		if err != nil {
			return err
		}
		return storage.GetJSON(txn, usrKey(id), &su)
	})
	return su.toModel(), err
}

func (s *Store) UpdateUser(_ context.Context, u User) error {
	return s.e.Update(func(txn *badger.Txn) error {
		var existing storableUser
		if err := storage.GetJSON(txn, usrKey(u.ID), &existing); err != nil {
			return err
		}
		return storage.SetJSON(txn, usrKey(u.ID), toStorableUser(u))
	})
}

func (s *Store) ListUsers(_ context.Context) ([]User, error) {
	users := []User{}
	err := s.e.View(func(txn *badger.Txn) error {
		return storage.IteratePrefix(txn, []byte("usr/"), func(_, val []byte) error {
			var su storableUser
			if err := json.Unmarshal(val, &su); err != nil {
				return err
			}
			users = append(users, su.toModel())
			return nil
		})
	})
	if err != nil {
		return nil, err
	}
	return users, nil
}

func (s *Store) GetSettings(_ context.Context, userID string) (UserSettings, error) {
	var settings UserSettings
	err := s.e.View(func(txn *badger.Txn) error {
		return storage.GetJSON(txn, settingsKey(userID), &settings)
	})
	if errors.Is(err, storage.ErrNotFound) {
		return DefaultUserSettings(), nil
	}
	return settings, err
}

func (s *Store) UpdateSettings(_ context.Context, userID string, st UserSettings) error {
	return s.e.Update(func(txn *badger.Txn) error {
		return storage.SetJSON(txn, settingsKey(userID), st)
	})
}
