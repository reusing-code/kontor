package ledger

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/dgraph-io/badger/v4"
	"github.com/google/uuid"
	"github.com/reusing-code/kontor/backend/internal/storage"
)

// Reasons of a draft marker besides EmailSeenTooLarge and EmailSeenUnreadable.
const (
	EmailDraftSeenAccepted = "accepted"
	EmailDraftSeenRejected = "rejected"
)

// Entity types an email can be linked to: the classifier's targets.
const (
	EmailLinkContract = "contract"
	EmailLinkPurchase = "purchase"
	EmailLinkCost     = "cost"
)

func ValidEmailLinkEntityType(t string) bool {
	return t == EmailLinkContract || t == EmailLinkPurchase || t == EmailLinkCost
}

// ledgerEmailDraftMessage is what is kept of a message while its draft is
// pending, so accepting it needs no mailbox access.
type ledgerEmailDraftMessage struct {
	MessageID string     `json:"messageId,omitempty"`
	Subject   string     `json:"subject,omitempty"`
	From      string     `json:"from,omitempty"`
	Date      *time.Time `json:"date,omitempty"`
}

// LedgerEmailLink is the reference from an entity to the email it was
// created from. MessageID is empty when the message had none that can be
// used in a link.
type LedgerEmailLink struct {
	EntityType     string     `json:"entityType"`
	EntityID       string     `json:"entityId"`
	EmailAccountID uuid.UUID  `json:"emailAccountId"`
	AccountName    string     `json:"accountName"`
	MessageID      string     `json:"messageId,omitempty"`
	Subject        string     `json:"subject,omitempty"`
	From           string     `json:"from,omitempty"`
	Date           *time.Time `json:"date,omitempty"`
	CreatedAt      time.Time  `json:"createdAt"`
}

func idxLedEmailDraftSeenKey(userID string, accountID uuid.UUID, key string) []byte {
	return []byte(fmt.Sprintf("u/%s/mod/ledger/idx/dseen/%s/%s", userID, accountID, key))
}

func idxLedEmailDraftSeenPrefix(userID string, accountID uuid.UUID) []byte {
	return []byte(fmt.Sprintf("u/%s/mod/ledger/idx/dseen/%s/", userID, accountID))
}

func ledEmailDraftMsgKey(userID string, accountID uuid.UUID, key string) []byte {
	return []byte(fmt.Sprintf("u/%s/mod/ledger/edmsg/%s/%s", userID, accountID, key))
}

func ledEmailDraftMsgPrefix(userID string, accountID uuid.UUID) []byte {
	return []byte(fmt.Sprintf("u/%s/mod/ledger/edmsg/%s/", userID, accountID))
}

func ledEmailLinkKey(userID, entityType, entityID string, accountID uuid.UUID, key string) []byte {
	return []byte(fmt.Sprintf("u/%s/mod/ledger/elink/%s/%s/%s/%s", userID, entityType, entityID, accountID, key))
}

func ledEmailLinkEntityPrefix(userID, entityType, entityID string) []byte {
	return []byte(fmt.Sprintf("u/%s/mod/ledger/elink/%s/%s/", userID, entityType, entityID))
}

func ledEmailLinkPrefix(userID string) []byte {
	return []byte(fmt.Sprintf("u/%s/mod/ledger/elink/", userID))
}

// HasLedgerEmailDraftSeen reports whether a message of the account's drafts
// folder needs no further processing: its draft was decided or it cannot be
// read. The markers are separate from those of the order scan.
func (s *Store) HasLedgerEmailDraftSeen(_ context.Context, userID string, accountID uuid.UUID, key string) (bool, error) {
	seen := false
	err := s.e.View(func(txn *badger.Txn) error {
		_, err := txn.Get(idxLedEmailDraftSeenKey(userID, accountID, key))
		if errors.Is(err, badger.ErrKeyNotFound) {
			return nil
		}
		seen = err == nil
		return err
	})
	return seen, err
}

func setLedgerEmailDraftSeen(txn *badger.Txn, userID string, accountID uuid.UUID, key, reason string) error {
	return storage.SetJSON(txn, idxLedEmailDraftSeenKey(userID, accountID, key), ledgerEmailSeen{Reason: reason, At: time.Now().UTC()})
}

func (s *Store) MarkLedgerEmailDraftSeen(_ context.Context, userID string, accountID uuid.UUID, reason, key string) error {
	return s.e.Update(func(txn *badger.Txn) error {
		return setLedgerEmailDraftSeen(txn, userID, accountID, key, reason)
	})
}

func (s *Store) putLedgerEmailDraftMessage(userID string, accountID uuid.UUID, key string, msg ledgerEmailDraftMessage) error {
	return s.e.Update(func(txn *badger.Txn) error {
		return storage.SetJSON(txn, ledEmailDraftMsgKey(userID, accountID, key), msg)
	})
}

