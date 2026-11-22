package ledger

import (
	"cmp"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/google/uuid"
	"github.com/reusing-code/kontor/backend/internal/storage"
)

const (
	maxLedgerAnalysisMonths = 1200
	ledgerAnalysisEUR       = "EUR"
)

// ErrLedgerAnalysisInvalid marks a request the analyses refuse (400).
var ErrLedgerAnalysisInvalid = errors.New("analysis")

func analysisError(msg string) error {
	return fmt.Errorf("%w: %s", ErrLedgerAnalysisInvalid, msg)
}

// LedgerAnalysisRange is an inclusive booking-date range.
type LedgerAnalysisRange struct{ From, To string }

// resolveLedgerAnalysisRange fills the omitted ends (to = today, from =
// earliest, or to when earliest is empty or after to) and checks the result.
func resolveLedgerAnalysisRange(from, to, today, earliest string) (LedgerAnalysisRange, error) {
	if to == "" {
		to = today
	}
	if from == "" {
		from = earliest
		if from == "" || from > to {
			from = to
		}
	}
	switch {
	case !validISODate(from):
		return LedgerAnalysisRange{}, analysisError("from must be a date (YYYY-MM-DD)")
	case !validISODate(to):
		return LedgerAnalysisRange{}, analysisError("to must be a date (YYYY-MM-DD)")
	case from > to:
		return LedgerAnalysisRange{}, analysisError("from must not be after to")
	}
	rng := LedgerAnalysisRange{From: from, To: to}
	if rng.monthIndex(to)+1 > maxLedgerAnalysisMonths {
		return LedgerAnalysisRange{}, analysisError(fmt.Sprintf("the range must not span more than %d months", maxLedgerAnalysisMonths))
	}
	return rng, nil
}

// validISODate checks the YYYY-MM-DD shape without parsing a time.
func validISODate(s string) bool {
	if len(s) != 10 || s[4] != '-' || s[7] != '-' {
		return false
	}
	for i := 0; i < len(s); i++ {
		if i != 4 && i != 7 && (s[i] < '0' || s[i] > '9') {
			return false
		}
	}
	_, month := isoYearMonth(s)
	day := int(s[8]-'0')*10 + int(s[9]-'0')
	return month >= 1 && month <= 12 && day >= 1 && day <= 31
}

func isoYearMonth(date string) (year, month int) {
	year = int(date[0]-'0')*1000 + int(date[1]-'0')*100 + int(date[2]-'0')*10 + int(date[3]-'0')
	month = int(date[5]-'0')*10 + int(date[6]-'0')
	return year, month
}

// months lists the calendar months of the range as YYYY-MM, ascending and
// without gaps.
func (r LedgerAnalysisRange) months() []string {
	year, month := isoYearMonth(r.From)
	months := make([]string, r.monthIndex(r.To)+1)
	for i := range months {
		months[i] = fmt.Sprintf("%04d-%02d", year, month)
		if month++; month > 12 {
			year, month = year+1, 1
		}
	}
	return months
}

// monthIndex is the position of date's month in months(); date must lie in
// the range.
func (r LedgerAnalysisRange) monthIndex(date string) int {
	fromYear, fromMonth := isoYearMonth(r.From)
	year, month := isoYearMonth(date)
	return (year-fromYear)*12 + month - fromMonth
}

func (r LedgerAnalysisRange) contains(date string) bool {
	return validISODate(date) && r.From <= date && date <= r.To
}

// LedgerAnalysisInput is what the category analyses read.
type LedgerAnalysisInput struct {
	Accounts     []LedgerAccount
	Categories   []LedgerCategory
	Transactions []LedgerTransaction
}

// ledgerCategoryTree resolves categories to their top-level ancestor. A
// category whose parent does not exist, or that lies on or below a parent
// cycle, is its own top level.
type ledgerCategoryTree struct {
	byID map[uuid.UUID]LedgerCategory
	top  map[uuid.UUID]uuid.UUID // category -> top-level ancestor (itself for a top level)
	sub  map[uuid.UUID]uuid.UUID // category -> ancestor directly below the top level (uuid.Nil for a top level)
}

func newLedgerCategoryTree(categories []LedgerCategory) ledgerCategoryTree {
	tree := ledgerCategoryTree{
		byID: make(map[uuid.UUID]LedgerCategory, len(categories)),
		top:  make(map[uuid.UUID]uuid.UUID, len(categories)),
		sub:  make(map[uuid.UUID]uuid.UUID, len(categories)),
	}
	for _, c := range categories {
		tree.byID[c.ID] = c
	}
	for _, c := range categories {
		top, sub := c.ID, uuid.Nil
		visited := map[uuid.UUID]struct{}{c.ID: {}}
		for current := c; current.ParentID != nil; {
			parent, ok := tree.byID[*current.ParentID]
			if !ok {
				break
			}
			if _, seen := visited[parent.ID]; seen {
				top, sub = c.ID, uuid.Nil
				break
			}
			visited[parent.ID] = struct{}{}
			top, sub = parent.ID, current.ID
			current = parent
		}
		tree.top[c.ID], tree.sub[c.ID] = top, sub
	}
	return tree
}

