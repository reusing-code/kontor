package store

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/reusing-code/kontor/backend/internal/model"
	"github.com/reusing-code/kontor/backend/internal/modules/auto"
	"github.com/reusing-code/kontor/backend/internal/modules/contracts"
	"github.com/reusing-code/kontor/backend/internal/storage"
	"github.com/reusing-code/kontor/backend/internal/storage/link"
)

const testUser = "test-user"
const testModule = "contracts"

func newTestStore(t *testing.T) *BadgerStore {
	t.Helper()
	logger := slog.New(slog.DiscardHandler)
	engine, err := storage.Open(t.TempDir(), logger)
	if err != nil {
		t.Fatalf("opening engine: %v", err)
	}
	t.Cleanup(func() { engine.Close() })
	links := link.NewRegistry()
	return New(engine, links, contracts.NewStore(engine, links), auto.NewStore(engine, links), logger)
}

func makeCategory(name string) model.Category {
	now := time.Now().UTC()
	return model.Category{
		ID:        uuid.New(),
		Name:      name,
		CreatedAt: now,
		UpdatedAt: now,
	}
}

func makeContract(categoryID uuid.UUID, name string) model.Contract {
	now := time.Now().UTC()
	return model.Contract{
		ID:         uuid.New(),
		CategoryID: categoryID,
		Name:       name,
		StartDate:  "2025-01-01",
		CreatedAt:  now,
		UpdatedAt:  now,
	}
}

