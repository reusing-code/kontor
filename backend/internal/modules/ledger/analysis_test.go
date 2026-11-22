package ledger

import (
	"errors"
	"fmt"
	"math/rand/v2"
	"slices"
	"testing"

	"github.com/google/uuid"
	"github.com/reusing-code/kontor/backend/internal/storage"
)

const analysisToday = "2026-11-22"

var analysisAccountID = uuid.MustParse("00000000-0000-0000-0000-0000000000a1")

func analysisAccount(id uuid.UUID, currency string) LedgerAccount {
	return LedgerAccount{ID: id, Name: "Account " + currency, Bank: "DKB", Currency: currency}
}

func analysisCategory(name string, parent *uuid.UUID) LedgerCategory {
	return LedgerCategory{ID: uuid.New(), Name: name, ParentID: parent}
}

// booked is an EUR transaction of the default analysis account.
func booked(date string, amountMinor int64, categoryID *uuid.UUID) LedgerTransaction {
	return LedgerTransaction{
		ID: uuid.New(), AccountID: analysisAccountID, BookingDate: date, AmountMinor: amountMinor,
		Currency: "EUR", CategoryID: categoryID, ReviewStatus: LedgerTransactionReviewConfirmed,
	}
}

func analysisInput(categories []LedgerCategory, txns ...LedgerTransaction) LedgerAnalysisInput {
	return LedgerAnalysisInput{
		Accounts:     []LedgerAccount{analysisAccount(analysisAccountID, "EUR")},
		Categories:   categories,
		Transactions: txns,
	}
}

func spending(t *testing.T, in LedgerAnalysisInput, from, to string, opts LedgerSpendingOptions) LedgerSpendingAnalysis {
	t.Helper()
	got, err := AnalyzeLedgerSpending(in, from, to, analysisToday, opts)
	if err != nil {
		t.Fatalf("AnalyzeLedgerSpending: %v", err)
	}
	return got
}

// seriesAmounts maps each series to its amounts, keyed by the category name
// or, for the series without a category of their own, by the kind.
func seriesAmounts(a LedgerSpendingAnalysis) map[string][]int64 {
	out := map[string][]int64{}
	for _, s := range a.Series {
		key := s.Name
		if s.Kind != LedgerSeriesCategory {
			key = s.Kind
		}
		out[key] = s.AmountsMinor
	}
	return out
}

func seriesOrder(a LedgerSpendingAnalysis) []string {
	order := make([]string, 0, len(a.Series))
	for _, s := range a.Series {
		if s.Kind != LedgerSeriesCategory {
			order = append(order, s.Kind)
			continue
		}
		order = append(order, s.Name)
	}
	return order
}

func wantAmounts(t *testing.T, got LedgerSpendingAnalysis, want map[string][]int64) {
	t.Helper()
	amounts := seriesAmounts(got)
	if len(amounts) != len(want) {
		t.Fatalf("series = %v, want %v", amounts, want)
	}
	for name, values := range want {
		if !slices.Equal(amounts[name], values) {
			t.Fatalf("series %q = %v, want %v (all: %v)", name, amounts[name], values, amounts)
		}
	}
}

