package server

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/moltnet/moltnet/core"
)

// Federation is pull-based: a follower requests a peer's signed change feed and
// re-verifies every record on ingest, so following a peer transports data
// without transferring trust.

func (s *Server) handleFederationChanges(w http.ResponseWriter, r *http.Request) {
	since, _ := strconv.ParseInt(r.URL.Query().Get("since"), 10, 64)
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	events, err := s.Store.Changes(since, limit)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	latest, _ := s.Store.LatestSeq()
	cursor := since
	if n := len(events); n > 0 {
		cursor = events[n-1].Seq
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"events": events,
		"cursor": cursor, // pass back as ?since= on the next pull
		"latest": latest, // caller has caught up when cursor == latest
	})
}

func (s *Server) handleFederationPeers(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"peers": s.Peers})
}

// StartFederation launches a background loop that pulls signed change feeds from
// each configured peer, re-verifies every record, and ingests idempotently.
func (s *Server) StartFederation(interval time.Duration) {
	if interval <= 0 || len(s.Peers) == 0 {
		return
	}
	go func() {
		time.Sleep(3 * time.Second)
		for {
			for _, peer := range s.Peers {
				if err := s.syncPeer(peer); err != nil {
					fmt.Printf("federation: peer %s: %v\n", peer, err)
				}
			}
			time.Sleep(interval)
		}
	}()
}

var fedClient = &http.Client{Timeout: 15 * time.Second}

// syncPeer pulls new events from one peer and ingests them, advancing the stored
// cursor. It returns after catching up (or on error).
func (s *Server) syncPeer(peer string) error {
	for {
		cursor, err := s.Store.GetPeerCursor(peer)
		if err != nil {
			return err
		}
		url := fmt.Sprintf("%s/federation/changes?since=%d&limit=200", peer, cursor)
		resp, err := fedClient.Get(url)
		if err != nil {
			return err
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode >= 300 {
			return fmt.Errorf("%s: %s", resp.Status, string(body))
		}
		var feed struct {
			Events []struct {
				Seq    int64           `json:"seq"`
				Kind   string          `json:"kind"`
				Record json.RawMessage `json:"record"`
			} `json:"events"`
			Latest int64 `json:"latest"`
		}
		if err := json.Unmarshal(body, &feed); err != nil {
			return err
		}
		if len(feed.Events) == 0 {
			return nil // caught up
		}
		for _, ev := range feed.Events {
			s.ingestFederated(ev.Kind, ev.Record)
			if err := s.Store.SetPeerCursor(peer, ev.Seq); err != nil {
				return err
			}
		}
	}
}

// ingestFederated re-verifies a synced record's signatures and stores it. Chain
// head is NOT enforced here (unlike direct writes): a peer's records arrive in
// its own order, and per-issuer chains are validated by readers over the full
// set. A record that fails signature verification is dropped.
func (s *Server) ingestFederated(kind string, record json.RawMessage) {
	switch kind {
	case "card":
		var c core.Card
		if json.Unmarshal(record, &c) != nil || c.Verify() != nil {
			return
		}
		if changed, _ := s.Store.PutCard(&c); changed {
			_, _ = s.recomputeScore(c.ID)
		}
	case "attestation":
		var a core.Attestation
		if json.Unmarshal(record, &a) != nil || a.Verify() != nil {
			return
		}
		if inserted, _ := s.Store.PutAttestation(&a); inserted {
			_, _ = s.recomputeScore(a.Subject)
		}
	case "rotation":
		var rot core.Rotation
		if json.Unmarshal(record, &rot) != nil || rot.Verify() != nil {
			return
		}
		// Only accept the rotation if the local old-agent card owner matches.
		if oldCard, _ := s.Store.GetCard(rot.OldAgent); oldCard != nil && oldCard.Owner == rot.Owner {
			_, _ = s.Store.PutRotation(&rot)
		}
	}
}
