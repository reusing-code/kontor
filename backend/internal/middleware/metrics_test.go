package middleware

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestStatusRecorderFlushes(t *testing.T) {
	under := httptest.NewRecorder()
	var rec http.ResponseWriter = &statusRecorder{ResponseWriter: under, status: http.StatusOK}

	if _, ok := rec.(http.Flusher); !ok {
		t.Fatal("statusRecorder is not an http.Flusher")
	}
	if err := http.NewResponseController(rec).Flush(); err != nil {
		t.Fatalf("ResponseController.Flush: %v", err)
	}
	if !under.Flushed {
		t.Fatal("flush did not reach the underlying writer")
	}
}

func TestStreamingThroughLoggingAndMetrics(t *testing.T) {
	h := Chain(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte("data: 1\n\n"))
		if err := http.NewResponseController(w).Flush(); err != nil {
			t.Errorf("Flush through the chain: %v", err)
		}
	}), Metrics, Logging(slog.New(slog.DiscardHandler)))
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/", nil))
}
