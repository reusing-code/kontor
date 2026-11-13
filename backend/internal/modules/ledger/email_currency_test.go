package ledger

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/reusing-code/kontor/backend/internal/fx"
	"github.com/reusing-code/kontor/backend/internal/llm/llmtest"
)

// stubRates answers every pair in rates ("USD/EUR") and records the dates.
type stubRates struct {
	rates map[string]float64
	err   error
	dates []string
}

func (r *stubRates) Rate(_ context.Context, from, to string, date time.Time) (fx.Rate, error) {
	r.dates = append(r.dates, date.Format("2006-01-02"))
	if r.err != nil {
		return fx.Rate{}, r.err
	}
	value, ok := r.rates[from+"/"+to]
	if !ok {
		return fx.Rate{}, fx.ErrUnknownCurrency
	}
	return fx.Rate{Value: value, Date: date.Format("2006-01-02")}, nil
}

func usdToEUR() *stubRates {
	return &stubRates{rates: map[string]float64{"USD/EUR": 0.9}}
}

func TestWithinFXTolerance(t *testing.T) {
	tests := []struct {
		amount, target int64
		want           bool
	}{
		{4499, 4499, true},
		{4723, 4499, true},
		{4724, 4499, false},
		{4275, 4499, true},
		{4274, 4499, false},
		{0, 0, false},
	}
	for _, tt := range tests {
		if got := withinFXTolerance(tt.amount, tt.target); got != tt.want {
			t.Errorf("withinFXTolerance(%d, %d) = %v, want %v", tt.amount, tt.target, got, tt.want)
		}
	}
}

func TestAutoLinkOrder_ForeignCurrency(t *testing.T) {
	// USD 49.99 at 0.9 is EUR 44.99; ±5 % allows 42.75 to 47.23.
	tests := []struct {
		name        string
		rates       fx.Rates
		amounts     []int64
		wantLinked  int // index into amounts, -1: not linked
		wantWarning string
	}{
		{"same number is not a match", usdToEUR(), []int64{-4999}, -1, ""},
		{"converted amount with fee", usdToEUR(), []int64{-4999, -4630}, 1, "linked by converted amount"},
		{"exact converted amount", usdToEUR(), []int64{-4499}, 0, "linked by converted amount"},
		{"above tolerance", usdToEUR(), []int64{-4730}, -1, ""},
		{"below tolerance", usdToEUR(), []int64{-4270}, -1, ""},
		{"ambiguous", usdToEUR(), []int64{-4499, -4630}, -1, "multiple converted transaction matches"},
		{"rates disabled", fx.Disabled{}, []int64{-4999, -4499}, -1, ""},
		{"rate lookup fails", &stubRates{err: errors.New("boom")}, []int64{-4999, -4499}, -1, ""},
		{"unknown currency", &stubRates{rates: map[string]float64{}}, []int64{-4999, -4499}, -1, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, s := newTestHandler(t)
			h.ledgerEmail = NewEmailService(s.Store, llmtest.New(), tt.rates, nil)
			accountID := addAccount(t, s, "Checking")
			txns := make([]LedgerTransaction, 0, len(tt.amounts))
			for _, amount := range tt.amounts {
				txns = append(txns, LedgerTransaction{ID: uuid.New(), AccountID: accountID, BookingDate: "2026-04-03", AmountMinor: amount, Currency: "EUR", CounterpartyName: "PayPal Europe"})
			}
			commitTransactions(t, s, accountID, txns...)
			order := LedgerEmailOrder{ID: uuid.New(), OrderDate: "2026-04-01", TotalMinor: 4999, Currency: "USD", PaymentMethod: PaymentMethodPayPal}

			linked, warnings, err := h.ledgerEmail.autoLinkOrder(context.Background(), testUserID, matchRuleForOrder(order), &order)
			if err != nil {
				t.Fatalf("autoLinkOrder: %v", err)
			}
			if linked != (tt.wantLinked >= 0) {
				t.Fatalf("linked = %v, want index %d", linked, tt.wantLinked)
			}
			if linked && (len(order.LinkedTransactionIDs) != 1 || order.LinkedTransactionIDs[0] != txns[tt.wantLinked].ID) {
				t.Fatalf("linked ids = %v, want [%s]", order.LinkedTransactionIDs, txns[tt.wantLinked].ID)
			}
			if tt.wantWarning == "" && len(warnings) != 0 || tt.wantWarning != "" && !slices.Contains(warnings, tt.wantWarning) {
				t.Fatalf("warnings = %v, want %q", warnings, tt.wantWarning)
			}
			if order.TotalMinor != 4999 || order.Currency != "USD" {
				t.Fatalf("order changed: %d %s", order.TotalMinor, order.Currency)
			}
		})
	}
}