func TestLedgerAnalysisRange(t *testing.T) {
	t.Run("months are gap-free across a year change", func(t *testing.T) {
		rng, err := resolveLedgerAnalysisRange("2025-11-15", "2026-02-10", analysisToday, "")
		if err != nil {
			t.Fatal(err)
		}
		if got, want := rng.months(), []string{"2025-11", "2025-12", "2026-01", "2026-02"}; !slices.Equal(got, want) {
			t.Fatalf("months = %v, want %v", got, want)
		}
		if rng.From != "2025-11-15" || rng.To != "2026-02-10" {
			t.Fatalf("range = %+v, the mid-month ends must be kept", rng)
		}
		for date, want := range map[string]int{"2025-11-15": 0, "2025-12-31": 1, "2026-01-01": 2, "2026-02-10": 3} {
			if got := rng.monthIndex(date); got != want {
				t.Fatalf("monthIndex(%s) = %d, want %d", date, got, want)
			}
		}
		for date, want := range map[string]bool{
			"2025-11-14": false, "2025-11-15": true, "2026-02-10": true, "2026-02-11": false, "": false, "2026-1-05": false, "garbage!!!": false,
		} {
			if got := rng.contains(date); got != want {
				t.Fatalf("contains(%q) = %v, want %v", date, got, want)
			}
		}
	})

	t.Run("a single day is one month", func(t *testing.T) {
		rng, err := resolveLedgerAnalysisRange("2026-03-05", "2026-03-05", analysisToday, "")
		if err != nil || !slices.Equal(rng.months(), []string{"2026-03"}) {
			t.Fatalf("range = %+v, %v", rng, err)
		}
	})

	t.Run("from after to", func(t *testing.T) {
		if _, err := resolveLedgerAnalysisRange("2026-03-02", "2026-03-01", analysisToday, ""); !errors.Is(err, ErrLedgerAnalysisInvalid) {
			t.Fatalf("err = %v, want ErrLedgerAnalysisInvalid", err)
		}
	})

	t.Run("at most 1200 months", func(t *testing.T) {
		rng, err := resolveLedgerAnalysisRange("1927-01-31", "2026-12-01", analysisToday, "")
		if err != nil || len(rng.months()) != 1200 {
			t.Fatalf("1200 months: %d, %v", len(rng.months()), err)
		}
		if _, err := resolveLedgerAnalysisRange("1926-12-31", "2026-12-01", analysisToday, ""); !errors.Is(err, ErrLedgerAnalysisInvalid) {
			t.Fatalf("1201 months: err = %v, want ErrLedgerAnalysisInvalid", err)
		}
	})

	t.Run("omitted ends", func(t *testing.T) {
		cases := []struct {
			name, from, to, earliest string
			want                     LedgerAnalysisRange
		}{
			{"both omitted", "", "", "2026-03-05", LedgerAnalysisRange{"2026-03-05", analysisToday}},
			{"to omitted", "2026-10-01", "", "2026-03-05", LedgerAnalysisRange{"2026-10-01", analysisToday}},
			{"from omitted", "", "2026-06-30", "2026-03-05", LedgerAnalysisRange{"2026-03-05", "2026-06-30"}},
			{"nothing booked", "", "", "", LedgerAnalysisRange{analysisToday, analysisToday}},
			{"earliest after to", "", "2026-01-31", "2026-03-05", LedgerAnalysisRange{"2026-01-31", "2026-01-31"}},
		}
		for _, tc := range cases {
			got, err := resolveLedgerAnalysisRange(tc.from, tc.to, analysisToday, tc.earliest)
			if err != nil || got != tc.want {
				t.Fatalf("%s: got %+v, %v, want %+v", tc.name, got, err, tc.want)
			}
		}
	})

	t.Run("malformed dates are refused", func(t *testing.T) {
		for _, tc := range [][2]string{{"2026-13-01", "2026-12-01"}, {"2026-01-01", "tomorrow"}, {"2026-01-00", "2026-12-01"}} {
			if _, err := resolveLedgerAnalysisRange(tc[0], tc[1], analysisToday, ""); !errors.Is(err, ErrLedgerAnalysisInvalid) {
				t.Fatalf("%v: err = %v, want ErrLedgerAnalysisInvalid", tc, err)
			}
		}
	})
}

func TestLedgerCategoryTree(t *testing.T) {
	top := analysisCategory("Top", nil)
	mid := analysisCategory("Mid", &top.ID)
	leaf := analysisCategory("Leaf", &mid.ID)
	gone := uuid.New()
	orphan := analysisCategory("Orphan", &gone)
	orphanChild := analysisCategory("Orphan child", &orphan.ID)
	cycleA := analysisCategory("Cycle A", nil)
	cycleB := analysisCategory("Cycle B", &cycleA.ID)
	cycleA.ParentID = &cycleB.ID
	belowCycle := analysisCategory("Below cycle", &cycleA.ID)

	tree := newLedgerCategoryTree([]LedgerCategory{top, mid, leaf, orphan, orphanChild, cycleA, cycleB, belowCycle})

	cases := []struct {
		name     string
		id       uuid.UUID
		top, sub uuid.UUID
	}{
		{"top level", top.ID, top.ID, uuid.Nil},
		{"second level", mid.ID, top.ID, mid.ID},
		{"third level rolls up", leaf.ID, top.ID, mid.ID},
		{"missing parent is its own top level", orphan.ID, orphan.ID, uuid.Nil},
		{"child of a category with a missing parent", orphanChild.ID, orphan.ID, orphanChild.ID},
		{"cycle member", cycleA.ID, cycleA.ID, uuid.Nil},
		{"other cycle member", cycleB.ID, cycleB.ID, uuid.Nil},
		{"below a cycle", belowCycle.ID, belowCycle.ID, uuid.Nil},
	}
	for _, tc := range cases {
		if got := tree.topLevel(&tc.id); got != tc.top {
			t.Fatalf("%s: top = %s, want %s", tc.name, got, tc.top)
		}
		if got := tree.sub[tc.id]; got != tc.sub {
			t.Fatalf("%s: sub = %s, want %s", tc.name, got, tc.sub)
		}
		if got, want := tree.isTopLevel(tc.id), tc.top == tc.id; got != want {
			t.Fatalf("%s: isTopLevel = %v, want %v", tc.name, got, want)
		}
	}

	unknown := uuid.New()
	if got := tree.topLevel(&unknown); got != uuid.Nil {
		t.Fatalf("unknown ID: top = %s, want uncategorized", got)
	}
	if got := tree.topLevel(nil); got != uuid.Nil {
		t.Fatalf("nil: top = %s, want uncategorized", got)
	}
	if tree.isTopLevel(unknown) {
		t.Fatal("an unknown ID is not a top-level category")
	}
}

