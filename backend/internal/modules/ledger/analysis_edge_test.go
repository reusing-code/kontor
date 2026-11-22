package ledger

import (
	"math/rand/v2"
	"reflect"
	"slices"
	"testing"

	"github.com/google/uuid"
)

// legacyBookingDates are values no importer writes; a transaction that
// carries one is skipped instead of breaking the analysis.
var legacyBookingDates = []string{"", "2026-1-5", "15.03.2026", "2026-13-01", "2026-00-10", "2026-01-00", "abcd-ef-gh", "2026-01-15T10:00:00Z", " 2026-01-15"}

func legacyBookings(categoryID *uuid.UUID) []LedgerTransaction {
	txns := make([]LedgerTransaction, len(legacyBookingDates))
	for i, date := range legacyBookingDates {
		txns[i] = booked(date, -999_999, categoryID)
		txns[i].CounterpartyName = "Legacy"
	}
	return txns
}

// shuffledAnalysisInput returns in with its slices in another order.
func shuffledAnalysisInput(rng *rand.Rand, in LedgerAnalysisInput) LedgerAnalysisInput {
	out := LedgerAnalysisInput{
		Accounts:     slices.Clone(in.Accounts),
		Categories:   slices.Clone(in.Categories),
		Transactions: slices.Clone(in.Transactions),
	}
	rng.Shuffle(len(out.Accounts), func(i, j int) { out.Accounts[i], out.Accounts[j] = out.Accounts[j], out.Accounts[i] })
	rng.Shuffle(len(out.Categories), func(i, j int) { out.Categories[i], out.Categories[j] = out.Categories[j], out.Categories[i] })
	rng.Shuffle(len(out.Transactions), func(i, j int) {
		out.Transactions[i], out.Transactions[j] = out.Transactions[j], out.Transactions[i]
	})
	return out
}

// tiedAnalysisInput has categories with equal totals and equal names, so the
// order of the result depends on every tie-break.
func tiedAnalysisInput() LedgerAnalysisInput {
	parent := analysisCategory("Food", nil)
	categories := []LedgerCategory{
		parent,
		analysisCategory("Food", nil),
		analysisCategory("food", nil),
		analysisCategory("Car", nil),
		analysisCategory("Groceries", &parent.ID),
		analysisCategory("Groceries", &parent.ID),
	}
	var txns []LedgerTransaction
	for i, c := range categories {
		for _, date := range []string{"2026-01-10", "2026-02-10"} {
			txn := booked(date, -500, &c.ID)
			txn.CounterpartyName = []string{"Laden", "LADEN", "laden ", "Markt", "markt", "Markt"}[i]
			txns = append(txns, txn)
		}
	}
	txns = append(txns, booked("2026-01-10", -1000, nil), booked("2026-02-10", -1000, nil))
	return analysisInput(categories, txns...)
}

func TestAnalyzeLedgerSpending_SkipsMalformedBookingDates(t *testing.T) {
	food := analysisCategory("Food", nil)
	in := analysisInput([]LedgerCategory{food}, append(legacyBookings(&food.ID), booked("2026-01-20", -100, &food.ID))...)

	got := spending(t, in, "", "", LedgerSpendingOptions{})
	if got.From != "2026-01-20" {
		t.Fatalf("from = %q, want the first well-formed booking date", got.From)
	}
	wantAmounts(t, got, map[string][]int64{"Food": {100, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}})

	// A date of the right shape that the calendar does not have counts in its month.
	in.Transactions = append(in.Transactions, booked("2026-02-31", -7, &food.ID))
	got = spending(t, in, "2026-02-01", "2026-03-31", LedgerSpendingOptions{})
	wantAmounts(t, got, map[string][]int64{"Food": {7, 0}})
}

func TestAnalyzeLedgerSpending_Boundaries(t *testing.T) {
	food := analysisCategory("Food", nil)
	in := analysisInput([]LedgerCategory{food},
		booked("2024-02-28", -1, &food.ID),
		booked("2024-02-29", -20, &food.ID),
		booked("2024-03-01", -300, &food.ID),
		booked("2023-12-31", -4000, &food.ID),
		booked("2024-01-01", -50000, &food.ID),
	)

	t.Run("a single leap day", func(t *testing.T) {
		got := spending(t, in, "2024-02-29", "2024-02-29", LedgerSpendingOptions{})
		if !slices.Equal(got.Months, []string{"2024-02"}) {
			t.Fatalf("months = %v", got.Months)
		}
		wantAmounts(t, got, map[string][]int64{"Food": {20}})
	})

	t.Run("the year change splits the months", func(t *testing.T) {
		got := spending(t, in, "2023-12-31", "2024-01-01", LedgerSpendingOptions{})
		if !slices.Equal(got.Months, []string{"2023-12", "2024-01"}) {
			t.Fatalf("months = %v", got.Months)
		}
		wantAmounts(t, got, map[string][]int64{"Food": {4000, 50000}})
	})

	t.Run("a range without any booking keeps its months", func(t *testing.T) {
		got := spending(t, in, "2025-01-15", "2025-03-15", LedgerSpendingOptions{})
		if len(got.Series) != 0 || got.Series == nil || !slices.Equal(got.TotalsMinor, []int64{0, 0, 0}) {
			t.Fatalf("series = %v, totals = %v, want none and three zero months", got.Series, got.TotalsMinor)
		}
	})

	t.Run("exactly 1200 months", func(t *testing.T) {
		got := spending(t, in, "1925-01-01", "2024-12-31", LedgerSpendingOptions{})
		if len(got.Months) != maxLedgerAnalysisMonths || len(got.Series[0].AmountsMinor) != maxLedgerAnalysisMonths || got.Series[0].TotalMinor != 54321 {
			t.Fatalf("%d months, series %d values, total %d", len(got.Months), len(got.Series[0].AmountsMinor), got.Series[0].TotalMinor)
		}
	})
}

// The result must not depend on the order in which the store returns the
// records (it iterates maps internally).
func TestAnalyzeLedgerSpending_IsIndependentOfTheInputOrder(t *testing.T) {
	in := tiedAnalysisInput()
	parent := in.Categories[0].ID
	rng := rand.New(rand.NewPCG(70, 1))
	for _, opts := range []LedgerSpendingOptions{{}, {ParentCategoryID: &parent}} {
		want := spending(t, in, "2026-01-01", "2026-02-28", opts)
		if len(want.Series) < 3 {
			t.Fatalf("series = %+v, want several tied ones", want.Series)
		}
		for range 50 {
			if got := spending(t, shuffledAnalysisInput(rng, in), "2026-01-01", "2026-02-28", opts); !reflect.DeepEqual(got, want) {
				t.Fatalf("result changed with the input order:\n got %+v\nwant %+v", got, want)
			}
		}
	}
}
