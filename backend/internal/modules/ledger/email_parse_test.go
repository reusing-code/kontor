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

func crlf(s string) string { return strings.ReplaceAll(s, "\n", "\r\n") }

func TestParseMessage_NestedMultipartWithAttachment(t *testing.T) {
	raw := crlf(`From: shop@example.com
Subject: Your order
Content-Type: multipart/mixed; boundary="outer"

--outer
Content-Type: multipart/alternative; boundary="inner"

--inner
Content-Type: text/plain; charset=utf-8

Order 4711, total 12,34 EUR
--inner
Content-Type: text/html; charset=utf-8

<p>Order 4711</p>
--inner--
--outer
Content-Type: text/plain; name="terms.txt"
Content-Disposition: attachment; filename="terms.txt"

General terms
--outer
Content-Type: application/pdf; name="invoice.pdf"
Content-Disposition: attachment; filename="invoice.pdf"
Content-Transfer-Encoding: base64

JVBERi0xLjQK
--outer--
`)
	got, err := parseMessage(strings.NewReader(raw))
	if err != nil {
		t.Fatalf("parseMessage: %v", err)
	}
	if got.TextBody != "Order 4711, total 12,34 EUR" {
		t.Errorf("TextBody = %q", got.TextBody)
	}
	if got.HTMLBody != "<p>Order 4711</p>" {
		t.Errorf("HTMLBody = %q", got.HTMLBody)
	}
}

func TestParseMessage_DecodesCharset(t *testing.T) {
	single := crlf("From: a@example.com\nSubject: s\nContent-Type: text/plain; charset=iso-8859-1\nContent-Transfer-Encoding: quoted-printable\n\nGr=FC=DFe, Bestellung f=FCr 5 =80\n")
	got, err := parseMessage(strings.NewReader(single))
	if err != nil {
		t.Fatalf("parseMessage: %v", err)
	}
	// Labels resolve as in WHATWG, where iso-8859-1 is windows-1252 (0x80 is €).
	if strings.TrimSpace(got.TextBody) != "Grüße, Bestellung für 5 €" {
		t.Errorf("single part TextBody = %q", got.TextBody)
	}

	multi := crlf("From: a@example.com\nSubject: s\nContent-Type: multipart/alternative; boundary=b\n\n--b\nContent-Type: text/html; charset=\"ISO-8859-1\"\n\n<p>M\xfcller</p>\n--b--\n")
	got, err = parseMessage(strings.NewReader(multi))
	if err != nil {
		t.Fatalf("parseMessage: %v", err)
	}
	if got.HTMLBody != "<p>Müller</p>" || got.TextBody != "Müller" {
		t.Errorf("multipart HTMLBody = %q, TextBody = %q", got.HTMLBody, got.TextBody)
	}
}

func TestParseMessage_LenientBase64(t *testing.T) {
	// "Order 4711 confirmed" in base64 without padding and with stray spaces.
	raw := crlf("From: a@example.com\nSubject: s\nContent-Type: text/plain; charset=utf-8\nContent-Transfer-Encoding: base64\n\nT3JkZXIgNDcx MSBj\n b25maXJtZWQ\n")
	got, err := parseMessage(strings.NewReader(raw))
	if err != nil {
		t.Fatalf("parseMessage: %v", err)
	}
	if got.TextBody != "Order 4711 confirmed" {
		t.Errorf("TextBody = %q", got.TextBody)
	}
}
