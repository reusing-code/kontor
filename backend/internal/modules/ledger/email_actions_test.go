package ledger

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/reusing-code/kontor/backend/internal/drafts"
	"github.com/reusing-code/kontor/backend/internal/llm"
	"github.com/reusing-code/kontor/backend/internal/llm/llmtest"
)

const (
	capsMove      = "IMAP4rev1 UIDPLUS MOVE"
	capsFallback  = "IMAP4rev1 UIDPLUS"
	capsNoExpunge = "IMAP4rev1"
	trashListLine = `* LIST (\HasNoChildren \Trash) "/" "Trash"`
)

var imapWriteCommands = []string{"UID MOVE", "UID COPY", "UID STORE", "UID EXPUNGE", "EXPUNGE"}

func assertNoMailboxWrites(t *testing.T, fake *fakeIMAP) {
	t.Helper()
	for _, prefix := range imapWriteCommands {
		if got := fake.sent(prefix); len(got) != 0 {
			t.Errorf("mailbox was written to: %q", got)
		}
	}
}

func assertFolder(t *testing.T, fake *fakeIMAP, folder string, want int) {
	t.Helper()
	if got := fake.uids(folder); len(got) != want {
		t.Errorf("%s holds %v, want %d message(s)", folder, got, want)
	}
}

// orderActionTest scans the folder "Orders" of a fake IMAP server that also
// has "Done" and "Trash". A mail whose subject contains "Newsletter" is not
// an order for the fake LLM, one with "Broken" gives unusable output.
type orderActionTest struct {
	svc     *EmailService
	store   *Store
	imap    *fakeIMAP
	llm     *llmtest.Fake
	account LedgerEmailAccount
	logs    *bytes.Buffer
}

func newOrderActionTest(t *testing.T, action string, mails ...string) *orderActionTest {
	t.Helper()
	s := newTestStore(t).Store
	fake := newFakeIMAP(t)
	fake.addFolder("Orders", mails...)
	fake.addFolder("Done")
	fake.addFolder("Trash")
	host, port := fake.host()
	now := time.Now().UTC()
	account := LedgerEmailAccount{
		ID: uuid.New(), Name: "Mailbox", IMAPHost: host, IMAPPort: port, Username: "u",
		Folder: "Orders", ScanSince: "2026-01-01", ProcessedAction: action, CreatedAt: now, UpdatedAt: now,
	}
	if action == EmailActionMove {
		account.ProcessedFolder = "Done"
	}
	if err := s.CreateLedgerEmailAccount(context.Background(), testUserID, account); err != nil {
		t.Fatal(err)
	}
	model := &llmtest.Fake{Func: func(req llm.Request) (json.RawMessage, error) {
		switch {
		case strings.Contains(req.Input, "Broken"):
			return nil, llm.ErrInvalidOutput
		case strings.Contains(req.Input, "Newsletter"):
			return json.Marshal(emailOrderExtraction{IsOrder: false})
		}
		return json.Marshal(emailOrderExtraction{IsOrder: true, Merchant: "Acme GmbH", PaymentMethod: "card",
			ExternalOrderID: "A-1", OrderDate: "2026-11-02", TotalMinor: 1249, Currency: "EUR"})
	}}
	logs := &bytes.Buffer{}
	svc := NewEmailService(s, model, nil, slog.New(slog.NewTextHandler(logs, &slog.HandlerOptions{Level: slog.LevelDebug})))
	return &orderActionTest{svc: svc, store: s, imap: fake, llm: model, account: account, logs: logs}
}

func (x *orderActionTest) scan(t *testing.T) LedgerEmailScanResult {
	t.Helper()
	result, err := x.svc.ScanMailbox(context.Background(), testUserID, x.account, "pw")
	if err != nil {
		t.Fatalf("ScanMailbox: %v", err)
	}
	return result
}

func (x *orderActionTest) pending(t *testing.T) []ledgerEmailPendingEntry {
	t.Helper()
	return pendingKeysOf(t, x.store, testUserID, x.account, emailSourceOrders)
}

