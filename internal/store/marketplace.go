package store

import (
	"database/sql"

	"github.com/moltnet/moltnet/core"
)

// Marketplace persistence (platform v0.2). This is the convenience layer: task
// rows and applications are mutable, hosted, and NOT trusted — reputation moves
// only through the signed task.completed / payment.receipt attestations on the
// normal ledger. The one anchor to the signed layer is tasks.id, which is the
// hash of the poster's signed offer (offer_json).

// Task lifecycle statuses.
const (
	TaskOpen     = "open"
	TaskAssigned = "assigned"
	TaskEscrow   = "escrow"
	TaskDone     = "done"
	TaskPaid     = "paid"
	TaskDisputed = "disputed"
)

// Task is a marketplace task row.
type Task struct {
	ID           string `json:"id"`
	Poster       string `json:"poster_did"`
	Title        string `json:"title"`
	Spec         string `json:"spec,omitempty"`
	Budget       string `json:"budget,omitempty"`
	Currency     string `json:"currency,omitempty"`
	Rail         string `json:"rail,omitempty"`
	Status       string `json:"status"`
	Assignee     string `json:"assignee_did,omitempty"`
	EscrowRef    string `json:"escrow_ref,omitempty"`
	ArtifactHash string `json:"artifact_hash,omitempty"`
	ArtifactURL  string `json:"artifact_url,omitempty"`
	CompletedAtt string `json:"completed_att,omitempty"`
	ReceiptAtt   string `json:"receipt_att,omitempty"`
	CreatedAt    string `json:"created_at"`
	UpdatedAt    string `json:"updated_at"`
}

// Application is an agent's bid on a task.
type Application struct {
	TaskID    string `json:"task_id"`
	Applicant string `json:"applicant_did"`
	Bid       string `json:"bid,omitempty"`
	Note      string `json:"note,omitempty"`
	CreatedAt string `json:"created_at"`
}

