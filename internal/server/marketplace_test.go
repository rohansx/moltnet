package server

import (
	"testing"

	"github.com/moltnet/moltnet/core"
)

// signedOffer builds a poster-signed self.claim task offer.
func signedOffer(t *testing.T, poster *core.KeyPair, title, budget string) *core.Attestation {
	t.Helper()
	o := core.NewAttestation(core.TypeSelfClaim, poster.DID, poster.DID)
	o.Body = map[string]any{"kind": "task.offer", "title": title, "budget": budget, "currency": "USDC", "rail": "x402"}
	if err := o.Sign(poster.Private); err != nil {
		t.Fatal(err)
	}
	return o
}

// TestMarketplaceHappyPath drives a task from a signed offer all the way to PAID
// and asserts the worker's MoltScore reflects the completion — proving the board
// is convenience but settlement produces a real, scored, signed attestation.
func TestMarketplaceHappyPath(t *testing.T) {
	ts, cleanup := testEnv(t)
	defer cleanup()

	posterOwner, _ := core.GenerateKeyPair()
	poster, _ := core.GenerateKeyPair()
	workerOwner, _ := core.GenerateKeyPair()
	worker, _ := core.GenerateKeyPair()

	// Both agents registered under DISTINCT owners (so the completion is not
	// self-dealing and actually counts — see the Phase 0 independence rule).
	for _, c := range []*core.Card{
		mustCard(t, posterOwner, poster, "poster"),
		mustCard(t, workerOwner, worker, "worker", "code.review"),
	} {
		if code, body := postJSON(t, ts.URL+"/v1/agents", c); code != 201 {
			t.Fatalf("register: %d %s", code, body)
		}
	}

	// 1. Poster posts a signed offer → OPEN task, id = offer hash.
	var task struct {
		ID     string `json:"id"`
		Status string `json:"status"`
	}
	if code, body := postJSON(t, ts.URL+"/v1/tasks", signedOffer(t, poster, "refactor auth", "300")); code != 201 {
		t.Fatalf("create task: %d %s", code, body)
	} else {
		decode(t, body, &task)
	}
	if task.Status != "open" || task.ID == "" {
		t.Fatalf("new task should be open with an id: %+v", task)
	}

	// 2. Worker's owner signs in and mints an API key for the worker agent.
	workerToken := loginOwner(t, ts.URL, workerOwner)
	var mint struct {
		Key string `json:"key"`
	}
	if code, body := postJSONAuth(t, ts.URL+"/v1/me/apikeys", workerToken, map[string]string{"agent_did": worker.DID, "name": "work"}); code != 201 {
		t.Fatalf("mint worker key: %d %s", code, body)
	} else {
		decode(t, body, &mint)
	}

	// 3. Worker applies with its API key.
	if code, body := postJSONAuth(t, ts.URL+"/v1/tasks/"+task.ID+"/apply", mint.Key, map[string]string{"bid": "300"}); code != 200 {
		t.Fatalf("apply: %d %s", code, body)
	}

	// 4. Poster's owner signs in, assigns the worker, and funds escrow.
	posterToken := loginOwner(t, ts.URL, posterOwner)
	if code, body := postJSONAuth(t, ts.URL+"/v1/tasks/"+task.ID+"/assign", posterToken, map[string]string{"assignee": worker.DID}); code != 200 {
		t.Fatalf("assign: %d %s", code, body)
	}
	if code, body := postJSONAuth(t, ts.URL+"/v1/tasks/"+task.ID+"/escrow", posterToken, map[string]string{"escrow_ref": "0xchain-tx-abc"}); code != 200 {
		t.Fatalf("escrow: %d %s", code, body)
	}

	// 5. Worker delivers.
	if code, body := postJSONAuth(t, ts.URL+"/v1/tasks/"+task.ID+"/deliver", mint.Key, map[string]string{"artifact_hash": "blake3:deadbeef"}); code != 200 {
		t.Fatalf("deliver: %d %s", code, body)
	}

	// 6. Poster signs the two settlement records and posts them to the normal
	//    signature-authed ledger. They chain: receipt.prev = completed.hash.
	completed := core.NewAttestation(core.TypeTaskCompleted, poster.DID, worker.DID)
	completed.Body = map[string]any{"task": task.ID, "outcome": "success", "artifact_hash": "blake3:deadbeef"}
	if err := completed.Sign(poster.Private); err != nil {
		t.Fatal(err)
	}
	compHash, _ := completed.Hash()
	if code, body := postJSON(t, ts.URL+"/v1/attestations", completed); code != 201 {
		t.Fatalf("post task.completed: %d %s", code, body)
	}
	receipt := core.NewAttestation(core.TypePaymentReceipt, poster.DID, worker.DID)
	receipt.Prev = compHash // chain onto the poster's head
	receipt.Body = map[string]any{"task": task.ID, "amount": "300", "currency": "USDC", "rail": "x402", "tx": "0xchain-tx-abc"}
	if err := receipt.Sign(poster.Private); err != nil {
		t.Fatal(err)
	}
	rcptHash, _ := receipt.Hash()
	if code, body := postJSON(t, ts.URL+"/v1/attestations", receipt); code != 201 {
		t.Fatalf("post payment.receipt: %d %s", code, body)
	}

	// 7. Settle: honest-by-construction — PAID only because the signed records exist.
	if code, body := postJSON(t, ts.URL+"/v1/tasks/"+task.ID+"/settle", map[string]string{"completed": compHash, "receipt": rcptHash}); code != 200 {
		t.Fatalf("settle: %d %s", code, body)
	}
	var got struct {
		Task struct {
			Status       string `json:"status"`
			CompletedAtt string `json:"completed_att"`
		} `json:"task"`
	}
	if code := getJSON(t, ts.URL+"/v1/tasks/"+task.ID, &got); code != 200 {
		t.Fatalf("get task: %d", code)
	}
	if got.Task.Status != "paid" || got.Task.CompletedAtt != compHash {
		t.Fatalf("task should be paid and linked: %+v", got.Task)
	}

	// 8. The worker's MoltScore now reflects a real completion from the marketplace.
	var sc struct {
		Inputs struct {
			Completions int `json:"completions"`
		} `json:"inputs"`
	}
	getJSON(t, ts.URL+"/v1/score/"+worker.DID, &sc)
	if sc.Inputs.Completions != 1 {
		t.Fatalf("settled task must produce one scored completion, got %d", sc.Inputs.Completions)
	}
}