// topLevel is uuid.Nil (uncategorized) without a category and for an ID that
// names none.
func (t ledgerCategoryTree) topLevel(id *uuid.UUID) uuid.UUID {
	if id == nil {
		return uuid.Nil
	}
	return t.top[*id]
}

func (t ledgerCategoryTree) isTopLevel(id uuid.UUID) bool {
	top, ok := t.top[id]
	return ok && top == id
}

func isLedgerTransfer(t LedgerTransaction) bool {
	return t.SpecialCategory == LedgerSpecialCategoryInternalTransfer || t.TransferPairTransactionID != nil
}

// ledgerAnalysisCurrency is the currency a transaction counts in: the
// account's when that is not EUR, else the transaction's own, else EUR.
func ledgerAnalysisCurrency(t LedgerTransaction, accountCurrency string) string {
	account := strings.ToUpper(strings.TrimSpace(accountCurrency))
	own := strings.ToUpper(strings.TrimSpace(t.Currency))
	switch {
	case account != "" && account != ledgerAnalysisEUR:
		return account
	case own != "":
		return own
	default:
		return ledgerAnalysisEUR
	}
}

// eachLedgerPart calls fn with the split parts and the remainder of t.
func eachLedgerPart(t LedgerTransaction, fn func(categoryID *uuid.UUID, amountMinor int64)) {
	for _, part := range t.Splits {
		fn(part.CategoryID, part.AmountMinor)
	}
	fn(t.CategoryID, ledgerSplitRemainder(t))
}

// eachCountedLedgerTransaction calls fn for every transaction that counts in
// the EUR figures of rng (booked inside it, no linked transfer, EUR, of
// accountID when given) and returns the currencies it left out.
func eachCountedLedgerTransaction(in LedgerAnalysisInput, rng LedgerAnalysisRange, accountID *uuid.UUID,
	fn func(t LedgerTransaction, month int)) []string {
	currencies := make(map[uuid.UUID]string, len(in.Accounts))
	for _, a := range in.Accounts {
		currencies[a.ID] = a.Currency
	}
	left := map[string]struct{}{}
	for _, t := range in.Transactions {
		if accountID != nil && t.AccountID != *accountID {
			continue
		}
		if !rng.contains(t.BookingDate) || isLedgerTransfer(t) {
			continue
		}
		if currency := ledgerAnalysisCurrency(t, currencies[t.AccountID]); currency != ledgerAnalysisEUR {
			left[currency] = struct{}{}
			continue
		}
		fn(t, rng.monthIndex(t.BookingDate))
	}
	excluded := make([]string, 0, len(left))
	for currency := range left {
		excluded = append(excluded, currency)
	}
	slices.Sort(excluded)
	return excluded
}

// ledgerNets sums the signed parts per series and month; group maps a part's
// category to its series and reports whether the part belongs to the view.
func ledgerNets(in LedgerAnalysisInput, rng LedgerAnalysisRange, accountID *uuid.UUID,
	group func(categoryID *uuid.UUID) (uuid.UUID, bool)) (nets map[uuid.UUID][]int64, excluded []string) {
	months := rng.monthIndex(rng.To) + 1
	nets = map[uuid.UUID][]int64{}
	excluded = eachCountedLedgerTransaction(in, rng, accountID, func(t LedgerTransaction, month int) {
		eachLedgerPart(t, func(categoryID *uuid.UUID, amountMinor int64) {
			key, ok := group(categoryID)
			if !ok {
				return
			}
			series, found := nets[key]
			if !found {
				series = make([]int64, months)
				nets[key] = series
			}
			series[month] += amountMinor
		})
	})
	return nets, excluded
}

// ledgerTopLevelNets nets every counted part under its top-level category;
// uuid.Nil is the uncategorized series.
func ledgerTopLevelNets(in LedgerAnalysisInput, rng LedgerAnalysisRange, accountID *uuid.UUID,
	tree ledgerCategoryTree) (map[uuid.UUID][]int64, []string) {
	return ledgerNets(in, rng, accountID, func(categoryID *uuid.UUID) (uuid.UUID, bool) {
		return tree.topLevel(categoryID), true
	})
}

// earliestLedgerBooking is the first booking date (of accountID when given),
// or "" without one.
func earliestLedgerBooking(txns []LedgerTransaction, accountID *uuid.UUID) string {
	earliest := ""
	for _, t := range txns {
		if accountID != nil && t.AccountID != *accountID {
			continue
		}
		if validISODate(t.BookingDate) && (earliest == "" || t.BookingDate < earliest) {
			earliest = t.BookingDate
		}
	}
	return earliest
}

const (
	LedgerSeriesCategory      = "category"
	LedgerSeriesUncategorized = "uncategorized"
	LedgerSeriesDirect        = "direct" // booked on the chosen parent itself
)

type LedgerSpendingSeries struct {
	Kind         string     `json:"kind"`
	CategoryID   *uuid.UUID `json:"categoryId,omitempty"`
	Name         string     `json:"name"` // empty for uncategorized and direct
	AmountsMinor []int64    `json:"amountsMinor"`
	TotalMinor   int64      `json:"totalMinor"`
}

