package middleware

import (
	"encoding/json"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// RateLimitPerIP limits each client IP to limit requests per window using a
// token bucket. A limit <= 0 disables the middleware. With trustProxy set,
// the client IP is taken from X-Real-IP / X-Forwarded-For (only enable when
// running behind a reverse proxy that sets these headers).
func RateLimitPerIP(limit int, window time.Duration, trustProxy bool) Middleware {
	if limit <= 0 {
		return func(next http.Handler) http.Handler { return next }
	}

	rl := &ipRateLimiter{
		buckets: make(map[string]*bucket),
		rate:    float64(limit) / window.Seconds(),
		burst:   float64(limit),
		idleTTL: window,
	}
	retryAfter := strconv.Itoa(int(time.Duration(float64(time.Second)/rl.rate).Round(time.Second).Seconds()))

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !rl.allow(clientIP(r, trustProxy), time.Now()) {
				w.Header().Set("Content-Type", "application/json")
				w.Header().Set("Retry-After", retryAfter)
				w.WriteHeader(http.StatusTooManyRequests)
				json.NewEncoder(w).Encode(map[string]string{"error": "too many requests, try again later"})
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

type ipRateLimiter struct {
	mu        sync.Mutex
	buckets   map[string]*bucket
	rate      float64 // tokens per second
	burst     float64
	idleTTL   time.Duration
	lastSweep time.Time
}

type bucket struct {
	tokens float64
	last   time.Time
}

func (rl *ipRateLimiter) allow(ip string, now time.Time) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	if now.Sub(rl.lastSweep) > rl.idleTTL {
		rl.sweep(now)
		rl.lastSweep = now
	}

	b, ok := rl.buckets[ip]
	if !ok {
		b = &bucket{tokens: rl.burst, last: now}
		rl.buckets[ip] = b
	}

	b.tokens = min(rl.burst, b.tokens+now.Sub(b.last).Seconds()*rl.rate)
	b.last = now
	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}

// sweep drops buckets idle long enough to have fully refilled.
func (rl *ipRateLimiter) sweep(now time.Time) {
	for ip, b := range rl.buckets {
		if now.Sub(b.last) > rl.idleTTL {
			delete(rl.buckets, ip)
		}
	}
}

func clientIP(r *http.Request, trustProxy bool) string {
	if trustProxy {
		if ip := r.Header.Get("X-Real-IP"); ip != "" {
			return strings.TrimSpace(ip)
		}
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			parts := strings.Split(xff, ",")
			return strings.TrimSpace(parts[len(parts)-1])
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