// CreateTask inserts a new OPEN task, keyed by the hash of its signed offer.
// offerJSON is the poster-signed self.claim, stored so the terms stay verifiable.
func (s *Store) CreateTask(t *Task, offerJSON, at string) error {
	_, err := s.db.Exec(
		`INSERT INTO tasks (id, poster_did, title, spec, budget, currency, rail, status, offer_json, created_at, updated_at)
         VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		t.ID, t.Poster, t.Title, t.Spec, t.Budget, t.Currency, t.Rail, TaskOpen, offerJSON, at, at)
	return err
}

func scanTask(row interface{ Scan(...any) error }) (*Task, error) {
	var t Task
	var spec, budget, currency, rail, assignee, escrow, ahash, aurl, comp, rcpt sql.NullString
	if err := row.Scan(&t.ID, &t.Poster, &t.Title, &spec, &budget, &currency, &rail,
		&t.Status, &assignee, &escrow, &ahash, &aurl, &comp, &rcpt, &t.CreatedAt, &t.UpdatedAt); err != nil {
		return nil, err
	}
	t.Spec, t.Budget, t.Currency, t.Rail = spec.String, budget.String, currency.String, rail.String
	t.Assignee, t.EscrowRef, t.ArtifactHash, t.ArtifactURL = assignee.String, escrow.String, ahash.String, aurl.String
	t.CompletedAtt, t.ReceiptAtt = comp.String, rcpt.String
	return &t, nil
}

const taskCols = `id, poster_did, title, spec, budget, currency, rail, status,
    assignee_did, escrow_ref, artifact_hash, artifact_url, completed_att, receipt_att, created_at, updated_at`

// GetTask returns a task, or (nil, nil) if absent.
func (s *Store) GetTask(id string) (*Task, error) {
	t, err := scanTask(s.db.QueryRow(`SELECT `+taskCols+` FROM tasks WHERE id = ?`, id))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return t, err
}

// ListTasks returns tasks filtered by status (empty = any) and/or poster/assignee
// (empty = any), newest first.
func (s *Store) ListTasks(status, poster, assignee string, limit int) ([]Task, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := s.db.Query(
		`SELECT `+taskCols+` FROM tasks
         WHERE (? = '' OR status = ?)
           AND (? = '' OR poster_did = ?)
           AND (? = '' OR assignee_did = ?)
         ORDER BY updated_at DESC LIMIT ?`,
		status, status, poster, poster, assignee, assignee, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Task
	for rows.Next() {
		t, err := scanTask(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *t)
	}
	return out, rows.Err()
}

// AddApplication records an agent's bid (idempotent on re-apply).
func (s *Store) AddApplication(a *Application, at string) error {
	_, err := s.db.Exec(
		`INSERT INTO task_applications (task_id, applicant_did, bid, note, created_at)
         VALUES (?, ?, ?, ?, ?)
         ON CONFLICT(task_id, applicant_did) DO UPDATE SET bid=excluded.bid, note=excluded.note`,
		a.TaskID, a.Applicant, a.Bid, a.Note, at)
	return err
}

// ListApplications returns all bids on a task.
func (s *Store) ListApplications(taskID string) ([]Application, error) {
	rows, err := s.db.Query(
		`SELECT task_id, applicant_did, COALESCE(bid,''), COALESCE(note,''), created_at
         FROM task_applications WHERE task_id = ? ORDER BY created_at ASC`, taskID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Application
	for rows.Next() {
		var a Application
		if err := rows.Scan(&a.TaskID, &a.Applicant, &a.Bid, &a.Note, &a.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// AssignTask sets the assignee and moves OPEN -> ASSIGNED. Returns (false, nil)
// if the task is not currently open.
func (s *Store) AssignTask(id, assignee, at string) (bool, error) {
	return s.advance(id, TaskOpen, TaskAssigned,
		`assignee_did = ?`, assignee, at)
}

// SetTaskEscrow records the (external) escrow reference and moves to ESCROW. The
// registry only witnesses the ref; it never custodies funds.
func (s *Store) SetTaskEscrow(id, ref, at string) (bool, error) {
	return s.advance(id, TaskAssigned, TaskEscrow, `escrow_ref = ?`, ref, at)
}

// SetTaskDelivery pins the worker's artifact hash and moves ESCROW -> DONE.
func (s *Store) SetTaskDelivery(id, artifactHash, artifactURL, at string) (bool, error) {
	res, err := s.db.Exec(
		`UPDATE tasks SET status = ?, artifact_hash = ?, artifact_url = ?, updated_at = ?
         WHERE id = ? AND status = ?`,
		TaskDone, artifactHash, artifactURL, at, id, TaskEscrow)
	return affected(res, err)
}

// SettleTask records the two settling attestation hashes and moves DONE -> PAID.
// The caller must have verified the signed records first; this only flips state.
func (s *Store) SettleTask(id, completedAtt, receiptAtt, at string) (bool, error) {
	res, err := s.db.Exec(
		`UPDATE tasks SET status = ?, completed_att = ?, receipt_att = ?, updated_at = ?
         WHERE id = ? AND status = ?`,
		TaskPaid, completedAtt, receiptAtt, at, id, TaskDone)
	return affected(res, err)
}

// advance flips a task from one status to the next, setting one extra column,
// only if it is currently in `from`. Returns (false, nil) on a status mismatch.
func (s *Store) advance(id, from, to, setCol, setVal, at string) (bool, error) {
	res, err := s.db.Exec(
		`UPDATE tasks SET status = ?, `+setCol+`, updated_at = ? WHERE id = ? AND status = ?`,
		to, setVal, at, id, from)
	return affected(res, err)
}

func affected(res sql.Result, err error) (bool, error) {
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// GetAttestationByHash returns a stored attestation by its content hash, or
// (nil, nil). The marketplace settle flow uses it to confirm the signed
// task.completed / payment.receipt exist before flipping a task to PAID.
func (s *Store) GetAttestationByHash(hash string) (*core.Attestation, error) {
	atts, err := s.queryAttestations(`SELECT raw_json FROM attestations WHERE hash = ?`, hash)
	if err != nil {
		return nil, err
	}
	if len(atts) == 0 {
		return nil, nil
	}
	return atts[0], nil
}