func TestAutoLinkOrder_ConvertsAtOrderDate(t *testing.T) {
	h, s := newTestHandler(t)
	rates := usdToEUR()
	h.ledgerEmail = NewEmailService(s.Store, llmtest.New(), rates, nil)
	accountID := addAccount(t, s, "Checking")
	commitTransactions(t, s, accountID,
		LedgerTransaction{ID: uuid.New(), AccountID: accountID, BookingDate: "2026-04-03", AmountMinor: -4499, Currency: "EUR", CounterpartyName: "PayPal"},
		LedgerTransaction{ID: uuid.New(), AccountID: accountID, BookingDate: "2026-04-04", AmountMinor: -100, Currency: "EUR", CounterpartyName: "PayPal"},
	)
	order := LedgerEmailOrder{ID: uuid.New(), OrderDate: "2026-04-01", TotalMinor: 4999, Currency: "USD", PaymentMethod: PaymentMethodPayPal}
	if _, _, err := h.ledgerEmail.autoLinkOrder(context.Background(), testUserID, matchRuleForOrder(order), &order); err != nil {
		t.Fatalf("autoLinkOrder: %v", err)
	}
	if !slices.Equal(rates.dates, []string{"2026-04-01"}) {
		t.Fatalf("rate lookups = %v, want one at the order date", rates.dates)
	}
}

func TestAutoLinkOrder_SameCurrencyNeedsNoRate(t *testing.T) {
	h, s := newTestHandler(t)
	rates := usdToEUR()
	h.ledgerEmail = NewEmailService(s.Store, llmtest.New(), rates, nil)
	accountID := addAccount(t, s, "Checking")
	txnID := uuid.New()
	commitTransactions(t, s, accountID,
		LedgerTransaction{ID: txnID, AccountID: accountID, BookingDate: "2026-04-03", AmountMinor: -4999, Currency: "EUR", CounterpartyName: "PayPal"},
	)
	order := LedgerEmailOrder{ID: uuid.New(), OrderDate: "2026-04-01", TotalMinor: 4999, Currency: "EUR", PaymentMethod: PaymentMethodPayPal}
	linked, warnings, err := h.ledgerEmail.autoLinkOrder(context.Background(), testUserID, matchRuleForOrder(order), &order)
	if err != nil || !linked || len(warnings) != 0 {
		t.Fatalf("linked = %v, warnings = %v, err = %v", linked, warnings, err)
	}
	if len(rates.dates) != 0 {
		t.Fatalf("rate lookups = %v, want none", rates.dates)
	}
}

func TestStoreOrder_KeepsOriginalCurrency(t *testing.T) {
	p := newPipelineTest(t, llmtest.New())
	p.svc = NewEmailService(p.store.Store, p.fake, usdToEUR(), nil)
	accountID := addAccount(t, p.store, "Checking")
	txnID := uuid.New()
	commitTransactions(t, p.store, accountID,
		LedgerTransaction{ID: txnID, AccountID: accountID, BookingDate: "2026-04-03", AmountMinor: -4630, Currency: "EUR", CounterpartyName: "PayPal"},
	)
	result := newScanResult()
	extracted := extractedOrder{Merchant: "Example Inc", PaymentMethod: PaymentMethodPayPal, OrderDate: "2026-04-01", TotalMinor: 4999, Currency: "USD"}
	if err := p.svc.storeOrder(context.Background(), testUserID, p.account, emailMessage{Label: "uid 1", Key: "<usd@example.com>"}, ParsedEmail{}, extracted, &result); err != nil {
		t.Fatalf("storeOrder: %v", err)
	}
	orders := p.orders(t)
	if len(orders) != 1 {
		t.Fatalf("orders = %d, want 1", len(orders))
	}
	order := orders[0]
	if order.TotalMinor != 4999 || order.Currency != "USD" {
		t.Fatalf("stored %d %s, want 4999 USD", order.TotalMinor, order.Currency)
	}
	if order.MatchStatus != LedgerEmailOrderStatusMatched || !slices.Equal(order.LinkedTransactionIDs, []uuid.UUID{txnID}) {
		t.Fatalf("order = %+v, want linked to %s", order, txnID)
	}
	if result.OrdersLinked != 1 {
		t.Fatalf("OrdersLinked = %d, want 1", result.OrdersLinked)
	}
}

