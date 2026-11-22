package ledger

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/dgraph-io/badger/v4"
	"github.com/google/uuid"
	"github.com/reusing-code/kontor/backend/internal/storage"
	"github.com/reusing-code/kontor/backend/internal/storage/link"
)

// analysisEdgeEndpoints are the analysis endpoints the edge tests run against.
var analysisEdgeEndpoints = []string{"spending"}

// seedAnalysisEdgeData stores an account with a balance, a category and a few
// bookings, and returns the account.
func seedAnalysisEdgeData(t *testing.T, s *testStores) uuid.UUID {
	t.Helper()
	accountID := addAccount(t, s, "Main")
	food := addCategory(t, s, "Food")
	putSnapshot(t, s, accountID, "2026-09-01", 100_000, "dkb.csv")
	for i, date := range []string{"2026-08-15", "2026-09-10", "2026-10-02", "2026-11-22"} {
		addTransaction(t, s, accountID, func(txn *LedgerTransaction) {
			txn.BookingDate, txn.AmountMinor, txn.CounterpartyName = date, int64(-1000*(i+1)), "Laden"
			if i%2 == 0 {
				txn.CategoryID = &food
			}
		})
	}
	return accountID
}

func TestLedgerAnalysis_HostileRangeParameters(t *testing.T) {
	h, s := newTestHandler(t)
	pinAnalysisToday(h)
	mux := newMux(h)
	seedAnalysisEdgeData(t, s)

	refused := []string{
		"2026-01-01\x00",
		"  2026-01-01",
		"2026-01-01 ",
		"2026-01-01T00:00:00Z",
		"-2026-01-01",
		"+2026-01-01",
		"２０２６-01-01",
		"2026-02-29",
		"2026-00-10",
		"2026-01-32",
		"20260101",
		"2026/01/01",
		"0x7ea-01-01",
		"<script>alert(1)</script>",
		"'; DROP TABLE txn; --",
		"../../etc/passwd",
		strings.Repeat("9", 10_000),
	}
	for _, endpoint := range analysisEdgeEndpoints {
		for _, name := range []string{"from", "to"} {
			for _, value := range refused {
				rec := getAnalysis(mux, endpoint+"?"+name+"="+url.QueryEscape(value))
				if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), name+" must be a date (YYYY-MM-DD)") {
					t.Fatalf("GET %s %s=%.40q = %d %.200s, want 400", endpoint, name, value, rec.Code, rec.Body)
				}
				if strings.Contains(rec.Body.String(), value) {
					t.Fatalf("GET %s: the error echoes the %s parameter: %.200s", endpoint, name, rec.Body)
				}
			}
		}
	}

	accepted := map[string][]string{
		"?from=&to=":                          {`"to":"2026-11-22"`},
		"?from=2024-02-29&to=2024-02-29":      {`"from":"2024-02-29"`, `"to":"2024-02-29"`},
		"?from=2026-09-01&from=1900-01-01":    {`"from":"2026-09-01"`},
		"?from=2026-09-01&unknown=1&FROM=bad": {`"from":"2026-09-01"`},
		"?from=0000-01-01&to=0000-01-31":      {`"from":"0000-01-01"`},
		"?from=9999-12-01&to=9999-12-31":      {`"to":"9999-12-31"`},
		"?from=1927-01-01&to=2026-12-31":      {`"from":"1927-01-01"`},
		"?to=2026-08-01":                      {`"from":"2026-08-01"`, `"to":"2026-08-01"`},
		"?from=2026-11-22":                    {`"from":"2026-11-22"`, `"to":"2026-11-22"`},
	}
	for _, endpoint := range analysisEdgeEndpoints {
		for query, wants := range accepted {
			rec := getAnalysis(mux, endpoint+query)
			if rec.Code != http.StatusOK {
				t.Fatalf("GET %s%s = %d %s, want 200", endpoint, query, rec.Code, rec.Body)
			}
			for _, want := range wants {
				if !strings.Contains(rec.Body.String(), want) {
					t.Fatalf("GET %s%s: body %.300s lacks %s", endpoint, query, rec.Body, want)
				}
			}
		}
	}

	// A range with one end omitted is checked once the data is loaded.
	for _, endpoint := range analysisEdgeEndpoints {
		wantAnalysisStatus(t, mux, endpoint+"?from=2026-11-23", http.StatusBadRequest, "from must not be after to")
		wantAnalysisStatus(t, mux, endpoint+"?from=1926-11-01", http.StatusBadRequest, "more than 1200 months")
	}
}

func TestLedgerSpendingEndpoint_HostileIDParameters(t *testing.T) {
	h, s := newTestHandler(t)
	pinAnalysisToday(h)
	mux := newMux(h)
	accountID := seedAnalysisEdgeData(t, s)

	for _, name := range []string{"accountId", "parentCategoryId"} {
		for _, value := range []string{"1", "null", "undefined", "00000000-0000-0000-0000-00000000000", "<b>x</b>", "' OR 1=1 --", strings.Repeat("a", 5000)} {
			rec := getAnalysis(mux, "spending?"+name+"="+url.QueryEscape(value))
			if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "invalid "+name) || strings.Contains(rec.Body.String(), value) {
				t.Fatalf("GET spending %s=%.40q = %d %.200s, want 400 without the value", name, value, rec.Code, rec.Body)
			}
		}
		// The nil UUID names no account and no category.
		wantAnalysisStatus(t, mux, "spending?"+name+"="+uuid.Nil.String(), http.StatusNotFound, "not found")
	}
	// An empty value is the same as no parameter; the ID is not case-sensitive.
	for _, query := range []string{"?accountId=&parentCategoryId=", "?accountId=" + strings.ToUpper(accountID.String())} {
		if rec := getAnalysis(mux, "spending"+query); rec.Code != http.StatusOK {
			t.Fatalf("GET spending%s = %d %s, want 200", query, rec.Code, rec.Body)
		}
	}
}

