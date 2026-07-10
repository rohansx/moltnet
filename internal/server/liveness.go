package server

import (
	"context"
	"net/http"
	"time"
)

// probeClient is used for liveness checks. It does not follow the request body
// and has a tight timeout so a slow endpoint cannot stall the prober.
var probeClient = &http.Client{Timeout: 8 * time.Second}

// probeOne performs a single liveness probe and records the result.
func (s *Server) probeOne(did, url string) {
	start := time.Now()
	reachable := false
	status := 0

	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err == nil {
		resp, derr := probeClient.Do(req)
		if derr == nil {
			status = resp.StatusCode
			reachable = resp.StatusCode < 500
			resp.Body.Close()
		}
	}
	latency := int(time.Since(start).Milliseconds())
	_ = s.Store.RecordLiveness(did, reachable, status, latency, time.Now().UTC().Format(time.RFC3339))
}

// StartLivenessProber launches a background loop that probes every enabled
// agent endpoint on the given interval. A non-positive interval disables it.
func (s *Server) StartLivenessProber(interval time.Duration) {
	if interval <= 0 {
		return
	}
	go func() {
		// Initial delay lets the server finish starting before the first sweep.
		time.Sleep(2 * time.Second)
		for {
			s.probeSweep()
			time.Sleep(interval)
		}
	}()
}

func (s *Server) probeSweep() {
	targets, err := s.Store.EnabledLivenessTargets()
	if err != nil {
		return
	}
	for _, t := range targets {
		s.probeOne(t.DID, t.URL)
	}
}
