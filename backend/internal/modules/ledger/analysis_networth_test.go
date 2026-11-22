package ledger

import (
	"errors"
	"slices"
	"testing"

	"github.com/google/uuid"
)

func snapshotAt(accountID uuid.UUID, asOf string, amountMinor int64) LedgerBalanceSnapshot {
	return LedgerBalanceSnapshot{AccountID: accountID, AsOf: asOf, AmountMinor: amountMinor, Source: "dkb.csv"}
}

func bookedOn(accountID uuid.UUID, date string, amountMinor int64) LedgerTransaction {
	return LedgerTransaction{ID: uuid.New(), AccountID: accountID, BookingDate: date, AmountMinor: amountMinor, Currency: "EUR"}
}

func netWorth(t *testing.T, in LedgerNetWorthInput, from, to string) LedgerNetWorthAnalysis {
	t.Helper()
	got, err := AnalyzeLedgerNetWorth(in, from, to, analysisToday)
	if err != nil {
		t.Fatalf("AnalyzeLedgerNetWorth: %v", err)
	}
	return got
}

func TestLedgerNetWorthDates(t *testing.T) {
	cases := []struct {
		name, from, to string
		want           []string
	}{
		{"several months, from mid-month", "2026-08-10", "2026-11-22", []string{"2026-08-31", "2026-09-30", "2026-10-31", "2026-11-22"}},
		{"from on a month-end", "2026-08-31", "2026-09-15", []string{"2026-08-31", "2026-09-15"}},
		{"to on a month-end is not doubled", "2026-08-01", "2026-09-30", []string{"2026-08-31", "2026-09-30"}},
		{"inside one month", "2026-09-05", "2026-09-20", []string{"2026-09-20"}},
		{"a single month-end", "2026-09-30", "2026-09-30", []string{"2026-09-30"}},
		{"to after today ends at today", "2026-10-15", "2027-02-28", []string{"2026-10-31", "2026-11-22"}},
		{"range after today", "2027-01-01", "2027-03-01", []string{"2026-11-22"}},
		{"leap year and year change", "2023-12-15", "2024-03-15", []string{"2023-12-31", "2024-01-31", "2024-02-29", "2024-03-15"}},
		{"no leap day in 2100", "2100-02-01", "2100-03-01", []string{"2026-11-22"}},
	}
	for _, tc := range cases {
		if got := ledgerNetWorthDates(LedgerAnalysisRange{From: tc.from, To: tc.to}, analysisToday); !slices.Equal(got, tc.want) {
			t.Fatalf("%s: dates = %v, want %v", tc.name, got, tc.want)
		}
	}
	if daysInMonth(2100, 2) != 28 || daysInMonth(2000, 2) != 29 || daysInMonth(2026, 4) != 30 || daysInMonth(2026, 12) != 31 {
		t.Fatal("daysInMonth is wrong")
	}
}

func TestLedgerBalanceAt(t *testing.T) {
	account := uuid.New()
	booked := newBookedSums([]LedgerTransaction{
		bookedOn(account, "2026-03-20", -50),
		bookedOn(account, "2026-03-01", 500),
		bookedOn(account, "2026-03-25", -300),
		bookedOn(account, "2026-03-10", -200),
		bookedOn(account, "2026-03-15", 1000),
	})
	snapshots := []LedgerBalanceSnapshot{snapshotAt(account, "2026-03-10", 10_000), snapshotAt(account, "2026-03-20", 12_000)}

	for _, tc := range []struct {
		name, date string
		want       int64
	}{
		{"at the first snapshot, its day's booking is included", "2026-03-10", 10_000},
		{"between the snapshots, nothing booked yet", "2026-03-12", 10_000},
		{"between the snapshots, rolled forward", "2026-03-15", 11_000},
		{"the day before the second snapshot", "2026-03-19", 11_000},
		{"at the second snapshot", "2026-03-20", 12_000},
		{"after the anchor", "2026-03-25", 11_700},
		{"long after the anchor", "2026-12-31", 11_700},
		{"the day before the first snapshot, walked back", "2026-03-09", 10_200},
		{"before the first snapshot, the day's own booking stays in", "2026-03-01", 10_200},
		{"before every booking", "2026-02-28", 9_700},
	} {
		if got := ledgerBalanceAt(snapshots, booked, tc.date); got != tc.want {
			t.Fatalf("%s (%s): balance = %d, want %d", tc.name, tc.date, got, tc.want)
		}
	}

	if got := booked.between("2026-03-10", "2026-03-10"); got != 0 {
		t.Fatalf("empty interval = %d", got)
	}
	if got := booked.between("", "9999-12-31"); got != 950 {
		t.Fatalf("everything = %d, want 950", got)
	}
}

