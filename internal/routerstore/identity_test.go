package routerstore

// ADR-062 §3 identity chain: each rejection has a positive control beside it
// (A35 — a guard never shown green AND red is untested).

import (
	"bytes"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"
)

type identityHarness struct {
	backend *SQLiteStore
	srv     *httptest.Server
	now     time.Time
}

func newIdentityHarness(t *testing.T) *identityHarness {
	t.Helper()
	h := &identityHarness{now: time.Date(2026, 9, 2, 22, 0, 0, 0, time.UTC)}
	h.backend = openBackendStore(t, filepath.Join(t.TempDir(), "router.db"))
	t.Cleanup(func() { _ = h.backend.Close() })
	h.backend.notifyDir = t.TempDir()
	handler, err := Handler(h.backend, ServerOptions{Token: "host-token", now: func() time.Time { return h.now }})
	if err != nil {
		t.Fatal(err)
	}
	h.srv = httptest.NewServer(handler)
	t.Cleanup(h.srv.Close)
	return h
}

// client returns a RemoteStore with no on-disk cache and a fixed clock.
func (h *identityHarness) client(agent string) *RemoteStore {
	rs := NewRemoteStore(h.srv.URL, "host-token")
	rs.sessionDir = ""
	rs.agent = agent
	rs.now = func() time.Time { return h.now }
	return rs
}

func TestIdentitySessionIsMintedAndSignedCallsSucceed(t *testing.T) {
	h := newIdentityHarness(t)
	rs := h.client("claude-a")
	if _, err := rs.Inbox("claude-a"); err != nil {
		t.Fatalf("positive control: signed call after mint should succeed: %v", err)
	}
	if rs.session.ID == "" || rs.session.Secret == "" {
		t.Fatal("client did not obtain a session")
	}
	got, err := h.backend.GetSession(rs.session.ID)
	if err != nil || got.RuntimeHash != rs.runtime || got.Agent != "claude-a" {
		t.Fatalf("server-side session mismatch: %+v err=%v", got, err)
	}
}

func TestIdentityWrongRuntimeIsRejectedAndSessionRevoked(t *testing.T) {
	h := newIdentityHarness(t)
	rs := h.client("claude-a")
	if _, err := rs.Inbox("claude-a"); err != nil {
		t.Fatal(err)
	}
	sid := rs.session.ID
	// A different binary presents the same session: refused, and the session
	// is dead from then on (a mismatch is not a retry).
	rs.runtime = "deadbeef"
	if _, err := rs.Inbox("claude-a"); err == nil {
		t.Fatal("wrong runtime hash must be refused")
	}
	if _, err := h.backend.GetSession(sid); !errors.Is(err, ErrSessionRevoked) {
		t.Fatalf("session should be revoked after a runtime mismatch, got %v", err)
	}
}

func TestIdentityStaleNonceIsRejected(t *testing.T) {
	h := newIdentityHarness(t)
	rs := h.client("claude-a")
	if _, err := rs.Inbox("claude-a"); err != nil {
		t.Fatal("positive control:", err)
	}
	// Client clock 61 s behind the server: outside the ±60 s window.
	rs.now = func() time.Time { return h.now.Add(-61 * time.Second) }
	if _, err := rs.Inbox("claude-a"); err == nil {
		t.Fatal("nonce older than the window must be refused")
	}
	rs.now = func() time.Time { return h.now.Add(-30 * time.Second) }
	if _, err := rs.Inbox("claude-a"); err != nil {
		t.Fatalf("nonce inside the window must pass: %v", err)
	}
}