func TestAnalyzeLedgerSpending_Range(t *testing.T) {
	food := analysisCategory("Food", nil)
	in := analysisInput([]LedgerCategory{food},
		booked("2026-01-14", -100, &food.ID),
		booked("2026-01-15", -200, &food.ID),
		booked("2026-03-10", -400, &food.ID),
		booked("2026-03-11", -800, &food.ID),
		booked("not-a-date", -1600, &food.ID),
	)

	got := spending(t, in, "2026-01-15", "2026-03-10", LedgerSpendingOptions{})
	if got.From != "2026-01-15" || got.To != "2026-03-10" {
		t.Fatalf("range = %s..%s", got.From, got.To)
	}
	if want := []string{"2026-01", "2026-02", "2026-03"}; !slices.Equal(got.Months, want) {
		t.Fatalf("months = %v, want %v", got.Months, want)
	}
	wantAmounts(t, got, map[string][]int64{"Food": {200, 0, 400}})
	if !slices.Equal(got.TotalsMinor, []int64{200, 0, 400}) || got.Series[0].TotalMinor != 600 {
		t.Fatalf("totals = %v, series total = %d", got.TotalsMinor, got.Series[0].TotalMinor)
	}

	all := spending(t, in, "", "", LedgerSpendingOptions{})
	if all.From != "2026-01-14" || all.To != analysisToday || len(all.Months) != 11 {
		t.Fatalf("all: %s..%s with %d months", all.From, all.To, len(all.Months))
	}
	if all.Series[0].TotalMinor != 1500 {
		t.Fatalf("all: total = %d, want 1500", all.Series[0].TotalMinor)
	}
}