func TestAnalyzeLedgerNetWorth_Drift(t *testing.T) {
	account := analysisAccount(uuid.New(), "EUR")
	in := LedgerNetWorthInput{
		Accounts:     []LedgerAccount{account},
		Snapshots:    []LedgerBalanceSnapshot{snapshotAt(account.ID, "2026-03-10", 10_000), snapshotAt(account.ID, "2026-04-15", 13_000)},
		Transactions: []LedgerTransaction{bookedOn(account.ID, "2026-03-20", 1000), bookedOn(account.ID, "2026-04-20", -500)},
	}

	for _, tc := range []struct {
		name, to string
		dates    []string
		values   []int64
	}{
		{"the day before the second snapshot follows the first", "2026-04-14", []string{"2026-03-31", "2026-04-14"}, []int64{11_000, 11_000}},
		{"the second snapshot's day jumps by the drift", "2026-04-15", []string{"2026-03-31", "2026-04-15"}, []int64{11_000, 13_000}},
		{"later points follow the second snapshot", "2026-05-31", []string{"2026-03-31", "2026-04-30", "2026-05-31"}, []int64{11_000, 12_500, 12_500}},
	} {
		got := netWorth(t, in, "2026-03-01", tc.to)
		if !slices.Equal(got.Dates, tc.dates) || !slices.Equal(got.Accounts[0].ValuesMinor, tc.values) || !slices.Equal(got.TotalMinor, tc.values) {
			t.Fatalf("%s: dates = %v, values = %v, total = %v, want %v, %v", tc.name, got.Dates, got.Accounts[0].ValuesMinor, got.TotalMinor, tc.dates, tc.values)
		}
	}
}

