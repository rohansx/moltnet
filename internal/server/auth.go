package server

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/moltnet/moltnet/core"
	"github.com/moltnet/moltnet/internal/store"
)

// This file implements authentication for moltnetd:
//
//   - Sign-In-With-Key (SIWK) for users (browser dashboard): prove control of
//     an owner key by Ed25519-signing a single-use, domain-bound challenge.
//   - Owner sessions: an opaque HttpOnly cookie (and a bearer token for the
//     CLI), stored only as a SHA-256 hash.
//   - Per-agent API keys (molt_sk_live_…) for programmatic clients, stored
//     only as a hash + a display prefix.
//
// Signed writes (register / attest / rotate) stay signature-authenticated and
// are NOT affected by auth — a session or key can never forge a signed record.
// Auth only gates the private dashboard view and credential management.

// SessionCookie is the cookie name carrying the owner session token.
const SessionCookie = "molt_sess"

// SessionTTL is how long a dashboard session lives.
const SessionTTL = 30 * 24 * time.Hour

// ChallengeTTL is how long a SIWK nonce is valid to sign.
const ChallengeTTL = 10 * time.Minute

const apiKeyPrefix = "molt_sk_live_"
const apiKeyRandLen = 24 // base62 chars after the prefix

// sessionLen is the byte length of a raw session token (hex-encoded → 64 chars).
const sessionLen = 32

// ---- helpers ---------------------------------------------------------------

// randHex returns n random bytes hex-encoded.
func randHex(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// hashToken returns the SHA-256 hex of a secret (session token or API key).
func hashToken(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

// randBase62 returns n chars from the base62 alphabet, uniformly.
//
// Rejection sampling, not modulo: 256 is not a multiple of 62, so `byte % 62`
// would make the first 8 symbols ~25% likelier than the rest and shave entropy
// off every API key. Bytes >= 248 (the largest multiple of 62) are discarded
// and redrawn instead.
func randBase62(n int) (string, error) {
	const alphabet = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"
	const maxUnbiased = 256 - (256 % len(alphabet)) // 248
	out := make([]byte, 0, n)
	buf := make([]byte, n)
	for len(out) < n {
		if _, err := rand.Read(buf); err != nil {
			return "", err
		}
		for _, c := range buf {
			if int(c) >= maxUnbiased {
				continue // biased tail — redraw
			}
			out = append(out, alphabet[int(c)%len(alphabet)])
			if len(out) == n {
				break
			}
		}
	}
	return string(out), nil
}

// generateAPIKey mints a full API key plus its unique id, hash, and display
// hints. `id` — not `prefix` — is the handle used to revoke: the prefix exposes
// only 4 random chars, so two of an owner's keys can collide on it.
func generateAPIKey() (full, id, keyHash, prefix, last4 string, err error) {
	body, err := randBase62(apiKeyRandLen)
	if err != nil {
		return "", "", "", "", "", err
	}
	if id, err = randHex(6); err != nil { // 12 hex chars, unique per key
		return "", "", "", "", "", err
	}
	full = apiKeyPrefix + body
	keyHash = hashToken(full)
	prefix = full[:len(apiKeyPrefix)+4] + "…"
	last4 = full[len(full)-4:]
	return full, id, keyHash, prefix, last4, nil
}

// generateSession mints a raw session token and its hash.
func generateSession() (raw, hashed string, err error) {
	raw, err = randHex(sessionLen)
	if err != nil {
		return "", "", err
	}
	return raw, hashToken(raw), nil
}

// siwkMessageFmt is the format of the payload an owner signs to sign in.
// Binding the domain + a single-use nonce + timestamp defeats replay and
// cross-site signature theft.
const siwkMessageFmt = "MOLTNET-SIWK-v1\n%s\n%s\n%s" // domain, nonce, issued_at

func siwkMessage(domain, nonce, issuedAt string) string {
	return fmt.Sprintf(siwkMessageFmt, domain, nonce, issuedAt)
}

func siwkPayload(domain, nonce, issuedAt string) []byte {
	return []byte(siwkMessage(domain, nonce, issuedAt))
}

// requestDomain returns the host the dashboard is served from, for SIWK
// domain binding. It prefers the Host header.
func requestDomain(r *http.Request) string {
	return r.Host
}

// nowRFC3339 is the current UTC time in RFC3339 (second precision).
func nowRFC3339() string { return time.Now().UTC().Format(time.RFC3339) }

// StartAuthGC reaps spent auth rows on an interval. This is load-bearing, not
// housekeeping: POST /v1/auth/challenge is unauthenticated and writes a row per
// call, so without a reaper anyone can grow the DB without bound — and because
// the store runs on a single write connection, a bloated challenges table drags
// down every other write (register/attest/rotate) too. Passing interval <= 0
// disables it.
func (s *Server) StartAuthGC(interval time.Duration) {
	if interval <= 0 {
		return
	}
	purge := func() {
		now := nowRFC3339()
		if err := s.Store.PurgeChallenges(now); err != nil {
			s.logf("auth gc: purge challenges: %v", err)
		}
		if err := s.Store.PurgeExpiredSessions(now); err != nil {
			s.logf("auth gc: purge sessions: %v", err)
		}
	}
	purge() // reap anything left over from a previous run at startup
	go func() {
		t := time.NewTicker(interval)
		defer t.Stop()
		for range t.C {
			purge()
		}
	}()
}

// logf writes a line to the server's log sink, if one is configured.
func (s *Server) logf(format string, args ...any) {
	if s.LogWriter != nil {
		fmt.Fprintf(s.LogWriter, format+"\n", args...)
	}
}

// setSessionCookie writes the HttpOnly session cookie. Secure is set whenever
// the request actually arrived over TLS (directly, or via a terminating proxy
// that set X-Forwarded-Proto), so a real deployment gets a Secure cookie while
// plain-HTTP localhost dev keeps working.
func setSessionCookie(w http.ResponseWriter, r *http.Request, token string, expires time.Time) {
	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookie,
		Value:    token,
		Path:     "/",
		Expires:  expires,
		MaxAge:   int(SessionTTL / time.Second),
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   isTLS(r),
	})
}

