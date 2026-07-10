package server

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/moltnet/moltnet/core"
	"github.com/moltnet/moltnet/internal/store"
)

// testEnv spins up an in-memory registry behind an httptest server.
func testEnv(t *testing.T) (*httptest.Server, func()) {
	t.Helper()
	st, err := store.Open(":memory:")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	srv := &Server{Store: st, Name: "test", Version: "test"}
	ts := httptest.NewServer(srv.Handler())
	return ts, func() { ts.Close(); st.Close() }
}

func mustCard(t *testing.T, owner, agent *core.KeyPair, name string, caps ...string) *core.Card {
	t.Helper()
	c := core.NewCard(agent.DID, owner.DID, name)
	for _, cap := range caps {
		c.Capabilities = append(c.Capabilities, core.Capability{Tag: cap})
	}
	if err := c.Sign(agent.Private, owner.Private); err != nil {
		t.Fatal(err)
	}
	return c
}

func postJSON(t *testing.T, url string, v any) (int, []byte) {
	t.Helper()
	data, _ := json.Marshal(v)
	resp, err := http.Post(url, "application/json", bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, body
}

func getJSON(t *testing.T, url string, out any) int {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if out != nil {
		_ = json.Unmarshal(body, out)
	}
	return resp.StatusCode
}

func TestRegisterAttestScoreFlow(t *testing.T) {
	ts, cleanup := testEnv(t)
	defer cleanup()

	owner, _ := core.GenerateKeyPair()
	agent, _ := core.GenerateKeyPair()
	issuer, _ := core.GenerateKeyPair()

	// Register subject agent.
	if code, body := postJSON(t, ts.URL+"/v1/agents", mustCard(t, owner, agent, "subject", "code.review")); code != 201 {
		t.Fatalf("register subject: got %d: %s", code, body)
	}
	// Register issuer as an agent too.
	if code, _ := postJSON(t, ts.URL+"/v1/agents", mustCard(t, owner, issuer, "issuer")); code != 201 {
		t.Fatalf("register issuer: %d", code)
	}

	// Tampered card must be rejected.
	bad := mustCard(t, owner, agent, "subject", "code.review")
	bad.Name = "tampered"
	if code, _ := postJSON(t, ts.URL+"/v1/agents", bad); code != 400 {
		t.Fatalf("expected 400 for tampered card, got %d", code)
	}

	// Build a task.completed attestation with the correct prev (empty = first).
	subjCard := mustCard(t, owner, agent, "subject", "code.review")
	subjHash, _ := subjCard.Hash()
	a := core.NewAttestation(core.TypeTaskCompleted, issuer.DID, agent.DID)
	a.SubjectCard = subjHash
	a.Prev = "" // issuer's first attestation
	a.Body = map[string]any{"outcome": "success", "capability": "code.review"}
	if err := a.Sign(issuer.Private); err != nil {
		t.Fatal(err)
	}
	if code, body := postJSON(t, ts.URL+"/v1/attestations", a); code != 201 {
		t.Fatalf("attest: got %d: %s", code, body)
	}

	// Re-submitting the SAME issuer's next attestation with a stale prev must 409.
	a2 := core.NewAttestation(core.TypeTaskCompleted, issuer.DID, agent.DID)
	a2.Prev = "" // wrong: head has advanced
	if err := a2.Sign(issuer.Private); err != nil {
		t.Fatal(err)
	}
	if code, _ := postJSON(t, ts.URL+"/v1/attestations", a2); code != http.StatusConflict {
		t.Fatalf("expected 409 for stale prev, got %d", code)
	}

	// Score should now reflect one completion.
	var scoreResp struct {
		Score  float64 `json:"score"`
		Inputs struct {
			Completions     int `json:"completions"`
			DistinctIssuers int `json:"distinct_issuers"`
		} `json:"inputs"`
	}
	if code := getJSON(t, ts.URL+"/v1/score/"+agent.DID, &scoreResp); code != 200 {
		t.Fatalf("score: %d", code)
	}
	if scoreResp.Inputs.Completions != 1 {
		t.Fatalf("expected 1 completion, got %d", scoreResp.Inputs.Completions)
	}
	if scoreResp.Score <= 0 {
		t.Fatalf("expected positive score, got %v", scoreResp.Score)
	}

	// Search should find the subject by capability.
	var searchResp struct {
		Count int `json:"count"`
	}
	getJSON(t, ts.URL+"/v1/search?cap=code.review", &searchResp)
	if searchResp.Count < 1 {
		t.Fatalf("search by capability returned %d", searchResp.Count)
	}

	// Badge endpoint returns SVG.
	resp, _ := http.Get(ts.URL + "/v1/agents/" + agent.DID + "/badge.svg")
	if resp.StatusCode != 200 || resp.Header.Get("Content-Type") != "image/svg+xml" {
		t.Fatalf("badge: status %d type %q", resp.StatusCode, resp.Header.Get("Content-Type"))
	}
	resp.Body.Close()

	// Federation change feed should contain card + attestation events.
	var feed struct {
		Events []struct {
			Kind string `json:"kind"`
		} `json:"events"`
	}
	getJSON(t, ts.URL+"/federation/changes?since=0", &feed)
	var cards, atts int
	for _, e := range feed.Events {
		switch e.Kind {
		case "card":
			cards++
		case "attestation":
			atts++
		}
	}
	if cards < 2 || atts < 1 {
		t.Fatalf("federation feed: cards=%d atts=%d", cards, atts)
	}
}

func TestUnknownAgentIs404(t *testing.T) {
	ts, cleanup := testEnv(t)
	defer cleanup()
	if code := getJSON(t, ts.URL+"/v1/agents/did:key:zNope", nil); code != 404 {
		t.Fatalf("expected 404, got %d", code)
	}
}
