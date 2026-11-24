package ledger

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/reusing-code/kontor/backend/internal/module"
)

func TestExportImport_RoundTripsAllTransactions(t *testing.T) {
	src := newTestStore(t)
	ctx := context.Background()
	now := time.Now().UTC()
	accountID := uuid.New()
	missingAccountID := uuid.New()

	if err := src.CreateLedgerAccount(ctx, "user-1", LedgerAccount{ID: accountID, Name: "Main", Bank: "DKB", Currency: "EUR", CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatalf("CreateLedgerAccount: %v", err)
	}
	commit := func(account uuid.UUID, n int) {
		t.Helper()
		batch := LedgerImportBatch{ID: uuid.New(), AccountID: account, SourceType: "dkb", ParserVersion: "1", FileSHA256: uuid.NewString(), Status: ImportStatusCommitted, CreatedAt: now, UpdatedAt: now}
		txns := make([]LedgerTransaction, n)
		for i := range txns {
			txns[i] = LedgerTransaction{
				ID:            uuid.New(),
				AccountID:     account,
				BookingDate:   fmt.Sprintf("2026-%02d-%02d", 1+i%12, 1+i%28),
				AmountMinor:   int64(-100 - i),
				Currency:      "EUR",
				Fingerprint:   fmt.Sprintf("fp-%s-%d", account, i),
				ImportBatchID: batch.ID,
				CreatedAt:     now,
				UpdatedAt:     now,
			}
		}
		if _, err := src.CommitLedgerImport(ctx, "user-1", batch, txns); err != nil {
			t.Fatalf("CommitLedgerImport: %v", err)
		}
	}
	commit(accountID, 600)
	commit(missingAccountID, 3)

	data, err := (&Module{store: src.Store}).ExportSection(ctx, "user-1")
	if err != nil {
		t.Fatalf("ExportSection: %v", err)
	}
	var section exportSection
	if err := json.Unmarshal(data, &section); err != nil {
		t.Fatal(err)
	}
	if len(section.Transactions) != 603 {
		t.Fatalf("exported %d transactions, want 603", len(section.Transactions))
	}

	dst := newTestStore(t)
	if err := (&Module{store: dst.Store}).ImportSection(ctx, "user-2", data, module.NewImportResult()); err != nil {
		t.Fatalf("ImportSection: %v", err)
	}
	for account, want := range map[uuid.UUID]int{accountID: 600, missingAccountID: 3} {
		txns, err := dst.ListLedgerTransactions(ctx, "user-2", account)
		if err != nil {
			t.Fatalf("ListLedgerTransactions: %v", err)
		}
		if len(txns) != want {
			t.Errorf("account %s: restored %d transactions, want %d", account, len(txns), want)
		}
	}
}
