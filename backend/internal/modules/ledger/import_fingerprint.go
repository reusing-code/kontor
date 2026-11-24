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

// ImportFingerprints returns one dedup key per row of a file. Identical rows
// are told apart by their occurrence: the first keeps the plain Fingerprint,
// so keys stored by earlier versions stay valid, the n-th (n >= 2) hashes the
// plain fingerprint with n. Two identical payments in one file thus both
// import, and an overlapping file finds each of them again.
func ImportFingerprints(accountID string, rows []ParsedRow) []string {
	out := make([]string, len(rows))
	seen := make(map[string]int, len(rows))
	for i, row := range rows {
		fp := Fingerprint(accountID, row)
		seen[fp]++
		if n := seen[fp]; n > 1 {
			fp = fmt.Sprintf("%x", sha256.Sum256(fmt.Appendf(nil, "%s#%d", fp, n)))
		}
		out[i] = fp
	}
	return out
}