func makePurchase(categoryID uuid.UUID, name string) model.Purchase {
	now := time.Now().UTC()
	return model.Purchase{
		ID:         uuid.New(),
		CategoryID: categoryID,
		ItemName:   name,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
}

// Category CRUD

func TestCreateAndGetCategory(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	cat := makeCategory("Insurance")

	if err := s.CreateCategory(ctx, testUser, testModule, cat); err != nil {
		t.Fatalf("CreateCategory: %v", err)
	}

	got, err := s.GetCategory(ctx, testUser, testModule, cat.ID)
	if err != nil {
		t.Fatalf("GetCategory: %v", err)
	}
	if got.Name != cat.Name {
		t.Errorf("Name = %q, want %q", got.Name, cat.Name)
	}
}

func TestListCategories(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	cats, err := s.ListCategories(ctx, testUser, testModule)
	if err != nil {
		t.Fatalf("ListCategories: %v", err)
	}
	if len(cats) != 0 {
		t.Fatalf("expected empty list, got %d", len(cats))
	}

	for _, name := range []string{"A", "B", "C"} {
		if err := s.CreateCategory(ctx, testUser, testModule, makeCategory(name)); err != nil {
			t.Fatalf("CreateCategory(%s): %v", name, err)
		}
	}

	cats, err = s.ListCategories(ctx, testUser, testModule)
	if err != nil {
		t.Fatalf("ListCategories: %v", err)
	}
	if len(cats) != 3 {
		t.Fatalf("expected 3 categories, got %d", len(cats))
	}
}

func TestUpdateCategory(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	cat := makeCategory("Old")

	if err := s.CreateCategory(ctx, testUser, testModule, cat); err != nil {
		t.Fatalf("CreateCategory: %v", err)
	}

	cat.Name = "New"
	cat.UpdatedAt = time.Now().UTC()
	if err := s.UpdateCategory(ctx, testUser, testModule, cat); err != nil {
		t.Fatalf("UpdateCategory: %v", err)
	}

	got, err := s.GetCategory(ctx, testUser, testModule, cat.ID)
	if err != nil {
		t.Fatalf("GetCategory: %v", err)
	}
	if got.Name != "New" {
		t.Errorf("Name = %q, want %q", got.Name, "New")
	}
}

func TestDeleteCategory(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	cat := makeCategory("ToDelete")

	if err := s.CreateCategory(ctx, testUser, testModule, cat); err != nil {
		t.Fatalf("CreateCategory: %v", err)
	}
	if err := s.DeleteCategory(ctx, testUser, testModule, cat.ID); err != nil {
		t.Fatalf("DeleteCategory: %v", err)
	}

	_, err := s.GetCategory(ctx, testUser, testModule, cat.ID)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestGetCategory_NotFound(t *testing.T) {
	s := newTestStore(t)
	_, err := s.GetCategory(context.Background(), testUser, testModule, uuid.New())
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestUpdateCategory_NotFound(t *testing.T) {
	s := newTestStore(t)
	err := s.UpdateCategory(context.Background(), testUser, testModule, makeCategory("Ghost"))
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestDeleteCategory_NotFound(t *testing.T) {
	s := newTestStore(t)
	err := s.DeleteCategory(context.Background(), testUser, testModule, uuid.New())
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

// Module isolation for categories

func TestCategoryModuleIsolation(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	cat := makeCategory("Insurance")
	s.CreateCategory(ctx, testUser, "contracts", cat)

	// Should not be visible under "purchases" module
	cats, _ := s.ListCategories(ctx, testUser, "purchases")
	if len(cats) != 0 {
		t.Errorf("purchases module should see 0 categories, got %d", len(cats))
	}

	_, err := s.GetCategory(ctx, testUser, "purchases", cat.ID)
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("purchases module should get ErrNotFound, got %v", err)
	}
}

// Contract CRUD

func TestCreateAndGetContract(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	cat := makeCategory("Cat")
	s.CreateCategory(ctx, testUser, testModule, cat)

	con := makeContract(cat.ID, "Phone Plan")
	if err := s.CreateContract(ctx, testUser, con); err != nil {
		t.Fatalf("CreateContract: %v", err)
	}

	got, err := s.GetContract(ctx, testUser, con.ID)
	if err != nil {
		t.Fatalf("GetContract: %v", err)
	}
	if got.Name != con.Name {
		t.Errorf("Name = %q, want %q", got.Name, con.Name)
	}
	if got.CategoryID != cat.ID {
		t.Errorf("CategoryID = %s, want %s", got.CategoryID, cat.ID)
	}
}

func TestListContracts(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	cat := makeCategory("Cat")
	s.CreateCategory(ctx, testUser, testModule, cat)

	for _, name := range []string{"A", "B"} {
		s.CreateContract(ctx, testUser, makeContract(cat.ID, name))
	}

	all, err := s.ListContracts(ctx, testUser)
	if err != nil {
		t.Fatalf("ListContracts: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("expected 2 contracts, got %d", len(all))
	}
}

func TestListContractsByCategory(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	cat1 := makeCategory("Cat1")
	cat2 := makeCategory("Cat2")
	s.CreateCategory(ctx, testUser, testModule, cat1)
	s.CreateCategory(ctx, testUser, testModule, cat2)

	s.CreateContract(ctx, testUser, makeContract(cat1.ID, "C1"))
	s.CreateContract(ctx, testUser, makeContract(cat1.ID, "C2"))
	s.CreateContract(ctx, testUser, makeContract(cat2.ID, "C3"))

	list, err := s.ListContractsByCategory(ctx, testUser, cat1.ID)
	if err != nil {
		t.Fatalf("ListContractsByCategory: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("expected 2 contracts for cat1, got %d", len(list))
	}

	list, err = s.ListContractsByCategory(ctx, testUser, cat2.ID)
	if err != nil {
		t.Fatalf("ListContractsByCategory: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 contract for cat2, got %d", len(list))
	}
}

func TestUpdateContract(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	cat := makeCategory("Cat")
	s.CreateCategory(ctx, testUser, testModule, cat)

	con := makeContract(cat.ID, "Old")
	s.CreateContract(ctx, testUser, con)

	con.Name = "New"
	con.UpdatedAt = time.Now().UTC()
	if err := s.UpdateContract(ctx, testUser, con); err != nil {
		t.Fatalf("UpdateContract: %v", err)
	}

	got, err := s.GetContract(ctx, testUser, con.ID)
	if err != nil {
		t.Fatalf("GetContract: %v", err)
	}
	if got.Name != "New" {
		t.Errorf("Name = %q, want %q", got.Name, "New")
	}
}

func TestDeleteContract(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	cat := makeCategory("Cat")
	s.CreateCategory(ctx, testUser, testModule, cat)

	con := makeContract(cat.ID, "ToDelete")
	s.CreateContract(ctx, testUser, con)

	if err := s.DeleteContract(ctx, testUser, con.ID); err != nil {
		t.Fatalf("DeleteContract: %v", err)
	}

	_, err := s.GetContract(ctx, testUser, con.ID)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestGetContract_NotFound(t *testing.T) {
	s := newTestStore(t)
	_, err := s.GetContract(context.Background(), testUser, uuid.New())
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestUpdateContract_NotFound(t *testing.T) {
	s := newTestStore(t)
	cat := makeCategory("Cat")
	s.CreateCategory(context.Background(), testUser, testModule, cat)
	err := s.UpdateContract(context.Background(), testUser, makeContract(cat.ID, "Ghost"))
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestDeleteContract_NotFound(t *testing.T) {
	s := newTestStore(t)
	err := s.DeleteContract(context.Background(), testUser, uuid.New())
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

// Cascade delete

func TestDeleteCategory_CascadesContracts(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	cat := makeCategory("Cat")
	s.CreateCategory(ctx, testUser, testModule, cat)

	con1 := makeContract(cat.ID, "C1")
	con2 := makeContract(cat.ID, "C2")
	s.CreateContract(ctx, testUser, con1)
	s.CreateContract(ctx, testUser, con2)

	if err := s.DeleteCategory(ctx, testUser, testModule, cat.ID); err != nil {
		t.Fatalf("DeleteCategory: %v", err)
	}

	// Both contracts should be gone
	for _, id := range []uuid.UUID{con1.ID, con2.ID} {
		_, err := s.GetContract(ctx, testUser, id)
		if !errors.Is(err, ErrNotFound) {
			t.Errorf("contract %s: expected ErrNotFound, got %v", id, err)
		}
	}

	// Index should be clean
	list, err := s.ListContractsByCategory(ctx, testUser, cat.ID)
	if err != nil {
		t.Fatalf("ListContractsByCategory: %v", err)
	}
	if len(list) != 0 {
		t.Errorf("expected 0 contracts after cascade, got %d", len(list))
	}
}

func TestDeleteCategory_CascadesPurchases(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	cat := makeCategory("Cat")
	s.CreateCategory(ctx, testUser, "purchases", cat)

	p1 := makePurchase(cat.ID, "Item1")
	p2 := makePurchase(cat.ID, "Item2")
	s.CreatePurchase(ctx, testUser, p1)
	s.CreatePurchase(ctx, testUser, p2)

	if err := s.DeleteCategory(ctx, testUser, "purchases", cat.ID); err != nil {
		t.Fatalf("DeleteCategory: %v", err)
	}

	for _, id := range []uuid.UUID{p1.ID, p2.ID} {
		_, err := s.GetPurchase(ctx, testUser, id)
		if !errors.Is(err, ErrNotFound) {
			t.Errorf("purchase %s: expected ErrNotFound, got %v", id, err)
		}
	}

	list, err := s.ListPurchasesByCategory(ctx, testUser, cat.ID)
	if err != nil {
		t.Fatalf("ListPurchasesByCategory: %v", err)
	}
	if len(list) != 0 {
		t.Errorf("expected 0 purchases after cascade, got %d", len(list))
	}
}

// Index consistency

func TestUpdateContract_CategoryChange_UpdatesIndex(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	cat1 := makeCategory("Cat1")
	cat2 := makeCategory("Cat2")
	s.CreateCategory(ctx, testUser, testModule, cat1)
	s.CreateCategory(ctx, testUser, testModule, cat2)

	con := makeContract(cat1.ID, "Moveable")
	s.CreateContract(ctx, testUser, con)

	con.CategoryID = cat2.ID
	con.UpdatedAt = time.Now().UTC()
	if err := s.UpdateContract(ctx, testUser, con); err != nil {
		t.Fatalf("UpdateContract: %v", err)
	}

	list1, _ := s.ListContractsByCategory(ctx, testUser, cat1.ID)
	list2, _ := s.ListContractsByCategory(ctx, testUser, cat2.ID)

	if len(list1) != 0 {
		t.Errorf("cat1 should have 0 contracts, got %d", len(list1))
	}
	if len(list2) != 1 {
		t.Errorf("cat2 should have 1 contract, got %d", len(list2))
	}
}

func TestDeleteContract_CleansIndex(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	cat := makeCategory("Cat")
	s.CreateCategory(ctx, testUser, testModule, cat)

	con := makeContract(cat.ID, "C")
	s.CreateContract(ctx, testUser, con)
	s.DeleteContract(ctx, testUser, con.ID)

	list, _ := s.ListContractsByCategory(ctx, testUser, cat.ID)
	if len(list) != 0 {
		t.Errorf("expected 0 contracts after delete, got %d", len(list))
	}
}

// Purchase CRUD

func TestCreateAndGetPurchase(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	cat := makeCategory("Cat")
	s.CreateCategory(ctx, testUser, "purchases", cat)

	p := makePurchase(cat.ID, "GPU")
	if err := s.CreatePurchase(ctx, testUser, p); err != nil {
		t.Fatalf("CreatePurchase: %v", err)
	}

	got, err := s.GetPurchase(ctx, testUser, p.ID)
	if err != nil {
		t.Fatalf("GetPurchase: %v", err)
	}
	if got.ItemName != p.ItemName {
		t.Errorf("ItemName = %q, want %q", got.ItemName, p.ItemName)
	}
	if got.CategoryID != cat.ID {
		t.Errorf("CategoryID = %s, want %s", got.CategoryID, cat.ID)
	}
}

func TestListPurchases(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	cat := makeCategory("Cat")
	s.CreateCategory(ctx, testUser, "purchases", cat)

	for _, name := range []string{"A", "B"} {
		s.CreatePurchase(ctx, testUser, makePurchase(cat.ID, name))
	}

	all, err := s.ListPurchases(ctx, testUser)
	if err != nil {
		t.Fatalf("ListPurchases: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("expected 2 purchases, got %d", len(all))
	}
}

func TestListPurchasesByCategory(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	cat1 := makeCategory("Cat1")
	cat2 := makeCategory("Cat2")
	s.CreateCategory(ctx, testUser, "purchases", cat1)
	s.CreateCategory(ctx, testUser, "purchases", cat2)

	s.CreatePurchase(ctx, testUser, makePurchase(cat1.ID, "P1"))
	s.CreatePurchase(ctx, testUser, makePurchase(cat1.ID, "P2"))
	s.CreatePurchase(ctx, testUser, makePurchase(cat2.ID, "P3"))

	list, err := s.ListPurchasesByCategory(ctx, testUser, cat1.ID)
	if err != nil {
		t.Fatalf("ListPurchasesByCategory: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("expected 2 purchases for cat1, got %d", len(list))
	}

	list, err = s.ListPurchasesByCategory(ctx, testUser, cat2.ID)
	if err != nil {
		t.Fatalf("ListPurchasesByCategory: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 purchase for cat2, got %d", len(list))
	}
}

func TestUpdatePurchase(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	cat := makeCategory("Cat")
	s.CreateCategory(ctx, testUser, "purchases", cat)

	p := makePurchase(cat.ID, "Old")
	s.CreatePurchase(ctx, testUser, p)

	p.ItemName = "New"
	p.UpdatedAt = time.Now().UTC()
	if err := s.UpdatePurchase(ctx, testUser, p); err != nil {
		t.Fatalf("UpdatePurchase: %v", err)
	}

	got, err := s.GetPurchase(ctx, testUser, p.ID)
	if err != nil {
		t.Fatalf("GetPurchase: %v", err)
	}
	if got.ItemName != "New" {
		t.Errorf("ItemName = %q, want %q", got.ItemName, "New")
	}
}

func TestDeletePurchase(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	cat := makeCategory("Cat")
	s.CreateCategory(ctx, testUser, "purchases", cat)

	p := makePurchase(cat.ID, "ToDelete")
	s.CreatePurchase(ctx, testUser, p)

	if err := s.DeletePurchase(ctx, testUser, p.ID); err != nil {
		t.Fatalf("DeletePurchase: %v", err)
	}

	_, err := s.GetPurchase(ctx, testUser, p.ID)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestGetPurchase_NotFound(t *testing.T) {
	s := newTestStore(t)
	_, err := s.GetPurchase(context.Background(), testUser, uuid.New())
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestUpdatePurchase_CategoryChange_UpdatesIndex(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	cat1 := makeCategory("Cat1")
	cat2 := makeCategory("Cat2")
	s.CreateCategory(ctx, testUser, "purchases", cat1)
	s.CreateCategory(ctx, testUser, "purchases", cat2)

	p := makePurchase(cat1.ID, "Moveable")
	s.CreatePurchase(ctx, testUser, p)

	p.CategoryID = cat2.ID
	p.UpdatedAt = time.Now().UTC()
	if err := s.UpdatePurchase(ctx, testUser, p); err != nil {
		t.Fatalf("UpdatePurchase: %v", err)
	}

	list1, _ := s.ListPurchasesByCategory(ctx, testUser, cat1.ID)
	list2, _ := s.ListPurchasesByCategory(ctx, testUser, cat2.ID)

	if len(list1) != 0 {
		t.Errorf("cat1 should have 0 purchases, got %d", len(list1))
	}
	if len(list2) != 1 {
		t.Errorf("cat2 should have 1 purchase, got %d", len(list2))
	}
}

// User auth

func TestCreateAndGetUserByEmail(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	user := model.User{
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

	user := model.User{
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
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("expected ErrConflict, got %v", err)
	}
}

func TestGetUserByEmail_NotFound(t *testing.T) {
	s := newTestStore(t)
	_, err := s.GetUserByEmail(context.Background(), "nobody@test.com")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

// User isolation

func TestUserIsolation(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	cat := makeCategory("UserA-Cat")
	s.CreateCategory(ctx, "user-a", testModule, cat)

	cats, _ := s.ListCategories(ctx, "user-b", testModule)
	if len(cats) != 0 {
		t.Errorf("user-b should see 0 categories, got %d", len(cats))
	}

	_, err := s.GetCategory(ctx, "user-b", testModule, cat.ID)
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("user-b should get ErrNotFound, got %v", err)
	}
}
