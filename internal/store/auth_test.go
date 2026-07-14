package store

import (
	"testing"
	"time"
)

func count(t *testing.T, s *Store, table string) int {
	t.Helper()
	var n int
	if err := s.DB().QueryRow(`SELECT COUNT(*) FROM ` + table).Scan(&n); err != nil {
		t.Fatalf("count %s: %v", table, err)
	}
	return n
}

// The auth reaper is load-bearing: POST /v1/auth/challenge is unauthenticated
// and writes a row per call, so without a purge anyone can grow the DB without
// bound. This pins that spent rows are actually deleted — and that live ones
// survive.
func TestPurgeChallengesAndSessions(t *testing.T) {
	s, err := Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	now := time.Now().UTC()
	past := now.Add(-time.Hour).Format(time.RFC3339)
	future := now.Add(time.Hour).Format(time.RFC3339)
	nowStr := now.Format(time.RFC3339)

	// expired, used-but-live, and still-valid challenges
	if err := s.CreateChallenge("expired", past, past); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateChallenge("used", nowStr, future); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateChallenge("live", nowStr, future); err != nil {
		t.Fatal(err)
	}
	if ok, err := s.ConsumeChallenge("used", nowStr); err != nil || !ok {
		t.Fatalf("consume used: ok=%v err=%v", ok, err)
	}

	// an expired session and a live one
	if err := s.CreateSession("hash-old", "did:key:zOwner", past, past); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateSession("hash-new", "did:key:zOwner", nowStr, future); err != nil {
		t.Fatal(err)
	}

	if got := count(t, s, "challenges"); got != 3 {
		t.Fatalf("challenges before purge = %d, want 3", got)
	}

	if err := s.PurgeChallenges(nowStr); err != nil {
		t.Fatal(err)
	}
	if err := s.PurgeExpiredSessions(nowStr); err != nil {
		t.Fatal(err)
	}

	// expired + used are gone; the unspent, unexpired one survives.
	if got := count(t, s, "challenges"); got != 1 {
		t.Fatalf("challenges after purge = %d, want 1 (only the live one)", got)
	}
	if ok, err := s.ConsumeChallenge("live", nowStr); err != nil || !ok {
		t.Fatalf("live challenge should still be consumable: ok=%v err=%v", ok, err)
	}

	// the expired session is gone; the live one still authenticates.
	if got := count(t, s, "sessions"); got != 1 {
		t.Fatalf("sessions after purge = %d, want 1", got)
	}
	sess, err := s.GetSession("hash-new", nowStr)
	if err != nil || sess == nil {
		t.Fatalf("live session should survive purge: %v", err)
	}
}

// A revoked key must be addressed by its unique id, not the 4-char display
// prefix: two of an owner's keys can share a prefix, and revoking the wrong
// credential is a security failure. Owner scoping is the authorization check.
func TestRevokeAPIKeyByIDIsOwnerScoped(t *testing.T) {
	s, err := Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	now := time.Now().UTC().Format(time.RFC3339)
	const owner, other = "did:key:zOwner", "did:key:zOther"

	// two keys for the same owner that deliberately COLLIDE on prefix+last4 —
	// only the id distinguishes them.
	if err := s.CreateAPIKey("id-aaa", "hash-a", "did:key:zAgent", owner, "a", "molt_sk_live_ab…", "wxyz", now); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateAPIKey("id-bbb", "hash-b", "did:key:zAgent", owner, "b", "molt_sk_live_ab…", "wxyz", now); err != nil {
		t.Fatal(err)
	}

	// another owner cannot revoke it even knowing the id
	if ok, err := s.RevokeAPIKey("id-aaa", other, now); err != nil || ok {
		t.Fatalf("cross-owner revoke must fail: ok=%v err=%v", ok, err)
	}
	if k, _ := s.GetAPIKey("hash-a"); k == nil {
		t.Fatal("key A must survive a cross-owner revoke attempt")
	}

	// the owner revokes exactly the key they named — and only that one
	if ok, err := s.RevokeAPIKey("id-aaa", owner, now); err != nil || !ok {
		t.Fatalf("owner revoke: ok=%v err=%v", ok, err)
	}
	if k, _ := s.GetAPIKey("hash-a"); k != nil {
		t.Fatal("key A should be revoked")
	}
	if k, _ := s.GetAPIKey("hash-b"); k == nil {
		t.Fatal("key B shares A's prefix and last4 — it must NOT have been revoked")
	}

	// revoking twice is not a success
	if ok, _ := s.RevokeAPIKey("id-aaa", owner, now); ok {
		t.Fatal("re-revoking an already-revoked key should report false")
	}
}