// LedgerSpendingAnalysis is the monthly spending per series. In the top-level
// view the amounts are never negative; with ParentCategoryID they are signed
// (a net inflow of a subcategory is negative).
type LedgerSpendingAnalysis struct {
	From               string                 `json:"from"`
	To                 string                 `json:"to"`
	Months             []string               `json:"months"`
	ParentCategoryID   *uuid.UUID             `json:"parentCategoryId,omitempty"`
	Series             []LedgerSpendingSeries `json:"series"`
	TotalsMinor        []int64                `json:"totalsMinor"`
	ExcludedCurrencies []string               `json:"excludedCurrencies"`
}

type LedgerSpendingOptions struct {
	AccountID        *uuid.UUID
	ParentCategoryID *uuid.UUID
}

// AnalyzeLedgerSpending nets the counted amounts per category and month.
// from and to may be empty (see resolveLedgerAnalysisRange).
func AnalyzeLedgerSpending(in LedgerAnalysisInput, from, to, today string, opts LedgerSpendingOptions) (LedgerSpendingAnalysis, error) {
	if opts.AccountID != nil && !slices.ContainsFunc(in.Accounts, func(a LedgerAccount) bool { return a.ID == *opts.AccountID }) {
		return LedgerSpendingAnalysis{}, fmt.Errorf("%w: account", storage.ErrNotFound)
	}
	tree := newLedgerCategoryTree(in.Categories)
	if parent := opts.ParentCategoryID; parent != nil {
		if _, ok := tree.byID[*parent]; !ok {
			return LedgerSpendingAnalysis{}, fmt.Errorf("%w: category", storage.ErrNotFound)
		}
		if !tree.isTopLevel(*parent) {
			return LedgerSpendingAnalysis{}, analysisError("parentCategoryId must be a top-level category")
		}
	}
	rng, err := resolveLedgerAnalysisRange(from, to, today, earliestLedgerBooking(in.Transactions, opts.AccountID))
	if err != nil {
		return LedgerSpendingAnalysis{}, err
	}

	var nets map[uuid.UUID][]int64
	var excluded []string
	if parent := opts.ParentCategoryID; parent != nil {
		nets, excluded = ledgerNets(in, rng, opts.AccountID, func(categoryID *uuid.UUID) (uuid.UUID, bool) {
			if categoryID == nil || tree.topLevel(categoryID) != *parent {
				return uuid.Nil, false
			}
			return tree.sub[*categoryID], true
		})
	} else {
		nets, excluded = ledgerTopLevelNets(in, rng, opts.AccountID, tree)
	}

	months := rng.months()
	series := ledgerSpendingSeries(nets, tree, opts.ParentCategoryID)
	totals := make([]int64, len(months))
	for _, s := range series {
		for m, amount := range s.AmountsMinor {
			totals[m] += amount
		}
	}
	return LedgerSpendingAnalysis{
		From:               rng.From,
		To:                 rng.To,
		Months:             months,
		ParentCategoryID:   opts.ParentCategoryID,
		Series:             series,
		TotalsMinor:        totals,
		ExcludedCurrencies: excluded,
	}, nil
}

// ledgerSpendingSeries turns nets into the listed series, largest first.
// Without a parent the values are the spending max(0, -net) per top-level
// category; with one they are the signed -net of the parent's direct children
// and of the parent itself (key uuid.Nil). A series without a non-zero value
// is left out.
func ledgerSpendingSeries(nets map[uuid.UUID][]int64, tree ledgerCategoryTree, parent *uuid.UUID) []LedgerSpendingSeries {
	series := make([]LedgerSpendingSeries, 0, len(nets))
	for key, net := range nets {
		s := LedgerSpendingSeries{AmountsMinor: make([]int64, len(net))}
		listed := false
		for m, n := range net {
			value := -n
			if parent == nil && value < 0 {
				value = 0
			}
			s.AmountsMinor[m] = value
			s.TotalMinor += value
			listed = listed || value != 0
		}
		if !listed {
			continue
		}
		switch {
		case key != uuid.Nil:
			id := key
			s.Kind, s.CategoryID, s.Name = LedgerSeriesCategory, &id, tree.byID[key].Name
		case parent != nil:
			id := *parent
			s.Kind, s.CategoryID = LedgerSeriesDirect, &id
		default:
			s.Kind = LedgerSeriesUncategorized
		}
		series = append(series, s)
	}
	slices.SortFunc(series, func(a, b LedgerSpendingSeries) int {
		return cmp.Or(
			cmp.Compare(b.TotalMinor, a.TotalMinor),
			cmp.Compare(seriesKindRank(a.Kind), seriesKindRank(b.Kind)),
			cmp.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name)),
			cmp.Compare(seriesID(a), seriesID(b)),
		)
	})
	return series
}

func seriesKindRank(kind string) int {
	if kind == LedgerSeriesCategory {
		return 0
	}
	return 1
}

func seriesID(s LedgerSpendingSeries) string {
	if s.CategoryID == nil {
		return ""
	}
	return s.CategoryID.String()
}
