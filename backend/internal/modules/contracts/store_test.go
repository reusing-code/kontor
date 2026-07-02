package contracts

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/reusing-code/kontor/backend/internal/categories"
	"github.com/reusing-code/kontor/backend/internal/storage"
	"github.com/reusing-code/kontor/backend/internal/storage/link"
)

const storeTestUser = "test-user"

func newStoreEnv(t *testing.T) (*Store, *categories.Store) {
	t.Helper()
	engine, err := storage.Open(t.TempDir(), slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatalf("opening engine: %v", err)
	}
	t.Cleanup(func() { engine.Close() })
	store := NewStore(engine, link.NewRegistry())
	catStore := categories.NewStore(engine)
	catStore.RegisterCascade(ModuleID, store.CategoryCascade)
	return store, catStore
}

func makeTestCategory(name string) categories.Category {
	now := time.Now().UTC()
	return categories.Category{
		ID:        uuid.New(),
		Name:      name,
		CreatedAt: now,
		UpdatedAt: now,
	}
}

func makeTestContract(categoryID uuid.UUID, name string) Contract {
	now := time.Now().UTC()
	return Contract{
		ID:         uuid.New(),
		CategoryID: categoryID,
		Name:       name,
		StartDate:  "2025-01-01",
		CreatedAt:  now,
		UpdatedAt:  now,
	}
}

func mustCreateCategory(t *testing.T, cats *categories.Store, cat categories.Category) {
	t.Helper()
	if err := cats.Create(context.Background(), storeTestUser, ModuleID, cat); err != nil {
		t.Fatalf("creating category: %v", err)
	}
}

func mustCreateContract(t *testing.T, s *Store, con Contract) {
	t.Helper()
	if err := s.Create(context.Background(), storeTestUser, con); err != nil {
		t.Fatalf("creating contract: %v", err)
	}
}

func TestCreateAndGetContract(t *testing.T) {
	s, cats := newStoreEnv(t)
	ctx := context.Background()
	cat := makeTestCategory("Cat")
	mustCreateCategory(t, cats, cat)

	con := makeTestContract(cat.ID, "Phone Plan")
	if err := s.Create(ctx, storeTestUser, con); err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := s.Get(ctx, storeTestUser, con.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Name != con.Name {
		t.Errorf("Name = %q, want %q", got.Name, con.Name)
	}
	if got.CategoryID != cat.ID {
		t.Errorf("CategoryID = %s, want %s", got.CategoryID, cat.ID)
	}
}

func TestListContracts(t *testing.T) {
	s, cats := newStoreEnv(t)
	ctx := context.Background()
	cat := makeTestCategory("Cat")
	mustCreateCategory(t, cats, cat)

	for _, name := range []string{"A", "B"} {
		mustCreateContract(t, s, makeTestContract(cat.ID, name))
	}

	all, err := s.List(ctx, storeTestUser)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("expected 2 contracts, got %d", len(all))
	}
}

