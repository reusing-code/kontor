package ledger

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"testing"
	"time"

	"github.com/dgraph-io/badger/v4"
	"github.com/google/uuid"
	"github.com/reusing-code/kontor/backend/internal/module"
	"github.com/reusing-code/kontor/backend/internal/modules/auto"
	"github.com/reusing-code/kontor/backend/internal/modules/contracts"
	"github.com/reusing-code/kontor/backend/internal/modules/purchases"
	"github.com/reusing-code/kontor/backend/internal/storage"
	"github.com/reusing-code/kontor/backend/internal/storage/link"
)

// testStores bundles the ledger store with the contracts, purchases and auto
// stores that serve as link targets, all over one temp engine.
type testStores struct {
	*Store
	purchases *purchases.Store
	contracts *contracts.Store
	auto      *auto.Store
}

func newTestStore(t *testing.T) *testStores {
	t.Helper()
	engine, err := storage.Open(t.TempDir(), slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatalf("opening engine: %v", err)
	}
	t.Cleanup(func() { engine.Close() })
	links := link.NewRegistry()
	return &testStores{
		Store:     NewStore(engine, links),
		purchases: purchases.NewStore(engine, links),
		contracts: contracts.NewStore(engine, links),
		auto:      auto.NewStore(engine, links),
	}
}

func (s *testStores) CreatePurchase(ctx context.Context, userID string, p purchases.Purchase) error {
	return s.purchases.Create(ctx, userID, p)
}

func (s *testStores) GetPurchase(ctx context.Context, userID string, id uuid.UUID) (purchases.Purchase, error) {
	return s.purchases.Get(ctx, userID, id)
}

func (s *testStores) DeletePurchase(ctx context.Context, userID string, id uuid.UUID) error {
	return s.purchases.Delete(ctx, userID, id)
}

func (s *testStores) CreateContract(ctx context.Context, userID string, c contracts.Contract) error {
	return s.contracts.Create(ctx, userID, c)
}

func (s *testStores) GetContract(ctx context.Context, userID string, id uuid.UUID) (contracts.Contract, error) {
	return s.contracts.Get(ctx, userID, id)
}

func TestLedgerAccount_CreateConflictOnIBAN(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	userID := "user-1"
	now := time.Now().UTC()

	first := LedgerAccount{ID: uuid.New(), Name: "A", Bank: "DKB", IBAN: "DE111", Currency: "EUR", CreatedAt: now, UpdatedAt: now}
	second := LedgerAccount{ID: uuid.New(), Name: "B", Bank: "DKB", IBAN: "DE111", Currency: "EUR", CreatedAt: now, UpdatedAt: now}

	if err := s.CreateLedgerAccount(ctx, userID, first); err != nil {
		t.Fatalf("CreateLedgerAccount first: %v", err)
	}
	if err := s.CreateLedgerAccount(ctx, userID, second); !errors.Is(err, storage.ErrConflict) {
		t.Fatalf("expected storage.ErrConflict, got %v", err)
	}
}

