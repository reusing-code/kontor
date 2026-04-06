package migration

import "github.com/dgraph-io/badger/v4"

var V3Ledger = Migration{
	Version:     3,
	Description: "add ledger keyspace (accounts, transactions, imports)",
	Run: func(_ *badger.DB) error {
		return nil
	},
}
