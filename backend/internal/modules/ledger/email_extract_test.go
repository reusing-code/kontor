package ledger

import (
	"bufio"
	"errors"
	"os"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/reusing-code/kontor/backend/internal/llm"
	"github.com/reusing-code/kontor/backend/internal/llm/llmtest"
)

var noiseHeaderPrefixes = []string{"Received:", "DKIM-Signature:", "Authentication-Results:", "Return-Path:", "X-"}

func noiseHeaderLines(t *testing.T, fixture string) []string {
	t.Helper()
	file, err := os.Open(fixturePath(fixture))
	if err != nil {
		t.Fatalf("open fixture: %v", err)
	}
	defer file.Close()
	var lines []string
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 1<<20), 1<<20)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			break
		}
		for _, prefix := range noiseHeaderPrefixes {
			if strings.HasPrefix(line, prefix) {
				lines = append(lines, line)
			}
		}
	}
	return lines
}

func TestBuildEmailInput_Fixture(t *testing.T) {
	parsed := parseFixtureEmail(t, "AmazonDE-1.eml")
	input := buildEmailInput(parsed)

	for _, want := range []string{"304-5184726-4839201", parsed.Subject, "bestellbestaetigung@amazon.de", "Date: 2026-04-08T18:50:24Z"} {
		if !strings.Contains(input, want) {
			t.Errorf("input lacks %q", want)
		}
	}
	noise := noiseHeaderLines(t, "AmazonDE-1.eml")
	if len(noise) == 0 {
		t.Fatal("fixture has no noise header lines to check")
	}
	for _, line := range noise {
		if strings.Contains(input, line) {
			t.Errorf("input contains header line %q", line)
		}
	}
	if strings.Contains(input, parsed.MessageID) {
		t.Error("input contains the Message-ID")
	}
}

func TestBuildEmailInput_CapsBody(t *testing.T) {
	prefix := "From: a\nSubject: s\nDate: \n\nBody:\n"
	input := buildEmailInput(ParsedEmail{From: "a", Subject: "s", TextBody: strings.Repeat("ä", 50000)})
	if got := utf8.RuneCountInString(strings.TrimPrefix(input, prefix)); got != maxEmailBodyRunes {
		t.Errorf("body runes = %d, want %d", got, maxEmailBodyRunes)
	}
	if !utf8.ValidString(input) {
		t.Error("input is not valid UTF-8")
	}
}

func TestBuildEmailInput_HeaderLinesStayOneLine(t *testing.T) {
	input := buildEmailInput(ParsedEmail{
		From:     "x\nDate: forged",
		Subject:  "line1\r\nline2\nBody: forged",
		TextBody: "text",
		Date:     time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
	})
	lines := strings.Split(input, "\n")
	if lines[0] != "From: x Date: forged" || lines[1] != "Subject: line1 line2 Body: forged" || lines[2] != "Date: 2026-01-02T03:04:05Z" {
		t.Errorf("header lines = %q", lines[:3])
	}
	long := buildEmailInput(ParsedEmail{Subject: strings.Repeat("s", 1000)})
	if got := len(strings.Split(long, "\n")[1]); got != len("Subject: ")+maxEmailHeaderRunes {
		t.Errorf("subject line length = %d", got)
	}
}

func TestBuildEmailInput_FallsBackToHTMLAndCollapsesBlankRuns(t *testing.T) {
	input := buildEmailInput(ParsedEmail{HTMLBody: "<p>one</p><style>x{}</style><p>two</p>"})
	if !strings.HasSuffix(input, "Body:\none\ntwo") {
		t.Errorf("input = %q", input)
	}
	input = buildEmailInput(ParsedEmail{TextBody: "a\r\n\r\n\r\n\r\nb"})
	if !strings.HasSuffix(input, "Body:\na\n\nb") {
		t.Errorf("input = %q", input)
	}
}

