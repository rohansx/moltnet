package server

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

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

func TestKeyRotation(t *testing.T) {
	ts, cleanup := testEnv(t)
	defer cleanup()

	owner, _ := core.GenerateKeyPair()
	oldAgent, _ := core.GenerateKeyPair()
	newAgent, _ := core.GenerateKeyPair()
	attacker, _ := core.GenerateKeyPair()

	// Register the original agent, owned by `owner`.
	if code, body := postJSON(t, ts.URL+"/v1/agents", mustCard(t, owner, oldAgent, "rotating-agent")); code != 201 {
		t.Fatalf("register: %d %s", code, body)
	}

	// A rotation signed by someone who is NOT the card's owner must be rejected.
	evil := core.NewRotation(attacker.DID, oldAgent.DID, newAgent.DID)
	if err := evil.Sign(attacker.Private); err != nil {
		t.Fatal(err)
	}
	if code, _ := postJSON(t, ts.URL+"/v1/rotations", evil); code == 201 {
		t.Fatal("rotation by non-owner must not be accepted")
	}

	// The real owner rotates old -> new.
	rot := core.NewRotation(owner.DID, oldAgent.DID, newAgent.DID)
	if err := rot.Sign(owner.Private); err != nil {
		t.Fatal(err)
	}
	if code, body := postJSON(t, ts.URL+"/v1/rotations", rot); code != 201 {
		t.Fatalf("rotation by owner: %d %s", code, body)
	}

	// GET on the old DID should surface that it rotated to the new DID.
	var resp struct {
		RotatedTo string `json:"rotated_to"`
	}
	if code := getJSON(t, ts.URL+"/v1/agents/"+oldAgent.DID, &resp); code != 200 {
		t.Fatalf("get old agent: %d", code)
	}
	if resp.RotatedTo != newAgent.DID {
		t.Fatalf("expected rotated_to=%s, got %q", newAgent.DID, resp.RotatedTo)
	}
}

