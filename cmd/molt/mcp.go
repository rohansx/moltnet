package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/moltnet/moltnet/core"
	"github.com/moltnet/moltnet/score"
)

// cmdMCP runs a Model Context Protocol server over stdio so agents and coding
// assistants can use a MoltNet registry natively. It speaks JSON-RPC 2.0 with
// newline-delimited framing (the MCP stdio transport) and proxies to a registry
// over REST — except moltnet_verify_agent, which recomputes trust locally.
func cmdMCP(args []string) error {
	fs := flag.NewFlagSet("mcp", flag.ExitOnError)
	registry := fs.String("registry", "", "registry base URL")
	fs.Parse(args)
	reg := registryURL(*registry)

	srv := &mcpServer{registry: reg}
	in := bufio.NewScanner(os.Stdin)
	in.Buffer(make([]byte, 0, 1<<20), 1<<24) // allow large card/attestation payloads
	out := json.NewEncoder(os.Stdout)

	for in.Scan() {
		line := in.Bytes()
		if len(line) == 0 {
			continue
		}
		var req rpcRequest
		if err := json.Unmarshal(line, &req); err != nil {
			continue // ignore unparseable frames
		}
		resp, isNotification := srv.handle(&req)
		if isNotification {
			continue // notifications get no response
		}
		if err := out.Encode(resp); err != nil {
			return err
		}
	}
	return in.Err()
}

// --- JSON-RPC 2.0 envelope ---------------------------------------------------

type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type mcpServer struct{ registry string }

func (s *mcpServer) handle(req *rpcRequest) (*rpcResponse, bool) {
	// Notifications (no id) are acknowledged silently.
	if len(req.ID) == 0 {
		return nil, true
	}
	resp := &rpcResponse{JSONRPC: "2.0", ID: req.ID}
	switch req.Method {
	case "initialize":
		resp.Result = map[string]any{
			"protocolVersion": "2024-11-05",
			"capabilities":    map[string]any{"tools": map[string]any{}},
			"serverInfo":      map[string]any{"name": "moltnet", "version": "0.1.0"},
		}
	case "tools/list":
		resp.Result = map[string]any{"tools": mcpTools()}
	case "tools/call":
		result, err := s.callTool(req.Params)
		if err != nil {
			resp.Result = toolError(err.Error())
		} else {
			resp.Result = result
		}
	case "ping":
		resp.Result = map[string]any{}
	default:
		resp.Error = &rpcError{Code: -32601, Message: "method not found: " + req.Method}
	}
	return resp, false
}

// --- Tool definitions --------------------------------------------------------

func mcpTools() []map[string]any {
	strProp := func(desc string) map[string]any {
		return map[string]any{"type": "string", "description": desc}
	}
	return []map[string]any{
		{
			"name":        "moltnet_verify_agent",
			"description": "Verify before invoke. Fetch an agent's card and full attestation chain, check every signature, verify each per-issuer hash chain, and recompute MoltScore locally — trusting the registry only for transport. Call this before hiring a stranger agent.",
			"inputSchema": map[string]any{
				"type":       "object",
				"properties": map[string]any{"did": strProp("agent DID (did:key:...)")},
				"required":   []string{"did"},
			},
		},
		{
			"name":        "moltnet_search_agents",
			"description": "Search the registry for agents by free text, capability tag, and minimum MoltScore. Returns ranked results with scores.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"query":      strProp("free-text query (name, description, capability)"),
					"capability": strProp("capability tag filter, e.g. code.review"),
					"min_score":  map[string]any{"type": "number", "description": "minimum MoltScore (0-100)"},
				},
			},
		},
		{
			"name":        "moltnet_get_agent",
			"description": "Fetch an agent's current card and cached score by DID.",
			"inputSchema": map[string]any{
				"type":       "object",
				"properties": map[string]any{"did": strProp("agent DID")},
				"required":   []string{"did"},
			},
		},
		{
			"name":        "moltnet_register_agent",
			"description": "Register a signed Agent Card (moltnet/card/v0.1). The card must already be signed by the agent key and owner key.",
			"inputSchema": map[string]any{
				"type":       "object",
				"properties": map[string]any{"card": map[string]any{"type": "object", "description": "signed card JSON"}},
				"required":   []string{"card"},
			},
		},
		{
			"name":        "moltnet_attest",
			"description": "Submit a signed attestation (moltnet/attestation/v0.1) about an agent. Must be signed by the issuer key and chain to the issuer's current head.",
			"inputSchema": map[string]any{
				"type":       "object",
				"properties": map[string]any{"attestation": map[string]any{"type": "object", "description": "signed attestation JSON"}},
				"required":   []string{"attestation"},
			},
		},
	}
}

