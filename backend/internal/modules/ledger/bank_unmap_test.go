package ledger

import (
	"errors"
	"testing"

	"github.com/reusing-code/kontor/backend/internal/enablebanking"
	"github.com/reusing-code/kontor/backend/internal/storage"
)

func TestBank_UnmapAndRemap(t *testing.T) {
	f := newSyncFixture(t)
	f.fake.SetTransactions("uid-1", []enablebanking.Transaction{apiTxn("a", "2026-04-01", "10.00", "DBIT")})
	f.sync(t)

	if _, err := f.svc.UnmapAccount(t.Context(), testUserID, f.conn.ID, "uid-1"); err != nil {
		t.Fatalf("UnmapAccount: %v", err)
	}
	if _, err := f.svc.UnmapAccount(t.Context(), testUserID, f.conn.ID, "nope"); !errors.Is(err, storage.ErrNotFound) {
		t.Fatalf("unknown uid: %v", err)
	}
	// the IBAN still matches, but the auto-mapping leaves an unmapped account alone
	views, err := f.svc.Connections(t.Context(), testUserID)
	if err != nil || views[0].Accounts[0].Mapped {
		t.Fatalf("re-mapped by IBAN: %+v, %v", views, err)
	}
	if s := f.sync(t); s.Imported != 0 || s.Accounts[0].Status != BankSyncUnmapped {
		t.Fatalf("sync while unmapped = %+v", s)
	}
	if got := len(f.stored(t)); got != 1 {
		t.Fatalf("synced transactions must stay: %d", got)
	}

	other := f.addLedgerAccount(t, "", "EUR")
	if _, err := f.svc.MapAccount(t.Context(), testUserID, f.conn.ID, "uid-1", other); err != nil {
		t.Fatalf("MapAccount: %v", err)
	}
	a := f.account(t, "uid-1")
	if a.LedgerAccountID == nil || *a.LedgerAccountID != other || a.Unmapped || a.LastSyncAttemptAt != nil {
		t.Fatalf("after remap = %+v", a)
	}
}