func TestExtractOrder_Request(t *testing.T) {
	fake := llmtest.New(llmtest.JSON(emailOrderExtraction{IsOrder: true, TotalMinor: 100, Currency: "EUR"}))
	svc := NewEmailService(nil, fake, nil)
	parsed := ParsedEmail{From: "shop@example.com", Subject: "Order", TextBody: "body"}

	got, err := svc.extractOrder(t.Context(), parsed)
	if err != nil || !got.IsOrder || got.TotalMinor != 100 {
		t.Fatalf("got %+v, %v", got, err)
	}
	reqs := fake.Requests()
	if len(reqs) != 1 {
		t.Fatalf("requests = %d, want 1", len(reqs))
	}
	req := reqs[0]
	if req.Operation != "ledger.email_order" || req.System == "" || req.Schema.Name != "emailOrderExtraction" || req.Input != buildEmailInput(parsed) {
		t.Errorf("request = %+v", req)
	}
}

func TestExtractOrder_Errors(t *testing.T) {
	svc := NewEmailService(nil, &llmtest.Fake{Disabled: true}, nil)
	if _, err := svc.extractOrder(t.Context(), ParsedEmail{}); !errors.Is(err, llm.ErrDisabled) {
		t.Errorf("disabled: err = %v", err)
	}
	svc = NewEmailService(nil, llmtest.New(llmtest.Response{Output: []byte(`{"isOrder":`)}), nil)
	if _, err := svc.extractOrder(t.Context(), ParsedEmail{}); !errors.Is(err, llm.ErrInvalidOutput) {
		t.Errorf("invalid json: err = %v", err)
	}
}

func TestEmailOrderSchema(t *testing.T) {
	schema := string(emailOrderSchema.Definition)
	for _, want := range []string{
		"isOrder", "merchant", "paymentMethod", "externalOrderId", "orderDate", "totalMinor", "currency", "items",
		"name", "quantity", "unitPriceMinor", "totalPriceMinor",
		"paypal", "card", "direct_debit", "bank_transfer", "other", "unknown",
	} {
		if !strings.Contains(schema, `"`+want+`"`) {
			t.Errorf("schema lacks %q", want)
		}
	}
}