func TestAnalyzeLedgerSpending_CountingRules(t *testing.T) {
	const from, to = "2026-01-01", "2026-02-28"
	food := analysisCategory("Food", nil)
	cash := analysisCategory("Cash", nil)
	salary := analysisCategory("Salary", nil)
	categories := []LedgerCategory{food, cash, salary}

	t.Run("split parts and the remainder", func(t *testing.T) {
		split := booked("2026-01-10", -1000, &food.ID)
		split.Splits = []LedgerTransactionSplit{{AmountMinor: -300, CategoryID: &cash.ID}}
		plain := booked("2026-02-03", -450, &cash.ID)
		got := spending(t, analysisInput(categories, split, plain), from, to, LedgerSpendingOptions{})
		wantAmounts(t, got, map[string][]int64{"Food": {700, 0}, "Cash": {300, 450}})
		if !slices.Equal(got.TotalsMinor, []int64{1000, 450}) {
			t.Fatalf("totals = %v, want the transaction amounts", got.TotalsMinor)
		}
	})

	t.Run("linked transfers are excluded", func(t *testing.T) {
		special := booked("2026-01-05", -500, nil)
		special.SpecialCategory = LedgerSpecialCategoryInternalTransfer
		pair := uuid.New()
		paired := booked("2026-01-06", -700, &food.ID)
		paired.TransferPairTransactionID = &pair
		got := spending(t, analysisInput(categories, special, paired, booked("2026-01-07", -100, &food.ID)), from, to, LedgerSpendingOptions{})
		wantAmounts(t, got, map[string][]int64{"Food": {100, 0}})
	})

	t.Run("an unreviewed transaction counts under its category", func(t *testing.T) {
		unreviewed := booked("2026-01-05", -250, &food.ID)
		unreviewed.ReviewStatus = LedgerTransactionReviewNeedsReview
		got := spending(t, analysisInput(categories, unreviewed), from, to, LedgerSpendingOptions{})
		wantAmounts(t, got, map[string][]int64{"Food": {250, 0}})
	})

	t.Run("amounts without a category are uncategorized", func(t *testing.T) {
		split := booked("2026-01-10", -1000, &food.ID)
		split.Splits = []LedgerTransactionSplit{{AmountMinor: -250}}
		deleted := uuid.New()
		got := spending(t, analysisInput(categories, split, booked("2026-01-11", -100, nil), booked("2026-02-01", -40, &deleted)), from, to, LedgerSpendingOptions{})
		wantAmounts(t, got, map[string][]int64{"Food": {750, 0}, LedgerSeriesUncategorized: {350, 40}})
		for _, s := range got.Series {
			if s.Kind == LedgerSeriesUncategorized && (s.CategoryID != nil || s.Name != "") {
				t.Fatalf("uncategorized series = %+v, want no ID and no name", s)
			}
		}
	})

	t.Run("refunds are netted and never negative", func(t *testing.T) {
		got := spending(t, analysisInput(categories,
			booked("2026-01-05", -1000, &food.ID),
			booked("2026-01-20", 300, &food.ID),
			booked("2026-02-05", -200, &food.ID),
			booked("2026-02-20", 500, &food.ID),
		), from, to, LedgerSpendingOptions{})
		wantAmounts(t, got, map[string][]int64{"Food": {700, 0}})
		if got.Series[0].TotalMinor != 700 || !slices.Equal(got.TotalsMinor, []int64{700, 0}) {
			t.Fatalf("total = %d, totals = %v", got.Series[0].TotalMinor, got.TotalsMinor)
		}
	})

	t.Run("a category with a positive net is absent", func(t *testing.T) {
		got := spending(t, analysisInput(categories,
			booked("2026-01-28", 300_000, &salary.ID),
			booked("2026-02-02", 900, &cash.ID),
			booked("2026-01-05", -100, &food.ID),
		), from, to, LedgerSpendingOptions{})
		wantAmounts(t, got, map[string][]int64{"Food": {100, 0}})
	})

	t.Run("other currencies are left out and reported", func(t *testing.T) {
		usdAccount := uuid.New()
		in := analysisInput(categories, booked("2026-01-05", -100, &food.ID))
		in.Accounts = append(in.Accounts, analysisAccount(usdAccount, "usd"))
		onUSD := booked("2026-01-06", -999, &food.ID)
		onUSD.AccountID, onUSD.Currency = usdAccount, "USD"
		onUSDMarkedEUR := booked("2026-01-07", -999, &food.ID)
		onUSDMarkedEUR.AccountID = usdAccount
		foreign := booked("2026-01-08", -999, &food.ID)
		foreign.Currency = "chf"
		noCurrency := booked("2026-01-09", -20, &food.ID)
		noCurrency.Currency = ""
		outOfRange := booked("2025-12-31", -999, &food.ID)
		outOfRange.Currency = "GBP"
		transfer := booked("2026-01-10", -999, nil)
		transfer.Currency, transfer.SpecialCategory = "SEK", LedgerSpecialCategoryInternalTransfer
		in.Transactions = append(in.Transactions, onUSD, onUSDMarkedEUR, foreign, noCurrency, outOfRange, transfer)

		got := spending(t, in, from, to, LedgerSpendingOptions{})
		wantAmounts(t, got, map[string][]int64{"Food": {120, 0}})
		if want := []string{"CHF", "USD"}; !slices.Equal(got.ExcludedCurrencies, want) {
			t.Fatalf("excludedCurrencies = %v, want %v", got.ExcludedCurrencies, want)
		}
	})

	t.Run("account filter", func(t *testing.T) {
		other := uuid.New()
		in := analysisInput(categories, booked("2026-02-05", -100, &food.ID))
		in.Accounts = append(in.Accounts, analysisAccount(other, "EUR"))
		elsewhere := booked("2025-06-01", -900, &cash.ID)
		elsewhere.AccountID = other
		in.Transactions = append(in.Transactions, elsewhere)

		got := spending(t, in, "", "2026-02-28", LedgerSpendingOptions{AccountID: &analysisAccountID})
		if got.From != "2026-02-05" {
			t.Fatalf("from = %s, want the account's first booking", got.From)
		}
		wantAmounts(t, got, map[string][]int64{"Food": {100}})

		unknown := uuid.New()
		if _, err := AnalyzeLedgerSpending(in, from, to, analysisToday, LedgerSpendingOptions{AccountID: &unknown}); !errors.Is(err, storage.ErrNotFound) {
			t.Fatalf("unknown account: err = %v, want ErrNotFound", err)
		}
	})

	t.Run("order", func(t *testing.T) {
		zoo := analysisCategory("zoo", nil)
		apple := analysisCategory("Apple", nil)
		big := analysisCategory("Big", nil)
		got := spending(t, analysisInput([]LedgerCategory{zoo, apple, big},
			booked("2026-01-05", -100, &zoo.ID),
			booked("2026-01-05", -100, nil),
			booked("2026-01-05", -100, &apple.ID),
			booked("2026-02-05", -500, &big.ID),
		), from, to, LedgerSpendingOptions{})
		if want := []string{"Big", "Apple", "zoo", LedgerSeriesUncategorized}; !slices.Equal(seriesOrder(got), want) {
			t.Fatalf("order = %v, want %v", seriesOrder(got), want)
		}
	})
}

