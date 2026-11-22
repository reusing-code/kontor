package ledger

import (
	"bytes"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func pinAnalysisToday(h *Handler) {
	h.now = func() time.Time { return time.Date(2026, 11, 22, 23, 30, 0, 0, time.UTC) }
}

func getAnalysis(mux http.Handler, path string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("GET", "/api/v1/ledger/analysis/"+path, nil))
	return rec
}

func addChildCategory(t *testing.T, s *testStores, name string, parent uuid.UUID) uuid.UUID {
	t.Helper()
	now := time.Now().UTC()
	id := uuid.New()
	if err := s.CreateLedgerCategory(t.Context(), testUserID, LedgerCategory{
		ID: id, Name: name, ParentID: &parent, CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("CreateLedgerCategory: %v", err)
	}
	return id
}

func addCategorized(t *testing.T, s *testStores, accountID uuid.UUID, date string, amount int64, categoryID *uuid.UUID) {
	t.Helper()
	addTransaction(t, s, accountID, func(txn *LedgerTransaction) {
		txn.BookingDate, txn.AmountMinor, txn.CategoryID = date, amount, categoryID
	})
}

func wantAnalysisStatus(t *testing.T, mux http.Handler, path string, status int, message string) {
	t.Helper()
	rec := getAnalysis(mux, path)
	if rec.Code != status || !strings.Contains(rec.Body.String(), message) {
		t.Fatalf("GET %s = %d %s, want %d with %q", path, rec.Code, rec.Body, status, message)
	}
}

func TestLedgerSpendingEndpoint(t *testing.T) {
	h, s := newTestHandler(t)
	pinAnalysisToday(h)
	mux := newMux(h)

	t.Run("empty user", func(t *testing.T) {
		rec := getAnalysis(mux, "spending")
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, body = %s", rec.Code, rec.Body)
		}
		body := rec.Body.String()
		for _, want := range []string{
			`"from":"2026-11-22"`, `"to":"2026-11-22"`, `"months":["2026-11"]`,
			`"series":[]`, `"totalsMinor":[0]`, `"excludedCurrencies":[]`,
		} {
			if !strings.Contains(body, want) {
				t.Fatalf("body %s lacks %s", body, want)
			}
		}
	})

	accountID := addAccount(t, s, "Main")
	food := addCategory(t, s, "Food")
	groceries := addChildCategory(t, s, "Groceries", food)
	addCategorized(t, s, accountID, "2026-09-10", -1500, &groceries)
	addCategorized(t, s, accountID, "2026-10-02", -700, &food)
	addCategorized(t, s, accountID, "2026-10-05", -300, nil)

	t.Run("top level with both ends omitted", func(t *testing.T) {
		rec := getAnalysis(mux, "spending")
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, body = %s", rec.Code, rec.Body)
		}
		body := rec.Body.String()
		for _, want := range []string{
			`"from":"2026-09-10"`, `"to":"2026-11-22"`, `"months":["2026-09","2026-10","2026-11"]`,
			`"kind":"category","categoryId":"` + food.String() + `","name":"Food","amountsMinor":[1500,700,0],"totalMinor":2200`,
			`{"kind":"uncategorized","name":"","amountsMinor":[0,300,0],"totalMinor":300}`,
			`"totalsMinor":[1500,1000,0]`,
		} {
			if !strings.Contains(body, want) {
				t.Fatalf("body %s lacks %s", body, want)
			}
		}
		if strings.Contains(body, "parentCategoryId") {
			t.Fatalf("body %s names a parent in the top-level view", body)
		}
	})

	t.Run("range, account and parent", func(t *testing.T) {
		rec := getAnalysis(mux, "spending?from=2026-10-01&to=2026-10-31&accountId="+accountID.String()+"&parentCategoryId="+food.String())
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, body = %s", rec.Code, rec.Body)
		}
		got := decodeJSON[LedgerSpendingAnalysis](t, rec)
		if got.From != "2026-10-01" || got.To != "2026-10-31" || got.ParentCategoryID == nil || *got.ParentCategoryID != food {
			t.Fatalf("got %+v", got)
		}
		if len(got.Series) != 1 || got.Series[0].Kind != LedgerSeriesDirect || got.Series[0].TotalMinor != 700 {
			t.Fatalf("series = %+v, want the amount booked on Food itself", got.Series)
		}
	})

	t.Run("bad requests", func(t *testing.T) {
		for path, message := range map[string]string{
			"spending?from=10.10.2026":                               "from must be a date (YYYY-MM-DD)",
			"spending?from=2026-02-30":                               "from must be a date (YYYY-MM-DD)",
			"spending?to=yesterday":                                  "to must be a date (YYYY-MM-DD)",
			"spending?from=2026-10-02&to=2026-10-01":                 "from must not be after to",
			"spending?from=2027-01-01":                               "from must not be after to",
			"spending?from=1900-01-01&to=2026-10-01":                 "more than 1200 months",
			"spending?accountId=nope":                                "invalid accountId",
			"spending?parentCategoryId=nope":                         "invalid parentCategoryId",
			"spending?parentCategoryId=" + groceries.String():        "parentCategoryId must be a top-level category",
			"spending?from=bad&accountId=" + accountID.String():      "from must be a date (YYYY-MM-DD)",
			"spending?to=2026-1-1&parentCategoryId=" + food.String(): "to must be a date (YYYY-MM-DD)",
		} {
			wantAnalysisStatus(t, mux, path, http.StatusBadRequest, message)
		}
	})

	t.Run("unknown account and parent", func(t *testing.T) {
		wantAnalysisStatus(t, mux, "spending?accountId="+uuid.NewString(), http.StatusNotFound, "not found")
		wantAnalysisStatus(t, mux, "spending?parentCategoryId="+uuid.NewString(), http.StatusNotFound, "not found")
	})
}

