package auto

import (
	"context"
	"log/slog"
	"slices"
	"testing"
	"time"

	"github.com/dgraph-io/badger/v4"
	"github.com/google/uuid"
	"github.com/reusing-code/kontor/backend/internal/storage"
	"github.com/reusing-code/kontor/backend/internal/storage/link"
)

const storeTestUser = "test-user"

func TestUpdateVehicle_KeepsLinksAddedMeanwhile(t *testing.T) {
	engine, err := storage.Open(t.TempDir(), slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatalf("opening engine: %v", err)
	}
	t.Cleanup(func() { engine.Close() })
	s := NewStore(engine, link.NewRegistry())
	ctx := context.Background()
	now := time.Now().UTC()
	v := Vehicle{ID: uuid.New(), Name: "Car", CreatedAt: now, UpdatedAt: now}
	if err := s.CreateVehicle(ctx, storeTestUser, v); err != nil {
		t.Fatalf("CreateVehicle: %v", err)
	}

	stale, err := s.GetVehicle(ctx, storeTestUser, v.ID)
	if err != nil {
		t.Fatal(err)
	}
	txnID := uuid.New()
	if err := engine.Update(func(txn *badger.Txn) error {
		return s.AddLink(txn, storeTestUser, v.ID, txnID)
	}); err != nil {
		t.Fatalf("AddLink: %v", err)
	}

	stale.Name = "Car 2"
	if err := s.UpdateVehicle(ctx, storeTestUser, stale); err != nil {
		t.Fatalf("UpdateVehicle: %v", err)
	}
	got, err := s.GetVehicle(ctx, storeTestUser, v.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "Car 2" {
		t.Errorf("Name = %q, want Car 2", got.Name)
	}
	if !slices.Equal(got.LinkedTransactionIDs, []uuid.UUID{txnID}) {
		t.Errorf("LinkedTransactionIDs = %v, want [%s]", got.LinkedTransactionIDs, txnID)
	}
}