func TestAnalyzeLedgerSpending_Subcategories(t *testing.T) {
	const from, to = "2026-01-01", "2026-02-28"
	food := analysisCategory("Food", nil)
	groceries := analysisCategory("Groceries", &food.ID)
	organic := analysisCategory("Organic", &groceries.ID)
	restaurants := analysisCategory("Restaurants", &food.ID)
	quiet := analysisCategory("Quiet", &food.ID)
	car := analysisCategory("Car", nil)
	in := analysisInput([]LedgerCategory{food, groceries, organic, restaurants, quiet, car},
		booked("2026-01-03", -100, &organic.ID),
		booked("2026-01-04", -200, &groceries.ID),
		booked("2026-01-05", -50, &food.ID),
		booked("2026-01-06", 80, &restaurants.ID),
		booked("2026-02-07", -300, &restaurants.ID),
		booked("2026-02-08", -999, &car.ID),
		booked("2026-02-09", -999, nil),
	)

	got := spending(t, in, from, to, LedgerSpendingOptions{ParentCategoryID: &food.ID})
	if got.ParentCategoryID == nil || *got.ParentCategoryID != food.ID {
		t.Fatalf("parentCategoryId = %v", got.ParentCategoryID)
	}
	wantAmounts(t, got, map[string][]int64{
		"Groceries":        {300, 0},
		"Restaurants":      {-80, 300},
		LedgerSeriesDirect: {50, 0},
	})
	if want := []string{"Groceries", "Restaurants", LedgerSeriesDirect}; !slices.Equal(seriesOrder(got), want) {
		t.Fatalf("order = %v, want %v", seriesOrder(got), want)
	}
	for _, s := range got.Series {
		switch s.Kind {
		case LedgerSeriesDirect:
			if s.CategoryID == nil || *s.CategoryID != food.ID || s.Name != "" {
				t.Fatalf("direct series = %+v, want the parent's ID and no name", s)
			}
		case LedgerSeriesCategory:
			if s.CategoryID == nil || (*s.CategoryID != groceries.ID && *s.CategoryID != restaurants.ID) {
				t.Fatalf("series %q has ID %v, want a direct child", s.Name, s.CategoryID)
			}
		default:
			t.Fatalf("unexpected series kind %q", s.Kind)
		}
	}

	topLevel := spending(t, in, from, to, LedgerSpendingOptions{})
	if !slices.Equal(got.TotalsMinor, seriesAmounts(topLevel)["Food"]) || !slices.Equal(got.TotalsMinor, []int64{270, 300}) {
		t.Fatalf("totals = %v, top-level Food = %v, want both 270, 300", got.TotalsMinor, seriesAmounts(topLevel)["Food"])
	}

	unknown := uuid.New()
	if _, err := AnalyzeLedgerSpending(in, from, to, analysisToday, LedgerSpendingOptions{ParentCategoryID: &unknown}); !errors.Is(err, storage.ErrNotFound) {
		t.Fatalf("unknown parent: err = %v, want ErrNotFound", err)
	}
	if _, err := AnalyzeLedgerSpending(in, from, to, analysisToday, LedgerSpendingOptions{ParentCategoryID: &groceries.ID}); !errors.Is(err, ErrLedgerAnalysisInvalid) {
		t.Fatalf("non-top-level parent: err = %v, want ErrLedgerAnalysisInvalid", err)
	}
}

func TestAnalyzeLedgerSpending_Empty(t *testing.T) {
	for name, in := range map[string]LedgerAnalysisInput{
		"no accounts":     {},
		"no transactions": analysisInput([]LedgerCategory{analysisCategory("Food", nil)}),
	} {
		got := spending(t, in, "", "", LedgerSpendingOptions{})
		if got.Series == nil || len(got.Series) != 0 || got.ExcludedCurrencies == nil || len(got.ExcludedCurrencies) != 0 {
			t.Fatalf("%s: series = %#v, excludedCurrencies = %#v, want empty non-nil slices", name, got.Series, got.ExcludedCurrencies)
		}
		if !slices.Equal(got.Months, []string{"2026-11"}) || !slices.Equal(got.TotalsMinor, []int64{0}) {
			t.Fatalf("%s: months = %v, totals = %v, want one zero month", name, got.Months, got.TotalsMinor)
		}
		if got.From != analysisToday || got.To != analysisToday {
			t.Fatalf("%s: range = %s..%s", name, got.From, got.To)
		}
	}
}

func TestAnalyzeLedgerCashFlow(t *testing.T) {
	food := analysisCategory("Food", nil)
	groceries := analysisCategory("Groceries", &food.ID)
	salary := analysisCategory("Salary", nil)
	usdAccount := uuid.New()
	in := analysisInput([]LedgerCategory{food, groceries, salary},
		booked("2026-01-05", -1000, &groceries.ID),
		booked("2026-01-20", 300, &food.ID),
		booked("2026-01-28", 250_000, &salary.ID),
		booked("2026-01-30", -400, nil),
		booked("2026-03-02", 900, nil),
		booked("2026-03-03", -200, &food.ID),
	)
	in.Accounts = append(in.Accounts, analysisAccount(usdAccount, "USD"))
	transfer := booked("2026-01-15", -50_000, nil)
	transfer.SpecialCategory = LedgerSpecialCategoryInternalTransfer
	foreign := booked("2026-03-04", -7777, &food.ID)
	foreign.AccountID, foreign.Currency = usdAccount, "USD"
	in.Transactions = append(in.Transactions, transfer, foreign)

	got, err := AnalyzeLedgerCashFlow(in, "2026-01-01", "2026-03-31", analysisToday)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"2026-01", "2026-02", "2026-03"}; !slices.Equal(got.Months, want) {
		t.Fatalf("months = %v, want %v", got.Months, want)
	}
	if want := []int64{250_000, 0, 900}; !slices.Equal(got.IncomeMinor, want) {
		t.Fatalf("income = %v, want %v", got.IncomeMinor, want)
	}
	if want := []int64{1100, 0, 200}; !slices.Equal(got.ExpensesMinor, want) {
		t.Fatalf("expenses = %v, want %v", got.ExpensesMinor, want)
	}
	if want := []int64{248_900, 0, 700}; !slices.Equal(got.NetMinor, want) {
		t.Fatalf("net = %v, want %v", got.NetMinor, want)
	}
	if !slices.Equal(got.ExcludedCurrencies, []string{"USD"}) {
		t.Fatalf("excludedCurrencies = %v", got.ExcludedCurrencies)
	}

	all, err := AnalyzeLedgerCashFlow(in, "", "", analysisToday)
	if err != nil || all.From != "2026-01-05" || all.To != analysisToday {
		t.Fatalf("all: %s..%s, %v", all.From, all.To, err)
	}

	empty, err := AnalyzeLedgerCashFlow(LedgerAnalysisInput{}, "", "", analysisToday)
	if err != nil {
		t.Fatal(err)
	}
	zero := []int64{0}
	if !slices.Equal(empty.IncomeMinor, zero) || !slices.Equal(empty.ExpensesMinor, zero) || !slices.Equal(empty.NetMinor, zero) ||
		empty.ExcludedCurrencies == nil || len(empty.ExcludedCurrencies) != 0 {
		t.Fatalf("empty = %+v, want one zero month and an empty non-nil currency list", empty)
	}
	if _, err := AnalyzeLedgerCashFlow(in, "2026-02-01", "2026-01-01", analysisToday); !errors.Is(err, ErrLedgerAnalysisInvalid) {
		t.Fatalf("from after to: err = %v", err)
	}
}

