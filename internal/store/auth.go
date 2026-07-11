package store

import (
	"database/sql"
	"time"
)

// This file is the auth persistence layer: single-use SIWK challenges, owner
// sessions, and per-agent API keys. Only hashes are stored — never a raw
// session token, never a raw API key, never a private key.

// ---- SIWK challenges -------------------------------------------------------

// Challenge is an issued sign-in challenge. A user proves control of an owner
// key by Ed25519-signing a message binding the nonce + domain + timestamp; the
// nonce is single-use so a captured signature cannot replay.
type Challenge struct {
	Nonce     string `json:"nonce"`
	IssuedAt  string `json:"issued_at"`
	ExpiresAt string `json:"expires_at"`
}

// CreateChallenge records a fresh single-use challenge.
func (s *Store) CreateChallenge(nonce, issuedAt, expiresAt string) error {
	_, err := s.db.Exec(
		`INSERT INTO challenges (nonce, issued_at, expires_at, used) VALUES (?, ?, ?, 0)`,
		nonce, issuedAt, expiresAt)
	return err
}

// ConsumeChallenge marks a challenge used. It returns (false, nil) if the
// nonce is absent, already used, or expired — i.e. not consumable.
func (s *Store) ConsumeChallenge(nonce, now string) (bool, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return false, err
	}
	defer tx.Rollback()

	var used int
	var expiresAt string
	err = tx.QueryRow(`SELECT used, expires_at FROM challenges WHERE nonce = ?`, nonce).Scan(&used, &expiresAt)
	if err == sql.ErrNoRows {
		return false, tx.Commit()
	}
	if err != nil {
		return false, err
	}
	if used != 0 {
		return false, tx.Commit()
	}
	if expiresAt < now {
		return false, tx.Commit()
	}
	if _, err = tx.Exec(`UPDATE challenges SET used = 1 WHERE nonce = ?`, nonce); err != nil {
		return false, err
	}
	return true, tx.Commit()
}

// PurgeChallenges deletes expired/used challenges older than cutoff. Best-effort.
func (s *Store) PurgeChallenges(cutoff string) error {
	_, err := s.db.Exec(`DELETE FROM challenges WHERE expires_at < ? OR used = 1`, cutoff)
	return err
}

// ---- Sessions --------------------------------------------------------------

// Session is an authenticated owner session. TokenHash is SHA-256 of the raw
// token; the raw token is handed to the client once and never stored.
type Session struct {
	OwnerDID  string `json:"owner_did"`
	CreatedAt string `json:"created_at"`
	ExpiresAt string `json:"expires_at"`
	LastSeen  string `json:"last_seen,omitempty"`
}

// CreateSession stores a session keyed by the hash of its token.
func (s *Store) CreateSession(tokenHash, ownerDID, createdAt, expiresAt string) error {
	_, err := s.db.Exec(
		`INSERT INTO sessions (token_hash, owner_did, created_at, expires_at, last_seen)
         VALUES (?, ?, ?, ?, ?)`,
		tokenHash, ownerDID, createdAt, expiresAt, createdAt)
	return err
}

// GetSession returns the session for a token hash, or (nil, nil) if there is
// no matching, unexpired session.
func (s *Store) GetSession(tokenHash, now string) (*Session, error) {
	var sess Session
	err := s.db.QueryRow(
		`SELECT owner_did, created_at, expires_at, COALESCE(last_seen,'')
         FROM sessions WHERE token_hash = ? AND expires_at > ?`,
		tokenHash, now).Scan(&sess.OwnerDID, &sess.CreatedAt, &sess.ExpiresAt, &sess.LastSeen)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &sess, nil
}

// TouchSession updates last_seen for a session.
func (s *Store) TouchSession(tokenHash, lastSeen string) error {
	_, err := s.db.Exec(`UPDATE sessions SET last_seen = ? WHERE token_hash = ?`, lastSeen, tokenHash)
	return err
}

// DeleteSession removes a session (logout).
func (s *Store) DeleteSession(tokenHash string) error {
	_, err := s.db.Exec(`DELETE FROM sessions WHERE token_hash = ?`, tokenHash)
	return err
}

// ---- API keys --------------------------------------------------------------

// APIKey is a per-agent programmatic credential. The raw key is shown to the
// caller exactly once at mint time; only the hash + a display prefix survive.
type APIKey struct {
	ID        string `json:"id"` // short prefix, used as a client-side id
	AgentDID  string `json:"agent_did"`
	OwnerDID  string `json:"owner_did"`
	Name      string `json:"name"`
	Prefix    string `json:"prefix"`
	Last4     string `json:"last4"`
	CreatedAt string `json:"created_at"`
	RevokedAt string `json:"revoked_at,omitempty"`
}