func TestLedgerAccounts_ListSortedByName(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	userID := "user-1"
	now := time.Now().UTC()

	accounts := []LedgerAccount{
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

	if err := s.CreateLedgerAccount(ctx, userID, LedgerAccount{ID: accountID, Name: "Main", Bank: "DKB", Currency: "EUR", CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatalf("CreateLedgerAccount: %v", err)
	}

	txns := []LedgerTransaction{
		{ID: uuid.New(), AccountID: accountID, BookingDate: "2026-04-01", Currency: "EUR", Fingerprint: "fp-1", ImportBatchID: uuid.New(), CreatedAt: now, UpdatedAt: now},
		{ID: uuid.New(), AccountID: accountID, BookingDate: "2026-04-02", Currency: "EUR", Fingerprint: "fp-2", ImportBatchID: uuid.New(), CreatedAt: now, UpdatedAt: now},
		{ID: uuid.New(), AccountID: accountID, BookingDate: "2026-04-03", Currency: "EUR", Fingerprint: "fp-3", ImportBatchID: uuid.New(), CreatedAt: now, UpdatedAt: now},
	}
	batch := LedgerImportBatch{ID: uuid.New(), AccountID: accountID, SourceType: "dkb.csv", ParserVersion: "1", Filename: "test.csv", FileSHA256: "hash-1", Status: ImportStatusCommitted, CreatedAt: now, UpdatedAt: now}
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

func TestLedgerTransactionsFiltered_EmptyResultHasNonNilItems(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	page, err := s.ListLedgerTransactionsFiltered(ctx, "user-1", LedgerTransactionListOptions{
		ReviewStatus: LedgerTransactionReviewNeedsReview,
		Limit:        10,
	})
	if err != nil {
		t.Fatalf("ListLedgerTransactionsFiltered: %v", err)
	}
	if page.Items == nil {
		t.Fatal("page.Items is nil, want empty slice (serializes to JSON null otherwise)")
	}
	if len(page.Items) != 0 {
		t.Fatalf("page.Items len = %d, want 0", len(page.Items))
	}
}

func TestLedgerCommitImport_RejectsDuplicateFile(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	userID := "user-1"
	accountID := uuid.New()
	now := time.Now().UTC()

	if err := s.CreateLedgerAccount(ctx, userID, LedgerAccount{ID: accountID, Name: "Main", Bank: "DKB", Currency: "EUR", CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatalf("CreateLedgerAccount: %v", err)
	}

	batch1 := LedgerImportBatch{ID: uuid.New(), AccountID: accountID, SourceType: "dkb.csv", ParserVersion: "1", Filename: "a.csv", FileSHA256: "same-hash", Status: ImportStatusCommitted, CreatedAt: now, UpdatedAt: now}
	batch2 := LedgerImportBatch{ID: uuid.New(), AccountID: accountID, SourceType: "dkb.csv", ParserVersion: "1", Filename: "b.csv", FileSHA256: "same-hash", Status: ImportStatusCommitted, CreatedAt: now, UpdatedAt: now}
	if _, err := s.CommitLedgerImport(ctx, userID, batch1, nil); err != nil {
		t.Fatalf("first commit: %v", err)
	}
	if _, err := s.CommitLedgerImport(ctx, userID, batch2, nil); !errors.Is(err, ErrLedgerFileImported) {
		t.Fatalf("expected ErrLedgerFileImported, got %v", err)
	}
}

func TestLedgerCommitImport_PersistsImportedRowCounts(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	userID := "user-1"
	accountID := uuid.New()
	now := time.Now().UTC()

	if err := s.CreateLedgerAccount(ctx, userID, LedgerAccount{ID: accountID, Name: "Main", Bank: "DKB", Currency: "EUR", CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatalf("CreateLedgerAccount: %v", err)
	}

	batch := LedgerImportBatch{ID: uuid.New(), AccountID: accountID, SourceType: "dkb.csv", ParserVersion: "1", Filename: "counts.csv", FileSHA256: "hash-counts", Status: ImportStatusCommitted, TotalRows: 2, CreatedAt: now, UpdatedAt: now}
	txns := []LedgerTransaction{
		{ID: uuid.New(), AccountID: accountID, BookingDate: "2026-04-01", Currency: "EUR", Fingerprint: "fp-counts-1", ImportBatchID: batch.ID, CreatedAt: now, UpdatedAt: now},
		{ID: uuid.New(), AccountID: accountID, BookingDate: "2026-04-02", Currency: "EUR", Fingerprint: "fp-counts-2", ImportBatchID: batch.ID, CreatedAt: now, UpdatedAt: now},
	}

	result, err := s.CommitLedgerImport(ctx, userID, batch, txns)
	if err != nil {
		t.Fatalf("CommitLedgerImport: %v", err)
	}
	if result.ImportedRows != 2 {
		t.Fatalf("ImportedRows = %d, want 2", result.ImportedRows)
	}

	imports, err := s.ListLedgerImports(ctx, userID)
	if err != nil {
		t.Fatalf("ListLedgerImports: %v", err)
	}
	if len(imports) != 1 {
		t.Fatalf("len(imports) = %d, want 1", len(imports))
	}
	if imports[0].ImportedRows != 2 {
		t.Fatalf("persisted ImportedRows = %d, want 2", imports[0].ImportedRows)
	}
	if imports[0].DuplicateRows != 0 {
		t.Fatalf("persisted DuplicateRows = %d, want 0", imports[0].DuplicateRows)
	}
}

func TestLedgerCategory_DeleteBlockedWhenHasChildren(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	userID := "user-1"
	now := time.Now().UTC()

	parentID := uuid.New()
	childID := uuid.New()
	parent := LedgerCategory{ID: parentID, Name: "Living expenses", CreatedAt: now, UpdatedAt: now}
	child := LedgerCategory{ID: childID, Name: "Food", ParentID: &parentID, CreatedAt: now, UpdatedAt: now}

	if err := s.CreateLedgerCategory(ctx, userID, parent); err != nil {
		t.Fatalf("CreateLedgerCategory parent: %v", err)
	}
	if err := s.CreateLedgerCategory(ctx, userID, child); err != nil {
		t.Fatalf("CreateLedgerCategory child: %v", err)
	}

	if err := s.DeleteLedgerCategory(ctx, userID, parentID); !errors.Is(err, ErrLedgerCategoryHasChild) {
		t.Fatalf("expected ErrLedgerCategoryHasChild, got %v", err)
	}

	if _, err := s.GetLedgerCategory(ctx, userID, parentID); err != nil {
		t.Fatalf("parent category should still exist: %v", err)
	}
}

func TestLedgerCategory_UpdateRejectsCycle(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	userID := "user-1"
	now := time.Now().UTC()

	rootID := uuid.New()
	childID := uuid.New()
	root := LedgerCategory{ID: rootID, Name: "Root", CreatedAt: now, UpdatedAt: now}
	child := LedgerCategory{ID: childID, Name: "Child", ParentID: &rootID, CreatedAt: now, UpdatedAt: now}

	if err := s.CreateLedgerCategory(ctx, userID, root); err != nil {
		t.Fatalf("CreateLedgerCategory root: %v", err)
	}
	if err := s.CreateLedgerCategory(ctx, userID, child); err != nil {
		t.Fatalf("CreateLedgerCategory child: %v", err)
	}

	root.ParentID = &childID
	root.UpdatedAt = now.Add(time.Minute)
	if err := s.UpdateLedgerCategory(ctx, userID, root); !errors.Is(err, ErrLedgerCategoryHasCycle) {
		t.Fatalf("expected ErrLedgerCategoryHasCycle, got %v", err)
	}
}

func TestLedgerCategory_DeleteUncategorizesTransactions(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	userID := "user-1"
	accountID := uuid.New()
	categoryID := uuid.New()
	now := time.Now().UTC()

	if err := s.CreateLedgerAccount(ctx, userID, LedgerAccount{ID: accountID, Name: "Main", Bank: "DKB", Currency: "EUR", CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatalf("CreateLedgerAccount: %v", err)
	}
	if err := s.CreateLedgerCategory(ctx, userID, LedgerCategory{ID: categoryID, Name: "Food", CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatalf("CreateLedgerCategory: %v", err)
	}

	txns := []LedgerTransaction{
		{
			ID:                   uuid.New(),
			AccountID:            accountID,
			CategoryID:           &categoryID,
			BookingDate:          "2026-04-02",
			Currency:             "EUR",
			ReviewStatus:         LedgerTransactionReviewConfirmed,
			CategorizationSource: LedgerCategorizationManual,
			Fingerprint:          "fp-ledger-cat-delete",
			ImportBatchID:        uuid.New(),
			CreatedAt:            now,
			UpdatedAt:            now,
		},
		{
			ID:                   uuid.New(),
			AccountID:            accountID,
			BookingDate:          "2026-04-01",
			Currency:             "EUR",
			ReviewStatus:         LedgerTransactionReviewNeedsReview,
			CategorizationSource: LedgerCategorizationNone,
			Fingerprint:          "fp-ledger-cat-delete-2",
			ImportBatchID:        uuid.New(),
			CreatedAt:            now,
			UpdatedAt:            now,
		},
	}
	batch := LedgerImportBatch{ID: uuid.New(), AccountID: accountID, SourceType: "dkb.csv", ParserVersion: "1", Filename: "test.csv", FileSHA256: "hash-ledger-cat-delete", Status: ImportStatusCommitted, CreatedAt: now, UpdatedAt: now}
	if _, err := s.CommitLedgerImport(ctx, userID, batch, txns); err != nil {
		t.Fatalf("CommitLedgerImport: %v", err)
	}

	if err := s.DeleteLedgerCategory(ctx, userID, categoryID); err != nil {
		t.Fatalf("DeleteLedgerCategory: %v", err)
	}

	updated, err := s.GetLedgerTransaction(ctx, userID, txns[0].ID)
	if err != nil {
		t.Fatalf("GetLedgerTransaction: %v", err)
	}
	if updated.CategoryID != nil {
		t.Fatalf("CategoryID = %v, want nil", *updated.CategoryID)
	}
	if updated.ReviewStatus != LedgerTransactionReviewNeedsReview {
		t.Fatalf("ReviewStatus = %q, want %q", updated.ReviewStatus, LedgerTransactionReviewNeedsReview)
	}
	if updated.CategorizationSource != LedgerCategorizationNone {
		t.Fatalf("CategorizationSource = %q, want %q", updated.CategorizationSource, LedgerCategorizationNone)
	}
}

func TestUpdateLedgerTransactionDetails_SyncsBidirectionalReferences(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	userID := "user-1"
	accountID := uuid.New()
	purchaseID := uuid.New()
	contractID := uuid.New()
	now := time.Now().UTC()

	if err := s.CreateLedgerAccount(ctx, userID, LedgerAccount{ID: accountID, Name: "Main", Bank: "DKB", Currency: "EUR", CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatalf("CreateLedgerAccount: %v", err)
	}
	if err := s.CreatePurchase(ctx, userID, purchases.Purchase{ID: purchaseID, CategoryID: uuid.New(), ItemName: "Monitor", CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatalf("CreatePurchase: %v", err)
	}
	if err := s.CreateContract(ctx, userID, contracts.Contract{ID: contractID, CategoryID: uuid.New(), Name: "Internet", BillingInterval: contracts.BillingMonthly, StartDate: "2026-01-01", CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatalf("CreateContract: %v", err)
	}

	txnID := uuid.New()
	batch := LedgerImportBatch{ID: uuid.New(), AccountID: accountID, SourceType: "dkb.csv", ParserVersion: "1", Filename: "test.csv", FileSHA256: "hash-details", Status: ImportStatusCommitted, CreatedAt: now, UpdatedAt: now}
	if _, err := s.CommitLedgerImport(ctx, userID, batch, []LedgerTransaction{{
		ID: txnID, AccountID: accountID, BookingDate: "2026-04-02", Currency: "EUR", Fingerprint: "fp-details-sync", ImportBatchID: batch.ID, CreatedAt: now, UpdatedAt: now,
	}}); err != nil {
		t.Fatalf("CommitLedgerImport: %v", err)
	}

	updated, err := s.UpdateLedgerTransactionDetails(ctx, userID, txnID, LedgerTransactionDetailsInput{
		Note:  "invoice and subscription",
		Links: []string{"https://example.com/doc.pdf"},
		References: []LedgerTransactionReference{
			{Type: LedgerReferencePurchase, TargetID: purchaseID},
			{Type: LedgerReferenceContract, TargetID: contractID},
		},
	})
	if err != nil {
		t.Fatalf("UpdateLedgerTransactionDetails: %v", err)
	}
	if len(updated.References) != 2 {
		t.Fatalf("len(references) = %d, want 2", len(updated.References))
	}

	purchase, err := s.GetPurchase(ctx, userID, purchaseID)
	if err != nil {
		t.Fatalf("GetPurchase: %v", err)
	}
	if len(purchase.LinkedTransactionIDs) != 1 || purchase.LinkedTransactionIDs[0] != txnID {
		t.Fatalf("purchase linkedTransactionIds = %#v", purchase.LinkedTransactionIDs)
	}

	contract, err := s.GetContract(ctx, userID, contractID)
	if err != nil {
		t.Fatalf("GetContract: %v", err)
	}
	if len(contract.LinkedTransactionIDs) != 1 || contract.LinkedTransactionIDs[0] != txnID {
		t.Fatalf("contract linkedTransactionIds = %#v", contract.LinkedTransactionIDs)
	}

	updated, err = s.UpdateLedgerTransactionDetails(ctx, userID, txnID, LedgerTransactionDetailsInput{
		References: []LedgerTransactionReference{{Type: LedgerReferencePurchase, TargetID: purchaseID}},
	})
	if err != nil {
		t.Fatalf("UpdateLedgerTransactionDetails second: %v", err)
	}
	if len(updated.References) != 1 || updated.References[0].Type != LedgerReferencePurchase {
		t.Fatalf("updated references = %#v", updated.References)
	}

	contract, err = s.GetContract(ctx, userID, contractID)
	if err != nil {
		t.Fatalf("GetContract after unlink: %v", err)
	}
	if len(contract.LinkedTransactionIDs) != 0 {
		t.Fatalf("contract linkedTransactionIds after unlink = %#v", contract.LinkedTransactionIDs)
	}
}

func TestDeletePurchase_RemovesLedgerReference(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	userID := "user-1"
	accountID := uuid.New()
	purchaseID := uuid.New()
	now := time.Now().UTC()

	if err := s.CreateLedgerAccount(ctx, userID, LedgerAccount{ID: accountID, Name: "Main", Bank: "DKB", Currency: "EUR", CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatalf("CreateLedgerAccount: %v", err)
	}
	if err := s.CreatePurchase(ctx, userID, purchases.Purchase{ID: purchaseID, CategoryID: uuid.New(), ItemName: "Chair", CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatalf("CreatePurchase: %v", err)
	}
	txnID := uuid.New()
	batch := LedgerImportBatch{ID: uuid.New(), AccountID: accountID, SourceType: "dkb.csv", ParserVersion: "1", Filename: "test.csv", FileSHA256: "hash-delete-purchase-link", Status: ImportStatusCommitted, CreatedAt: now, UpdatedAt: now}
	if _, err := s.CommitLedgerImport(ctx, userID, batch, []LedgerTransaction{{
		ID:            txnID,
		AccountID:     accountID,
		BookingDate:   "2026-04-02",
		Currency:      "EUR",
		References:    []LedgerTransactionReference{{Type: LedgerReferencePurchase, TargetID: purchaseID}},
		Fingerprint:   "fp-delete-purchase-link",
		ImportBatchID: batch.ID,
		CreatedAt:     now,
		UpdatedAt:     now,
	}}); err != nil {
		t.Fatalf("CommitLedgerImport: %v", err)
	}
	if _, err := s.UpdateLedgerTransactionDetails(ctx, userID, txnID, LedgerTransactionDetailsInput{
		References: []LedgerTransactionReference{{Type: LedgerReferencePurchase, TargetID: purchaseID}},
	}); err != nil {
		t.Fatalf("UpdateLedgerTransactionDetails: %v", err)
	}

	if err := s.DeletePurchase(ctx, userID, purchaseID); err != nil {
		t.Fatalf("DeletePurchase: %v", err)
	}

	txn, err := s.GetLedgerTransaction(ctx, userID, txnID)
	if err != nil {
		t.Fatalf("GetLedgerTransaction: %v", err)
	}
	if len(txn.References) != 0 {
		t.Fatalf("references after delete = %#v", txn.References)
	}
}

func TestListLedgerTransferCandidates_MatchesOppositeAmountWithinDateWindow(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	userID := "user-1"
	accountA := uuid.New()
	accountB := uuid.New()
	now := time.Now().UTC()

	if err := s.CreateLedgerAccount(ctx, userID, LedgerAccount{ID: accountA, Name: "Checking", IBAN: "DE111", Bank: "DKB", Currency: "EUR", CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatalf("CreateLedgerAccount A: %v", err)
	}
	if err := s.CreateLedgerAccount(ctx, userID, LedgerAccount{ID: accountB, Name: "Savings", IBAN: "DE222", Bank: "DKB", Currency: "EUR", CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatalf("CreateLedgerAccount B: %v", err)
	}
	sourceID := uuid.New()
	candidateID := uuid.New()
	batch := LedgerImportBatch{ID: uuid.New(), AccountID: accountA, SourceType: "dkb.csv", ParserVersion: "1", Filename: "test.csv", FileSHA256: "hash-transfer-candidates", Status: ImportStatusCommitted, CreatedAt: now, UpdatedAt: now}
	_, err := s.CommitLedgerImport(ctx, userID, batch, []LedgerTransaction{
		{ID: sourceID, AccountID: accountA, BookingDate: "2026-04-02", AmountMinor: -12500, Currency: "EUR", CounterpartyIBAN: "DE222", Fingerprint: "fp-transfer-source", ImportBatchID: batch.ID, CreatedAt: now, UpdatedAt: now},
		{ID: candidateID, AccountID: accountB, BookingDate: "2026-04-03", AmountMinor: 12500, Currency: "EUR", CounterpartyIBAN: "DE111", Fingerprint: "fp-transfer-candidate", ImportBatchID: batch.ID, CreatedAt: now, UpdatedAt: now},
	})
	if err != nil {
		t.Fatalf("CommitLedgerImport: %v", err)
	}

	result, err := s.ListLedgerTransferCandidates(ctx, userID, sourceID)
	if err != nil {
		t.Fatalf("ListLedgerTransferCandidates: %v", err)
	}
	if len(result.Items) != 1 {
		t.Fatalf("len(candidates) = %d, want 1", len(result.Items))
	}
	if result.Items[0].Transaction.ID != candidateID {
		t.Fatalf("candidate id = %s", result.Items[0].Transaction.ID)
	}
	if !result.Items[0].IBANMatch {
		t.Fatal("expected IBAN match")
	}
}

func TestLinkAndUnlinkLedgerTransfer_SetsSpecialCategoryOnBothSides(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	userID := "user-1"
	accountA := uuid.New()
	accountB := uuid.New()
	now := time.Now().UTC()

	if err := s.CreateLedgerAccount(ctx, userID, LedgerAccount{ID: accountA, Name: "Checking", Bank: "DKB", Currency: "EUR", CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatalf("CreateLedgerAccount A: %v", err)
	}
	if err := s.CreateLedgerAccount(ctx, userID, LedgerAccount{ID: accountB, Name: "Savings", Bank: "DKB", Currency: "EUR", CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatalf("CreateLedgerAccount B: %v", err)
	}
	leftID := uuid.New()
	rightID := uuid.New()
	batch := LedgerImportBatch{ID: uuid.New(), AccountID: accountA, SourceType: "dkb.csv", ParserVersion: "1", Filename: "test.csv", FileSHA256: "hash-transfer-link", Status: ImportStatusCommitted, CreatedAt: now, UpdatedAt: now}
	if _, err := s.CommitLedgerImport(ctx, userID, batch, []LedgerTransaction{
		{ID: leftID, AccountID: accountA, BookingDate: "2026-04-02", AmountMinor: -8000, Currency: "EUR", Fingerprint: "fp-transfer-left", ImportBatchID: batch.ID, CreatedAt: now, UpdatedAt: now},
		{ID: rightID, AccountID: accountB, BookingDate: "2026-04-04", AmountMinor: 8000, Currency: "EUR", Fingerprint: "fp-transfer-right", ImportBatchID: batch.ID, CreatedAt: now, UpdatedAt: now},
	}); err != nil {
		t.Fatalf("CommitLedgerImport: %v", err)
	}

	result, err := s.LinkLedgerTransfer(ctx, userID, leftID, LedgerTransferLinkInput{PairedTransactionID: rightID})
	if err != nil {
		t.Fatalf("LinkLedgerTransfer: %v", err)
	}
	if result.Transaction.SpecialCategory != LedgerSpecialCategoryInternalTransfer {
		t.Fatalf("special category = %q", result.Transaction.SpecialCategory)
	}
	if result.PairedTransaction.SpecialCategory != LedgerSpecialCategoryInternalTransfer {
		t.Fatalf("paired special category = %q", result.PairedTransaction.SpecialCategory)
	}

	unlinked, err := s.UnlinkLedgerTransfer(ctx, userID, leftID)
	if err != nil {
		t.Fatalf("UnlinkLedgerTransfer: %v", err)
	}
	if unlinked.Transaction.TransferPairTransactionID != nil {
		t.Fatal("expected source transfer pair to be cleared")
	}
	if unlinked.PairedTransaction == nil || unlinked.PairedTransaction.TransferPairTransactionID != nil {
		t.Fatal("expected paired transfer pair to be cleared")
	}
}

func TestUpdateLedgerTransactionDetails_KeepsTransferLink(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	userID := "user-1"
	accountA := uuid.New()
	accountB := uuid.New()
	now := time.Now().UTC()

	if err := s.CreateLedgerAccount(ctx, userID, LedgerAccount{ID: accountA, Name: "Checking", Bank: "DKB", Currency: "EUR", CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatalf("CreateLedgerAccount A: %v", err)
	}
	if err := s.CreateLedgerAccount(ctx, userID, LedgerAccount{ID: accountB, Name: "Savings", Bank: "DKB", Currency: "EUR", CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatalf("CreateLedgerAccount B: %v", err)
	}
	leftID := uuid.New()
	rightID := uuid.New()
	batch := LedgerImportBatch{ID: uuid.New(), AccountID: accountA, SourceType: "dkb.csv", ParserVersion: "1", Filename: "test.csv", FileSHA256: "hash-transfer-details", Status: ImportStatusCommitted, CreatedAt: now, UpdatedAt: now}
	if _, err := s.CommitLedgerImport(ctx, userID, batch, []LedgerTransaction{
		{ID: leftID, AccountID: accountA, BookingDate: "2026-04-02", AmountMinor: -8000, Currency: "EUR", Fingerprint: "fp-transfer-left-details", ImportBatchID: batch.ID, CreatedAt: now, UpdatedAt: now},
		{ID: rightID, AccountID: accountB, BookingDate: "2026-04-04", AmountMinor: 8000, Currency: "EUR", Fingerprint: "fp-transfer-right-details", ImportBatchID: batch.ID, CreatedAt: now, UpdatedAt: now},
	}); err != nil {
		t.Fatalf("CommitLedgerImport: %v", err)
	}
	if _, err := s.LinkLedgerTransfer(ctx, userID, leftID, LedgerTransferLinkInput{PairedTransactionID: rightID}); err != nil {
		t.Fatalf("LinkLedgerTransfer: %v", err)
	}

	updated, err := s.UpdateLedgerTransactionDetails(ctx, userID, leftID, LedgerTransactionDetailsInput{Note: "keep link"})
	if err != nil {
		t.Fatalf("UpdateLedgerTransactionDetails: %v", err)
	}
	if updated.TransferPairTransactionID == nil || *updated.TransferPairTransactionID != rightID {
		t.Fatalf("transferPairTransactionId = %v, want %s", updated.TransferPairTransactionID, rightID)
	}
	if updated.SpecialCategory != LedgerSpecialCategoryInternalTransfer {
		t.Fatalf("special category = %q", updated.SpecialCategory)
	}

	paired, err := s.GetLedgerTransaction(ctx, userID, rightID)
	if err != nil {
		t.Fatalf("GetLedgerTransaction paired: %v", err)
	}
	if paired.TransferPairTransactionID == nil || *paired.TransferPairTransactionID != leftID {
		t.Fatalf("paired transferPairTransactionId = %v, want %s", paired.TransferPairTransactionID, leftID)
	}
}

func TestReviewLedgerTransaction_RejectsCategoryAssignmentForLinkedTransfer(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	userID := "user-1"
	accountA := uuid.New()
	accountB := uuid.New()
	categoryID := uuid.New()
	now := time.Now().UTC()

	if err := s.CreateLedgerAccount(ctx, userID, LedgerAccount{ID: accountA, Name: "Checking", Bank: "DKB", Currency: "EUR", CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatalf("CreateLedgerAccount A: %v", err)
	}
	if err := s.CreateLedgerAccount(ctx, userID, LedgerAccount{ID: accountB, Name: "Savings", Bank: "DKB", Currency: "EUR", CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatalf("CreateLedgerAccount B: %v", err)
	}
	if err := s.CreateLedgerCategory(ctx, userID, LedgerCategory{ID: categoryID, Name: "Household", CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatalf("CreateLedgerCategory: %v", err)
	}
	leftID := uuid.New()
	rightID := uuid.New()
	batch := LedgerImportBatch{ID: uuid.New(), AccountID: accountA, SourceType: "dkb.csv", ParserVersion: "1", Filename: "test.csv", FileSHA256: "hash-transfer-review", Status: ImportStatusCommitted, CreatedAt: now, UpdatedAt: now}
	if _, err := s.CommitLedgerImport(ctx, userID, batch, []LedgerTransaction{
		{ID: leftID, AccountID: accountA, BookingDate: "2026-04-02", AmountMinor: -8000, Currency: "EUR", Fingerprint: "fp-transfer-left-review", ImportBatchID: batch.ID, CreatedAt: now, UpdatedAt: now},
		{ID: rightID, AccountID: accountB, BookingDate: "2026-04-04", AmountMinor: 8000, Currency: "EUR", Fingerprint: "fp-transfer-right-review", ImportBatchID: batch.ID, CreatedAt: now, UpdatedAt: now},
	}); err != nil {
		t.Fatalf("CommitLedgerImport: %v", err)
	}
	if _, err := s.LinkLedgerTransfer(ctx, userID, leftID, LedgerTransferLinkInput{PairedTransactionID: rightID}); err != nil {
		t.Fatalf("LinkLedgerTransfer: %v", err)
	}

	_, err := s.ReviewLedgerTransaction(ctx, userID, leftID, LedgerTransactionReviewInput{CategoryID: &categoryID})
	if !errors.Is(err, ErrLedgerTransferLinked) {
		t.Fatalf("ReviewLedgerTransaction error = %v, want %v", err, ErrLedgerTransferLinked)
	}

	left, err := s.GetLedgerTransaction(ctx, userID, leftID)
	if err != nil {
		t.Fatalf("GetLedgerTransaction: %v", err)
	}
	if left.TransferPairTransactionID == nil || *left.TransferPairTransactionID != rightID {
		t.Fatalf("transferPairTransactionId = %v, want %s", left.TransferPairTransactionID, rightID)
	}
}

func TestLedgerEmailAccount_FolderSettingsSurvivePersistence(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	now := time.Now().UTC()

	account := LedgerEmailAccount{
		ID: uuid.New(), Name: "Mailbox", IMAPHost: "imap.example.com", IMAPPort: 993, Username: "user",
		EncryptedPassword: "x", Folder: "Rechnungen", PreClassify: true, CreatedAt: now, UpdatedAt: now,
	}
	if err := s.CreateLedgerEmailAccount(ctx, "test-user", account); err != nil {
		t.Fatalf("CreateLedgerEmailAccount: %v", err)
	}
	got, err := s.GetLedgerEmailAccount(ctx, "test-user", account.ID)
	if err != nil {
		t.Fatalf("GetLedgerEmailAccount: %v", err)
	}
	if got.Folder != "Rechnungen" || !got.PreClassify {
		t.Errorf("folder = %q, preClassify = %v, want Rechnungen/true", got.Folder, got.PreClassify)
	}
}

func TestLedgerEmailAccount_LegacyStoredJSONHasNoFolder(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	id := uuid.New()
	legacy := fmt.Sprintf(`{"id":%q,"name":"Mailbox","imapHost":"h","imapPort":993,"username":"u","encryptedPassword":"x","useTls":true,"scanSince":"2026-01-01","createdAt":"2026-01-01T00:00:00Z","updatedAt":"2026-01-01T00:00:00Z"}`, id)
	if err := s.e.Update(func(txn *badger.Txn) error {
		return txn.Set(ledEmailAccKey("test-user", id), []byte(legacy))
	}); err != nil {
		t.Fatalf("storing legacy account: %v", err)
	}
	got, err := s.GetLedgerEmailAccount(ctx, "test-user", id)
	if err != nil {
		t.Fatalf("GetLedgerEmailAccount: %v", err)
	}
	if got.Folder != "" || got.PreClassify || got.EncryptedPassword != "x" {
		t.Errorf("legacy account = %+v", got)
	}
}

func TestLedgerEmailAccount_EncryptedPasswordSurvivesPersistence(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	now := time.Now().UTC()

	account := LedgerEmailAccount{
		ID:                uuid.New(),
		Name:              "Mailbox",
		IMAPHost:          "imap.example.com",
		IMAPPort:          993,
		Username:          "user@example.com",
		EncryptedPassword: "encrypted-secret",
		UseTLS:            true,
		ScanSince:         "2026-01-01",
		CreatedAt:         now,
		UpdatedAt:         now,
	}
	if err := s.CreateLedgerEmailAccount(ctx, "test-user", account); err != nil {
		t.Fatalf("CreateLedgerEmailAccount: %v", err)
	}

	got, err := s.GetLedgerEmailAccount(ctx, "test-user", account.ID)
	if err != nil {
		t.Fatalf("GetLedgerEmailAccount: %v", err)
	}
	if got.EncryptedPassword != "encrypted-secret" {
		t.Errorf("EncryptedPassword = %q, want %q — must survive persistence despite json:\"-\" on the API type", got.EncryptedPassword, "encrypted-secret")
	}

	got.LastScanStatusMessage = "scanned"
	if err := s.UpdateLedgerEmailAccount(ctx, "test-user", got); err != nil {
		t.Fatalf("UpdateLedgerEmailAccount: %v", err)
	}

	list, err := s.ListLedgerEmailAccounts(ctx, "test-user")
	if err != nil {
		t.Fatalf("ListLedgerEmailAccounts: %v", err)
	}
	if len(list) != 1 || list[0].EncryptedPassword != "encrypted-secret" {
		t.Errorf("list = %+v, want the encrypted password preserved", list)
	}
}

func TestLedgerEmailSeen(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	accountA, accountB := uuid.New(), uuid.New()
	oddKey := "<a/b+c@mail.example.com>"

	has := func(user string, account uuid.UUID, key string) bool {
		t.Helper()
		seen, err := s.HasLedgerEmailSeen(ctx, user, account, key)
		if err != nil {
			t.Fatalf("HasLedgerEmailSeen: %v", err)
		}
		return seen
	}

	if has("test-user", accountA, oddKey) {
		t.Fatal("unmarked message reported as seen")
	}
	if err := s.MarkLedgerEmailsSeen(ctx, "test-user", accountA, EmailSeenNotOrder, oddKey, "nomid:abc"); err != nil {
		t.Fatalf("MarkLedgerEmailsSeen: %v", err)
	}
	if err := s.MarkLedgerEmailsSeen(ctx, "test-user", accountA, EmailSeenPreClass, oddKey); err != nil {
		t.Fatalf("marking twice: %v", err)
	}
	if err := s.MarkLedgerEmailsSeen(ctx, "test-user", accountA, EmailSeenNotOrder); err != nil {
		t.Fatalf("marking nothing: %v", err)
	}
	if !has("test-user", accountA, oddKey) || !has("test-user", accountA, "nomid:abc") {
		t.Error("marked messages not reported as seen")
	}
	if has("test-user", accountB, oddKey) {
		t.Error("marker leaked to another account")
	}
	if has("other-user", accountA, oddKey) {
		t.Error("marker leaked to another user")
	}
}

func TestDeleteLedgerEmailAccount_RemovesOnlyItsSeenMarkers(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	now := time.Now().UTC()
	newAccount := func() LedgerEmailAccount {
		a := LedgerEmailAccount{ID: uuid.New(), Name: "M", IMAPHost: "h", IMAPPort: 993, Username: "u", CreatedAt: now, UpdatedAt: now}
		if err := s.CreateLedgerEmailAccount(ctx, "test-user", a); err != nil {
			t.Fatalf("CreateLedgerEmailAccount: %v", err)
		}
		return a
	}
	doomed, kept := newAccount(), newAccount()
	for _, a := range []LedgerEmailAccount{doomed, kept} {
		if err := s.MarkLedgerEmailsSeen(ctx, "test-user", a.ID, EmailSeenNotOrder, "k1", "k2"); err != nil {
			t.Fatalf("MarkLedgerEmailsSeen: %v", err)
		}
	}
	if err := s.DeleteLedgerEmailAccount(ctx, "test-user", doomed.ID); err != nil {
		t.Fatalf("DeleteLedgerEmailAccount: %v", err)
	}
	for _, key := range []string{"k1", "k2"} {
		if seen, _ := s.HasLedgerEmailSeen(ctx, "test-user", doomed.ID, key); seen {
			t.Errorf("marker %s of the deleted account survived", key)
		}
		if seen, _ := s.HasLedgerEmailSeen(ctx, "test-user", kept.ID, key); !seen {
			t.Errorf("marker %s of the other account was removed", key)
		}
	}
}

func TestSetLedgerEmailAccountScanState_DoesNotRevertConcurrentEdits(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	now := time.Now().UTC()
	account := LedgerEmailAccount{
		ID: uuid.New(), Name: "Mailbox", IMAPHost: "h", IMAPPort: 993, Username: "u",
		EncryptedPassword: "old-secret", Folder: "Old", CreatedAt: now, UpdatedAt: now,
	}
	if err := s.CreateLedgerEmailAccount(ctx, "test-user", account); err != nil {
		t.Fatalf("CreateLedgerEmailAccount: %v", err)
	}

	// The user edits the account while a scan that loaded it earlier is running.
	edited := account
	edited.Folder, edited.EncryptedPassword, edited.PreClassify = "New", "new-secret", true
	if err := s.UpdateLedgerEmailAccount(ctx, "test-user", edited); err != nil {
		t.Fatalf("UpdateLedgerEmailAccount: %v", err)
	}
	done := time.Now().UTC()
	if err := s.SetLedgerEmailAccountScanState(ctx, "test-user", account.ID, LedgerEmailScanState{Status: "scanned", AttemptedAt: &done, CompletedAt: &done}); err != nil {
		t.Fatalf("SetLedgerEmailAccountScanState: %v", err)
	}

	got, err := s.GetLedgerEmailAccount(ctx, "test-user", account.ID)
	if err != nil {
		t.Fatalf("GetLedgerEmailAccount: %v", err)
	}
	if got.Folder != "New" || got.EncryptedPassword != "new-secret" || !got.PreClassify {
		t.Errorf("edit was reverted: %+v", got)
	}
	if got.LastScanStatusMessage != "scanned" || got.LastScanAt == nil || got.LastScanAttemptAt == nil {
		t.Errorf("scan state not stored: %+v", got)
	}
	if err := s.SetLedgerEmailAccountScanState(ctx, "test-user", uuid.New(), LedgerEmailScanState{Status: "x"}); !errors.Is(err, storage.ErrNotFound) {
		t.Errorf("unknown account: err = %v, want ErrNotFound", err)
	}
}

func TestCreateScannedLedgerEmailOrder_ConcurrentCreatesStoreOneOrder(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	now := time.Now().UTC()
	for round := range 30 {
		key := fmt.Sprintf("<race-%d@example.com>", round)
		const workers = 6
		start := make(chan struct{})
		errs := make(chan error, workers)
		for range workers {
			go func() {
				order := LedgerEmailOrder{ID: uuid.New(), EmailAccountID: uuid.New(), ImporterID: LedgerEmailImporterLLM, OrderDate: "2026-04-01", TotalMinor: 100, Currency: "EUR", EmailMessageID: key, CreatedAt: now, UpdatedAt: now}
				<-start
				errs <- s.CreateScannedLedgerEmailOrder(ctx, "test-user", order)
			}()
		}
		close(start)
		created := 0
		for range workers {
			switch err := <-errs; {
			case err == nil:
				created++
			case errors.Is(err, storage.ErrConflict):
			default:
				t.Fatalf("round %d: err = %v, want nil or storage.ErrConflict (never badger.ErrConflict)", round, err)
			}
		}
		if created != 1 {
			t.Fatalf("round %d: %d orders created, want 1", round, created)
		}
	}
}

func TestRecordLedgerEmailRejection_CountsPerMessageAndIsDeletedWithTheAccount(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	now := time.Now().UTC()
	account := LedgerEmailAccount{ID: uuid.New(), Name: "M", IMAPHost: "h", IMAPPort: 993, Username: "u", CreatedAt: now, UpdatedAt: now}
	if err := s.CreateLedgerEmailAccount(ctx, "test-user", account); err != nil {
		t.Fatalf("CreateLedgerEmailAccount: %v", err)
	}
	for want := 1; want <= 3; want++ {
		if got, err := s.RecordLedgerEmailRejection(ctx, "test-user", account.ID, "uid:1"); err != nil || got != want {
			t.Fatalf("count = %d, err = %v, want %d", got, err, want)
		}
	}
	if got, _ := s.RecordLedgerEmailRejection(ctx, "test-user", account.ID, "uid:2"); got != 1 {
		t.Errorf("other message count = %d, want 1", got)
	}
	if err := s.DeleteLedgerEmailAccount(ctx, "test-user", account.ID); err != nil {
		t.Fatalf("DeleteLedgerEmailAccount: %v", err)
	}
	if got, _ := s.RecordLedgerEmailRejection(ctx, "test-user", account.ID, "uid:1"); got != 1 {
		t.Errorf("count after account delete = %d, want 1 (counters removed)", got)
	}
}

func TestRecordLedgerEmailExtractionFailure_CountsPerMessageAndAccountUntilTheMessageIsFinal(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	now := time.Now().UTC()
	newAccount := func() LedgerEmailAccount {
		a := LedgerEmailAccount{ID: uuid.New(), Name: "M", IMAPHost: "h", IMAPPort: 993, Username: uuid.NewString(), CreatedAt: now, UpdatedAt: now}
		if err := s.CreateLedgerEmailAccount(ctx, "test-user", a); err != nil {
			t.Fatalf("CreateLedgerEmailAccount: %v", err)
		}
		return a
	}
	record := func(account LedgerEmailAccount, key string, want int) {
		t.Helper()
		if got, err := s.RecordLedgerEmailExtractionFailure(ctx, "test-user", account.ID, key); err != nil || got != want {
			t.Fatalf("count = %d, err = %v, want %d", got, err, want)
		}
	}
	a, b := newAccount(), newAccount()
	record(a, "<m1@example.com>", 1)
	record(a, "<m1@example.com>", 2)
	record(a, "<m2@example.com>", 1)
	record(b, "<m1@example.com>", 1)
	if got, _ := s.RecordLedgerEmailRejection(ctx, "test-user", a.ID, "<m1@example.com>"); got != 1 {
		t.Errorf("refusal counter shares the extraction counter: %d", got)
	}

	if err := s.MarkLedgerEmailsSeen(ctx, "test-user", a.ID, EmailSeenUnusable, "<m1@example.com>"); err != nil {
		t.Fatalf("MarkLedgerEmailsSeen: %v", err)
	}
	record(a, "<m1@example.com>", 1)
	record(a, "<m2@example.com>", 2)

	order := LedgerEmailOrder{ID: uuid.New(), EmailAccountID: a.ID, EmailMessageID: "<m2@example.com>", TotalMinor: 100, Currency: "EUR", CreatedAt: now, UpdatedAt: now}
	if err := s.CreateScannedLedgerEmailOrder(ctx, "test-user", order); err != nil {
		t.Fatalf("CreateScannedLedgerEmailOrder: %v", err)
	}
	record(a, "<m2@example.com>", 1)

	if err := s.DeleteLedgerEmailAccount(ctx, "test-user", a.ID); err != nil {
		t.Fatalf("DeleteLedgerEmailAccount: %v", err)
	}
	record(a, "<m1@example.com>", 1)
	record(b, "<m1@example.com>", 2)
}

func TestCommitLedgerImport_WithoutFileHash(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	userID := "user-1"
	accountID := uuid.New()
	now := time.Now().UTC()

	if err := s.CreateLedgerAccount(ctx, userID, LedgerAccount{ID: accountID, Name: "Main", Bank: "DKB", Currency: "EUR", CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatalf("CreateLedgerAccount: %v", err)
	}

	for i := range 2 {
		batch := LedgerImportBatch{ID: uuid.New(), AccountID: accountID, SourceType: "enablebanking", ParserVersion: "1", Status: ImportStatusCommitted, CreatedAt: now, UpdatedAt: now}
		txns := []LedgerTransaction{{ID: uuid.New(), AccountID: accountID, BookingDate: "2026-04-01", Currency: "EUR", Fingerprint: fmt.Sprintf("fp-nohash-%d", i), ImportBatchID: batch.ID, CreatedAt: now, UpdatedAt: now}}
		result, err := s.CommitLedgerImport(ctx, userID, batch, txns)
		if err != nil {
			t.Fatalf("commit %d: %v", i, err)
		}
		if result.ImportedRows != 1 {
			t.Fatalf("commit %d imported %d, want 1", i, result.ImportedRows)
		}
	}

	err := s.e.View(func(txn *badger.Txn) error {
		it := txn.NewIterator(badger.DefaultIteratorOptions)
		defer it.Close()
		prefix := []byte(fmt.Sprintf("u/%s/mod/ledger/idx/file_hash/", userID))
		if it.Seek(prefix); it.ValidForPrefix(prefix) {
			t.Fatalf("unexpected file hash key %q", it.Item().Key())
		}
		return nil
	})
	if err != nil {
		t.Fatalf("scan: %v", err)
	}

	csv1 := LedgerImportBatch{ID: uuid.New(), AccountID: accountID, SourceType: "dkb.csv", ParserVersion: "1", Filename: "a.csv", FileSHA256: "csv-hash", Status: ImportStatusCommitted, CreatedAt: now, UpdatedAt: now}
	csv2 := csv1
	csv2.ID = uuid.New()
	if _, err := s.CommitLedgerImport(ctx, userID, csv1, nil); err != nil {
		t.Fatalf("csv commit: %v", err)
	}
	if _, err := s.CommitLedgerImport(ctx, userID, csv2, nil); !errors.Is(err, ErrLedgerFileImported) {
		t.Fatalf("expected ErrLedgerFileImported, got %v", err)
	}
}

func TestExportImportKeepsHashlessBatches(t *testing.T) {
	src := newTestStore(t)
	ctx := context.Background()
	accountID := uuid.New()
	now := time.Now().UTC()

	if err := src.CreateLedgerAccount(ctx, "user-1", LedgerAccount{ID: accountID, Name: "Main", Bank: "DKB", Currency: "EUR", CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatalf("CreateLedgerAccount: %v", err)
	}
	for i := range 2 {
		batch := LedgerImportBatch{ID: uuid.New(), AccountID: accountID, SourceType: "enablebanking", ParserVersion: "1", Status: ImportStatusCommitted, CreatedAt: now.Add(time.Duration(i) * time.Second), UpdatedAt: now}
		txns := []LedgerTransaction{{ID: uuid.New(), AccountID: accountID, BookingDate: "2026-04-01", Currency: "EUR", Fingerprint: fmt.Sprintf("fp-exp-%d", i), ImportBatchID: batch.ID, CreatedAt: now, UpdatedAt: now}}
		if _, err := src.CommitLedgerImport(ctx, "user-1", batch, txns); err != nil {
			t.Fatalf("commit %d: %v", i, err)
		}
	}

	srcMod := &Module{store: src.Store}
	data, err := srcMod.ExportSection(ctx, "user-1")
	if err != nil {
		t.Fatalf("ExportSection: %v", err)
	}

	dst := newTestStore(t)
	dstMod := &Module{store: dst.Store}
	res := module.NewImportResult()
	if err := dstMod.ImportSection(ctx, "user-2", data, res); err != nil {
		t.Fatalf("ImportSection: %v", err)
	}
	imports, err := dst.ListLedgerImports(ctx, "user-2")
	if err != nil {
		t.Fatalf("ListLedgerImports: %v", err)
	}
	if len(imports) != 2 {
		t.Fatalf("restored %d batches, want 2", len(imports))
	}
	txns, err := dst.ListLedgerTransactions(ctx, "user-2", accountID)
	if err != nil {
		t.Fatalf("ListLedgerTransactions: %v", err)
	}
	if len(txns) != 2 {
		t.Fatalf("restored %d transactions, want 2", len(txns))
	}
}
