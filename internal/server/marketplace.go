package server

import (
	"encoding/json"
	"net/http"

	"github.com/moltnet/moltnet/core"
	"github.com/moltnet/moltnet/internal/store"
)

// Marketplace HTTP surface (platform v0.2). The board (tasks, applications,
// lifecycle) is CONVENIENCE state — if the registry lies about a status, no
// reputation moves. The only trust-bearing steps are:
//   - create: the task is a poster-signed self.claim offer (id = its hash), so
//     the terms are non-repudiable.
//   - settle: PAID is reachable only once the buyer's signed task.completed AND
//     payment.receipt (posted through the normal signature-authed /v1/attestations)
//     already exist referencing this task. Honest-by-construction.
// Those two attestations are ordinary signed records that feed MoltScore; the
// marketplace neither holds keys nor moves any score itself.

func strField(m map[string]any, k string) string {
	if v, ok := m[k].(string); ok {
		return v
	}
	return ""
}

// POST /v1/tasks — body is a poster-signed self.claim offer attestation.
func (s *Server) handleCreateTask(w http.ResponseWriter, r *http.Request) {
	var offer core.Attestation
	if err := json.NewDecoder(r.Body).Decode(&offer); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid offer json: "+err.Error())
		return
	}
	if offer.Type != core.TypeSelfClaim {
		writeErr(w, http.StatusBadRequest, "a task offer must be a signed self.claim")
		return
	}
	if err := offer.Verify(); err != nil {
		writeErr(w, http.StatusBadRequest, "offer does not verify: "+err.Error())
		return
	}
	// The poster attests about itself — the offer authorizes the task.
	if offer.Issuer != offer.Subject {
		writeErr(w, http.StatusBadRequest, "offer issuer and subject must both be the poster")
		return
	}
	if strField(offer.Body, "kind") != "task.offer" {
		writeErr(w, http.StatusBadRequest, `offer body.kind must be "task.offer"`)
		return
	}
	title := strField(offer.Body, "title")
	if title == "" {
		writeErr(w, http.StatusBadRequest, "offer body.title is required")
		return
	}
	id, err := offer.Hash()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	raw, _ := json.Marshal(offer)
	t := &store.Task{
		ID: id, Poster: offer.Issuer, Title: title,
		Spec:     strField(offer.Body, "spec"),
		Budget:   strField(offer.Body, "budget"),
		Currency: strField(offer.Body, "currency"),
		Rail:     strField(offer.Body, "rail"),
	}
	if err := s.Store.CreateTask(t, string(raw), nowRFC3339()); err != nil {
		writeErr(w, http.StatusInternalServerError, "store: "+err.Error())
		return
	}
	full, _ := s.Store.GetTask(id)
	writeJSON(w, http.StatusCreated, full)
}

// GET /v1/tasks?status=&poster=&assignee=&limit=
func (s *Server) handleListTasks(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit, _ := pageParams(r)
	tasks, err := s.Store.ListTasks(q.Get("status"), q.Get("poster"), q.Get("assignee"), limit)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if tasks == nil {
		tasks = []store.Task{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"tasks": tasks, "count": len(tasks)})
}

// GET /v1/tasks/{id}
func (s *Server) handleGetTask(w http.ResponseWriter, r *http.Request) {
	t, err := s.Store.GetTask(r.PathValue("id"))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if t == nil {
		writeErr(w, http.StatusNotFound, "task not found")
		return
	}
	apps, _ := s.Store.ListApplications(t.ID)
	if apps == nil {
		apps = []store.Application{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"task": t, "applications": apps})
}

// POST /v1/tasks/{id}/apply — agent-API-key auth; applicant = the key's agent.
func (s *Server) handleApplyTask(w http.ResponseWriter, r *http.Request) {
	applicant, _ := s.agentKeyFromRequest(r)
	if applicant == "" {
		writeErr(w, http.StatusUnauthorized, "an agent API key is required to apply")
		return
	}
	t := s.mustOpenTask(w, r)
	if t == nil {
		return
	}
	var body struct{ Bid, Note string }
	_ = json.NewDecoder(r.Body).Decode(&body)
	if err := s.Store.AddApplication(&store.Application{
		TaskID: t.ID, Applicant: applicant, Bid: body.Bid, Note: body.Note,
	}, nowRFC3339()); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"applied": true})
}

// POST /v1/tasks/{id}/assign — poster's owner session; body {assignee}.
func (s *Server) handleAssignTask(w http.ResponseWriter, r *http.Request) {
	t := s.taskOwnedBySession(w, r)
	if t == nil {
		return
	}
	var body struct{ Assignee string }
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Assignee == "" {
		writeErr(w, http.StatusBadRequest, "assignee is required")
		return
	}
	ok, err := s.Store.AssignTask(t.ID, body.Assignee, nowRFC3339())
	s.writeAdvance(w, ok, err, "task is not open")
}

