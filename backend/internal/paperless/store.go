package paperless

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/dgraph-io/badger/v4"
	"github.com/reusing-code/kontor/backend/internal/storage"
)

type Store struct {
	e *storage.Engine
}

func NewStore(e *storage.Engine) *Store {
	return &Store{e: e}
}

// storableConfig includes EncryptedToken for persistence (Config has json:"-" on it).
type storableConfig struct {
	BaseURL        string    `json:"baseUrl"`
	EncryptedToken string    `json:"encryptedToken"`
	CustomFieldID  int       `json:"customFieldId,omitempty"`
	UpdatedAt      time.Time `json:"updatedAt"`
}

func configKey(userID string) []byte {
	return []byte(fmt.Sprintf("u/%s/paperless/config", userID))
}

func pipelineKey(userID string) []byte {
	return []byte(fmt.Sprintf("u/%s/paperless/pipeline", userID))
}

func linkKey(userID, entityType, entityID string, documentID int) []byte {
	return []byte(fmt.Sprintf("u/%s/paperless/link/%s/%s/%d", userID, entityType, entityID, documentID))
}

func linkEntityPrefix(userID, entityType, entityID string) []byte {
	return []byte(fmt.Sprintf("u/%s/paperless/link/%s/%s/", userID, entityType, entityID))
}

func linkPrefix(userID string) []byte {
	return []byte(fmt.Sprintf("u/%s/paperless/link/", userID))
}

func (s *Store) GetConfig(_ context.Context, userID string) (Config, error) {
	var sc storableConfig
	err := s.e.View(func(txn *badger.Txn) error {
		return storage.GetJSON(txn, configKey(userID), &sc)
	})
	return Config(sc), err
}

func (s *Store) PutConfig(_ context.Context, userID string, cfg Config) error {
	return s.e.Update(func(txn *badger.Txn) error {
		return storage.SetJSON(txn, configKey(userID), storableConfig(cfg))
	})
}

func (s *Store) DeleteConfig(_ context.Context, userID string) error {
	return s.e.Update(func(txn *badger.Txn) error {
		err := txn.Delete(configKey(userID))
		if errors.Is(err, badger.ErrKeyNotFound) {
			return nil
		}
		return err
	})
}

// GetPipelineSettings returns the defaults when nothing is stored.
func (s *Store) GetPipelineSettings(_ context.Context, userID string) (PipelineSettings, error) {
	settings := DefaultPipelineSettings()
	err := s.e.View(func(txn *badger.Txn) error {
		return storage.GetJSON(txn, pipelineKey(userID), &settings)
	})
	if errors.Is(err, storage.ErrNotFound) {
		return DefaultPipelineSettings(), nil
	}
	return settings, err
}

func (s *Store) PutPipelineSettings(_ context.Context, userID string, settings PipelineSettings) error {
	return s.e.Update(func(txn *badger.Txn) error {
		return storage.SetJSON(txn, pipelineKey(userID), settings)
	})
}

// HasDocumentLinks reports whether any entity still links the document.
func (s *Store) HasDocumentLinks(ctx context.Context, userID string, documentID int) (bool, error) {
	links, err := s.ListAllLinks(ctx, userID)
	if err != nil {
		return false, err
	}
	for _, l := range links {
		if l.DocumentID == documentID {
			return true, nil
		}
	}
	return false, nil
}

func (s *Store) ListLinks(_ context.Context, userID, entityType, entityID string) ([]Link, error) {
	links := []Link{}
	err := s.e.View(func(txn *badger.Txn) error {
		return storage.IteratePrefix(txn, linkEntityPrefix(userID, entityType, entityID), func(_, val []byte) error {
			var l Link
			if err := json.Unmarshal(val, &l); err != nil {
				return err
			}
			links = append(links, l)
			return nil
		})
	})
	if err != nil {
		return nil, err
	}
	return links, nil
}

func (s *Store) PutLink(_ context.Context, userID string, l Link) error {
	return s.e.Update(func(txn *badger.Txn) error {
		return storage.SetJSON(txn, linkKey(userID, l.EntityType, l.EntityID, l.DocumentID), l)
	})
}

// DeleteLink removes a link and returns it, so callers can clean up the
// back-link stored in its EntityURL.
func (s *Store) DeleteLink(_ context.Context, userID, entityType, entityID string, documentID int) (Link, error) {
	var l Link
	err := s.e.Update(func(txn *badger.Txn) error {
		key := linkKey(userID, entityType, entityID, documentID)
		if err := storage.GetJSON(txn, key, &l); err != nil {
			return err
		}
		return txn.Delete(key)
	})
	return l, err
}

func (s *Store) ListAllLinks(_ context.Context, userID string) ([]Link, error) {
	links := []Link{}
	err := s.e.View(func(txn *badger.Txn) error {
		return storage.IteratePrefix(txn, linkPrefix(userID), func(_, val []byte) error {
			var l Link
			if err := json.Unmarshal(val, &l); err != nil {
				return err
			}
			links = append(links, l)
			return nil
		})
	})
	if err != nil {
		return nil, err
	}
	return links, nil
}

func (s *Store) ImportLinks(_ context.Context, userID string, links []Link) error {
	return s.e.Update(func(txn *badger.Txn) error {
		for _, l := range links {
			if err := storage.SetJSON(txn, linkKey(userID, l.EntityType, l.EntityID, l.DocumentID), l); err != nil {
				return err
			}
		}
		return nil
	})
}
