package ledger

import (
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"
)

var recurringToday = time.Date(2026, 11, 13, 10, 0, 0, 0, time.UTC)

func recurPay(name, date string, amount int64, mutate ...func(*LedgerTransaction)) LedgerTransaction {
	t := LedgerTransaction{
		ID: uuid.New(), BookingDate: date, AmountMinor: -amount, Currency: "EUR", CounterpartyName: name,
	}
	for _, m := range mutate {
		m(&t)
	}
	return t
}

// recurSeries books one payment per step starting at start; amounts cycle.
func recurSeries(name, start string, months, n int, amounts ...int64) []LedgerTransaction {
	first, err := time.Parse(time.DateOnly, start)
	if err != nil {
		panic(err)
	}
	out := make([]LedgerTransaction, n)
	for i := range n {
		out[i] = recurPay(name, first.AddDate(0, i*months, 0).Format(time.DateOnly), amounts[i%len(amounts)])
	}
	return out
}

func recurDates(name string, amount int64, dates ...string) []LedgerTransaction {
	out := make([]LedgerTransaction, len(dates))
	for i, d := range dates {
		out[i] = recurPay(name, d, amount)
	}
	return out
}

type wantRecurring struct {
	name     string
	interval string
	count    int
	price    int64
	start    string
	last     string
	ended    bool
}

