package server

import (
	"net/http"

	"github.com/moltnet/moltnet/core"
	"github.com/moltnet/moltnet/score"
)

// a2aCard maps a MoltNet Agent Card to an A2A-compatible Agent Card so that A2A
// (Agent2Agent) skill discovery resolves to the same identity. MoltNet-specific
// data (DID, MoltScore, a verify link) is carried in an `x-moltnet` extension so
// A2A clients keep working while MoltNet-aware ones get the trust signal.
func a2aCard(c *core.Card, out score.Output, baseURL string) map[string]any {
	// Service URL: prefer the declared A2A agent card, then HTTP, then MCP.
	url := protoField(c, "a2a", "agent_card")
	if url == "" {
		url = protoField(c, "a2a", "endpoint")
	}
	if url == "" {
		url = protoField(c, "http", "endpoint")
	}
	if url == "" {
		url = protoField(c, "mcp", "endpoint")
	}

	skills := make([]map[string]any, 0, len(c.Capabilities))
	for _, cap := range c.Capabilities {
		skills = append(skills, map[string]any{
			"id":          cap.Tag,
			"name":        cap.Tag,
			"description": cap.Desc,
			"tags":        []string{cap.Tag},
		})
	}

	return map[string]any{
		"name":               c.Name,
		"description":        c.Description,
		"url":                url,
		"version":            c.Version,
		"protocolVersion":    "0.2.0",
		"capabilities":       map[string]any{"streaming": false, "pushNotifications": false},
		"defaultInputModes":  []string{"text"},
		"defaultOutputModes": []string{"text"},
		"provider":           map[string]any{"organization": c.Owner},
		"skills":             skills,
		"x-moltnet": map[string]any{
			"did":       c.ID,
			"moltscore": out.Score,
			"algorithm": out.Algorithm,
			"verify":    baseURL + "/profile.html?did=" + c.ID,
			"card":      baseURL + "/v1/agents/" + c.ID,
		},
	}
}

// protoField extracts protocols[proto][field] as a string, if present.
func protoField(c *core.Card, proto, field string) string {
	p, ok := c.Protocols[proto]
	if !ok {
		return ""
	}
	m, ok := p.(map[string]any)
	if !ok {
		return ""
	}
	if v, ok := m[field].(string); ok {
		return v
	}
	return ""
}

func (s *Server) handleA2A(w http.ResponseWriter, r *http.Request) {
	did := r.PathValue("did")
	c, err := s.Store.GetCard(did)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if c == nil {
		writeErr(w, http.StatusNotFound, "agent not found")
		return
	}
	out, _ := s.recomputeScore(did)
	base := baseURLOf(r)
	writeJSON(w, http.StatusOK, a2aCard(c, out, base))
}

// baseURLOf reconstructs the externally-visible base URL of the request.
func baseURLOf(r *http.Request) string {
	scheme := "http"
	if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}
	return scheme + "://" + r.Host
}