// A range given in full is refused before the store is read: the handler
// here has no store, so loading would panic.
func TestLedgerSpendingEndpoint_RefusesABadRangeBeforeLoading(t *testing.T) {
	h := &Handler{logger: slog.New(slog.DiscardHandler)}
	pinAnalysisToday(h)
	mux := newMux(h)

	wantAnalysisStatus(t, mux, "spending?from=0202-12-01&to=2026-11-22", http.StatusBadRequest, "more than 1200 months")
	wantAnalysisStatus(t, mux, "spending?from=2026-10-02&to=2026-10-01", http.StatusBadRequest, "from must not be after to")
}

func TestLedgerAnalysis_LogsNoTransactionData(t *testing.T) {
	h, s := newTestHandler(t)
	pinAnalysisToday(h)
	logs := &bytes.Buffer{}
	h.logger = slog.New(slog.NewTextHandler(logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	mux := newMux(h)

	const counterparty, purpose, iban = "Geheimladen Mueller", "Rechnung 4711-XYZ", "DE89370400440532013000"
	accountID := addAccount(t, s, "Main")
	food := addCategory(t, s, "Food")
	addTransaction(t, s, accountID, func(txn *LedgerTransaction) {
		txn.BookingDate, txn.AmountMinor, txn.CategoryID = "2026-10-02", -123457, &food
		txn.CounterpartyName, txn.Purpose, txn.CounterpartyIBAN = counterparty, purpose, iban
	})

	for path, status := range analysisLogRequests(accountID) {
		if rec := getAnalysis(mux, path); rec.Code != status {
			t.Fatalf("GET %s = %d %s, want %d", path, rec.Code, rec.Body, status)
		}
	}
	for _, secret := range []string{counterparty, purpose, iban, "123457"} {
		if strings.Contains(logs.String(), secret) {
			t.Fatalf("log contains %q:\n%s", secret, logs)
		}
	}
}

func analysisLogRequests(accountID uuid.UUID) map[string]int {
	return map[string]int{
		"spending": http.StatusOK,
		"spending?accountId=" + accountID.String():      http.StatusOK,
		"spending?from=bad":                             http.StatusBadRequest,
		"spending?from=2026-10-02&to=2026-10-01":        http.StatusBadRequest,
		"spending?accountId=" + uuid.NewString():        http.StatusNotFound,
		"spending?parentCategoryId=" + uuid.NewString(): http.StatusNotFound,
		"cash-flow":                             http.StatusOK,
		"cash-flow?to=bad":                      http.StatusBadRequest,
		"ranking":                               http.StatusOK,
		"ranking?from=2026-10-02&to=2026-10-01": http.StatusBadRequest,
	}
}

func TestLedgerCashFlowAndRankingEndpoints(t *testing.T) {
	h, s := newTestHandler(t)
	pinAnalysisToday(h)
	mux := newMux(h)

	t.Run("empty user", func(t *testing.T) {
		for path, want := range map[string][]string{
			"cash-flow": {`"from":"2026-11-22"`, `"to":"2026-11-22"`, `"months":["2026-11"]`, `"incomeMinor":[0]`, `"expensesMinor":[0]`, `"netMinor":[0]`, `"excludedCurrencies":[]`},
			"ranking":   {`"from":"2026-11-22"`, `"to":"2026-11-22"`, `"categories":[]`, `"counterparties":[]`, `"excludedCurrencies":[]`},
		} {
			rec := getAnalysis(mux, path)
			if rec.Code != http.StatusOK {
				t.Fatalf("GET %s = %d %s", path, rec.Code, rec.Body)
			}
			for _, part := range want {
				if !strings.Contains(rec.Body.String(), part) {
					t.Fatalf("GET %s: body %s lacks %s", path, rec.Body, part)
				}
			}
		}
	})

	accountID := addAccount(t, s, "Main")
	food := addCategory(t, s, "Food")
	salary := addCategory(t, s, "Salary")
	pay := func(date string, amount int64, category *uuid.UUID, counterparty string) {
		addTransaction(t, s, accountID, func(txn *LedgerTransaction) {
			txn.BookingDate, txn.AmountMinor, txn.CategoryID, txn.CounterpartyName = date, amount, category, counterparty
		})
	}
	pay("2026-09-10", -1500, &food, "Supermarkt")
	pay("2026-09-28", 250_000, &salary, "Employer")
	pay("2026-10-05", -300, nil, "")

	t.Run("cash flow", func(t *testing.T) {
		rec := getAnalysis(mux, "cash-flow?from=2026-09-01&to=2026-10-31")
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, body = %s", rec.Code, rec.Body)
		}
		for _, want := range []string{
			`"from":"2026-09-01"`, `"to":"2026-10-31"`, `"months":["2026-09","2026-10"]`,
			`"incomeMinor":[250000,0]`, `"expensesMinor":[1500,300]`, `"netMinor":[248500,-300]`,
		} {
			if !strings.Contains(rec.Body.String(), want) {
				t.Fatalf("body %s lacks %s", rec.Body, want)
			}
		}
	})

	t.Run("ranking", func(t *testing.T) {
		rec := getAnalysis(mux, "ranking")
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, body = %s", rec.Code, rec.Body)
		}
		for _, want := range []string{
			`"from":"2026-09-10"`, `"to":"2026-11-22"`,
			`"categories":[{"kind":"category","categoryId":"` + food.String() + `","name":"Food","amountMinor":1500},{"kind":"uncategorized","name":"","amountMinor":300}]`,
			`"counterparties":[{"name":"Supermarkt","amountMinor":1500},{"name":"","unknown":true,"amountMinor":300}]`,
		} {
			if !strings.Contains(rec.Body.String(), want) {
				t.Fatalf("body %s lacks %s", rec.Body, want)
			}
		}
	})

	t.Run("bad ranges", func(t *testing.T) {
		for _, endpoint := range []string{"cash-flow", "ranking"} {
			for query, message := range map[string]string{
				"?from=1.9.2026":                 "from must be a date (YYYY-MM-DD)",
				"?to=2026-13-01":                 "to must be a date (YYYY-MM-DD)",
				"?from=2026-10-02&to=2026-10-01": "from must not be after to",
				"?from=1900-01-01&to=2026-10-01": "more than 1200 months",
			} {
				wantAnalysisStatus(t, mux, endpoint+query, http.StatusBadRequest, message)
			}
		}
	})
}