func (x *orderActionTest) assertOrders(t *testing.T, want int) {
	t.Helper()
	orders, err := x.store.ListLedgerEmailOrders(context.Background(), testUserID)
	if err != nil || len(orders) != want {
		t.Fatalf("orders = %d, %v; want %d", len(orders), err, want)
	}
}

func TestOrderScan_MovesProcessedMessages(t *testing.T) {
	for _, capabilities := range []string{capsMove, capsFallback} {
		t.Run(capabilities, func(t *testing.T) {
			x := newOrderActionTest(t, EmailActionMove, plainMail("order", "Your order"), plainMail("news", "Newsletter"))
			x.imap.capabilities = capabilities
			result := x.scan(t)
			if result.OrdersNew != 1 || len(result.Warnings) != 0 {
				t.Fatalf("result = %+v", result)
			}
			assertFolder(t, x.imap, "Orders", 0)
			assertFolder(t, x.imap, "Done", 2)
			if got := x.pending(t); len(got) != 0 {
				t.Errorf("pending = %+v", got)
			}
			if seen, _ := x.store.HasLedgerEmailSeen(context.Background(), testUserID, x.account.ID, "<news@example.org>"); !seen {
				t.Error("the non-order has no marker")
			}
			if got := x.imap.sent("EXPUNGE"); len(got) != 0 {
				t.Errorf("plain EXPUNGE sent: %q", got)
			}
			if moved := len(x.imap.sent("UID MOVE")); (capabilities == capsMove) != (moved == 2) {
				t.Errorf("UID MOVE sent %d times with %q", moved, capabilities)
			}
		})
	}
}

func TestOrderScan_DeletesProcessedMessages(t *testing.T) {
	t.Run("to the trash folder", func(t *testing.T) {
		x := newOrderActionTest(t, EmailActionDelete, plainMail("order", "Your order"), plainMail("news", "Newsletter"))
		x.imap.listLines = []string{trashListLine}
		if result := x.scan(t); len(result.Warnings) != 0 {
			t.Fatalf("warnings = %q", result.Warnings)
		}
		assertFolder(t, x.imap, "Orders", 0)
		assertFolder(t, x.imap, "Trash", 2)
		assertFolder(t, x.imap, "Done", 0)
	})
	t.Run("expunged without a trash folder", func(t *testing.T) {
		x := newOrderActionTest(t, EmailActionDelete, plainMail("order", "Your order"), plainMail("news", "Newsletter"))
		if result := x.scan(t); len(result.Warnings) != 0 {
			t.Fatalf("warnings = %q", result.Warnings)
		}
		assertFolder(t, x.imap, "Orders", 0)
		assertFolder(t, x.imap, "Trash", 0)
		if got := x.imap.sent("UID EXPUNGE"); len(got) != 2 {
			t.Errorf("UID EXPUNGE = %q, want one per message", got)
		}
		if got := x.imap.sent("EXPUNGE"); len(got) != 0 {
			t.Errorf("plain EXPUNGE sent: %q", got)
		}
	})
	t.Run("only flagged without UIDPLUS", func(t *testing.T) {
		x := newOrderActionTest(t, EmailActionDelete, plainMail("order", "Your order"))
		x.imap.capabilities = capsNoExpunge
		result := x.scan(t)
		if len(result.Warnings) != 1 || !strings.Contains(result.Warnings[0], "1 processed message(s) are only flagged as deleted") {
			t.Fatalf("warnings = %q", result.Warnings)
		}
		assertFolder(t, x.imap, "Orders", 1)
		if got := append(x.imap.sent("EXPUNGE"), x.imap.sent("UID EXPUNGE")...); len(got) != 0 {
			t.Errorf("expunge sent without UIDPLUS: %q", got)
		}
		// not tried again: the flag is all this server allows
		if got := x.pending(t); len(got) != 0 {
			t.Errorf("pending = %+v", got)
		}
		x.assertOrders(t, 1)
	})
}

