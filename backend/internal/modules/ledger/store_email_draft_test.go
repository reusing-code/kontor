package ledger

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
)

func newDraftTestAccount(t *testing.T, s *Store, name string) LedgerEmailAccount {
	t.Helper()
	now := time.Now().UTC()
	a := LedgerEmailAccount{ID: uuid.New(), Name: name, IMAPHost: "h", IMAPPort: 993, Username: "u",
		Folder: "Orders", DraftsFolder: "Kontor", CreatedAt: now, UpdatedAt: now}
	if err := s.CreateLedgerEmailAccount(context.Background(), "test-user", a); err != nil {
		t.Fatalf("CreateLedgerEmailAccount: %v", err)
	}
	return a
}

func TestAcceptLedgerEmailDraft_LinksAndMarks(t *testing.T) {
	s := newTestStore(t).Store
	ctx := context.Background()
	account := newDraftTestAccount(t, s, "Mail")
	entityID := uuid.NewString()
	date := time.Date(2026, 11, 1, 10, 0, 0, 0, time.UTC)
	const key = "<a/b@example.org>"
	if err := s.putLedgerEmailDraftMessage("test-user", account.ID, key,
		ledgerEmailDraftMessage{MessageID: key, Subject: "Your contract", From: "Provider", Date: &date}); err != nil {
		t.Fatal(err)
	}

	linked, err := s.AcceptLedgerEmailDraft(ctx, "test-user", account.ID, key, EmailLinkContract, entityID)
	if err != nil || !linked {
		t.Fatalf("AcceptLedgerEmailDraft = %v, %v", linked, err)
	}
	if seen, _ := s.HasLedgerEmailDraftSeen(ctx, "test-user", account.ID, key); !seen {
		t.Error("accepted message has no marker")
	}
	if seen, _ := s.HasLedgerEmailSeen(ctx, "test-user", account.ID, key); seen {
		t.Error("the draft marker leaked into the order markers")
	}
	links, err := s.ListLedgerEmailLinks(ctx, "test-user", EmailLinkContract, entityID)
	if err != nil || len(links) != 1 {
		t.Fatalf("links = %+v, %v", links, err)
	}
	got := links[0]
	if got.MessageID != key || got.Subject != "Your contract" || got.From != "Provider" || got.AccountName != "Mail" ||
		got.EmailAccountID != account.ID || got.Date == nil || !got.Date.Equal(date) || got.EntityID != entityID {
		t.Errorf("link = %+v", got)
	}
	if other, _ := s.ListLedgerEmailLinks(ctx, "test-user", EmailLinkPurchase, entityID); len(other) != 0 {
		t.Errorf("link listed for another entity type: %+v", other)
	}

	// a second accept finds no message data: marker only
	linked, err = s.AcceptLedgerEmailDraft(ctx, "test-user", account.ID, key, EmailLinkContract, uuid.NewString())
	if err != nil || linked {
		t.Fatalf("accept without message data = %v, %v", linked, err)
	}
}

func TestLedgerEmailDraft_RejectAndMissingAccount(t *testing.T) {
	s := newTestStore(t).Store
	ctx := context.Background()
	account := newDraftTestAccount(t, s, "Mail")
	if err := s.putLedgerEmailDraftMessage("test-user", account.ID, "k", ledgerEmailDraftMessage{Subject: "s"}); err != nil {
		t.Fatal(err)
	}
	if err := s.DecideLedgerEmailDraft(ctx, "test-user", account.ID, "k", EmailDraftSeenRejected); err != nil {
		t.Fatal(err)
	}
	if seen, _ := s.HasLedgerEmailDraftSeen(ctx, "test-user", account.ID, "k"); !seen {
		t.Error("rejected message has no marker")
	}
	if linked, err := s.AcceptLedgerEmailDraft(ctx, "test-user", account.ID, "k", EmailLinkCost, uuid.NewString()); err != nil || linked {
		t.Errorf("message data survived the rejection: %v, %v", linked, err)
	}

	gone := uuid.New()
	if linked, err := s.AcceptLedgerEmailDraft(ctx, "test-user", gone, "k", EmailLinkCost, uuid.NewString()); err != nil || linked {
		t.Errorf("accept for a deleted account = %v, %v", linked, err)
	}
	if err := s.DecideLedgerEmailDraft(ctx, "test-user", gone, "k", EmailDraftSeenRejected); err != nil {
		t.Errorf("reject for a deleted account: %v", err)
	}
	if seen, _ := s.HasLedgerEmailDraftSeen(ctx, "test-user", gone, "k"); seen {
		t.Error("marker written for a deleted account")
	}
}

func TestDeleteLedgerEmailAccount_RemovesItsDraftData(t *testing.T) {
	s := newTestStore(t).Store
	ctx := context.Background()
	doomed, kept := newDraftTestAccount(t, s, "A"), newDraftTestAccount(t, s, "B")
	entityID := uuid.NewString()
	for _, a := range []LedgerEmailAccount{doomed, kept} {
		for _, key := range []string{"linked", "pending"} {
			if err := s.putLedgerEmailDraftMessage("test-user", a.ID, key, ledgerEmailDraftMessage{Subject: key}); err != nil {
				t.Fatal(err)
			}
		}
		if _, err := s.AcceptLedgerEmailDraft(ctx, "test-user", a.ID, "linked", EmailLinkPurchase, entityID); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.DeleteLedgerEmailAccount(ctx, "test-user", doomed.ID); err != nil {
		t.Fatal(err)
	}
	links, err := s.ListLedgerEmailLinks(ctx, "test-user", EmailLinkPurchase, entityID)
	if err != nil || len(links) != 1 || links[0].EmailAccountID != kept.ID {
		t.Fatalf("links after delete = %+v, %v", links, err)
	}
	if seen, _ := s.HasLedgerEmailDraftSeen(ctx, "test-user", kept.ID, "linked"); !seen {
		t.Error("marker of the other account was removed")
	}
	// recreate the deleted account's ID: nothing of it may be left
	doomed.CreatedAt = time.Now().UTC()
	if err := s.CreateLedgerEmailAccount(ctx, "test-user", doomed); err != nil {
		t.Fatal(err)
	}
	if seen, _ := s.HasLedgerEmailDraftSeen(ctx, "test-user", doomed.ID, "linked"); seen {
		t.Error("marker of the deleted account survived")
	}
	if linked, _ := s.AcceptLedgerEmailDraft(ctx, "test-user", doomed.ID, "pending", EmailLinkPurchase, uuid.NewString()); linked {
		t.Error("pending message data of the deleted account survived")
	}
}