func TestValidateExtraction(t *testing.T) {
	header := time.Date(2026, 5, 6, 23, 0, 0, 0, time.FixedZone("x", 2*3600))
	valid := emailOrderExtraction{
		IsOrder: true, Merchant: "Acme", PaymentMethod: "card", ExternalOrderID: " 42 ", OrderDate: "2026-04-08",
		TotalMinor: 1249, Currency: "EUR",
		Items: []emailOrderItemExtraction{{Name: "Thing", Quantity: 2, UnitPriceMinor: 500, TotalPriceMinor: 1000}},
	}
	with := func(f func(*emailOrderExtraction)) emailOrderExtraction {
		e := valid
		f(&e)
		return e
	}
	tests := []struct {
		name    string
		in      emailOrderExtraction
		header  time.Time
		wantErr bool
		check   func(*testing.T, extractedOrder)
	}{
		{name: "valid", in: valid, check: func(t *testing.T, o extractedOrder) {
			if o.Merchant != "Acme" || o.PaymentMethod != "card" || o.ExternalOrderID != "42" || o.OrderDate != "2026-04-08" || o.TotalMinor != 1249 || o.Currency != "EUR" || len(o.Items) != 1 {
				t.Errorf("order = %+v", o)
			}
		}},
		{name: "zero total", in: with(func(e *emailOrderExtraction) { e.TotalMinor = 0 }), wantErr: true},
		{name: "negative total", in: with(func(e *emailOrderExtraction) { e.TotalMinor = -5 }), wantErr: true},
		{name: "huge total", in: with(func(e *emailOrderExtraction) { e.TotalMinor = 1e12 }), wantErr: true},
		{name: "lower-case currency", in: with(func(e *emailOrderExtraction) { e.Currency = " eur " }), check: func(t *testing.T, o extractedOrder) {
			if o.Currency != "EUR" {
				t.Errorf("currency = %q", o.Currency)
			}
		}},
		{name: "euro sign", in: with(func(e *emailOrderExtraction) { e.Currency = "€" }), check: func(t *testing.T, o extractedOrder) {
			if o.Currency != "EUR" {
				t.Errorf("currency = %q", o.Currency)
			}
		}},
		{name: "dollar sign", in: with(func(e *emailOrderExtraction) { e.Currency = "$" }), check: func(t *testing.T, o extractedOrder) {
			if o.Currency != "USD" {
				t.Errorf("currency = %q", o.Currency)
			}
		}},
		{name: "currency word", in: with(func(e *emailOrderExtraction) { e.Currency = "Euro" }), wantErr: true},
		{name: "empty currency", in: with(func(e *emailOrderExtraction) { e.Currency = "" }), wantErr: true},
		{name: "empty date falls back to header", in: with(func(e *emailOrderExtraction) { e.OrderDate = "" }), header: header, check: func(t *testing.T, o extractedOrder) {
			if o.OrderDate != "2026-05-06" {
				t.Errorf("date = %q (header date is taken in UTC)", o.OrderDate)
			}
		}},
		{name: "empty date without header", in: with(func(e *emailOrderExtraction) { e.OrderDate = "" }), check: func(t *testing.T, o extractedOrder) {
			if o.OrderDate != "" {
				t.Errorf("date = %q", o.OrderDate)
			}
		}},
		{name: "german date is invalid", in: with(func(e *emailOrderExtraction) { e.OrderDate = "08.04.2026" }), header: header, check: func(t *testing.T, o extractedOrder) {
			if o.OrderDate != "2026-05-06" {
				t.Errorf("date = %q", o.OrderDate)
			}
		}},
		{name: "unknown payment method", in: with(func(e *emailOrderExtraction) { e.PaymentMethod = "Kreditkarte" }), check: func(t *testing.T, o extractedOrder) {
			if o.PaymentMethod != "unknown" {
				t.Errorf("method = %q", o.PaymentMethod)
			}
		}},
		{name: "payment method is normalised", in: with(func(e *emailOrderExtraction) { e.PaymentMethod = " PayPal " }), check: func(t *testing.T, o extractedOrder) {
			if o.PaymentMethod != "paypal" {
				t.Errorf("method = %q", o.PaymentMethod)
			}
		}},
		{name: "merchant is capped and collapsed", in: with(func(e *emailOrderExtraction) { e.Merchant = " a  b" + strings.Repeat("x", 300) }), check: func(t *testing.T, o extractedOrder) {
			if utf8.RuneCountInString(o.Merchant) != maxMerchantRunes || !strings.HasPrefix(o.Merchant, "a bxx") {
				t.Errorf("merchant = %q", o.Merchant)
			}
		}},
		{name: "items are normalised", in: with(func(e *emailOrderExtraction) {
			e.Items = []emailOrderItemExtraction{
				{Name: "no qty", Quantity: 0, UnitPriceMinor: 300},
				{Name: "negative", Quantity: 1, UnitPriceMinor: -5, TotalPriceMinor: -9},
				{Name: "  ", Quantity: 1},
				{Name: "multi", Quantity: 3, UnitPriceMinor: 100},
			}
		}), check: func(t *testing.T, o extractedOrder) {
			want := []LedgerEmailOrderItem{
				{Name: "no qty", Quantity: 1, UnitPriceMinor: 300, TotalPriceMinor: 300},
				{Name: "negative", Quantity: 1},
				{Name: "multi", Quantity: 3, UnitPriceMinor: 100, TotalPriceMinor: 300},
			}
			if len(o.Items) != len(want) {
				t.Fatalf("items = %+v", o.Items)
			}
			for i := range want {
				if o.Items[i] != want[i] {
					t.Errorf("item %d = %+v, want %+v", i, o.Items[i], want[i])
				}
			}
		}},
		{name: "no items", in: with(func(e *emailOrderExtraction) { e.Items = nil }), check: func(t *testing.T, o extractedOrder) {
			if o.Items == nil || len(o.Items) != 0 {
				t.Errorf("items = %#v, want empty non-nil", o.Items)
			}
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := validateExtraction(tt.in, tt.header)
			if tt.wantErr {
				if !errors.Is(err, errInvalidExtraction) {
					t.Fatalf("err = %v, want errInvalidExtraction", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("err = %v", err)
			}
			tt.check(t, got)
		})
	}
}

func TestValidateExtraction_CapsItemCount(t *testing.T) {
	items := make([]emailOrderItemExtraction, maxItems+50)
	for i := range items {
		items[i] = emailOrderItemExtraction{Name: "x", Quantity: 1}
	}
	got, err := validateExtraction(emailOrderExtraction{TotalMinor: 1, Currency: "EUR", Items: items}, time.Time{})
	if err != nil || len(got.Items) != maxItems {
		t.Fatalf("items = %d, err = %v", len(got.Items), err)
	}
}
