package server

import (
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/moltnet/moltnet/internal/store"
)

func TestRateLimiterBurstThenBlockThenRefill(t *testing.T) {
	now := time.Unix(1000, 0)
	rl := newRateLimiter(2, 1.0) // burst 2, refill 1 token/sec
	rl.now = func() time.Time { return now }

	ip := "1.2.3.4"
	if !rl.allow(ip) || !rl.allow(ip) {
		t.Fatal("first two requests within burst should be allowed")
	}
	if rl.allow(ip) {
		t.Fatal("third request should be blocked (burst exhausted)")
	}

	// A separate IP has its own independent bucket.
	if !rl.allow("5.6.7.8") {
		t.Fatal("a different IP should not be affected")
	}

	// After one second, one token refills.
	now = now.Add(time.Second)
	if !rl.allow(ip) {
		t.Fatal("should be allowed after a 1s refill")
	}
	if rl.allow(ip) {
		t.Fatal("should be blocked again after consuming the refilled token")
	}

	// Refill is capped at the burst size (no unbounded accumulation).
	now = now.Add(10 * time.Second)
	allowed := 0
	for i := 0; i < 5; i++ {
		if rl.allow(ip) {
			allowed++
		}
	}
	if allowed != 2 {
		t.Fatalf("refill should cap at burst=2, got %d allowed", allowed)
	}
}

// An untrusted caller must not be able to mint a fresh token bucket per request
// by inventing an X-Forwarded-For. The limiter is the only cost control on
// unauthenticated writes (challenges, attestations), so spoofable keying makes
// it decorative.
func TestClientIPIgnoresForwardedForFromUntrustedPeer(t *testing.T) {
	r := httptest.NewRequest("POST", "/v1/attestations", nil)
	r.RemoteAddr = "203.0.113.9:5555"
	r.Header.Set("X-Forwarded-For", "10.9.9.9")

	if got := clientIP(r, nil); got != "203.0.113.9" {
		t.Fatalf("with no trusted proxies, clientIP must use RemoteAddr; got %q", got)
	}
}

// Behind a real proxy (moltnet.ai runs behind Traefik) the header is the ONLY
// way to see the caller. Ignoring it unconditionally would bucket the whole
// internet as one client, so a configured proxy must still be honoured.
func TestClientIPHonoursForwardedForFromTrustedProxy(t *testing.T) {
	trusted := mustCIDRs(t, []string{"172.16.0.0/12"})
	r := httptest.NewRequest("POST", "/v1/attestations", nil)
	r.RemoteAddr = "172.18.0.5:5555" // the docker-network proxy
	r.Header.Set("X-Forwarded-For", "198.51.100.7, 172.18.0.5")

	if got := clientIP(r, trusted); got != "198.51.100.7" {
		t.Fatalf("trusted proxy's XFF should yield the original client; got %q", got)
	}
}

// A spoofer behind the trusted proxy prepends their own entry. The rightmost
// entries are appended by infrastructure we trust, so walk from the right and
// take the last address that isn't itself a trusted hop.
func TestClientIPResistsSpoofedPrefixBehindTrustedProxy(t *testing.T) {
	trusted := mustCIDRs(t, []string{"172.16.0.0/12"})
	r := httptest.NewRequest("POST", "/v1/attestations", nil)
	r.RemoteAddr = "172.18.0.5:5555"
	// Attacker sent "X-Forwarded-For: 1.1.1.1"; Traefik appended the real peer.
	r.Header.Set("X-Forwarded-For", "1.1.1.1, 198.51.100.7")

	if got := clientIP(r, trusted); got != "198.51.100.7" {
		t.Fatalf("must use the last untrusted hop, not the attacker's forged prefix; got %q", got)
	}
}

// End to end: spoofing the header must not buy extra write budget.
func TestRateLimitNotBypassableByForgedForwardedFor(t *testing.T) {
	st, _ := store.Open(":memory:")
	defer st.Close()
	srv := &Server{Store: st, Name: "t", Version: "t", RateLimitPerMin: 3}
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	got429 := false
	for i := 0; i < 8; i++ {
		req, _ := http.NewRequest("POST", ts.URL+"/v1/agents", strings.NewReader("{}"))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Forwarded-For", fmt.Sprintf("10.9.%d.%d", i, i)) // a new "IP" each time
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode == http.StatusTooManyRequests {
			got429 = true
		}
	}
	if !got429 {
		t.Fatal("forged X-Forwarded-For bypassed the write rate limit")
	}
}

func mustCIDRs(t *testing.T, s []string) []*net.IPNet {
	t.Helper()
	n, err := parseTrustedProxies(s)
	if err != nil {
		t.Fatalf("parseTrustedProxies: %v", err)
	}
	return n
}

func TestRateLimitReturns429OnWritesButNotReads(t *testing.T) {
	st, _ := store.Open(":memory:")
	defer st.Close()
	srv := &Server{Store: st, Name: "t", Version: "t", RateLimitPerMin: 3}
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	// Reads must never be limited.
	for i := 0; i < 10; i++ {
		resp, err := http.Get(ts.URL + "/v1/taxonomy")
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode == http.StatusTooManyRequests {
			t.Fatal("GET should never be rate limited")
		}
	}

	// Writes: burst is 3, so the 4th+ POST from the same IP should 429.
	var got429 bool
	for i := 0; i < 6; i++ {
		resp, err := http.Post(ts.URL+"/v1/agents", "application/json", strings.NewReader("{}"))
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode == http.StatusTooManyRequests {
			got429 = true
		}
	}
	if !got429 {
		t.Fatal("expected at least one 429 after exceeding the write burst")
	}
}