// isTLS reports whether the request reached us over HTTPS.
func isTLS(r *http.Request) bool {
	if r.TLS != nil {
		return true
	}
	return strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
}

func clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name: SessionCookie, Value: "", Path: "/",
		MaxAge: -1, HttpOnly: true, SameSite: http.SameSiteLaxMode,
	})
}

// sessionFromRequest resolves the owner session for a request, looking first at
// the cookie and then at an `Authorization: Bearer <token>` header (for the
// CLI). Returns the owner DID + token hash, or "" if unauthenticated.
func (s *Server) sessionFromRequest(r *http.Request) (ownerDID, tokenHash string) {
	var token string
	if c, err := r.Cookie(SessionCookie); err == nil && c.Value != "" {
		token = c.Value
	} else if h := r.Header.Get("Authorization"); strings.HasPrefix(h, "Bearer ") {
		token = strings.TrimPrefix(h, "Bearer ")
	}
	if token == "" {
		return "", ""
	}
	th := hashToken(token)
	now := time.Now().UTC()
	sess, err := s.Store.GetSession(th, now.Format(time.RFC3339))
	if err != nil || sess == nil {
		return "", ""
	}
	// Only refresh last_seen if it's over a minute stale, so a burst of owner
	// reads doesn't become a burst of writes on the single-writer store.
	_ = s.Store.TouchSession(th, now.Format(time.RFC3339), now.Add(-time.Minute).Format(time.RFC3339))
	return sess.OwnerDID, th
}

