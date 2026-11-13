package ledger

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/dgraph-io/badger/v4"
	"github.com/google/uuid"
	"github.com/reusing-code/kontor/backend/internal/storage"
)

const bankTestSessionID = "SESSION-ID-PLAINTEXT-42"

func newTestBankConnection(now time.Time) LedgerBankConnection {
	return LedgerBankConnection{
		ID:                 uuid.New(),
		ASPSPName:          "DKB",
		ASPSPCountry:       "DE",
		EncryptedSessionID: "ciphertext-of-session",
		AuthorizedAt:       now,
		ValidUntil:         now.Add(90 * 24 * time.Hour),
		Accounts: []LedgerBankAccount{
			{UID: "uid-1", IBAN: "DE02120300000000202051", Currency: "EUR", Name: "Giro"},
			{UID: "uid-2", IBAN: "DE89370400440532013000", Currency: "EUR", Name: "Tagesgeld"},
		},
		CreatedAt: now,
		UpdatedAt: now,
	}
}

func TestLedgerBankConnection_RoundTripKeepsSessionEncrypted(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)
	c := newTestBankConnection(now)
	c.EncryptedSessionID = "enc:" + "abc123"

	if err := s.PutLedgerBankConnection(ctx, "user-1", c); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetLedgerBankConnection(ctx, "user-1", c.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.EncryptedSessionID != c.EncryptedSessionID || len(got.Accounts) != 2 || got.Accounts[1].UID != "uid-2" {
		t.Fatalf("got %+v", got)
	}

	if err := s.e.View(func(txn *badger.Txn) error {
		item, err := txn.Get(ledBankConnKey("user-1", c.ID))
		if err != nil {
			return err
		}
		return item.Value(func(val []byte) error {
			if !strings.Contains(string(val), `"encryptedSessionId":"enc:abc123"`) {
				t.Fatalf("stored JSON lacks the encrypted session: %s", val)
			}
			return nil
		})
	}); err != nil {
		t.Fatal(err)
	}

	public, err := json.Marshal(c)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(public), "enc:abc123") || strings.Contains(strings.ToLower(string(public)), "session") {
		t.Fatalf("public JSON exposes the session: %s", public)
	}
	view, err := json.Marshal(NewBankConnectionView(c, now))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(strings.ToLower(string(view)), "session") || strings.Contains(string(view), "identificationHash") {
		t.Fatalf("view exposes internals: %s", view)
	}
}