func TestIdentityReplayedNonceIsRejected(t *testing.T) {
	h := newIdentityHarness(t)
	rs := h.client("claude-a")
	if _, err := rs.Inbox("claude-a"); err != nil {
		t.Fatal(err)
	}
	// Force the same nonce twice.
	fixed := "1788386400000.aaaaaaaaaaaaaaaa"
	rs.now = func() time.Time { return h.now }
	// Drive two raw requests with an identical nonce through the signed path.
	body := []byte(`{"args":["claude-a"]}`)
	first := signedPost(t, h.srv.URL, "host-token", rs.session, rs.runtime, "Inbox", fixed, body)
	second := signedPost(t, h.srv.URL, "host-token", rs.session, rs.runtime, "Inbox", fixed, body)
	if first != 200 {
		t.Fatalf("positive control: first use of a nonce should be 200, got %d", first)
	}
	if second != 401 {
		t.Fatalf("replayed nonce should be 401, got %d", second)
	}
}

func TestIdentityBadSignatureIsRejected(t *testing.T) {
	h := newIdentityHarness(t)
	rs := h.client("claude-a")
	if _, err := rs.Inbox("claude-a"); err != nil {
		t.Fatal(err)
	}
	forged := rs.session
	forged.Secret = "not-the-secret"
	body := []byte(`{"args":["claude-a"]}`)
	if code := signedPost(t, h.srv.URL, "host-token", forged, rs.runtime, "Inbox", "1788386400000.bbbbbbbbbbbbbbbb", body); code != 401 {
		t.Fatalf("wrong secret must be 401, got %d", code)
	}
}

func TestIdentityLeaseOwnershipIsPerSession(t *testing.T) {
	h := newIdentityHarness(t)
	a := h.client("claude-a")
	b := h.client("claude-b-impostor")

	id, _, err := a.SendGuarded(SendReq{From: "x", To: "claude-a", Title: "t", Type: "proposal", Instructions: "i"})
	if err != nil {
		t.Fatal(err)
	}
	lease, err := a.ClaimNext("claude-a", time.Minute)
	if err != nil || lease == nil {
		t.Fatalf("claim: %v", err)
	}
	// Same lease token, different session: the token alone is not ownership.
	if err := b.Complete(id, lease.Token, "stolen"); !errors.Is(err, ErrNotOwner) {
		t.Fatalf("another session completing my lease: want ErrNotOwner, got %v", err)
	}
	// Positive control: the claiming session completes it.
	if err := a.Complete(id, lease.Token, "mine"); err != nil {
		t.Fatalf("owner completing its own lease: %v", err)
	}
}

// TestIdentityStaleReclaimPreservesNewerResult is the authenticated-session
// counterpart to TestIdentityLeaseOwnershipIsPerSession: that test proves a
// DIFFERENT session's stolen-but-still-live token is refused; this one
// proves that once session A's lease actually EXPIRES and session B reclaims
// it through the authenticated ClaimNext path, A's now-stale token is
// refused and B's completion is the one durably preserved — the full
// RemoteStore -> Handler -> checkItemOwner chain, not direct store access
// (codex-pantheon's PR944 retained-gap request, item 20261001-140902/145640:
// authenticated two-instance coverage on both backends, not just OpenPath).
// Runs on whichever backend openBackendStore resolved to (SQLite by default,
// Postgres when SIRSI_TEST_PG_DSN is set).
func TestIdentityStaleReclaimPreservesNewerResult(t *testing.T) {
	h := newIdentityHarness(t)
	a := h.client("claude-a")
	b := h.client("claude-b")

	id, _, err := a.SendGuarded(SendReq{From: "x", To: "claude-a", Title: "t", Type: "proposal", Instructions: "i"})
	if err != nil {
		t.Fatal(err)
	}
	leaseA, err := a.ClaimNext("claude-a", time.Nanosecond)
	if err != nil || leaseA == nil {
		t.Fatalf("instance A claim: %v, %v", leaseA, err)
	}
	time.Sleep(time.Millisecond) // past A's lease expiry
	leaseB, err := b.ClaimNext("claude-a", time.Minute)
	if err != nil || leaseB == nil {
		t.Fatalf("instance B reclaim: %v, %v", leaseB, err)
	}
	if leaseB.Token == leaseA.Token {
		t.Fatal("instance B must mint a fresh token on reclaim, not reuse A's stale token")
	}
	// A, unaware of the reclaim, completes with its stale token through its
	// OWN authenticated session — refused by the same ownership fence proven
	// above, now exercised against an expired-then-reclaimed lease rather
	// than a merely-stolen live one.
	if staleErr := a.Complete(id, leaseA.Token, "stale result from A"); !errors.Is(staleErr, ErrNotOwner) && !errors.Is(staleErr, ErrLeaseInvalid) {
		t.Fatalf("A's stale authenticated Complete = %v, want ErrNotOwner or ErrLeaseInvalid", staleErr)
	}
	if completeErr := b.Complete(id, leaseB.Token, "result from B"); completeErr != nil {
		t.Fatalf("B's authenticated Complete with the live reclaimed token: %v", completeErr)
	}
	// Read back through a THIRD independent authenticated session to prove
	// the preserved result is durable, not local to B's connection.
	c := h.client("claude-c-reader")
	got, err := c.Get(id)
	if err != nil {
		t.Fatalf("instance C authenticated read: %v", err)
	}
	if got.Result != "result from B" {
		t.Fatalf("preserved result = %q, want B's result", got.Result)
	}
	if got.Status != "completed" {
		t.Fatalf("status = %q, want completed", got.Status)
	}
}

