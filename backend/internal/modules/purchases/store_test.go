package purchases

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

const testUserID = "test-user"

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

func makeCategory(name string) categories.Category {
	now := time.Now().UTC()
	return categories.Category{
		ID:        uuid.New(),
		Name:      name,
		CreatedAt: now,
		UpdatedAt: now,
	}
}

func makePurchase(categoryID uuid.UUID, name string) Purchase {
	now := time.Now().UTC()
	return Purchase{
		ID:         uuid.New(),
		CategoryID: categoryID,
		ItemName:   name,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
}

func mustCreateCategory(t *testing.T, cats *categories.Store, cat categories.Category) {
	t.Helper()
	if err := cats.Create(context.Background(), testUserID, ModuleID, cat); err != nil {
		t.Fatalf("creating category: %v", err)
	}
}

func mustCreatePurchase(t *testing.T, s *Store, p Purchase) {
	t.Helper()
	if err := s.Create(context.Background(), testUserID, p); err != nil {
		t.Fatalf("creating purchase: %v", err)
	}
}

func TestCreateAndGetPurchase(t *testing.T) {
	s, cats := newStoreEnv(t)
	ctx := context.Background()
	cat := makeCategory("Cat")
	mustCreateCategory(t, cats, cat)

	p := makePurchase(cat.ID, "GPU")
	if err := s.Create(ctx, testUserID, p); err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := s.Get(ctx, testUserID, p.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.ItemName != p.ItemName {
		t.Errorf("ItemName = %q, want %q", got.ItemName, p.ItemName)
	}
	if got.CategoryID != cat.ID {
		t.Errorf("CategoryID = %s, want %s", got.CategoryID, cat.ID)
	}
}

func TestListPurchases(t *testing.T) {
	s, cats := newStoreEnv(t)
	ctx := context.Background()
	cat := makeCategory("Cat")
	mustCreateCategory(t, cats, cat)

	for _, name := range []string{"A", "B"} {
		mustCreatePurchase(t, s, makePurchase(cat.ID, name))
	}

	all, err := s.List(ctx, testUserID)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("expected 2 purchases, got %d", len(all))
	}
}

func TestListPurchasesByCategory(t *testing.T) {
	s, cats := newStoreEnv(t)
	ctx := context.Background()

	cat1 := makeCategory("Cat1")
	cat2 := makeCategory("Cat2")
	mustCreateCategory(t, cats, cat1)
	mustCreateCategory(t, cats, cat2)

	mustCreatePurchase(t, s, makePurchase(cat1.ID, "P1"))
	mustCreatePurchase(t, s, makePurchase(cat1.ID, "P2"))
	mustCreatePurchase(t, s, makePurchase(cat2.ID, "P3"))

	list, err := s.ListByCategory(ctx, testUserID, cat1.ID)
	if err != nil {
		t.Fatalf("ListByCategory: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("expected 2 purchases for cat1, got %d", len(list))
	}

	list, err = s.ListByCategory(ctx, testUserID, cat2.ID)
	if err != nil {
		t.Fatalf("ListByCategory: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 purchase for cat2, got %d", len(list))
	}
}

func TestUpdatePurchase(t *testing.T) {
	s, cats := newStoreEnv(t)
	ctx := context.Background()
	cat := makeCategory("Cat")
	mustCreateCategory(t, cats, cat)

	p := makePurchase(cat.ID, "Old")
	mustCreatePurchase(t, s, p)

	p.ItemName = "New"
	p.UpdatedAt = time.Now().UTC()
	if err := s.Update(ctx, testUserID, p); err != nil {
		t.Fatalf("Update: %v", err)
	}

	got, err := s.Get(ctx, testUserID, p.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.ItemName != "New" {
		t.Errorf("ItemName = %q, want %q", got.ItemName, "New")
	}
}

func TestDeletePurchase(t *testing.T) {
	s, cats := newStoreEnv(t)
	ctx := context.Background()
	cat := makeCategory("Cat")
	mustCreateCategory(t, cats, cat)

	p := makePurchase(cat.ID, "ToDelete")
	mustCreatePurchase(t, s, p)

	if err := s.Delete(ctx, testUserID, p.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	_, err := s.Get(ctx, testUserID, p.ID)
	if !errors.Is(err, storage.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestGetPurchase_NotFound(t *testing.T) {
	s, _ := newStoreEnv(t)
	_, err := s.Get(context.Background(), testUserID, uuid.New())
	if !errors.Is(err, storage.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestUpdatePurchase_CategoryChange_UpdatesIndex(t *testing.T) {
	s, cats := newStoreEnv(t)
	ctx := context.Background()

	cat1 := makeCategory("Cat1")
	cat2 := makeCategory("Cat2")
	mustCreateCategory(t, cats, cat1)
	mustCreateCategory(t, cats, cat2)

	p := makePurchase(cat1.ID, "Moveable")
	mustCreatePurchase(t, s, p)

	p.CategoryID = cat2.ID
	p.UpdatedAt = time.Now().UTC()
	if err := s.Update(ctx, testUserID, p); err != nil {
		t.Fatalf("Update: %v", err)
	}

	list1, _ := s.ListByCategory(ctx, testUserID, cat1.ID)
	list2, _ := s.ListByCategory(ctx, testUserID, cat2.ID)

	if len(list1) != 0 {
		t.Errorf("cat1 should have 0 purchases, got %d", len(list1))
	}
	if len(list2) != 1 {
		t.Errorf("cat2 should have 1 purchase, got %d", len(list2))
	}
}

func TestDeleteCategory_CascadesPurchases(t *testing.T) {
	s, cats := newStoreEnv(t)
	ctx := context.Background()
	cat := makeCategory("Cat")
	mustCreateCategory(t, cats, cat)

	p1 := makePurchase(cat.ID, "Item1")
	p2 := makePurchase(cat.ID, "Item2")
	mustCreatePurchase(t, s, p1)
	mustCreatePurchase(t, s, p2)

	if err := cats.Delete(ctx, testUserID, ModuleID, cat.ID); err != nil {
		t.Fatalf("deleting category: %v", err)
	}

	for _, id := range []uuid.UUID{p1.ID, p2.ID} {
		_, err := s.Get(ctx, testUserID, id)
		if !errors.Is(err, storage.ErrNotFound) {
			t.Errorf("purchase %s: expected ErrNotFound, got %v", id, err)
		}
	}

	list, err := s.ListByCategory(ctx, testUserID, cat.ID)
	if err != nil {
		t.Fatalf("ListByCategory: %v", err)
	}
	if len(list) != 0 {
		t.Errorf("expected 0 purchases after cascade, got %d", len(list))
	}
}