// CreateAPIKey stores a hashed API key. Prefix is a non-secret display hint
// (e.g. "molt_sk_live_…ab12"); last4 is the final 4 chars.
func (s *Store) CreateAPIKey(keyHash, agentDID, ownerDID, name, prefix, last4, createdAt string) error {
	_, err := s.db.Exec(
		`INSERT INTO api_keys (key_hash, agent_did, owner_did, name, prefix, last4, created_at)
         VALUES (?, ?, ?, ?, ?, ?, ?)`,
		keyHash, agentDID, ownerDID, name, prefix, last4, createdAt)
	return err
}

// GetAPIKey returns the (non-revoked) API key for a key hash, or (nil, nil).
func (s *Store) GetAPIKey(keyHash string) (*APIKey, error) {
	var k APIKey
	var revokedAt sql.NullString
	err := s.db.QueryRow(
		`SELECT key_hash, agent_did, owner_did, name, prefix, last4, created_at, revoked_at
         FROM api_keys WHERE key_hash = ? AND revoked_at IS NULL`, keyHash).
		Scan(&k.ID, &k.AgentDID, &k.OwnerDID, &k.Name, &k.Prefix, &k.Last4, &k.CreatedAt, &revokedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if revokedAt.Valid {
		k.RevokedAt = revokedAt.String
	}
	k.ID = k.Prefix
	return &k, nil
}

// APIKeysForOwner lists all API keys (revoked ones last) for an owner's agents.
func (s *Store) APIKeysForOwner(ownerDID string) ([]APIKey, error) {
	rows, err := s.db.Query(
		`SELECT key_hash, agent_did, owner_did, name, prefix, last4, created_at, COALESCE(revoked_at,'')
         FROM api_keys WHERE owner_did = ?
         ORDER BY (revoked_at IS NULL) DESC, created_at DESC`, ownerDID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []APIKey
	for rows.Next() {
		var k APIKey
		var rv string
		if err := rows.Scan(&k.ID, &k.AgentDID, &k.OwnerDID, &k.Name, &k.Prefix, &k.Last4, &k.CreatedAt, &rv); err != nil {
			return nil, err
		}
		k.RevokedAt = rv
		k.ID = k.Prefix
		out = append(out, k)
	}
	return out, rows.Err()
}

// RevokeAPIKey marks an API key revoked. It must belong to the given owner.
// Returns (false, nil) if no matching live key exists.
func (s *Store) RevokeAPIKey(keyHash, ownerDID, revokedAt string) (bool, error) {
	res, err := s.db.Exec(
		`UPDATE api_keys SET revoked_at = ?
         WHERE key_hash = ? AND owner_did = ? AND revoked_at IS NULL`,
		revokedAt, keyHash, ownerDID)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// APIKeyHashByPrefix returns the stored key hash for an owner's live key
// with the given display prefix, or "" if none. Used by the revoke flow.
func (s *Store) APIKeyHashByPrefix(ownerDID, prefix string) (string, error) {
	var kh string
	err := s.db.QueryRow(
		`SELECT key_hash FROM api_keys WHERE owner_did = ? AND prefix = ? AND revoked_at IS NULL`,
		ownerDID, prefix).Scan(&kh)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return kh, err
}

// AgentsByOwner lists every agent whose card owner is ownerDID, newest first,
// with its cached score. This is the data behind the user dashboard.
func (s *Store) AgentsByOwner(ownerDID string) ([]Agent, error) {
	rows, err := s.db.Query(
		`SELECT a.did, a.name, COALESCE(a.description,''), COALESCE(a.capabilities,''), COALESCE(s.score,0)
         FROM agents a LEFT JOIN scores s ON s.did = a.did
         WHERE json_extract(a.card_json, '$.owner') = ?
         ORDER BY a.updated_at DESC`, ownerDID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Agent
	for rows.Next() {
		var a Agent
		var caps string
		if err := rows.Scan(&a.DID, &a.Name, &a.Description, &caps, &a.Score); err != nil {
			return nil, err
		}
		if caps != "" {
			a.Capabilities = splitFields(caps)
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// nowRFC3339 returns the current UTC time in RFC3339 form, with second
// precision (matching the rest of the codebase).
func nowRFC3339() string {
	return time.Now().UTC().Format(time.RFC3339)
}
