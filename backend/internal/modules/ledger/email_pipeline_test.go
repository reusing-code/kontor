package ledger

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/reusing-code/kontor/backend/internal/llm"
	"github.com/reusing-code/kontor/backend/internal/llm/llmtest"
)

func fixturePath(name string) string {
	return filepath.Join("..", "..", "..", "..", "docs", "design", "email", name)
}

func parseFixtureEmail(t *testing.T, name string) ParsedEmail {
	t.Helper()
	file, err := os.Open(fixturePath(name))
	if err != nil {
		t.Fatalf("open fixture %s: %v", name, err)
	}
	defer file.Close()
	parsed, err := parseMessage(file)
	if err != nil {
		t.Fatalf("parse fixture %s: %v", name, err)
	}
	return parsed
}

func TestParseHeader_MatchesParseMessage(t *testing.T) {
	for _, name := range []string{"AmazonDE-1.eml", "PaypalDE-2.eml"} {
		t.Run(name, func(t *testing.T) {
			raw, err := os.ReadFile(fixturePath(name))
			if err != nil {
				t.Fatalf("read fixture: %v", err)
			}
			full := parseFixtureEmail(t, name)
			header, err := parseHeader(headerBlock(raw))
			if err != nil {
				t.Fatalf("parseHeader: %v", err)
			}
			if header.MessageID != full.MessageID || header.From != full.From || header.Subject != full.Subject || !header.Date.Equal(full.Date) {
				t.Errorf("header = %+v, full = %+v", header, full)
			}
			if header.MessageID == "" || header.Date.IsZero() {
				t.Errorf("fixture header incomplete: %+v", header)
			}
			if header.TextBody != "" || header.HTMLBody != "" {
				t.Error("parseHeader returned a body")
			}
		})
	}
}

func TestParseHeader_ErrorIsFixedText(t *testing.T) {
	_, err := parseHeader([]byte("this is not a header SECRET-SUBJECT"))
	if err == nil {
		t.Fatal("want error")
	}
	if strings.Contains(err.Error(), "SECRET") {
		t.Errorf("error quotes input: %v", err)
	}
}

func TestHeaderBlock(t *testing.T) {
	tests := []struct{ name, in, want string }{
		{"crlf", "A: 1\r\nB: 2\r\n\r\nbody\r\n", "A: 1\r\nB: 2"},
		{"lf", "A: 1\nB: 2\n\nbody\n", "A: 1\nB: 2"},
		{"header only", "A: 1\r\nB: 2\r\n\r\n", "A: 1\r\nB: 2"},
		{"no blank line", "A: 1\r\nB: 2\r\n", "A: 1\r\nB: 2"},
		{"blank in body kept out", "A: 1\n\nx\n\ny", "A: 1"},
	}
	for _, tt := range tests {
		if got := string(headerBlock([]byte(tt.in))); got != tt.want {
			t.Errorf("%s: headerBlock = %q, want %q", tt.name, got, tt.want)
		}
	}
}

func TestEmailDedupKey(t *testing.T) {
	if got := emailDedupKey("  <id@x>  ", []byte("ignored")); got != "<id@x>" {
		t.Errorf("message id key = %q", got)
	}
	crlf := emailDedupKey("", []byte("From: a\r\nSubject: s\r\n"))
	lf := emailDedupKey("", []byte("From: a\nSubject: s"))
	other := emailDedupKey("", []byte("From: b\nSubject: s"))
	if !strings.HasPrefix(crlf, "nomid:") || len(crlf) != len("nomid:")+32 {
		t.Errorf("fallback key = %q", crlf)
	}
	if crlf != lf {
		t.Errorf("CRLF and LF variants differ: %q vs %q", crlf, lf)
	}
	if crlf == other {
		t.Error("different headers share a key")
	}
}

type pipelineTest struct {
	svc     *EmailService
	store   *testStores
	fake    *llmtest.Fake
	account LedgerEmailAccount
	logs    *bytes.Buffer
}

