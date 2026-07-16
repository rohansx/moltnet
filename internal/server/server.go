// Package server implements moltnetd's HTTP surface: the REST API, badge SVGs,
// instance metadata and the static web UI. Writes are authenticated by
// signatures, not sessions — trust lives in the signed records themselves.
package server

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/moltnet/moltnet/core"
	"github.com/moltnet/moltnet/internal/store"
	"github.com/moltnet/moltnet/score"
)

// Server holds the store and configuration for the registry.
type Server struct {
	Store   *store.Store
	AppDir  string // optional path to a built SPA (e.g. frontend/dist) served at /
	Name    string
	Version string
	Peers   []string // federation peers this instance follows (allowlist)
	// RateLimitPerMin caps write (POST/PUT/PATCH/DELETE) requests per client IP
	// per minute. 0 disables rate limiting. Reads are never limited.
	RateLimitPerMin int
	// LogWriter, if set, receives one structured JSON log line per request.
	LogWriter io.Writer
}

// Handler builds the HTTP router. Go 1.22+ method+path patterns keep us on the
// standard library with no framework dependency.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("POST /v1/agents", s.handleRegister)
	mux.HandleFunc("GET /v1/agents/{did}", s.handleGetAgent)
	mux.HandleFunc("GET /v1/agents/{did}/history", s.handleHistory)
	mux.HandleFunc("GET /v1/agents/{did}/attestations", s.handleAttestations)
	mux.HandleFunc("GET /v1/agents/{did}/badge.svg", s.handleBadge)
	mux.HandleFunc("GET /v1/agents/{did}/liveness", s.handleLiveness)
	mux.HandleFunc("GET /v1/agents/{did}/a2a", s.handleA2A)
	mux.HandleFunc("POST /v1/attestations", s.handleAttest)
	mux.HandleFunc("POST /v1/rotations", s.handleRotation)
	mux.HandleFunc("GET /v1/issuers/{did}/head", s.handleIssuerHead)
	mux.HandleFunc("GET /v1/search", s.handleSearch)
	mux.HandleFunc("GET /v1/score/{did}", s.handleScore)
	mux.HandleFunc("GET /v1/taxonomy", s.handleTaxonomy)
	mux.HandleFunc("GET /v1/graph", s.handleGraph)
	mux.HandleFunc("GET /.well-known/moltnet", s.handleWellKnown)
	mux.HandleFunc("GET /openapi.json", s.handleOpenAPI)
	mux.HandleFunc("GET /v1/stats", s.handleStats)
	mux.HandleFunc("GET /healthz", s.handleHealthz)
	mux.HandleFunc("GET /federation/changes", s.handleFederationChanges)
	mux.HandleFunc("GET /federation/peers", s.handleFederationPeers)

	// ---- auth (SIWK + sessions + agent API keys) ----
	mux.HandleFunc("POST /v1/auth/challenge", s.handleAuthChallenge)
	mux.HandleFunc("POST /v1/auth/login", s.handleAuthLogin)
	mux.HandleFunc("POST /v1/auth/logout", s.handleAuthLogout)
	mux.HandleFunc("GET /v1/auth/me", s.requireOwner(s.handleAuthMe))
	mux.HandleFunc("GET /v1/me/agents", s.requireOwner(s.handleMyAgents))
	mux.HandleFunc("GET /v1/me/apikeys", s.requireOwner(s.handleListAPIKeys))
	mux.HandleFunc("POST /v1/me/apikeys", s.requireOwner(s.handleCreateAPIKey))
	mux.HandleFunc("DELETE /v1/me/apikeys/{id}", s.requireOwner(s.handleRevokeAPIKey))
	mux.HandleFunc("GET /v1/agent/me", s.handleAgentMe)

	// ---- marketplace (platform v0.2) ----
	// Reads are public. Create is authorized by the poster's signed offer;
	// settle by the signed settlement records (no session). Poster-only actions
	// (assign/escrow) require the owner session; agent actions (apply/deliver)
	// use an agent API key. Reputation moves only through /v1/attestations.
	mux.HandleFunc("POST /v1/tasks", s.handleCreateTask)
	mux.HandleFunc("GET /v1/tasks", s.handleListTasks)
	mux.HandleFunc("GET /v1/tasks/{id}", s.handleGetTask)
	mux.HandleFunc("POST /v1/tasks/{id}/apply", s.handleApplyTask)
	mux.HandleFunc("POST /v1/tasks/{id}/assign", s.requireOwner(s.handleAssignTask))
	mux.HandleFunc("POST /v1/tasks/{id}/escrow", s.requireOwner(s.handleEscrowTask))
	mux.HandleFunc("POST /v1/tasks/{id}/deliver", s.handleDeliverTask)
	mux.HandleFunc("POST /v1/tasks/{id}/settle", s.handleSettleTask)

	// ---- web UI ----
	// The UI is a built React SPA (frontend/dist): real files are served
	// directly, and any other path falls back to index.html so client-side
	// routes (/dashboard, /profile/:did, …) survive a hard refresh or deep link.
	//
	// Route protection lives in the API, not here: /v1/me/* and /v1/auth/me are
	// behind requireOwner, so serving the SPA shell to a signed-out visitor
	// leaks nothing — the app just renders its login redirect.
	if s.AppDir != "" {
		mux.HandleFunc("GET /", s.handleSPA)
	}

	var handler http.Handler = mux
	if s.RateLimitPerMin > 0 {
		// Burst = the per-minute cap; refill at cap/60 tokens per second.
		rl := newRateLimiter(s.RateLimitPerMin, float64(s.RateLimitPerMin)/60.0)
		handler = rateLimitWrites(rl, handler)
	}
	handler = withCORS(handler)
	// Logging is outermost so it records the final status (incl. 429/CORS).
	return withLogging(s.LogWriter, handler)
}