func TestOrderScan_KeepNeverWritesToTheMailbox(t *testing.T) {
	for _, action := range []string{"", EmailActionKeep} {
		x := newOrderActionTest(t, action, plainMail("order", "Your order"), plainMail("news", "Newsletter"))
		if result := x.scan(t); result.OrdersNew != 1 || len(result.Warnings) != 0 {
			t.Fatalf("result = %+v", result)
		}
		assertNoMailboxWrites(t, x.imap)
		assertFolder(t, x.imap, "Orders", 2)
		if got := x.pending(t); len(got) != 0 {
			t.Errorf("pending = %+v", got)
		}
	}
}

func TestOrderScan_OnlyHandledMessagesAreActedOn(t *testing.T) {
	x := newOrderActionTest(t, EmailActionDelete, plainMail("broken", "Broken"), plainMail("order", "Your order"))
	x.svc.SetMaxExtractionFailures(1)
	x.scan(t)
	// the message Kontor gave up on stays where the user can see it
	if got := x.imap.uids("Orders"); !slices.Equal(got, []uint32{1}) {
		t.Fatalf("Orders = %v, want the unusable message (uid 1)", got)
	}
	if seen, _ := x.store.HasLedgerEmailSeen(context.Background(), testUserID, x.account.ID, "<broken@example.org>"); !seen {
		t.Error("the unusable message has no marker")
	}

	// an uploaded message is not in the mailbox
	upload := UploadedMessage{Filename: "upload.eml", Reader: strings.NewReader(plainMail("upload", "Your order"))}
	if _, err := x.svc.ScanUploadedMessages(context.Background(), testUserID, x.account, []UploadedMessage{upload}); err != nil {
		t.Fatalf("upload: %v", err)
	}
	x.assertOrders(t, 2)
	if got := x.pending(t); len(got) != 0 {
		t.Errorf("pending = %+v", got)
	}
}

func TestOrderScan_FailingActionKeepsTheOrderAndIsRetried(t *testing.T) {
	x := newOrderActionTest(t, EmailActionMove, plainMail("order", "Your order SECRETSUBJECT"))
	x.imap.setRefuse("UID MOVE", true)

	result := x.scan(t)
	if result.OrdersNew != 1 || result.Partial || result.Skipped != 0 {
		t.Fatalf("a refused move changed the scan: %+v", result)
	}
	if len(result.Warnings) != 1 || !strings.Contains(result.Warnings[0], "1 processed message(s) could not be moved in the mailbox; tried again next time") {
		t.Fatalf("warnings = %q", result.Warnings)
	}
	x.assertOrders(t, 1)
	assertFolder(t, x.imap, "Orders", 1)
	if got := x.pending(t); len(got) != 1 || got[0].Attempts != 1 || got[0].UID != 1 {
		t.Fatalf("pending = %+v", got)
	}

	// the next scan retries without asking the model again
	x.imap.setRefuse("UID MOVE", false)
	asked := len(x.llm.Requests())
	if result := x.scan(t); len(result.Warnings) != 0 || result.OrdersNew != 0 {
		t.Fatalf("retry = %+v", result)
	}
	if len(x.llm.Requests()) != asked {
		t.Error("the retry sent the message to the LLM again")
	}
	assertFolder(t, x.imap, "Orders", 0)
	assertFolder(t, x.imap, "Done", 1)
	if got := x.pending(t); len(got) != 0 {
		t.Errorf("pending = %+v", got)
	}
	if out := x.logs.String(); strings.Contains(out, "SECRETSUBJECT") || strings.Contains(out, "order@example.org") {
		t.Errorf("log names the message: %s", out)
	}
}

