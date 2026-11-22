package ledger

import (
	"net/http"
	"testing"

	"github.com/reusing-code/kontor/backend/internal/enablebanking"
)

func apiBalance(kind, amount, currency, referenceDate string) enablebanking.Balance {
	return enablebanking.Balance{
		Name: kind, BalanceType: kind, ReferenceDate: referenceDate,
		BalanceAmount: enablebanking.Amount{Currency: currency, Amount: amount},
	}
}

func TestPickBookedBalance(t *testing.T) {
	const today = "2026-11-13"
	for name, tc := range map[string]struct {
		balances []enablebanking.Balance
		want     ParsedBalance
		ok       bool
	}{
		"interim before closing on the same day": {
			balances: []enablebanking.Balance{apiBalance("CLBD", "10.00", "EUR", today), apiBalance("ITBD", "12.50", "EUR", today), apiBalance("XPCD", "99.00", "EUR", today)},
			want:     ParsedBalance{AmountMinor: 1250, AsOf: today}, ok: true,
		},
		"newer closing wins over an older interim": {
			balances: []enablebanking.Balance{apiBalance("ITBD", "1.00", "EUR", "2026-11-10"), apiBalance("CLBD", "2.00", "EUR", "2026-11-12")},
			want:     ParsedBalance{AmountMinor: 200, AsOf: "2026-11-12"}, ok: true,
		},
		"negative": {
			balances: []enablebanking.Balance{apiBalance("CLBD", "-42.10", "EUR", "2026-11-12")},
			want:     ParsedBalance{AmountMinor: -4210, AsOf: "2026-11-12"}, ok: true,
		},
		"date from the last change": {
			balances: []enablebanking.Balance{{BalanceType: "itbd", BalanceAmount: enablebanking.Amount{Currency: "eur", Amount: "3"}, LastChangeDateTime: "2026-11-11T08:00:00Z"}},
			want:     ParsedBalance{AmountMinor: 300, AsOf: "2026-11-11"}, ok: true,
		},
		"no date and a future date are today": {
			balances: []enablebanking.Balance{apiBalance("CLBD", "1.00", "EUR", "2027-01-01")},
			want:     ParsedBalance{AmountMinor: 100, AsOf: today}, ok: true,
		},
		"available and expected only": {balances: []enablebanking.Balance{apiBalance("XPCD", "1.00", "EUR", today), apiBalance("CLAV", "1.00", "EUR", today)}},
		"other currency":              {balances: []enablebanking.Balance{apiBalance("CLBD", "1.00", "USD", today)}},
		"unreadable amount":           {balances: []enablebanking.Balance{apiBalance("CLBD", "1,00", "EUR", today)}},
		"nothing":                     {},
	} {
		t.Run(name, func(t *testing.T) {
			got, ok := pickBookedBalance(tc.balances, today)
			if ok != tc.ok || got != tc.want {
				t.Fatalf("got %+v, %v; want %+v, %v", got, ok, tc.want, tc.ok)
			}
		})
	}
}

func TestSync_StoresTheBookedBalance(t *testing.T) {
	f := newSyncFixture(t)
	if !f.conn.BalancesRequested {
		t.Fatal("a new consent must be marked as asking for balances")
	}
	f.fake.SetTransactions("uid-1", []enablebanking.Transaction{apiTxn("a", "2026-11-12", "5.00", "DBIT")})
	f.fake.SetBalances("uid-1", []enablebanking.Balance{apiBalance("ITBD", "100.00", "EUR", "2026-11-12"), apiBalance("XPCD", "80.00", "EUR", "2026-11-13")})

	summary := f.sync(t)
	if summary.Accounts[0].Status != BankSyncOK || summary.Accounts[0].BalanceStatus != BankBalanceOK {
		t.Fatalf("summary = %+v", summary.Accounts[0])
	}
	if a := f.account(t, "uid-1"); a.LastBalanceStatus != BankBalanceOK {
		t.Fatalf("account = %+v", a)
	}
	balance, err := f.GetLedgerAccountBalance(t.Context(), testUserID, f.ledgerID)
	if err != nil || balance == nil {
		t.Fatalf("balance = %+v, %v", balance, err)
	}
	if balance.CurrentMinor != 10000 || balance.AsOf != "2026-11-12" || balance.Source != string(SourceEnableBanking) {
		t.Fatalf("balance = %+v", balance)
	}
}

func TestSync_BalanceProblemsDoNotFailTheSync(t *testing.T) {
	for name, tc := range map[string]struct {
		setup func(f *syncFixture)
		want  string
	}{
		"consent without balance access": {
			setup: func(f *syncFixture) { f.fake.FailAlways("/accounts/uid-1/balances", http.StatusForbidden, "") },
			want:  BankBalanceUnavailable,
		},
		"no booked balance": {
			setup: func(f *syncFixture) {
				f.fake.SetBalances("uid-1", []enablebanking.Balance{apiBalance("XPCD", "1.00", "EUR", "2026-11-13")})
			},
			want: BankBalanceUnavailable,
		},
		"bank error": {
			setup: func(f *syncFixture) { f.fake.FailAlways("/accounts/uid-1/balances", http.StatusBadGateway, "") },
			want:  BankBalanceError,
		},
	} {
		t.Run(name, func(t *testing.T) {
			f := newSyncFixture(t)
			f.fake.SetTransactions("uid-1", []enablebanking.Transaction{apiTxn("a", "2026-11-12", "5.00", "DBIT")})
			tc.setup(f)

			summary := f.sync(t)
			if got := summary.Accounts[0]; got.Status != BankSyncOK || got.Imported != 1 || got.BalanceStatus != tc.want {
				t.Fatalf("summary = %+v", got)
			}
			if a := f.account(t, "uid-1"); a.LastSyncStatus != BankSyncOK || a.LastBalanceStatus != tc.want {
				t.Fatalf("account = %+v", a)
			}
			if b, err := f.GetLedgerAccountBalance(t.Context(), testUserID, f.ledgerID); err != nil || b != nil {
				t.Fatalf("balance = %+v, %v", b, err)
			}
		})
	}
}

func TestSync_FailedTransactionSyncFetchesNoBalance(t *testing.T) {
	f := newSyncFixture(t)
	f.fake.FailNext("/accounts/uid-1/transactions", http.StatusBadGateway, "")
	summary := f.sync(t)
	if got := summary.Accounts[0]; got.Status == BankSyncOK || got.BalanceStatus != "" {
		t.Fatalf("summary = %+v", got)
	}
	if calls := f.fake.BalanceCalls(); len(calls) != 0 {
		t.Fatalf("balance calls = %v", calls)
	}
}

func TestReauthorization_ResetsTheBalanceStatus(t *testing.T) {
	f := newSyncFixture(t)
	f.fake.FailAlways("/accounts/uid-1/balances", http.StatusForbidden, "")
	f.sync(t)
	if a := f.account(t, "uid-1"); a.LastBalanceStatus != BankBalanceUnavailable {
		t.Fatalf("account = %+v", a)
	}
	renewed := f.connect(t, "code-2", bankTestSession("SESSION-2", bankTestAccount("uid-1", bankTestIBAN1)))
	if !renewed.BalancesRequested || renewed.Accounts[0].LastBalanceStatus != "" {
		t.Fatalf("renewed = %+v", renewed)
	}
}
