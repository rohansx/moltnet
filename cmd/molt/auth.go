package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/moltnet/moltnet/core"
)

// auth.go — CLI commands for SIWK sign-in and agent API keys.

// authedGet/authedPost/authedDelete prepend the saved session's registry
// base URL and attach the session bearer token. path is a relative path like
// "/v1/me/apikeys".
func authedGet(path string, out any) error {
	s, err := loadSession()
	if err != nil {
		return fmt.Errorf("not signed in — run `molt login`")
	}
	return authedRequest("GET", s.Registry+path, s.Token, nil, out)
}

func authedPost(path string, payload any, out any) error {
	s, err := loadSession()
	if err != nil {
		return fmt.Errorf("not signed in — run `molt login`")
	}
	data, _ := json.Marshal(payload)
	return authedRequest("POST", s.Registry+path, s.Token, data, out)
}

func authedDelete(path string, out any) error {
	s, err := loadSession()
	if err != nil {
		return fmt.Errorf("not signed in — run `molt login`")
	}
	return authedRequest("DELETE", s.Registry+path, s.Token, nil, out)
}

// authedGetURL/authedPostURL/authedDeleteURL are path-based aliases kept for
// readability at call sites.
var authedGetURL = authedGet
var authedPostURL = authedPost
var authedDeleteURL = authedDelete

func authedRequest(method, url, token string, body []byte, out any) error {
	var rdr io.Reader
	if body != nil {
		rdr = bytes.NewReader(body)
	}
	req, _ := http.NewRequest(method, url, rdr)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode == http.StatusUnauthorized {
		return fmt.Errorf("session expired — run `molt login` again")
	}
	if resp.StatusCode >= 300 {
		return fmt.Errorf("%s %s: %s: %s", method, url, resp.Status, string(respBody))
	}
	if out != nil {
		return json.Unmarshal(respBody, out)
	}
	return nil
}

// cmdLogin performs SIWK sign-in with an owner keyfile, saving the session.
func cmdLogin(args []string) error {
	fs := flag.NewFlagSet("login", flag.ExitOnError)
	ownerFile := fs.String("owner", "owner.key", "owner keyfile")
	registry := fs.String("registry", "", "registry base URL")
	fs.Parse(args)

	ownerKP, err := loadKeyfile(*ownerFile)
	if err != nil {
		return fmt.Errorf("load owner key: %w", err)
	}
	reg := registryURL(*registry)

	// 1. fetch a single-use challenge
	var chall struct {
		Nonce   string `json:"nonce"`
		Message string `json:"message"`
		Domain  string `json:"domain"`
		Expires string `json:"expires_at"`
	}
	if err := httpPostJSON(reg+"/v1/auth/challenge", map[string]string{"did": ownerKP.DID}, &chall); err != nil {
		return fmt.Errorf("challenge: %w", err)
	}
	if chall.Message == "" {
		return fmt.Errorf("registry returned no message to sign")
	}

	// 2. sign the SIWK message locally with the owner key
	sig := core.Sign(ownerKP.Private, []byte(chall.Message))

	// 3. submit the signature → receive a session token
	var login struct {
		OK        bool   `json:"ok"`
		OwnerDID  string `json:"owner_did"`
		Session   string `json:"session"`
		ExpiresAt string `json:"expires_at"`
	}
	if err := httpPostJSON(reg+"/v1/auth/login", map[string]string{
		"did": ownerKP.DID, "nonce": chall.Nonce, "sig": sig,
	}, &login); err != nil {
		return fmt.Errorf("login: %w", err)
	}
	if !login.OK || login.Session == "" {
		return fmt.Errorf("login failed")
	}

	if err := saveSession(&sessionFile{Registry: reg, OwnerDID: login.OwnerDID, Token: login.Session}); err != nil {
		return fmt.Errorf("save session: %w", err)
	}
	fmt.Printf("signed in as owner\n  DID:     %s\n  expires: %s\n  session saved to %s\n",
		login.OwnerDID, login.ExpiresAt, sessionPath())
	return nil
}

// cmdLogout clears the local session and tells the server to delete it.
func cmdLogout(args []string) error {
	fs := flag.NewFlagSet("logout", flag.ExitOnError)
	registry := fs.String("registry", "", "registry base URL")
	fs.Parse(args)
	s, err := loadSession()
	if err != nil {
		clearSession()
		fmt.Println("no session to clear")
		return nil
	}
	reg := *registry
	if reg == "" {
		reg = s.Registry
	}
	req, _ := http.NewRequest("POST", reg+"/v1/auth/logout", nil)
	req.Header.Set("Authorization", "Bearer "+s.Token)
	resp, err := httpClient.Do(req)
	if err == nil {
		resp.Body.Close()
	}
	clearSession()
	fmt.Println("signed out")
	return nil
}

