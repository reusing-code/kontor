package ledger

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
}

func dkbTestCSV() string {
	return "\xEF\xBB\xBF" + `"Girokonto";"DE12345678901234567890"
""
"Kontostand vom DD.MM.YYYY:";"111.111,11 €"
""
"Buchungsdatum";"Wertstellung";"Status";"Zahlungspflichtige*r";"Zahlungsempfänger*in";"Verwendungszweck";"Umsatztyp";"IBAN";"Betrag (€)";"Gläubiger-ID";"Mandatsreferenz";"Kundenreferenz"
"07.04.26";"02.04.26";"Gebucht";"DKB AG";"Mustermann,Fred";"Depot 0123";"Eingang";"0000000000";"800,23";"";"";""
"01.04.26";"01.04.26";"Gebucht";"Fred";"Fred";"Transfer";"Eingang";"DE11222233334444555566";"50,00";"";"";""
`
}

func createDKBAccount(t *testing.T, s *testStores) uuid.UUID {
	t.Helper()
	now := time.Now().UTC()
	accID := uuid.New()
	if err := s.CreateLedgerAccount(context.Background(), "user1", LedgerAccount{
		ID:        accID,
		Name:      "Main",
		Bank:      "DKB",
		IBAN:      "DE12345678901234567890",
		Currency:  "EUR",
		CreatedAt: now,
		UpdatedAt: now,
	}); err != nil {
		t.Fatalf("CreateLedgerAccount: %v", err)
	}
	return accID
}