func TestOrderScan_ActionIsGivenUpAfterRepeatedRefusals(t *testing.T) {
	x := newOrderActionTest(t, EmailActionMove, plainMail("order", "Your order"))
	x.account.ProcessedFolder = "Missing" // the fake answers NO [TRYCREATE]
	if _, err := x.store.ModifyLedgerEmailAccount(context.Background(), testUserID, x.account.ID, func(a *LedgerEmailAccount) {
		a.ProcessedFolder = "Missing"
	}); err != nil {
		t.Fatal(err)
	}
	var last LedgerEmailScanResult
	for range maxEmailActionAttempts {
		last = x.scan(t)
	}
	if len(last.Warnings) != 1 || !strings.Contains(last.Warnings[0], "could not be moved in the mailbox repeatedly and stay in the folder") {
		t.Fatalf("warnings = %q", last.Warnings)
	}
	if got := x.pending(t); len(got) != 0 {
		t.Fatalf("pending = %+v", got)
	}
	if result := x.scan(t); len(result.Warnings) != 0 {
		t.Errorf("a given-up message warns again: %q", result.Warnings)
	}
	assertFolder(t, x.imap, "Orders", 1)
	x.assertOrders(t, 1)
	if got := x.imap.sent("CREATE"); len(got) != 0 {
		t.Errorf("Kontor created a folder: %q", got)
	}
}

// drainTest runs the executor on its own against records written by hand.
type drainTest struct {
	*orderActionTest
	actions *emailActions
}

func newDrainTest(t *testing.T, action string, mails ...string) *drainTest {
	x := newOrderActionTest(t, action, mails...)
	return &drainTest{orderActionTest: x, actions: x.svc.actions}
}

func (d *drainTest) queue(t *testing.T, key string, uid, uidValidity uint32) {
	t.Helper()
	if err := d.store.MarkLedgerEmailsProcessed(context.Background(), testUserID, d.account.ID, EmailSeenNotOrder,
		emailProcessed{Key: key, Mailbox: &emailMailboxRef{UID: uid, UIDValidity: uidValidity}}); err != nil {
		t.Fatal(err)
	}
}

func (d *drainTest) drain(t *testing.T) []string {
	t.Helper()
	client, err := openMailbox(d.account, "pw", d.account.Folder)
	if err != nil {
		t.Fatalf("openMailbox: %v", err)
	}
	defer client.Close()
	warnings, err := d.actions.drain(context.Background(), client, testUserID, d.account.ID, emailSourceOrders)
	if err != nil {
		t.Fatalf("drain: %v", err)
	}
	return warnings
}

