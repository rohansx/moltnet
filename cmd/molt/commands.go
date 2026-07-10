package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/moltnet/moltnet/core"
	"github.com/moltnet/moltnet/internal/server"
	"github.com/moltnet/moltnet/internal/store"
	"github.com/moltnet/moltnet/score"
)

func cmdKeygen(args []string) error {
	fs := flag.NewFlagSet("keygen", flag.ExitOnError)
	out := fs.String("out", "", "output keyfile path (default: <kind>.key)")
	kind := fs.String("kind", "agent", "key kind: owner or agent")
	fs.Parse(args)

	if *kind != "owner" && *kind != "agent" {
		return fmt.Errorf("--kind must be owner or agent")
	}
	path := *out
	if path == "" {
		path = *kind + ".key"
	}
	kp, err := core.GenerateKeyPair()
	if err != nil {
		return err
	}
	if err := writeKeyfile(path, kp, *kind); err != nil {
		return err
	}
	fmt.Printf("created %s keypair\n  DID:  %s\n  file: %s (keep private)\n", *kind, kp.DID, path)
	return nil
}

// stringSlice collects repeated flags.
type stringSlice []string

func (s *stringSlice) String() string { return strings.Join(*s, ",") }
func (s *stringSlice) Set(v string) error {
	*s = append(*s, v)
	return nil
}

func cmdCard(args []string) error {
	if len(args) == 0 || args[0] != "new" {
		return fmt.Errorf("usage: molt card new [flags]")
	}
	fs := flag.NewFlagSet("card new", flag.ExitOnError)
	agentFile := fs.String("agent", "agent.key", "agent keyfile")
	ownerFile := fs.String("owner", "owner.key", "owner keyfile")
	name := fs.String("name", "", "agent name (required)")
	desc := fs.String("desc", "", "agent description")
	version := fs.String("agent-version", "0.1.0", "agent version string")
	out := fs.String("out", "card.json", "output card path")
	livenessURL := fs.String("liveness-url", "", "opt-in health-probe URL (enables liveness)")
	httpEndpoint := fs.String("http", "", "HTTP endpoint binding for the http protocol")
	mcpEndpoint := fs.String("mcp", "", "MCP endpoint binding for the mcp protocol")
	site := fs.String("site", "", "public site link")
	source := fs.String("source", "", "source repository link")
	var caps stringSlice
	fs.Var(&caps, "cap", "capability tag (repeatable, e.g. code.review)")
	fs.Parse(args[1:])

	if *name == "" {
		return fmt.Errorf("--name is required")
	}
	agentKP, err := loadKeyfile(*agentFile)
	if err != nil {
		return err
	}
	ownerKP, err := loadKeyfile(*ownerFile)
	if err != nil {
		return err
	}
	c := core.NewCard(agentKP.DID, ownerKP.DID, *name)
	c.Description = *desc
	c.Version = *version
	for _, tag := range caps {
		c.Capabilities = append(c.Capabilities, core.Capability{Tag: tag})
	}
	if *httpEndpoint != "" || *mcpEndpoint != "" {
		c.Protocols = map[string]any{}
		if *httpEndpoint != "" {
			c.Protocols["http"] = map[string]any{"endpoint": *httpEndpoint}
		}
		if *mcpEndpoint != "" {
			c.Protocols["mcp"] = map[string]any{"endpoint": *mcpEndpoint}
		}
	}
	if *site != "" || *source != "" {
		c.Links = map[string]string{}
		if *site != "" {
			c.Links["site"] = *site
		}
		if *source != "" {
			c.Links["source"] = *source
		}
	}
	if *livenessURL != "" {
		c.Liveness = &core.Liveness{Enabled: true, URL: *livenessURL}
	}
	if err := c.Sign(agentKP.Private, ownerKP.Private); err != nil {
		return err
	}
	data, _ := json.MarshalIndent(c, "", "  ")
	if err := os.WriteFile(*out, append(data, '\n'), 0o644); err != nil {
		return err
	}
	hash, _ := c.Hash()
	fmt.Printf("signed card written to %s\n  agent: %s\n  hash:  %s\n", *out, c.ID, hash)
	return nil
}

func cmdRegister(args []string) error {
	fs := flag.NewFlagSet("register", flag.ExitOnError)
	cardFile := fs.String("card", "card.json", "signed card to submit")
	registry := fs.String("registry", "", "registry base URL")
	fs.Parse(args)

	data, err := os.ReadFile(*cardFile)
	if err != nil {
		return err
	}
	var c core.Card
	if err := json.Unmarshal(data, &c); err != nil {
		return err
	}
	if err := c.Verify(); err != nil {
		return fmt.Errorf("card fails local verification (not submitting): %w", err)
	}
	reg := registryURL(*registry)
	var resp map[string]any
	if err := httpPostJSON(reg+"/v1/agents", &c, &resp); err != nil {
		return err
	}
	fmt.Printf("registered %s at %s\n", c.ID, reg)
	return nil
}