func TestGraphEndpoint(t *testing.T) {
	ts, cleanup := testEnv(t)
	defer cleanup()

	owner, _ := core.GenerateKeyPair()
	subject, _ := core.GenerateKeyPair()
	issuer, _ := core.GenerateKeyPair()

	postJSON(t, ts.URL+"/v1/agents", mustCard(t, owner, subject, "subject", "code.review"))
	postJSON(t, ts.URL+"/v1/agents", mustCard(t, owner, issuer, "issuer"))

	// Two completed tasks from issuer -> subject should collapse to one weighted edge.
	head := ""
	for i := 0; i < 2; i++ {
		a := core.NewAttestation(core.TypeTaskCompleted, issuer.DID, subject.DID)
		a.Prev = head
		if err := a.Sign(issuer.Private); err != nil {
			t.Fatal(err)
		}
		if code, body := postJSON(t, ts.URL+"/v1/attestations", a); code != 201 {
			t.Fatalf("attest %d: %d %s", i, code, body)
		}
		head, _ = a.Hash()
	}

	var graph struct {
		Nodes []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"nodes"`
		Edges []struct {
			Source string `json:"source"`
			Target string `json:"target"`
			Type   string `json:"type"`
			Count  int    `json:"count"`
		} `json:"edges"`
	}
	if code := getJSON(t, ts.URL+"/v1/graph", &graph); code != 200 {
		t.Fatalf("graph: %d", code)
	}
	if len(graph.Nodes) < 2 {
		t.Fatalf("expected >=2 nodes, got %d", len(graph.Nodes))
	}
	var found bool
	for _, e := range graph.Edges {
		if e.Source == issuer.DID && e.Target == subject.DID && e.Type == core.TypeTaskCompleted {
			found = true
			if e.Count != 2 {
				t.Fatalf("expected edge count 2, got %d", e.Count)
			}
		}
	}
	if !found {
		t.Fatalf("expected an issuer->subject task.completed edge; edges=%+v", graph.Edges)
	}
}

func TestCardForkSurfaced(t *testing.T) {
	ts, cleanup := testEnv(t)
	defer cleanup()

	owner, _ := core.GenerateKeyPair()
	agent, _ := core.GenerateKeyPair()

	genesis := mustCard(t, owner, agent, "genesis")
	if code, _ := postJSON(t, ts.URL+"/v1/agents", genesis); code != 201 {
		t.Fatal("genesis register failed")
	}
	gh, _ := genesis.Hash()

	// A competing version branching from genesis with a different name.
	fork := core.NewCard(agent.DID, owner.DID, "fork-branch")
	fork.Prev = gh
	fork.Version = "9.9.9"
	if err := fork.Sign(agent.Private, owner.Private); err != nil {
		t.Fatal(err)
	}
	// It has prev=genesis, but the head is still genesis... make head advance first.
	update := core.NewCard(agent.DID, owner.DID, "legit-update")
	update.Prev = gh
	if err := update.Sign(agent.Private, owner.Private); err != nil {
		t.Fatal(err)
	}
	postJSON(t, ts.URL+"/v1/agents", update) // head -> legit-update
	postJSON(t, ts.URL+"/v1/agents", fork)   // branches from genesis, not head -> fork

	var resp struct {
		Card struct {
			Name string `json:"name"`
		} `json:"card"`
		Fork *struct {
			CompetingHash string `json:"competing_hash"`
		} `json:"fork"`
	}
	if code := getJSON(t, ts.URL+"/v1/agents/"+agent.DID, &resp); code != 200 {
		t.Fatalf("get: %d", code)
	}
	if resp.Card.Name != "legit-update" {
		t.Fatalf("head should be legit-update, got %q", resp.Card.Name)
	}
	if resp.Fork == nil {
		t.Fatal("expected fork to be surfaced on the agent response")
	}
	fh, _ := fork.Hash()
	if resp.Fork.CompetingHash != fh {
		t.Fatalf("fork competing hash mismatch")
	}
}

func TestAttestationPagination(t *testing.T) {
	ts, cleanup := testEnv(t)
	defer cleanup()

	owner, _ := core.GenerateKeyPair()
	subject, _ := core.GenerateKeyPair()
	issuer, _ := core.GenerateKeyPair()
	postJSON(t, ts.URL+"/v1/agents", mustCard(t, owner, subject, "subject"))
	postJSON(t, ts.URL+"/v1/agents", mustCard(t, owner, issuer, "issuer"))

	const total = 5
	head := ""
	for i := 0; i < total; i++ {
		a := core.NewAttestation(core.TypeTaskCompleted, issuer.DID, subject.DID)
		a.Prev = head
		a.IssuedAt = time.Now().UTC().Add(time.Duration(i) * time.Second).Format(time.RFC3339)
		if err := a.Sign(issuer.Private); err != nil {
			t.Fatal(err)
		}
		if code, body := postJSON(t, ts.URL+"/v1/attestations", a); code != 201 {
			t.Fatalf("attest %d: %d %s", i, code, body)
		}
		head, _ = a.Hash()
	}

	type page struct {
		Attestations []core.Attestation `json:"attestations"`
		Total        int                `json:"total"`
		Limit        int                `json:"limit"`
		Offset       int                `json:"offset"`
		NextOffset   *int               `json:"next_offset"`
	}
	var p1 page
	getJSON(t, ts.URL+"/v1/agents/"+subject.DID+"/attestations?limit=2&offset=0", &p1)
	if p1.Total != total {
		t.Fatalf("total = %d, want %d", p1.Total, total)
	}
	if len(p1.Attestations) != 2 {
		t.Fatalf("page 1 len = %d, want 2", len(p1.Attestations))
	}
	if p1.NextOffset == nil || *p1.NextOffset != 2 {
		t.Fatalf("next_offset = %v, want 2", p1.NextOffset)
	}

	// Last page: offset 4 -> 1 item, no next.
	var p3 page
	getJSON(t, ts.URL+"/v1/agents/"+subject.DID+"/attestations?limit=2&offset=4", &p3)
	if len(p3.Attestations) != 1 {
		t.Fatalf("last page len = %d, want 1", len(p3.Attestations))
	}
	if p3.NextOffset != nil {
		t.Fatalf("expected no next_offset on last page, got %v", *p3.NextOffset)
	}
}

func TestSearchPagination(t *testing.T) {
	ts, cleanup := testEnv(t)
	defer cleanup()
	owner, _ := core.GenerateKeyPair()
	for i := 0; i < 5; i++ {
		agent, _ := core.GenerateKeyPair()
		postJSON(t, ts.URL+"/v1/agents", mustCard(t, owner, agent, "searchable-agent", "code.review"))
	}
	var resp struct {
		Count      int  `json:"count"`
		Total      int  `json:"total"`
		NextOffset *int `json:"next_offset"`
	}
	getJSON(t, ts.URL+"/v1/search?cap=code.review&limit=2&offset=0", &resp)
	if resp.Total != 5 {
		t.Fatalf("total = %d, want 5", resp.Total)
	}
	if resp.Count != 2 {
		t.Fatalf("count = %d, want 2 (page size)", resp.Count)
	}
	if resp.NextOffset == nil || *resp.NextOffset != 2 {
		t.Fatalf("next_offset = %v, want 2", resp.NextOffset)
	}
}

func TestA2AResolution(t *testing.T) {
	ts, cleanup := testEnv(t)
	defer cleanup()

	owner, _ := core.GenerateKeyPair()
	agent, _ := core.GenerateKeyPair()

	c := core.NewCard(agent.DID, owner.DID, "resolver-agent")
	c.Description = "does resolvable things"
	c.Version = "1.2.3"
	c.Capabilities = []core.Capability{{Tag: "code.review", Desc: "reviews PRs"}}
	c.Protocols = map[string]any{
		"a2a":  map[string]any{"agent_card": "https://agent.example/.well-known/agent.json"},
		"http": map[string]any{"endpoint": "https://agent.example/v1"},
	}
	if err := c.Sign(agent.Private, owner.Private); err != nil {
		t.Fatal(err)
	}
	if code, body := postJSON(t, ts.URL+"/v1/agents", c); code != 201 {
		t.Fatalf("register: %d %s", code, body)
	}

	var a2a struct {
		Name    string `json:"name"`
		Version string `json:"version"`
		URL     string `json:"url"`
		Skills  []struct {
			ID   string   `json:"id"`
			Tags []string `json:"tags"`
		} `json:"skills"`
		Moltnet struct {
			DID       string  `json:"did"`
			MoltScore float64 `json:"moltscore"`
			Verify    string  `json:"verify"`
		} `json:"x-moltnet"`
	}
	if code := getJSON(t, ts.URL+"/v1/agents/"+agent.DID+"/a2a", &a2a); code != 200 {
		t.Fatalf("a2a: %d", code)
	}
	if a2a.Name != "resolver-agent" || a2a.Version != "1.2.3" {
		t.Fatalf("name/version mismatch: %+v", a2a)
	}
	if a2a.URL != "https://agent.example/.well-known/agent.json" {
		t.Fatalf("url = %q", a2a.URL)
	}
	if len(a2a.Skills) != 1 || a2a.Skills[0].ID != "code.review" {
		t.Fatalf("skills = %+v", a2a.Skills)
	}
	if a2a.Moltnet.DID != agent.DID {
		t.Fatalf("x-moltnet.did = %q, want %q", a2a.Moltnet.DID, agent.DID)
	}
	if a2a.Moltnet.Verify == "" {
		t.Fatal("expected x-moltnet.verify link")
	}
}

func TestOpenAPISpec(t *testing.T) {
	ts, cleanup := testEnv(t)
	defer cleanup()

	resp, err := http.Get(ts.URL + "/openapi.json")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("status %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "application/json" {
		t.Fatalf("content-type = %q", ct)
	}
	var doc struct {
		OpenAPI string                 `json:"openapi"`
		Info    map[string]any         `json:"info"`
		Paths   map[string]any         `json:"paths"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&doc); err != nil {
		t.Fatalf("openapi.json is not valid JSON: %v", err)
	}
	if !strings.HasPrefix(doc.OpenAPI, "3.") {
		t.Fatalf("expected OpenAPI 3.x, got %q", doc.OpenAPI)
	}
	for _, p := range []string{"/v1/agents", "/v1/agents/{did}", "/v1/search", "/v1/attestations", "/v1/score/{did}"} {
		if _, ok := doc.Paths[p]; !ok {
			t.Fatalf("openapi paths missing %q", p)
		}
	}
}

func TestHealthz(t *testing.T) {
	ts, cleanup := testEnv(t)
	defer cleanup()
	var resp struct {
		Status string `json:"status"`
	}
	if code := getJSON(t, ts.URL+"/healthz", &resp); code != 200 {
		t.Fatalf("healthz status %d", code)
	}
	if resp.Status != "ok" {
		t.Fatalf("healthz body = %q", resp.Status)
	}
}

func TestUnknownAgentIs404(t *testing.T) {
	ts, cleanup := testEnv(t)
	defer cleanup()
	if code := getJSON(t, ts.URL+"/v1/agents/did:key:zNope", nil); code != 404 {
		t.Fatalf("expected 404, got %d", code)
	}
}