func TestEmailActions_FindsTheMessageAgain(t *testing.T) {
	headerKey := func(mail string) string {
		raw := []byte(strings.ReplaceAll(mail, "\n", "\r\n"))
		return emailDedupKey("", headerBlock(raw))
	}
	noID := "From: Sender <s@example.org>\nSubject: No id\n\nbody\n"

	t.Run("by Message-ID after the UIDVALIDITY changed", func(t *testing.T) {
		d := newDrainTest(t, EmailActionMove, plainMail("other", "Other"), plainMail("done", "Done one"))
		d.queue(t, "<done@example.org>", 1, 1) // uid 1 is another message by now
		d.imap.uidValidity = 2
		if w := d.drain(t); len(w) != 0 {
			t.Fatalf("warnings = %q", w)
		}
		if got := d.imap.uids("Orders"); !slices.Equal(got, []uint32{1}) {
			t.Errorf("Orders = %v, want only the other message (uid 1)", got)
		}
		assertFolder(t, d.imap, "Done", 1)
	})
	t.Run("a UID that now belongs to another message is not trusted", func(t *testing.T) {
		d := newDrainTest(t, EmailActionDelete, plainMail("other", "Other"), plainMail("done", "Done one"))
		d.queue(t, "<done@example.org>", 1, 1)
		if w := d.drain(t); len(w) != 0 {
			t.Fatalf("warnings = %q", w)
		}
		if got := d.imap.uids("Orders"); !slices.Equal(got, []uint32{1}) {
			t.Errorf("Orders = %v, want only the other message (uid 1)", got)
		}
	})
	t.Run("a message the user already removed", func(t *testing.T) {
		d := newDrainTest(t, EmailActionDelete, plainMail("other", "Other"))
		d.queue(t, "<gone@example.org>", 7, 1)
		if w := d.drain(t); len(w) != 0 {
			t.Fatalf("warnings = %q", w)
		}
		assertNoMailboxWrites(t, d.imap)
		if got := d.pending(t); len(got) != 0 {
			t.Errorf("pending = %+v", got)
		}
	})
	t.Run("without a Message-ID by its UID", func(t *testing.T) {
		d := newDrainTest(t, EmailActionMove, noID)
		d.queue(t, headerKey(noID), 1, 1)
		if w := d.drain(t); len(w) != 0 {
			t.Fatalf("warnings = %q", w)
		}
		assertFolder(t, d.imap, "Done", 1)
	})
	t.Run("without a Message-ID after the UIDVALIDITY changed", func(t *testing.T) {
		d := newDrainTest(t, EmailActionMove, noID)
		d.queue(t, headerKey(noID), 1, 1)
		d.imap.uidValidity = 2
		w := d.drain(t)
		if len(w) != 1 || !strings.Contains(w[0], "1 processed message(s) could not be found in the folder again") {
			t.Fatalf("warnings = %q", w)
		}
		assertNoMailboxWrites(t, d.imap)
		if got := d.pending(t); len(got) != 0 {
			t.Errorf("pending = %+v", got)
		}
	})
	t.Run("a refused search is tried again", func(t *testing.T) {
		d := newDrainTest(t, EmailActionMove, plainMail("done", "Done one"))
		d.queue(t, "<done@example.org>", 0, 0)
		d.imap.setRefuse("UID SEARCH HEADER", true)
		if w := d.drain(t); len(w) != 1 {
			t.Fatalf("warnings = %q", w)
		}
		if got := d.pending(t); len(got) != 1 || got[0].Attempts != 1 {
			t.Fatalf("pending = %+v", got)
		}
		d.imap.setRefuse("UID SEARCH HEADER", false)
		d.drain(t)
		assertFolder(t, d.imap, "Done", 1)
	})
}

func TestEmailActions_FollowTheCurrentSetting(t *testing.T) {
	modify := func(t *testing.T, d *drainTest, fn func(*LedgerEmailAccount)) {
		t.Helper()
		if _, err := d.store.ModifyLedgerEmailAccount(context.Background(), testUserID, d.account.ID, fn); err != nil {
			t.Fatal(err)
		}
	}
	t.Run("switched back to keep", func(t *testing.T) {
		d := newDrainTest(t, EmailActionDelete, plainMail("done", "Done one"))
		d.queue(t, "<done@example.org>", 1, 1)
		modify(t, d, func(a *LedgerEmailAccount) { a.ProcessedAction = EmailActionKeep })
		if w := d.drain(t); len(w) != 0 {
			t.Fatalf("warnings = %q", w)
		}
		assertNoMailboxWrites(t, d.imap)
		if got := d.pending(t); len(got) != 0 {
			t.Errorf("pending = %+v, want dropped", got)
		}
		// turning the action on again does not reach back
		modify(t, d, func(a *LedgerEmailAccount) { a.ProcessedAction = EmailActionDelete })
		d.drain(t)
		assertFolder(t, d.imap, "Orders", 1)
	})
	t.Run("switched from delete to move", func(t *testing.T) {
		d := newDrainTest(t, EmailActionDelete, plainMail("done", "Done one"))
		d.queue(t, "<done@example.org>", 1, 1)
		modify(t, d, func(a *LedgerEmailAccount) { a.ProcessedAction, a.ProcessedFolder = EmailActionMove, "Done" })
		d.drain(t)
		assertFolder(t, d.imap, "Done", 1)
	})
	t.Run("a move without a target does nothing", func(t *testing.T) {
		d := newDrainTest(t, EmailActionMove, plainMail("done", "Done one"))
		d.queue(t, "<done@example.org>", 1, 1)
		modify(t, d, func(a *LedgerEmailAccount) { a.ProcessedFolder = "" })
		d.drain(t)
		assertNoMailboxWrites(t, d.imap)
	})
	t.Run("the scanned folder was changed", func(t *testing.T) {
		d := newDrainTest(t, EmailActionDelete, plainMail("done", "Done one"))
		d.queue(t, "<done@example.org>", 1, 1)
		modify(t, d, func(a *LedgerEmailAccount) { a.Folder = "Elsewhere" })
		d.drain(t) // the client still has "Orders" selected
		assertNoMailboxWrites(t, d.imap)
	})
	t.Run("the trash folder is the scanned folder", func(t *testing.T) {
		d := newDrainTest(t, EmailActionDelete, plainMail("done", "Done one"))
		d.imap.listLines = []string{`* LIST (\Trash) "/" "Orders"`}
		d.queue(t, "<done@example.org>", 1, 1)
		d.drain(t)
		assertFolder(t, d.imap, "Orders", 0)
		if got := d.imap.sent("UID EXPUNGE"); len(got) != 1 {
			t.Errorf("UID EXPUNGE = %q", got)
		}
	})
	t.Run("a refused folder list deletes nothing", func(t *testing.T) {
		d := newDrainTest(t, EmailActionDelete, plainMail("done", "Done one"))
		d.queue(t, "<done@example.org>", 1, 1)
		d.imap.setRefuse("LIST", true)
		if w := d.drain(t); len(w) != 1 || !strings.Contains(w[0], "did not list its folders") {
			t.Fatalf("warnings = %q", w)
		}
		assertNoMailboxWrites(t, d.imap)
		if got := d.pending(t); len(got) != 1 {
			t.Errorf("pending = %+v", got)
		}
	})
}