// agentKeyFromRequest resolves the agent DID authorized by an
// `Authorization: Bearer molt_sk_live_…` API key, or "" if none/invalid.
func (s *Server) agentKeyFromRequest(r *http.Request) (agentDID, ownerDID string) {
	h := r.Header.Get("Authorization")
	if !strings.HasPrefix(h, "Bearer ") {
		return "", ""
	}
	raw := strings.TrimPrefix(h, "Bearer ")
	if !strings.HasPrefix(raw, apiKeyPrefix) {
		return "", ""
	}
	k, err := s.Store.GetAPIKey(hashToken(raw))
	if err != nil || k == nil {
		return "", ""
	}
	return k.AgentDID, k.OwnerDID
}

// requireOwner is middleware gating a handler behind a valid owner session.
// On failure it writes 401 JSON (API callers) — for the dashboard HTML guard
// The SPA shell is public; the API is the trust boundary.
func (s *Server) requireOwner(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		owner, _ := s.sessionFromRequest(r)
		if owner == "" {
			writeErr(w, http.StatusUnauthorized, "sign in required")
			return
		}
		r = r.WithContext(withOwner(r.Context(), owner))
		h(w, r)
	}
}

type ownerCtxKey struct{}

func withOwner(ctx context.Context, owner string) context.Context {
	return context.WithValue(ctx, ownerCtxKey{}, owner)
}

// ownerFromContext returns the authenticated owner DID, or "".
func ownerFromContext(r *http.Request) string {
	if v, ok := r.Context().Value(ownerCtxKey{}).(string); ok {
		return v
	}
	return ""
}

// ---- SPA + legacy static serving -------------------------------------------

// handleSPA serves the built front-end app (frontend/dist). Real files
// (hashed assets under /assets/) are served directly; everything else falls
// handleSPA serves the built React app (frontend/dist): real files as-is, and
// anything else falls back to index.html so client-side routes (/login,
// /dashboard, /profile/:did, …) resolve on a deep link or hard refresh.
func (s *Server) handleSPA(w http.ResponseWriter, r *http.Request) {
	p := r.URL.Path
	// A real file in the build (e.g. /assets/index-*.js) is served as-is…
	if f := s.AppDir + p; fileExists(f) {
		http.ServeFile(w, r, f)
		return
	}
	// …anything else is a client-side route: hand back the shell so deep links
	// and hard refreshes work. Safe because the API, not this handler, is the
	// trust boundary (/v1/me/* and /v1/auth/me require a session).
	http.ServeFile(w, r, s.AppDir+"/index.html")
}

func fileExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && !st.IsDir()
}

// ---- dashboard HTML gate --------------------------------------------------

// ---- handlers --------------------------------------------------------------