func TestListLedgerEmailOrderCandidates_ForeignCurrency(t *testing.T) {
	setup := func(t *testing.T, rates fx.Rates) (LedgerEmailOrderCandidatesResult, []uuid.UUID) {
		t.Helper()
		h, s := newTestHandler(t)
		h.ledgerEmail = NewEmailService(s.Store, llmtest.New(), rates, nil)
		accountID := addAccount(t, s, "Checking")
		sameNumber, converted := uuid.New(), uuid.New()
		commitTransactions(t, s, accountID,
			LedgerTransaction{ID: sameNumber, AccountID: accountID, BookingDate: "2026-04-02", AmountMinor: -4999, Currency: "EUR"},
			LedgerTransaction{ID: converted, AccountID: accountID, BookingDate: "2026-04-05", AmountMinor: -4630, Currency: "EUR"},
		)
		now := time.Now().UTC()
		order := LedgerEmailOrder{
			ID: uuid.New(), EmailAccountID: uuid.New(), ImporterID: LedgerEmailImporterLLM, PaymentMethod: PaymentMethodPayPal,
			OrderDate: "2026-04-01", TotalMinor: 4999, Currency: "USD", MatchStatus: LedgerEmailOrderStatusUnmatched, CreatedAt: now, UpdatedAt: now,
		}
		if err := s.CreateLedgerEmailOrder(context.Background(), testUserID, order); err != nil {
			t.Fatalf("CreateLedgerEmailOrder: %v", err)
		}
		rec := httptest.NewRecorder()
		newMux(h).ServeHTTP(rec, httptest.NewRequest("GET", "/api/v1/ledger/email-orders/"+order.ID.String()+"/candidates", nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
		}
		return decodeJSON[LedgerEmailOrderCandidatesResult](t, rec), []uuid.UUID{sameNumber, converted}
	}
	ids := func(result LedgerEmailOrderCandidatesResult) []uuid.UUID {
		out := make([]uuid.UUID, 0, len(result.Items))
		for _, item := range result.Items {
			out = append(out, item.Transaction.ID)
		}
		return out
	}

	t.Run("converted", func(t *testing.T) {
		result, txns := setup(t, usdToEUR())
		want := LedgerEmailOrderConversion{Currency: "EUR", TotalMinor: 4499, Rate: 0.9, RateDate: "2026-04-01"}
		if len(result.Conversions) != 1 || result.Conversions[0] != want {
			t.Fatalf("conversions = %+v, want %+v", result.Conversions, want)
		}
		if len(result.UnconvertedCurrencies) != 0 {
			t.Fatalf("unconverted = %v, want none", result.UnconvertedCurrencies)
		}
		if !slices.Equal(ids(result), []uuid.UUID{txns[1], txns[0]}) {
			t.Fatalf("order = %v, want the converted amount first", ids(result))
		}
		for _, item := range result.Items {
			if !item.AmountComparable {
				t.Fatalf("item %s not comparable", item.Transaction.ID)
			}
		}
	})

	t.Run("no rate", func(t *testing.T) {
		result, txns := setup(t, fx.Disabled{})
		if len(result.Conversions) != 0 || !slices.Equal(result.UnconvertedCurrencies, []string{"EUR"}) {
			t.Fatalf("conversions = %+v, unconverted = %v", result.Conversions, result.UnconvertedCurrencies)
		}
		// Not comparable by amount: closest date first.
		if !slices.Equal(ids(result), []uuid.UUID{txns[0], txns[1]}) {
			t.Fatalf("order = %v, want by date", ids(result))
		}
		for _, item := range result.Items {
			if item.AmountComparable {
				t.Fatalf("item %s comparable without a rate", item.Transaction.ID)
			}
		}
	})
}
