package ledger

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
)

func newPendingTestAccount(t *testing.T, s *Store, action, draftsAction string) LedgerEmailAccount {
	t.Helper()
	now := time.Now().UTC()
	a := LedgerEmailAccount{ID: uuid.New(), Name: "Mail", IMAPHost: "h", IMAPPort: 993, Username: "u",
		Folder: "Orders", DraftsFolder: "Kontor", ProcessedAction: action, ProcessedFolder: "Done",
		DraftsProcessedAction: draftsAction, DraftsProcessedFolder: "Done", CreatedAt: now, UpdatedAt: now}
	if err := s.CreateLedgerEmailAccount(context.Background(), "test-user", a); err != nil {
		t.Fatalf("CreateLedgerEmailAccount: %v", err)
	}
	return a
}

func pendingKeys(t *testing.T, s *Store, account LedgerEmailAccount, source string) []ledgerEmailPendingEntry {
	t.Helper()
	entries, err := s.ListLedgerEmailPending(context.Background(), "test-user", account.ID, source)
	if err != nil {
		t.Fatalf("ListLedgerEmailPending: %v", err)
	}
	return entries
}

func scannedOrder(account LedgerEmailAccount, key string) LedgerEmailOrder {
	now := time.Now().UTC()
	return LedgerEmailOrder{ID: uuid.New(), EmailAccountID: account.ID, ImporterID: LedgerEmailImporterLLM, OrderDate: "2026-11-01",
		TotalMinor: 100, Currency: "EUR", EmailMessageID: key, MatchStatus: LedgerEmailOrderStatusUnmatched, CreatedAt: now, UpdatedAt: now}
}

func TestLedgerEmailPending_QueuedWithOrdersAndMarkers(t *testing.T) {
	s := newTestStore(t).Store
	ctx := context.Background()
	account := newPendingTestAccount(t, s, EmailActionMove, EmailActionKeep)
	ref := &emailMailboxRef{UID: 7, UIDValidity: 3}

	if err := s.CreateScannedLedgerEmailOrder(ctx, "test-user", scannedOrder(account, "<order/1@example.org>"), ref); err != nil {
		t.Fatalf("CreateScannedLedgerEmailOrder: %v", err)
	}
	if err := s.MarkLedgerEmailsProcessed(ctx, "test-user", account.ID, EmailSeenNotOrder,
		emailProcessed{Key: "<news@example.org>", Mailbox: &emailMailboxRef{UID: 8, UIDValidity: 3}},
		emailProcessed{Key: "<upload@example.org>"}); err != nil {
		t.Fatalf("MarkLedgerEmailsProcessed: %v", err)
	}
	// markers of messages Kontor could not handle queue nothing
	if err := s.MarkLedgerEmailsSeen(ctx, "test-user", account.ID, EmailSeenUnusable, "<broken@example.org>"); err != nil {
		t.Fatalf("MarkLedgerEmailsSeen: %v", err)
	}
	// an uploaded order has no mailbox message
	if err := s.CreateScannedLedgerEmailOrder(ctx, "test-user", scannedOrder(account, "<order/2@example.org>"), nil); err != nil {
		t.Fatalf("CreateScannedLedgerEmailOrder: %v", err)
	}

	got := pendingKeys(t, s, account, emailSourceOrders)
	if len(got) != 2 || got[0].Key != "<order/1@example.org>" || got[0].UID != 7 || got[0].UIDValidity != 3 ||
		got[1].Key != "<news@example.org>" || got[1].UID != 8 {
		t.Fatalf("pending = %+v", got)
	}
	for _, key := range []string{"<news@example.org>", "<upload@example.org>", "<broken@example.org>"} {
		if seen, _ := s.HasLedgerEmailSeen(ctx, "test-user", account.ID, key); !seen {
			t.Errorf("%s has no marker", key)
		}
	}
	if drafts := pendingKeys(t, s, account, emailSourceDrafts); len(drafts) != 0 {
		t.Errorf("order actions listed for the drafts folder: %+v", drafts)
	}
}

func TestLedgerEmailPending_NothingQueuedWhileTheActionIsKeep(t *testing.T) {
	s := newTestStore(t).Store
	ctx := context.Background()
	account := newPendingTestAccount(t, s, "", EmailActionKeep)
	ref := &emailMailboxRef{UID: 1, UIDValidity: 1}
	const key = "<a@example.org>"

	if err := s.CreateScannedLedgerEmailOrder(ctx, "test-user", scannedOrder(account, key), ref); err != nil {
		t.Fatal(err)
	}
	if err := s.MarkLedgerEmailsProcessed(ctx, "test-user", account.ID, EmailSeenPreClass, emailProcessed{Key: "<b@example.org>", Mailbox: ref}); err != nil {
		t.Fatal(err)
	}
	if err := s.DecideLedgerEmailDraft(ctx, "test-user", account.ID, key, EmailDraftSeenRejected); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AcceptLedgerEmailDraft(ctx, "test-user", account.ID, "<c@example.org>", EmailLinkContract, uuid.NewString()); err != nil {
		t.Fatal(err)
	}
	if got := append(pendingKeys(t, s, account, emailSourceOrders), pendingKeys(t, s, account, emailSourceDrafts)...); len(got) != 0 {
		t.Errorf("pending = %+v, want none", got)
	}
}