// TestIdentityItemOwnershipSurvivesSessionRemintSameAgentThread reproduces
// and fixes the deadlock reported in router item 20261001-144907: the
// on-disk session cache (~/.sirsi/sessions/<agent>.json) is keyed on
// (agent, runtime_hash, thread_id) and drops + re-mints whenever any of
// those changes (e.g. the sirsi binary gets rebuilt between claim and
// complete). Before this fix, ownership was bound to the raw session id, so
// a remint orphaned the lease: the SAME logical worker's future Complete
// calls failed with ErrNotOwner forever, with no CLI escape hatch. The fix
// (sameWorkerAcrossRemint in serve.go) treats two DIFFERENT session ids as
// the same owner when they share a non-empty thread_id and agent.
func TestIdentityItemOwnershipSurvivesSessionRemintSameAgentThread(t *testing.T) {
	h := newIdentityHarness(t)
	const thread = "thr-remint-fixed"

	first := h.client("claude-remint")
	first.threadID = thread
	id, _, err := first.SendGuarded(SendReq{From: "x", To: "claude-remint", Title: "t", Type: "proposal", Instructions: "i"})
	if err != nil {
		t.Fatal(err)
	}
	lease, err := first.ClaimNext("claude-remint", time.Minute)
	if err != nil || lease == nil {
		t.Fatalf("claim: %v, %v", lease, err)
	}

	// Simulate the remint: a brand new RemoteStore (fresh session cache) for
	// the SAME agent and SAME registered thread — exactly what a binary
	// rebuild produces, not a different logical worker.
	second := h.client("claude-remint")
	second.threadID = thread

	// Before the fix: ErrNotOwner, forever.
	if err := second.Complete(id, lease.Token, "completed after remint"); err != nil {
		t.Fatalf("Complete from the reminted session = %v, want success", err)
	}
}

// TestIdentityTaskOwnershipSurvivesSessionRemintSameAgentThread is the
// task-ledger twin — the exact shape claude-home reported (task complete /
// release deadlocking on a session remint), proven through the real
// ClaimNextTask -> CompleteTaskLease authenticated path.
func TestIdentityTaskOwnershipSurvivesSessionRemintSameAgentThread(t *testing.T) {
	h := newIdentityHarness(t)
	const thread = "thr-remint-fixed-task"

	first := h.client("claude-remint-task")
	first.threadID = thread
	if err := first.AddTask(Task{Agent: "claude-remint-task", TaskID: "remint-task", Subject: "s"}); err != nil {
		t.Fatalf("AddTask: %v", err)
	}
	lease, err := first.ClaimNextTask("claude-remint-task", "worker", thread, time.Minute)
	if err != nil || lease == nil {
		t.Fatalf("claim: %v, %v", lease, err)
	}

	second := h.client("claude-remint-task")
	second.threadID = thread

	// Before the fix: ErrNotOwner, forever — the exact deadlock reported.
	if err := second.CompleteTaskLease("claude-remint-task", "remint-task", lease.Token, "done"); err != nil {
		t.Fatalf("CompleteTaskLease from the reminted session = %v, want success", err)
	}
}

