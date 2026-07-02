package categories

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/reusing-code/kontor/backend/internal/storage"
)

const storeTestUser = "test-user"
const storeTestModule = "contracts"

func makeCategory(name string) Category {
	now := time.Now().UTC()
	return Category{
		ID:        uuid.New(),
		Name:      name,
		CreatedAt: now,
		UpdatedAt: now,
	}
}

func TestCreateAndGetCategory(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	cat := makeCategory("Insurance")

	if err := s.Create(ctx, storeTestUser, storeTestModule, cat); err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := s.Get(ctx, storeTestUser, storeTestModule, cat.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Name != cat.Name {
		t.Errorf("Name = %q, want %q", got.Name, cat.Name)
	}
}

func TestListCategories(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	cats, err := s.List(ctx, storeTestUser, storeTestModule)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(cats) != 0 {
		t.Fatalf("expected empty list, got %d", len(cats))
	}

	for _, name := range []string{"A", "B", "C"} {
		if err := s.Create(ctx, storeTestUser, storeTestModule, makeCategory(name)); err != nil {
			t.Fatalf("Create(%s): %v", name, err)
		}
	}

	cats, err = s.List(ctx, storeTestUser, storeTestModule)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(cats) != 3 {
		t.Fatalf("expected 3 categories, got %d", len(cats))
	}
}

func TestUpdateCategory(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	cat := makeCategory("Old")

	if err := s.Create(ctx, storeTestUser, storeTestModule, cat); err != nil {
		t.Fatalf("Create: %v", err)
	}

	cat.Name = "New"
	cat.UpdatedAt = time.Now().UTC()
	if err := s.Update(ctx, storeTestUser, storeTestModule, cat); err != nil {
		t.Fatalf("Update: %v", err)
	}

	got, err := s.Get(ctx, storeTestUser, storeTestModule, cat.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Name != "New" {
		t.Errorf("Name = %q, want %q", got.Name, "New")
	}
}

func TestDeleteCategory(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	cat := makeCategory("ToDelete")

	if err := s.Create(ctx, storeTestUser, storeTestModule, cat); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := s.Delete(ctx, storeTestUser, storeTestModule, cat.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	_, err := s.Get(ctx, storeTestUser, storeTestModule, cat.ID)
	if !errors.Is(err, storage.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestStoreGetCategory_NotFound(t *testing.T) {
	s := newTestStore(t)
	_, err := s.Get(context.Background(), storeTestUser, storeTestModule, uuid.New())
	if !errors.Is(err, storage.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestStoreUpdateCategory_NotFound(t *testing.T) {
	s := newTestStore(t)
	err := s.Update(context.Background(), storeTestUser, storeTestModule, makeCategory("Ghost"))
	if !errors.Is(err, storage.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestStoreDeleteCategory_NotFound(t *testing.T) {
	s := newTestStore(t)
	err := s.Delete(context.Background(), storeTestUser, storeTestModule, uuid.New())
	if !errors.Is(err, storage.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestCategoryModuleIsolation(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	cat := makeCategory("Insurance")
	if err := s.Create(ctx, storeTestUser, "contracts", cat); err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Should not be visible under "purchases" module
	cats, err := s.List(ctx, storeTestUser, "purchases")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(cats) != 0 {
		t.Errorf("purchases module should see 0 categories, got %d", len(cats))
	}

	_, err = s.Get(ctx, storeTestUser, "purchases", cat.ID)
	if !errors.Is(err, storage.ErrNotFound) {
		t.Errorf("purchases module should get ErrNotFound, got %v", err)
	}
}

func TestUserIsolation(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	cat := makeCategory("UserA-Cat")
	if err := s.Create(ctx, "user-a", storeTestModule, cat); err != nil {
		t.Fatalf("Create: %v", err)
	}

	cats, err := s.List(ctx, "user-b", storeTestModule)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(cats) != 0 {
		t.Errorf("user-b should see 0 categories, got %d", len(cats))
	}

	_, err = s.Get(ctx, "user-b", storeTestModule, cat.ID)
	if !errors.Is(err, storage.ErrNotFound) {
		t.Errorf("user-b should get ErrNotFound, got %v", err)
	}
}
