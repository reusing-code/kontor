package store

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/tobi/contracts/backend/internal/model"
)

func TestLedgerAccount_CreateConflictOnIBAN(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	userID := "user-1"
	now := time.Now().UTC()

	first := model.LedgerAccount{ID: uuid.New(), Name: "A", Bank: "DKB", IBAN: "DE111", Currency: "EUR", CreatedAt: now, UpdatedAt: now}
	second := model.LedgerAccount{ID: uuid.New(), Name: "B", Bank: "DKB", IBAN: "DE111", Currency: "EUR", CreatedAt: now, UpdatedAt: now}

	if err := s.CreateLedgerAccount(ctx, userID, first); err != nil {
		t.Fatalf("CreateLedgerAccount first: %v", err)
	}
	if err := s.CreateLedgerAccount(ctx, userID, second); !errors.Is(err, ErrConflict) {
		t.Fatalf("expected ErrConflict, got %v", err)
	}
}

func TestLedgerAccounts_ListSortedByName(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	userID := "user-1"
	now := time.Now().UTC()

	accounts := []model.LedgerAccount{
		{ID: uuid.New(), Name: "Zulu", Bank: "DKB", Currency: "EUR", CreatedAt: now.Add(2 * time.Minute), UpdatedAt: now.Add(2 * time.Minute)},
		{ID: uuid.New(), Name: "alpha", Bank: "DKB", Currency: "EUR", CreatedAt: now, UpdatedAt: now},
		{ID: uuid.New(), Name: "Beta", Bank: "DKB", Currency: "EUR", CreatedAt: now.Add(time.Minute), UpdatedAt: now.Add(time.Minute)},
	}
	for _, account := range accounts {
		if err := s.CreateLedgerAccount(ctx, userID, account); err != nil {
			t.Fatalf("CreateLedgerAccount: %v", err)
		}
	}

	got, err := s.ListLedgerAccounts(ctx, userID)
	if err != nil {
		t.Fatalf("ListLedgerAccounts: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("len(accounts) = %d, want 3", len(got))
	}
	if got[0].Name != "alpha" || got[1].Name != "Beta" || got[2].Name != "Zulu" {
		t.Fatalf("unexpected order: %q, %q, %q", got[0].Name, got[1].Name, got[2].Name)
	}
}

func TestLedgerTransactionsPage_PaginatesNewestFirst(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	userID := "user-1"
	accountID := uuid.New()
	now := time.Now().UTC()

	if err := s.CreateLedgerAccount(ctx, userID, model.LedgerAccount{ID: accountID, Name: "Main", Bank: "DKB", Currency: "EUR", CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatalf("CreateLedgerAccount: %v", err)
	}

	txns := []model.LedgerTransaction{
		{ID: uuid.New(), AccountID: accountID, BookingDate: "2026-04-01", Currency: "EUR", Fingerprint: "fp-1", ImportBatchID: uuid.New(), CreatedAt: now, UpdatedAt: now},
		{ID: uuid.New(), AccountID: accountID, BookingDate: "2026-04-02", Currency: "EUR", Fingerprint: "fp-2", ImportBatchID: uuid.New(), CreatedAt: now, UpdatedAt: now},
		{ID: uuid.New(), AccountID: accountID, BookingDate: "2026-04-03", Currency: "EUR", Fingerprint: "fp-3", ImportBatchID: uuid.New(), CreatedAt: now, UpdatedAt: now},
	}
	batch := model.LedgerImportBatch{ID: uuid.New(), AccountID: accountID, SourceType: "dkb.csv", ParserVersion: "1", Filename: "test.csv", FileSHA256: "hash-1", Status: model.ImportStatusCommitted, CreatedAt: now, UpdatedAt: now}
	if _, err := s.CommitLedgerImport(ctx, userID, batch, txns); err != nil {
		t.Fatalf("CommitLedgerImport: %v", err)
	}

	page1, err := s.ListLedgerTransactionsPage(ctx, userID, accountID, 2, "")
	if err != nil {
		t.Fatalf("ListLedgerTransactionsPage page1: %v", err)
	}
	if len(page1.Items) != 2 {
		t.Fatalf("page1 len = %d, want 2", len(page1.Items))
	}
	if page1.Items[0].BookingDate != "2026-04-03" || page1.Items[1].BookingDate != "2026-04-02" {
		t.Fatalf("page1 order = %q, %q", page1.Items[0].BookingDate, page1.Items[1].BookingDate)
	}
	if page1.NextCursor == "" {
		t.Fatal("expected next cursor")
	}

	page2, err := s.ListLedgerTransactionsPage(ctx, userID, accountID, 2, page1.NextCursor)
	if err != nil {
		t.Fatalf("ListLedgerTransactionsPage page2: %v", err)
	}
	if len(page2.Items) != 1 {
		t.Fatalf("page2 len = %d, want 1", len(page2.Items))
	}
	if page2.Items[0].BookingDate != "2026-04-01" {
		t.Fatalf("page2 date = %q, want 2026-04-01", page2.Items[0].BookingDate)
	}
}

func TestLedgerCommitImport_RejectsDuplicateFile(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	userID := "user-1"
	accountID := uuid.New()
	now := time.Now().UTC()

	if err := s.CreateLedgerAccount(ctx, userID, model.LedgerAccount{ID: accountID, Name: "Main", Bank: "DKB", Currency: "EUR", CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatalf("CreateLedgerAccount: %v", err)
	}

	batch1 := model.LedgerImportBatch{ID: uuid.New(), AccountID: accountID, SourceType: "dkb.csv", ParserVersion: "1", Filename: "a.csv", FileSHA256: "same-hash", Status: model.ImportStatusCommitted, CreatedAt: now, UpdatedAt: now}
	batch2 := model.LedgerImportBatch{ID: uuid.New(), AccountID: accountID, SourceType: "dkb.csv", ParserVersion: "1", Filename: "b.csv", FileSHA256: "same-hash", Status: model.ImportStatusCommitted, CreatedAt: now, UpdatedAt: now}
	if _, err := s.CommitLedgerImport(ctx, userID, batch1, nil); err != nil {
		t.Fatalf("first commit: %v", err)
	}
	if _, err := s.CommitLedgerImport(ctx, userID, batch2, nil); !errors.Is(err, ErrLedgerFileImported) {
		t.Fatalf("expected ErrLedgerFileImported, got %v", err)
	}
}
