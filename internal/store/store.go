// Package store is the append-only persistence layer for moltnetd: agent cards,
// their version history, attestations, and cached MoltScores. The raw canonical
// JSON blob is the source of truth; indexed columns are rebuildable.
package store

import (
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/moltnet/moltnet/core"
	"github.com/moltnet/moltnet/score"

	_ "modernc.org/sqlite"
)

type Store struct {
	db *sql.DB
}

const schema = `
CREATE TABLE IF NOT EXISTS agents (
    did         TEXT PRIMARY KEY,
    name        TEXT NOT NULL,
    description TEXT,
    capabilities TEXT,     -- space-joined tags for LIKE search
    card_hash   TEXT NOT NULL,
    card_json   TEXT NOT NULL,
    version     TEXT,
    created_at  TEXT,
    updated_at  TEXT
);
CREATE TABLE IF NOT EXISTS card_history (
    id        INTEGER PRIMARY KEY AUTOINCREMENT,
    did       TEXT NOT NULL,
    card_hash TEXT NOT NULL,
    card_json TEXT NOT NULL,
    ts        TEXT
);
CREATE INDEX IF NOT EXISTS idx_history_did ON card_history(did);
CREATE TABLE IF NOT EXISTS attestations (
    hash      TEXT PRIMARY KEY,
    issuer    TEXT NOT NULL,
    subject   TEXT NOT NULL,
    type      TEXT NOT NULL,
    prev      TEXT,
    issued_at TEXT,
    raw_json  TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_att_subject ON attestations(subject);
CREATE INDEX IF NOT EXISTS idx_att_issuer  ON attestations(issuer);
CREATE TABLE IF NOT EXISTS scores (
    did        TEXT PRIMARY KEY,
    score      REAL NOT NULL,
    output_json TEXT NOT NULL,
    updated_at TEXT
);
CREATE TABLE IF NOT EXISTS liveness (
    did         TEXT PRIMARY KEY,
    url         TEXT NOT NULL,
    enabled     INTEGER NOT NULL DEFAULT 0,
    reachable   INTEGER,
    status_code INTEGER,
    latency_ms  INTEGER,
    checked_at  TEXT
);
CREATE TABLE IF NOT EXISTS events (
    seq      INTEGER PRIMARY KEY AUTOINCREMENT,
    kind     TEXT NOT NULL,       -- 'card' | 'attestation'
    hash     TEXT NOT NULL,       -- content hash of the record
    record   TEXT NOT NULL,       -- the full signed JSON record
    ts       TEXT
);
CREATE TABLE IF NOT EXISTS peer_cursors (
    peer   TEXT PRIMARY KEY,
    cursor INTEGER NOT NULL DEFAULT 0
);
CREATE TABLE IF NOT EXISTS rotations (
    hash      TEXT PRIMARY KEY,
    owner     TEXT NOT NULL,
    old_agent TEXT NOT NULL,
    new_agent TEXT NOT NULL,
    issued_at TEXT,
    raw_json  TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_rot_old ON rotations(old_agent);
`

// Open opens (creating if needed) a SQLite-backed store at path. Use ":memory:"
// for an ephemeral store.
func Open(path string) (*Store, error) {
	dsn := path
	if path != ":memory:" {
		dsn = "file:" + path + "?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)"
	}
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1) // serialize writes; simplest correct model for v0.1
	if _, err := db.Exec(schema); err != nil {
		return nil, fmt.Errorf("init schema: %w", err)
	}
	return &Store{db: db}, nil
}

func (s *Store) Close() error { return s.db.Close() }

func capabilityBlob(c *core.Card) string {
	out := ""
	for i, cap := range c.Capabilities {
		if i > 0 {
			out += " "
		}
		out += cap.Tag
	}
	return out
}