// TestIdentityOwnershipRemintFallbackRequiresSameAgent is the negative
// control: a shared thread_id alone must never bridge two DIFFERENT agents.
// Equivalence requires agent AND thread_id to both match — otherwise the
// remint fallback would become a cross-agent lease-theft vector.
func TestIdentityOwnershipRemintFallbackRequiresSameAgent(t *testing.T) {
	h := newIdentityHarness(t)
	const thread = "thr-shared-by-mistake"

	owner := h.client("claude-owner")
	owner.threadID = thread
	id, _, err := owner.SendGuarded(SendReq{From: "x", To: "claude-owner", Title: "t", Type: "proposal", Instructions: "i"})
	if err != nil {
		t.Fatal(err)
	}
	lease, err := owner.ClaimNext("claude-owner", time.Minute)
	if err != nil || lease == nil {
		t.Fatalf("claim: %v, %v", lease, err)
	}

	impostor := h.client("claude-different-agent")
	impostor.threadID = thread // same thread id, DIFFERENT agent
	if err := impostor.Complete(id, lease.Token, "stolen via shared thread id"); !errors.Is(err, ErrNotOwner) {
		t.Fatalf("cross-agent Complete sharing a thread id = %v, want ErrNotOwner", err)
	}
	// Positive control: the real owner still completes fine.
	if err := owner.Complete(id, lease.Token, "mine"); err != nil {
		t.Fatalf("owner completing its own lease: %v", err)
	}
}

// TestIdentityOwnershipRemintFallbackRequiresSameHostItem is the fix for
// codex-pantheon's CHANGES REQUIRED on the first version of this fallback
// (router item 20261001-154515, PR947): agent+thread_id alone is not
// sufficient, because MintSessionForThread accepts a caller-supplied
// agent/thread with no registration check of its own (the Rule of Ra gate
// defaults to "log", not "enforce"). A second, genuinely different host can
// mint its OWN correctly-authenticated session claiming the SAME agent and
// thread_id strings as the real owner, then — without this host check —
// complete the owner's lease with a copied token. Mirrors codex's
// reproduction shape (two distinct hosts, real Handler, separate signed
// sessions, same agent/thread strings) but through the package's existing
// RemoteStore/httptest harness rather than a custom RoundTripper.
func TestIdentityOwnershipRemintFallbackRequiresSameHostItem(t *testing.T) {
	h := newIdentityHarness(t)
	const thread = "thr-cross-host"

	owner := h.client("claude-cross-host")
	owner.host = "host-a"
	owner.threadID = thread
	id, _, err := owner.SendGuarded(SendReq{From: "x", To: "claude-cross-host", Title: "t", Type: "proposal", Instructions: "i"})
	if err != nil {
		t.Fatal(err)
	}
	lease, err := owner.ClaimNext("claude-cross-host", time.Minute)
	if err != nil || lease == nil {
		t.Fatalf("claim: %v, %v", lease, err)
	}

	// A second, DIFFERENT host, same agent and thread_id strings, its own
	// independently minted and signed session — the exact cross-host
	// reproduction, not a copied credential.
	impostor := h.client("claude-cross-host")
	impostor.host = "host-b"
	impostor.threadID = thread
	if err := impostor.Complete(id, lease.Token, "stolen via cross-host remint"); !errors.Is(err, ErrNotOwner) {
		t.Fatalf("cross-host Complete with matching agent+thread = %v, want ErrNotOwner", err)
	}
	// Positive control: the real owner, same host, still completes fine.
	if err := owner.Complete(id, lease.Token, "mine"); err != nil {
		t.Fatalf("owner completing its own lease: %v", err)
	}
}

