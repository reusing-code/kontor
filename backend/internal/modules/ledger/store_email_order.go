package ledger

import (
	"context"
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/dgraph-io/badger/v4"
	"github.com/google/uuid"
	"github.com/reusing-code/kontor/backend/internal/storage"
	"github.com/reusing-code/kontor/backend/internal/storage/link"
)

func loadLedgerEmailOrder(txn *badger.Txn, userID string, id uuid.UUID) (LedgerEmailOrder, error) {
	order, err := storage.Get[LedgerEmailOrder](txn, ledEmailOrderKey(userID, id))
	if err != nil {
		return order, err
	}
	return NormalizeLedgerEmailOrder(order), nil
}

func storeLedgerEmailOrder(txn *badger.Txn, userID string, order LedgerEmailOrder) error {
	order = NormalizeLedgerEmailOrder(order)
	if err := storage.SetJSON(txn, ledEmailOrderKey(userID, order.ID), order); err != nil {
		return err
	}
	if err := txn.Set(idxLedEmailAccOrderKey(userID, order.EmailAccountID, order.ID), nil); err != nil {
		return err
	}
	if strings.TrimSpace(order.EmailMessageID) != "" {
		if err := txn.Set(idxLedEmailMsgIDKey(userID, order.EmailMessageID), []byte(order.ID.String())); err != nil {
			return err
		}
	}
	return nil
}

func collectLedgerEmailOrders(txn *badger.Txn, userID string) ([]LedgerEmailOrder, error) {
	items, err := storage.List[LedgerEmailOrder](txn, ledEmailOrderPrefix(userID))
	if err != nil {
		return nil, err
	}
	for i := range items {
		items[i] = NormalizeLedgerEmailOrder(items[i])
	}
	return items, nil
}

func collectLedgerEmailOrdersByAccount(txn *badger.Txn, userID string, accountID uuid.UUID) ([]LedgerEmailOrder, error) {
	all, err := collectLedgerEmailOrders(txn, userID)
	if err != nil {
		return nil, err
	}
	items := make([]LedgerEmailOrder, 0)
	for _, item := range all {
		if item.EmailAccountID == accountID {
			items = append(items, item)
		}
	}
	return items, nil
}

func (s *Store) ListLedgerEmailOrders(_ context.Context, userID string) ([]LedgerEmailOrder, error) {
	items := []LedgerEmailOrder{}
	err := s.e.View(func(txn *badger.Txn) error {
		var err error
		items, err = collectLedgerEmailOrders(txn, userID)
		return err
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].OrderDate == items[j].OrderDate {
			return items[i].CreatedAt.After(items[j].CreatedAt)
		}
		return items[i].OrderDate > items[j].OrderDate
	})
	return items, nil
}

func (s *Store) ListLedgerEmailOrdersByAccount(_ context.Context, userID string, accountID uuid.UUID) ([]LedgerEmailOrder, error) {
	items := []LedgerEmailOrder{}
	err := s.e.View(func(txn *badger.Txn) error {
		var err error
		items, err = collectLedgerEmailOrdersByAccount(txn, userID, accountID)
		return err
	})
	return items, err
}

func (s *Store) ListLedgerEmailOrdersByTransaction(_ context.Context, userID string, transactionID uuid.UUID) ([]LedgerEmailOrder, error) {
	items, err := s.ListLedgerEmailOrders(context.Background(), userID)
	if err != nil {
		return nil, err
	}
	filtered := make([]LedgerEmailOrder, 0)
	for _, item := range items {
		if containsUUID(item.LinkedTransactionIDs, transactionID) {
			filtered = append(filtered, item)
		}
	}
	return filtered, nil
}

func (s *Store) GetLedgerEmailOrder(_ context.Context, userID string, id uuid.UUID) (LedgerEmailOrder, error) {
	var order LedgerEmailOrder
	err := s.e.View(func(txn *badger.Txn) error {
		var err error
		order, err = loadLedgerEmailOrder(txn, userID, id)
		return err
	})
	return order, err
}

func (s *Store) GetLedgerEmailOrderByMessageID(_ context.Context, userID string, messageID string) (LedgerEmailOrder, error) {
	var order LedgerEmailOrder
	err := s.e.View(func(txn *badger.Txn) error {
		item, err := txn.Get(idxLedEmailMsgIDKey(userID, messageID))
		if err != nil {
			if errors.Is(err, badger.ErrKeyNotFound) {
				return storage.ErrNotFound
			}
			return err
		}
		var idStr string
		if err := item.Value(func(val []byte) error { idStr = string(val); return nil }); err != nil {
			return err
		}
		id, err := uuid.Parse(idStr)
		if err != nil {
			return err
		}
		order, err = loadLedgerEmailOrder(txn, userID, id)
		return err
	})
	return order, err
}

