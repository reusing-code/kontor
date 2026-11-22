package ledger

import (
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/google/uuid"
)

const (
	LedgerNetWorthNoBalance = "no_balance"
	LedgerNetWorthCurrency  = "currency"
)

// LedgerNetWorthInput is what the net worth series reads.
type LedgerNetWorthInput struct {
	Accounts     []LedgerAccount // in list order (name)
	Transactions []LedgerTransaction
	Snapshots    []LedgerBalanceSnapshot
}

type LedgerNetWorthAccount struct {
	AccountID   uuid.UUID `json:"accountId"`
	Name        string    `json:"name"`
	ValuesMinor []int64   `json:"valuesMinor"`
}

type LedgerNetWorthExcluded struct {
	AccountID uuid.UUID `json:"accountId"`
	Name      string    `json:"name"`
	Reason    string    `json:"reason"`
	Currency  string    `json:"currency,omitempty"` // only for reason currency
}

// LedgerNetWorthAnalysis is the balance of the EUR accounts that have a
// balance snapshot at every date of Dates, with their sum. The other accounts
// are listed in ExcludedAccounts and count nowhere.
type LedgerNetWorthAnalysis struct {
	From             string                   `json:"from"`
	To               string                   `json:"to"`
	Dates            []string                 `json:"dates"`
	TotalMinor       []int64                  `json:"totalMinor"`
	Accounts         []LedgerNetWorthAccount  `json:"accounts"`
	ExcludedAccounts []LedgerNetWorthExcluded `json:"excludedAccounts"`
}

// AnalyzeLedgerNetWorth derives the balance history from the snapshots and
// the transactions. from and to may be empty: to = today, from = the first
// snapshot or booking of the included accounts.
func AnalyzeLedgerNetWorth(in LedgerNetWorthInput, from, to, today string) (LedgerNetWorthAnalysis, error) {
	snapshots := map[uuid.UUID][]LedgerBalanceSnapshot{}
	for _, s := range in.Snapshots {
		snapshots[s.AccountID] = append(snapshots[s.AccountID], s)
	}
	included := make([]LedgerAccount, 0, len(in.Accounts))
	excluded := make([]LedgerNetWorthExcluded, 0)
	for _, a := range in.Accounts {
		currency := strings.ToUpper(strings.TrimSpace(a.Currency))
		switch {
		case currency != "" && currency != ledgerAnalysisEUR:
			excluded = append(excluded, LedgerNetWorthExcluded{AccountID: a.ID, Name: a.Name, Reason: LedgerNetWorthCurrency, Currency: currency})
		case len(snapshots[a.ID]) == 0:
			excluded = append(excluded, LedgerNetWorthExcluded{AccountID: a.ID, Name: a.Name, Reason: LedgerNetWorthNoBalance})
		default:
			slices.SortStableFunc(snapshots[a.ID], func(x, y LedgerBalanceSnapshot) int { return strings.Compare(x.AsOf, y.AsOf) })
			included = append(included, a)
		}
	}

	transactions := make(map[uuid.UUID][]LedgerTransaction, len(included))
	for _, a := range included {
		transactions[a.ID] = nil
	}
	for _, t := range in.Transactions {
		if bucket, ok := transactions[t.AccountID]; ok {
			transactions[t.AccountID] = append(bucket, t)
		}
	}

	earliest := ""
	for _, a := range included {
		for _, date := range []string{snapshots[a.ID][0].AsOf, earliestLedgerBooking(transactions[a.ID], nil)} {
			if validISODate(date) && (earliest == "" || date < earliest) {
				earliest = date
			}
		}
	}
	rng, err := resolveLedgerAnalysisRange(from, to, today, earliest)
	if err != nil {
		return LedgerNetWorthAnalysis{}, err
	}
	result := LedgerNetWorthAnalysis{
		From:             rng.From,
		To:               rng.To,
		Dates:            []string{},
		TotalMinor:       []int64{},
		Accounts:         []LedgerNetWorthAccount{},
		ExcludedAccounts: excluded,
	}
	if len(included) == 0 {
		return result, nil
	}

	result.Dates = ledgerNetWorthDates(rng, today)
	result.TotalMinor = make([]int64, len(result.Dates))
	last := len(result.Dates) - 1
	for _, a := range included {
		booked := newBookedSums(transactions[a.ID])
		values := make([]int64, len(result.Dates))
		for i, date := range result.Dates {
			values[i] = ledgerBalanceAt(snapshots[a.ID], booked, date)
		}
		if rng.To >= today {
			values[last] = computeLedgerAccountBalance(snapshots[a.ID], transactions[a.ID]).CurrentMinor
		}
		for i, value := range values {
			result.TotalMinor[i] += value
		}
		result.Accounts = append(result.Accounts, LedgerNetWorthAccount{AccountID: a.ID, Name: a.Name, ValuesMinor: values})
	}
	return result, nil
}

// ledgerNetWorthDates lists the month-ends d with from <= d < last, then
// last = min(to, today).
func ledgerNetWorthDates(rng LedgerAnalysisRange, today string) []string {
	last := min(rng.To, today)
	dates := []string{}
	year, month := isoYearMonth(rng.From)
	for {
		end := fmt.Sprintf("%04d-%02d-%02d", year, month, daysInMonth(year, month))
		if end >= last {
			break
		}
		if end >= rng.From {
			dates = append(dates, end)
		}
		if month++; month > 12 {
			year, month = year+1, 1
		}
	}
	return append(dates, last)
}

func daysInMonth(year, month int) int {
	switch month {
	case 4, 6, 9, 11:
		return 30
	case 2:
		if year%4 == 0 && (year%100 != 0 || year%400 == 0) {
			return 29
		}
		return 28
	default:
		return 31
	}
}

// bookedSums answers sums of amounts by booking date from sorted dates and
// prefix sums.
type bookedSums struct {
	dates  []string
	prefix []int64 // prefix[i] = sum of the first i amounts
}

func newBookedSums(txns []LedgerTransaction) bookedSums {
	sorted := slices.Clone(txns)
	slices.SortFunc(sorted, func(a, b LedgerTransaction) int { return strings.Compare(a.BookingDate, b.BookingDate) })
	sums := bookedSums{dates: make([]string, len(sorted)), prefix: make([]int64, len(sorted)+1)}
	for i, t := range sorted {
		sums.dates[i] = t.BookingDate
		sums.prefix[i+1] = sums.prefix[i] + t.AmountMinor
	}
	return sums
}

// between sums the amounts booked after `after` and on or before upTo.
func (b bookedSums) between(after, upTo string) int64 {
	if upTo <= after {
		return 0
	}
	through := func(date string) int {
		return sort.Search(len(b.dates), func(i int) bool { return b.dates[i] > date })
	}
	return b.prefix[through(upTo)] - b.prefix[through(after)]
}

// ledgerBalanceAt is the balance at the end of day date: the nearest snapshot
// at or before it rolled forward by the later bookings, or, before the first
// snapshot, that snapshot walked back. snapshots must be sorted by AsOf and
// not empty.
func ledgerBalanceAt(snapshots []LedgerBalanceSnapshot, booked bookedSums, date string) int64 {
	i := sort.Search(len(snapshots), func(i int) bool { return snapshots[i].AsOf > date })
	if i == 0 {
		first := snapshots[0]
		return first.AmountMinor - booked.between(date, first.AsOf)
	}
	base := snapshots[i-1]
	return base.AmountMinor + booked.between(base.AsOf, date)
}