// PutCard inserts or updates an agent's current card and appends to history.
// It is idempotent: if the stored card already has this exact hash, it is a
// no-op and reports changed=false (so federation re-syncs don't amplify).
func (s *Store) PutCard(c *core.Card) (bool, error) {
	hash, err := c.Hash()
	if err != nil {
		return false, err
	}
	raw, err := json.Marshal(c)
	if err != nil {
		return false, err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return false, err
	}
	defer tx.Rollback()

	var existing string
	_ = tx.QueryRow(`SELECT card_hash FROM agents WHERE did = ?`, c.ID).Scan(&existing)
	if existing == hash {
		return false, tx.Commit() // already current
	}

	_, err = tx.Exec(`
        INSERT INTO agents (did, name, description, capabilities, card_hash, card_json, version, created_at, updated_at)
        VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
        ON CONFLICT(did) DO UPDATE SET
            name=excluded.name, description=excluded.description,
            capabilities=excluded.capabilities, card_hash=excluded.card_hash,
            card_json=excluded.card_json, version=excluded.version,
            updated_at=excluded.updated_at`,
		c.ID, c.Name, c.Description, capabilityBlob(c), hash, string(raw), c.Version,
		c.CreatedAt, c.CreatedAt)
	if err != nil {
		return false, err
	}
	if _, err = tx.Exec(
		`INSERT INTO card_history (did, card_hash, card_json, ts) VALUES (?, ?, ?, ?)`,
		c.ID, hash, string(raw), c.CreatedAt); err != nil {
		return false, err
	}
	if err = appendEvent(tx, "card", hash, string(raw), c.CreatedAt); err != nil {
		return false, err
	}
	return true, tx.Commit()
}

// appendEvent records a change in the federation event log within a tx.
func appendEvent(tx *sql.Tx, kind, hash, record, ts string) error {
	_, err := tx.Exec(`INSERT INTO events (kind, hash, record, ts) VALUES (?, ?, ?, ?)`,
		kind, hash, record, ts)
	return err
}

// GetCard returns the current card for a DID, or (nil, nil) if not found.
func (s *Store) GetCard(did string) (*core.Card, error) {
	var raw string
	err := s.db.QueryRow(`SELECT card_json FROM agents WHERE did = ?`, did).Scan(&raw)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var c core.Card
	if err := json.Unmarshal([]byte(raw), &c); err != nil {
		return nil, err
	}
	return &c, nil
}

// CardHistory returns every version of a card, oldest first.
func (s *Store) CardHistory(did string) ([]*core.Card, error) {
	rows, err := s.db.Query(
		`SELECT card_json FROM card_history WHERE did = ? ORDER BY id ASC`, did)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*core.Card
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		var c core.Card
		if err := json.Unmarshal([]byte(raw), &c); err != nil {
			return nil, err
		}
		out = append(out, &c)
	}
	return out, rows.Err()
}

// IssuerHead returns the hash of an issuer's most recent attestation (its chain
// head), or "" if the issuer has none.
func (s *Store) IssuerHead(issuer string) (string, error) {
	rows, err := s.db.Query(
		`SELECT raw_json FROM attestations WHERE issuer = ? ORDER BY issued_at ASC`, issuer)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	var lastHash string
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return "", err
		}
		var a core.Attestation
		if err := json.Unmarshal([]byte(raw), &a); err != nil {
			return "", err
		}
		h, err := a.Hash()
		if err != nil {
			return "", err
		}
		lastHash = h
	}
	return lastHash, rows.Err()
}

// PutAttestation stores a verified attestation. Idempotent on content hash: a
// duplicate (e.g. re-synced from a peer) is ignored and reports inserted=false.
func (s *Store) PutAttestation(a *core.Attestation) (bool, error) {
	hash, err := a.Hash()
	if err != nil {
		return false, err
	}
	raw, err := json.Marshal(a)
	if err != nil {
		return false, err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return false, err
	}
	defer tx.Rollback()

	res, err := tx.Exec(
		`INSERT INTO attestations (hash, issuer, subject, type, prev, issued_at, raw_json)
         VALUES (?, ?, ?, ?, ?, ?, ?)
         ON CONFLICT(hash) DO NOTHING`,
		hash, a.Issuer, a.Subject, a.Type, a.Prev, a.IssuedAt, string(raw))
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return false, tx.Commit() // already have it
	}
	if err = appendEvent(tx, "attestation", hash, string(raw), a.IssuedAt); err != nil {
		return false, err
	}
	return true, tx.Commit()
}