// draftActionTest decides email drafts through the hooks of an account whose
// drafts folder has an action; "Done" and "Trash" exist on the server.
type draftActionTest struct {
	*emailDraftTest
	hooks EmailDraftHooks
}

func newDraftActionTest(t *testing.T, action string, mails ...string) *draftActionTest {
	t.Helper()
	env := newEmailDraftTest(t, mails...)
	env.imap.addFolder("Done")
	env.imap.addFolder("Trash")
	account, err := env.store.ModifyLedgerEmailAccount(context.Background(), testUserID, env.account.ID, func(a *LedgerEmailAccount) {
		a.DraftsProcessedAction = action
		if action == EmailActionMove {
			a.DraftsProcessedFolder = "Done"
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	env.account = account
	logger := slog.New(slog.NewTextHandler(env.logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	actions := newEmailActions(env.store, logger)
	actions.setEncryptionKey(env.pipeline.encryptionKey)
	env.pipeline.actions = actions
	return &draftActionTest{emailDraftTest: env, hooks: EmailDraftHooks{store: env.store, logger: logger, actions: actions}}
}

// propose runs the pipeline once with "none" answers and returns the drafts
// by the Message-ID local part of plainMail.
func (x *draftActionTest) propose(t *testing.T, n int) map[string]drafts.Draft {
	t.Helper()
	for range n {
		x.llm.Push(noneClassification())
	}
	if res := x.run(t); res.Created != n || len(res.Warnings) != 0 {
		t.Fatalf("run = %+v, want %d drafts", res, n)
	}
	out := map[string]drafts.Draft{}
	for _, d := range x.pending(t) {
		_, key, _ := parseEmailDraftRef(d.Source.Ref)
		out[strings.TrimSuffix(strings.TrimPrefix(key, "<"), "@example.org>")] = d
	}
	return out
}

func (x *draftActionTest) accept(t *testing.T, d drafts.Draft, entityType string) {
	t.Helper()
	warnings, err := x.hooks.OnAccept(context.Background(), testUserID, d, drafts.AcceptInput{EntityType: entityType, EntityID: uuid.NewString()})
	if err != nil || len(warnings) != 0 {
		t.Fatalf("OnAccept = %q, %v", warnings, err)
	}
}

func (x *draftActionTest) reject(t *testing.T, d drafts.Draft) {
	t.Helper()
	if warnings := x.hooks.OnReject(context.Background(), testUserID, d); len(warnings) != 0 {
		t.Fatalf("OnReject = %q", warnings)
	}
}

func (x *draftActionTest) wait() { x.hooks.actions.wg.Wait() }

func (x *draftActionTest) pendingActions(t *testing.T) []ledgerEmailPendingEntry {
	t.Helper()
	return pendingKeysOf(t, x.store, testUserID, x.account, emailSourceDrafts)
}

func (x *draftActionTest) assertDecided(t *testing.T, ids ...string) {
	t.Helper()
	for _, id := range ids {
		key := fmt.Sprintf("<%s@example.org>", id)
		if seen, err := x.store.HasLedgerEmailDraftSeen(context.Background(), testUserID, x.account.ID, key); err != nil || !seen {
			t.Errorf("%s has no draft marker (%v)", key, err)
		}
	}
}

func TestEmailDraftDecisions_MoveTheMessage(t *testing.T) {
	for _, capabilities := range []string{capsMove, capsFallback} {
		t.Run(capabilities, func(t *testing.T) {
			x := newDraftActionTest(t, EmailActionMove, plainMail("a", "Contract"), plainMail("b", "Bill"), plainMail("c", "Notice"), plainMail("d", "Open"))
			x.imap.capabilities = capabilities
			found := x.propose(t, 4)
			x.accept(t, found["a"], drafts.TargetContract)
			x.accept(t, found["b"], drafts.TargetVehicle) // a type emails are not linked to
			x.reject(t, found["c"])
			x.wait()

			// the undecided message stays
			if got := x.imap.uids(draftTestFolder); !slices.Equal(got, []uint32{4}) {
				t.Errorf("%s = %v, want [4]", draftTestFolder, got)
			}
			assertFolder(t, x.imap, "Done", 3)
			x.assertDecided(t, "a", "b", "c")
			if got := x.pendingActions(t); len(got) != 0 {
				t.Errorf("pending = %+v", got)
			}
			if got := x.imap.sent("EXPUNGE"); len(got) != 0 {
				t.Errorf("plain EXPUNGE sent: %q", got)
			}
			if res := x.run(t); res.Created != 0 || len(res.Warnings) != 0 {
				t.Errorf("next pass = %+v", res)
			}
		})
	}
}

func TestEmailDraftDecisions_DeleteTheMessage(t *testing.T) {
	t.Run("to the trash folder", func(t *testing.T) {
		x := newDraftActionTest(t, EmailActionDelete, plainMail("a", "Contract"), plainMail("b", "Bill"))
		x.imap.listLines = []string{trashListLine}
		found := x.propose(t, 2)
		x.accept(t, found["a"], drafts.TargetContract)
		x.reject(t, found["b"])
		x.wait()
		assertFolder(t, x.imap, draftTestFolder, 0)
		assertFolder(t, x.imap, "Trash", 2)
	})
	t.Run("expunged without a trash folder", func(t *testing.T) {
		x := newDraftActionTest(t, EmailActionDelete, plainMail("a", "Contract"), plainMail("b", "Bill"))
		found := x.propose(t, 2)
		x.accept(t, found["a"], drafts.TargetContract)
		x.reject(t, found["b"])
		x.wait()
		assertFolder(t, x.imap, draftTestFolder, 0)
		assertFolder(t, x.imap, "Trash", 0)
		if got := x.imap.sent("EXPUNGE"); len(got) != 0 {
			t.Errorf("plain EXPUNGE sent: %q", got)
		}
	})
}

func TestEmailDraftDecisions_KeepNeverTouchesTheMailbox(t *testing.T) {
	x := newDraftActionTest(t, EmailActionKeep, plainMail("a", "Contract"), plainMail("b", "Bill"))
	found := x.propose(t, 2)
	logins := len(x.imap.sent("SELECT"))
	x.accept(t, found["a"], drafts.TargetContract)
	x.reject(t, found["b"])
	x.wait()
	if got := len(x.imap.sent("SELECT")); got != logins {
		t.Errorf("a decision opened the mailbox %d time(s)", got-logins)
	}
	assertNoMailboxWrites(t, x.imap)
	x.assertDecided(t, "a", "b")
	if got := x.pendingActions(t); len(got) != 0 {
		t.Errorf("pending = %+v", got)
	}
}

func TestEmailDraftDecisions_FailingActionKeepsTheDecisionAndIsRetried(t *testing.T) {
	x := newDraftActionTest(t, EmailActionMove, plainMail("a", "Contract SECRETSUBJECT"), plainMail("b", "Bill"))
	found := x.propose(t, 2)
	x.imap.setRefuse("UID MOVE", true)
	entityID := uuid.NewString()
	warnings, err := x.hooks.OnAccept(context.Background(), testUserID, found["a"], drafts.AcceptInput{EntityType: drafts.TargetContract, EntityID: entityID})
	if err != nil || len(warnings) != 0 {
		t.Fatalf("OnAccept = %q, %v", warnings, err)
	}
	x.reject(t, found["b"])
	x.wait()

	// the decisions stand: markers, the email link, nothing proposed again
	x.assertDecided(t, "a", "b")
	if links, err := x.store.ListLedgerEmailLinks(context.Background(), testUserID, EmailLinkContract, entityID); err != nil || len(links) != 1 {
		t.Fatalf("links = %+v, %v", links, err)
	}
	assertFolder(t, x.imap, draftTestFolder, 2)
	if got := x.pendingActions(t); len(got) != 2 || got[0].Attempts < 1 || got[1].Attempts < 1 {
		t.Fatalf("pending = %+v", got)
	}

	// the next pass tries again and reports it
	res := x.run(t)
	if res.Created != 0 || len(res.Warnings) != 1 ||
		res.Warnings[0] != "Mailbox: 2 processed message(s) could not be moved in the mailbox; tried again next time" {
		t.Fatalf("pass with the refusal = %+v", res)
	}
	x.imap.setRefuse("UID MOVE", false)
	if res := x.run(t); res.Created != 0 || len(res.Warnings) != 0 {
		t.Fatalf("pass after the refusal = %+v", res)
	}
	assertFolder(t, x.imap, draftTestFolder, 0)
	assertFolder(t, x.imap, "Done", 2)
	if got := x.pendingActions(t); len(got) != 0 {
		t.Errorf("pending = %+v", got)
	}
	if out := x.logs.String(); strings.Contains(out, "SECRETSUBJECT") || strings.Contains(out, "@example.org") {
		t.Errorf("log names a message: %s", out)
	}
}

func TestEmailDraftDecisions_UnreachableServerKeepsTheDecision(t *testing.T) {
	x := newDraftActionTest(t, EmailActionDelete, plainMail("a", "Contract"))
	found := x.propose(t, 1)
	x.imap.ln.Close()
	x.reject(t, found["a"])
	x.wait()
	x.assertDecided(t, "a")
	if got := x.pendingActions(t); len(got) != 1 || got[0].Attempts != 0 {
		t.Fatalf("pending = %+v, want the action kept without a counted refusal", got)
	}
}

func TestEmailDraftDecisions_DraftFromBeforeTheMailboxPositionWasStored(t *testing.T) {
	x := newDraftActionTest(t, EmailActionMove, plainMail("a", "Contract"))
	found := x.propose(t, 1)
	// as stored by a version that did not keep UID and UIDVALIDITY
	if err := x.store.putLedgerEmailDraftMessage(testUserID, x.account.ID, "<a@example.org>", ledgerEmailDraftMessage{MessageID: "<a@example.org>"}); err != nil {
		t.Fatal(err)
	}
	x.accept(t, found["a"], drafts.TargetContract)
	x.wait()
	assertFolder(t, x.imap, draftTestFolder, 0)
	assertFolder(t, x.imap, "Done", 1)
	if got := x.imap.sent("UID SEARCH HEADER"); len(got) != 1 {
		t.Errorf("searches = %q, want one by Message-ID", got)
	}
}
