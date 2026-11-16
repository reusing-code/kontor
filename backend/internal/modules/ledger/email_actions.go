package ledger

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/reusing-code/kontor/backend/internal/cryptoutil"
	"github.com/reusing-code/kontor/backend/internal/storage"
)

const (
	// maxEmailActionAttempts is how often the server may refuse the mailbox
	// action of a message before it is given up and the message stays.
	maxEmailActionAttempts = 5
	// emailActionKickTimeout bounds the mailbox work started by a draft decision.
	emailActionKickTimeout = 30 * time.Second
	// maxEmailActionSearchHits bounds the messages checked for one Message-ID.
	maxEmailActionSearchHits = 10
)

// emailActions carries out the mailbox action (move, delete) an account has
// configured for processed messages. It works off the pending records the
// stores write together with the local result, so it is best effort: a
// failure never undoes an order or a draft decision, it leaves the record
// for the next run.
type emailActions struct {
	store  *Store
	logger *slog.Logger

	mu            sync.Mutex
	encryptionKey []byte
	baseCtx       context.Context
	locks         map[string]*sync.Mutex
	// kicks holds the accounts with a running kick; true asks it to run again.
	kicks map[uuid.UUID]bool
	wg    sync.WaitGroup
}

func newEmailActions(store *Store, logger *slog.Logger) *emailActions {
	return &emailActions{
		store: store, logger: logger.With("component", "ledger-email-actions"), baseCtx: context.Background(),
		locks: map[string]*sync.Mutex{}, kicks: map[uuid.UUID]bool{},
	}
}

// setBackgroundContext bounds the work started by Kick.
func (a *emailActions) setBackgroundContext(ctx context.Context) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.baseCtx = ctx
}

// setEncryptionKey lets Kick open a mailbox on its own; without a key only
// the scans carry out actions, on their connections.
func (a *emailActions) setEncryptionKey(key []byte) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.encryptionKey = key
}

func (a *emailActions) lock(accountID uuid.UUID, source string) *sync.Mutex {
	a.mu.Lock()
	defer a.mu.Unlock()
	key := accountID.String() + "/" + source
	if a.locks[key] == nil {
		a.locks[key] = &sync.Mutex{}
	}
	return a.locks[key]
}

// emailActionResult counts one drain.
type emailActionResult struct {
	Done        int // moved or deleted
	Failed      int // refused by the server, tried again next time
	GivenUp     int // refused maxEmailActionAttempts times
	NotExpunged int // flagged \Deleted, the server cannot expunge single messages
	NotFound    int // no longer identifiable in the folder
	TrashFailed bool
}

// warnings are fixed texts with counts: nothing of a message gets into them.
func (r emailActionResult) warnings(action string) []string {
	verb := "moved"
	if action == EmailActionDelete {
		verb = "deleted"
	}
	var out []string
	add := func(n int, text string) {
		if n > 0 {
			out = append(out, fmt.Sprintf("%d processed message(s) %s", n, text))
		}
	}
	add(r.Failed, "could not be "+verb+" in the mailbox; tried again next time")
	add(r.GivenUp, "could not be "+verb+" in the mailbox repeatedly and stay in the folder")
	add(r.NotExpunged, "are only flagged as deleted: the mail server cannot remove single messages (no UIDPLUS)")
	add(r.NotFound, "could not be found in the folder again and stay as they are")
	if r.TrashFailed {
		out = append(out, "processed messages could not be deleted: the mail server did not list its folders; tried again next time")
	}
	return out
}

