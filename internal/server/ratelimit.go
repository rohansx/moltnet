package server

import (
	"fmt"
	"net"
	"net/http"
	"strings"
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
	rate    float64          // tokens per second
	now     func() time.Time // injectable for tests
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

// parseTrustedProxies turns CIDR strings (or bare IPs) into networks. Used to
// decide whose X-Forwarded-For we believe.
func parseTrustedProxies(cidrs []string) ([]*net.IPNet, error) {
	var out []*net.IPNet
	for _, c := range cidrs {
		c = strings.TrimSpace(c)
		if c == "" {
			continue
		}
		if _, n, err := net.ParseCIDR(c); err == nil {
			out = append(out, n)
			continue
		}
		// Bare IP: treat as a single-host network.
		ip := net.ParseIP(c)
		if ip == nil {
			return nil, fmt.Errorf("trusted proxy %q is not an IP or CIDR", c)
		}
		bits := 32
		if ip.To4() == nil {
			bits = 128
		}
		out = append(out, &net.IPNet{IP: ip, Mask: net.CIDRMask(bits, bits)})
	}
	return out, nil
}

func isTrusted(ip net.IP, trusted []*net.IPNet) bool {
	for _, n := range trusted {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}

// clientIP extracts the key the rate limiter buckets on.
//
// X-Forwarded-For is attacker-controlled: any caller can set it, and honouring
// it unconditionally hands every request a fresh bucket, which silently removes
// the only cost control on unauthenticated writes. So it is believed ONLY when
// the immediate peer is a configured trusted proxy (--trusted-proxy).
//
// When it is believed, we walk the list from the RIGHT and take the last entry
// that is not itself a trusted hop. Proxies append the peer they saw, so the
// rightmost entries are the ones our own infrastructure wrote; anything further
// left may have been forged by the client.
func clientIP(r *http.Request, trusted []*net.IPNet) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	peer := net.ParseIP(host)
	if len(trusted) == 0 || peer == nil || !isTrusted(peer, trusted) {
		return host // untrusted peer: its own address is the only honest signal
	}
	xff := r.Header.Get("X-Forwarded-For")
	if xff == "" {
		return host
	}
	parts := strings.Split(xff, ",")
	for i := len(parts) - 1; i >= 0; i-- {
		ip := net.ParseIP(strings.TrimSpace(parts[i]))
		if ip == nil {
			continue
		}
		if !isTrusted(ip, trusted) {
			return ip.String()
		}
	}
	return host // every hop was a trusted proxy; fall back to the peer
}

// rateLimitWrites wraps a handler, limiting only mutating (POST/PUT/PATCH/DELETE)
// requests per client IP. Reads (GET) are never limited so verify/search stay
// open. A nil limiter passes everything through.
func rateLimitWrites(rl *rateLimiter, trusted []*net.IPNet, h http.Handler) http.Handler {
	if rl == nil {
		return h
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
			if !rl.allow(clientIP(r, trusted)) {
				w.Header().Set("Retry-After", "60")
				writeErr(w, http.StatusTooManyRequests, "rate limit exceeded")
				return
			}
		}
		h.ServeHTTP(w, r)
	})
}
