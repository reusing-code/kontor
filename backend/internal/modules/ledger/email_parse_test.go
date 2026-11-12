package ledger

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestStripHTML(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"line breaks", "a<br>b<br/>c<br />d", "a\nb\nc\nd"},
		{"block ends", "<div>one</div><p>two</p>", "one\ntwo"},
		{"entities", "Tom &amp; Jerry &lt;3 &euro;5&nbsp;x", "Tom & Jerry <3 €5 x"},
		{"drops style head script", "<head><title>t</title></head><style>p{color:red}</style><script>var a=1;</script><p>text</p>", "text"},
		{"head is not header", "<header>kept</header>", "kept"},
		{"collapses whitespace", "<p>a   \t b</p>\n\n\n  <p>c</p>", "a b\nc"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := stripHTML(tt.in); got != tt.want {
				t.Errorf("stripHTML() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestStripHTML_Fixtures(t *testing.T) {
	paypal := parseFixtureEmail(t, "PaypalDE-3.eml")
	if n := utf8.RuneCountInString(paypal.TextBody); n >= 3000 {
		t.Errorf("PayPal text has %d runes, want < 3000", n)
	}
	for _, want := range []string{"Transaktionscode", "8JX72041NC563984Q", "8,47"} {
		if !strings.Contains(paypal.TextBody, want) {
			t.Errorf("PayPal text lacks %q", want)
		}
	}
	for _, unwanted := range []string{"{", "font-family", "&amp;"} {
		if strings.Contains(paypal.TextBody, unwanted) {
			t.Errorf("PayPal text contains %q", unwanted)
		}
	}

	amazon := parseFixtureEmail(t, "AmazonDE-1.eml")
	if n := utf8.RuneCountInString(stripHTML(amazon.HTMLBody)); n >= 15000 {
		t.Errorf("Amazon stripped HTML has %d runes, want < 15000", n)
	}
}