func newPipelineTest(t *testing.T, fake *llmtest.Fake) *pipelineTest {
	t.Helper()
	s := newTestStore(t)
	now := time.Now().UTC()
	account := LedgerEmailAccount{
		ID: uuid.New(), Name: "Mailbox", IMAPHost: "imap.invalid", IMAPPort: 993, Username: "u",
		Folder: "Orders", CreatedAt: now, UpdatedAt: now,
	}
	if err := s.CreateLedgerEmailAccount(context.Background(), testUserID, account); err != nil {
		t.Fatalf("CreateLedgerEmailAccount: %v", err)
	}
	logs := &bytes.Buffer{}
	logger := slog.New(slog.NewTextHandler(logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	return &pipelineTest{svc: NewEmailService(s.Store, fake, logger), store: s, fake: fake, account: account, logs: logs}
}

func (p *pipelineTest) upload(t *testing.T, names ...string) (LedgerEmailScanResult, error) {
	t.Helper()
	files := make([]UploadedMessage, 0, len(names))
	for _, name := range names {
		raw, err := os.ReadFile(fixturePath(name))
		if err != nil {
			t.Fatalf("read fixture: %v", err)
		}
		files = append(files, UploadedMessage{Filename: name, Reader: bytes.NewReader(raw)})
	}
	return p.svc.ScanUploadedMessages(context.Background(), testUserID, p.account, files)
}

// imapMessage builds an emailMessage the way ScanMailbox does: the header
// fields come from the header block only, Load returns the whole message.
func imapMessage(t *testing.T, name string, uid int) emailMessage {
	t.Helper()
	raw, err := os.ReadFile(fixturePath(name))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	header, err := parseHeader(raw)
	if err != nil {
		t.Fatalf("parseHeader: %v", err)
	}
	full := parseFixtureEmail(t, name)
	return emailMessage{
		Label:  fmt.Sprintf("uid %d", uid),
		Key:    emailDedupKey(header.MessageID, headerBlock(raw)),
		Header: header,
		Load:   func() (ParsedEmail, error) { return full, nil },
	}
}

func (p *pipelineTest) process(ctx context.Context, msgs ...emailMessage) (LedgerEmailScanResult, error) {
	result := newScanResult()
	err := p.svc.processMessages(ctx, testUserID, p.account, msgs, false, &result)
	return result, err
}

func (p *pipelineTest) orders(t *testing.T) []LedgerEmailOrder {
	t.Helper()
	orders, err := p.store.ListLedgerEmailOrders(context.Background(), testUserID)
	if err != nil {
		t.Fatalf("ListLedgerEmailOrders: %v", err)
	}
	return orders
}

func (p *pipelineTest) seen(t *testing.T, key string) bool {
	t.Helper()
	seen, err := p.store.HasLedgerEmailSeen(context.Background(), testUserID, p.account.ID, key)
	if err != nil {
		t.Fatalf("HasLedgerEmailSeen: %v", err)
	}
	return seen
}

type goldenOrder struct {
	fixture    string
	orderID    string
	totalMinor int64
	currency   string
	date       string
	merchant   string
	payment    string
	items      []string // item names
}

func amazonItems(first string, count int) []string {
	items := []string{first}
	for i := 2; i <= count; i++ {
		items = append(items, fmt.Sprintf("Beispielartikel %d", i))
	}
	return items
}

var goldenOrders = []goldenOrder{
	{"AmazonDE-1.eml", "304-5184726-4839201", 2140, "EUR", "2026-04-08", "Amazon.de", PaymentMethodCard, amazonItems("Beispielartikel Elektronikzubehoer", 1)},
	{"AmazonDE-2.eml", "304-6418207-5931846", 1648, "EUR", "2026-03-18", "Amazon.de", PaymentMethodCard, amazonItems("Beispielartikel Elektronikzubehoer A", 2)},
	{"AmazonDE-3.eml", "302-7413580-6841293", 4827, "EUR", "2026-02-26", "Amazon.de", PaymentMethodCard, amazonItems("Beispielartikel Pflegegeraet Kompakt", 3)},
	{"AmazonDE-4.eml", "304-6821945-4173086", 4156, "EUR", "2026-01-26", "Amazon.de", PaymentMethodCard, amazonItems("Beispielartikel Elektronikzubehoer A", 4)},
	{"AmazonDE-6.eml", "304-5603418-7281649", 34291, "EUR", "2026-02-13", "Amazon.de", PaymentMethodCard, amazonItems("Beispielartikel Kleidung", 16)},
	{"PaypalDE-1.eml", "EU-DE5827314", 1249, "EUR", "2026-04-06", "Streaming Service G...", PaymentMethodPayPal, []string{"Streaming Service G..."}},
	{"PaypalDE-2.eml", "512904683771285406", 349, "EUR", "2026-03-28", "www.digitalmarket.example", PaymentMethodPayPal, []string{"www.digitalmarket.example"}},
	{"PaypalDE-3.eml", "8JX72041NC563984Q", 847, "USD", "2026-03-07", "CodeHost, Inc.", PaymentMethodPayPal, []string{"CodeHost, Inc."}},
}

func (g goldenOrder) extraction() emailOrderExtraction {
	e := emailOrderExtraction{
		IsOrder: true, Merchant: g.merchant, PaymentMethod: g.payment, ExternalOrderID: g.orderID,
		OrderDate: g.date, TotalMinor: g.totalMinor, Currency: g.currency,
	}
	for _, name := range g.items {
		item := emailOrderItemExtraction{Name: name, Quantity: 1}
		if len(g.items) == 1 {
			item.UnitPriceMinor, item.TotalPriceMinor = g.totalMinor, g.totalMinor
		}
		e.Items = append(e.Items, item)
	}
	return e
}

func TestPipeline_FixtureOrders(t *testing.T) {
	for _, g := range goldenOrders {
		t.Run(g.fixture, func(t *testing.T) {
			p := newPipelineTest(t, llmtest.New(llmtest.JSON(g.extraction())))
			real := parseFixtureEmail(t, g.fixture)

			result, err := p.upload(t, g.fixture)
			if err != nil {
				t.Fatalf("scan: %v", err)
			}
			if len(result.Warnings) != 0 || result.EmailsScanned != 1 || result.OrdersNew != 1 || result.OrdersFound != 1 || len(result.Orders) != 1 {
				t.Fatalf("result = %+v", result)
			}
			orders := p.orders(t)
			if len(orders) != 1 {
				t.Fatalf("stored orders = %d, want 1", len(orders))
			}
			o := orders[0]
			if o.ImporterID != "llm" || o.Merchant != g.merchant || o.PaymentMethod != g.payment ||
				o.ExternalOrderID != g.orderID || o.OrderDate != g.date || o.TotalMinor != g.totalMinor || o.Currency != g.currency {
				t.Errorf("order = %+v", o)
			}
			if o.EmailSubject != real.Subject || o.EmailMessageID != real.MessageID || o.EmailAccountID != p.account.ID {
				t.Errorf("subject/message id/account = %q / %q / %s", o.EmailSubject, o.EmailMessageID, o.EmailAccountID)
			}
			if len(o.Items) != len(g.items) || o.Items[0].Name != g.items[0] {
				t.Errorf("items = %+v, want %d starting with %q", o.Items, len(g.items), g.items[0])
			}
			if len(g.items) == 1 && o.Items[0].TotalPriceMinor != g.totalMinor {
				t.Errorf("item total = %d, want %d", o.Items[0].TotalPriceMinor, g.totalMinor)
			}

			reqs := p.fake.Requests()
			if len(reqs) != 1 {
				t.Fatalf("LLM requests = %d, want 1", len(reqs))
			}
			if reqs[0].Operation != "ledger.email_order" {
				t.Errorf("operation = %q", reqs[0].Operation)
			}
			if !strings.Contains(reqs[0].Input, g.orderID) {
				t.Errorf("input lacks the order id text %q", g.orderID)
			}
			noise := noiseHeaderLines(t, g.fixture)
			if len(noise) == 0 {
				t.Fatal("fixture has no noise header lines to check")
			}
			for _, line := range noise {
				if strings.Contains(reqs[0].Input, line) {
					t.Errorf("input contains header line %q", line)
				}
			}

			again, err := p.upload(t, g.fixture)
			if err != nil {
				t.Fatalf("second scan: %v", err)
			}
			if again.OrdersNew != 0 || again.OrdersFound != 1 || len(again.Orders) != 1 || again.Orders[0].ID != o.ID {
				t.Errorf("second result = %+v", again)
			}
			if len(p.orders(t)) != 1 {
				t.Error("second scan created a duplicate order")
			}
			if got := len(p.fake.Requests()); got != 1 {
				t.Errorf("LLM requests after second scan = %d, want 1", got)
			}
		})
	}
}

func TestPipeline_FixtureOrders_IMAPShapedPath(t *testing.T) {
	for i, g := range goldenOrders {
		t.Run(g.fixture, func(t *testing.T) {
			p := newPipelineTest(t, llmtest.New(llmtest.JSON(g.extraction())))
			msg := imapMessage(t, g.fixture, i+1)

			result, err := p.process(context.Background(), msg)
			if err != nil || result.OrdersNew != 1 || len(result.Warnings) != 0 {
				t.Fatalf("result = %+v, err = %v", result, err)
			}
			o := p.orders(t)[0]
			real := parseFixtureEmail(t, g.fixture)
			if o.ExternalOrderID != g.orderID || o.TotalMinor != g.totalMinor || o.Currency != g.currency || o.EmailMessageID != real.MessageID || o.EmailSubject != real.Subject {
				t.Errorf("order = %+v", o)
			}

			again, err := p.process(context.Background(), imapMessage(t, g.fixture, i+1))
			if err != nil || again.OrdersNew != 0 || again.OrdersFound != 1 {
				t.Fatalf("second result = %+v, err = %v", again, err)
			}
			if got := len(p.fake.Requests()); got != 1 {
				t.Errorf("LLM requests = %d, want 1", got)
			}
		})
	}
}

func TestPipeline_UploadAndIMAPShareDedupKey(t *testing.T) {
	p := newPipelineTest(t, llmtest.New(llmtest.JSON(goldenOrders[0].extraction())))
	if _, err := p.upload(t, "AmazonDE-1.eml"); err != nil {
		t.Fatalf("upload: %v", err)
	}
	result, err := p.process(context.Background(), imapMessage(t, "AmazonDE-1.eml", 9))
	if err != nil || result.OrdersNew != 0 || result.OrdersFound != 1 || len(p.fake.Requests()) != 1 {
		t.Fatalf("result = %+v, err = %v, requests = %d", result, err, len(p.fake.Requests()))
	}
}

func TestPipeline_SameMessageTwiceInOneBatchIsExtractedOnce(t *testing.T) {
	p := newPipelineTest(t, llmtest.New(llmtest.JSON(goldenOrders[0].extraction())))
	result, err := p.upload(t, "AmazonDE-1.eml", "AmazonDE-1.eml")
	if err != nil || result.OrdersNew != 1 || len(p.fake.Requests()) != 1 || len(p.orders(t)) != 1 {
		t.Fatalf("result = %+v, err = %v, requests = %d", result, err, len(p.fake.Requests()))
	}
}

func TestPipeline_NotAnOrderIsRememberedAndNotRetried(t *testing.T) {
	notOrder := llmtest.JSON(emailOrderExtraction{IsOrder: false})
	t.Run("upload", func(t *testing.T) {
		p := newPipelineTest(t, llmtest.New(notOrder))
		result, err := p.upload(t, "AmazonDE-1.eml")
		if err != nil || result.OrdersNew != 0 || result.OrdersFound != 0 || len(result.Warnings) != 0 || len(p.orders(t)) != 0 {
			t.Fatalf("result = %+v, err = %v", result, err)
		}
		if !p.seen(t, parseFixtureEmail(t, "AmazonDE-1.eml").MessageID) {
			t.Error("message not marked as processed")
		}
		if _, err := p.upload(t, "AmazonDE-1.eml"); err != nil {
			t.Fatalf("second scan: %v", err)
		}
		if got := len(p.fake.Requests()); got != 1 {
			t.Errorf("LLM requests = %d, want 1", got)
		}
	})
	t.Run("imap", func(t *testing.T) {
		p := newPipelineTest(t, llmtest.New(notOrder))
		msg := imapMessage(t, "AmazonDE-2.eml", 1)
		for range 2 {
			result, err := p.process(context.Background(), msg)
			if err != nil || len(result.Warnings) != 0 || result.OrdersNew != 0 {
				t.Fatalf("result = %+v, err = %v", result, err)
			}
		}
		if got := len(p.fake.Requests()); got != 1 {
			t.Errorf("LLM requests = %d, want 1", got)
		}
	})
}

func TestPipeline_InvalidOutputWarnsAndIsRetried(t *testing.T) {
	tests := []struct {
		name   string
		answer llmtest.Response
	}{
		{"zero total", llmtest.JSON(emailOrderExtraction{IsOrder: true, Currency: "EUR", Merchant: "X"})},
		{"invalid output error", llmtest.Error(llm.ErrInvalidOutput)},
		{"refused", llmtest.Error(llm.ErrRefused)},
		{"bad request", llmtest.Error(&llm.APIError{StatusCode: 400})},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := goldenOrders[0]
			p := newPipelineTest(t, llmtest.New(tt.answer, llmtest.JSON(g.extraction())))
			key := parseFixtureEmail(t, g.fixture).MessageID

			result, err := p.upload(t, g.fixture)
			if err != nil {
				t.Fatalf("scan: %v", err)
			}
			if len(result.Warnings) != 1 || !strings.HasPrefix(result.Warnings[0], g.fixture+": ") || result.OrdersNew != 0 || len(p.orders(t)) != 0 {
				t.Fatalf("result = %+v", result)
			}
			if p.seen(t, key) {
				t.Error("failed message was marked as processed")
			}

			retry, err := p.upload(t, g.fixture)
			if err != nil || retry.OrdersNew != 1 || len(p.fake.Requests()) != 2 {
				t.Fatalf("retry = %+v, err = %v, requests = %d", retry, err, len(p.fake.Requests()))
			}
		})
	}
}

