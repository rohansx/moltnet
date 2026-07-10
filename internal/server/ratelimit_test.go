package server

import (
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