func TestAnalyzeLedgerNetWorth_Accounts(t *testing.T) {
	main := analysisAccount(uuid.New(), "EUR")
	main.Name = "Main"
	savings := analysisAccount(uuid.New(), "")
	savings.Name = "Savings"
	noBalance := analysisAccount(uuid.New(), "EUR")
	noBalance.Name = "No balance"
	dollars := analysisAccount(uuid.New(), "usd")
	dollars.Name = "Dollars"
	dollarsNoBalance := analysisAccount(uuid.New(), "USD")
	dollarsNoBalance.Name = "More dollars"
	in := LedgerNetWorthInput{
		Accounts: []LedgerAccount{dollars, main, dollarsNoBalance, noBalance, savings},
		Snapshots: []LedgerBalanceSnapshot{
			snapshotAt(dollars.ID, "2026-09-01", 777_777),
			snapshotAt(main.ID, "2026-09-15", 50_000),
			snapshotAt(savings.ID, "2026-10-20", 200_000),
		},
		Transactions: []LedgerTransaction{
			bookedOn(noBalance.ID, "2025-01-01", -999),
			bookedOn(dollars.ID, "2025-02-01", -999),
			bookedOn(main.ID, "2026-08-20", -1000),
			bookedOn(main.ID, "2026-10-05", -2000),
			bookedOn(main.ID, "2026-11-22", -300),
			bookedOn(main.ID, "2026-12-01", -40),
			bookedOn(savings.ID, "2026-11-10", 5000),
		},
	}

	t.Run("all time ends at the current balance", func(t *testing.T) {
		got := netWorth(t, in, "", "")
		if got.From != "2026-08-20" || got.To != analysisToday {
			t.Fatalf("range = %s..%s, want the first booking of an included account up to today", got.From, got.To)
		}
		if want := []string{"2026-08-31", "2026-09-30", "2026-10-31", "2026-11-22"}; !slices.Equal(got.Dates, want) {
			t.Fatalf("dates = %v, want %v", got.Dates, want)
		}
		if len(got.Accounts) != 2 || got.Accounts[0].AccountID != main.ID || got.Accounts[0].Name != "Main" || got.Accounts[1].AccountID != savings.ID {
			t.Fatalf("accounts = %+v, want Main and Savings in input order", got.Accounts)
		}
		// The last point includes the booking dated after today, like currentMinor.
		if want := []int64{50_000, 50_000, 48_000, 47_660}; !slices.Equal(got.Accounts[0].ValuesMinor, want) {
			t.Fatalf("Main = %v, want %v", got.Accounts[0].ValuesMinor, want)
		}
		if want := []int64{200_000, 200_000, 200_000, 205_000}; !slices.Equal(got.Accounts[1].ValuesMinor, want) {
			t.Fatalf("Savings = %v, want %v", got.Accounts[1].ValuesMinor, want)
		}
		if want := []int64{250_000, 250_000, 248_000, 252_660}; !slices.Equal(got.TotalMinor, want) {
			t.Fatalf("total = %v, want %v", got.TotalMinor, want)
		}
		want := []LedgerNetWorthExcluded{
			{AccountID: dollars.ID, Name: "Dollars", Reason: LedgerNetWorthCurrency, Currency: "USD"},
			{AccountID: dollarsNoBalance.ID, Name: "More dollars", Reason: LedgerNetWorthCurrency, Currency: "USD"},
			{AccountID: noBalance.ID, Name: "No balance", Reason: LedgerNetWorthNoBalance},
		}
		if !slices.Equal(got.ExcludedAccounts, want) {
			t.Fatalf("excluded = %+v, want %+v", got.ExcludedAccounts, want)
		}
	})

	t.Run("a range that ends before today uses the balance of that day", func(t *testing.T) {
		got := netWorth(t, in, "2026-11-01", "2026-11-21")
		if !slices.Equal(got.Dates, []string{"2026-11-21"}) || !slices.Equal(got.TotalMinor, []int64{48_000 + 205_000}) {
			t.Fatalf("dates = %v, total = %v", got.Dates, got.TotalMinor)
		}
	})

	t.Run("a range after today is the point at today", func(t *testing.T) {
		got := netWorth(t, in, "2027-01-01", "2027-06-30")
		if got.From != "2027-01-01" || got.To != "2027-06-30" || !slices.Equal(got.Dates, []string{analysisToday}) || !slices.Equal(got.TotalMinor, []int64{252_660}) {
			t.Fatalf("got %+v", got)
		}
	})

	t.Run("only excluded accounts", func(t *testing.T) {
		got := netWorth(t, LedgerNetWorthInput{Accounts: []LedgerAccount{dollars, noBalance}, Snapshots: in.Snapshots, Transactions: in.Transactions}, "", "")
		if len(got.Dates)+len(got.TotalMinor)+len(got.Accounts) != 0 || got.Dates == nil || got.TotalMinor == nil || got.Accounts == nil {
			t.Fatalf("got %#v, want empty non-nil series", got)
		}
		if len(got.ExcludedAccounts) != 2 || got.From != analysisToday || got.To != analysisToday {
			t.Fatalf("got %+v", got)
		}
	})

	t.Run("no accounts", func(t *testing.T) {
		got := netWorth(t, LedgerNetWorthInput{}, "2026-01-01", "2026-06-30")
		if got.Dates == nil || got.TotalMinor == nil || got.Accounts == nil || got.ExcludedAccounts == nil ||
			len(got.Dates)+len(got.TotalMinor)+len(got.Accounts)+len(got.ExcludedAccounts) != 0 {
			t.Fatalf("got %#v, want empty non-nil slices", got)
		}
		if got.From != "2026-01-01" || got.To != "2026-06-30" {
			t.Fatalf("range = %s..%s", got.From, got.To)
		}
	})

	t.Run("bad range", func(t *testing.T) {
		if _, err := AnalyzeLedgerNetWorth(in, "2026-06-01", "2026-05-01", analysisToday); !errors.Is(err, ErrLedgerAnalysisInvalid) {
			t.Fatalf("err = %v, want ErrLedgerAnalysisInvalid", err)
		}
	})

	t.Run("unsorted snapshots", func(t *testing.T) {
		shuffled := LedgerNetWorthInput{
			Accounts:  []LedgerAccount{main},
			Snapshots: []LedgerBalanceSnapshot{snapshotAt(main.ID, "2026-10-01", 900), snapshotAt(main.ID, "2026-09-01", 100)},
		}
		got := netWorth(t, shuffled, "2026-09-01", "2026-10-31")
		if !slices.Equal(got.TotalMinor, []int64{100, 900}) {
			t.Fatalf("total = %v, want 100, 900", got.TotalMinor)
		}
	})
}
