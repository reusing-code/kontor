package ledger

import (
	"crypto/sha256"
	"fmt"
)

// Fingerprint generates a deterministic dedup key for a transaction.
// It hashes: accountID + bookingDate + amountMinor + counterpartyName + purpose + bankReference.
// This means the same real-world transaction imported twice will produce the same fingerprint.
func Fingerprint(accountID string, row ParsedRow) string {
	h := sha256.New()
	fmt.Fprintf(h, "%s|%s|%d|%s|%s|%s",
		accountID,
		row.BookingDate,
		row.AmountMinor,
		row.CounterpartyName,
		row.Purpose,
		row.BankReference,
	)
	return fmt.Sprintf("%x", h.Sum(nil))
}
