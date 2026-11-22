package ledger

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/dgraph-io/badger/v4"
	"github.com/google/uuid"
	"github.com/reusing-code/kontor/backend/internal/storage"
)

func ledBalKey(userID string, accountID uuid.UUID, asOf string) []byte {
	return []byte(fmt.Sprintf("u/%s/mod/ledger/bal/%s/%s", userID, accountID, asOf))
}

func ledBalAccountPrefix(userID string, accountID uuid.UUID) []byte {
	return []byte(fmt.Sprintf("u/%s/mod/ledger/bal/%s/", userID, accountID))
}

func ledBalPrefix(userID string) []byte {
	return []byte(fmt.Sprintf("u/%s/mod/ledger/bal/", userID))
}

// putLedgerBalanceSnapshotTxn stores the snapshot, replacing one of the same
// account and day. The account must exist.
func putLedgerBalanceSnapshotTxn(txn *badger.Txn, userID string, snapshot LedgerBalanceSnapshot) error {
	if err := snapshot.validate(); err != nil {
		return err
	}
	if _, err := loadLedgerAccountTxn(txn, userID, snapshot.AccountID); err != nil {
		return err
	}
	return storage.SetJSON(txn, ledBalKey(userID, snapshot.AccountID, snapshot.AsOf), snapshot)
}

// collectLedgerBalanceSnapshots returns the snapshots under prefix, sorted by
// account and date (the key order).
func collectLedgerBalanceSnapshots(txn *badger.Txn, prefix []byte) ([]LedgerBalanceSnapshot, error) {
	snapshots := []LedgerBalanceSnapshot{}
	err := storage.IteratePrefix(txn, prefix, func(_, val []byte) error {
		var s LedgerBalanceSnapshot
		if err := json.Unmarshal(val, &s); err != nil {
			return err
		}
		snapshots = append(snapshots, s)
		return nil
	})
	return snapshots, err
}

// accountLedgerTransactions loads an account's transactions through its
// booking-date index.
func accountLedgerTransactions(txn *badger.Txn, userID string, accountID uuid.UUID) ([]LedgerTransaction, error) {
	ids, err := accountTransactionIDs(txn, userID, accountID, 0)
	if err != nil {
		return nil, err
	}
	txns := make([]LedgerTransaction, 0, len(ids))
	for _, id := range ids {
		t, err := loadLedgerTransaction(txn, userID, id)
		if errors.Is(err, storage.ErrNotFound) {
			continue
		}
		if err != nil {
			return nil, err
		}
		txns = append(txns, t)
	}
	return txns, nil
}

func (s *Store) PutLedgerBalanceSnapshot(_ context.Context, userID string, snapshot LedgerBalanceSnapshot) error {
	return retryOnConflict(func() error {
		return s.e.Update(func(txn *badger.Txn) error {
			return putLedgerBalanceSnapshotTxn(txn, userID, snapshot)
		})
	})
}

func (s *Store) ListLedgerBalanceSnapshots(_ context.Context, userID string, accountID uuid.UUID) ([]LedgerBalanceSnapshot, error) {
	var snapshots []LedgerBalanceSnapshot
	err := s.e.View(func(txn *badger.Txn) error {
		var err error
		snapshots, err = collectLedgerBalanceSnapshots(txn, ledBalAccountPrefix(userID, accountID))
		return err
	})
	return snapshots, err
}

// ListAllLedgerBalanceSnapshots returns every snapshot of the user (export).
func (s *Store) ListAllLedgerBalanceSnapshots(_ context.Context, userID string) ([]LedgerBalanceSnapshot, error) {
	var snapshots []LedgerBalanceSnapshot
	err := s.e.View(func(txn *badger.Txn) error {
		var err error
		snapshots, err = collectLedgerBalanceSnapshots(txn, ledBalPrefix(userID))
		return err
	})
	return snapshots, err
}