// randomAnalysisInput builds a mix of categorized, split, refunded,
// uncategorized, transfer and foreign-currency transactions over the first
// months of 2026; the first category mostly receives income.
func randomAnalysisInput(rng *rand.Rand, count, months int) LedgerAnalysisInput {
	usdAccount := uuid.New()
	in := LedgerAnalysisInput{Accounts: []LedgerAccount{analysisAccount(analysisAccountID, "EUR"), analysisAccount(usdAccount, "USD")}}
	for i := range 5 {
		top := analysisCategory(fmt.Sprintf("Top %d", i), nil)
		child := analysisCategory(fmt.Sprintf("Child %d", i), &top.ID)
		grandchild := analysisCategory(fmt.Sprintf("Grandchild %d", i), &child.ID)
		in.Categories = append(in.Categories, top, child, grandchild)
	}
	category := func() *uuid.UUID {
		i := rng.IntN(len(in.Categories) + 2)
		if i >= len(in.Categories) {
			return nil
		}
		return &in.Categories[i].ID
	}
	for i := range count {
		amount := rng.Int64N(60_000) - 45_000
		txn := booked(fmt.Sprintf("2026-%02d-%02d", 1+rng.IntN(months), 1+rng.IntN(28)), amount, category())
		txn.CounterpartyName = fmt.Sprintf("Shop %d", rng.IntN(40))
		if rng.IntN(8) == 0 {
			txn.CategoryID, txn.AmountMinor = &in.Categories[0].ID, 100_000+rng.Int64N(200_000)
			amount = txn.AmountMinor
		}
		switch roll := rng.IntN(20); {
		case roll == 0:
			txn.CategoryID, txn.SpecialCategory = nil, LedgerSpecialCategoryInternalTransfer
		case roll == 1:
			txn.AccountID, txn.Currency = usdAccount, "USD"
		case roll < 5 && (amount > 3 || amount < -3):
			txn.Splits = []LedgerTransactionSplit{{AmountMinor: amount / 3, CategoryID: category()}, {AmountMinor: amount / 4, CategoryID: category()}}
		}
		if i%7 == 0 {
			txn.ReviewStatus = LedgerTransactionReviewNeedsReview
		}
		in.Transactions = append(in.Transactions, txn)
	}
	return in
}