func TestPipeline_RepeatedTransientFailuresStopTheScan(t *testing.T) {
	down := llmtest.Error(&llm.APIError{StatusCode: 503})
	p := newPipelineTest(t, llmtest.New(down, down, down))
	msgs := []emailMessage{
		imapMessage(t, "AmazonDE-1.eml", 1), imapMessage(t, "AmazonDE-2.eml", 2),
		imapMessage(t, "AmazonDE-3.eml", 3), imapMessage(t, "AmazonDE-4.eml", 4),
	}
	result, err := p.process(context.Background(), msgs...)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if !result.Partial || len(p.fake.Requests()) != 3 || len(p.orders(t)) != 0 {
		t.Fatalf("partial = %v, requests = %d", result.Partial, len(p.fake.Requests()))
	}
	if !strings.Contains(strings.Join(result.Warnings, "\n"), "scan stopped after repeated LLM failures") {
		t.Errorf("warnings = %v", result.Warnings)
	}
	for _, m := range msgs {
		if p.seen(t, m.Key) {
			t.Error("transiently failed message was marked as processed")
		}
	}
}

func TestPipeline_SuccessResetsTheFailureCounter(t *testing.T) {
	down := llmtest.Error(&llm.APIError{StatusCode: 503})
	ok := func(i int) llmtest.Response { return llmtest.JSON(goldenOrders[i].extraction()) }
	p := newPipelineTest(t, llmtest.New(down, ok(1), down, down, ok(4)))
	msgs := []emailMessage{
		imapMessage(t, "AmazonDE-1.eml", 1), imapMessage(t, "AmazonDE-2.eml", 2), imapMessage(t, "AmazonDE-3.eml", 3),
		imapMessage(t, "AmazonDE-4.eml", 4), imapMessage(t, "AmazonDE-6.eml", 5),
	}
	result, err := p.process(context.Background(), msgs...)
	if err != nil || result.Partial || len(p.fake.Requests()) != 5 || result.OrdersNew != 2 {
		t.Fatalf("result = %+v, err = %v, requests = %d", result, err, len(p.fake.Requests()))
	}
}