// "Today" is the UTC date whatever zone the clock reports.
func TestLedgerAnalysis_TodayIsTheUTCDate(t *testing.T) {
	h, _ := newTestHandler(t)
	mux := newMux(h)
	for _, now := range []time.Time{
		time.Date(2026, 11, 23, 5, 0, 0, 0, time.FixedZone("ahead", 14*3600)),
		time.Date(2026, 11, 21, 20, 0, 0, 0, time.FixedZone("behind", -12*3600)),
		time.Date(2026, 11, 22, 23, 59, 59, 0, time.UTC),
	} {
		h.now = func() time.Time { return now }
		for _, endpoint := range analysisEdgeEndpoints {
			rec := getAnalysis(mux, endpoint)
			if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"to":"2026-11-22"`) {
				t.Fatalf("GET %s at %s = %d %s, want to = 2026-11-22", endpoint, now, rec.Code, rec.Body)
			}
		}
	}
}

// A store failure is a plain 500: nothing of the cause reaches the client.
func TestLedgerAnalysis_StoreFailure(t *testing.T) {
	logs := &bytes.Buffer{}
	logger := slog.New(slog.NewTextHandler(logs, nil))
	engine, err := storage.Open(t.TempDir(), slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatalf("opening engine: %v", err)
	}
	if err := engine.Close(); err != nil {
		t.Fatalf("closing engine: %v", err)
	}
	h := &Handler{store: NewStore(engine, link.NewRegistry()), logger: logger}
	pinAnalysisToday(h)
	mux := newMux(h)

	for _, endpoint := range analysisEdgeEndpoints {
		logs.Reset()
		rec := getAnalysis(mux, endpoint+"?from=2026-01-01&to=2026-03-31")
		if rec.Code != http.StatusInternalServerError || strings.TrimSpace(rec.Body.String()) != `{"error":"internal error"}` {
			t.Fatalf("GET %s = %d %s, want a plain 500", endpoint, rec.Code, rec.Body)
		}
		if !strings.Contains(logs.String(), "store error") {
			t.Fatalf("GET %s: the failure is not logged: %s", endpoint, logs)
		}
	}
}

// ledgerKeyVersions maps every ledger key of the test user to its version; a
// write to a key changes its version.
func ledgerKeyVersions(t *testing.T, s *testStores) map[string]uint64 {
	t.Helper()
	prefix := []byte("u/" + testUserID + "/")
	versions := map[string]uint64{}
	if err := s.e.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.PrefetchValues = false
		it := txn.NewIterator(opts)
		defer it.Close()
		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			versions[string(it.Item().Key())] = it.Item().Version()
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	return versions
}

// The analyses are read-only and repeatable: the same request gives the same
// bytes and no key is written.
func TestLedgerAnalysis_RepeatsAreIdenticalAndWriteNothing(t *testing.T) {
	h, s := newTestHandler(t)
	pinAnalysisToday(h)
	mux := newMux(h)
	seedAnalysisEdgeData(t, s)
	before := ledgerKeyVersions(t, s)
	if len(before) == 0 {
		t.Fatal("no keys seeded")
	}

	for _, endpoint := range analysisEdgeEndpoints {
		for _, query := range []string{"", "?from=2026-09-05&to=2026-10-20", "?from=bad"} {
			first := getAnalysis(mux, endpoint+query)
			for range 5 {
				again := getAnalysis(mux, endpoint+query)
				if again.Code != first.Code || again.Body.String() != first.Body.String() {
					t.Fatalf("GET %s%s changed between calls:\n%d %s\n%d %s", endpoint, query, first.Code, first.Body, again.Code, again.Body)
				}
			}
		}
	}

	after := ledgerKeyVersions(t, s)
	if len(after) != len(before) {
		t.Fatalf("%d keys before, %d after", len(before), len(after))
	}
	for key, version := range before {
		if after[key] != version {
			t.Fatalf("key %s was written (version %d -> %d)", key, version, after[key])
		}
	}
}

// Requests answer while bookings and balances are written. This is a smoke
// test for runs with -race (task ci runs without it): on its own it only
// asserts that the reads answer 200, and nothing forces them to overlap the
// writes.
func TestLedgerAnalysis_ConcurrentWithWrites(t *testing.T) {
	h, s := newTestHandler(t)
	pinAnalysisToday(h)
	mux := newMux(h)
	accountID := seedAnalysisEdgeData(t, s)

	var readers sync.WaitGroup
	failures := make(chan string, 64)
	for worker := range 4 {
		readers.Add(1)
		go func() {
			defer readers.Done()
			for i := range 15 {
				endpoint := analysisEdgeEndpoints[(worker+i)%len(analysisEdgeEndpoints)]
				if rec := getAnalysis(mux, endpoint); rec.Code != http.StatusOK {
					select {
					case failures <- endpoint + ": " + rec.Body.String():
					default:
					}
				}
			}
		}()
	}
	for i := range 10 {
		addBooking(t, s, accountID, "2026-10-1"+string(rune('0'+i)), -100)
		putSnapshot(t, s, accountID, "2026-10-1"+string(rune('0'+i)), int64(90_000+i), LedgerBalanceSourceManual)
	}
	readers.Wait()
	close(failures)
	for failure := range failures {
		t.Fatalf("request failed during writes: %s", failure)
	}
}