func TestLedgerEmailPending_QueuedWithDraftDecisions(t *testing.T) {
	s := newTestStore(t).Store
	ctx := context.Background()
	account := newPendingTestAccount(t, s, EmailActionKeep, EmailActionDelete)
	put := func(key string, uid uint32) {
		t.Helper()
		if err := s.putLedgerEmailDraftMessage("test-user", account.ID, key,
			ledgerEmailDraftMessage{MessageID: key, Subject: "S", emailMailboxRef: emailMailboxRef{UID: uid, UIDValidity: 9}}); err != nil {
			t.Fatal(err)
		}
	}
	put("<accepted@example.org>", 1)
	put("<rejected@example.org>", 2)
	put("<other-type@example.org>", 3)

	if linked, err := s.AcceptLedgerEmailDraft(ctx, "test-user", account.ID, "<accepted@example.org>", EmailLinkContract, uuid.NewString()); err != nil || !linked {
		t.Fatalf("accept = %v, %v", linked, err)
	}
	if err := s.DecideLedgerEmailDraft(ctx, "test-user", account.ID, "<rejected@example.org>", EmailDraftSeenRejected); err != nil {
		t.Fatal(err)
	}
	if err := s.DecideLedgerEmailDraft(ctx, "test-user", account.ID, "<other-type@example.org>", EmailDraftSeenAccepted); err != nil {
		t.Fatal(err)
	}
	// a draft from before the mailbox position was stored: found by Message-ID later
	if linked, err := s.AcceptLedgerEmailDraft(ctx, "test-user", account.ID, "<old@example.org>", EmailLinkContract, uuid.NewString()); err != nil || linked {
		t.Fatalf("accept without message data = %v, %v", linked, err)
	}

	got := map[string]uint32{}
	for _, e := range pendingKeys(t, s, account, emailSourceDrafts) {
		if e.UID != 0 && e.UIDValidity != 9 {
			t.Errorf("%s: uidValidity = %d, want 9", e.Key, e.UIDValidity)
		}
		got[e.Key] = e.UID
	}
	want := map[string]uint32{"<accepted@example.org>": 1, "<rejected@example.org>": 2, "<other-type@example.org>": 3, "<old@example.org>": 0}
	if len(got) != len(want) {
		t.Fatalf("pending = %v, want %v", got, want)
	}
	for key, uid := range want {
		if got[key] != uid {
			t.Errorf("%s: uid = %d, want %d", key, got[key], uid)
		}
	}
	if orders := pendingKeys(t, s, account, emailSourceOrders); len(orders) != 0 {
		t.Errorf("draft actions listed for the order folder: %+v", orders)
	}
}

func TestLedgerEmailPending_FailureCountClearAndAccountDeletion(t *testing.T) {
	s := newTestStore(t).Store
	ctx := context.Background()
	account := newPendingTestAccount(t, s, EmailActionMove, EmailActionMove)
	ref := &emailMailboxRef{UID: 1, UIDValidity: 1}
	mark := func(keys ...string) {
		t.Helper()
		for _, key := range keys {
			if err := s.MarkLedgerEmailsProcessed(ctx, "test-user", account.ID, EmailSeenNotOrder, emailProcessed{Key: key, Mailbox: ref}); err != nil {
				t.Fatal(err)
			}
		}
	}
	mark("<a@example.org>", "<b@example.org>")

	for want := 1; want <= 2; want++ {
		if got, err := s.RecordLedgerEmailPendingFailure(ctx, "test-user", account.ID, emailSourceOrders, "<a@example.org>"); err != nil || got != want {
			t.Fatalf("failure count = %d, %v; want %d", got, err, want)
		}
	}
	if got, err := s.RecordLedgerEmailPendingFailure(ctx, "test-user", account.ID, emailSourceOrders, "<gone@example.org>"); err != nil || got != 0 {
		t.Fatalf("failure of an unknown record = %d, %v; want 0 and no new record", got, err)
	}
	if err := s.DeleteLedgerEmailPending(ctx, "test-user", account.ID, emailSourceOrders, "<a@example.org>"); err != nil {
		t.Fatal(err)
	}
	if got := pendingKeys(t, s, account, emailSourceOrders); len(got) != 1 || got[0].Key != "<b@example.org>" || got[0].Attempts != 0 {
		t.Fatalf("pending = %+v", got)
	}

	if err := s.DecideLedgerEmailDraft(ctx, "test-user", account.ID, "<d@example.org>", EmailDraftSeenRejected); err != nil {
		t.Fatal(err)
	}
	if n, err := s.ClearLedgerEmailPending(ctx, "test-user", account.ID, emailSourceOrders); err != nil || n != 1 {
		t.Fatalf("ClearLedgerEmailPending = %d, %v", n, err)
	}
	if got := pendingKeys(t, s, account, emailSourceDrafts); len(got) != 1 {
		t.Fatalf("clearing the order folder touched the drafts folder: %+v", got)
	}

	mark("<c@example.org>")
	other := newPendingTestAccount(t, s, EmailActionMove, EmailActionKeep)
	if err := s.MarkLedgerEmailsProcessed(ctx, "test-user", other.ID, EmailSeenNotOrder, emailProcessed{Key: "<c@example.org>", Mailbox: ref}); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteLedgerEmailAccount(ctx, "test-user", account.ID); err != nil {
		t.Fatal(err)
	}
	if got := append(pendingKeys(t, s, account, emailSourceOrders), pendingKeys(t, s, account, emailSourceDrafts)...); len(got) != 0 {
		t.Errorf("pending actions survived the account: %+v", got)
	}
	if got := pendingKeys(t, s, other, emailSourceOrders); len(got) != 1 {
		t.Errorf("another account lost its pending actions: %+v", got)
	}
	// a scan that ends after its account was deleted queues nothing
	mark("<late@example.org>")
	if got := pendingKeys(t, s, account, emailSourceOrders); len(got) != 0 {
		t.Errorf("pending action for a deleted account: %+v", got)
	}
}