// GetLedgerAccountBalance returns the account's balance, nil when it has no
// snapshot.
func (s *Store) GetLedgerAccountBalance(_ context.Context, userID string, accountID uuid.UUID) (*LedgerAccountBalance, error) {
	var balance *LedgerAccountBalance
	err := s.e.View(func(txn *badger.Txn) error {
		snapshots, err := collectLedgerBalanceSnapshots(txn, ledBalAccountPrefix(userID, accountID))
		if err != nil || len(snapshots) == 0 {
			return err
		}
		txns, err := accountLedgerTransactions(txn, userID, accountID)
		if err != nil {
			return err
		}
		balance = computeLedgerAccountBalance(snapshots, txns)
		return nil
	})
	return balance, err
}

// ListLedgerAccountBalances returns the balance of every account that has a
// snapshot.
func (s *Store) ListLedgerAccountBalances(_ context.Context, userID string) (map[uuid.UUID]LedgerAccountBalance, error) {
	balances := map[uuid.UUID]LedgerAccountBalance{}
	err := s.e.View(func(txn *badger.Txn) error {
		snapshots, err := collectLedgerBalanceSnapshots(txn, ledBalPrefix(userID))
		if err != nil || len(snapshots) == 0 {
			return err
		}
		byAccount := map[uuid.UUID][]LedgerBalanceSnapshot{}
		for _, snapshot := range snapshots {
			byAccount[snapshot.AccountID] = append(byAccount[snapshot.AccountID], snapshot)
		}
		for accountID, accountSnapshots := range byAccount {
			txns, err := accountLedgerTransactions(txn, userID, accountID)
			if err != nil {
				return err
			}
			if balance := computeLedgerAccountBalance(accountSnapshots, txns); balance != nil {
				balances[accountID] = *balance
			}
		}
		return nil
	})
	return balances, err
}

// LedgerPageBalance is the balance around one page of an account's
// transaction list: ClosingMinor after its newest transaction (top),
// OpeningMinor before its oldest one (bottom).
type LedgerPageBalance struct {
	ClosingMinor int64 `json:"closingMinor"`
	OpeningMinor int64 `json:"openingMinor"`
}

// LedgerAccountTransactionPage is a page of an account's transactions with
// the balances; both are nil when the account has no snapshot.
type LedgerAccountTransactionPage struct {
	LedgerTransactionPage
	PageBalance *LedgerPageBalance
	Balance     *LedgerAccountBalance
}

// ListLedgerAccountTransactionsPage pages an account's transactions like
// ListLedgerTransactionsPage and adds the balances around the page, derived
// from the current balance by taking back every transaction listed above it.
func (s *Store) ListLedgerAccountTransactionsPage(_ context.Context, userID string, accountID uuid.UUID, limit int, cursor string) (LedgerAccountTransactionPage, error) {
	var page LedgerAccountTransactionPage
	err := s.e.View(func(txn *badger.Txn) error {
		txns, err := filterLedgerTransactions(txn, userID, LedgerTransactionListOptions{AccountID: &accountID})
		if err != nil {
			return err
		}
		page.LedgerTransactionPage = paginateLedgerTransactions(txns, limit, cursor)
		snapshots, err := collectLedgerBalanceSnapshots(txn, ledBalAccountPrefix(userID, accountID))
		if err != nil {
			return err
		}
		page.Balance = computeLedgerAccountBalance(snapshots, txns)
		if page.Balance != nil {
			start, end := ledgerPageBounds(txns, limit, cursor)
			closing := page.Balance.CurrentMinor - sumAmounts(txns[:start])
			page.PageBalance = &LedgerPageBalance{ClosingMinor: closing, OpeningMinor: closing - sumAmounts(txns[start:end])}
		}
		return nil
	})
	return page, err
}

func sumAmounts(txns []LedgerTransaction) int64 {
	var sum int64
	for _, t := range txns {
		sum += t.AmountMinor
	}
	return sum
}