func TestLedgerAnalysis_FiveThousandTransactions(t *testing.T) {
	h, s := newTestHandler(t)
	pinAnalysisToday(h)
	mux := newMux(h)
	accountID := addAccount(t, s, "Main")
	categories := make([]uuid.UUID, 0, 12)
	for i := range 6 {
		top := addCategory(t, s, fmt.Sprintf("Top %d", i))
		categories = append(categories, top, addChildCategory(t, s, fmt.Sprintf("Child %d", i), top))
	}

	const total, chunk = 5000, 500
	for start := 0; start < total; start += chunk {
		txns := make([]LedgerTransaction, chunk)
		for i := range txns {
			n := start + i
			txns[i] = LedgerTransaction{
				ID: uuid.New(), AccountID: accountID, Currency: "EUR", CategoryID: &categories[n%len(categories)],
				BookingDate:      fmt.Sprintf("%04d-%02d-%02d", 2021+n%5, 1+n%12, 1+n%28),
				AmountMinor:      int64(n%9000) - 7000,
				CounterpartyName: fmt.Sprintf("Shop %d", n%150),
			}
		}
		commitTransactions(t, s, accountID, txns...)
	}

	for _, path := range []string{"spending", "spending?parentCategoryId=" + categories[0].String(), "cash-flow", "ranking"} {
		start := time.Now()
		rec := getAnalysis(mux, path)
		elapsed := time.Since(start)
		if rec.Code != http.StatusOK {
			t.Fatalf("GET %s = %d %s", path, rec.Code, rec.Body)
		}
		if elapsed > 2*time.Second {
			t.Fatalf("GET %s took %s with %d transactions", path, elapsed, total)
		}
	}
	flow := decodeJSON[LedgerCashFlowAnalysis](t, getAnalysis(mux, "cash-flow"))
	var net, want int64
	for _, n := range flow.NetMinor {
		net += n
	}
	for n := range total {
		want += int64(n%9000) - 7000
	}
	if net != want || flow.From != "2021-01-01" {
		t.Fatalf("net over all months = %d from %s, want %d from 2021-01-01", net, flow.From, want)
	}
}