// drain carries out the pending actions of a source on a client that has
// the source's folder selected. The returned error is an ErrIMAP for an
// unusable connection or a store error; refusals are counted in the result.
func (a *emailActions) drain(ctx context.Context, client *imapClient, userID string, accountID uuid.UUID, source string) ([]string, error) {
	mu := a.lock(accountID, source)
	mu.Lock()
	defer mu.Unlock()

	// the setting as it is now, not as it was when the scan started
	account, err := a.store.GetLedgerEmailAccount(ctx, userID, accountID)
	if errors.Is(err, storage.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	action, target := account.processedAction(source)
	if action != EmailActionMove && action != EmailActionDelete {
		_, err := a.store.ClearLedgerEmailPending(ctx, userID, accountID, source)
		return nil, err
	}
	// a scan that started before the folder was changed has the old one selected
	if client.folder != account.sourceFolder(source) || action == EmailActionMove && target == "" {
		return nil, nil
	}
	entries, err := a.store.ListLedgerEmailPending(ctx, userID, accountID, source)
	if err != nil || len(entries) == 0 {
		return nil, err
	}
	var res emailActionResult
	err = a.run(ctx, client, userID, accountID, source, action, target, entries, &res)
	if res != (emailActionResult{}) {
		a.logger.Info("mailbox actions", "userID", userID, "accountID", accountID, "source", source, "action", action,
			"done", res.Done, "failed", res.Failed, "givenUp", res.GivenUp, "notExpunged", res.NotExpunged, "notFound", res.NotFound)
	}
	return res.warnings(action), err
}

func (a *emailActions) run(ctx context.Context, client *imapClient, userID string, accountID uuid.UUID, source, action, target string,
	entries []ledgerEmailPendingEntry, res *emailActionResult) error {
	if action == EmailActionDelete {
		// "delete" means the server's trash when it has one: recoverable, and
		// on Gmail the only way that deletes instead of removing a label
		trash, err := client.TrashFolder()
		switch {
		case errors.Is(err, errIMAPCommandFailed):
			res.TrashFailed = true
			return nil
		case err != nil:
			return imapError(err)
		}
		target = trash
		if target == client.folder {
			target = ""
		}
	}
	for _, entry := range entries {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		drop := func() error { return a.store.DeleteLedgerEmailPending(ctx, userID, accountID, source, entry.Key) }
		uid, found, err := locateEmail(client, entry)
		switch {
		case err != nil:
		case !found:
			// a message with a Message-ID that is not there was moved or
			// deleted by the user; without one it may just be unidentifiable
			if !isVerbatimMessageID(entry.Key) {
				res.NotFound++
			}
			if err := drop(); err != nil {
				return err
			}
			continue
		case target != "":
			err = client.Move(uid, target)
		default:
			err = client.Delete(uid)
		}
		switch {
		case err == nil:
			res.Done++
			err = drop()
		case errors.Is(err, errIMAPNotExpunged):
			// not retried: the copy exists, another attempt would add one more
			res.NotExpunged++
			err = drop()
		case errors.Is(err, errIMAPCommandFailed):
			var attempts int
			if attempts, err = a.store.RecordLedgerEmailPendingFailure(ctx, userID, accountID, source, entry.Key); err != nil {
				return err
			}
			if attempts < maxEmailActionAttempts {
				res.Failed++
				continue
			}
			res.GivenUp++
			err = drop()
		case errors.Is(err, ErrIMAP):
			return err
		default:
			return imapError(err)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

// isVerbatimMessageID reports a dedup key that is the Message-ID itself and
// can be searched for.
func isVerbatimMessageID(key string) bool {
	return !strings.HasPrefix(key, "mid:") && !strings.HasPrefix(key, "nomid:")
}

// locateEmail finds the message of a pending action in the selected folder.
// A UID is only trusted after its headers gave the expected dedup key: UIDs
// change with the UIDVALIDITY, and a wrong one would move or delete a
// message Kontor never processed. A search the server refuses is an
// errIMAPCommandFailed, an unusable connection an ErrIMAP.
func locateEmail(client *imapClient, entry ledgerEmailPendingEntry) (uint32, bool, error) {
	if entry.UID != 0 && entry.UIDValidity != 0 && entry.UIDValidity == client.UIDValidity() {
		ok, err := emailAtUID(client, entry.UID, entry.Key)
		if err != nil || ok {
			return entry.UID, ok, err
		}
	}
	if !isVerbatimMessageID(entry.Key) {
		return 0, false, nil
	}
	uids, err := client.SearchHeader("Message-ID", entry.Key)
	switch {
	case errors.Is(err, errIMAPCommandFailed):
		return 0, false, err
	case err != nil:
		return 0, false, imapError(err)
	}
	if len(uids) > maxEmailActionSearchHits {
		uids = uids[len(uids)-maxEmailActionSearchHits:]
	}
	for _, uid := range uids {
		ok, err := emailAtUID(client, uid, entry.Key)
		if err != nil || ok {
			return uid, ok, err
		}
	}
	return 0, false, nil
}

// emailAtUID reports whether the message at the UID has the dedup key.
func emailAtUID(client *imapClient, uid uint32, key string) (bool, error) {
	raw, err := client.FetchHeader(uid)
	switch {
	case errors.Is(err, errIMAPRejected), errors.Is(err, errMessageTooLarge):
		return false, nil
	case err != nil:
		return false, imapError(err)
	}
	header, err := parseHeader(raw)
	if err != nil {
		return false, nil
	}
	return emailDedupKey(header.MessageID, headerBlock(raw)) == key, nil
}

// Kick carries out the pending actions of an account's drafts folder in the
// background, after a draft was decided. Decisions never wait for the mail
// server; whatever fails here is picked up by the next pass over the folder.
func (a *emailActions) Kick(userID string, accountID uuid.UUID) {
	if a == nil {
		return
	}
	a.mu.Lock()
	if _, running := a.kicks[accountID]; running {
		a.kicks[accountID] = true
		a.mu.Unlock()
		return
	}
	a.kicks[accountID] = false
	base, key := a.baseCtx, a.encryptionKey
	a.mu.Unlock()

	a.wg.Go(func() {
		for {
			ctx, cancel := context.WithTimeout(base, emailActionKickTimeout)
			a.kick(ctx, userID, accountID, key)
			cancel()
			a.mu.Lock()
			again := a.kicks[accountID]
			if !again {
				delete(a.kicks, accountID)
				a.mu.Unlock()
				return
			}
			a.kicks[accountID] = false
			a.mu.Unlock()
		}
	})
}

func (a *emailActions) kick(ctx context.Context, userID string, accountID uuid.UUID, encryptionKey []byte) {
	account, err := a.store.GetLedgerEmailAccount(ctx, userID, accountID)
	if err != nil {
		return
	}
	if action, _ := account.processedAction(emailSourceDrafts); action == EmailActionKeep {
		if _, err := a.store.ClearLedgerEmailPending(ctx, userID, accountID, emailSourceDrafts); err != nil {
			a.logger.Error("clearing pending mailbox actions", "userID", userID, "accountID", accountID, "error", err)
		}
		return
	}
	entries, err := a.store.ListLedgerEmailPending(ctx, userID, accountID, emailSourceDrafts)
	if err != nil || len(entries) == 0 || account.DraftsFolder == "" || account.EncryptedPassword == "" || len(encryptionKey) == 0 {
		return
	}
	password, err := cryptoutil.DecryptString(account.EncryptedPassword, encryptionKey)
	if err != nil {
		a.logger.Error("decrypting email account password", "userID", userID, "accountID", accountID, "error", err)
		return
	}
	client, err := openMailbox(account, password, account.DraftsFolder)
	if err != nil {
		a.logger.Warn("mailbox actions: mailbox failed", "userID", userID, "accountID", accountID)
		return
	}
	defer client.Close()
	defer client.Logout()
	if _, err := a.drain(ctx, client, userID, accountID, emailSourceDrafts); err != nil {
		a.logger.Warn("mailbox actions: stopped early", "userID", userID, "accountID", accountID)
	}
}
