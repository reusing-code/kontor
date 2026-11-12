package ledger

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestDeriveMatchRule(t *testing.T) {
	tests := []struct {
		name          string
		merchant      string
		paymentMethod string
		window        ChargeWindow
		matches       []string
		nonMatches    []string
		noPattern     bool
	}{
		{name: "paypal without merchant", paymentMethod: PaymentMethodPayPal, window: payPalChargeWindow, matches: []string{"PayPal (Europe) S.a.r.l."}},
		{name: "paypal with merchant", merchant: "Twitch", paymentMethod: PaymentMethodPayPal, window: payPalChargeWindow, matches: []string{"PAYPAL"}, nonMatches: []string{"TWITCH"}},
		{name: "amazon card", merchant: "Amazon.de", paymentMethod: PaymentMethodCard, window: amazonChargeWindow, matches: []string{"AMAZON PAYMENTS EUROPE"}},
		{name: "amazon uppercase", merchant: "AMAZON EU S.A.R.L.", paymentMethod: PaymentMethodUnknown, window: amazonChargeWindow, matches: []string{"amazon"}},
		{name: "amazon lowercase", merchant: "amazon marketplace", paymentMethod: "", window: amazonChargeWindow, matches: []string{"Amazon"}},
		{name: "amazon via paypal", merchant: "Amazon.de", paymentMethod: PaymentMethodPayPal, window: payPalChargeWindow, matches: []string{"PayPal"}, nonMatches: []string{"AMAZON"}},
		{name: "generic merchant is quoted", merchant: "Acme (Berlin) GmbH", paymentMethod: PaymentMethodCard, window: defaultChargeWindow, matches: []string{"ACME (BERLIN) GMBH"}, nonMatches: []string{"ACMEXBERLINX"}},
		{name: "merchant whitespace collapsed", merchant: "  Acme \n  GmbH ", paymentMethod: PaymentMethodDirectDebit, window: defaultChargeWindow, matches: []string{"acme gmbh"}},
		{name: "one letter merchant", merchant: "X", paymentMethod: PaymentMethodCard, window: defaultChargeWindow, noPattern: true},
		{name: "two letter merchant", merchant: " DM ", paymentMethod: PaymentMethodDirectDebit, window: defaultChargeWindow, noPattern: true},
		{name: "three letter merchant", merchant: "Aldi", paymentMethod: PaymentMethodCard, window: defaultChargeWindow, matches: []string{"ALDI SUED"}},
		{name: "short merchant paid via paypal", merchant: "X", paymentMethod: PaymentMethodPayPal, window: payPalChargeWindow, matches: []string{"PAYPAL"}},
		{name: "blank merchant", merchant: "  \t", paymentMethod: PaymentMethodCard, window: defaultChargeWindow, noPattern: true},
		{name: "empty merchant and method", window: defaultChargeWindow, noPattern: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rule := deriveMatchRule(tt.merchant, tt.paymentMethod)
			assertRule(t, rule, tt.window, tt.noPattern, tt.matches, tt.nonMatches)
		})
	}
}

func TestDeriveMatchRule_BoundsPatternLength(t *testing.T) {
	rule := deriveMatchRule(strings.Repeat("x", 200), PaymentMethodCard)
	if rule.Pattern == nil {
		t.Fatal("pattern is nil")
	}
	if got := len(rule.Pattern.String()); got > len("(?i)")+maxPatternMerchantRunes {
		t.Fatalf("pattern length = %d, want at most %d", got, len("(?i)")+maxPatternMerchantRunes)
	}
}

func TestMatchRuleForOrder(t *testing.T) {
	tests := []struct {
		name       string
		order      LedgerEmailOrder
		window     ChargeWindow
		matches    []string
		nonMatches []string
		noPattern  bool
	}{
		{name: "legacy amazon", order: LedgerEmailOrder{ImporterID: LedgerEmailImporterAmazonDE}, window: ChargeWindow{BeforeDays: 3, AfterDays: 60}, matches: []string{"AMAZON PAYMENTS"}},
		{name: "legacy paypal", order: LedgerEmailOrder{ImporterID: LedgerEmailImporterPayPalDE}, window: ChargeWindow{BeforeDays: 3, AfterDays: 7}, matches: []string{"PAYPAL EUROPE"}},
		{name: "legacy unknown importer", order: LedgerEmailOrder{ImporterID: "gone.de"}, window: ChargeWindow{BeforeDays: 3, AfterDays: 30}, noPattern: true},
		{name: "llm order derives from merchant", order: LedgerEmailOrder{ImporterID: LedgerEmailImporterLLM, Merchant: "Acme GmbH", PaymentMethod: PaymentMethodCard}, window: defaultChargeWindow, matches: []string{"ACME GMBH"}, nonMatches: []string{"PAYPAL"}},
		{name: "llm order paid via paypal", order: LedgerEmailOrder{ImporterID: LedgerEmailImporterLLM, Merchant: "Acme GmbH", PaymentMethod: PaymentMethodPayPal}, window: payPalChargeWindow, matches: []string{"PAYPAL"}},
		{name: "llm order without merchant", order: LedgerEmailOrder{ImporterID: LedgerEmailImporterLLM, PaymentMethod: PaymentMethodCard}, window: defaultChargeWindow, noPattern: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertRule(t, matchRuleForOrder(tt.order), tt.window, tt.noPattern, tt.matches, tt.nonMatches)
		})
	}
}

func assertRule(t *testing.T, rule matchRule, window ChargeWindow, noPattern bool, matches, nonMatches []string) {
	t.Helper()
	if rule.Window != window {
		t.Errorf("window = %+v, want %+v", rule.Window, window)
	}
	if noPattern {
		if rule.Pattern != nil {
			t.Errorf("pattern = %q, want nil", rule.Pattern)
		}
		return
	}
	if rule.Pattern == nil {
		t.Fatal("pattern is nil")
	}
	for _, s := range matches {
		if !rule.Pattern.MatchString(s) {
			t.Errorf("pattern %q does not match %q", rule.Pattern, s)
		}
	}
	for _, s := range nonMatches {
		if rule.Pattern.MatchString(s) {
			t.Errorf("pattern %q matches %q", rule.Pattern, s)
		}
	}
}

func TestAutoLinkOrder_NilPatternNeverLinks(t *testing.T) {
	h, s := newTestHandler(t)
	accountID := addAccount(t, s, "Card")
	commitTransactions(t, s, accountID,
		LedgerTransaction{ID: uuid.New(), AccountID: accountID, BookingDate: "2026-04-02", AmountMinor: -4999, Currency: "EUR", CounterpartyName: "ANYTHING"},
	)
	order := LedgerEmailOrder{ID: uuid.New(), OrderDate: "2026-04-01", TotalMinor: 4999, Currency: "EUR"}

	linked, warnings, err := h.ledgerEmail.autoLinkOrder(context.Background(), testUserID, matchRule{Window: defaultChargeWindow}, &order)
	if err != nil {
		t.Fatalf("autoLinkOrder: %v", err)
	}
	if linked || len(warnings) != 0 || len(order.LinkedTransactionIDs) != 0 {
		t.Fatalf("linked = %v, warnings = %v, ids = %v; want nothing", linked, warnings, order.LinkedTransactionIDs)
	}
}
