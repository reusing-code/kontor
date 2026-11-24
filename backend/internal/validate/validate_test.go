package validate

import (
	"errors"
	"strings"
	"testing"
)

func fp(v float64) *float64 { return &v }

func ip(v int) *int { return &v }

func TestChecks(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want string
	}{
		{"name ok", Name("name", new("  Golf ")), ""},
		{"name blank", Name("name", new("   ")), "name is required"},
		{"name too long", Name("name", new(strings.Repeat("ä", MaxName+1))), "name must be at most 200 characters"},
		{"name at the cap", Name("name", new(strings.Repeat("ä", MaxName))), ""},
		{"text ok", Text("comments", strings.Repeat("x", MaxText), MaxText), ""},
		{"text too long", Text("url", strings.Repeat("x", MaxURL+1), MaxURL), "url must be at most 2048 characters"},
		{"date ok", Date("date", "2026-02-28"), ""},
		{"date missing", Date("date", ""), "date is required"},
		{"date bad", Date("date", "2026-02-30"), "date must be a date in YYYY-MM-DD format"},
		{"optional date empty", OptionalDate("endDate", ""), ""},
		{"optional date bad", OptionalDate("endDate", "28.02.2026"), "endDate must be a date in YYYY-MM-DD format"},
		{"non-negative nil", NonNegative("price", nil), ""},
		{"non-negative zero", NonNegative("price", fp(0)), ""},
		{"negative", NonNegative("price", fp(-0.01)), "price must not be negative"},
		{"months ok", Months("m", MaxMonths), ""},
		{"months negative", Months("m", -1), "m must not be negative"},
		{"months too many", Months("m", MaxMonths+1), "m must be at most 1200"},
		{"optional months nil", OptionalMonths("m", nil), ""},
		{"optional months bad", OptionalMonths("m", ip(-3)), "m must not be negative"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ""
			if tt.err != nil {
				got = tt.err.Error()
			}
			if got != tt.want {
				t.Errorf("error = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestName_TrimsInPlace(t *testing.T) {
	v := "\t Golf \n"
	if err := Name("name", &v); err != nil || v != "Golf" {
		t.Errorf("Name = %v, value %q", err, v)
	}
}

func TestFirst(t *testing.T) {
	a, b := errors.New("a"), errors.New("b")
	if err := First(nil, a, b); err != a {
		t.Errorf("First = %v, want a", err)
	}
	if err := First(nil, nil); err != nil {
		t.Errorf("First = %v, want nil", err)
	}
}