func TestPipeline_CancellationStopsAndKeepsOrders(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	calls := 0
	fake := &llmtest.Fake{Func: func(llm.Request) (json.RawMessage, error) {
		calls++
		if calls == 2 {
			cancel()
			return nil, context.Canceled
		}
		return json.Marshal(goldenOrders[0].extraction())
	}}
	p := newPipelineTest(t, fake)
	result, err := p.process(ctx, imapMessage(t, "AmazonDE-1.eml", 1), imapMessage(t, "AmazonDE-2.eml", 2), imapMessage(t, "AmazonDE-3.eml", 3))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	if !result.Partial || result.OrdersNew != 1 || len(p.orders(t)) != 1 || len(fake.Requests()) != 2 {
		t.Fatalf("result = %+v, orders = %d, requests = %d", result, len(p.orders(t)), len(fake.Requests()))
	}
}

func TestPipeline_LLMDisabled(t *testing.T) {
	p := newPipelineTest(t, &llmtest.Fake{Disabled: true})
	ctx := context.Background()

	result, err := p.upload(t, "AmazonDE-1.eml")
	if !errors.Is(err, ErrEmailLLMDisabled) || result.OrdersNew != 0 || len(p.orders(t)) != 0 {
		t.Fatalf("upload: result = %+v, err = %v", result, err)
	}
	if p.seen(t, parseFixtureEmail(t, "AmazonDE-1.eml").MessageID) {
		t.Error("message was marked as processed")
	}

	result, err = p.svc.ScanMailbox(ctx, testUserID, p.account, "pw")
	if !errors.Is(err, ErrEmailLLMDisabled) || errors.Is(err, ErrIMAP) {
		t.Fatalf("mailbox: err = %v, want ErrEmailLLMDisabled without dialing", err)
	}
	if len(result.Orders) != 0 || len(p.fake.Requests()) != 0 {
		t.Errorf("orders = %d, requests = %d", len(result.Orders), len(p.fake.Requests()))
	}
}

func TestPipeline_MailboxWithoutFolderDoesNotDial(t *testing.T) {
	p := newPipelineTest(t, llmtest.New())
	account := p.account
	account.Folder = ""
	_, err := p.svc.ScanMailbox(context.Background(), testUserID, account, "pw")
	if !errors.Is(err, ErrEmailFolderMissing) || errors.Is(err, ErrIMAP) {
		t.Fatalf("err = %v, want ErrEmailFolderMissing", err)
	}
}

func TestPipeline_MessageWithoutMessageIDDedupsOnHeaderHash(t *testing.T) {
	eml := "From: shop@example.com\r\nSubject: Your order\r\nDate: Wed, 8 Apr 2026 18:50:24 +0000\r\n\r\nOrder 4711 total 12,00 EUR\r\n"
	fake := llmtest.New(llmtest.JSON(emailOrderExtraction{IsOrder: true, Merchant: "Shop", PaymentMethod: "card", ExternalOrderID: "4711", OrderDate: "2026-04-08", TotalMinor: 1200, Currency: "EUR"}))
	p := newPipelineTest(t, fake)
	scan := func(filename string) LedgerEmailScanResult {
		result, err := p.svc.ScanUploadedMessages(context.Background(), testUserID, p.account,
			[]UploadedMessage{{Filename: filename, Reader: strings.NewReader(eml)}})
		if err != nil {
			t.Fatalf("scan: %v", err)
		}
		return result
	}
	first := scan("a.eml")
	second := scan("renamed.eml")
	if first.OrdersNew != 1 || second.OrdersNew != 0 || second.OrdersFound != 1 || len(fake.Requests()) != 1 {
		t.Fatalf("first = %+v, second = %+v, requests = %d", first, second, len(fake.Requests()))
	}
	if key := p.orders(t)[0].EmailMessageID; !strings.HasPrefix(key, "nomid:") {
		t.Errorf("EmailMessageID = %q, want nomid: prefix", key)
	}
}

func TestPipeline_UnparseableFileWarnsWithoutQuotingContent(t *testing.T) {
	p := newPipelineTest(t, llmtest.New())
	result, err := p.svc.ScanUploadedMessages(context.Background(), testUserID, p.account,
		[]UploadedMessage{{Filename: "broken.eml", Reader: strings.NewReader("SECRET-CONTENT without headers")}})
	if err != nil || len(result.Warnings) != 1 || result.Warnings[0] != "broken.eml: could not parse message" {
		t.Fatalf("result = %+v, err = %v", result, err)
	}
	if len(p.fake.Requests()) != 0 {
		t.Error("unparseable message reached the LLM")
	}
}