func TestDetectRecurring(t *testing.T) {
	contractRef := func(t *LedgerTransaction) {
		t.References = []LedgerTransactionReference{{Type: LedgerReferenceContract, TargetID: uuid.New()}}
	}
	linkedChain := recurSeries("Gym", "2026-01-03", 1, 6, 2990)
	contractRef(&linkedChain[2])

	tests := []struct {
		name string
		txns []LedgerTransaction
		want []wantRecurring
	}{
		{
			name: "monthly",
			txns: recurSeries("Netflix International B.V.", "2026-01-05", 1, 11, 999),
			want: []wantRecurring{{"Netflix International B.V.", RecurringMonthly, 11, 999, "2026-01-05", "2026-11-05", false}},
		},
		{
			name: "quarterly",
			txns: recurSeries("Stadtwerke", "2025-11-15", 3, 4, 12000),
			want: []wantRecurring{{"Stadtwerke", RecurringQuarterly, 4, 12000, "2025-11-15", "2026-08-15", false}},
		},
		{
			name: "yearly",
			txns: recurSeries("ADAC", "2025-03-01", 12, 2, 9400),
			want: []wantRecurring{{"ADAC", RecurringYearly, 2, 9400, "2025-03-01", "2026-03-01", false}},
		},
		{
			name: "gradual price increase stays one group",
			txns: []LedgerTransaction{
				recurPay("Spotify", "2026-07-01", 1000), recurPay("Spotify", "2026-08-01", 1050),
				recurPay("Spotify", "2026-09-01", 1100), recurPay("Spotify", "2026-10-01", 1150),
				recurPay("Spotify", "2026-11-01", 1200),
			},
			want: []wantRecurring{{"Spotify", RecurringMonthly, 5, 1200, "2026-07-01", "2026-11-01", false}},
		},
		{
			name: "10 percent tolerance edge joins",
			txns: recurSeries("Telekom", "2026-06-10", 1, 6, 2000, 2200),
			want: []wantRecurring{{"Telekom", RecurringMonthly, 6, 2200, "2026-06-10", "2026-11-10", false}},
		},
		{
			name: "above 10 percent splits into chains without a rhythm",
			txns: recurSeries("Telekom", "2026-06-10", 1, 6, 2000, 2201),
		},
		{
			name: "1 EUR minimum tolerance edge joins",
			txns: recurSeries("Bäcker Hansen", "2026-06-10", 1, 6, 500, 600),
			want: []wantRecurring{{"Bäcker Hansen", RecurringMonthly, 6, 600, "2026-06-10", "2026-11-10", false}},
		},
		{
			name: "above 1 EUR minimum tolerance splits",
			txns: recurSeries("Bäcker Hansen", "2026-06-10", 1, 6, 500, 601),
		},
		{
			name: "too few monthly",
			txns: recurSeries("Netflix", "2026-09-05", 1, 2, 999),
		},
		{
			name: "too few quarterly",
			txns: recurSeries("Stadtwerke", "2026-02-15", 3, 2, 12000),
		},
		{
			name: "single yearly payment",
			txns: recurSeries("ADAC", "2026-03-01", 12, 1, 9400),
		},
		{
			name: "irregular gaps",
			txns: recurDates("Amazon", 2500, "2026-01-01", "2026-01-10", "2026-03-01", "2026-03-20", "2026-06-01"),
		},
		{
			name: "one missed month within 80 percent",
			txns: recurDates("Netflix", 999, "2026-01-05", "2026-02-05", "2026-03-05", "2026-05-05", "2026-06-05", "2026-07-05"),
			want: []wantRecurring{{"Netflix", RecurringMonthly, 6, 999, "2026-01-05", "2026-07-05", true}},
		},
		{
			name: "one missed month below 80 percent",
			txns: recurDates("Netflix", 999, "2026-06-05", "2026-07-05", "2026-09-05", "2026-10-05", "2026-11-05"),
		},
		{
			name: "ended monthly",
			txns: recurSeries("Sky", "2026-01-05", 1, 6, 2500),
			want: []wantRecurring{{"Sky", RecurringMonthly, 6, 2500, "2026-01-05", "2026-06-05", true}},
		},
		{
			name: "monthly on the ended boundary is still running",
			txns: recurSeries("Sky", "2026-07-13", 1, 3, 2500),
			want: []wantRecurring{{"Sky", RecurringMonthly, 3, 2500, "2026-07-13", "2026-09-13", false}},
		},
		{
			name: "ended yearly",
			txns: recurSeries("Old Insurance", "2022-01-01", 12, 2, 30000),
			want: []wantRecurring{{"Old Insurance", RecurringYearly, 2, 30000, "2022-01-01", "2023-01-01", true}},
		},
		{
			name: "skipped transactions",
			txns: slices.Concat(
				recurMutate(recurSeries("Dollar Shop", "2026-01-05", 1, 6, 999), func(t *LedgerTransaction) { t.Currency = "USD" }),
				recurSeries("Employer", "2026-01-05", 1, 6, -300000),
				recurMutate(recurSeries("Savings", "2026-01-05", 1, 6, 50000), func(t *LedgerTransaction) {
					t.SpecialCategory = LedgerSpecialCategoryInternalTransfer
				}),
				recurMutate(recurSeries("Pair", "2026-01-05", 1, 6, 50000), func(t *LedgerTransaction) {
					pair := uuid.New()
					t.TransferPairTransactionID = &pair
				}),
				recurMutate(recurSeries("No Currency", "2026-01-05", 1, 6, 999), func(t *LedgerTransaction) { t.Currency = "" }),
				linkedChain,
			),
		},
		{
			name: "same counterparty with two amounts",
			txns: slices.Concat(
				recurSeries("PayPal Europe S.a.r.l. et Cie S.C.A", "2026-01-02", 1, 5, 999),
				recurSeries("PayPal Europe S.a.r.l. et Cie S.C.A", "2026-01-20", 1, 5, 4999),
			),
			want: []wantRecurring{
				{"PayPal Europe S.a.r.l. et Cie S.C.A", RecurringMonthly, 5, 999, "2026-01-02", "2026-05-02", true},
				{"PayPal Europe S.a.r.l. et Cie S.C.A", RecurringMonthly, 5, 4999, "2026-01-20", "2026-05-20", true},
			},
		},
		{
			name: "IBAN fallback",
			txns: []LedgerTransaction{
				recurPay("", "2026-08-01", 4000, withIBAN("de89 3704 0044 0532 0130 00")),
				recurPay("GmbH 123", "2026-09-01", 4000, withIBAN("DE89370400440532013000")),
				recurPay("", "2026-10-01", 4000, withIBAN("DE89 3704 0044 0532 0130 00")),
			},
			want: []wantRecurring{{"DE89370400440532013000", RecurringMonthly, 3, 4000, "2026-08-01", "2026-10-01", false}},
		},
		{
			name: "neither name tokens nor IBAN",
			txns: recurSeries("12 GmbH", "2026-08-01", 1, 4, 4000),
		},
		{
			name: "sorted by name",
			txns: slices.Concat(recurSeries("zeit online", "2026-08-01", 1, 3, 1500), recurSeries("Audible", "2026-08-03", 1, 3, 995)),
			want: []wantRecurring{
				{"Audible", RecurringMonthly, 3, 995, "2026-08-03", "2026-10-03", false},
				{"zeit online", RecurringMonthly, 3, 1500, "2026-08-01", "2026-10-01", false},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DetectRecurring(tt.txns, recurringToday)
			if len(got) != len(tt.want) {
				t.Fatalf("got %d groups, want %d: %+v", len(got), len(tt.want), got)
			}
			for i, w := range tt.want {
				g := got[i]
				have := wantRecurring{g.Name, g.Interval, g.Count, g.PriceMinor, g.StartDate, g.LastDate, g.Ended}
				if have != w {
					t.Errorf("group %d = %+v, want %+v", i, have, w)
				}
				if len(g.TransactionIDs) != g.Count || len(g.Ref) != recurringRefLength {
					t.Errorf("group %d: %d ids, ref %q", i, len(g.TransactionIDs), g.Ref)
				}
			}
		})
	}
}

