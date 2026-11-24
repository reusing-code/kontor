package ledger

import (
	"math/rand/v2"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func breakdown(t *testing.T, in LedgerAnalysisInput, from, to string) LedgerCategoryBreakdown {
	t.Helper()
	got, err := AnalyzeLedgerCategoryBreakdown(in, from, to, analysisToday)
	if err != nil {
		t.Fatalf("AnalyzeLedgerCategoryBreakdown: %v", err)
	}
	return got
}

// breakdownShape renders the tree as "Name=amount[child=amount,…]" with the
// kind for the slices without a name of their own, in response order.
func breakdownShape(b LedgerCategoryBreakdown) string {
	parts := make([]string, 0, len(b.Categories))
	for _, c := range b.Categories {
		name := c.Name
		if c.Kind != LedgerSeriesCategory {
			name = c.Kind
		}
		children := make([]string, 0, len(c.Children))
		for _, child := range c.Children {
			childName := child.Name
			if child.Kind != LedgerSeriesCategory {
				childName = child.Kind
			}
			children = append(children, childName+"="+strconv.FormatInt(child.AmountMinor, 10))
		}
		parts = append(parts, name+"="+strconv.FormatInt(c.AmountMinor, 10)+"["+strings.Join(children, ",")+"]")
	}
	return strings.Join(parts, " ")
}

func wantBreakdown(t *testing.T, got LedgerCategoryBreakdown, want string) {
	t.Helper()
	if shape := breakdownShape(got); shape != want {
		t.Fatalf("breakdown = %s\nwant        %s", shape, want)
	}
	var total int64
	for _, c := range got.Categories {
		var sum int64
		for _, child := range c.Children {
			sum += child.AmountMinor
		}
		if c.Kind == LedgerSeriesCategory && sum != c.AmountMinor {
			t.Fatalf("%s: children sum to %d, parent is %d", c.Name, sum, c.AmountMinor)
		}
		total += c.AmountMinor
	}
	if total != got.TotalMinor {
		t.Fatalf("totalMinor = %d, categories sum to %d", got.TotalMinor, total)
	}
}

func TestAnalyzeLedgerCategoryBreakdown(t *testing.T) {
	const from, to = "2026-01-01", "2026-02-28"
	food := analysisCategory("Food", nil)
	groceries := analysisCategory("Groceries", &food.ID)
	organic := analysisCategory("Organic", &groceries.ID)
	restaurant := analysisCategory("Restaurant", &food.ID)
	cash := analysisCategory("Cash", nil)
	salary := analysisCategory("Salary", nil)
	categories := []LedgerCategory{food, groceries, organic, restaurant, cash, salary}

	t.Run("roll-up and direct amounts", func(t *testing.T) {
		got := breakdown(t, analysisInput(categories,
			booked("2026-01-05", -1000, &groceries.ID),
			booked("2026-01-06", -500, &organic.ID),
			booked("2026-02-07", -700, &restaurant.ID),
			booked("2026-02-08", -200, &food.ID),
			booked("2026-02-09", -300, &cash.ID),
		), from, to)
		wantBreakdown(t, got, "Food=2400[Groceries=1500,Restaurant=700,direct=200] Cash=300[direct=300]")
		if got.From != from || got.To != to || got.TotalMinor != 2700 || got.DroppedInflows != 0 {
			t.Fatalf("got %+v", got)
		}
		direct := got.Categories[0].Children[2]
		if direct.CategoryID != food.ID || direct.Name != "" {
			t.Fatalf("direct child = %+v, want the parent's ID and no name", direct)
		}
		if got.Categories[0].Children[0].CategoryID != groceries.ID {
			t.Fatalf("child ID = %s, want the direct child, not the booked descendant", got.Categories[0].Children[0].CategoryID)
		}
	})

	t.Run("split parts and the remainder", func(t *testing.T) {
		split := booked("2026-01-10", -1000, &groceries.ID)
		split.Splits = []LedgerTransactionSplit{{AmountMinor: -300, CategoryID: &cash.ID}, {AmountMinor: -100, CategoryID: &restaurant.ID}}
		got := breakdown(t, analysisInput(categories, split), from, to)
		wantBreakdown(t, got, "Food=700[Groceries=600,Restaurant=100] Cash=300[direct=300]")
	})

	t.Run("netting over the whole range", func(t *testing.T) {
		got := breakdown(t, analysisInput(categories,
			booked("2026-01-05", -1000, &groceries.ID),
			booked("2026-02-20", 400, &groceries.ID), // refund in another month lowers the range net
			booked("2026-01-07", -100, &restaurant.ID),
			booked("2026-02-07", 300, &restaurant.ID), // net inflow: dropped, parent not lowered
			booked("2026-01-28", 300_000, &salary.ID), // net inflow top level without children
			booked("2026-01-09", -50, &cash.ID),
			booked("2026-02-09", 50, &cash.ID), // nets to zero: dropped, not counted as inflow
		), from, to)
		wantBreakdown(t, got, "Food=600[Groceries=600]")
		if got.DroppedInflows != 2 {
			t.Fatalf("droppedInflows = %d, want 2 (Restaurant, Salary)", got.DroppedInflows)
		}
	})

	t.Run("uncategorized and unknown categories", func(t *testing.T) {
		split := booked("2026-01-10", -1000, &cash.ID)
		split.Splits = []LedgerTransactionSplit{{AmountMinor: -250}}
		deleted := uuid.New()
		got := breakdown(t, analysisInput(categories, split, booked("2026-01-11", -100, nil), booked("2026-02-01", -40, &deleted)), from, to)
		wantBreakdown(t, got, "Cash=750[direct=750] uncategorized=390[]")
		u := got.Categories[1]
		if u.CategoryID != nil || u.Name != "" || u.Children == nil {
			t.Fatalf("uncategorized = %+v, want no ID, no name, empty children", u)
		}

		inflow := breakdown(t, analysisInput(categories, booked("2026-01-11", 100, nil), booked("2026-01-12", -10, &cash.ID)), from, to)
		wantBreakdown(t, inflow, "Cash=10[direct=10]")
		if inflow.DroppedInflows != 1 {
			t.Fatalf("droppedInflows = %d, want the uncategorized inflow", inflow.DroppedInflows)
		}
	})

	t.Run("linked transfers and other currencies are left out", func(t *testing.T) {
		special := booked("2026-01-05", -500, nil)
		special.SpecialCategory = LedgerSpecialCategoryInternalTransfer
		pair := uuid.New()
		paired := booked("2026-01-06", -700, &groceries.ID)
		paired.TransferPairTransactionID = &pair
		usdAccount := uuid.New()
		onUSD := booked("2026-01-07", -999, &groceries.ID)
		onUSD.AccountID = usdAccount
		foreign := booked("2026-01-08", -999, &cash.ID)
		foreign.Currency = "chf"
		outOfRange := booked("2025-12-31", -999, &cash.ID)
		in := analysisInput(categories, special, paired, onUSD, foreign, outOfRange, booked("2026-01-09", -100, &groceries.ID))
		in.Accounts = append(in.Accounts, analysisAccount(usdAccount, "USD"))

		got := breakdown(t, in, from, to)
		wantBreakdown(t, got, "Food=100[Groceries=100]")
		if want := []string{"CHF", "USD"}; !slices.Equal(got.ExcludedCurrencies, want) {
			t.Fatalf("excludedCurrencies = %v, want %v", got.ExcludedCurrencies, want)
		}
	})

	t.Run("order", func(t *testing.T) {
		zoo := analysisCategory("zoo", nil)
		apple := analysisCategory("Apple", nil)
		big := analysisCategory("Big", nil)
		bChild := analysisCategory("b", &big.ID)
		aChild := analysisCategory("A", &big.ID)
		got := breakdown(t, analysisInput([]LedgerCategory{zoo, apple, big, bChild, aChild},
			booked("2026-01-05", -100, &zoo.ID),
			booked("2026-01-05", -100, nil),
			booked("2026-01-05", -100, &apple.ID),
			booked("2026-02-05", -200, &bChild.ID),
			booked("2026-02-05", -200, &aChild.ID),
			booked("2026-02-05", -200, &big.ID),
		), from, to)
		wantBreakdown(t, got, "Big=600[A=200,b=200,direct=200] Apple=100[direct=100] zoo=100[direct=100] uncategorized=100[]")
	})

	t.Run("range from the data and empty ledger", func(t *testing.T) {
		got := breakdown(t, analysisInput(categories, booked("2026-03-04", -10, &cash.ID)), "", "")
		if got.From != "2026-03-04" || got.To != analysisToday {
			t.Fatalf("range = %s..%s", got.From, got.To)
		}
		empty := breakdown(t, analysisInput(nil), "", "")
		if empty.Categories == nil || len(empty.Categories) != 0 || empty.ExcludedCurrencies == nil || empty.TotalMinor != 0 {
			t.Fatalf("empty = %+v, want non-nil empty arrays", empty)
		}
		if _, err := AnalyzeLedgerCategoryBreakdown(analysisInput(nil), "2026-02-01", "2026-01-01", analysisToday); err == nil {
			t.Fatal("from > to: want an error")
		}
	})
}

// Without inflows the breakdown's top levels are the range totals of the
// spending view's top level, and each one equals the sum of its subcategory
// view.
func TestLedgerAnalysis_BreakdownMatchesSpending(t *testing.T) {
	rng := rand.New(rand.NewPCG(7, 82))
	in := randomAnalysisInput(rng, 400, 6)
	// Spending only, so no child has a net inflow and both views agree.
	for i := range in.Transactions {
		txn := &in.Transactions[i]
		txn.AmountMinor = -abs(txn.AmountMinor)
		for j := range txn.Splits {
			txn.Splits[j].AmountMinor = -abs(txn.Splits[j].AmountMinor)
		}
	}
	got := breakdown(t, in, "", "")
	top := spending(t, in, got.From, got.To, LedgerSpendingOptions{})
	want := map[string]int64{}
	for _, s := range top.Series {
		want[seriesID(s)] = s.TotalMinor
	}
	for _, c := range got.Categories {
		if want[breakdownID(c)] != c.AmountMinor {
			t.Fatalf("%s: breakdown %d, spending %d", c.Name, c.AmountMinor, want[breakdownID(c)])
		}
		if c.CategoryID == nil {
			continue
		}
		sub := spending(t, in, got.From, got.To, LedgerSpendingOptions{ParentCategoryID: c.CategoryID})
		var subTotal int64
		for _, s := range sub.Series {
			subTotal += s.TotalMinor
		}
		if subTotal != c.AmountMinor {
			t.Fatalf("%s: subcategory view sums to %d, breakdown %d", c.Name, subTotal, c.AmountMinor)
		}
	}
	if len(got.Categories) != len(top.Series) {
		t.Fatalf("%d categories, %d spending series", len(got.Categories), len(top.Series))
	}
}

func abs(n int64) int64 {
	if n < 0 {
		return -n
	}
	return n
}