// TestSettleRequiresSignedRecords proves the board cannot fake a payout: settle
// is rejected until the real signed attestations exist and reference the task.
func TestSettleRequiresSignedRecords(t *testing.T) {
	ts, cleanup := testEnv(t)
	defer cleanup()

	posterOwner, _ := core.GenerateKeyPair()
	poster, _ := core.GenerateKeyPair()
	if code, _ := postJSON(t, ts.URL+"/v1/agents", mustCard(t, posterOwner, poster, "poster")); code != 201 {
		t.Fatalf("register poster")
	}

	var task struct {
		ID string `json:"id"`
	}
	_, body := postJSON(t, ts.URL+"/v1/tasks", signedOffer(t, poster, "task", "10"))
	decode(t, body, &task)

	// No settlement attestations exist → settle must not flip to PAID.
	if code, _ := postJSON(t, ts.URL+"/v1/tasks/"+task.ID+"/settle",
		map[string]string{"completed": "blake3:nope", "receipt": "blake3:nope"}); code != 400 {
		t.Fatalf("settle without signed records must be rejected, got %d", code)
	}
	// And even a real, valid attestation that references a DIFFERENT task cannot
	// settle this one (task-binding check).
	other := core.NewAttestation(core.TypeTaskCompleted, poster.DID, poster.DID)
	other.Body = map[string]any{"task": "blake3:some-other-task"}
	_ = other.Sign(poster.Private)
	oh, _ := other.Hash()
	postJSON(t, ts.URL+"/v1/attestations", other)
	if code, _ := postJSON(t, ts.URL+"/v1/tasks/"+task.ID+"/settle",
		map[string]string{"completed": oh, "receipt": oh}); code != 400 {
		t.Fatalf("settle with a mismatched-task record must be rejected, got %d", code)
	}
}