const ledgerEmailOrderCandidateLimit = 100

// ListLedgerEmailOrderCandidates returns outgoing transactions from all
// accounts that are not linked to an email order yet and were booked within
// window around the order date, closest amount first. targets holds the
// absolute order total per transaction currency; transactions in a currency
// without a target are not comparable by amount and come last.
func (s *Store) ListLedgerEmailOrderCandidates(_ context.Context, userID string, order LedgerEmailOrder, window ChargeWindow, targets map[string]int64) (LedgerEmailOrderCandidatesResult, error) {
	result := LedgerEmailOrderCandidatesResult{Items: []LedgerEmailOrderCandidate{}, ChargeWindow: window}
	err := s.e.View(func(txn *badger.Txn) error {
		accounts, err := collectLedgerAccounts(txn, userID)
		if err != nil {
			return err
		}
		accountNameByID := make(map[uuid.UUID]string, len(accounts))
		for _, account := range accounts {
			accountNameByID[account.ID] = account.Name
		}
		transactions, err := collectLedgerTransactions(txn, userID)
		if err != nil {
			return err
		}
		for _, candidate := range transactions {
			if candidate.AmountMinor >= 0 || len(candidate.EmailOrderIDs) > 0 {
				continue
			}
			if !window.Contains(candidate.BookingDate, order.OrderDate) {
				continue
			}
			_, comparable := targets[normalizeCurrency(candidate.Currency)]
			item := LedgerEmailOrderCandidate{Transaction: candidate, AccountName: accountNameByID[candidate.AccountID], AmountComparable: comparable}
			if delta, ok := ledgerSignedDateDeltaDays(candidate.BookingDate, order.OrderDate); ok {
				item.DateDeltaDays = &delta
			}
			result.Items = append(result.Items, item)
		}
		return nil
	})
	if err != nil {
		return result, err
	}
	amountDelta := func(c LedgerEmailOrderCandidate) int64 {
		return absMinor(-c.Transaction.AmountMinor - targets[normalizeCurrency(c.Transaction.Currency)])
	}
	dateDelta := func(c LedgerEmailOrderCandidate) int {
		if c.DateDeltaDays == nil {
			return 0
		}
		if *c.DateDeltaDays < 0 {
			return -*c.DateDeltaDays
		}
		return *c.DateDeltaDays
	}
	sort.SliceStable(result.Items, func(i, j int) bool {
		left, right := result.Items[i], result.Items[j]
		if left.AmountComparable != right.AmountComparable {
			return left.AmountComparable
		}
		if a, b := amountDelta(left), amountDelta(right); left.AmountComparable && a != b {
			return a < b
		}
		if a, b := dateDelta(left), dateDelta(right); a != b {
			return a < b
		}
		return left.Transaction.BookingDate > right.Transaction.BookingDate
	})
	if len(result.Items) > ledgerEmailOrderCandidateLimit {
		result.Items = result.Items[:ledgerEmailOrderCandidateLimit]
	}
	return result, nil
}

func (s *Store) CreateLedgerEmailOrder(_ context.Context, userID string, order LedgerEmailOrder) error {
	return s.e.Update(func(txn *badger.Txn) error {
		if _, err := txn.Get(ledEmailOrderKey(userID, order.ID)); err == nil {
			return storage.ErrConflict
		} else if !errors.Is(err, badger.ErrKeyNotFound) {
			return err
		}
		return storeLedgerEmailOrder(txn, userID, order)
	})
}

// CreateScannedLedgerEmailOrder stores an order found by a scan. Unlike
// CreateLedgerEmailOrder it refuses (ErrConflict) a message ID that already
// has an order, so two scans sharing a mailbox cannot store a message twice.
// With mailbox (the message came from the account's folder, not an upload)
// the mailbox action configured for the order folder is queued with it.
func (s *Store) CreateScannedLedgerEmailOrder(ctx context.Context, userID string, order LedgerEmailOrder, mailbox *emailMailboxRef) error {
	return retryConflicts(ctx, func() error { return s.createScannedLedgerEmailOrder(userID, order, mailbox) })
}

