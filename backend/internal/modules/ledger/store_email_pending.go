package ledger

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/dgraph-io/badger/v4"
	"github.com/google/uuid"
	"github.com/reusing-code/kontor/backend/internal/storage"
)

// emailMailboxRef is where a scan saw a message in its folder. A UID only
// means something together with the folder's UIDVALIDITY.
type emailMailboxRef struct {
	UID         uint32 `json:"uid,omitempty"`
	UIDValidity uint32 `json:"uidValidity,omitempty"`
}

// ledgerEmailPending is a processed message whose mailbox action (move or
// delete, as configured on the account when it runs) is still to be done.
type ledgerEmailPending struct {
	emailMailboxRef
	Attempts int       `json:"attempts,omitempty"`
	At       time.Time `json:"at"`
}

// ledgerEmailPendingEntry is a pending action with the dedup key of its message.
type ledgerEmailPendingEntry struct {
	Key string
	ledgerEmailPending
}

// emailProcessed is a message that was just processed: its dedup key and,
// when it came from a mailbox (not an upload), where the scan saw it.
type emailProcessed struct {
	Key     string
	Mailbox *emailMailboxRef
}

func idxLedEmailPendingKey(userID string, accountID uuid.UUID, source, key string) []byte {
	return []byte(fmt.Sprintf("u/%s/mod/ledger/idx/epend/%s/%s/%s", userID, accountID, source, key))
}

func idxLedEmailPendingSourcePrefix(userID string, accountID uuid.UUID, source string) []byte {
	return []byte(fmt.Sprintf("u/%s/mod/ledger/idx/epend/%s/%s/", userID, accountID, source))
}

func idxLedEmailPendingPrefix(userID string, accountID uuid.UUID) []byte {
	return []byte(fmt.Sprintf("u/%s/mod/ledger/idx/epend/%s/", userID, accountID))
}

// queueLedgerEmailAction records the mailbox action for a message that the
// surrounding transaction marks as processed. Nothing is recorded while the
// source's action is keep, so turning an action on later does not reach back
// to messages processed before.
func queueLedgerEmailAction(txn *badger.Txn, account LedgerEmailAccount, userID, source, key string, ref emailMailboxRef) error {
	if action, _ := account.processedAction(source); action != EmailActionMove && action != EmailActionDelete {
		return nil
	}
	return storage.SetJSON(txn, idxLedEmailPendingKey(userID, account.ID, source, key),
		ledgerEmailPending{emailMailboxRef: ref, At: time.Now().UTC()})
}

// queueLedgerEmailActions is queueLedgerEmailAction for the messages of a
// scan; uploads (no mailbox) are left out and a deleted account queues nothing.
func queueLedgerEmailActions(txn *badger.Txn, userID string, accountID uuid.UUID, source string, msgs []emailProcessed) error {
	var account *LedgerEmailAccount
	for _, m := range msgs {
		if m.Mailbox == nil {
			continue
		}
		if account == nil {
			loaded, err := loadLedgerEmailAccount(txn, userID, accountID)
			if errors.Is(err, storage.ErrNotFound) {
				return nil
			}
			if err != nil {
				return err
			}
			account = &loaded
		}
		if err := queueLedgerEmailAction(txn, *account, userID, source, m.Key, *m.Mailbox); err != nil {
			return err
		}
	}
	return nil
}

// ListLedgerEmailPending returns the pending mailbox actions of a source,
// oldest first.
func (s *Store) ListLedgerEmailPending(_ context.Context, userID string, accountID uuid.UUID, source string) ([]ledgerEmailPendingEntry, error) {
	var entries []ledgerEmailPendingEntry
	prefix := idxLedEmailPendingSourcePrefix(userID, accountID, source)
	err := s.e.View(func(txn *badger.Txn) error {
		return storage.IteratePrefix(txn, prefix, func(key, val []byte) error {
			entry := ledgerEmailPendingEntry{Key: strings.TrimPrefix(string(key), string(prefix))}
			if err := json.Unmarshal(val, &entry.ledgerEmailPending); err != nil {
				return err
			}
			entries = append(entries, entry)
			return nil
		})
	})
	if err != nil {
		return nil, err
	}
	slices.SortStableFunc(entries, func(a, b ledgerEmailPendingEntry) int { return a.At.Compare(b.At) })
	return entries, nil
}

// DeleteLedgerEmailPending drops a pending action: done or given up.
func (s *Store) DeleteLedgerEmailPending(_ context.Context, userID string, accountID uuid.UUID, source, key string) error {
	return s.e.Update(func(txn *badger.Txn) error {
		return deleteIfPresent(txn, idxLedEmailPendingKey(userID, accountID, source, key))
	})
}

// RecordLedgerEmailPendingFailure counts a mailbox action the server refused
// and returns the new count; 0 when the record no longer exists.
func (s *Store) RecordLedgerEmailPendingFailure(ctx context.Context, userID string, accountID uuid.UUID, source, key string) (int, error) {
	attempts := 0
	err := retryConflicts(ctx, func() error {
		attempts = 0
		return s.e.Update(func(txn *badger.Txn) error {
			dbKey := idxLedEmailPendingKey(userID, accountID, source, key)
			var pending ledgerEmailPending
			err := storage.GetJSON(txn, dbKey, &pending)
			if errors.Is(err, storage.ErrNotFound) {
				return nil
			}
			if err != nil {
				return err
			}
			pending.Attempts++
			attempts = pending.Attempts
			return storage.SetJSON(txn, dbKey, pending)
		})
	})
	return attempts, err
}

// ClearLedgerEmailPending drops all pending actions of a source and returns
// how many there were.
func (s *Store) ClearLedgerEmailPending(_ context.Context, userID string, accountID uuid.UUID, source string) (int, error) {
	count := 0
	err := s.e.Update(func(txn *badger.Txn) error {
		var keys [][]byte
		if err := storage.IteratePrefix(txn, idxLedEmailPendingSourcePrefix(userID, accountID, source), func(key, _ []byte) error {
			keys = append(keys, key)
			return nil
		}); err != nil {
			return err
		}
		count = len(keys)
		for _, key := range keys {
			if err := txn.Delete(key); err != nil {
				return err
			}
		}
		return nil
	})
	return count, err
}