func withCORS(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		h.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}

// --- Handlers ---------------------------------------------------------------

func (s *Server) handleRegister(w http.ResponseWriter, r *http.Request) {
	var c core.Card
	if err := json.NewDecoder(r.Body).Decode(&c); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid card json: "+err.Error())
		return
	}
	if err := c.Verify(); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if _, err := s.Store.PutCard(&c); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	// Opt-in liveness: record config and kick an immediate probe in the
	// background so the profile shows a status without waiting for the sweep.
	if c.Liveness != nil && c.Liveness.Enabled && c.Liveness.URL != "" {
		_ = s.Store.SetLivenessConfig(c.ID, c.Liveness.URL, true)
		go s.probeOne(c.ID, c.Liveness.URL)
	} else if c.Liveness != nil {
		_ = s.Store.SetLivenessConfig(c.ID, c.Liveness.URL, false)
	}
	out, _ := s.recomputeScore(c.ID)
	hash, _ := c.Hash()
	writeJSON(w, http.StatusCreated, map[string]any{
		"id": c.ID, "card_hash": hash, "score": out,
	})
}

func (s *Server) handleGetAgent(w http.ResponseWriter, r *http.Request) {
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
	live, _ := s.Store.GetLiveness(did)
	resp := map[string]any{"card": c, "score": out, "liveness": live}
	// Surface a card-version fork if one was detected (competing signed versions).
	if fork, ferr := s.Store.GetFork(did); ferr == nil && fork != nil {
		resp["fork"] = fork
	}
	// Surface whether this identity has been rotated to a newer agent key.
	if rots, err := s.Store.AllRotations(); err == nil {
		if current, rerr := core.ResolveCurrentAgent(rots, did); rerr == nil && current != did {
			resp["rotated_to"] = current
		}
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleRotation(w http.ResponseWriter, r *http.Request) {
	var rot core.Rotation
	if err := json.NewDecoder(r.Body).Decode(&rot); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid rotation json: "+err.Error())
		return
	}
	if err := rot.Verify(); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	// Only the owner recorded on the old agent's card may rotate its key.
	oldCard, err := s.Store.GetCard(rot.OldAgent)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if oldCard == nil {
		writeErr(w, http.StatusNotFound, "old_agent not found")
		return
	}
	if oldCard.Owner != rot.Owner {
		writeErr(w, http.StatusForbidden, "rotation owner does not match the agent card owner")
		return
	}
	if _, err := s.Store.PutRotation(&rot); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	hash, _ := rot.Hash()
	writeJSON(w, http.StatusCreated, map[string]any{
		"hash": hash, "old_agent": rot.OldAgent, "new_agent": rot.NewAgent,
	})
}

func (s *Server) handleHistory(w http.ResponseWriter, r *http.Request) {
	hist, err := s.Store.CardHistory(r.PathValue("did"))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"versions": hist})
}

func (s *Server) handleAttestations(w http.ResponseWriter, r *http.Request) {
	did := r.PathValue("did")
	limit, offset := pageParams(r)
	atts, total, err := s.Store.AttestationsForSubjectPaged(did, limit, offset)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if atts == nil {
		atts = []*core.Attestation{}
	}
	resp := map[string]any{
		"subject": did, "attestations": atts,
		"total": total, "limit": limit, "offset": offset,
	}
	if next := offset + len(atts); next < total {
		resp["next_offset"] = next
	}
	writeJSON(w, http.StatusOK, resp)
}

// pageParams reads limit/offset query params, defaulting limit to 100.
func pageParams(r *http.Request) (limit, offset int) {
	limit, _ = strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	offset, _ = strconv.Atoi(r.URL.Query().Get("offset"))
	if offset < 0 {
		offset = 0
	}
	return
}

