package pat

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"sync"
	"time"

	"github.com/dgraph-io/badger/v4"
	"github.com/google/uuid"
	"github.com/reusing-code/kontor/backend/internal/middleware"
	"github.com/reusing-code/kontor/backend/internal/storage"
)

type Store struct {
	engine *storage.Engine
	logger *slog.Logger
	now    func() time.Time

	// touched remembers when each token's last use was last written, so that
	// parallel requests on an idle token cause one write, not one per request.
	// The record's LastUsedAt stays the source of truth after a restart.
	mu      sync.Mutex
	touched map[uuid.UUID]time.Time
}

func NewStore(e *storage.Engine, logger *slog.Logger) *Store {
	return &Store{engine: e, logger: logger, now: time.Now, touched: map[uuid.UUID]time.Time{}}
}

func patPrefix(userID string) []byte { return []byte("u/" + userID + "/pat/") }

func patKey(userID string, id uuid.UUID) []byte {
	return append(patPrefix(userID), id.String()...)
}

func hashKey(hash string) []byte { return []byte("pat_hash/" + hash) }

func hashToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

// Create stores a new token and returns it with the raw token, which is
// shown to the caller exactly once.
func (s *Store) Create(ctx context.Context, userID string, in CreateInput) (Token, string, error) {
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		return Token{}, "", fmt.Errorf("generating token: %w", err)
	}
	raw := middleware.PATPrefix + base64.RawURLEncoding.EncodeToString(secret)
	hash := hashToken(raw)

	tok := Token{
		ID:        uuid.New(),
		Name:      in.Name,
		Scopes:    in.Scopes,
		CreatedAt: s.now().UTC(),
		ExpiresAt: in.ExpiresAt,
	}
	err := s.engine.Update(func(txn *badger.Txn) error {
		count := 0
		if err := storage.IteratePrefix(txn, patPrefix(userID), func(_, _ []byte) error {
			count++
			return nil
		}); err != nil {
			return err
		}
		if count >= MaxPerUser {
			return ErrLimitReached
		}
		if err := storage.SetJSON(txn, patKey(userID, tok.ID), record{Token: tok, Hash: hash}); err != nil {
			return err
		}
		return storage.SetJSON(txn, hashKey(hash), indexEntry{UserID: userID, ID: tok.ID})
	})
	if err != nil {
		return Token{}, "", err
	}
	return tok, raw, nil
}

// List returns the user's tokens, newest first.
func (s *Store) List(_ context.Context, userID string) ([]Token, error) {
	tokens := []Token{}
	err := s.engine.View(func(txn *badger.Txn) error {
		return storage.IteratePrefix(txn, patPrefix(userID), func(_, val []byte) error {
			var rec record
			if err := json.Unmarshal(val, &rec); err != nil {
				return err
			}
			tokens = append(tokens, rec.Token)
			return nil
		})
	})
	if err != nil {
		return nil, err
	}
	slices.SortFunc(tokens, func(a, b Token) int { return b.CreatedAt.Compare(a.CreatedAt) })
	return tokens, nil
}

// Revoke deletes the token and its hash index entry. Another user's ID is
// simply not found.
func (s *Store) Revoke(_ context.Context, userID string, id uuid.UUID) error {
	err := s.engine.Update(func(txn *badger.Txn) error {
		var rec record
		if err := storage.GetJSON(txn, patKey(userID, id), &rec); err != nil {
			return err
		}
		if err := txn.Delete(hashKey(rec.Hash)); err != nil {
			return err
		}
		return txn.Delete(patKey(userID, id))
	})
	if err == nil {
		s.mu.Lock()
		delete(s.touched, id)
		s.mu.Unlock()
	}
	return err
}

// Authenticate resolves a raw token. Unknown, revoked and expired tokens all
// yield middleware.ErrInvalidPAT.
func (s *Store) Authenticate(_ context.Context, raw string) (middleware.PATIdentity, error) {
	var (
		rec    record
		userID string
	)
	err := s.engine.View(func(txn *badger.Txn) error {
		var idx indexEntry
		if err := storage.GetJSON(txn, hashKey(hashToken(raw)), &idx); err != nil {
			return err
		}
		userID = idx.UserID
		return storage.GetJSON(txn, patKey(idx.UserID, idx.ID), &rec)
	})
	if errors.Is(err, storage.ErrNotFound) {
		return middleware.PATIdentity{}, middleware.ErrInvalidPAT
	}
	if err != nil {
		return middleware.PATIdentity{}, err
	}

	now := s.now()
	if rec.ExpiresAt != nil && !now.Before(*rec.ExpiresAt) {
		return middleware.PATIdentity{}, middleware.ErrInvalidPAT
	}
	if rec.LastUsedAt == nil || now.Sub(*rec.LastUsedAt) >= touchThreshold {
		s.touch(userID, rec.ID, now)
	}
	return middleware.PATIdentity{UserID: userID, TokenID: rec.ID.String(), Scopes: rec.Scopes}, nil
}

// touch records the last use. It re-reads the record so a concurrent revoke
// is never resurrected; failures are logged and ignored.
func (s *Store) touch(userID string, id uuid.UUID, now time.Time) {
	s.mu.Lock()
	last, seen := s.touched[id]
	if seen && now.Sub(last) < touchThreshold {
		s.mu.Unlock()
		return
	}
	s.touched[id] = now
	s.mu.Unlock()

	err := s.engine.Update(func(txn *badger.Txn) error {
		var rec record
		if err := storage.GetJSON(txn, patKey(userID, id), &rec); err != nil {
			if errors.Is(err, storage.ErrNotFound) {
				// revoked while this request was in flight: forget the entry
				// that was just inserted so it cannot outlive the token
				s.mu.Lock()
				delete(s.touched, id)
				s.mu.Unlock()
				return nil
			}
			return err
		}
		used := now.UTC()
		rec.LastUsedAt = &used
		return storage.SetJSON(txn, patKey(userID, id), rec)
	})
	// A conflict means another request wrote the timestamp first.
	if err != nil && !errors.Is(err, badger.ErrConflict) {
		s.logger.Warn("pat touch failed", "tokenId", id.String())
	}
}
