package ledger

import (
	"context"

	"github.com/dgraph-io/badger/v4"
)

// LoadLedgerAnalysisData reads what the category analyses need in one read transaction.
func (s *Store) LoadLedgerAnalysisData(_ context.Context, userID string) (LedgerAnalysisInput, error) {
	var in LedgerAnalysisInput
	err := s.e.View(func(txn *badger.Txn) error {
		var err error
		if in.Accounts, err = collectLedgerAccounts(txn, userID); err != nil {
			return err
		}
		if in.Categories, err = collectLedgerCategories(txn, userID); err != nil {
			return err
		}
		in.Transactions, err = collectLedgerTransactions(txn, userID)
		return err
	})
	return in, err
}