// POST /v1/auth/challenge
//
//	body: {"did":"did:key:z…"}
//	→ {"nonce","domain","issued_at","expires_at"}
//	Anyone may request a challenge; the cost is bounded by write rate limiting.
func (s *Server) handleAuthChallenge(w http.ResponseWriter, r *http.Request) {
	var body struct {
		DID string `json:"did"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	if !strings.HasPrefix(body.DID, "did:key:") {
		writeErr(w, http.StatusBadRequest, "did must be a did:key")
		return
	}
	// Fail fast if the key is not decodable — no point issuing a challenge for
	// a malformed DID.
	if _, err := core.PublicKeyFromDID(body.DID); err != nil {
		writeErr(w, http.StatusBadRequest, "unrecognized did:key: "+err.Error())
		return
	}
	nonce, err := randHex(32)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "nonce: "+err.Error())
		return
	}
	now := time.Now().UTC()
	exp := now.Add(ChallengeTTL)
	if err := s.Store.CreateChallenge(nonce, now.Format(time.RFC3339), exp.Format(time.RFC3339)); err != nil {
		writeErr(w, http.StatusInternalServerError, "store: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{
		"nonce":      nonce,
		"domain":     requestDomain(r),
		"issued_at":  now.Format(time.RFC3339),
		"expires_at": exp.Format(time.RFC3339),
		"message":    siwkMessage(requestDomain(r), nonce, now.Format(time.RFC3339)),
	})
}

// POST /v1/auth/login
//
//	body: {"did","nonce","sig"}
//	Verifies the single-use challenge, the owner signature, and that the DID
//	is decodable. Issues a session cookie + a bearer token.
func (s *Server) handleAuthLogin(w http.ResponseWriter, r *http.Request) {
	var body struct {
		DID   string `json:"did"`
		Nonce string `json:"nonce"`
		Sig   string `json:"sig"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	if body.DID == "" || body.Nonce == "" || body.Sig == "" {
		writeErr(w, http.StatusBadRequest, "did, nonce and sig are required")
		return
	}
	now := time.Now().UTC().Format(time.RFC3339)
	// Look up the challenge to rebind the signature to the same issued_at it
	// was minted with — clients sign the exact message we returned.
	var ch storeChallenge
	if err := s.dbChallenge(body.Nonce, &ch); err != nil || ch.Nonce == "" {
		writeErr(w, http.StatusUnauthorized, "challenge not found")
		return
	}
	if ch.Used {
		writeErr(w, http.StatusUnauthorized, "challenge already used")
		return
	}
	if ch.ExpiresAt < now {
		writeErr(w, http.StatusUnauthorized, "challenge expired")
		return
	}
	payload := siwkPayload(requestDomain(r), body.Nonce, ch.IssuedAt)
	if err := core.Verify(body.DID, payload, body.Sig); err != nil {
		writeErr(w, http.StatusUnauthorized, "signature invalid: "+err.Error())
		return
	}
	// Single-use: consume AFTER a successful verify.
	if ok, err := s.Store.ConsumeChallenge(body.Nonce, now); err != nil || !ok {
		writeErr(w, http.StatusUnauthorized, "challenge could not be consumed")
		return
	}
	token, th, err := generateSession()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "session: "+err.Error())
		return
	}
	exp := time.Now().UTC().Add(SessionTTL)
	if err := s.Store.CreateSession(th, body.DID, now, exp.Format(time.RFC3339)); err != nil {
		writeErr(w, http.StatusInternalServerError, "store: "+err.Error())
		return
	}
	setSessionCookie(w, r, token, exp)
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":         true,
		"owner_did":  body.DID,
		"session":    token,
		"expires_at": exp.Format(time.RFC3339),
	})
}

// storeChallenge is a tiny local view of a challenge row, used to rebind the
// signed issued_at without re-querying through the store's consume path.
type storeChallenge struct {
	Nonce     string
	IssuedAt  string
	ExpiresAt string
	Used      bool
}

func (s *Server) dbChallenge(nonce string, out *storeChallenge) error {
	row := s.Store.DB().QueryRow(
		`SELECT nonce, issued_at, expires_at, used FROM challenges WHERE nonce = ?`, nonce)
	var used int
	if err := row.Scan(&out.Nonce, &out.IssuedAt, &out.ExpiresAt, &used); err != nil {
		return err
	}
	out.Used = used != 0
	return nil
}

// POST /v1/auth/logout
func (s *Server) handleAuthLogout(w http.ResponseWriter, r *http.Request) {
	_, th := s.sessionFromRequest(r)
	if th != "" {
		_ = s.Store.DeleteSession(th)
	}
	clearSessionCookie(w)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// GET /v1/auth/me  → {owner_did, agents:[…]} for the session owner.
func (s *Server) handleAuthMe(w http.ResponseWriter, r *http.Request) {
	owner := ownerFromContext(r)
	if owner == "" {
		writeErr(w, http.StatusUnauthorized, "sign in required")
		return
	}
	agents, _ := s.Store.AgentsByOwner(owner)
	if agents == nil {
		agents = []store.Agent{}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"owner_did": owner,
		"agents":    agents,
	})
}

// GET /v1/me/agents — same agent list as /v1/auth/me, kept as a distinct
// resource the dashboard's "my agents" view fetches directly.
func (s *Server) handleMyAgents(w http.ResponseWriter, r *http.Request) {
	owner := ownerFromContext(r)
	agents, _ := s.Store.AgentsByOwner(owner)
	if agents == nil {
		agents = []store.Agent{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"owner": owner, "agents": agents})
}