// POST /v1/tasks/{id}/escrow — poster's owner session; body {escrow_ref}.
// The registry records the external reference; it never holds funds.
func (s *Server) handleEscrowTask(w http.ResponseWriter, r *http.Request) {
	t := s.taskOwnedBySession(w, r)
	if t == nil {
		return
	}
	var body struct {
		EscrowRef string `json:"escrow_ref"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.EscrowRef == "" {
		writeErr(w, http.StatusBadRequest, "escrow_ref is required")
		return
	}
	ok, err := s.Store.SetTaskEscrow(t.ID, body.EscrowRef, nowRFC3339())
	s.writeAdvance(w, ok, err, "task is not assigned")
}

// POST /v1/tasks/{id}/deliver — assignee's agent API key; body {artifact_hash, artifact_url}.
func (s *Server) handleDeliverTask(w http.ResponseWriter, r *http.Request) {
	worker, _ := s.agentKeyFromRequest(r)
	if worker == "" {
		writeErr(w, http.StatusUnauthorized, "an agent API key is required to deliver")
		return
	}
	t, err := s.Store.GetTask(r.PathValue("id"))
	if err != nil || t == nil {
		writeErr(w, http.StatusNotFound, "task not found")
		return
	}
	if t.Assignee != worker {
		writeErr(w, http.StatusForbidden, "only the assigned agent may deliver")
		return
	}
	var body struct {
		ArtifactHash string `json:"artifact_hash"`
		ArtifactURL  string `json:"artifact_url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.ArtifactHash == "" {
		writeErr(w, http.StatusBadRequest, "artifact_hash is required")
		return
	}
	ok, err := s.Store.SetTaskDelivery(t.ID, body.ArtifactHash, body.ArtifactURL, nowRFC3339())
	s.writeAdvance(w, ok, err, "task is not in escrow")
}

// POST /v1/tasks/{id}/settle — no auth beyond the signed records: PAID is only
// reachable once the buyer's task.completed + payment.receipt exist on the
// ledger, referencing this task and the assignee. body {completed, receipt}.
func (s *Server) handleSettleTask(w http.ResponseWriter, r *http.Request) {
	t, err := s.Store.GetTask(r.PathValue("id"))
	if err != nil || t == nil {
		writeErr(w, http.StatusNotFound, "task not found")
		return
	}
	var body struct{ Completed, Receipt string }
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Completed == "" || body.Receipt == "" {
		writeErr(w, http.StatusBadRequest, "completed and receipt attestation hashes are required")
		return
	}

	comp, _ := s.Store.GetAttestationByHash(body.Completed)
	rcpt, _ := s.Store.GetAttestationByHash(body.Receipt)
	if comp == nil || rcpt == nil {
		writeErr(w, http.StatusBadRequest, "settlement attestations not found on the ledger")
		return
	}
	// The completion is the counterparty (poster) signing off on the assignee's
	// work; the receipt records payment to the assignee. Both must reference THIS
	// task so a signed record for a different task cannot settle this one.
	if comp.Type != core.TypeTaskCompleted || comp.Subject != t.Assignee ||
		comp.Issuer != t.Poster || strField(comp.Body, "task") != t.ID {
		writeErr(w, http.StatusBadRequest, "task.completed must be issued by the poster for the assignee and reference this task")
		return
	}
	// The receipt must be signed by the PAYER — the poster. Without this the
	// assignee can sign its own receipt (or have a throwaway identity sign it),
	// flip the task to PAID with no money moving, and bank the score: a receipt
	// from a second free keypair is not same-owner, so the independence rule
	// does not discount it either. The receipt is the only signal in MoltScore
	// backed by economic cost; an unbound issuer removes that cost entirely.
	if rcpt.Type != core.TypePaymentReceipt || rcpt.Subject != t.Assignee ||
		rcpt.Issuer != t.Poster || strField(rcpt.Body, "task") != t.ID {
		writeErr(w, http.StatusBadRequest, "payment.receipt must be issued by the poster for the assignee and reference this task")
		return
	}

	ok, err := s.Store.SettleTask(t.ID, body.Completed, body.Receipt, nowRFC3339())
	s.writeAdvance(w, ok, err, "task is not delivered (must be in 'done' to settle)")
}

// ---- helpers ----

// mustOpenTask loads the path task and requires it be OPEN, or writes an error
// and returns nil.
func (s *Server) mustOpenTask(w http.ResponseWriter, r *http.Request) *store.Task {
	t, err := s.Store.GetTask(r.PathValue("id"))
	if err != nil || t == nil {
		writeErr(w, http.StatusNotFound, "task not found")
		return nil
	}
	if t.Status != store.TaskOpen {
		writeErr(w, http.StatusConflict, "task is no longer open")
		return nil
	}
	return t
}

// taskOwnedBySession loads the path task and requires the session owner to own
// the poster agent (the authorization for poster-only actions).
func (s *Server) taskOwnedBySession(w http.ResponseWriter, r *http.Request) *store.Task {
	owner := ownerFromContext(r)
	if owner == "" {
		writeErr(w, http.StatusUnauthorized, "sign in required")
		return nil
	}
	t, err := s.Store.GetTask(r.PathValue("id"))
	if err != nil || t == nil {
		writeErr(w, http.StatusNotFound, "task not found")
		return nil
	}
	card, _ := s.Store.GetCard(t.Poster)
	if card == nil || card.Owner != owner {
		writeErr(w, http.StatusForbidden, "you do not own the task poster")
		return nil
	}
	return t
}

// writeAdvance renders the result of a lifecycle transition: a status mismatch
// (ok==false) is a 409, a store error is a 500, success is 200 with the task.
func (s *Server) writeAdvance(w http.ResponseWriter, ok bool, err error, conflictMsg string) {
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !ok {
		writeErr(w, http.StatusConflict, conflictMsg)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