func TestPipeline_AutoLink(t *testing.T) {
	type txn struct {
		booking, counterparty string
		amount                int64
	}
	generic := goldenOrder{"AmazonDE-2.eml", "ACME-1", 1648, "EUR", "2026-03-18", "Acme GmbH", PaymentMethodCard, []string{"Widget"}}
	tests := []struct {
		name       string
		order      goldenOrder
		txn        txn
		wantLinked bool
	}{
		{"paypal-paid order", goldenOrders[5], txn{"2026-04-08", "PayPal (Europe) S.a.r.l.", -1249}, true},
		{"paypal-paid order outside the 7 day window", goldenOrders[5], txn{"2026-04-20", "PayPal (Europe) S.a.r.l.", -1249}, false},
		{"directly paid amazon order within 60 days", goldenOrders[0], txn{"2026-04-20", "AMAZON PAYMENTS EUROPE", -2140}, true},
		{"directly paid amazon order after 60 days", goldenOrders[0], txn{"2026-07-01", "AMAZON PAYMENTS EUROPE", -2140}, false},
		{"generic merchant uses the merchant name", generic, txn{"2026-04-10", "ACME GMBH", -1648}, true},
		{"generic merchant, other counterparty", generic, txn{"2026-04-10", "SOMEONE ELSE", -1648}, false},
		{"usd order is not converted", goldenOrders[7], txn{"2026-03-08", "PAYPAL EUROPE", -780}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := newPipelineTest(t, llmtest.New(llmtest.JSON(tt.order.extraction())))
			accountID := addAccount(t, p.store, "Card")
			txnID := uuid.New()
			commitTransactions(t, p.store, accountID, LedgerTransaction{
				ID: txnID, AccountID: accountID, BookingDate: tt.txn.booking, AmountMinor: tt.txn.amount,
				Currency: "EUR", CounterpartyName: tt.txn.counterparty,
			})
			result, err := p.upload(t, tt.order.fixture)
			if err != nil || result.OrdersNew != 1 {
				t.Fatalf("result = %+v, err = %v", result, err)
			}
			order := p.orders(t)[0]
			if tt.wantLinked {
				if result.OrdersLinked != 1 || order.MatchStatus != LedgerEmailOrderStatusMatched || len(order.LinkedTransactionIDs) != 1 || order.LinkedTransactionIDs[0] != txnID {
					t.Fatalf("order = %+v, result = %+v", order, result)
				}
				stored, err := p.store.GetLedgerTransaction(context.Background(), testUserID, txnID)
				if err != nil || len(stored.EmailOrderIDs) != 1 || stored.EmailOrderIDs[0] != order.ID {
					t.Errorf("transaction order links = %v, err = %v", stored.EmailOrderIDs, err)
				}
				return
			}
			if result.OrdersLinked != 0 || order.MatchStatus != LedgerEmailOrderStatusUnmatched || len(order.LinkedTransactionIDs) != 0 {
				t.Fatalf("order = %+v, result = %+v", order, result)
			}
		})
	}
}

func TestPipeline_OrderWithoutMerchantIsNotAutoLinked(t *testing.T) {
	g := goldenOrder{"AmazonDE-1.eml", "X-1", 2140, "EUR", "2026-04-08", "", PaymentMethodCard, nil}
	p := newPipelineTest(t, llmtest.New(llmtest.JSON(g.extraction())))
	accountID := addAccount(t, p.store, "Card")
	commitTransactions(t, p.store, accountID, LedgerTransaction{
		ID: uuid.New(), AccountID: accountID, BookingDate: "2026-04-09", AmountMinor: -2140, Currency: "EUR", CounterpartyName: "AMAZON",
	})
	result, err := p.upload(t, g.fixture)
	if err != nil || result.OrdersNew != 1 || result.OrdersLinked != 0 {
		t.Fatalf("result = %+v, err = %v", result, err)
	}
}

func TestPipeline_LogsAndWarningsContainNoEmailData(t *testing.T) {
	g := goldenOrders[0]
	scripted := g.extraction()
	p := newPipelineTest(t, llmtest.New(
		llmtest.JSON(scripted),
		llmtest.JSON(emailOrderExtraction{IsOrder: false}),
		llmtest.JSON(emailOrderExtraction{IsOrder: true, Merchant: "SecretMerchant", ExternalOrderID: "SECRET-ID"}),
		llmtest.Error(&llm.APIError{StatusCode: 503, Message: "SECRET-UPSTREAM"}),
		llmtest.Error(llm.ErrInvalidOutput),
	))
	accountID := addAccount(t, p.store, "Card")
	commitTransactions(t, p.store, accountID,
		LedgerTransaction{ID: uuid.New(), AccountID: accountID, BookingDate: "2026-04-09", AmountMinor: -2140, Currency: "EUR", CounterpartyName: "AMAZON"},
		LedgerTransaction{ID: uuid.New(), AccountID: accountID, BookingDate: "2026-04-10", AmountMinor: -2140, Currency: "EUR", CounterpartyName: "AMAZON"},
	)

	names := []string{"AmazonDE-1.eml", "AmazonDE-2.eml", "AmazonDE-3.eml", "AmazonDE-4.eml", "AmazonDE-6.eml"}
	result, err := p.upload(t, names...)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if len(result.Warnings) == 0 {
		t.Fatal("expected warnings")
	}

	output := p.logs.String() + "\n" + strings.Join(result.Warnings, "\n")
	secrets := []string{"SecretMerchant", "SECRET-ID", "SECRET-UPSTREAM", g.merchant, g.orderID, g.items[0], "Beispielartikel"}
	for _, name := range names {
		secrets = append(secrets, parseFixtureEmail(t, name).Subject, parseFixtureEmail(t, name).MessageID)
	}
	for _, secret := range secrets {
		if strings.Contains(output, secret) {
			t.Errorf("logs or warnings contain %q", secret)
		}
	}
}

func TestIsTransientLLMError(t *testing.T) {
	tests := []struct {
		err  error
		want bool
	}{
		{llm.ErrInvalidOutput, false},
		{fmt.Errorf("wrapped: %w", llm.ErrRefused), false},
		{llm.ErrTruncated, false},
		{&llm.APIError{StatusCode: 400}, false},
		{&llm.APIError{StatusCode: 413}, false},
		{&llm.APIError{StatusCode: 422}, false},
		{&llm.APIError{StatusCode: 408}, true},
		{&llm.APIError{StatusCode: 429}, true},
		{&llm.APIError{StatusCode: 401}, true},
		{&llm.APIError{StatusCode: 403}, true},
		{&llm.APIError{StatusCode: 500}, true},
		{&llm.APIError{StatusCode: 503}, true},
		{context.DeadlineExceeded, true},
		{errors.New("connection refused"), true},
	}
	for _, tt := range tests {
		if got := isTransientLLMError(tt.err); got != tt.want {
			t.Errorf("isTransientLLMError(%v) = %v, want %v", tt.err, got, tt.want)
		}
	}
}

