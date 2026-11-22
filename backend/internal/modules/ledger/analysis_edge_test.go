package ledger

import (
	"errors"
	"fmt"
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

func TestAnalyzeLedgerCashFlowAndRanking_SkipMalformedBookingDates(t *testing.T) {
	food := analysisCategory("Food", nil)
	legacy := legacyBookings(&food.ID)
	for i := range legacy {
		if i%2 == 0 {
			legacy[i].AmountMinor = 888_888
		}
	}
	valid := booked("2026-10-20", -100, &food.ID)
	valid.CounterpartyName = "Laden"
	in := analysisInput([]LedgerCategory{food}, append(legacy, valid)...)

	flow, err := AnalyzeLedgerCashFlow(in, "", "", analysisToday)
	if err != nil {
		t.Fatalf("AnalyzeLedgerCashFlow: %v", err)
	}
	if flow.From != "2026-10-20" || !slices.Equal(flow.IncomeMinor, []int64{0, 0}) || !slices.Equal(flow.ExpensesMinor, []int64{100, 0}) || !slices.Equal(flow.NetMinor, []int64{-100, 0}) {
		t.Fatalf("cash flow = %+v, want only the well-formed booking", flow)
	}

	ranking, err := AnalyzeLedgerRanking(in, "", "", analysisToday)
	if err != nil {
		t.Fatalf("AnalyzeLedgerRanking: %v", err)
	}
	if len(ranking.Categories) != 1 || ranking.Categories[0].AmountMinor != 100 {
		t.Fatalf("categories = %+v", ranking.Categories)
	}
	if want := []LedgerRankedCounterparty{{Name: "Laden", AmountMinor: 100}}; !slices.Equal(ranking.Counterparties, want) {
		t.Fatalf("counterparties = %+v, want %+v", ranking.Counterparties, want)
	}
}

func TestAnalyzeLedgerCashFlowAndRanking_IsIndependentOfTheInputOrder(t *testing.T) {
	in := tiedAnalysisInput()
	rng := rand.New(rand.NewPCG(70, 2))
	analyze := func(in LedgerAnalysisInput) (LedgerCashFlowAnalysis, LedgerRankingAnalysis) {
		flow, err := AnalyzeLedgerCashFlow(in, "2026-01-01", "2026-02-28", analysisToday)
		if err != nil {
			t.Fatalf("AnalyzeLedgerCashFlow: %v", err)
		}
		ranking, err := AnalyzeLedgerRanking(in, "2026-01-01", "2026-02-28", analysisToday)
		if err != nil {
			t.Fatalf("AnalyzeLedgerRanking: %v", err)
		}
		return flow, ranking
	}
	wantFlow, wantRanking := analyze(in)
	// "Laden", "LADEN" and "laden " are one counterparty, shown in the smallest spelling of its newest day.
	if want := []LedgerRankedCounterparty{{Name: "LADEN", AmountMinor: 3000}, {Name: "Markt", AmountMinor: 3000}, {Name: "", Unknown: true, AmountMinor: 2000}}; !slices.Equal(wantRanking.Counterparties, want) {
		t.Fatalf("counterparties = %+v, want %+v", wantRanking.Counterparties, want)
	}
	for range 50 {
		flow, ranking := analyze(shuffledAnalysisInput(rng, in))
		if !reflect.DeepEqual(flow, wantFlow) || !reflect.DeepEqual(ranking, wantRanking) {
			t.Fatalf("result changed with the input order:\n got %+v %+v\nwant %+v %+v", flow, ranking, wantFlow, wantRanking)
		}
	}
}

func TestAnalyzeLedgerRanking_CounterpartyNameEdges(t *testing.T) {
	in := analysisInput(nil,
		paidTo("2026-01-05", -100, "\t  "),
		paidTo("2026-01-06", -50, ""),
		paidTo("2026-01-07", -300, "ÄRZTE am Markt"),
		paidTo("2026-01-08", -200, "ärzte  AM\tmarkt"),
		paidTo("2026-01-09", -400, "Zero"),
		paidTo("2026-01-10", 400, "zero"),
		paidTo("2026-01-11", -1, "<script>alert(1)</script>"),
	)
	got, err := AnalyzeLedgerRanking(in, "2026-01-01", "2026-01-31", analysisToday)
	if err != nil {
		t.Fatalf("AnalyzeLedgerRanking: %v", err)
	}
	want := []LedgerRankedCounterparty{
		{Name: "ärzte AM markt", AmountMinor: 500},
		{Name: "", Unknown: true, AmountMinor: 150},
		{Name: "<script>alert(1)</script>", AmountMinor: 1},
	}
	if !slices.Equal(got.Counterparties, want) {
		t.Fatalf("counterparties = %+v, want %+v", got.Counterparties, want)
	}
}

// With an omitted end the range is checked by the analysis itself.
func TestAnalyzeLedgerCashFlowAndRanking_RangeFromTheData(t *testing.T) {
	in := analysisInput(nil, booked("1900-05-01", -100, nil), booked("2026-03-01", -100, nil))
	for name, analyze := range map[string]func(from, to string) error{
		"cash flow": func(from, to string) error { _, err := AnalyzeLedgerCashFlow(in, from, to, analysisToday); return err },
		"ranking":   func(from, to string) error { _, err := AnalyzeLedgerRanking(in, from, to, analysisToday); return err },
	} {
		for _, ends := range [][2]string{{"2026-11-23", ""}, {"", ""}, {"", "2000-06-30"}, {"bad", ""}, {"", "2026-02-30x"}} {
			if err := analyze(ends[0], ends[1]); !errors.Is(err, ErrLedgerAnalysisInvalid) {
				t.Fatalf("%s %q..%q: err = %v, want ErrLedgerAnalysisInvalid", name, ends[0], ends[1], err)
			}
		}
		if err := analyze("", "1999-12-31"); err != nil {
			t.Fatalf("%s: 1200 months up to the given end: %v", name, err)
		}
	}
}

// The net worth places bookings by the string order of their date, exactly as
// the account balance does, so bookings with a malformed date cannot break it:
// the last point stays the current balance of the account list, and the
// start of "all" is the first well-formed date.
func TestAnalyzeLedgerNetWorth_MalformedBookingDates(t *testing.T) {
	account := analysisAccount(uuid.New(), "EUR")
	txns := []LedgerTransaction{bookedOn(account.ID, "2026-08-10", -1000), bookedOn(account.ID, "2026-10-05", -200)}
	for _, date := range legacyBookingDates {
		txns = append(txns, bookedOn(account.ID, date, -999_999))
	}
	in := LedgerNetWorthInput{
		Accounts:     []LedgerAccount{account},
		Transactions: txns,
		Snapshots:    []LedgerBalanceSnapshot{snapshotAt(account.ID, "2026-09-15", 50_000)},
	}

	all := netWorth(t, in, "", "")
	if all.From != "2026-08-10" || len(all.Dates) != 4 {
		t.Fatalf("from = %q with dates %v, want the first well-formed booking", all.From, all.Dates)
	}
	current := computeLedgerAccountBalance(in.Snapshots, txns).CurrentMinor
	if last := all.TotalMinor[len(all.TotalMinor)-1]; last != current {
		t.Fatalf("last point = %d, currentMinor = %d", last, current)
	}
	// Up to the snapshot only the well-formed bookings lie between the dates.
	if !slices.Equal(all.TotalMinor[:2], []int64{50_000, 50_000}) {
		t.Fatalf("total = %v, want 50000 at the end of August and September", all.TotalMinor)
	}
}

func TestAnalyzeLedgerNetWorth_Edges(t *testing.T) {
	account := analysisAccount(uuid.New(), "EUR")
	other := analysisAccount(uuid.New(), "EUR")
	other.Name = "Other"

	t.Run("a snapshot dated after today", func(t *testing.T) {
		in := LedgerNetWorthInput{
			Accounts:     []LedgerAccount{account},
			Transactions: []LedgerTransaction{bookedOn(account.ID, "2026-11-01", -100), bookedOn(account.ID, "2026-12-15", -50)},
			Snapshots:    []LedgerBalanceSnapshot{snapshotAt(account.ID, "2026-12-31", 1_000)},
		}
		got := netWorth(t, in, "", "")
		if !slices.Equal(got.Dates, []string{analysisToday}) || got.From != "2026-11-01" {
			t.Fatalf("dates = %v from %s", got.Dates, got.From)
		}
		if current := computeLedgerAccountBalance(in.Snapshots, in.Transactions).CurrentMinor; got.TotalMinor[0] != current {
			t.Fatalf("last point = %d, currentMinor = %d", got.TotalMinor[0], current)
		}
		// Before today the snapshot is walked back through the bookings up to it.
		if past := netWorth(t, in, "2026-10-01", "2026-10-31"); !slices.Equal(past.TotalMinor, []int64{1_150}) {
			t.Fatalf("total on 2026-10-31 = %v, want 1150", past.TotalMinor)
		}
	})

	t.Run("two accounts with a snapshot on the same day and negative balances", func(t *testing.T) {
		in := LedgerNetWorthInput{
			Accounts: []LedgerAccount{account, other},
			Snapshots: []LedgerBalanceSnapshot{
				snapshotAt(account.ID, "2026-09-30", -70_000),
				snapshotAt(other.ID, "2026-09-30", 20_000),
			},
			Transactions: []LedgerTransaction{bookedOn(account.ID, "2026-09-30", -1), bookedOn(other.ID, "2026-10-01", -30_000)},
		}
		got := netWorth(t, in, "2026-09-30", "2026-10-31")
		if !slices.Equal(got.Dates, []string{"2026-09-30", "2026-10-31"}) || !slices.Equal(got.TotalMinor, []int64{-50_000, -80_000}) {
			t.Fatalf("dates = %v, total = %v", got.Dates, got.TotalMinor)
		}
		if !slices.Equal(got.Accounts[1].ValuesMinor, []int64{20_000, -10_000}) {
			t.Fatalf("Other = %v", got.Accounts[1].ValuesMinor)
		}
	})

	t.Run("a transaction of an unknown account counts nowhere", func(t *testing.T) {
		in := LedgerNetWorthInput{
			Accounts:     []LedgerAccount{account},
			Snapshots:    []LedgerBalanceSnapshot{snapshotAt(account.ID, "2026-09-30", 500), snapshotAt(uuid.New(), "2020-01-01", 999_999)},
			Transactions: []LedgerTransaction{bookedOn(uuid.New(), "2019-01-01", -999_999)},
		}
		got := netWorth(t, in, "", "")
		if got.From != "2026-09-30" || len(got.Accounts) != 1 || got.TotalMinor[len(got.TotalMinor)-1] != 500 || len(got.ExcludedAccounts) != 0 {
			t.Fatalf("got %+v", got)
		}
	})

	t.Run("the ends of the range", func(t *testing.T) {
		in := LedgerNetWorthInput{Accounts: []LedgerAccount{account}, Snapshots: []LedgerBalanceSnapshot{snapshotAt(account.ID, "1900-01-01", 5)}}
		for _, ends := range [][2]string{{"", ""}, {"2026-11-23", ""}, {"", "2026-02-30"}, {"x", "y"}} {
			if _, err := AnalyzeLedgerNetWorth(in, ends[0], ends[1], analysisToday); !errors.Is(err, ErrLedgerAnalysisInvalid) {
				t.Fatalf("%q..%q: err = %v, want ErrLedgerAnalysisInvalid", ends[0], ends[1], err)
			}
		}
		got := netWorth(t, in, "", "1999-12-31")
		if len(got.Dates) != maxLedgerAnalysisMonths || got.Dates[0] != "1900-01-31" || got.Dates[len(got.Dates)-1] != "1999-12-31" {
			t.Fatalf("%d dates from %s", len(got.Dates), got.Dates[0])
		}
	})

	t.Run("independent of the order of bookings and snapshots", func(t *testing.T) {
		in := LedgerNetWorthInput{Accounts: []LedgerAccount{account, other}}
		for day := 1; day <= 28; day++ {
			for month := 6; month <= 11; month++ {
				date := fmt.Sprintf("2026-%02d-%02d", month, day)
				in.Transactions = append(in.Transactions, bookedOn(account.ID, date, int64(day*month)), bookedOn(other.ID, date, int64(-day)))
				if day%9 == 0 {
					in.Snapshots = append(in.Snapshots, snapshotAt(account.ID, date, int64(1000*day)), snapshotAt(other.ID, date, int64(month)))
				}
			}
		}
		want := netWorth(t, in, "", "")
		rng := rand.New(rand.NewPCG(70, 3))
		for range 30 {
			shuffled := LedgerNetWorthInput{Accounts: in.Accounts, Transactions: slices.Clone(in.Transactions), Snapshots: slices.Clone(in.Snapshots)}
			rng.Shuffle(len(shuffled.Transactions), func(i, j int) {
				shuffled.Transactions[i], shuffled.Transactions[j] = shuffled.Transactions[j], shuffled.Transactions[i]
			})
			rng.Shuffle(len(shuffled.Snapshots), func(i, j int) {
				shuffled.Snapshots[i], shuffled.Snapshots[j] = shuffled.Snapshots[j], shuffled.Snapshots[i]
			})
			if got := netWorth(t, shuffled, "", ""); !reflect.DeepEqual(got, want) {
				t.Fatalf("result changed with the input order:\n got %+v\nwant %+v", got, want)
			}
		}
	})
}