func cmdAttest(args []string) error {
	fs := flag.NewFlagSet("attest", flag.ExitOnError)
	typ := fs.String("type", core.TypeTaskCompleted, "attestation type")
	issuerFile := fs.String("issuer", "agent.key", "issuer keyfile")
	subject := fs.String("subject", "", "subject agent DID (required)")
	outcome := fs.String("outcome", "success", "outcome for task.completed")
	capability := fs.String("capability", "", "capability tag exercised")
	note := fs.String("note", "", "free-text note / reason")
	registry := fs.String("registry", "", "registry base URL")
	fs.Parse(args)

	if *subject == "" {
		return fmt.Errorf("--subject is required")
	}
	if !core.ValidType(*typ) {
		return fmt.Errorf("unknown attestation type %q", *typ)
	}
	issuerKP, err := loadKeyfile(*issuerFile)
	if err != nil {
		return err
	}
	reg := registryURL(*registry)

	// Look up the subject's current card hash and the issuer's chain head.
	subjCard, _, err := fetchAgent(reg, *subject)
	if err != nil {
		return fmt.Errorf("fetch subject: %w", err)
	}
	if subjCard == nil {
		return fmt.Errorf("subject %s not found on registry", *subject)
	}
	subjHash, _ := subjCard.Hash()
	head, err := fetchIssuerHead(reg, issuerKP.DID)
	if err != nil {
		return fmt.Errorf("fetch issuer head: %w", err)
	}

	a := core.NewAttestation(*typ, issuerKP.DID, *subject)
	a.SubjectCard = subjHash
	a.Prev = head
	a.Body = map[string]any{}
	if *capability != "" {
		a.Body["capability"] = *capability
	}
	if *typ == core.TypeTaskCompleted {
		a.Body["outcome"] = *outcome
	}
	if *note != "" {
		a.Body["note"] = *note
	}
	if err := a.Sign(issuerKP.Private); err != nil {
		return err
	}
	var resp map[string]any
	if err := httpPostJSON(reg+"/v1/attestations", a, &resp); err != nil {
		return err
	}
	hash, _ := a.Hash()
	fmt.Printf("attestation %s issued\n  type:    %s\n  subject: %s\n  hash:    %s\n",
		*typ, *typ, *subject, hash)
	return nil
}

func cmdSearch(args []string) error {
	fs := flag.NewFlagSet("search", flag.ExitOnError)
	capTag := fs.String("cap", "", "capability tag filter")
	minScore := fs.Float64("min-score", 0, "minimum MoltScore")
	registry := fs.String("registry", "", "registry base URL")
	positional := parseInterspersed(fs, args)

	query := strings.Join(positional, " ")
	reg := registryURL(*registry)
	url := fmt.Sprintf("%s/v1/search?q=%s&cap=%s&min_score=%g",
		reg, urlEscape(query), urlEscape(*capTag), *minScore)
	var resp struct {
		Count   int           `json:"count"`
		Results []store.Agent `json:"results"`
	}
	if err := httpGet(url, &resp); err != nil {
		return err
	}
	fmt.Printf("%d result(s)\n", resp.Count)
	for _, a := range resp.Results {
		fmt.Printf("  %5.1f  %-24s  %s\n    %s\n", a.Score, a.Name, a.DID, strings.Join(a.Capabilities, ", "))
	}
	return nil
}

func cmdBadge(args []string) error {
	fs := flag.NewFlagSet("badge", flag.ExitOnError)
	registry := fs.String("registry", "", "registry base URL")
	positional := parseInterspersed(fs, args)
	if len(positional) < 1 {
		return fmt.Errorf("usage: molt badge <did>")
	}
	did := positional[0]
	reg := registryURL(*registry)
	badge := fmt.Sprintf("%s/v1/agents/%s/badge.svg", reg, did)
	verify := fmt.Sprintf("%s/v1/agents/%s", reg, did)
	fmt.Printf("[![MoltScore](%s)](%s)\n", badge, verify)
	return nil
}

func cmdServe(args []string) error {
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	addr := fs.String("addr", ":8787", "listen address")
	dbPath := fs.String("db", "moltnet.db", "SQLite path or :memory:")
	webDir := fs.String("web", "", "static web dir to serve at / (optional)")
	fs.Parse(args)

	st, err := store.Open(*dbPath)
	if err != nil {
		return err
	}
	defer st.Close()
	srv := &server.Server{Store: st, WebDir: *webDir, Name: "molt serve", Version: "0.1.0"}
	srv.StartLivenessProber(5 * time.Minute)
	fmt.Printf("moltnetd listening on http://localhost%s (db: %s)\n", *addr, *dbPath)
	return httpListen(*addr, srv)
}

// scoreLine formats a one-line score summary.
func scoreLine(out score.Output) string {
	return fmt.Sprintf("%.1f/100  (completions=%d disputes=%d incidents=%d distinct_issuers=%d)",
		out.Score, out.Inputs.Completions, out.Inputs.Disputes, out.Inputs.Incidents, out.Inputs.DistinctIssuers)
}

var _ = time.Now // retained for future timestamp flags