func scanRequest(t *testing.T, mux http.Handler, accountID uuid.UUID, fixture string) *httptest.ResponseRecorder {
	t.Helper()
	raw, err := os.ReadFile(fixturePath(fixture))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	body, contentType := multipartBody(t, nil, "files", fixture, raw)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/ledger/email-accounts/"+accountID.String()+"/scan", body)
	req.Header.Set("Content-Type", contentType)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}

func TestScanHandler_UploadCreatesOrder(t *testing.T) {
	g := goldenOrders[0]
	fake := llmtest.New(llmtest.JSON(g.extraction()))
	h, s := newTestHandlerWithLLM(t, fake)
	now := time.Now().UTC()
	account := LedgerEmailAccount{ID: uuid.New(), Name: "M", IMAPHost: "h", IMAPPort: 993, Username: "u", CreatedAt: now, UpdatedAt: now}
	if err := s.CreateLedgerEmailAccount(context.Background(), testUserID, account); err != nil {
		t.Fatalf("CreateLedgerEmailAccount: %v", err)
	}

	rec := scanRequest(t, newMux(h), account.ID, g.fixture)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body: %s", rec.Code, rec.Body.String())
	}
	result := decodeJSON[LedgerEmailScanResult](t, rec)
	if result.OrdersNew != 1 || len(result.Orders) != 1 || result.Orders[0].ExternalOrderID != g.orderID {
		t.Errorf("result = %+v", result)
	}
}

func TestScanHandler_LLMDisabledIs409(t *testing.T) {
	h, s := newTestHandlerWithLLM(t, &llmtest.Fake{Disabled: true})
	now := time.Now().UTC()
	account := LedgerEmailAccount{ID: uuid.New(), Name: "M", IMAPHost: "h", IMAPPort: 993, Username: "u", Folder: "INBOX", EncryptedPassword: "x", CreatedAt: now, UpdatedAt: now}
	if err := s.CreateLedgerEmailAccount(context.Background(), testUserID, account); err != nil {
		t.Fatalf("CreateLedgerEmailAccount: %v", err)
	}
	mux := newMux(h)

	rec := scanRequest(t, mux, account.ID, "AmazonDE-1.eml")
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "the LLM is not configured") {
		t.Fatalf("upload: status = %d, body: %s", rec.Code, rec.Body.String())
	}
	imap := httptest.NewRecorder()
	mux.ServeHTTP(imap, httptest.NewRequest(http.MethodPost, "/api/v1/ledger/email-accounts/"+account.ID.String()+"/scan", nil))
	if imap.Code != http.StatusConflict || !strings.Contains(imap.Body.String(), "the LLM is not configured") {
		t.Fatalf("imap: status = %d, body: %s", imap.Code, imap.Body.String())
	}
	orders, _ := s.ListLedgerEmailOrders(context.Background(), testUserID)
	if len(orders) != 0 {
		t.Errorf("orders = %d, want 0", len(orders))
	}
}

func TestScan_OnlyOneScanPerAccountAtATime(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	fake := &llmtest.Fake{Func: func(llm.Request) (json.RawMessage, error) {
		close(entered)
		<-release
		return json.Marshal(goldenOrders[0].extraction())
	}}
	p := newPipelineTest(t, fake)

	done := make(chan error, 1)
	go func() {
		_, err := p.upload(t, "AmazonDE-1.eml")
		done <- err
	}()
	<-entered

	if _, err := p.upload(t, "AmazonDE-2.eml"); !errors.Is(err, ErrEmailScanRunning) {
		t.Fatalf("concurrent scan: err = %v, want ErrEmailScanRunning", err)
	}
	other := p.account
	other.ID = uuid.New()
	if unlock, ok := p.svc.lockAccount(other.ID); !ok {
		t.Error("another account is blocked")
	} else {
		unlock()
	}

	close(release)
	if err := <-done; err != nil {
		t.Fatalf("first scan: %v", err)
	}
	if unlock, ok := p.svc.lockAccount(p.account.ID); !ok {
		t.Error("lock not released after the scan")
	} else {
		unlock()
	}
}

func TestScanHandler_TimeoutReturnsPartialResult(t *testing.T) {
	old := emailScanTimeout
	emailScanTimeout = 200 * time.Millisecond
	t.Cleanup(func() { emailScanTimeout = old })

	fake := &llmtest.Fake{Func: func(llm.Request) (json.RawMessage, error) {
		time.Sleep(600 * time.Millisecond)
		return json.Marshal(goldenOrders[0].extraction())
	}}
	h, s := newTestHandlerWithLLM(t, fake)
	now := time.Now().UTC()
	account := LedgerEmailAccount{ID: uuid.New(), Name: "M", IMAPHost: "h", IMAPPort: 993, Username: "u", CreatedAt: now, UpdatedAt: now}
	if err := s.CreateLedgerEmailAccount(context.Background(), testUserID, account); err != nil {
		t.Fatalf("CreateLedgerEmailAccount: %v", err)
	}
	mux := newMux(h)
	raw1, _ := os.ReadFile(fixturePath("AmazonDE-1.eml"))
	raw2, _ := os.ReadFile(fixturePath("AmazonDE-2.eml"))
	body, contentType := multipartTwoFiles(t, raw1, raw2)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/ledger/email-accounts/"+account.ID.String()+"/scan", body)
	req.Header.Set("Content-Type", contentType)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body: %s", rec.Code, rec.Body.String())
	}
	result := decodeJSON[LedgerEmailScanResult](t, rec)
	if !result.Partial || result.OrdersNew != 1 || len(fake.Requests()) != 1 {
		t.Fatalf("result = %+v, requests = %d", result, len(fake.Requests()))
	}
	if !strings.Contains(strings.Join(result.Warnings, "\n"), "scan stopped at the time limit") {
		t.Errorf("warnings = %v", result.Warnings)
	}
	stored, err := s.GetLedgerEmailAccount(context.Background(), testUserID, account.ID)
	if err != nil || stored.LastScanAt != nil || !strings.Contains(stored.LastScanStatusMessage, "incomplete") {
		t.Errorf("account = %+v, err = %v", stored, err)
	}
}

