package ledger

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

// LedgerBalanceSourceManual marks a balance the user entered by hand; the
// other sources are the SourceType of the import that reported the balance.
const LedgerBalanceSourceManual = "manual"

// LedgerBalanceSnapshot is the balance of an account at the end of day AsOf:
// it includes every transaction booked on or before AsOf.
type LedgerBalanceSnapshot struct {
	AccountID   uuid.UUID `json:"accountId"`
	AsOf        string    `json:"asOf"`
	AmountMinor int64     `json:"amountMinor"`
	Source      string    `json:"source"`
	RecordedAt  time.Time `json:"recordedAt"`
}

func (s LedgerBalanceSnapshot) validate() error {
	if s.AccountID == uuid.Nil {
		return errors.New("accountId is required")
	}
	if _, err := time.Parse(time.DateOnly, s.AsOf); err != nil {
		return errors.New("asOf must be a date (YYYY-MM-DD)")
	}
	if s.Source == "" {
		return errors.New("source is required")
	}
	return nil
}

// LedgerAccountBalance is the balance derived from the newest snapshot (the
// anchor) and the transactions booked after it. DriftMinor compares the anchor
// with the snapshot before it plus the transactions in between; it is only set
// when there is an earlier snapshot, and anything but zero means transactions
// are missing, duplicated or skipped.
type LedgerAccountBalance struct {
	CurrentMinor  int64  `json:"currentMinor"`
	SnapshotMinor int64  `json:"snapshotMinor"`
	AsOf          string `json:"asOf"`
	Source        string `json:"source"`
	DriftMinor    *int64 `json:"driftMinor,omitempty"`
	PreviousAsOf  string `json:"previousAsOf,omitempty"`
}

// computeLedgerAccountBalance needs the account's snapshots sorted by AsOf and
// the account's transactions in any order; it returns nil without a snapshot.
func computeLedgerAccountBalance(snapshots []LedgerBalanceSnapshot, txns []LedgerTransaction) *LedgerAccountBalance {
	if len(snapshots) == 0 {
		return nil
	}
	anchor := snapshots[len(snapshots)-1]
	balance := &LedgerAccountBalance{
		CurrentMinor:  anchor.AmountMinor + sumBookedBetween(txns, anchor.AsOf, ""),
		SnapshotMinor: anchor.AmountMinor,
		AsOf:          anchor.AsOf,
		Source:        anchor.Source,
	}
	if len(snapshots) > 1 {
		previous := snapshots[len(snapshots)-2]
		drift := anchor.AmountMinor - previous.AmountMinor - sumBookedBetween(txns, previous.AsOf, anchor.AsOf)
		balance.DriftMinor = &drift
		balance.PreviousAsOf = previous.AsOf
	}
	return balance
}

// sumBookedBetween adds the transactions booked after `after` and, unless
// upTo is empty, on or before upTo.
func sumBookedBetween(txns []LedgerTransaction, after, upTo string) int64 {
	var sum int64
	for _, t := range txns {
		if t.BookingDate > after && (upTo == "" || t.BookingDate <= upTo) {
			sum += t.AmountMinor
		}
	}
	return sum
}
