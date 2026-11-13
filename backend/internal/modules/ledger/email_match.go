package ledger

import (
	"regexp"
	"strings"
	"unicode/utf8"
)

type ChargeWindow struct {
	BeforeDays int `json:"beforeDays"`
	AfterDays  int `json:"afterDays"`
}

// Contains reports whether bookingDate lies within the window around orderDate.
// An order without a date imposes no constraint.
func (w ChargeWindow) Contains(bookingDate, orderDate string) bool {
	if orderDate == "" {
		return true
	}
	delta, ok := ledgerSignedDateDeltaDays(bookingDate, orderDate)
	return ok && delta >= -w.BeforeDays && delta <= w.AfterDays
}

// matchRule says which transactions may pay an order: the counterparty/purpose
// pattern and the range around the order date in which the payment is booked.
type matchRule struct {
	Pattern *regexp.Regexp // nil: never auto-link
	Window  ChargeWindow
}

const (
	maxPatternMerchantRunes = 40
	// minPatternMerchantRunes keeps a merchant like "X" or "DM" from matching
	// almost every transaction; such orders are only linked manually.
	minPatternMerchantRunes = 3
)

var (
	defaultChargeWindow = ChargeWindow{BeforeDays: 3, AfterDays: 30}
	payPalChargeWindow  = ChargeWindow{BeforeDays: 3, AfterDays: 7}
	amazonChargeWindow  = ChargeWindow{BeforeDays: 3, AfterDays: 60}
	payPalPattern       = regexp.MustCompile(`(?i)paypal`)
	amazonPattern       = regexp.MustCompile(`(?i)amazon`)
)

type merchantOverride struct {
	Contains string // lower-case substring of the merchant
	Pattern  *regexp.Regexp
	Window   ChargeWindow
}

// merchantOverrides apply only to orders that were not paid via PayPal.
var merchantOverrides = []merchantOverride{
	{Contains: "amazon", Pattern: amazonPattern, Window: amazonChargeWindow},
}

func deriveMatchRule(merchant, paymentMethod string) matchRule {
	if paymentMethod == PaymentMethodPayPal {
		return matchRule{Pattern: payPalPattern, Window: payPalChargeWindow}
	}
	name := collapseSpaces(merchant)
	if name == "" {
		return matchRule{Window: defaultChargeWindow}
	}
	lower := strings.ToLower(name)
	for _, o := range merchantOverrides {
		if strings.Contains(lower, o.Contains) {
			return matchRule{Pattern: o.Pattern, Window: o.Window}
		}
	}
	if utf8.RuneCountInString(name) < minPatternMerchantRunes {
		return matchRule{Window: defaultChargeWindow}
	}
	re, err := regexp.Compile("(?i)" + regexp.QuoteMeta(truncateRunes(name, maxPatternMerchantRunes)))
	if err != nil {
		return matchRule{Window: defaultChargeWindow}
	}
	return matchRule{Pattern: re, Window: defaultChargeWindow}
}

// matchRuleForOrder keeps the window and pattern of orders stored before the
// LLM extraction existed (no merchant and no payment method).
func matchRuleForOrder(o LedgerEmailOrder) matchRule {
	if o.Merchant != "" || o.PaymentMethod != "" {
		return deriveMatchRule(o.Merchant, o.PaymentMethod)
	}
	switch o.ImporterID {
	case LedgerEmailImporterAmazonDE:
		return matchRule{Pattern: amazonPattern, Window: amazonChargeWindow}
	case LedgerEmailImporterPayPalDE:
		return matchRule{Pattern: payPalPattern, Window: payPalChargeWindow}
	default:
		return matchRule{Window: defaultChargeWindow}
	}
}

// fxToleranceBasisPoints is how far a payment in another currency may deviate
// from the order total converted at the ECB rate (conversion fees, rate drift
// between order and charge).
const fxToleranceBasisPoints = 500

func withinFXTolerance(amount, target int64) bool {
	delta := amount - target
	if delta < 0 {
		delta = -delta
	}
	return target > 0 && delta*10000 <= target*fxToleranceBasisPoints
}

func normalizeCurrency(currency string) string {
	currency = strings.ToUpper(strings.TrimSpace(currency))
	if currency == "" {
		return "EUR"
	}
	return currency
}

func absMinor(v int64) int64 {
	if v < 0 {
		return -v
	}
	return v
}

func collapseSpaces(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

func truncateRunes(s string, max int) string {
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return string(runes[:max])
}