func TestScanHandler_UploadDoesNotMoveLastScanAt(t *testing.T) {
	g := goldenOrders[0]
	h, s := newTestHandlerWithLLM(t, llmtest.New(llmtest.JSON(g.extraction())))
	past := time.Now().UTC().Add(-72 * time.Hour)
	account := LedgerEmailAccount{ID: uuid.New(), Name: "M", IMAPHost: "h", IMAPPort: 993, Username: "u", LastScanAt: &past, CreatedAt: past, UpdatedAt: past}
	if err := s.CreateLedgerEmailAccount(context.Background(), testUserID, account); err != nil {
		t.Fatalf("CreateLedgerEmailAccount: %v", err)
	}
	if rec := scanRequest(t, newMux(h), account.ID, g.fixture); rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body: %s", rec.Code, rec.Body.String())
	}
	stored, err := s.GetLedgerEmailAccount(context.Background(), testUserID, account.ID)
	if err != nil || stored.LastScanAt == nil || !stored.LastScanAt.Equal(past) || stored.LastScanStatusMessage != "Processed uploaded email messages" {
		t.Errorf("account = %+v, err = %v", stored, err)
	}
}

func multipartTwoFiles(t *testing.T, first, second []byte) (*bytes.Buffer, string) {
	t.Helper()
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	for i, content := range [][]byte{first, second} {
		part, err := writer.CreateFormFile("files", fmt.Sprintf("m%d.eml", i))
		if err != nil {
			t.Fatalf("create form file: %v", err)
		}
		_, _ = part.Write(content)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close writer: %v", err)
	}
	return body, writer.FormDataContentType()
}

func TestEmailDedupKey_BoundsAndCleansMessageIDs(t *testing.T) {
	short := "<abc@example.com>"
	if got := emailDedupKey(short, nil); got != short {
		t.Errorf("short id changed: %q", got)
	}
	long := emailDedupKey("<"+strings.Repeat("a", 70000)+">", nil)
	if !strings.HasPrefix(long, "mid:") || len(long) != len("mid:")+32 {
		t.Errorf("long id key = %q", long)
	}
	ctrl := emailDedupKey("<a\x00b\nc@example.com>", nil)
	if !strings.HasPrefix(ctrl, "mid:") {
		t.Errorf("control character id key = %q", ctrl)
	}
	if emailDedupKey("<"+strings.Repeat("a", 70000)+">", nil) != long || emailDedupKey("<x"+strings.Repeat("a", 70000)+">", nil) == long {
		t.Error("hashed keys are not stable/distinct")
	}
}

func TestPipeline_HugeOrControlCharMessageIDsDoNotBreakTheScan(t *testing.T) {
	ids := map[string]string{
		"huge":    "<" + strings.Repeat("x", 70000) + "@example.com>",
		"control": "<a\x01b/../c@example.com>",
	}
	for name, id := range ids {
		t.Run(name, func(t *testing.T) {
			fake := llmtest.New(
				llmtest.JSON(emailOrderExtraction{IsOrder: false}),
				llmtest.JSON(goldenOrders[0].extraction()),
			)
			p := newPipelineTest(t, fake)
			mk := func(label string) emailMessage {
				return emailMessage{
					Label:  label,
					Key:    emailDedupKey(id+label, nil),
					Header: ParsedEmail{MessageID: id},
					Load:   func() (ParsedEmail, error) { return ParsedEmail{Subject: "s", TextBody: "b"}, nil },
				}
			}
			notOrder, order := mk("a"), mk("b")
			for range 2 {
				result, err := p.process(context.Background(), notOrder, order)
				if err != nil {
					t.Fatalf("scan failed: %v", err)
				}
				if len(result.Warnings) != 0 {
					t.Fatalf("warnings = %v", result.Warnings)
				}
			}
			if len(p.orders(t)) != 1 || len(fake.Requests()) != 2 || !p.seen(t, notOrder.Key) {
				t.Errorf("orders = %d, requests = %d", len(p.orders(t)), len(fake.Requests()))
			}
		})
	}
}

func TestScanHandler_ScanResultDoesNotRevertAccountEditsMadeDuringTheScan(t *testing.T) {
	var s *testStores
	var account LedgerEmailAccount
	fake := &llmtest.Fake{Func: func(llm.Request) (json.RawMessage, error) {
		edited := account
		edited.Folder, edited.PreClassify = "Edited", true
		if err := s.UpdateLedgerEmailAccount(context.Background(), testUserID, edited); err != nil {
			t.Errorf("concurrent edit: %v", err)
		}
		return json.Marshal(goldenOrders[0].extraction())
	}}
	var h *Handler
	h, s = newTestHandlerWithLLM(t, fake)
	now := time.Now().UTC()
	account = LedgerEmailAccount{ID: uuid.New(), Name: "M", IMAPHost: "h", IMAPPort: 993, Username: "u", Folder: "Original", CreatedAt: now, UpdatedAt: now}
	if err := s.CreateLedgerEmailAccount(context.Background(), testUserID, account); err != nil {
		t.Fatalf("CreateLedgerEmailAccount: %v", err)
	}
	if rec := scanRequest(t, newMux(h), account.ID, goldenOrders[0].fixture); rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body: %s", rec.Code, rec.Body.String())
	}
	stored, err := s.GetLedgerEmailAccount(context.Background(), testUserID, account.ID)
	if err != nil || stored.Folder != "Edited" || !stored.PreClassify || stored.LastScanStatusMessage != "Processed uploaded email messages" {
		t.Errorf("account = %+v, err = %v", stored, err)
	}
}

