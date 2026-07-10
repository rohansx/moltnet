// Package server implements moltnetd's HTTP surface: the REST API, badge SVGs,
// instance metadata and the static web UI. Writes are authenticated by
// signatures, not sessions — trust lives in the signed records themselves.
package server

import (
	"encoding/json"
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
	WebDir  string // optional path to static web assets ("" disables)
	Name    string
	Version string
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
	mux.HandleFunc("POST /v1/attestations", s.handleAttest)
	mux.HandleFunc("GET /v1/issuers/{did}/head", s.handleIssuerHead)
	mux.HandleFunc("GET /v1/search", s.handleSearch)
	mux.HandleFunc("GET /v1/score/{did}", s.handleScore)
	mux.HandleFunc("GET /v1/taxonomy", s.handleTaxonomy)
	mux.HandleFunc("GET /.well-known/moltnet", s.handleWellKnown)
	mux.HandleFunc("GET /v1/stats", s.handleStats)

	if s.WebDir != "" {
		mux.Handle("/", http.FileServer(http.Dir(s.WebDir)))
	}
	return withCORS(mux)
}

func withCORS(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
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
	if err := s.Store.PutCard(&c); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
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
	writeJSON(w, http.StatusOK, map[string]any{"card": c, "score": out})
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
	atts, err := s.Store.AttestationsForSubject(did)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"subject": did, "attestations": atts})
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
	if err := s.Store.PutAttestation(&a); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	out, _ := s.recomputeScore(a.Subject)
	hash, _ := a.Hash()
	writeJSON(w, http.StatusCreated, map[string]any{"hash": hash, "subject_score": out})
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
	limit, _ := strconv.Atoi(q.Get("limit"))
	results, err := s.Store.Search(q.Get("q"), q.Get("cap"), minScore, limit)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"count": len(results), "results": results})
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
	weights := map[string]float64{}
	for _, a := range atts {
		if _, seen := weights[a.Issuer]; seen {
			continue
		}
		if v, ok, _ := s.Store.CachedScore(a.Issuer); ok {
			weights[a.Issuer] = v / 100.0
		} else {
			weights[a.Issuer] = 0.25 // unregistered / fresh issuer
		}
	}
	out := score.Compute(atts, weights, time.Now().UTC())
	_ = s.Store.SetScore(did, out)
	return out, nil
}