func TestLedgerAnalysis_SpendingMatchesCashFlow(t *testing.T) {
	const months = 6
	in := randomAnalysisInput(rand.New(rand.NewPCG(70, 3)), 600, months)
	const from, to = "2026-01-01", "2026-06-30"

	spent := spending(t, in, from, to, LedgerSpendingOptions{})
	flow, err := AnalyzeLedgerCashFlow(in, from, to, analysisToday)
	if err != nil {
		t.Fatal(err)
	}
	ranking, err := AnalyzeLedgerRanking(in, from, to, analysisToday)
	if err != nil {
		t.Fatal(err)
	}

	// The expectation is derived without the code under test: walk every
	// part's parent chain and net per top-level category and month.
	parents := map[uuid.UUID]*uuid.UUID{}
	for _, c := range in.Categories {
		parents[c.ID] = c.ParentID
	}
	topOf := func(id *uuid.UUID) uuid.UUID {
		if id == nil {
			return uuid.Nil
		}
		top := *id
		for parents[top] != nil {
			top = *parents[top]
		}
		return top
	}
	type cell struct {
		top   uuid.UUID
		month int
	}
	nets := map[cell]int64{}
	counted := make([]int64, months)
	for _, txn := range in.Transactions {
		if txn.SpecialCategory != "" || txn.Currency != "EUR" {
			continue
		}
		month := int(txn.BookingDate[5]-'0')*10 + int(txn.BookingDate[6]-'0') - 1
		counted[month] += txn.AmountMinor
		rest := txn.AmountMinor
		for _, p := range txn.Splits {
			nets[cell{topOf(p.CategoryID), month}] += p.AmountMinor
			rest -= p.AmountMinor
		}
		nets[cell{topOf(txn.CategoryID), month}] += rest
	}
	wantIncome, wantExpenses := make([]int64, months), make([]int64, months)
	for c, net := range nets {
		if net > 0 {
			wantIncome[c.month] += net
		} else {
			wantExpenses[c.month] -= net
		}
	}

	seriesSum := make([]int64, months)
	for _, s := range spent.Series {
		for m, amount := range s.AmountsMinor {
			if amount < 0 {
				t.Fatalf("series %q has a negative month: %v", s.Name, s.AmountsMinor)
			}
			seriesSum[m] += amount
		}
	}
	for m := range months {
		if seriesSum[m] != flow.ExpensesMinor[m] || spent.TotalsMinor[m] != flow.ExpensesMinor[m] || flow.ExpensesMinor[m] != wantExpenses[m] {
			t.Fatalf("month %d: series sum %d, totals %d, cash-flow expenses %d, want %d", m, seriesSum[m], spent.TotalsMinor[m], flow.ExpensesMinor[m], wantExpenses[m])
		}
		if flow.IncomeMinor[m] != wantIncome[m] {
			t.Fatalf("month %d: income = %d, want %d", m, flow.IncomeMinor[m], wantIncome[m])
		}
		if flow.NetMinor[m] != counted[m] || flow.NetMinor[m] != flow.IncomeMinor[m]-flow.ExpensesMinor[m] {
			t.Fatalf("month %d: net = %d, want the counted amounts %d", m, flow.NetMinor[m], counted[m])
		}
		if wantExpenses[m] == 0 || wantIncome[m] == 0 {
			t.Fatalf("month %d: the generated data has no income or no expenses", m)
		}
	}
	if len(ranking.Categories) != len(spent.Series) {
		t.Fatalf("ranking has %d categories, spending %d series", len(ranking.Categories), len(spent.Series))
	}
	for i, c := range ranking.Categories {
		if s := spent.Series[i]; c.AmountMinor != s.TotalMinor || c.Name != s.Name || c.Kind != s.Kind {
			t.Fatalf("ranking[%d] = %+v, spending series = %+v", i, c, s)
		}
	}
	if !slices.Equal(spent.ExcludedCurrencies, []string{"USD"}) || !slices.Equal(flow.ExcludedCurrencies, []string{"USD"}) || !slices.Equal(ranking.ExcludedCurrencies, []string{"USD"}) {
		t.Fatalf("excluded = %v, %v, %v", spent.ExcludedCurrencies, flow.ExcludedCurrencies, ranking.ExcludedCurrencies)
	}
}

func paidTo(date string, amountMinor int64, counterparty string) LedgerTransaction {
	txn := booked(date, amountMinor, nil)
	txn.CounterpartyName = counterparty
	return txn
}