func TestServicePreview_DKB_AutoResolveAccount(t *testing.T) {
	s := newTestStore(t)
	accID := createDKBAccount(t, s)

	svc := NewImportService(s.Store, testLogger())
	result, err := svc.Preview(context.Background(), PreviewRequest{
		File:       strings.NewReader(dkbTestCSV()),
		Filename:   "test.csv",
		SourceType: SourceDKBCSV,
		UserID:     "user1",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.AccountID != accID.String() {
		t.Errorf("AccountID = %q, want %q", result.AccountID, accID.String())
	}
	if result.TotalRows != 2 {
		t.Errorf("TotalRows = %d, want 2", result.TotalRows)
	}
	if result.NewRows != 2 {
		t.Errorf("NewRows = %d, want 2", result.NewRows)
	}
	if result.DuplicateRows != 0 {
		t.Errorf("DuplicateRows = %d, want 0", result.DuplicateRows)
	}
	if result.PreviewID == "" {
		t.Error("PreviewID should not be empty")
	}
}

func TestServicePreview_DKB_NoAccount(t *testing.T) {
	s := newTestStore(t)
	svc := NewImportService(s.Store, testLogger())

	result, err := svc.Preview(context.Background(), PreviewRequest{
		File:       strings.NewReader(dkbTestCSV()),
		Filename:   "test.csv",
		SourceType: SourceDKBCSV,
		UserID:     "user1",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.AccountID != "" {
		t.Errorf("AccountID = %q, want empty (no matching account)", result.AccountID)
	}
	if result.IBAN != "DE12345678901234567890" {
		t.Errorf("IBAN = %q, want DE12345678901234567890", result.IBAN)
	}
}

func TestServicePreview_DuplicateFile(t *testing.T) {
	s := newTestStore(t)
	createDKBAccount(t, s)
	svc := NewImportService(s.Store, testLogger())

	preview, err := svc.Preview(context.Background(), PreviewRequest{
		File:       strings.NewReader(dkbTestCSV()),
		Filename:   "test.csv",
		SourceType: SourceDKBCSV,
		UserID:     "user1",
	})
	if err != nil {
		t.Fatalf("first preview: %v", err)
	}
	if _, err := svc.Commit(context.Background(), CommitRequest{
		PreviewID: preview.PreviewID,
		UserID:    "user1",
	}); err != nil {
		t.Fatalf("commit: %v", err)
	}

	// The file hash is registered on commit, so previewing the exact same
	// file again must be rejected.
	_, err = svc.Preview(context.Background(), PreviewRequest{
		File:       strings.NewReader(dkbTestCSV()),
		Filename:   "test.csv",
		SourceType: SourceDKBCSV,
		UserID:     "user1",
	})
	if !errors.Is(err, ErrLedgerFileImported) {
		t.Fatalf("expected ErrLedgerFileImported, got %v", err)
	}
}

func TestServiceCommit_NewAccount(t *testing.T) {
	s := newTestStore(t)
	svc := NewImportService(s.Store, testLogger())
	ctx := context.Background()

	preview, err := svc.Preview(ctx, PreviewRequest{
		File:       strings.NewReader(dkbTestCSV()),
		Filename:   "test.csv",
		SourceType: SourceDKBCSV,
		UserID:     "user1",
	})
	if err != nil {
		t.Fatalf("preview: %v", err)
	}

	result, err := svc.Commit(ctx, CommitRequest{
		PreviewID: preview.PreviewID,
		NewAccount: &LedgerAccountInput{
			Name:     "My DKB Account",
			Bank:     "DKB",
			IBAN:     "DE12345678901234567890",
			Currency: "EUR",
		},
		UserID: "user1",
	})
	if err != nil {
		t.Fatalf("commit: %v", err)
	}

	if result.ImportedRows != 2 {
		t.Errorf("ImportedRows = %d, want 2", result.ImportedRows)
	}
	if result.DuplicateRows != 0 {
		t.Errorf("DuplicateRows = %d, want 0", result.DuplicateRows)
	}
	accounts, err := s.ListLedgerAccounts(ctx, "user1")
	if err != nil {
		t.Fatalf("ListLedgerAccounts: %v", err)
	}
	if len(accounts) != 1 {
		t.Errorf("accounts count = %d, want 1", len(accounts))
	}
	txns, err := s.ListLedgerTransactions(ctx, "user1", result.AccountID)
	if err != nil {
		t.Fatalf("ListLedgerTransactions: %v", err)
	}
	if len(txns) != 2 {
		t.Errorf("transactions count = %d, want 2", len(txns))
	}
}

func TestServiceCommit_ExistingAccount(t *testing.T) {
	s := newTestStore(t)
	accID := createDKBAccount(t, s)
	svc := NewImportService(s.Store, testLogger())
	ctx := context.Background()

	preview, err := svc.Preview(ctx, PreviewRequest{
		File:       strings.NewReader(dkbTestCSV()),
		Filename:   "test.csv",
		SourceType: SourceDKBCSV,
		UserID:     "user1",
	})
	if err != nil {
		t.Fatalf("preview: %v", err)
	}

	result, err := svc.Commit(ctx, CommitRequest{
		PreviewID: preview.PreviewID,
		UserID:    "user1",
	})
	if err != nil {
		t.Fatalf("commit: %v", err)
	}

	if result.AccountID != accID {
		t.Errorf("AccountID = %v, want %v", result.AccountID, accID)
	}
	if result.ImportedRows != 2 {
		t.Errorf("ImportedRows = %d, want 2", result.ImportedRows)
	}
}

func TestServiceCommit_DeduplicatesOnSecondImport(t *testing.T) {
	s := newTestStore(t)
	accID := createDKBAccount(t, s)
	svc := NewImportService(s.Store, testLogger())
	ctx := context.Background()

	// First import
	preview1, err := svc.Preview(ctx, PreviewRequest{
		File:       strings.NewReader(dkbTestCSV()),
		Filename:   "test.csv",
		SourceType: SourceDKBCSV,
		UserID:     "user1",
	})
	if err != nil {
		t.Fatalf("first preview: %v", err)
	}
	_, err = svc.Commit(ctx, CommitRequest{
		PreviewID: preview1.PreviewID,
		UserID:    "user1",
	})
	if err != nil {
		t.Fatalf("first commit: %v", err)
	}

	// Second import with different file content but overlapping transactions
	csv2 := "\xEF\xBB\xBF" + `"Girokonto";"DE12345678901234567890"
""
"Kontostand vom DD.MM.YYYY:";"999,99 €"
""
"Buchungsdatum";"Wertstellung";"Status";"Zahlungspflichtige*r";"Zahlungsempfänger*in";"Verwendungszweck";"Umsatztyp";"IBAN";"Betrag (€)";"Gläubiger-ID";"Mandatsreferenz";"Kundenreferenz"
"07.04.26";"02.04.26";"Gebucht";"DKB AG";"Mustermann,Fred";"Depot 0123";"Eingang";"0000000000";"800,23";"";"";""
"10.04.26";"10.04.26";"Gebucht";"New Sender";"New Receiver";"New purpose";"Ausgang";"DE99999999999999999999";"-25,00";"";"";""
`
	preview2, err := svc.Preview(ctx, PreviewRequest{
		File:       strings.NewReader(csv2),
		Filename:   "test2.csv",
		SourceType: SourceDKBCSV,
		UserID:     "user1",
	})
	if err != nil {
		t.Fatalf("second preview: %v", err)
	}

	if preview2.DuplicateRows != 1 {
		t.Errorf("second preview DuplicateRows = %d, want 1", preview2.DuplicateRows)
	}
	if preview2.NewRows != 1 {
		t.Errorf("second preview NewRows = %d, want 1", preview2.NewRows)
	}

	result2, err := svc.Commit(ctx, CommitRequest{
		PreviewID: preview2.PreviewID,
		UserID:    "user1",
	})
	if err != nil {
		t.Fatalf("second commit: %v", err)
	}

	if result2.ImportedRows != 1 {
		t.Errorf("second commit ImportedRows = %d, want 1", result2.ImportedRows)
	}
	if result2.DuplicateRows != 1 {
		t.Errorf("second commit DuplicateRows = %d, want 1", result2.DuplicateRows)
	}

	// Total transactions should be 3 (2 from first + 1 new from second)
	txns, err := s.ListLedgerTransactions(ctx, "user1", accID)
	if err != nil {
		t.Fatalf("ListLedgerTransactions: %v", err)
	}
	if len(txns) != 3 {
		t.Errorf("total transactions = %d, want 3", len(txns))
	}
}

func TestServiceCommit_ExpiredPreview(t *testing.T) {
	s := newTestStore(t)
	svc := NewImportService(s.Store, testLogger())

	_, err := svc.Commit(context.Background(), CommitRequest{
		PreviewID: "nonexistent",
		AccountID: uuid.New().String(),
		UserID:    "user1",
	})
	if err == nil {
		t.Fatal("expected error for nonexistent preview")
	}
}

func TestServiceCommit_NoAccountProvided(t *testing.T) {
	s := newTestStore(t)
	svc := NewImportService(s.Store, testLogger())

	preview, err := svc.Preview(context.Background(), PreviewRequest{
		File:       strings.NewReader(dkbTestCSV()),
		Filename:   "test.csv",
		SourceType: SourceDKBCSV,
		UserID:     "user1",
	})
	if err != nil {
		t.Fatalf("preview: %v", err)
	}

	// No account and no new account provided
	_, err = svc.Commit(context.Background(), CommitRequest{
		PreviewID: preview.PreviewID,
		UserID:    "user1",
	})
	if err == nil {
		t.Fatal("expected error when no account is provided")
	}
}
