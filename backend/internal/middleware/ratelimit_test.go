package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func newLimitedHandler(limit int, window time.Duration, trustProxy bool) http.Handler {
	ok := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	return RateLimitPerIP(limit, window, trustProxy)(ok)
}

func doRequest(h http.Handler, remoteAddr string, headers map[string]string) int {
	req := httptest.NewRequest("POST", "/auth/login", nil)
	req.RemoteAddr = remoteAddr
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec.Code
}

func TestRateLimit_BlocksAfterLimit(t *testing.T) {
	h := newLimitedHandler(3, time.Minute, false)

	for i := 0; i < 3; i++ {
		if code := doRequest(h, "1.2.3.4:1234", nil); code != http.StatusOK {
			t.Fatalf("request %d: status = %d, want %d", i+1, code, http.StatusOK)
		}
	}
	if code := doRequest(h, "1.2.3.4:1234", nil); code != http.StatusTooManyRequests {
		t.Fatalf("request over limit: status = %d, want %d", code, http.StatusTooManyRequests)
	}
}

func TestRateLimit_IsolatesIPs(t *testing.T) {
	h := newLimitedHandler(1, time.Minute, false)

	if code := doRequest(h, "1.2.3.4:1234", nil); code != http.StatusOK {
		t.Fatalf("first IP: status = %d, want %d", code, http.StatusOK)
	}
	if code := doRequest(h, "1.2.3.4:9999", nil); code != http.StatusTooManyRequests {
		t.Fatalf("same IP, different port: status = %d, want %d", code, http.StatusTooManyRequests)
	}
	if code := doRequest(h, "5.6.7.8:1234", nil); code != http.StatusOK {
		t.Fatalf("other IP: status = %d, want %d", code, http.StatusOK)
	}
}

func TestRateLimit_RefillsAfterWindow(t *testing.T) {
	rl := &ipRateLimiter{
		buckets: make(map[string]*bucket),
		rate:    2.0 / 60.0, // 2 per minute
		burst:   2,
		idleTTL: time.Minute,
	}
	now := time.Now()

	for i := 0; i < 2; i++ {
		if !rl.allow("ip", now) {
			t.Fatalf("expected burst request %d to be allowed", i+1)
		}
	}
	if rl.allow("ip", now) {
		t.Fatal("expected third request to be blocked")
	}
	if !rl.allow("ip", now.Add(31*time.Second)) {
		t.Fatal("expected a token after half the window")
	}
	if rl.allow("ip", now.Add(31*time.Second)) {
		t.Fatal("expected only one token to have refilled")
	}
}

func TestRateLimit_SweepDropsIdleBuckets(t *testing.T) {
	rl := &ipRateLimiter{
		buckets: make(map[string]*bucket),
		rate:    1.0 / 60.0,
		burst:   1,
		idleTTL: time.Minute,
	}
	now := time.Now()
	rl.lastSweep = now

	rl.allow("a", now)
	rl.allow("b", now.Add(2*time.Minute))
	rl.allow("c", now.Add(3*time.Minute)) // triggers sweep of "a" and "b"

	if _, ok := rl.buckets["a"]; ok {
		t.Error("expected idle bucket 'a' to be swept")
	}
	if _, ok := rl.buckets["c"]; !ok {
		t.Error("expected active bucket 'c' to remain")
	}
}

func TestRateLimit_Disabled(t *testing.T) {
	h := newLimitedHandler(0, time.Minute, false)

	for i := 0; i < 20; i++ {
		if code := doRequest(h, "1.2.3.4:1234", nil); code != http.StatusOK {
			t.Fatalf("request %d: status = %d, want %d", i+1, code, http.StatusOK)
		}
	}
}

func TestRateLimit_TrustProxy(t *testing.T) {
	h := newLimitedHandler(1, time.Minute, true)

	if code := doRequest(h, "10.0.0.1:1234", map[string]string{"X-Real-IP": "1.1.1.1"}); code != http.StatusOK {
		t.Fatalf("first client: status = %d, want %d", code, http.StatusOK)
	}
	if code := doRequest(h, "10.0.0.1:1234", map[string]string{"X-Real-IP": "1.1.1.1"}); code != http.StatusTooManyRequests {
		t.Fatalf("first client again: status = %d, want %d", code, http.StatusTooManyRequests)
	}
	if code := doRequest(h, "10.0.0.1:1234", map[string]string{"X-Real-IP": "2.2.2.2"}); code != http.StatusOK {
		t.Fatalf("second client via same proxy: status = %d, want %d", code, http.StatusOK)
	}
}

func TestRateLimit_IgnoresProxyHeadersByDefault(t *testing.T) {
	h := newLimitedHandler(1, time.Minute, false)

	if code := doRequest(h, "10.0.0.1:1234", map[string]string{"X-Real-IP": "1.1.1.1"}); code != http.StatusOK {
		t.Fatalf("first request: status = %d, want %d", code, http.StatusOK)
	}
	// Spoofed header must not reset the budget when proxy headers are untrusted.
	if code := doRequest(h, "10.0.0.1:1234", map[string]string{"X-Real-IP": "2.2.2.2"}); code != http.StatusTooManyRequests {
		t.Fatalf("spoofed header: status = %d, want %d", code, http.StatusTooManyRequests)
	}
}

func TestClientIP_ForwardedFor(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "10.0.0.1:1234"
	req.Header.Set("X-Forwarded-For", "1.1.1.1, 2.2.2.2")

	if ip := clientIP(req, true); ip != "2.2.2.2" {
		t.Errorf("trusted: ip = %q, want last XFF entry %q", ip, "2.2.2.2")
	}
	if ip := clientIP(req, false); ip != "10.0.0.1" {
		t.Errorf("untrusted: ip = %q, want %q", ip, "10.0.0.1")
	}
}
