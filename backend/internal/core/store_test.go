package core

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/reusing-code/kontor/backend/internal/storage"
)

func TestCreateAndGetUserByEmail(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	user := User{
		ID:           uuid.New(),
		Email:        "test@example.com",
		PasswordHash: "$2a$10$fakehashfortest",
		CreatedAt:    time.Now().UTC(),
	}

	if err := s.CreateUser(ctx, user); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	got, err := s.GetUserByEmail(ctx, "test@example.com")
	if err != nil {
		t.Fatalf("GetUserByEmail: %v", err)
	}
	if got.ID != user.ID {
		t.Errorf("ID = %s, want %s", got.ID, user.ID)
	}
	if got.Email != user.Email {
		t.Errorf("Email = %q, want %q", got.Email, user.Email)
	}
	if got.PasswordHash != user.PasswordHash {
		t.Errorf("PasswordHash = %q, want %q", got.PasswordHash, user.PasswordHash)
	}
}

func TestCreateUser_DuplicateEmail(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	user := User{
		ID:           uuid.New(),
		Email:        "dup@example.com",
		PasswordHash: "hash",
		CreatedAt:    time.Now().UTC(),
	}
	if err := s.CreateUser(ctx, user); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	user.ID = uuid.New()
	err := s.CreateUser(ctx, user)
	if !errors.Is(err, storage.ErrConflict) {
		t.Fatalf("expected ErrConflict, got %v", err)
	}
}

func TestGetUserByEmail_NotFound(t *testing.T) {
	s := newTestStore(t)
	_, err := s.GetUserByEmail(context.Background(), "nobody@test.com")
	if !errors.Is(err, storage.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}
