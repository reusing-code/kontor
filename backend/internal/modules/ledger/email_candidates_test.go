package ledger

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
)

func addEmailOrder(t *testing.T, s *testStores, importerID string, orderDate string, totalMinor int64) LedgerEmailOrder {
	t.Helper()
	now := time.Now().UTC()
	order := LedgerEmailOrder{
		ID:             uuid.New(),
		EmailAccountID: uuid.New(),
		ImporterID:     importerID,
		OrderDate:      orderDate,
		TotalMinor:     totalMinor,
		Currency:       "EUR",
		MatchStatus:    LedgerEmailOrderStatusUnmatched,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if err := s.CreateLedgerEmailOrder(context.Background(), testUserID, order); err != nil {
		t.Fatalf("CreateLedgerEmailOrder: %v", err)
	}
	return order
}

func TestChargeWindow_Contains(t *testing.T) {
	window := ChargeWindow{BeforeDays: 3, AfterDays: 60}
	tests := []struct {
		bookingDate string
		orderDate   string
		want        bool
	}{
		{"2026-04-01", "2026-04-01", true},
		{"2026-03-29", "2026-04-01", true},
		{"2026-03-28", "2026-04-01", false},
		{"2026-05-31", "2026-04-01", true},
		{"2026-06-01", "2026-04-01", false},
		{"2026-03-22", "2026-04-01", false},
		{"2026-04-05", "", true},
		{"invalid", "2026-04-01", false},
	}
	for _, tt := range tests {
		if got := window.Contains(tt.bookingDate, tt.orderDate); got != tt.want {
			t.Errorf("Contains(%q, %q) = %v, want %v", tt.bookingDate, tt.orderDate, got, tt.want)
		}
	}
}

func TestAutoLinkOrder_UsesImporterChargeWindow(t *testing.T) {
	tests := []struct {
		name     string
		importer Importer
		purpose  string
		want     bool
	}{
		{"amazon charged on shipping", newAmazonDEImporter(), "AMAZON PAYMENTS", true},
		{"paypal outside window", newPayPalDEImporter(), "PAYPAL EUROPE", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, s := newTestHandler(t)
			accountID := addAccount(t, s, "Card")
			txnID := uuid.New()
			commitTransactions(t, s, accountID,
				LedgerTransaction{ID: txnID, AccountID: accountID, BookingDate: "2026-04-21", AmountMinor: -4999, Currency: "EUR", CounterpartyName: tt.purpose},
			)
			order := LedgerEmailOrder{ID: uuid.New(), OrderDate: "2026-04-01", TotalMinor: 4999, Currency: "EUR"}

			linked, _, err := h.ledgerEmail.autoLinkOrder(context.Background(), testUserID, tt.importer, &order)
			if err != nil {
				t.Fatalf("autoLinkOrder: %v", err)
			}
			if linked != tt.want {
				t.Fatalf("linked = %v, want %v", linked, tt.want)
			}
			if tt.want && (len(order.LinkedTransactionIDs) != 1 || order.LinkedTransactionIDs[0] != txnID) {
				t.Fatalf("linked ids = %v, want [%s]", order.LinkedTransactionIDs, txnID)
			}
		})
	}
}

func TestListLedgerEmailOrderCandidates(t *testing.T) {
	h, s := newTestHandler(t)
	mux := newMux(h)

	checking := addAccount(t, s, "Checking")
	card := addAccount(t, s, "Card")
	exact := uuid.New()
	alreadyLinked := uuid.New()
	late := uuid.New()
	tooEarly := uuid.New()
	incoming := uuid.New()
	commitTransactions(t, s, checking,
		LedgerTransaction{ID: alreadyLinked, AccountID: checking, BookingDate: "2026-04-03", AmountMinor: -5100, Currency: "EUR"},
		LedgerTransaction{ID: tooEarly, AccountID: checking, BookingDate: "2026-03-20", AmountMinor: -5000, Currency: "EUR"},
		LedgerTransaction{ID: incoming, AccountID: checking, BookingDate: "2026-04-02", AmountMinor: 5000, Currency: "EUR"},
	)
	commitTransactions(t, s, card,
		LedgerTransaction{ID: exact, AccountID: card, BookingDate: "2026-04-20", AmountMinor: -5000, Currency: "EUR"},
		LedgerTransaction{ID: late, AccountID: card, BookingDate: "2026-08-01", AmountMinor: -5000, Currency: "EUR"},
	)
	order := addEmailOrder(t, s, LedgerEmailImporterAmazonDE, "2026-04-01", 5000)

	linkedOrder := addEmailOrder(t, s, LedgerEmailImporterAmazonDE, "2026-04-01", 5100)
	if _, err := s.LinkLedgerEmailOrder(context.Background(), testUserID, linkedOrder.ID, LedgerEmailOrderLinkInput{TransactionIDs: []uuid.UUID{alreadyLinked}}); err != nil {
		t.Fatalf("LinkLedgerEmailOrder: %v", err)
	}

	get := func(query string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest("GET", "/api/v1/ledger/email-orders/"+order.ID.String()+"/candidates"+query, nil))
		return rec
	}

	t.Run("default window", func(t *testing.T) {
		rec := get("")
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
		}
		result := decodeJSON[LedgerEmailOrderCandidatesResult](t, rec)
		if result.BeforeDays != 3 || result.AfterDays != 60 {
			t.Fatalf("window = %+v, want amazon window", result.ChargeWindow)
		}
		if len(result.Items) != 1 || result.Items[0].Transaction.ID != exact {
			t.Fatalf("items = %+v, want only %s", result.Items, exact)
		}
		item := result.Items[0]
		if item.AccountName != "Card" || item.DateDeltaDays == nil || *item.DateDeltaDays != 19 {
			t.Fatalf("item = %+v, want account Card and +19 days", item)
		}
	})

	t.Run("afterDays override", func(t *testing.T) {
		rec := get("?afterDays=365")
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
		}
		result := decodeJSON[LedgerEmailOrderCandidatesResult](t, rec)
		if result.AfterDays != 365 || len(result.Items) != 2 || result.Items[0].Transaction.ID != exact || result.Items[1].Transaction.ID != late {
			t.Fatalf("result = %+v, want [%s %s]", result, exact, late)
		}
	})

	for _, query := range []string{"?afterDays=0", "?afterDays=366", "?afterDays=abc"} {
		t.Run("invalid "+query, func(t *testing.T) {
			if rec := get(query); rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
			}
		})
	}

	t.Run("unknown order", func(t *testing.T) {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest("GET", "/api/v1/ledger/email-orders/"+uuid.NewString()+"/candidates", nil))
		if rec.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
		}
	})
}
