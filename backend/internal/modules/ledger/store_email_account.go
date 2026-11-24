package ledger

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/dgraph-io/badger/v4"
	"github.com/google/uuid"
	"github.com/reusing-code/kontor/backend/internal/storage"
)

func loadLedgerEmailAccount(txn *badger.Txn, userID string, id uuid.UUID) (LedgerEmailAccount, error) {
	stored, err := storage.Get[storableLedgerEmailAccount](txn, ledEmailAccKey(userID, id))
	return LedgerEmailAccount(stored), err
}

func storeLedgerEmailAccount(txn *badger.Txn, userID string, account LedgerEmailAccount) error {
	return storage.SetJSON(txn, ledEmailAccKey(userID, account.ID), storableLedgerEmailAccount(account))
}

func (s *Store) ListLedgerEmailAccounts(_ context.Context, userID string) ([]LedgerEmailAccount, error) {
	items := []LedgerEmailAccount{}
	err := s.e.View(func(txn *badger.Txn) error {
		stored, err := storage.List[storableLedgerEmailAccount](txn, ledEmailAccPrefix(userID))
		if err != nil {
			return err
		}
		for _, a := range stored {
			items = append(items, LedgerEmailAccount(a))
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(items, func(i, j int) bool {
		if strings.EqualFold(items[i].Name, items[j].Name) {
			return items[i].CreatedAt.Before(items[j].CreatedAt)
		}
		return strings.ToLower(items[i].Name) < strings.ToLower(items[j].Name)
	})
	return items, nil
}

func (s *Store) GetLedgerEmailAccount(_ context.Context, userID string, id uuid.UUID) (LedgerEmailAccount, error) {
	var account LedgerEmailAccount
	err := s.e.View(func(txn *badger.Txn) error {
		var err error
		account, err = loadLedgerEmailAccount(txn, userID, id)
		return err
	})
	return account, err
}

func (s *Store) CreateLedgerEmailAccount(_ context.Context, userID string, account LedgerEmailAccount) error {
	return s.e.Update(func(txn *badger.Txn) error {
		if _, err := txn.Get(ledEmailAccKey(userID, account.ID)); err == nil {
			return storage.ErrConflict
		} else if !errors.Is(err, badger.ErrKeyNotFound) {
			return err
		}
		return storeLedgerEmailAccount(txn, userID, account)
	})
}

func (s *Store) UpdateLedgerEmailAccount(_ context.Context, userID string, account LedgerEmailAccount) error {
	return s.e.Update(func(txn *badger.Txn) error {
		if _, err := txn.Get(ledEmailAccKey(userID, account.ID)); err != nil {
			if errors.Is(err, badger.ErrKeyNotFound) {
				return storage.ErrNotFound
			}
			return err
		}
		return storeLedgerEmailAccount(txn, userID, account)
	})
}

// ModifyLedgerEmailAccount applies fn to the stored account in one
// transaction, so concurrent scan-state writes are not lost.
func (s *Store) ModifyLedgerEmailAccount(_ context.Context, userID string, id uuid.UUID, fn func(*LedgerEmailAccount)) (LedgerEmailAccount, error) {
	var updated LedgerEmailAccount
	err := retryOnConflict(func() error {
		return s.e.Update(func(txn *badger.Txn) error {
			account, err := loadLedgerEmailAccount(txn, userID, id)
			if err != nil {
				return err
			}
			fn(&account)
			updated = account
			return storeLedgerEmailAccount(txn, userID, account)
		})
	})
	return updated, err
}

// RecordLedgerEmailRejection counts how often the server refused to deliver a
// message and returns the new count.
func (s *Store) RecordLedgerEmailRejection(_ context.Context, userID string, accountID uuid.UUID, key string) (int, error) {
	return s.incrementCounter(idxLedEmailRejectKey(userID, accountID, key))
}

// RecordLedgerEmailExtractionFailure counts how often the model gave no
// usable answer for a message and returns the new count.
func (s *Store) RecordLedgerEmailExtractionFailure(_ context.Context, userID string, accountID uuid.UUID, key string) (int, error) {
	return s.incrementCounter(idxLedEmailExtractFailKey(userID, accountID, key))
}

func (s *Store) incrementCounter(k []byte) (int, error) {
	count := 0
	err := retryOnConflict(func() error {
		return s.e.Update(func(txn *badger.Txn) error {
			count = 0
			item, err := txn.Get(k)
			switch {
			case errors.Is(err, badger.ErrKeyNotFound):
			case err != nil:
				return err
			default:
				if err := item.Value(func(val []byte) error {
					count, _ = strconv.Atoi(string(val))
					return nil
				}); err != nil {
					return err
				}
			}
			count++
			return txn.Set(k, []byte(strconv.Itoa(count)))
		})
	})
	return count, err
}

// SetLedgerEmailAccountScanState records a scan outcome without touching any
// other account field, so edits made while the scan ran are not reverted.
func (s *Store) SetLedgerEmailAccountScanState(_ context.Context, userID string, id uuid.UUID, state LedgerEmailScanState) error {
	return retryOnConflict(func() error { return s.setLedgerEmailAccountScanState(userID, id, state) })
}

func (s *Store) setLedgerEmailAccountScanState(userID string, id uuid.UUID, state LedgerEmailScanState) error {
	return s.e.Update(func(txn *badger.Txn) error {
		account, err := loadLedgerEmailAccount(txn, userID, id)
		if err != nil {
			return err
		}
		if account.LastScanStatusMessage == state.Status && state.AttemptedAt == nil && state.CompletedAt == nil {
			return nil
		}
		account.LastScanStatusMessage = state.Status
		if state.AttemptedAt != nil {
			account.LastScanAttemptAt = state.AttemptedAt
			account.ScanResumePending = state.Resume
		}
		if state.CompletedAt != nil {
			account.LastScanAt = state.CompletedAt
		}
		account.UpdatedAt = time.Now().UTC()
		return storeLedgerEmailAccount(txn, userID, account)
	})
}

func (s *Store) DeleteLedgerEmailAccount(_ context.Context, userID string, id uuid.UUID) error {
	return s.e.Update(func(txn *badger.Txn) error {
		account, err := loadLedgerEmailAccount(txn, userID, id)
		if err != nil {
			return err
		}
		orders, err := collectLedgerEmailOrdersByAccount(txn, userID, account.ID)
		if err != nil {
			return err
		}
		for _, order := range orders {
			if err := unlinkLedgerEmailOrderTxn(txn, userID, order.ID); err != nil {
				return err
			}
			if err := txn.Delete(ledEmailOrderKey(userID, order.ID)); err != nil {
				return err
			}
			if err := txn.Delete(idxLedEmailAccOrderKey(userID, order.EmailAccountID, order.ID)); err != nil && !errors.Is(err, badger.ErrKeyNotFound) {
				return err
			}
			if strings.TrimSpace(order.EmailMessageID) != "" {
				if err := txn.Delete(idxLedEmailMsgIDKey(userID, order.EmailMessageID)); err != nil && !errors.Is(err, badger.ErrKeyNotFound) {
					return err
				}
			}
		}
		if err := deleteLedgerEmailSeen(txn, userID, id); err != nil {
			return err
		}
		if err := deleteLedgerEmailDraftData(txn, userID, id); err != nil {
			return err
		}
		return txn.Delete(ledEmailAccKey(userID, id))
	})
}

func deleteLedgerEmailSeen(txn *badger.Txn, userID string, accountID uuid.UUID) error {
	var keys [][]byte
	prefixes := [][]byte{
		idxLedEmailSeenPrefix(userID, accountID),
		idxLedEmailRejectPrefix(userID, accountID),
		idxLedEmailExtractFailPrefix(userID, accountID),
		idxLedEmailPendingPrefix(userID, accountID),
	}
	for _, prefix := range prefixes {
		it := txn.NewIterator(badger.IteratorOptions{Prefix: prefix})
		for it.Rewind(); it.Valid(); it.Next() {
			keys = append(keys, it.Item().KeyCopy(nil))
		}
		it.Close()
	}
	for _, key := range keys {
		if err := txn.Delete(key); err != nil {
			return err
		}
	}
	return nil
}

// HasLedgerEmailSeen reports whether a message of the account was processed
// without producing an order.
func (s *Store) HasLedgerEmailSeen(_ context.Context, userID string, accountID uuid.UUID, key string) (bool, error) {
	seen := false
	err := s.e.View(func(txn *badger.Txn) error {
		_, err := txn.Get(idxLedEmailSeenKey(userID, accountID, key))
		if errors.Is(err, badger.ErrKeyNotFound) {
			return nil
		}
		seen = err == nil
		return err
	})
	return seen, err
}

// MarkLedgerEmailsSeen records messages that need no further processing, in
// one transaction, and drops their extraction failure counters.
func (s *Store) MarkLedgerEmailsSeen(_ context.Context, userID string, accountID uuid.UUID, reason string, keys ...string) error {
	msgs := make([]emailProcessed, 0, len(keys))
	for _, key := range keys {
		msgs = append(msgs, emailProcessed{Key: key})
	}
	return s.markLedgerEmailsSeen(userID, accountID, reason, msgs)
}

// MarkLedgerEmailsProcessed is MarkLedgerEmailsSeen for messages the scan
// dealt with as intended (not an order): in the same transaction it queues
// the mailbox action the account has configured for its order folder.
func (s *Store) MarkLedgerEmailsProcessed(_ context.Context, userID string, accountID uuid.UUID, reason string, msgs ...emailProcessed) error {
	return s.markLedgerEmailsSeen(userID, accountID, reason, msgs)
}

func (s *Store) markLedgerEmailsSeen(userID string, accountID uuid.UUID, reason string, msgs []emailProcessed) error {
	if len(msgs) == 0 {
		return nil
	}
	data, err := json.Marshal(ledgerEmailSeen{Reason: reason, At: time.Now().UTC()})
	if err != nil {
		return err
	}
	return retryOnConflict(func() error {
		return s.e.Update(func(txn *badger.Txn) error {
			for _, m := range msgs {
				if err := txn.Set(idxLedEmailSeenKey(userID, accountID, m.Key), data); err != nil {
					return err
				}
				if err := deleteIfPresent(txn, idxLedEmailExtractFailKey(userID, accountID, m.Key)); err != nil {
					return err
				}
			}
			return queueLedgerEmailActions(txn, userID, accountID, emailSourceOrders, msgs)
		})
	})
}
