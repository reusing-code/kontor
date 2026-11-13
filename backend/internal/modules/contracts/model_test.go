package contracts

import "testing"

func TestNormalizedPrices(t *testing.T) {
	price := 30.0
	tests := []struct {
		interval      BillingInterval
		monthly, year float64
	}{
		{"", 30, 360},
		{BillingMonthly, 30, 360},
		{BillingQuarterly, 10, 120},
		{BillingYearly, 2.5, 30},
	}
	for _, tt := range tests {
		c := Contract{Price: &price, BillingInterval: tt.interval}
		if got := c.MonthlyPrice(); got != tt.monthly {
			t.Errorf("%q: MonthlyPrice = %v, want %v", tt.interval, got, tt.monthly)
		}
		if got := c.YearlyPrice(); got != tt.year {
			t.Errorf("%q: YearlyPrice = %v, want %v", tt.interval, got, tt.year)
		}
	}
}

func TestNormalizedPricesWithoutPrice(t *testing.T) {
	c := Contract{BillingInterval: BillingQuarterly}
	if c.MonthlyPrice() != 0 || c.YearlyPrice() != 0 {
		t.Errorf("got %v / %v, want 0", c.MonthlyPrice(), c.YearlyPrice())
	}
}

func TestContractInputValidateBillingInterval(t *testing.T) {
	for _, bi := range []BillingInterval{"", BillingMonthly, BillingQuarterly, BillingYearly} {
		in := ContractInput{Name: "Internet", StartDate: "2026-01-01", BillingInterval: bi}
		if err := in.Validate(); err != nil {
			t.Errorf("%q: unexpected error %v", bi, err)
		}
	}
	in := ContractInput{Name: "Internet", StartDate: "2026-01-01", BillingInterval: "weekly"}
	err := in.Validate()
	if err == nil || err.Error() != "billingInterval must be 'monthly', 'quarterly' or 'yearly'" {
		t.Errorf("weekly: err = %v", err)
	}
}