func recurMutate(txns []LedgerTransaction, fn func(*LedgerTransaction)) []LedgerTransaction {
	for i := range txns {
		fn(&txns[i])
	}
	return txns
}

func withIBAN(iban string) func(*LedgerTransaction) {
	return func(t *LedgerTransaction) { t.CounterpartyIBAN = iban }
}

func TestDetectRecurring_DeterministicOrderAndRef(t *testing.T) {
	txns := recurSeries("Netflix", "2026-04-05", 1, 7, 999)
	same := recurPay("Netflix", "2026-04-05", 999)
	txns = append(txns, same)
	first := txns[0].ID
	if same.ID.String() < first.String() {
		first = same.ID
	}
	txns = append(txns, recurSeries("PayPal", "2026-06-20", 1, 4, 4999)...)

	groups := DetectRecurring(txns, recurringToday)
	if len(groups) != 2 || groups[0].Name != "Netflix" || groups[0].Count != 8 {
		t.Fatalf("groups = %+v", groups)
	}
	if g := groups[0]; g.TransactionIDs[0] != first || g.Ref != recurringRef("netflix", RecurringMonthly, first) {
		t.Fatalf("group = %+v, first = %s", g, first)
	}

	reversed := slices.Clone(txns)
	slices.Reverse(reversed)
	again := DetectRecurring(reversed, recurringToday)
	if !slices.EqualFunc(again, groups, func(a, b RecurringGroup) bool {
		return a.Ref == b.Ref && slices.Equal(a.TransactionIDs, b.TransactionIDs)
	}) {
		t.Fatalf("detection depends on input order: %+v vs %+v", again, groups)
	}
}

func TestDetectRecurring_RefStableWhenSeriesGrows(t *testing.T) {
	txns := recurSeries("Netflix", "2026-06-05", 1, 3, 999)
	before := DetectRecurring(txns, recurringToday)
	after := DetectRecurring(append(txns, recurPay("Netflix", "2026-09-05", 999)), recurringToday)
	if len(before) != 1 || len(after) != 1 || before[0].Ref != after[0].Ref || after[0].Count != 4 {
		t.Fatalf("before = %+v, after = %+v", before, after)
	}
	if before[0].Ref != recurringRef("netflix", RecurringMonthly, txns[0].ID) {
		t.Fatalf("ref = %q", before[0].Ref)
	}
}
