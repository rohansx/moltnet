package server

import (
	"net"
	"net/http"
	"sync"
	"time"
)

// rateLimiter is a per-key token-bucket limiter. Trust in MoltNet lives in
// signatures, not sessions, so this is purely abuse control on write volume — a
// registry may enable it without affecting the trust model.
type rateLimiter struct {
	mu      sync.Mutex
	buckets map[string]*bucket
	burst   float64
	rate    float64             // tokens per second
	now     func() time.Time    // injectable for tests
}

type bucket struct {
	tokens float64
	last   time.Time
}

func newRateLimiter(burst int, ratePerSec float64) *rateLimiter {
	return &rateLimiter{
		buckets: make(map[string]*bucket),
		burst:   float64(burst),
		rate:    ratePerSec,
		now:     time.Now,
	}
}

// allow consumes one token for key, refilling first. Returns false when empty.
func (rl *rateLimiter) allow(key string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	now := rl.now()
	b, ok := rl.buckets[key]
	if !ok {
		rl.buckets[key] = &bucket{tokens: rl.burst - 1, last: now}
		return true
	}
	// Refill based on elapsed time, capped at burst.
	elapsed := now.Sub(b.last).Seconds()
	if elapsed > 0 {
		b.tokens = min(rl.burst, b.tokens+elapsed*rl.rate)
		b.last = now
	}
	if b.tokens >= 1 {
		b.tokens -= 1
		return true
	}
	return false
}

// clientIP extracts the caller's IP, honouring X-Forwarded-For when present.
func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		// first entry is the original client
		if i := indexByte(xff, ','); i >= 0 {
			return trimSpace(xff[:i])
		}
		return trimSpace(xff)
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func indexByte(s string, c byte) int {
	for i := 0; i < len(s); i++ {
		if s[i] == c {
			return i
		}
	}
	return -1
}

func trimSpace(s string) string {
	start, end := 0, len(s)
	for start < end && (s[start] == ' ' || s[start] == '\t') {
		start++
	}
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t') {
		end--
	}
	return s[start:end]
}

// rateLimitWrites wraps a handler, limiting only mutating (POST/PUT/PATCH/DELETE)
// requests per client IP. Reads (GET) are never limited so verify/search stay
// open. A nil limiter passes everything through.
func rateLimitWrites(rl *rateLimiter, h http.Handler) http.Handler {
	if rl == nil {
		return h
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
			if !rl.allow(clientIP(r)) {
				w.Header().Set("Retry-After", "60")
				writeErr(w, http.StatusTooManyRequests, "rate limit exceeded")
				return
			}
		}
		h.ServeHTTP(w, r)
	})
}
