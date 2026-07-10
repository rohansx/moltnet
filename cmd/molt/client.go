package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/moltnet/moltnet/core"
	"github.com/moltnet/moltnet/internal/server"
)

func urlEscape(s string) string { return url.QueryEscape(s) }

// parseInterspersed parses a flag set allowing flags and positional arguments
// to appear in any order (Go's stdlib flag stops at the first positional). It
// returns the collected positional arguments. This lets us honour the spec's
// `molt search "pii redaction" --cap privacy.redaction` ordering.
func parseInterspersed(fs *flag.FlagSet, args []string) []string {
	var positional []string
	for {
		if err := fs.Parse(args); err != nil {
			return positional
		}
		if fs.NArg() == 0 {
			break
		}
		positional = append(positional, fs.Arg(0))
		args = fs.Args()[1:]
	}
	return positional
}

// httpListen runs a server instance (used by `molt serve`).
func httpListen(addr string, srv *server.Server) error {
	return http.ListenAndServe(addr, srv.Handler())
}

// registryURL resolves the registry base URL from --registry, MOLTNET_REGISTRY,
// or the local default.
func registryURL(flagVal string) string {
	if flagVal != "" {
		return flagVal
	}
	if env := os.Getenv("MOLTNET_REGISTRY"); env != "" {
		return env
	}
	return "http://localhost:8787"
}

var httpClient = &http.Client{Timeout: 15 * time.Second}

func httpGet(url string, out any) error {
	resp, err := httpClient.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return fmt.Errorf("GET %s: %s: %s", url, resp.Status, string(body))
	}
	if out != nil {
		return json.Unmarshal(body, out)
	}
	return nil
}

func httpPostJSON(url string, payload any, out any) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	resp, err := httpClient.Post(url, "application/json", bytes.NewReader(data))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return fmt.Errorf("POST %s: %s: %s", url, resp.Status, string(body))
	}
	if out != nil {
		return json.Unmarshal(body, out)
	}
	return nil
}

// fetchIssuerHead returns the issuer's current chain head from the registry.
func fetchIssuerHead(registry, issuerDID string) (string, error) {
	var resp struct {
		Head string `json:"head"`
	}
	err := httpGet(registry+"/v1/issuers/"+issuerDID+"/head", &resp)
	return resp.Head, err
}

// fetchAgent returns the card and raw attestations for a DID.
func fetchAgent(registry, did string) (*core.Card, []*core.Attestation, error) {
	var agentResp struct {
		Card *core.Card `json:"card"`
	}
	if err := httpGet(registry+"/v1/agents/"+did, &agentResp); err != nil {
		return nil, nil, err
	}
	var attResp struct {
		Attestations []*core.Attestation `json:"attestations"`
	}
	if err := httpGet(registry+"/v1/agents/"+did+"/attestations", &attResp); err != nil {
		return nil, nil, err
	}
	return agentResp.Card, attResp.Attestations, nil
}