func (s *Store) createScannedLedgerEmailOrder(userID string, order LedgerEmailOrder, mailbox *emailMailboxRef) error {
	return s.e.Update(func(txn *badger.Txn) error {
		if _, err := txn.Get(ledEmailOrderKey(userID, order.ID)); err == nil {
			return storage.ErrConflict
		} else if !errors.Is(err, badger.ErrKeyNotFound) {
			return err
		}
		if strings.TrimSpace(order.EmailMessageID) != "" {
			if _, err := txn.Get(idxLedEmailMsgIDKey(userID, order.EmailMessageID)); err == nil {
				return storage.ErrConflict
			} else if !errors.Is(err, badger.ErrKeyNotFound) {
				return err
			}
		}
		if err := deleteIfPresent(txn, idxLedEmailExtractFailKey(userID, order.EmailAccountID, order.EmailMessageID)); err != nil {
			return err
		}
		if err := queueLedgerEmailActions(txn, userID, order.EmailAccountID, emailSourceOrders,
			[]emailProcessed{{Key: order.EmailMessageID, Mailbox: mailbox}}); err != nil {
			return err
		}
		return storeLedgerEmailOrder(txn, userID, order)
	})
}

func unlinkLedgerEmailOrderTxn(txn *badger.Txn, userID string, orderID uuid.UUID) error {
	order, err := loadLedgerEmailOrder(txn, userID, orderID)
	if err != nil {
		return err
	}
	for _, transactionID := range order.LinkedTransactionIDs {
		ledgerTxn, err := loadLedgerTransaction(txn, userID, transactionID)
		if err != nil {
			if errors.Is(err, storage.ErrNotFound) {
				continue
			}
			return err
		}
		ledgerTxn.EmailOrderIDs = withoutUUID(ledgerTxn.EmailOrderIDs, order.ID)
		ledgerTxn.UpdatedAt = time.Now().UTC()
		if err := storeLedgerTransaction(txn, userID, ledgerTxn); err != nil {
			return err
		}
	}
	order.LinkedTransactionIDs = []uuid.UUID{}
	order.MatchStatus = LedgerEmailOrderStatusUnmatched
	order.UpdatedAt = time.Now().UTC()
	return storeLedgerEmailOrder(txn, userID, order)
}

func (s *Store) LinkLedgerEmailOrder(_ context.Context, userID string, id uuid.UUID, input LedgerEmailOrderLinkInput) (LedgerEmailOrder, error) {
	updated := LedgerEmailOrder{}
	err := s.e.Update(func(txn *badger.Txn) error {
		order, err := loadLedgerEmailOrder(txn, userID, id)
		if err != nil {
			return err
		}
		for _, existingTxnID := range order.LinkedTransactionIDs {
			ledgerTxn, err := loadLedgerTransaction(txn, userID, existingTxnID)
			if err == nil {
				ledgerTxn.EmailOrderIDs = withoutUUID(ledgerTxn.EmailOrderIDs, order.ID)
				ledgerTxn.UpdatedAt = time.Now().UTC()
				if err := storeLedgerTransaction(txn, userID, ledgerTxn); err != nil {
					return err
				}
			} else if !errors.Is(err, storage.ErrNotFound) {
				return err
			}
		}
		order.LinkedTransactionIDs = link.NormalizeIDs(input.TransactionIDs)
		order.MatchStatus = LedgerEmailOrderStatusMatched
		order.UpdatedAt = time.Now().UTC()
		for _, transactionID := range order.LinkedTransactionIDs {
			ledgerTxn, err := loadLedgerTransaction(txn, userID, transactionID)
			if err != nil {
				return err
			}
			if !containsUUID(ledgerTxn.EmailOrderIDs, order.ID) {
				ledgerTxn.EmailOrderIDs = append(ledgerTxn.EmailOrderIDs, order.ID)
			}
			ledgerTxn.EmailOrderIDs = link.NormalizeIDs(ledgerTxn.EmailOrderIDs)
			ledgerTxn.UpdatedAt = time.Now().UTC()
			if err := storeLedgerTransaction(txn, userID, ledgerTxn); err != nil {
				return err
			}
		}
		if err := storeLedgerEmailOrder(txn, userID, order); err != nil {
			return err
		}
		updated = order
		return nil
	})
	return updated, err
}

func (s *Store) RejectLedgerEmailOrder(_ context.Context, userID string, id uuid.UUID) (LedgerEmailOrder, error) {
	updated := LedgerEmailOrder{}
	err := s.e.Update(func(txn *badger.Txn) error {
		if err := unlinkLedgerEmailOrderTxn(txn, userID, id); err != nil {
			return err
		}
		order, err := loadLedgerEmailOrder(txn, userID, id)
		if err != nil {
			return err
		}
		order.MatchStatus = LedgerEmailOrderStatusRejected
		order.UpdatedAt = time.Now().UTC()
		if err := storeLedgerEmailOrder(txn, userID, order); err != nil {
			return err
		}
		updated = order
		return nil
	})
	return updated, err
}