func TestListContractsByCategory(t *testing.T) {
	s, cats := newStoreEnv(t)
	ctx := context.Background()

	cat1 := makeTestCategory("Cat1")
	cat2 := makeTestCategory("Cat2")
	mustCreateCategory(t, cats, cat1)
	mustCreateCategory(t, cats, cat2)

	mustCreateContract(t, s, makeTestContract(cat1.ID, "C1"))
	mustCreateContract(t, s, makeTestContract(cat1.ID, "C2"))
	mustCreateContract(t, s, makeTestContract(cat2.ID, "C3"))

	list, err := s.ListByCategory(ctx, storeTestUser, cat1.ID)
	if err != nil {
		t.Fatalf("ListByCategory: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("expected 2 contracts for cat1, got %d", len(list))
	}

	list, err = s.ListByCategory(ctx, storeTestUser, cat2.ID)
	if err != nil {
		t.Fatalf("ListByCategory: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 contract for cat2, got %d", len(list))
	}
}

func TestUpdateContract(t *testing.T) {
	s, cats := newStoreEnv(t)
	ctx := context.Background()
	cat := makeTestCategory("Cat")
	mustCreateCategory(t, cats, cat)

	con := makeTestContract(cat.ID, "Old")
	mustCreateContract(t, s, con)

	con.Name = "New"
	con.UpdatedAt = time.Now().UTC()
	if err := s.Update(ctx, storeTestUser, con); err != nil {
		t.Fatalf("Update: %v", err)
	}

	got, err := s.Get(ctx, storeTestUser, con.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Name != "New" {
		t.Errorf("Name = %q, want %q", got.Name, "New")
	}
}

func TestDeleteContract(t *testing.T) {
	s, cats := newStoreEnv(t)
	ctx := context.Background()
	cat := makeTestCategory("Cat")
	mustCreateCategory(t, cats, cat)

	con := makeTestContract(cat.ID, "ToDelete")
	mustCreateContract(t, s, con)

	if err := s.Delete(ctx, storeTestUser, con.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	_, err := s.Get(ctx, storeTestUser, con.ID)
	if !errors.Is(err, storage.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestStoreGetContract_NotFound(t *testing.T) {
	s, _ := newStoreEnv(t)
	_, err := s.Get(context.Background(), storeTestUser, uuid.New())
	if !errors.Is(err, storage.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestStoreUpdateContract_NotFound(t *testing.T) {
	s, cats := newStoreEnv(t)
	cat := makeTestCategory("Cat")
	mustCreateCategory(t, cats, cat)
	err := s.Update(context.Background(), storeTestUser, makeTestContract(cat.ID, "Ghost"))
	if !errors.Is(err, storage.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestStoreDeleteContract_NotFound(t *testing.T) {
	s, _ := newStoreEnv(t)
	err := s.Delete(context.Background(), storeTestUser, uuid.New())
	if !errors.Is(err, storage.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestDeleteCategory_CascadesContracts(t *testing.T) {
	s, cats := newStoreEnv(t)
	ctx := context.Background()
	cat := makeTestCategory("Cat")
	mustCreateCategory(t, cats, cat)

	con1 := makeTestContract(cat.ID, "C1")
	con2 := makeTestContract(cat.ID, "C2")
	mustCreateContract(t, s, con1)
	mustCreateContract(t, s, con2)

	if err := cats.Delete(ctx, storeTestUser, ModuleID, cat.ID); err != nil {
		t.Fatalf("deleting category: %v", err)
	}

	// Both contracts should be gone
	for _, id := range []uuid.UUID{con1.ID, con2.ID} {
		_, err := s.Get(ctx, storeTestUser, id)
		if !errors.Is(err, storage.ErrNotFound) {
			t.Errorf("contract %s: expected ErrNotFound, got %v", id, err)
		}
	}

	// Index should be clean
	list, err := s.ListByCategory(ctx, storeTestUser, cat.ID)
	if err != nil {
		t.Fatalf("ListByCategory: %v", err)
	}
	if len(list) != 0 {
		t.Errorf("expected 0 contracts after cascade, got %d", len(list))
	}
}

func TestUpdateContract_CategoryChange_UpdatesIndex(t *testing.T) {
	s, cats := newStoreEnv(t)
	ctx := context.Background()

	cat1 := makeTestCategory("Cat1")
	cat2 := makeTestCategory("Cat2")
	mustCreateCategory(t, cats, cat1)
	mustCreateCategory(t, cats, cat2)

	con := makeTestContract(cat1.ID, "Moveable")
	mustCreateContract(t, s, con)

	con.CategoryID = cat2.ID
	con.UpdatedAt = time.Now().UTC()
	if err := s.Update(ctx, storeTestUser, con); err != nil {
		t.Fatalf("Update: %v", err)
	}

	list1, _ := s.ListByCategory(ctx, storeTestUser, cat1.ID)
	list2, _ := s.ListByCategory(ctx, storeTestUser, cat2.ID)

	if len(list1) != 0 {
		t.Errorf("cat1 should have 0 contracts, got %d", len(list1))
	}
	if len(list2) != 1 {
		t.Errorf("cat2 should have 1 contract, got %d", len(list2))
	}
}

func TestDeleteContract_CleansIndex(t *testing.T) {
	s, cats := newStoreEnv(t)
	ctx := context.Background()

	cat := makeTestCategory("Cat")
	mustCreateCategory(t, cats, cat)

	con := makeTestContract(cat.ID, "C")
	mustCreateContract(t, s, con)
	if err := s.Delete(ctx, storeTestUser, con.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	list, _ := s.ListByCategory(ctx, storeTestUser, cat.ID)
	if len(list) != 0 {
		t.Errorf("expected 0 contracts after delete, got %d", len(list))
	}
}