func (s *Store) deleteLedgerEmailDraftMessage(userID string, accountID uuid.UUID, key string) error {
	return s.e.Update(func(txn *badger.Txn) error {
		return deleteIfPresent(txn, ledEmailDraftMsgKey(userID, accountID, key))
	})
}

// AcceptLedgerEmailDraft links the message to the entity created from its
// draft and marks it as done, in one transaction. linked is false when the
// account or the stored message data is gone: then there is nothing to link.
func (s *Store) AcceptLedgerEmailDraft(ctx context.Context, userID string, accountID uuid.UUID, key, entityType, entityID string) (linked bool, err error) {
	err = storage.RetryConflicts(ctx, referenceRetries, func() error {
		linked = false
		return s.e.Update(func(txn *badger.Txn) error {
			account, err := loadLedgerEmailAccount(txn, userID, accountID)
			if errors.Is(err, storage.ErrNotFound) {
				return nil
			}
			if err != nil {
				return err
			}
			if err := setLedgerEmailDraftSeen(txn, userID, accountID, key, EmailDraftSeenAccepted); err != nil {
				return err
			}
			var msg ledgerEmailDraftMessage
			err = storage.GetJSON(txn, ledEmailDraftMsgKey(userID, accountID, key), &msg)
			if errors.Is(err, storage.ErrNotFound) {
				return nil
			}
			if err != nil {
				return err
			}
			linked = true
			link := LedgerEmailLink{
				EntityType: entityType, EntityID: entityID, EmailAccountID: accountID, AccountName: account.Name,
				MessageID: msg.MessageID, Subject: msg.Subject, From: msg.From, Date: msg.Date, CreatedAt: time.Now().UTC(),
			}
			if err := storage.SetJSON(txn, ledEmailLinkKey(userID, entityType, entityID, accountID, key), link); err != nil {
				return err
			}
			return txn.Delete(ledEmailDraftMsgKey(userID, accountID, key))
		})
	})
	return linked, err
}

// DecideLedgerEmailDraft marks the message with the reason (accepted without
// a link, or rejected) so it is not proposed again.
func (s *Store) DecideLedgerEmailDraft(ctx context.Context, userID string, accountID uuid.UUID, key, reason string) error {
	return storage.RetryConflicts(ctx, referenceRetries, func() error {
		return s.e.Update(func(txn *badger.Txn) error {
			if _, err := loadLedgerEmailAccount(txn, userID, accountID); errors.Is(err, storage.ErrNotFound) {
				return nil
			} else if err != nil {
				return err
			}
			if err := setLedgerEmailDraftSeen(txn, userID, accountID, key, reason); err != nil {
				return err
			}
			return deleteIfPresent(txn, ledEmailDraftMsgKey(userID, accountID, key))
		})
	})
}

// ListLedgerEmailLinks returns the emails an entity was created from, oldest
// link first.
func (s *Store) ListLedgerEmailLinks(_ context.Context, userID, entityType, entityID string) ([]LedgerEmailLink, error) {
	links := []LedgerEmailLink{}
	err := s.e.View(func(txn *badger.Txn) error {
		return storage.IteratePrefix(txn, ledEmailLinkEntityPrefix(userID, entityType, entityID), func(_, val []byte) error {
			var l LedgerEmailLink
			if err := json.Unmarshal(val, &l); err != nil {
				return err
			}
			links = append(links, l)
			return nil
		})
	})
	if err != nil {
		return nil, err
	}
	slices.SortStableFunc(links, func(a, b LedgerEmailLink) int { return a.CreatedAt.Compare(b.CreatedAt) })
	return links, nil
}

// deleteLedgerEmailDraftData removes everything the draft source stored for
// an account: markers, pending message data and its email links.
func deleteLedgerEmailDraftData(txn *badger.Txn, userID string, accountID uuid.UUID) error {
	var keys [][]byte
	for _, prefix := range [][]byte{idxLedEmailDraftSeenPrefix(userID, accountID), ledEmailDraftMsgPrefix(userID, accountID)} {
		if err := storage.IteratePrefix(txn, prefix, func(key, _ []byte) error {
			keys = append(keys, key)
			return nil
		}); err != nil {
			return err
		}
	}
	if err := storage.IteratePrefix(txn, ledEmailLinkPrefix(userID), func(key, val []byte) error {
		var l struct {
			EmailAccountID uuid.UUID `json:"emailAccountId"`
		}
		if err := json.Unmarshal(val, &l); err != nil {
			return err
		}
		if l.EmailAccountID == accountID {
			keys = append(keys, key)
		}
		return nil
	}); err != nil {
		return err
	}
	for _, key := range keys {
		if err := txn.Delete(key); err != nil {
			return err
		}
	}
	return nil
}
