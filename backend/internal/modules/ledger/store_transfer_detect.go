package ledger

import (
	"context"
	"strings"
	"time"

	"github.com/dgraph-io/badger/v4"
	"github.com/google/uuid"
	"github.com/reusing-code/kontor/backend/internal/storage"
)

const (
	transferDetectRetries = 3
	transferDetectTimeout = 30 * time.Second
)

// ledgerTransferIBANMatch reports whether either side names the other side's
// account IBAN as its counterparty.
func ledgerTransferIBANMatch(accountIBAN map[uuid.UUID]string, a, b LedgerTransaction) bool {
	matches := func(account uuid.UUID, counterpartyIBAN string) bool {
		iban := normalizeIBAN(accountIBAN[account])
		return iban != "" && iban == normalizeIBAN(counterpartyIBAN)
	}
	return matches(b.AccountID, a.CounterpartyIBAN) || matches(a.AccountID, b.CounterpartyIBAN)
}

func sameLedgerCounterparty(a, b LedgerTransaction) bool {
	left := strings.Join(strings.Fields(a.CounterpartyName), " ")
	right := strings.Join(strings.Fields(b.CounterpartyName), " ")
	return left != "" && strings.EqualFold(left, right)
}

// autoLinkableTransfer is a transaction the automatic detection may change:
// not reviewed, not linked and without splits.
func autoLinkableTransfer(t LedgerTransaction) bool {
	return t.ReviewStatus != LedgerTransactionReviewConfirmed &&
		t.TransferPairTransactionID == nil &&
		len(t.Splits) == 0
}

type ledgerTransferKey struct {
	currency    string
	amountMinor int64
}

// detectLedgerTransferPairs returns the pairs the automatic detection links.
// A pair is two linkable transactions that are each other's only possible
// match (isPotentialLedgerTransferMatch, counted over all transactions, so a
// reviewed look-alike makes the pair ambiguous) and that are tied together by
// an IBAN or the same counterparty name.
func detectLedgerTransferPairs(transactions []LedgerTransaction, accounts []LedgerAccount) [][2]LedgerTransaction {
	accountIBAN := make(map[uuid.UUID]string, len(accounts))
	for _, a := range accounts {
		accountIBAN[a.ID] = a.IBAN
	}
	byAmount := map[ledgerTransferKey][]LedgerTransaction{}
	for _, t := range transactions {
		if t.AmountMinor == 0 {
			continue
		}
		key := ledgerTransferKey{t.Currency, t.AmountMinor}
		byAmount[key] = append(byAmount[key], t)
	}
	matchesOf := func(t LedgerTransaction) []LedgerTransaction {
		var out []LedgerTransaction
		for _, c := range byAmount[ledgerTransferKey{t.Currency, -t.AmountMinor}] {
			if isPotentialLedgerTransferMatch(t, c) {
				out = append(out, c)
			}
		}
		return out
	}

	var pairs [][2]LedgerTransaction
	paired := map[uuid.UUID]bool{}
	for _, t := range transactions {
		if t.AmountMinor >= 0 || paired[t.ID] || !autoLinkableTransfer(t) {
			continue
		}
		matches := matchesOf(t)
		if len(matches) != 1 {
			continue
		}
		other := matches[0]
		if paired[other.ID] || !autoLinkableTransfer(other) {
			continue
		}
		if back := matchesOf(other); len(back) != 1 || back[0].ID != t.ID {
			continue
		}
		if !ledgerTransferIBANMatch(accountIBAN, t, other) && !sameLedgerCounterparty(t, other) {
			continue
		}
		paired[t.ID], paired[other.ID] = true, true
		pairs = append(pairs, [2]LedgerTransaction{t, other})
	}
	return pairs
}

// AutoLinkLedgerTransfers links the clear-cut internal transfers among the
// user's unreviewed transactions (see detectLedgerTransferPairs) and returns
// how many pairs it linked. Linked transactions get the internal transfer
// special category and count as reviewed, like a manual link.
func (s *Store) AutoLinkLedgerTransfers(ctx context.Context, userID string) (int, error) {
	linked := 0
	err := storage.RetryConflicts(ctx, transferDetectRetries, func() error {
		linked = 0
		return s.e.Update(func(txn *badger.Txn) error {
			transactions, err := collectLedgerTransactions(txn, userID)
			if err != nil {
				return err
			}
			accounts, err := collectLedgerAccounts(txn, userID)
			if err != nil {
				return err
			}
			for _, pair := range detectLedgerTransferPairs(transactions, accounts) {
				left, right := pair[0], pair[1]
				applyLedgerTransferState(&left, right.ID)
				applyLedgerTransferState(&right, left.ID)
				if err := storeLedgerTransaction(txn, userID, left); err != nil {
					return err
				}
				if err := storeLedgerTransaction(txn, userID, right); err != nil {
					return err
				}
				linked++
			}
			return nil
		})
	})
	return linked, err
}