// cmdWhoami shows the owner the current session belongs to + their agents.
func cmdWhoami(args []string) error {
	fs := flag.NewFlagSet("whoami", flag.ExitOnError)
	fs.Parse(args)
	var me struct {
		OwnerDID string `json:"owner_did"`
		Agents   []struct {
			ID    string   `json:"id"`
			Name  string   `json:"name"`
			Score float64  `json:"score"`
			Caps  []string `json:"capabilities"`
		} `json:"agents"`
	}
	if err := authedGet("/v1/auth/me", &me); err != nil {
		return err
	}
	fmt.Printf("owner %s\n", me.OwnerDID)
	if len(me.Agents) == 0 {
		fmt.Println("  no agents registered under this owner")
		return nil
	}
	for _, a := range me.Agents {
		fmt.Printf("  %5.1f  %-20s  %s\n    %s\n", a.Score, a.Name, a.ID, strings.Join(a.Caps, ", "))
	}
	return nil
}

// cmdAPIKey subcommands: create / list / revoke
func cmdAPIKey(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: molt apikey <create|list|revoke> [flags]")
	}
	switch args[0] {
	case "create":
		return cmdAPIKeyCreate(args[1:])
	case "list":
		return cmdAPIKeyList(args[1:])
	case "revoke":
		return cmdAPIKeyRevoke(args[1:])
	default:
		return fmt.Errorf("unknown apikey subcommand %q", args[0])
	}
}

func cmdAPIKeyCreate(args []string) error {
	fs := flag.NewFlagSet("apikey create", flag.ExitOnError)
	agentDID := fs.String("agent", "", "agent DID to scope the key to (required)")
	name := fs.String("name", "default", "human label for the key")
	fs.Parse(args)
	if *agentDID == "" {
		return fmt.Errorf("--agent is required")
	}
	var out struct {
		Key    string `json:"key"`
		Prefix string `json:"prefix"`
		Last4  string `json:"last4"`
	}
	if err := authedPost("/v1/me/apikeys", map[string]string{"agent_did": *agentDID, "name": *name}, &out); err != nil {
		return err
	}
	fmt.Printf("new API key (shown once — copy it now):\n  %s\n  scope: agent %s\n", out.Key, *agentDID)
	return nil
}

func cmdAPIKeyList(args []string) error {
	fs := flag.NewFlagSet("apikey list", flag.ExitOnError)
	fs.Parse(args)
	var out struct {
		Keys []struct {
			Prefix    string `json:"prefix"`
			Last4     string `json:"last4"`
			AgentDID  string `json:"agent_did"`
			Name      string `json:"name"`
			CreatedAt string `json:"created_at"`
			RevokedAt string `json:"revoked_at"`
		} `json:"keys"`
	}
	if err := authedGetURL("/v1/me/apikeys", &out); err != nil {
		return err
	}
	if len(out.Keys) == 0 {
		fmt.Println("no API keys")
		return nil
	}
	for _, k := range out.Keys {
		st := "live"
		if k.RevokedAt != "" {
			st = "revoked"
		}
		fmt.Printf("  %-20s %s  %s  %s\n", k.Prefix+"••••"+k.Last4, st, k.Name, k.CreatedAt[:10])
	}
	return nil
}

func cmdAPIKeyRevoke(args []string) error {
	fs := flag.NewFlagSet("apikey revoke", flag.ExitOnError)
	prefix := fs.String("prefix", "", "display prefix of the key to revoke (required)")
	fs.Parse(args)
	if *prefix == "" {
		return fmt.Errorf("--prefix is required")
	}
	if err := authedDeleteURL("/v1/me/apikeys/"+*prefix, nil); err != nil {
		return err
	}
	fmt.Println("revoked")
	return nil
}

// cmdAgent is the agent-self dispatcher (currently only `me`).
func cmdAgent(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: molt agent me [--key ...]")
	}
	switch args[0] {
	case "me":
		return cmdAgentMe(args[1:])
	default:
		return fmt.Errorf("unknown agent subcommand %q", args[0])
	}
}

// cmdAgentMe — the agent's own view, authorized by an API key.
func cmdAgentMe(args []string) error {
	fs := flag.NewFlagSet("agent me", flag.ExitOnError)
	key := fs.String("key", "", "agent API key (molt_sk_live_…) or $MOLTNET_API_KEY")
	fs.Parse(args)
	k := *key
	if k == "" {
		k = os.Getenv("MOLTNET_API_KEY")
	}
	if k == "" {
		return fmt.Errorf("--key or $MOLTNET_API_KEY is required")
	}
	reg := registryURL("")
	req, _ := http.NewRequest("GET", reg+"/v1/agent/me", nil)
	req.Header.Set("Authorization", "Bearer "+k)
	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode == http.StatusUnauthorized {
		return fmt.Errorf("invalid or revoked API key")
	}
	if resp.StatusCode >= 300 {
		return fmt.Errorf("%s: %s", resp.Status, string(body))
	}
	var out struct {
		Card struct {
			ID      string `json:"id"`
			Name    string `json:"name"`
			Owner   string `json:"owner"`
			Version string `json:"version"`
		} `json:"card"`
		Score struct {
			Score float64 `json:"score"`
		} `json:"score"`
	}
	_ = json.Unmarshal(body, &out)
	fmt.Printf("agent %s\n  name:    %s\n  owner:   %s\n  score:   %.1f\n",
		out.Card.ID, out.Card.Name, out.Card.Owner, out.Score.Score)
	return nil
}
