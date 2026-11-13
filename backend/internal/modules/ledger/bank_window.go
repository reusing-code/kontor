package ledger

import "time"

const (
	bankLookbackDays = 14
	bankFallbackDays = 90
)

// BankWindow is the date range of one account fetch. Longest asks the bank for
// everything it has and sends no dates.
type BankWindow struct {
	Longest  bool
	DateFrom string
	DateTo   string
}

// BankSyncWindow picks what to fetch: the longest history while the ledger
// account has no transactions, else the newest stored booking date minus a
// 14 day overlap up to today (UTC).
func BankSyncWindow(newestBooking string, hasTransactions bool, today time.Time) BankWindow {
	if !hasTransactions {
		return BankWindow{Longest: true}
	}
	today = today.UTC().Truncate(24 * time.Hour)
	from := today.AddDate(0, 0, -bankFallbackDays)
	if newest, err := time.Parse(time.DateOnly, newestBooking); err == nil {
		from = newest.AddDate(0, 0, -bankLookbackDays)
	}
	if from.After(today) {
		from = today
	}
	return BankWindow{DateFrom: from.Format(time.DateOnly), DateTo: today.Format(time.DateOnly)}
}