func (s *Server) handleBadge(w http.ResponseWriter, r *http.Request) {
	did := r.PathValue("did")
	c, err := s.Store.GetCard(did)
	if err != nil || c == nil {
		http.Error(w, "agent not found", http.StatusNotFound)
		return
	}
	out, _ := s.recomputeScore(did)
	w.Header().Set("Content-Type", "image/svg+xml")
	w.Header().Set("Cache-Control", "max-age=300")
	_, _ = w.Write([]byte(badgeSVG(c.Name, out)))
}

func (s *Server) handleAttest(w http.ResponseWriter, r *http.Request) {
	var a core.Attestation
	if err := json.NewDecoder(r.Body).Decode(&a); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid attestation json: "+err.Error())
		return
	}
	if err := a.Verify(); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	// Enforce the per-issuer hash chain: prev must match the issuer's current head.
	head, err := s.Store.IssuerHead(a.Issuer)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if a.Prev != head {
		writeErr(w, http.StatusConflict,
			"attestation prev does not match issuer chain head; expected \""+head+"\"")
		return
	}
	if _, err := s.Store.PutAttestation(&a); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	out, _ := s.recomputeScore(a.Subject)
	hash, _ := a.Hash()
	writeJSON(w, http.StatusCreated, map[string]any{"hash": hash, "subject_score": out})
}

func (s *Server) handleLiveness(w http.ResponseWriter, r *http.Request) {
	live, err := s.Store.GetLiveness(r.PathValue("did"))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if live == nil {
		writeJSON(w, http.StatusOK, map[string]any{"enabled": false})
		return
	}
	writeJSON(w, http.StatusOK, live)
}

func (s *Server) handleIssuerHead(w http.ResponseWriter, r *http.Request) {
	head, err := s.Store.IssuerHead(r.PathValue("did"))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"issuer": r.PathValue("did"), "head": head})
}

func (s *Server) handleSearch(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	minScore, _ := strconv.ParseFloat(q.Get("min_score"), 64)
	limit, offset := pageParams(r)
	results, total, err := s.Store.Search(q.Get("q"), q.Get("cap"), minScore, limit, offset)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	resp := map[string]any{
		"count": len(results), "results": results,
		"total": total, "limit": limit, "offset": offset,
	}
	if next := offset + len(results); next < total {
		resp["next_offset"] = next
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleScore(w http.ResponseWriter, r *http.Request) {
	did := r.PathValue("did")
	c, err := s.Store.GetCard(did)
	if err != nil || c == nil {
		writeErr(w, http.StatusNotFound, "agent not found")
		return
	}
	out, err := s.recomputeScore(did)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleTaxonomy(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"tags": Taxonomy})
}

func (s *Server) handleGraph(w http.ResponseWriter, r *http.Request) {
	nodes, edges, err := s.Store.Graph(r.URL.Query().Get("did"))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"nodes": nodes, "edges": edges})
}

func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	// A cheap DB round-trip confirms the store is reachable, not just the process.
	if _, err := s.Store.AgentCount(); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "degraded", "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "version": s.Version})
}

func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	n, _ := s.Store.AgentCount()
	writeJSON(w, http.StatusOK, map[string]any{"agents": n, "instance": s.Name})
}

func (s *Server) handleWellKnown(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"name":       s.Name,
		"software":   "moltnetd",
		"version":    s.Version,
		"spec":       []string{core.CardSpec, core.AttestationSpec, score.Algorithm},
		"protocols":  []string{"rest"},
		"openapi":    "/openapi.json",
		"federation": map[string]any{"pull_based": true, "since_cursor": true},
	})
}

// recomputeScore recomputes a subject's MoltScore using cached issuer scores as
// issuer weights (unknown issuers weighted low as a sybil defense), caches the
// result, and returns it. Clients can always recompute trustlessly from the raw
// chain with default weights.
func (s *Server) recomputeScore(did string) (score.Output, error) {
	atts, err := s.Store.AttestationsForSubject(did)
	if err != nil {
		return score.Output{}, err
	}
	// Per-issuer trust weight + owner resolution for the independence rule.
	// ownerOf maps the subject and each issuer to its controlling owner so
	// score.Compute can drop self-dealing (same-owner) attestations. Owners come
	// from the signed cards, so the discount is reproducible by anyone who
	// fetches those cards — it is a property of the function, not this server.
	weights := map[string]float64{}
	ownerOf := map[string]string{}
	if c, _ := s.Store.GetCard(did); c != nil {
		ownerOf[did] = c.Owner
	}
	for _, a := range atts {
		if _, seen := weights[a.Issuer]; seen {
			continue
		}
		if v, ok, _ := s.Store.CachedScore(a.Issuer); ok {
			weights[a.Issuer] = v / 100.0
		} else {
			weights[a.Issuer] = 0.25 // unregistered / fresh issuer
		}
		if c, _ := s.Store.GetCard(a.Issuer); c != nil {
			ownerOf[a.Issuer] = c.Owner
		}
	}
	out := score.Compute(atts, weights, ownerOf, time.Now().UTC())
	_ = s.Store.SetScore(did, out)
	return out, nil
}