func TestAnalyzeLedgerRanking(t *testing.T) {
	const from, to = "2026-01-01", "2026-03-31"
	rank := func(t *testing.T, in LedgerAnalysisInput) LedgerRankingAnalysis {
		t.Helper()
		got, err := AnalyzeLedgerRanking(in, from, to, analysisToday)
		if err != nil {
			t.Fatalf("AnalyzeLedgerRanking: %v", err)
		}
		return got
	}

	t.Run("categories equal the spending totals", func(t *testing.T) {
		food := analysisCategory("Food", nil)
		groceries := analysisCategory("Groceries", &food.ID)
		car := analysisCategory("Car", nil)
		salary := analysisCategory("Salary", nil)
		in := analysisInput([]LedgerCategory{food, groceries, car, salary},
			booked("2026-01-05", -1000, &groceries.ID),
			booked("2026-02-05", -500, &food.ID),
			booked("2026-02-06", 200, &food.ID),
			booked("2026-02-07", -5000, &car.ID),
			booked("2026-02-08", -90, nil),
			booked("2026-02-09", 300_000, &salary.ID),
			booked("2026-04-01", -9999, &food.ID),
		)
		got := rank(t, in)
		spent := spending(t, in, from, to, LedgerSpendingOptions{})
		want := []LedgerRankedCategory{
			{Kind: LedgerSeriesCategory, CategoryID: &car.ID, Name: "Car", AmountMinor: 5000},
			{Kind: LedgerSeriesCategory, CategoryID: &food.ID, Name: "Food", AmountMinor: 1300},
			{Kind: LedgerSeriesUncategorized, AmountMinor: 90},
		}
		if len(got.Categories) != len(want) || len(spent.Series) != len(want) {
			t.Fatalf("categories = %+v", got.Categories)
		}
		for i, w := range want {
			c := got.Categories[i]
			sameID := (c.CategoryID == nil) == (w.CategoryID == nil) && (c.CategoryID == nil || *c.CategoryID == *w.CategoryID)
			if c.Kind != w.Kind || c.Name != w.Name || c.AmountMinor != w.AmountMinor || !sameID {
				t.Fatalf("categories[%d] = %+v, want %+v", i, c, w)
			}
			if c.AmountMinor != spent.Series[i].TotalMinor {
				t.Fatalf("categories[%d] = %d, spending total = %d", i, c.AmountMinor, spent.Series[i].TotalMinor)
			}
		}
		if got.From != from || got.To != to {
			t.Fatalf("range = %s..%s", got.From, got.To)
		}
	})

	t.Run("counterparties", func(t *testing.T) {
		food := analysisCategory("Food", nil)
		cash := analysisCategory("Cash", nil)
		split := paidTo("2026-01-10", -1000, "Supermarkt  Nord")
		split.CategoryID = &food.ID
		split.Splits = []LedgerTransactionSplit{{AmountMinor: -400, CategoryID: &cash.ID}}
		transfer := paidTo("2026-01-11", -80_000, "Own savings")
		transfer.SpecialCategory = LedgerSpecialCategoryInternalTransfer
		got := rank(t, analysisInput([]LedgerCategory{food, cash},
			split,
			paidTo("2026-02-10", -500, " SUPERMARKT nord "),
			paidTo("2026-02-10", -100, "supermarkt Nord"),
			paidTo("2026-02-20", 250, "Supermarkt Nord"),
			paidTo("2026-01-15", -700, ""),
			paidTo("2026-01-16", -200, "   "),
			paidTo("2026-01-17", -3000, "Landlord"),
			paidTo("2026-01-18", 250_000, "Employer"),
			paidTo("2026-01-19", -300, "Refunder"),
			paidTo("2026-01-20", 300, "Refunder"),
			paidTo("2026-01-21", -900, "Beta"),
			paidTo("2026-01-21", -900, "alpha"),
			paidTo("2026-04-01", -9999, "Later"),
			transfer,
		))
		want := []LedgerRankedCounterparty{
			{Name: "Landlord", AmountMinor: 3000},
			{Name: "Supermarkt Nord", AmountMinor: 1350},
			{Name: "", Unknown: true, AmountMinor: 900},
			{Name: "alpha", AmountMinor: 900},
			{Name: "Beta", AmountMinor: 900},
		}
		if !slices.Equal(got.Counterparties, want) {
			t.Fatalf("counterparties = %+v, want %+v", got.Counterparties, want)
		}
	})

	t.Run("spelling of the newest transaction", func(t *testing.T) {
		got := rank(t, analysisInput(nil,
			paidTo("2026-01-10", -100, "old name gmbh"),
			paidTo("2026-03-10", -100, "Old Name GmbH"),
			paidTo("2026-03-10", -100, "OLD NAME GMBH"),
			paidTo("2026-02-10", -100, "old Name gmbh"),
		))
		if len(got.Counterparties) != 1 || got.Counterparties[0] != (LedgerRankedCounterparty{Name: "OLD NAME GMBH", AmountMinor: 400}) {
			t.Fatalf("counterparties = %+v", got.Counterparties)
		}
	})

	t.Run("capped at ten", func(t *testing.T) {
		var categories []LedgerCategory
		var txns []LedgerTransaction
		for i := range 12 {
			c := analysisCategory(fmt.Sprintf("Category %02d", i), nil)
			categories = append(categories, c)
			txn := paidTo("2026-02-01", -int64(100+i), fmt.Sprintf("Shop %02d", i))
			txn.CategoryID = &c.ID
			txns = append(txns, txn)
		}
		got := rank(t, analysisInput(categories, txns...))
		if len(got.Categories) != ledgerRankingLimit || len(got.Counterparties) != ledgerRankingLimit {
			t.Fatalf("got %d categories and %d counterparties, want %d each", len(got.Categories), len(got.Counterparties), ledgerRankingLimit)
		}
		if got.Categories[0].Name != "Category 11" || got.Categories[9].Name != "Category 02" {
			t.Fatalf("categories run from %q to %q", got.Categories[0].Name, got.Categories[9].Name)
		}
		if got.Counterparties[0].Name != "Shop 11" || got.Counterparties[9].Name != "Shop 02" {
			t.Fatalf("counterparties run from %q to %q", got.Counterparties[0].Name, got.Counterparties[9].Name)
		}
	})

	t.Run("empty", func(t *testing.T) {
		got := rank(t, LedgerAnalysisInput{})
		if got.Categories == nil || got.Counterparties == nil || got.ExcludedCurrencies == nil ||
			len(got.Categories)+len(got.Counterparties)+len(got.ExcludedCurrencies) != 0 {
			t.Fatalf("empty = %#v, want empty non-nil slices", got)
		}
	})
}

func BenchmarkAnalyzeLedgerSpending(b *testing.B) {
	in := randomAnalysisInput(rand.New(rand.NewPCG(70, 5)), 5000, 12)
	for b.Loop() {
		if _, err := AnalyzeLedgerSpending(in, "", "", analysisToday, LedgerSpendingOptions{}); err != nil {
			b.Fatal(err)
		}
	}
}
