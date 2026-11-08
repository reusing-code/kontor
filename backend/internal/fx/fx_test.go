package fx

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

var today = time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC)

func newTestFrankfurter(t *testing.T, handler http.HandlerFunc) (*Frankfurter, *atomic.Int32) {
	t.Helper()
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		handler(w, r)
	}))
	t.Cleanup(srv.Close)
	f := NewFrankfurter(srv.URL + "/v1/")
	f.now = func() time.Time { return today }
	return f, &calls
}

func TestFrankfurter_RateAndCache(t *testing.T) {
	var gotPath, gotQuery string
	f, calls := newTestFrankfurter(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotQuery = r.URL.Path, r.URL.RawQuery
		w.Write([]byte(`{"amount":1.0,"base":"USD","date":"2026-10-30","rates":{"EUR":0.89509}}`))
	})
	date := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)

	r, err := f.Rate(t.Context(), "USD", "EUR", date)
	if err != nil {
		t.Fatal(err)
	}
	if r != (Rate{Value: 0.89509, Date: "2026-10-30"}) {
		t.Errorf("rate = %+v", r)
	}
	if gotPath != "/v1/2026-11-01" || gotQuery != "base=USD&symbols=EUR" {
		t.Errorf("request = %s?%s", gotPath, gotQuery)
	}
	if _, err := f.Rate(t.Context(), "USD", "EUR", date); err != nil || calls.Load() != 1 {
		t.Errorf("past dates must be cached: calls = %d, err = %v", calls.Load(), err)
	}
}

func TestFrankfurter_ClampsFutureDatesAndSkipsCacheForToday(t *testing.T) {
	var gotPath string
	f, calls := newTestFrankfurter(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Write([]byte(`{"date":"2026-11-06","rates":{"EUR":0.9}}`))
	})
	future := time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)
	for range 2 {
		if _, err := f.Rate(t.Context(), "USD", "EUR", future); err != nil {
			t.Fatal(err)
		}
	}
	if gotPath != "/v1/2026-11-08" {
		t.Errorf("path = %s, want today", gotPath)
	}
	if calls.Load() != 2 {
		t.Errorf("today's rate must not be cached: calls = %d", calls.Load())
	}
}

func TestFrankfurter_Errors(t *testing.T) {
	tests := []struct {
		name    string
		status  int
		body    string
		unknown bool
	}{
		{name: "not found", status: http.StatusNotFound, body: `{"message":"not found"}`, unknown: true},
		{name: "missing symbol", status: http.StatusOK, body: `{"date":"2026-11-06","rates":{}}`, unknown: true},
		{name: "server error", status: http.StatusInternalServerError},
		{name: "malformed", status: http.StatusOK, body: `nope`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, _ := newTestFrankfurter(t, func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tt.status)
				w.Write([]byte(tt.body))
			})
			_, err := f.Rate(t.Context(), "XYZ", "EUR", today)
			if err == nil {
				t.Fatal("expected an error")
			}
			if errors.Is(err, ErrUnknownCurrency) != tt.unknown {
				t.Errorf("err = %v, unknown = %v", err, tt.unknown)
			}
		})
	}
}

func TestDisabled(t *testing.T) {
	if _, err := (Disabled{}).Rate(t.Context(), "USD", "EUR", today); !errors.Is(err, ErrDisabled) {
		t.Errorf("err = %v", err)
	}
}
