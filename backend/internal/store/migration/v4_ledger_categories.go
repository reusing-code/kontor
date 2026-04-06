package migration

import "github.com/dgraph-io/badger/v4"

var V4LedgerCategories = Migration{
	Version:     4,
	Description: "add ledger categories and review metadata",
	Run: func(_ *badger.DB) error {
		return nil
	},
}