func TestPipeline_SkippedCountsOnlyRetryWorthyFailures(t *testing.T) {
	g := goldenOrders[0]
	ok := llmtest.JSON(g.extraction())
	tests := []struct {
		name        string
		answers     []llmtest.Response
		wantSkipped int
	}{
		{"transient failure", []llmtest.Response{llmtest.Error(&llm.APIError{StatusCode: 503})}, 1},
		{"unusable output", []llmtest.Response{llmtest.Error(llm.ErrInvalidOutput)}, 0},
		{"incomplete extraction", []llmtest.Response{llmtest.JSON(emailOrderExtraction{IsOrder: true})}, 0},
		{"success", []llmtest.Response{ok}, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := newPipelineTest(t, llmtest.New(tt.answers...))
			result, err := p.process(context.Background(), imapMessage(t, "AmazonDE-1.eml", 1))
			if err != nil || result.Skipped != tt.wantSkipped || result.Complete() != (tt.wantSkipped == 0) {
				t.Errorf("skipped = %d, complete = %v, err = %v", result.Skipped, result.Complete(), err)
			}
		})
	}
}

func TestPipeline_LoadFailuresAreSkippedOrFatalDependingOnTheCause(t *testing.T) {
	load := func(err error) emailMessage {
		return emailMessage{Label: "uid 1", Key: "k", Load: func() (ParsedEmail, error) { return ParsedEmail{}, err }}
	}
	p := newPipelineTest(t, llmtest.New())

	result, err := p.process(context.Background(), load(fmt.Errorf("%w: NO", errIMAPRejected)))
	if err != nil || result.Skipped != 1 {
		t.Errorf("rejected fetch: skipped = %d, err = %v", result.Skipped, err)
	}
	result, err = p.process(context.Background(), load(errMessageParse))
	if err != nil || result.Skipped != 0 || len(result.Warnings) != 1 {
		t.Errorf("parse failure: skipped = %d, warnings = %v, err = %v", result.Skipped, result.Warnings, err)
	}
	result, err = p.process(context.Background(), load(errors.New("reading message literal: EOF")))
	if !errors.Is(err, ErrIMAP) {
		t.Errorf("I/O failure: err = %v, want ErrIMAP", err)
	}
	result, err = p.process(context.Background(), load(errMessageTooLarge))
	if err != nil || !p.seen(t, "k") {
		t.Errorf("too large: err = %v, seen = %v (want marked, not retried)", err, p.seen(t, "k"))
	}
	_ = result
}

func TestScanHandler_ScanFinishesWhenTheClientGivesUp(t *testing.T) {
	reqCtx, cancelRequest := context.WithCancel(context.Background())
	defer cancelRequest()
	fake := &llmtest.Fake{Func: func(llm.Request) (json.RawMessage, error) {
		cancelRequest() // the browser or proxy hangs up while the LLM is working
		return json.Marshal(goldenOrders[0].extraction())
	}}
	h, s := newTestHandlerWithLLM(t, fake)
	now := time.Now().UTC()
	account := LedgerEmailAccount{ID: uuid.New(), Name: "M", IMAPHost: "h", IMAPPort: 993, Username: "u", CreatedAt: now, UpdatedAt: now}
	if err := s.CreateLedgerEmailAccount(context.Background(), testUserID, account); err != nil {
		t.Fatalf("CreateLedgerEmailAccount: %v", err)
	}
	raw, _ := os.ReadFile(fixturePath(goldenOrders[0].fixture))
	body, contentType := multipartBody(t, nil, "files", goldenOrders[0].fixture, raw)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/ledger/email-accounts/"+account.ID.String()+"/scan", body).WithContext(reqCtx)
	req.Header.Set("Content-Type", contentType)
	newMux(h).ServeHTTP(httptest.NewRecorder(), req)

	orders, _ := s.ListLedgerEmailOrders(context.Background(), testUserID)
	stored, err := s.GetLedgerEmailAccount(context.Background(), testUserID, account.ID)
	if len(orders) != 1 || err != nil || stored.LastScanStatusMessage != "Processed uploaded email messages" {
		t.Errorf("orders = %d, account = %+v, err = %v", len(orders), stored, err)
	}
}

func TestPipeline_MessageStoredByAnotherScanMeanwhileIsNotDuplicated(t *testing.T) {
	g := goldenOrders[0]
	msg := imapMessage(t, g.fixture, 1)
	var p *pipelineTest
	fake := &llmtest.Fake{Func: func(llm.Request) (json.RawMessage, error) {
		// A scan of another account with the same mailbox stores the message first.
		now := time.Now().UTC()
		other := LedgerEmailOrder{ID: uuid.New(), EmailAccountID: uuid.New(), ImporterID: LedgerEmailImporterLLM, OrderDate: g.date, TotalMinor: g.totalMinor, Currency: "EUR", EmailMessageID: msg.Key, MatchStatus: LedgerEmailOrderStatusUnmatched, CreatedAt: now, UpdatedAt: now}
		if err := p.store.CreateScannedLedgerEmailOrder(context.Background(), testUserID, other); err != nil {
			t.Errorf("other scan: %v", err)
		}
		return json.Marshal(g.extraction())
	}}
	p = newPipelineTest(t, fake)

	result, err := p.process(context.Background(), msg)
	if err != nil || result.OrdersNew != 0 || result.OrdersFound != 1 || len(result.Orders) != 1 {
		t.Fatalf("result = %+v, err = %v", result, err)
	}
	if len(p.orders(t)) != 1 {
		t.Errorf("stored orders = %d, want 1", len(p.orders(t)))
	}
}

func TestPipeline_RepeatedlyRejectedFullFetchIsGivenUpOn(t *testing.T) {
	p := newPipelineTest(t, llmtest.New())
	msg := emailMessage{Label: "uid 1", Key: "<k@example.com>", Load: func() (ParsedEmail, error) {
		return ParsedEmail{}, fmt.Errorf("%w: NO", errIMAPRejected)
	}}
	for scan := 1; scan <= maxFetchRejections; scan++ {
		result, err := p.process(context.Background(), msg)
		if err != nil {
			t.Fatalf("scan %d: %v", scan, err)
		}
		wantSkipped := 0
		if scan < maxFetchRejections {
			wantSkipped = 1
		}
		if result.Skipped != wantSkipped {
			t.Fatalf("scan %d: skipped = %d, want %d", scan, result.Skipped, wantSkipped)
		}
	}
	if !p.seen(t, msg.Key) {
		t.Error("message not marked after the last rejection")
	}
	if result, _ := p.process(context.Background(), msg); len(result.Warnings) != 0 || result.Skipped != 0 {
		t.Errorf("a given-up message is processed again: %+v", result)
	}
}
