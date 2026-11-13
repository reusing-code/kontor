package ledger

import (
	"cmp"
	"crypto/sha256"
	"encoding/hex"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Billing intervals of detected groups; the values match the contracts
// module's billing intervals.
const (
	RecurringMonthly   = "monthly"
	RecurringQuarterly = "quarterly"
	RecurringYearly    = "yearly"
)

const (
	recurringCurrency = "EUR"
	// A payment joins a chain when it differs from the chain's last amount by
	// at most 10 % of that amount or 1 EUR, whichever is larger.
	recurringAmountToleranceDivisor  = 10
	recurringAmountToleranceMinMinor = 100
	// At least 80 % of the gaps of a chain must lie in the interval's window.
	recurringMinGapSharePercent = 80
	// A group whose last payment is older than this many intervals has ended.
	recurringEndedAfterIntervals = 2
	recurringRefLength           = 32
)

type recurringWindow struct {
	interval       string
	minDays        int
	maxDays        int
	minCount       int
	intervalMonths int
}

var recurringWindows = []recurringWindow{
	{interval: RecurringMonthly, minDays: 26, maxDays: 35, minCount: 3, intervalMonths: 1},
	{interval: RecurringQuarterly, minDays: 80, maxDays: 100, minCount: 3, intervalMonths: 3},
	{interval: RecurringYearly, minDays: 350, maxDays: 380, minCount: 2, intervalMonths: 12},
}

// RecurringGroup is a series of outgoing payments to one counterparty with a
// stable amount and a regular interval.
type RecurringGroup struct {
	Ref            string
	Name           string
	Interval       string
	PriceMinor     int64
	StartDate      string
	LastDate       string
	Ended          bool
	TransactionIDs []uuid.UUID
	Count          int
}

type recurringPayment struct {
	id     uuid.UUID
	date   time.Time
	amount int64
	txn    LedgerTransaction
}

// DetectRecurring finds recurring outgoing EUR payments. Transactions are
// grouped by counterparty (name tokens, else IBAN), split into chains of
// similar amounts and classified by the median gap between payments. A chain
// that contains a transaction already linked to a contract is skipped, so a
// contract created from a group is not proposed again once new payments
// arrive.
func DetectRecurring(txns []LedgerTransaction, today time.Time) []RecurringGroup {
	byKey := map[string][]recurringPayment{}
	for _, t := range txns {
		key, ok := recurringKey(t)
		if !ok || !isRecurringCandidate(t) {
			continue
		}
		date, err := time.Parse(time.DateOnly, t.BookingDate)
		if err != nil {
			continue
		}
		byKey[key] = append(byKey[key], recurringPayment{id: t.ID, date: date, amount: -t.AmountMinor, txn: t})
	}

	todayDate := time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, time.UTC)
	groups := []RecurringGroup{}
	for key, payments := range byKey {
		slices.SortFunc(payments, func(a, b recurringPayment) int {
			if c := a.date.Compare(b.date); c != 0 {
				return c
			}
			return strings.Compare(a.id.String(), b.id.String())
		})
		for _, chain := range amountChains(payments) {
			if g, ok := classifyChain(key, chain, todayDate); ok {
				groups = append(groups, g)
			}
		}
	}
	slices.SortFunc(groups, func(a, b RecurringGroup) int {
		return cmp.Or(
			strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name)),
			strings.Compare(a.StartDate, b.StartDate),
			strings.Compare(a.Ref, b.Ref),
		)
	})
	return groups
}

func isRecurringCandidate(t LedgerTransaction) bool {
	return t.AmountMinor < 0 &&
		strings.EqualFold(strings.TrimSpace(t.Currency), recurringCurrency) &&
		t.SpecialCategory != LedgerSpecialCategoryInternalTransfer &&
		t.TransferPairTransactionID == nil
}