func TestLedgerBankConnection_StoredJSONHasNoPlaintextSession(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	now := time.Now().UTC()
	enc, err := encryptBankTestSession(bankTestSessionID)
	if err != nil {
		t.Fatal(err)
	}
	c := newTestBankConnection(now)
	c.EncryptedSessionID = enc
	if err := s.PutLedgerBankConnection(ctx, "user-1", c); err != nil {
		t.Fatal(err)
	}
	err = s.e.View(func(txn *badger.Txn) error {
		item, err := txn.Get(ledBankConnKey("user-1", c.ID))
		if err != nil {
			return err
		}
		return item.Value(func(val []byte) error {
			if strings.Contains(string(val), bankTestSessionID) {
				t.Fatal("plaintext session ID stored")
			}
			return nil
		})
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestLedgerBankConnection_ListFindDelete(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	now := time.Now().UTC()

	if _, err := s.GetLedgerBankConnection(ctx, "user-1", uuid.New()); !errors.Is(err, storage.ErrNotFound) {
		t.Fatalf("get missing: %v", err)
	}
	if err := s.DeleteLedgerBankConnection(ctx, "user-1", uuid.New()); !errors.Is(err, storage.ErrNotFound) {
		t.Fatalf("delete missing: %v", err)
	}

	zeta := newTestBankConnection(now)
	zeta.ASPSPName = "Zeta"
	alpha := newTestBankConnection(now)
	alpha.ASPSPName = "alpha"
	for _, c := range []LedgerBankConnection{zeta, alpha} {
		if err := s.PutLedgerBankConnection(ctx, "user-1", c); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.PutLedgerBankConnection(ctx, "user-2", newTestBankConnection(now)); err != nil {
		t.Fatal(err)
	}

	list, err := s.ListLedgerBankConnections(ctx, "user-1")
	if err != nil || len(list) != 2 || list[0].ASPSPName != "alpha" {
		t.Fatalf("list = %+v, %v", list, err)
	}
	found, err := s.FindLedgerBankConnectionByASPSP(ctx, "user-1", "ZETA", "de")
	if err != nil || found.ID != zeta.ID {
		t.Fatalf("find = %+v, %v", found, err)
	}
	if _, err := s.FindLedgerBankConnectionByASPSP(ctx, "user-1", "Zeta", "FI"); !errors.Is(err, storage.ErrNotFound) {
		t.Fatalf("find other country: %v", err)
	}
	if err := s.DeleteLedgerBankConnection(ctx, "user-1", zeta.ID); err != nil {
		t.Fatal(err)
	}
	if list, _ = s.ListLedgerBankConnections(ctx, "user-1"); len(list) != 1 {
		t.Fatalf("after delete: %d", len(list))
	}
}

func TestModifyLedgerBankConnection_ConcurrentAccountUpdates(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	c := newTestBankConnection(time.Now().UTC())
	if err := s.PutLedgerBankConnection(ctx, "user-1", c); err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for i, uid := range []string{"uid-1", "uid-2"} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := s.ModifyLedgerBankConnection(ctx, "user-1", c.ID, func(c *LedgerBankConnection) {
				for j := range c.Accounts {
					if c.Accounts[j].UID == uid {
						c.Accounts[j].LastImported = i + 10
					}
				}
			})
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	got, err := s.GetLedgerBankConnection(ctx, "user-1", c.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Accounts[0].LastImported != 10 || got.Accounts[1].LastImported != 11 {
		t.Fatalf("lost update: %+v", got.Accounts)
	}
	if _, err := s.ModifyLedgerBankConnection(ctx, "user-1", uuid.New(), func(*LedgerBankConnection) {}); !errors.Is(err, storage.ErrNotFound) {
		t.Fatalf("modify missing: %v", err)
	}
}

func TestLedgerBankAuthState_SingleUseExpiryAndPrune(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	now := time.Date(2026, 11, 13, 10, 0, 0, 0, time.UTC)
	state := BankAuthState{ASPSPName: "DKB", ASPSPCountry: "DE", CreatedAt: now, ExpiresAt: now.Add(bankAuthStateTTL)}

	if err := s.CreateLedgerBankAuthState(ctx, "user-1", "hash-a", state); err != nil {
		t.Fatal(err)
	}
	got, err := s.ConsumeLedgerBankAuthState(ctx, "user-1", "hash-a", now.Add(time.Minute))
	if err != nil || got.ASPSPName != "DKB" {
		t.Fatalf("consume = %+v, %v", got, err)
	}
	if _, err := s.ConsumeLedgerBankAuthState(ctx, "user-1", "hash-a", now.Add(time.Minute)); !errors.Is(err, ErrBankStateInvalid) {
		t.Fatalf("reuse: %v", err)
	}
	if _, err := s.ConsumeLedgerBankAuthState(ctx, "user-2", "hash-a", now); !errors.Is(err, ErrBankStateInvalid) {
		t.Fatalf("other user: %v", err)
	}

	if err := s.CreateLedgerBankAuthState(ctx, "user-1", "hash-b", state); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ConsumeLedgerBankAuthState(ctx, "user-1", "hash-b", state.ExpiresAt); !errors.Is(err, ErrBankStateInvalid) {
		t.Fatalf("expired: %v", err)
	}
	if err := s.e.View(func(txn *badger.Txn) error {
		if _, err := txn.Get(ledBankStateKey("user-1", "hash-b")); !errors.Is(err, badger.ErrKeyNotFound) {
			t.Fatalf("expired state not deleted on touch: %v", err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	for _, h := range []string{"old-1", "old-2"} {
		if err := s.CreateLedgerBankAuthState(ctx, "user-1", h, state); err != nil {
			t.Fatal(err)
		}
	}
	fresh := state
	fresh.ExpiresAt = now.Add(time.Hour)
	if err := s.CreateLedgerBankAuthState(ctx, "user-1", "fresh", fresh); err != nil {
		t.Fatal(err)
	}
	n, err := s.PruneLedgerBankAuthStates(ctx, "user-1", now.Add(30*time.Minute))
	if err != nil || n != 2 {
		t.Fatalf("prune = %d, %v", n, err)
	}
	if _, err := s.ConsumeLedgerBankAuthState(ctx, "user-1", "fresh", now.Add(30*time.Minute)); err != nil {
		t.Fatalf("fresh state pruned: %v", err)
	}

	if err := s.CreateLedgerBankAuthState(ctx, "user-1", "gone", state); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteLedgerBankAuthState(ctx, "user-1", "gone"); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteLedgerBankAuthState(ctx, "user-1", "gone"); err != nil {
		t.Fatalf("deleting a missing state must not fail: %v", err)
	}
}

func TestBankConnectionStatus(t *testing.T) {
	now := time.Date(2026, 11, 13, 10, 0, 0, 0, time.UTC)
	lost := now.Add(-time.Hour)
	tests := []struct {
		name string
		c    LedgerBankConnection
		want string
	}{
		{"far", LedgerBankConnection{ValidUntil: now.Add(60 * 24 * time.Hour)}, BankStatusActive},
		{"exactly 14 days", LedgerBankConnection{ValidUntil: now.Add(bankExpiringWindow)}, BankStatusActive},
		{"just under 14 days", LedgerBankConnection{ValidUntil: now.Add(bankExpiringWindow - time.Second)}, BankStatusExpiring},
		{"one day", LedgerBankConnection{ValidUntil: now.Add(24 * time.Hour)}, BankStatusExpiring},
		{"at validUntil", LedgerBankConnection{ValidUntil: now}, BankStatusExpired},
		{"past", LedgerBankConnection{ValidUntil: now.Add(-time.Hour)}, BankStatusExpired},
		{"lost", LedgerBankConnection{ValidUntil: now.Add(60 * 24 * time.Hour), LostAt: &lost}, BankStatusExpired},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := BankConnectionStatus(tt.c, now); got != tt.want {
				t.Fatalf("status = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestNormalizeIBAN(t *testing.T) {
	for in, want := range map[string]string{
		"de02 1203 0000 0000 2020 51":  "DE02120300000000202051",
		" de02120300000000202051\t": "DE02120300000000202051",
		"":                          "",
	} {
		if got := normalizeIBAN(in); got != want {
			t.Errorf("normalizeIBAN(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestNewBankConnectionView_StatusMessages(t *testing.T) {
	now := time.Now().UTC()
	mapped := uuid.New()
	c := newTestBankConnection(now)
	c.Accounts[0].LedgerAccountID = &mapped
	c.Accounts[0].LastSyncStatus = BankSyncOK
	c.Accounts[0].LastImported = 3
	c.Accounts[0].LastDuplicates = 7

	view := NewBankConnectionView(c, now)
	if view.Status != BankStatusActive || len(view.Accounts) != 2 {
		t.Fatalf("view = %+v", view)
	}
	if a := view.Accounts[0]; !a.Mapped || a.LastSyncStatus != BankSyncOK || !strings.Contains(a.StatusMessage, "3 imported") {
		t.Fatalf("mapped account = %+v", a)
	}
	if a := view.Accounts[1]; a.Mapped || a.LastSyncStatus != BankSyncUnmapped {
		t.Fatalf("unmapped account = %+v", a)
	}
	for _, code := range []string{"", BankSyncOK, BankSyncUnmapped, BankSyncConsentExpired, BankSyncAccessDenied, BankSyncAccountNotAccessible,
		BankSyncRateLimited, BankSyncBankUnavailable, BankSyncAppError, BankSyncTimeout, BankSyncError} {
		if bankStatusMessage(code, 0, 0) == "" {
			t.Errorf("no message for %q", code)
		}
	}
}
