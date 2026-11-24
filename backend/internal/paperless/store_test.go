package paperless

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/reusing-code/kontor/backend/internal/storage"
)

const storeTestUser = "test-user"

func newTestStore(t *testing.T) *Store {
	t.Helper()
	e, err := storage.Open(t.TempDir(), slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatalf("opening engine: %v", err)
	}
	t.Cleanup(func() { e.Close() })
	return NewStore(e)
}

func makeLink(entityType, entityID string, docID int) Link {
	return Link{
		EntityType: entityType,
		EntityID:   entityID,
		DocumentID: docID,
		Title:      "Invoice",
		EntityURL:  "https://kontor.example/x",
		CreatedAt:  time.Now().UTC(),
	}
}

func TestConfigRoundtrip(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	_, err := s.GetConfig(ctx, storeTestUser)
	if !errors.Is(err, storage.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}

	cfg := Config{
		BaseURL:        "https://paperless.example",
		EncryptedToken: "encrypted-token",
		CustomFieldID:  7,
		UpdatedAt:      time.Now().UTC(),
	}
	if err := s.PutConfig(ctx, storeTestUser, cfg); err != nil {
		t.Fatalf("PutConfig: %v", err)
	}

	got, err := s.GetConfig(ctx, storeTestUser)
	if err != nil {
		t.Fatalf("GetConfig: %v", err)
	}
	if got.BaseURL != cfg.BaseURL {
		t.Errorf("BaseURL = %q, want %q", got.BaseURL, cfg.BaseURL)
	}
	if got.EncryptedToken != cfg.EncryptedToken {
		t.Errorf("EncryptedToken = %q, want %q — encrypted token must survive persistence", got.EncryptedToken, cfg.EncryptedToken)
	}
	if got.CustomFieldID != cfg.CustomFieldID {
		t.Errorf("CustomFieldID = %d, want %d", got.CustomFieldID, cfg.CustomFieldID)
	}
}

func TestDeleteConfig(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	if err := s.DeleteConfig(ctx, storeTestUser); err != nil {
		t.Fatalf("DeleteConfig on missing config: %v", err)
	}

	if err := s.PutConfig(ctx, storeTestUser, Config{BaseURL: "https://p.example", EncryptedToken: "t"}); err != nil {
		t.Fatalf("PutConfig: %v", err)
	}
	if err := s.DeleteConfig(ctx, storeTestUser); err != nil {
		t.Fatalf("DeleteConfig: %v", err)
	}
	_, err := s.GetConfig(ctx, storeTestUser)
	if !errors.Is(err, storage.ErrNotFound) {
		t.Fatalf("expected ErrNotFound after delete, got %v", err)
	}
}

func TestLinksRoundtrip(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	entityID := uuid.NewString()

	links, err := s.ListLinks(ctx, storeTestUser, EntityContract, entityID)
	if err != nil {
		t.Fatalf("ListLinks: %v", err)
	}
	if len(links) != 0 {
		t.Fatalf("expected empty list, got %d", len(links))
	}

	for _, docID := range []int{1, 2, 3} {
		if err := s.PutLink(ctx, storeTestUser, makeLink(EntityContract, entityID, docID)); err != nil {
			t.Fatalf("PutLink(%d): %v", docID, err)
		}
	}

	links, err = s.ListLinks(ctx, storeTestUser, EntityContract, entityID)
	if err != nil {
		t.Fatalf("ListLinks: %v", err)
	}
	if len(links) != 3 {
		t.Fatalf("expected 3 links, got %d", len(links))
	}

	deleted, err := s.DeleteLink(ctx, storeTestUser, EntityContract, entityID, 2)
	if err != nil {
		t.Fatalf("DeleteLink: %v", err)
	}
	if deleted.DocumentID != 2 || deleted.EntityURL != "https://kontor.example/x" {
		t.Errorf("deleted link = %+v", deleted)
	}

	links, err = s.ListLinks(ctx, storeTestUser, EntityContract, entityID)
	if err != nil {
		t.Fatalf("ListLinks: %v", err)
	}
	if len(links) != 2 {
		t.Fatalf("expected 2 links after delete, got %d", len(links))
	}
}

func TestDeleteLink_NotFound(t *testing.T) {
	s := newTestStore(t)
	_, err := s.DeleteLink(context.Background(), storeTestUser, EntityContract, uuid.NewString(), 99)
	if !errors.Is(err, storage.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestLinksEntityIsolation(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	entityID := uuid.NewString()

	if err := s.PutLink(ctx, storeTestUser, makeLink(EntityContract, entityID, 1)); err != nil {
		t.Fatalf("PutLink: %v", err)
	}

	links, err := s.ListLinks(ctx, storeTestUser, EntityPurchase, entityID)
	if err != nil {
		t.Fatalf("ListLinks: %v", err)
	}
	if len(links) != 0 {
		t.Errorf("purchase entity should see 0 links, got %d", len(links))
	}

	links, err = s.ListLinks(ctx, "other-user", EntityContract, entityID)
	if err != nil {
		t.Fatalf("ListLinks: %v", err)
	}
	if len(links) != 0 {
		t.Errorf("other user should see 0 links, got %d", len(links))
	}
}

func TestListLinksByType_TypeAndUserIsolation(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	costID := uuid.NewString()
	for _, l := range []Link{
		makeLink(EntityCost, costID, 1),
		makeLink(EntityContract, uuid.NewString(), 2),
	} {
		if err := s.PutLink(ctx, storeTestUser, l); err != nil {
			t.Fatalf("PutLink: %v", err)
		}
	}
	if err := s.PutLink(ctx, "other-user", makeLink(EntityCost, uuid.NewString(), 3)); err != nil {
		t.Fatalf("PutLink: %v", err)
	}

	links, err := s.ListLinksByType(ctx, storeTestUser, EntityCost)
	if err != nil {
		t.Fatalf("ListLinksByType: %v", err)
	}
	if len(links) != 1 || links[0].EntityID != costID || links[0].DocumentID != 1 {
		t.Errorf("cost links = %+v", links)
	}

	links, err = s.ListLinksByType(ctx, storeTestUser, EntityPurchase)
	if err != nil {
		t.Fatalf("ListLinksByType: %v", err)
	}
	if links == nil || len(links) != 0 {
		t.Errorf("purchase links = %#v, want empty non-nil", links)
	}
}

func TestListAllAndImportLinks(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	source := []Link{
		makeLink(EntityContract, uuid.NewString(), 1),
		makeLink(EntityTransaction, uuid.NewString(), 2),
	}
	if err := s.ImportLinks(ctx, storeTestUser, source); err != nil {
		t.Fatalf("ImportLinks: %v", err)
	}

	all, err := s.ListAllLinks(ctx, storeTestUser)
	if err != nil {
		t.Fatalf("ListAllLinks: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("expected 2 links, got %d", len(all))
	}
}