func hasContractReference(t LedgerTransaction) bool {
	return slices.ContainsFunc(t.References, func(r LedgerTransactionReference) bool {
		return r.Type == LedgerReferenceContract
	})
}

func recurringKey(t LedgerTransaction) (string, bool) {
	if tokens := counterpartyTokens(t.CounterpartyName); len(tokens) > 0 {
		return strings.Join(tokens, " "), true
	}
	if iban := normalizeRecurringIBAN(t.CounterpartyIBAN); iban != "" {
		return "iban:" + iban, true
	}
	return "", false
}

func normalizeRecurringIBAN(s string) string {
	return strings.ToUpper(strings.Join(strings.Fields(s), ""))
}

// amountChains splits date-ordered payments into chains: a payment joins the
// first chain whose last amount is close enough, otherwise it starts one.
func amountChains(payments []recurringPayment) [][]recurringPayment {
	var chains [][]recurringPayment
	for _, p := range payments {
		joined := false
		for i, chain := range chains {
			if amountsClose(chain[len(chain)-1].amount, p.amount) {
				chains[i] = append(chain, p)
				joined = true
				break
			}
		}
		if !joined {
			chains = append(chains, []recurringPayment{p})
		}
	}
	return chains
}

func amountsClose(last, next int64) bool {
	diff := next - last
	if diff < 0 {
		diff = -diff
	}
	return diff <= recurringAmountToleranceMinMinor || diff*recurringAmountToleranceDivisor <= last
}

func classifyChain(key string, chain []recurringPayment, today time.Time) (RecurringGroup, bool) {
	if len(chain) < 2 || slices.ContainsFunc(chain, func(p recurringPayment) bool { return hasContractReference(p.txn) }) {
		return RecurringGroup{}, false
	}
	gaps := make([]int, 0, len(chain)-1)
	for i := 1; i < len(chain); i++ {
		gaps = append(gaps, int(chain[i].date.Sub(chain[i-1].date).Hours()/24))
	}
	median := medianInt(gaps)
	for _, w := range recurringWindows {
		if median < w.minDays || median > w.maxDays {
			continue
		}
		inside := 0
		for _, g := range gaps {
			if g >= w.minDays && g <= w.maxDays {
				inside++
			}
		}
		if len(chain) < w.minCount || inside*100 < len(gaps)*recurringMinGapSharePercent {
			return RecurringGroup{}, false
		}
		return buildRecurringGroup(key, w, chain, today), true
	}
	return RecurringGroup{}, false
}

func medianInt(values []int) int {
	sorted := slices.Sorted(slices.Values(values))
	n := len(sorted)
	if n%2 == 1 {
		return sorted[n/2]
	}
	return (sorted[n/2-1] + sorted[n/2]) / 2
}

func buildRecurringGroup(key string, w recurringWindow, chain []recurringPayment, today time.Time) RecurringGroup {
	first, last := chain[0], chain[len(chain)-1]
	ids := make([]uuid.UUID, len(chain))
	for i, p := range chain {
		ids[i] = p.id
	}
	name := strings.TrimSpace(last.txn.CounterpartyName)
	if name == "" {
		name = normalizeRecurringIBAN(last.txn.CounterpartyIBAN)
	}
	endedBefore := today.AddDate(0, -recurringEndedAfterIntervals*w.intervalMonths, 0)
	return RecurringGroup{
		Ref:            recurringRef(key, w.interval, first.id),
		Name:           name,
		Interval:       w.interval,
		PriceMinor:     last.amount,
		StartDate:      first.txn.BookingDate,
		LastDate:       last.txn.BookingDate,
		Ended:          last.date.Before(endedBefore),
		TransactionIDs: ids,
		Count:          len(chain),
	}
}

func recurringRef(key, interval string, firstID uuid.UUID) string {
	sum := sha256.Sum256([]byte(key + "|" + interval + "|" + firstID.String()))
	return hex.EncodeToString(sum[:])[:recurringRefLength]
}