// PutRotation stores a verified key-rotation record. Idempotent on content
// hash; emits a federation event when newly stored.
func (s *Store) PutRotation(r *core.Rotation) (bool, error) {
	hash, err := r.Hash()
	if err != nil {
		return false, err
	}
	raw, err := json.Marshal(r)
	if err != nil {
		return false, err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return false, err
	}
	defer tx.Rollback()

	res, err := tx.Exec(
		`INSERT INTO rotations (hash, owner, old_agent, new_agent, issued_at, raw_json)
         VALUES (?, ?, ?, ?, ?, ?) ON CONFLICT(hash) DO NOTHING`,
		hash, r.Owner, r.OldAgent, r.NewAgent, r.IssuedAt, string(raw))
	if err != nil {
		return false, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return false, tx.Commit()
	}
	if err = appendEvent(tx, "rotation", hash, string(raw), r.IssuedAt); err != nil {
		return false, err
	}
	return true, tx.Commit()
}

// AllRotations returns every rotation record (used to resolve current DIDs).
func (s *Store) AllRotations() ([]*core.Rotation, error) {
	rows, err := s.db.Query(`SELECT raw_json FROM rotations ORDER BY issued_at ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*core.Rotation
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		var r core.Rotation
		if err := json.Unmarshal([]byte(raw), &r); err != nil {
			return nil, err
		}
		out = append(out, &r)
	}
	return out, rows.Err()
}

// Event is a single entry in the federation change feed.
type Event struct {
	Seq    int64           `json:"seq"`
	Kind   string          `json:"kind"`
	Hash   string          `json:"hash"`
	Record json.RawMessage `json:"record"`
}

// Changes returns federation events with seq greater than since, oldest first.
func (s *Store) Changes(since int64, limit int) ([]Event, error) {
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	rows, err := s.db.Query(
		`SELECT seq, kind, hash, record FROM events WHERE seq > ? ORDER BY seq ASC LIMIT ?`,
		since, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Event
	for rows.Next() {
		var e Event
		var record string
		if err := rows.Scan(&e.Seq, &e.Kind, &e.Hash, &record); err != nil {
			return nil, err
		}
		e.Record = json.RawMessage(record)
		out = append(out, e)
	}
	return out, rows.Err()
}

// LatestSeq returns the highest event sequence number (0 if none).
func (s *Store) LatestSeq() (int64, error) {
	var seq sql.NullInt64
	if err := s.db.QueryRow(`SELECT MAX(seq) FROM events`).Scan(&seq); err != nil {
		return 0, err
	}
	return seq.Int64, nil
}

// GetPeerCursor / SetPeerCursor track how far this instance has synced a peer.
func (s *Store) GetPeerCursor(peer string) (int64, error) {
	var c int64
	err := s.db.QueryRow(`SELECT cursor FROM peer_cursors WHERE peer = ?`, peer).Scan(&c)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	return c, err
}

func (s *Store) SetPeerCursor(peer string, cursor int64) error {
	_, err := s.db.Exec(
		`INSERT INTO peer_cursors (peer, cursor) VALUES (?, ?)
         ON CONFLICT(peer) DO UPDATE SET cursor=excluded.cursor`, peer, cursor)
	return err
}

// AttestationsForSubject returns every attestation about a subject, oldest first.
func (s *Store) AttestationsForSubject(did string) ([]*core.Attestation, error) {
	return s.queryAttestations(`SELECT raw_json FROM attestations WHERE subject = ? ORDER BY issued_at ASC`, did)
}

func (s *Store) queryAttestations(q string, args ...any) ([]*core.Attestation, error) {
	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*core.Attestation
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		var a core.Attestation
		if err := json.Unmarshal([]byte(raw), &a); err != nil {
			return nil, err
		}
		out = append(out, &a)
	}
	return out, rows.Err()
}

// SetScore caches a computed score for a DID.
func (s *Store) SetScore(did string, out score.Output) error {
	raw, err := json.Marshal(out)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(
		`INSERT INTO scores (did, score, output_json, updated_at) VALUES (?, ?, ?, ?)
         ON CONFLICT(did) DO UPDATE SET score=excluded.score, output_json=excluded.output_json, updated_at=excluded.updated_at`,
		did, out.Score, string(raw), out.ComputedAt)
	return err
}

// CachedScore returns the cached score value for a DID (0 if none), used as an
// issuer weight input.
func (s *Store) CachedScore(did string) (float64, bool, error) {
	var v float64
	err := s.db.QueryRow(`SELECT score FROM scores WHERE did = ?`, did).Scan(&v)
	if err == sql.ErrNoRows {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	return v, true, nil
}

// Liveness is the stored health-probe config and last observation for an agent.
type Liveness struct {
	DID        string `json:"did"`
	URL        string `json:"url"`
	Enabled    bool   `json:"enabled"`
	Reachable  *bool  `json:"reachable"`
	StatusCode *int   `json:"status_code,omitempty"`
	LatencyMs  *int   `json:"latency_ms,omitempty"`
	CheckedAt  string `json:"checked_at,omitempty"`
}

// SetLivenessConfig upserts an agent's probe URL and enabled flag, preserving
// any prior observation.
func (s *Store) SetLivenessConfig(did, url string, enabled bool) error {
	e := 0
	if enabled {
		e = 1
	}
	_, err := s.db.Exec(
		`INSERT INTO liveness (did, url, enabled) VALUES (?, ?, ?)
         ON CONFLICT(did) DO UPDATE SET url=excluded.url, enabled=excluded.enabled`,
		did, url, e)
	return err
}

// RecordLiveness stores the result of a probe.
func (s *Store) RecordLiveness(did string, reachable bool, status, latencyMs int, checkedAt string) error {
	r := 0
	if reachable {
		r = 1
	}
	_, err := s.db.Exec(
		`UPDATE liveness SET reachable=?, status_code=?, latency_ms=?, checked_at=? WHERE did=?`,
		r, status, latencyMs, checkedAt, did)
	return err
}

// GetLiveness returns an agent's liveness row, or (nil, nil) if none.
func (s *Store) GetLiveness(did string) (*Liveness, error) {
	var l Liveness
	var enabled int
	var reachable, status, latency *int64
	var checkedAt *string
	err := s.db.QueryRow(
		`SELECT did, url, enabled, reachable, status_code, latency_ms, checked_at FROM liveness WHERE did=?`, did).
		Scan(&l.DID, &l.URL, &enabled, &reachable, &status, &latency, &checkedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	l.Enabled = enabled == 1
	if reachable != nil {
		b := *reachable == 1
		l.Reachable = &b
	}
	if status != nil {
		i := int(*status)
		l.StatusCode = &i
	}
	if latency != nil {
		i := int(*latency)
		l.LatencyMs = &i
	}
	if checkedAt != nil {
		l.CheckedAt = *checkedAt
	}
	return &l, nil
}

// EnabledLivenessTargets returns all agents with liveness probing enabled.
func (s *Store) EnabledLivenessTargets() ([]Liveness, error) {
	rows, err := s.db.Query(`SELECT did, url FROM liveness WHERE enabled=1`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Liveness
	for rows.Next() {
		var l Liveness
		if err := rows.Scan(&l.DID, &l.URL); err != nil {
			return nil, err
		}
		l.Enabled = true
		out = append(out, l)
	}
	return out, rows.Err()
}

// Agent is a lightweight row for search/listing.
type Agent struct {
	DID          string   `json:"id"`
	Name         string   `json:"name"`
	Description  string   `json:"description"`
	Capabilities []string `json:"capabilities"`
	Score        float64  `json:"score"`
}

// Search returns agents matching a free-text query, an optional capability tag,
// and a minimum score, ordered by score descending.
func (s *Store) Search(q, capTag string, minScore float64, limit int) ([]Agent, error) {
	if limit <= 0 {
		limit = 50
	}
	sqlText := `
        SELECT a.did, a.name, COALESCE(a.description,''), COALESCE(a.capabilities,''),
               COALESCE(s.score, 0)
        FROM agents a LEFT JOIN scores s ON s.did = a.did
        WHERE (? = '' OR a.name LIKE '%'||?||'%' OR a.description LIKE '%'||?||'%' OR a.capabilities LIKE '%'||?||'%')
          AND (? = '' OR a.capabilities LIKE '%'||?||'%')
          AND COALESCE(s.score,0) >= ?
        ORDER BY COALESCE(s.score,0) DESC
        LIMIT ?`
	rows, err := s.db.Query(sqlText, q, q, q, q, capTag, capTag, minScore, limit)
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

// AgentCount returns the number of registered agents.
func (s *Store) AgentCount() (int, error) {
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM agents`).Scan(&n)
	return n, err
}

func splitFields(s string) []string {
	var out []string
	cur := ""
	for _, r := range s {
		if r == ' ' {
			if cur != "" {
				out = append(out, cur)
				cur = ""
			}
			continue
		}
		cur += string(r)
	}
	if cur != "" {
		out = append(out, cur)
	}
	return out
}
