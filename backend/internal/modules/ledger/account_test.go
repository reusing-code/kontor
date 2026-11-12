package ledger

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

const validIBANForTest = "DE89370400440532013000"

func postAccount(mux http.Handler, body any) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("POST", "/api/v1/ledger/accounts", jsonBody(body)))
	return rec
}

func TestCreateLedgerAccount_Success(t *testing.T) {
	h, _ := newTestHandler(t)
	mux := newMux(h)

	rec := postAccount(mux, map[string]any{"name": " Main ", "bank": "DKB", "iban": "de89 3704 0044 0532 0130 00"})
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body)
	}
	got := decodeJSON[LedgerAccount](t, rec)
	if got.Name != "Main" || got.Currency != "EUR" || got.IBAN != validIBANForTest || got.ID.String() == "" {
		t.Fatalf("account = %#v", got)
	}

	list := httptest.NewRecorder()
	mux.ServeHTTP(list, httptest.NewRequest("GET", "/api/v1/ledger/accounts", nil))
	accounts := decodeJSON[[]LedgerAccount](t, list)
	if len(accounts) != 1 || accounts[0].ID != got.ID {
		t.Fatalf("list = %#v", accounts)
	}
}

func TestCreateLedgerAccount_CurrencyNormalized(t *testing.T) {
	h, _ := newTestHandler(t)
	rec := postAccount(newMux(h), map[string]any{"name": "USD", "bank": "B", "currency": "usd"})
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body)
	}
	if got := decodeJSON[LedgerAccount](t, rec); got.Currency != "USD" {
		t.Fatalf("currency = %q", got.Currency)
	}
}

func TestCreateLedgerAccount_Invalid(t *testing.T) {
	h, _ := newTestHandler(t)
	mux := newMux(h)
	cases := map[string]map[string]any{
		"missing name":     {"bank": "B"},
		"blank name":       {"name": "  ", "bank": "B"},
		"missing bank":     {"name": "N"},
		"long name":        {"name": string(repeat('x', 201)), "bank": "B"},
		"bad currency":     {"name": "N", "bank": "B", "currency": "EURO"},
		"numeric currency": {"name": "N", "bank": "B", "currency": "12A"},
		"bad iban format":  {"name": "N", "bank": "B", "iban": "nope"},
		"bad iban check":   {"name": "N", "bank": "B", "iban": "DE89370400440532013001"},
		"unknown field":    {"name": "N", "bank": "B", "x": 1},
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			if rec := postAccount(mux, body); rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, body = %s", rec.Code, rec.Body)
			}
		})
	}
}

func repeat(c byte, n int) []byte {
	b := make([]byte, n)
	for i := range b {
		b[i] = c
	}
	return b
}

func TestCreateLedgerAccount_DuplicateIBAN(t *testing.T) {
	h, _ := newTestHandler(t)
	mux := newMux(h)
	body := map[string]any{"name": "A", "bank": "B", "iban": validIBANForTest}
	if rec := postAccount(mux, body); rec.Code != http.StatusCreated {
		t.Fatalf("first = %d", rec.Code)
	}
	body["iban"] = "DE89 3704 0044 0532 0130 00"
	if rec := postAccount(mux, body); rec.Code != http.StatusConflict {
		t.Fatalf("duplicate = %d, body = %s", rec.Code, rec.Body)
	}
}

func TestValidIBAN(t *testing.T) {
	for iban, want := range map[string]bool{
		validIBANForTest:           true,
		"GB82WEST12345698765432":   true,
		"DE89370400440532013001":   false,
		"DE8937040044053201300":    false,
		"1289370400440532013000":   false,
		"DE89370400440532013000XX": false,
	} {
		if got := validIBAN(iban); got != want {
			t.Errorf("validIBAN(%q) = %v, want %v", iban, got, want)
		}
	}
}

func TestLedgerAccountInput_ValidateKeepsImportBehavior(t *testing.T) {
	in := LedgerAccountInput{Name: "n", Bank: "b", IBAN: "whatever"}
	if err := in.Validate(); err != nil || in.Currency != "EUR" || in.IBAN != "whatever" {
		t.Fatalf("Validate changed: %v %#v", err, in)
	}
}
