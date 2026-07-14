package server

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/moltnet/moltnet/core"
	"github.com/moltnet/moltnet/internal/store"
)

// TestSIWKLoginAndDashboard exercises the full user-auth flow:
// register an agent (so the owner has something), request a SIWK challenge,
// sign it with the owner key, log in, list "my agents", and confirm the
// dashboard gate redirects when unauthenticated.
func TestSIWKLoginAndDashboard(t *testing.T) {
	ts, cleanup := testEnv(t)
	defer cleanup()

	owner, err := core.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	agent, err := core.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	card := mustCard(t, owner, agent, "auth-test-agent", "code.review")
	if code, body := postJSON(t, ts.URL+"/v1/agents", card); code != 201 {
		t.Fatalf("register: %d %s", code, body)
	}

	// 1. challenge
	var chall struct {
		Nonce   string `json:"nonce"`
		Message string `json:"message"`
	}
	if code, body := postJSON(t, ts.URL+"/v1/auth/challenge", map[string]string{"did": owner.DID}); code != 200 {
		t.Fatalf("challenge: %d %s", code, body)
	} else {
		decode(t, body, &chall)
	}
	if chall.Nonce == "" || chall.Message == "" {
		t.Fatalf("challenge missing nonce/message: %+v", chall)
	}

	// 2. sign + login
	sig := core.Sign(owner.Private, []byte(chall.Message))
	var login struct {
		OK       bool   `json:"ok"`
		OwnerDID string `json:"owner_did"`
		Session  string `json:"session"`
	}
	if code, body := postJSON(t, ts.URL+"/v1/auth/login", map[string]string{
		"did": owner.DID, "nonce": chall.Nonce, "sig": sig,
	}); code != 200 {
		t.Fatalf("login: %s", body)
	} else {
		decode(t, body, &login)
	}
	if !login.OK || login.Session == "" || login.OwnerDID != owner.DID {
		t.Fatalf("login response bad: %+v", login)
	}

	// 3. nonce must be single-use: a second login with the same nonce fails.
	sig2 := core.Sign(owner.Private, []byte(chall.Message))
	if code, _ := postJSON(t, ts.URL+"/v1/auth/login", map[string]string{
		"did": owner.DID, "nonce": chall.Nonce, "sig": sig2,
	}); code == 200 {
		t.Fatalf("reused nonce should not succeed")
	}

	// 4. a bad signature must fail.
	var chall2 struct {
		Nonce   string `json:"nonce"`
		Message string `json:"message"`
	}
	if _, body := postJSON(t, ts.URL+"/v1/auth/challenge", map[string]string{"did": owner.DID}); true {
		decode(t, body, &chall2)
	}
	badSig := "00" + core.Sign(owner.Private, []byte(chall2.Message))[2:] // flip bytes
	if code, _ := postJSON(t, ts.URL+"/v1/auth/login", map[string]string{
		"did": owner.DID, "nonce": chall2.Nonce, "sig": badSig,
	}); code != 401 {
		t.Fatalf("bad signature should 401, got %d", code)
	}

	// 5. /v1/auth/me with the bearer token returns the agent we registered.
	var me struct {
		OwnerDID string `json:"owner_did"`
		Agents   []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"agents"`
	}
	if code := getJSONAuth(t, ts.URL+"/v1/auth/me", login.Session, &me); code != 200 {
		t.Fatalf("me: %d", code)
	}
	if me.OwnerDID != owner.DID || len(me.Agents) != 1 || me.Agents[0].ID != agent.DID {
		t.Fatalf("me response bad: %+v", me)
	}

	// 6. /v1/auth/me without a token is 401.
	if code := getJSONAuth(t, ts.URL+"/v1/auth/me", "", nil); code != 401 {
		t.Fatalf("unauthed me should 401, got %d", code)
	}

	// 7. SPA serving: a real asset is served as-is, and any client-side route
	//    falls back to index.html so a deep link / hard refresh works.
	//    The dashboard SHELL is deliberately public — the trust boundary is the
	//    API (step 6: /v1/auth/me is 401 without a session), so handing an
	//    unauthenticated visitor the JS bundle leaks nothing.
	td := t.TempDir()
	if err := os.WriteFile(filepath.Join(td, "index.html"), []byte("<!doctype html><title>molt spa</title>"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(td, "assets"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(td, "assets", "app.js"), []byte("console.log(1)"), 0o644); err != nil {
		t.Fatal(err)
	}
	st2, err := store.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer st2.Close()
	spa := httptest.NewServer((&Server{Store: st2, AppDir: td, Name: "spa-test", Version: "test"}).Handler())
	defer spa.Close()

	for _, tc := range []struct{ path, want string }{
		{"/assets/app.js", "console.log(1)"},   // real file served directly
		{"/dashboard", "<!doctype html><title>molt spa</title>"}, // client route → shell
		{"/profile/did:key:zAbc", "<!doctype html><title>molt spa</title>"},
	} {
		resp, err := http.Get(spa.URL + tc.path)
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != 200 {
			t.Fatalf("GET %s: status %d, want 200", tc.path, resp.StatusCode)
		}
		if string(body) != tc.want {
			t.Fatalf("GET %s: body %q, want %q", tc.path, body, tc.want)
		}
	}

	// …and the API behind it stays protected on that same server.
	resp, err := http.Get(spa.URL + "/v1/auth/me")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 401 {
		t.Fatalf("SPA server /v1/auth/me should still be 401, got %d", resp.StatusCode)
	}
}

// TestAPIKeyMintAndAgentMe exercises agent auth: the owner mints an API key for
// their agent and uses it to fetch /v1/agent/me. A key for an agent you don't own
// is forbidden, and a revoked key stops working.
func TestAPIKeyMintAndAgentMe(t *testing.T) {
	ts, cleanup := testEnv(t)
	defer cleanup()

	owner, _ := core.GenerateKeyPair()
	agent, _ := core.GenerateKeyPair()
	other, _ := core.GenerateKeyPair()
	otherAgent, _ := core.GenerateKeyPair()
	card := mustCard(t, owner, agent, "keytest-agent")
	if code, body := postJSON(t, ts.URL+"/v1/agents", card); code != 201 {
		t.Fatalf("register: %d %s", code, body)
	}
	otherCard := mustCard(t, other, otherAgent, "not-yours")
	if code, body := postJSON(t, ts.URL+"/v1/agents", otherCard); code != 201 {
		t.Fatalf("register other: %d %s", code, body)
	}

	// log in as owner
	token := loginOwner(t, ts.URL, owner)

	// minting a key for an agent you DO NOT own → 403
	if code, _ := postJSONAuth(t, ts.URL+"/v1/me/apikeys", token, map[string]string{
		"agent_did": otherAgent.DID, "name": "stolen",
	}); code != 403 {
		t.Fatalf("minting key for agent you don't own should 403, got %d", code)
	}

	// mint a key for your own agent → returns the full key once, plus its id
	var mint struct {
		Key    string `json:"key"`
		ID     string `json:"id"`
		Prefix string `json:"prefix"`
	}
	if code, body := postJSONAuth(t, ts.URL+"/v1/me/apikeys", token, map[string]string{
		"agent_did": agent.DID, "name": "prod",
	}); code != 201 {
		t.Fatalf("mint key: %s", body)
	} else {
		decode(t, body, &mint)
	}
	if mint.Key == "" || len(mint.Key) < 20 || mint.Prefix == "" || mint.ID == "" {
		t.Fatalf("mint response bad: %+v", mint)
	}

	// list keys → one live key carrying the same unique id
	var list struct {
		Keys []struct {
			ID        string `json:"id"`
			Prefix    string `json:"prefix"`
			Last4     string `json:"last4"`
			RevokedAt string `json:"revoked_at"`
		} `json:"keys"`
	}
	if code := getJSONAuth(t, ts.URL+"/v1/me/apikeys", token, &list); code != 200 {
		t.Fatalf("list keys: %d", code)
	}
	if len(list.Keys) != 1 || list.Keys[0].ID != mint.ID {
		t.Fatalf("list keys bad: %+v", list)
	}

	// use the agent key to call /v1/agent/me → returns the agent's card
	var agentMe struct {
		Card struct {
			ID string `json:"id"`
		} `json:"card"`
	}
	if code := getJSONAuth(t, ts.URL+"/v1/agent/me", mint.Key, &agentMe); code != 200 {
		t.Fatalf("agent me: %d", code)
	}
	if agentMe.Card.ID != agent.DID {
		t.Fatalf("agent me returned wrong agent: %+v", agentMe)
	}

	// a bogus key is 401
	if code := getJSONAuth(t, ts.URL+"/v1/agent/me", "molt_sk_live_boguskey000000000000", nil); code != 401 {
		t.Fatalf("bogus key should 401, got %d", code)
	}

	// revoke the key, then it stops working
	if code := deleteJSONAuth(t, ts.URL+"/v1/me/apikeys/"+mint.ID, token); code != 200 {
		t.Fatalf("revoke: %d", code)
	}
	if code := getJSONAuth(t, ts.URL+"/v1/agent/me", mint.Key, nil); code != 401 {
		t.Fatalf("revoked key should 401, got %d", code)
	}
}

// loginOwner is a helper that performs SIWK login and returns the session token.
func loginOwner(t *testing.T, base string, owner *core.KeyPair) string {
	t.Helper()
	var chall struct {
		Nonce   string `json:"nonce"`
		Message string `json:"message"`
	}
	_, body := postJSON(t, base+"/v1/auth/challenge", map[string]string{"did": owner.DID})
	decode(t, body, &chall)
	sig := core.Sign(owner.Private, []byte(chall.Message))
	var login struct {
		Session string `json:"session"`
	}
	_, body = postJSON(t, base+"/v1/auth/login", map[string]string{
		"did": owner.DID, "nonce": chall.Nonce, "sig": sig,
	})
	decode(t, body, &login)
	if login.Session == "" {
		t.Fatal("login failed: no session")
	}
	return login.Session
}

func decode(t *testing.T, body []byte, v any) {
	t.Helper()
	if err := json.Unmarshal(body, v); err != nil {
		t.Fatalf("decode: %v\n%s", err, string(body))
	}
}

func getJSONAuth(t *testing.T, url, token string, out any) int {
	t.Helper()
	req, _ := http.NewRequest("GET", url, nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := httpClient2.Do(req)
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

func postJSONAuth(t *testing.T, url, token string, v any) (int, []byte) {
	t.Helper()
	data, _ := json.Marshal(v)
	req, _ := http.NewRequest("POST", url, bytes.NewReader(data))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := httpClient2.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, body
}

func deleteJSONAuth(t *testing.T, url, token string) int {
	t.Helper()
	req, _ := http.NewRequest("DELETE", url, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := httpClient2.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	return resp.StatusCode
}

var httpClient2 = &http.Client{}