// TestIdentityOwnershipRemintFallbackRequiresSameHostTask is the task-ledger
// twin of the cross-host item test — the same vector against
// checkTaskOwner/ClaimNextTask/CompleteTaskLease.
func TestIdentityOwnershipRemintFallbackRequiresSameHostTask(t *testing.T) {
	h := newIdentityHarness(t)
	const thread = "thr-cross-host-task"

	owner := h.client("claude-cross-host-task")
	owner.host = "host-a"
	owner.threadID = thread
	if err := owner.AddTask(Task{Agent: "claude-cross-host-task", TaskID: "cross-host-task", Subject: "s"}); err != nil {
		t.Fatalf("AddTask: %v", err)
	}
	lease, err := owner.ClaimNextTask("claude-cross-host-task", "worker", thread, time.Minute)
	if err != nil || lease == nil {
		t.Fatalf("claim: %v, %v", lease, err)
	}

	impostor := h.client("claude-cross-host-task")
	impostor.host = "host-b"
	impostor.threadID = thread
	if err := impostor.CompleteTaskLease("claude-cross-host-task", "cross-host-task", lease.Token, "stolen via cross-host remint"); !errors.Is(err, ErrNotOwner) {
		t.Fatalf("cross-host CompleteTaskLease with matching agent+thread = %v, want ErrNotOwner", err)
	}
	if err := owner.CompleteTaskLease("claude-cross-host-task", "cross-host-task", lease.Token, "mine"); err != nil {
		t.Fatalf("owner completing its own task lease: %v", err)
	}
}

// TestIdentityOwnershipRemintFallbackRequiresRegisteredThread is the second
// negative control: an EMPTY thread_id on both sides must never match —
// collapsing unregistered/legacy sessions into a shared owner would let any
// two threadless sessions for the same agent steal each other's leases.
func TestIdentityOwnershipRemintFallbackRequiresRegisteredThread(t *testing.T) {
	h := newIdentityHarness(t)

	first := h.client("claude-threadless")
	first.threadID = "" // explicitly unregistered
	id, _, err := first.SendGuarded(SendReq{From: "x", To: "claude-threadless", Title: "t", Type: "proposal", Instructions: "i"})
	if err != nil {
		t.Fatal(err)
	}
	lease, err := first.ClaimNext("claude-threadless", time.Minute)
	if err != nil || lease == nil {
		t.Fatalf("claim: %v, %v", lease, err)
	}

	second := h.client("claude-threadless")
	second.threadID = "" // also unregistered — must NOT be treated as equivalent
	if err := second.Complete(id, lease.Token, "stolen via empty thread id"); !errors.Is(err, ErrNotOwner) {
		t.Fatalf("Complete from a second threadless session = %v, want ErrNotOwner", err)
	}
	if err := first.Complete(id, lease.Token, "mine"); err != nil {
		t.Fatalf("original threadless session completing its own lease: %v", err)
	}
}

func TestIdentityServerOnlyMethodsAreNotServed(t *testing.T) {
	h := newIdentityHarness(t)
	rs := h.client("claude-a")
	if _, err := rs.Inbox("claude-a"); err != nil {
		t.Fatal(err)
	}
	for _, m := range []string{"GetSession", "RevokeSession", "BindItemSession", "ItemSession", "TaskSession", "BindTaskSession", "TouchSession"} {
		body := []byte(`{"args":["x"]}`)
		if code := signedPost(t, h.srv.URL, "host-token", rs.session, rs.runtime, m, "1788386400000."+m, body); code != 404 {
			t.Fatalf("%s must not be reachable over the wire, got %d", m, code)
		}
	}
}

// signedPost sends one raw signed request and returns the HTTP status.
func signedPost(t *testing.T, base, token string, sess Session, runtime, method, nonce string, body []byte) int {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, base+"/v1/call/"+method, bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("X-Sirsi-Session", sess.ID)
	req.Header.Set("X-Sirsi-Nonce", nonce)
	req.Header.Set("X-Sirsi-Runtime", runtime)
	req.Header.Set("X-Sirsi-Signature", Sign(sess.Secret, method, nonce, body))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	return resp.StatusCode
}