// GET /v1/me/apikeys — list the session owner's API keys (last4 only).
func (s *Server) handleListAPIKeys(w http.ResponseWriter, r *http.Request) {
	owner := ownerFromContext(r)
	keys, _ := s.Store.APIKeysForOwner(owner)
	if keys == nil {
		keys = []store.APIKey{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"keys": keys})
}

// POST /v1/me/apikeys
//
//	body: {"agent_did","name"}  (agent must be owned by the session owner)
//	→ {key:"molt_sk_live_…", prefix, last4, agent_did}  (key shown once)
func (s *Server) handleCreateAPIKey(w http.ResponseWriter, r *http.Request) {
	owner := ownerFromContext(r)
	var body struct {
		AgentDID string `json:"agent_did"`
		Name     string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	if body.AgentDID == "" {
		writeErr(w, http.StatusBadRequest, "agent_did is required")
		return
	}
	if body.Name == "" {
		body.Name = "default"
	}
	// Verify the agent exists AND is owned by the session owner. This is the
	// authorization check that ties a key to an identity you control.
	card, err := s.Store.GetCard(body.AgentDID)
	if err != nil || card == nil {
		writeErr(w, http.StatusNotFound, "agent not found")
		return
	}
	if card.Owner != owner {
		writeErr(w, http.StatusForbidden, "you do not own this agent")
		return
	}
	full, id, kh, prefix, last4, err := generateAPIKey()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "keygen: "+err.Error())
		return
	}
	if err := s.Store.CreateAPIKey(id, kh, body.AgentDID, owner, body.Name, prefix, last4, nowRFC3339()); err != nil {
		writeErr(w, http.StatusInternalServerError, "store: "+err.Error())
		return
	}
	// `key` is returned exactly once and never stored in the clear.
	writeJSON(w, http.StatusCreated, map[string]any{
		"key":       full,
		"id":        id,
		"prefix":    prefix,
		"last4":     last4,
		"agent_did": body.AgentDID,
		"name":      body.Name,
		"created":   true,
	})
}

// DELETE /v1/me/apikeys/{prefix} — revoke by display prefix. The caller must
// the key's unique id (from the mint response or the list endpoint).
//
// The UPDATE is scoped to the session owner, so knowing another owner's key id
// is not enough to revoke it — that scoping IS the authorization check.
func (s *Server) handleRevokeAPIKey(w http.ResponseWriter, r *http.Request) {
	owner := ownerFromContext(r)
	id := r.PathValue("id")
	if id == "" {
		writeErr(w, http.StatusBadRequest, "key id is required")
		return
	}
	revoked, err := s.Store.RevokeAPIKey(id, owner, nowRFC3339())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !revoked {
		writeErr(w, http.StatusNotFound, "no live key with that id")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"revoked": true})
}

// ---- agent-key endpoint ----------------------------------------------------

// GET /v1/agent/me — the agent's own card + score + liveness, authorized by
// an API key. Lets a programmatic client fetch its own state without a
// signature round-trip. Reads only.
func (s *Server) handleAgentMe(w http.ResponseWriter, r *http.Request) {
	agentDID, ownerDID := s.agentKeyFromRequest(r)
	if agentDID == "" {
		writeErr(w, http.StatusUnauthorized, "valid agent API key required")
		return
	}
	c, err := s.Store.GetCard(agentDID)
	if err != nil || c == nil {
		writeErr(w, http.StatusNotFound, "agent not found")
		return
	}
	out, _ := s.recomputeScore(agentDID)
	live, _ := s.Store.GetLiveness(agentDID)
	writeJSON(w, http.StatusOK, map[string]any{
		"card":     c,
		"score":    out,
		"liveness": live,
		"owner":    ownerDID,
	})
}
