package contracts

import (
	"context"
	"log/slog"
	"slices"
	"testing"

	"github.com/dgraph-io/badger/v4"
	"github.com/google/uuid"
	"github.com/reusing-code/kontor/backend/internal/categories"
	"github.com/reusing-code/kontor/backend/internal/storage"
	"github.com/reusing-code/kontor/backend/internal/storage/link"
)

type removedRefs struct {
	transactionIDs []uuid.UUID
	refType        string
	targetID       uuid.UUID
}

// fakeTxnSide records what the store asks the ledger side to strip.
type fakeTxnSide struct{ removed []removedRefs }

func (f *fakeTxnSide) RemoveReferences(_ *badger.Txn, _ string, transactionIDs []uuid.UUID, refType string, targetID uuid.UUID) error {
	f.removed = append(f.removed, removedRefs{transactionIDs, refType, targetID})
	return nil
}

func (f *fakeTxnSide) TransactionExists(*badger.Txn, string, uuid.UUID) bool { return true }

func newLinkedStoreEnv(t *testing.T) (*Store, *categories.Store, *fakeTxnSide) {
	t.Helper()
	engine, err := storage.Open(t.TempDir(), slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatalf("opening engine: %v", err)
	}
	t.Cleanup(func() { engine.Close() })
	side := &fakeTxnSide{}
	links := link.NewRegistry()
	links.SetTransactionSide(side)
	store := NewStore(engine, links)
	catStore := categories.NewStore(engine)
	catStore.RegisterCascade(ModuleID, store.CategoryCascade)
	return store, catStore, side
}

func addTestLink(t *testing.T, s *Store, contractID, transactionID uuid.UUID) {
	t.Helper()
	if err := s.e.Update(func(txn *badger.Txn) error {
		return s.AddLink(txn, storeTestUser, contractID, transactionID)
	}); err != nil {
		t.Fatalf("AddLink: %v", err)
	}
}

func TestUpdateContract_KeepsLinksAddedMeanwhile(t *testing.T) {
	s, cats, _ := newLinkedStoreEnv(t)
	ctx := context.Background()
	cat := makeTestCategory("Cat")
	mustCreateCategory(t, cats, cat)
	con := makeTestContract(cat.ID, "Phone")
	mustCreateContract(t, s, con)

	stale, err := s.Get(ctx, storeTestUser, con.ID)
	if err != nil {
		t.Fatal(err)
	}
	txnID := uuid.New()
	addTestLink(t, s, con.ID, txnID)

	stale.Name = "Phone 2"
	if err := s.Update(ctx, storeTestUser, stale); err != nil {
		t.Fatalf("Update: %v", err)
	}
	got, err := s.Get(ctx, storeTestUser, con.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "Phone 2" {
		t.Errorf("Name = %q, want Phone 2", got.Name)
	}
	if !slices.Equal(got.LinkedTransactionIDs, []uuid.UUID{txnID}) {
		t.Errorf("LinkedTransactionIDs = %v, want [%s]", got.LinkedTransactionIDs, txnID)
	}
}

func TestDeleteCategory_RemovesLedgerReferences(t *testing.T) {
	s, cats, side := newLinkedStoreEnv(t)
	cat := makeTestCategory("Cat")
	mustCreateCategory(t, cats, cat)
	con := makeTestContract(cat.ID, "Phone")
	mustCreateContract(t, s, con)
	txnID := uuid.New()
	addTestLink(t, s, con.ID, txnID)

	if err := cats.Delete(context.Background(), storeTestUser, ModuleID, cat.ID); err != nil {
		t.Fatalf("deleting category: %v", err)
	}
	want := []removedRefs{{[]uuid.UUID{txnID}, link.RefContract, con.ID}}
	if len(side.removed) != 1 || !slices.Equal(side.removed[0].transactionIDs, want[0].transactionIDs) ||
		side.removed[0].refType != want[0].refType || side.removed[0].targetID != want[0].targetID {
		t.Errorf("removed references = %+v, want %+v", side.removed, want)
	}
}
