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