// --- Tool dispatch -----------------------------------------------------------

func (s *mcpServer) callTool(params json.RawMessage) (any, error) {
	var call struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	}
	if err := json.Unmarshal(params, &call); err != nil {
		return nil, err
	}
	switch call.Name {
	case "moltnet_verify_agent":
		return s.toolVerify(call.Arguments)
	case "moltnet_search_agents":
		return s.toolSearch(call.Arguments)
	case "moltnet_get_agent":
		return s.toolGet(call.Arguments)
	case "moltnet_register_agent":
		return s.toolRegister(call.Arguments)
	case "moltnet_attest":
		return s.toolAttest(call.Arguments)
	default:
		return nil, fmt.Errorf("unknown tool %q", call.Name)
	}
}

func (s *mcpServer) toolVerify(argsRaw json.RawMessage) (any, error) {
	var a struct {
		DID string `json:"did"`
	}
	json.Unmarshal(argsRaw, &a)
	if a.DID == "" {
		return nil, fmt.Errorf("did is required")
	}
	card, atts, err := fetchAgent(s.registry, a.DID)
	if err != nil {
		return nil, err
	}
	if card == nil {
		return nil, fmt.Errorf("agent %s not found", a.DID)
	}
	cardErr := card.Verify()
	chainErr := core.VerifyAll(atts)
	out := score.Compute(atts, nil, time.Now().UTC())
	verdict := map[string]any{
		"did":                a.DID,
		"name":               card.Name,
		"card_signature_ok":  cardErr == nil,
		"chain_ok":           chainErr == nil,
		"verified":           cardErr == nil && chainErr == nil,
		"moltscore":          out.Score,
		"algorithm":          out.Algorithm,
		"inputs":             out.Inputs,
		"attestation_count":  len(atts),
		"recomputed_locally": true,
	}
	if cardErr != nil {
		verdict["card_error"] = cardErr.Error()
	}
	if chainErr != nil {
		verdict["chain_error"] = chainErr.Error()
	}
	return toolJSON(verdict), nil
}

func (s *mcpServer) toolSearch(argsRaw json.RawMessage) (any, error) {
	var a struct {
		Query      string  `json:"query"`
		Capability string  `json:"capability"`
		MinScore   float64 `json:"min_score"`
	}
	json.Unmarshal(argsRaw, &a)
	url := fmt.Sprintf("%s/v1/search?q=%s&cap=%s&min_score=%g&limit=25",
		s.registry, urlEscape(a.Query), urlEscape(a.Capability), a.MinScore)
	var resp any
	if err := httpGet(url, &resp); err != nil {
		return nil, err
	}
	return toolJSON(resp), nil
}

func (s *mcpServer) toolGet(argsRaw json.RawMessage) (any, error) {
	var a struct {
		DID string `json:"did"`
	}
	json.Unmarshal(argsRaw, &a)
	if a.DID == "" {
		return nil, fmt.Errorf("did is required")
	}
	var resp any
	if err := httpGet(s.registry+"/v1/agents/"+urlEscape(a.DID), &resp); err != nil {
		return nil, err
	}
	return toolJSON(resp), nil
}

func (s *mcpServer) toolRegister(argsRaw json.RawMessage) (any, error) {
	var a struct {
		Card core.Card `json:"card"`
	}
	if err := json.Unmarshal(argsRaw, &a); err != nil {
		return nil, err
	}
	if err := a.Card.Verify(); err != nil {
		return nil, fmt.Errorf("card fails verification: %w", err)
	}
	var resp any
	if err := httpPostJSON(s.registry+"/v1/agents", &a.Card, &resp); err != nil {
		return nil, err
	}
	return toolJSON(resp), nil
}

func (s *mcpServer) toolAttest(argsRaw json.RawMessage) (any, error) {
	var a struct {
		Attestation core.Attestation `json:"attestation"`
	}
	if err := json.Unmarshal(argsRaw, &a); err != nil {
		return nil, err
	}
	if err := a.Attestation.Verify(); err != nil {
		return nil, fmt.Errorf("attestation fails verification: %w", err)
	}
	var resp any
	if err := httpPostJSON(s.registry+"/v1/attestations", &a.Attestation, &resp); err != nil {
		return nil, err
	}
	return toolJSON(resp), nil
}

// --- MCP content helpers -----------------------------------------------------

func toolJSON(v any) map[string]any {
	data, _ := json.MarshalIndent(v, "", "  ")
	return map[string]any{"content": []map[string]any{{"type": "text", "text": string(data)}}}
}

func toolError(msg string) map[string]any {
	return map[string]any{
		"content": []map[string]any{{"type": "text", "text": "error: " + msg}},
		"isError": true,
	}
}
