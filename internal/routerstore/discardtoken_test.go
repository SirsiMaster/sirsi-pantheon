package routerstore

// Schema v29: sessions.token_id and DiscardMintedToken (publish-failure cleanup that must not sign out the host).
// Synthetic values only; no test prints a token.

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"path/filepath"
	"testing"
	"time"
)

const discardHost = "m1"

// tokenClient is a RemoteStore authenticating with one specific host token (it mints through that bearer).
func (h *identityHarness) tokenClient(token, agent string) *RemoteStore {
	rs := NewRemoteStore(h.srv.URL, token)
	rs.sessionDir = ""
	rs.host, rs.agent = discardHost, agent
	rs.now = func() time.Time { return h.now }
	return rs
}

// sessionToken reads sessions.token_id straight from the table (the field is deliberately not on Session).
func sessionToken(t *testing.T, s *SQLiteStore, sessionID string) string {
	t.Helper()
	var id string
	if err := s.db.QueryRow(`SELECT token_id FROM sessions WHERE session_id=?`, sessionID).Scan(&id); err != nil {
		t.Fatalf("read token_id of %s: %v", sessionID, err)
	}
	return id
}

func sessionLive(s *SQLiteStore, id string) bool {
	_, err := s.GetSession(id)
	return err == nil
}

// replacementFixture is the failed same-host replacement: an old token with a live session, a freshly minted
// replacement with a session, and a legacy (” token_id) session on the same host.
type replacementFixture struct {
	h                  *identityHarness
	oldTok, newTok     HostToken
	oldPlain, newPlain string
	oldSess, newSess   string
	legacySess         string
	otherHostSess      string
}

func newReplacementFixture(t *testing.T) *replacementFixture {
	t.Helper()
	h := newIdentityHarness(t)
	f := &replacementFixture{h: h}
	var err error
	if f.oldPlain, f.oldTok, err = h.backend.MintHostToken(discardHost, "old"); err != nil {
		t.Fatal(err)
	}
	if f.newPlain, f.newTok, err = h.backend.MintHostToken(discardHost, "new"); err != nil {
		t.Fatal(err)
	}
	oc := h.tokenClient(f.oldPlain, "lane-old")
	nc := h.tokenClient(f.newPlain, "lane-new")
	if _, err := oc.Inbox("lane-old"); err != nil {
		t.Fatal(err)
	}
	if _, err := nc.Inbox("lane-new"); err != nil {
		t.Fatal(err)
	}
	f.oldSess, f.newSess = oc.session.ID, nc.session.ID
	legacy, err := h.backend.MintSession(discardHost, "lane-legacy", "rt") // in-process mint: unattributed
	if err != nil {
		t.Fatal(err)
	}
	f.legacySess = legacy.ID
	other, err := h.backend.MintSession("m5", "lane-m5", "rt")
	if err != nil {
		t.Fatal(err)
	}
	f.otherHostSess = other.ID
	return f
}

// Every mint path stamps the issuing token from the authenticated bearer; bootstrap and in-process mints are ”.
func TestSessionRecordsIssuingTokenFromAuthenticatedBearer(t *testing.T) {
	f := newReplacementFixture(t)
	s := f.h.backend
	if got := sessionToken(t, s, f.oldSess); got != f.oldTok.ID {
		t.Fatalf("old-token session token_id = %q, want the old token's id", got)
	}
	if got := sessionToken(t, s, f.newSess); got != f.newTok.ID {
		t.Fatalf("new-token session token_id = %q, want the new token's id", got)
	}
	if got := sessionToken(t, s, f.legacySess); got != "" {
		t.Fatalf("in-process mint token_id = %q, want ''", got)
	}
	boot := f.h.client("lane-boot") // bootstrap bearer
	boot.host = discardHost
	if _, err := boot.Inbox("lane-boot"); err != nil {
		t.Fatal(err)
	}
	if got := sessionToken(t, s, boot.session.ID); got != "" {
		t.Fatalf("bootstrap-token session token_id = %q, want ''", got)
	}
	// MintSessionForThread (thread-bound lanes) goes through the same server path.
	tc := f.h.tokenClient(f.newPlain, "lane-thread")
	tc.threadID = "thr-synthetic"
	if _, err := tc.Inbox("lane-thread"); err != nil {
		t.Fatal(err)
	}
	if got := sessionToken(t, s, tc.session.ID); got != f.newTok.ID {
		t.Fatalf("thread-bound session token_id = %q, want the new token's id", got)
	}
}

// A client cannot choose token_id: the wire has no such argument, and an extra argument is ignored.
func TestSessionTokenIDIsNeverClientSupplied(t *testing.T) {
	f := newReplacementFixture(t)
	body, _ := json.Marshal(map[string]any{"args": []any{discardHost, "lane-forge", "rt", "thr-x", f.oldTok.ID, "extra"}})
	req, err := http.NewRequest(http.MethodPost, f.h.srv.URL+"/v1/call/MintSessionForThread", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+f.newPlain)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	var out struct {
		Result []Session `json:"result"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil || resp.StatusCode != 200 || len(out.Result) != 1 {
		t.Fatalf("forged-arg mint: status %d err %v", resp.StatusCode, err)
	}
	if got := sessionToken(t, f.h.backend, out.Result[0].ID); got != f.newTok.ID {
		t.Fatalf("token_id = %q: a client-supplied value must never win over the authenticated bearer (%q)", got, f.newTok.ID)
	}
}

// Refresh path: a client whose session died re-mints through its own bearer, so the new session inherits the
// SAME token_id. There is no session-from-session mint: a mint needs a bearer, and the bearer decides token_id.
func TestSessionRemintInheritsTokenID(t *testing.T) {
	f := newReplacementFixture(t)
	nc := f.h.tokenClient(f.newPlain, "lane-new")
	if _, err := nc.Inbox("lane-new"); err != nil {
		t.Fatal(err)
	}
	first := nc.session.ID
	if err := f.h.backend.RevokeSession(first); err != nil {
		t.Fatal(err)
	}
	if _, err := nc.Inbox("lane-new"); err != nil {
		t.Fatalf("client must re-mint after its session died: %v", err)
	}
	if nc.session.ID == first {
		t.Fatal("expected a fresh session after revoke")
	}
	if got := sessionToken(t, f.h.backend, nc.session.ID); got != f.newTok.ID {
		t.Fatalf("re-minted session token_id = %q, want inherited %q", got, f.newTok.ID)
	}
	// And the discard still reaches it.
	if err := f.h.backend.DiscardMintedToken(f.newTok.ID); err != nil {
		t.Fatal(err)
	}
	if sessionLive(f.h.backend, nc.session.ID) {
		t.Fatal("re-minted session survived DiscardMintedToken")
	}
}

// The failed-publish cleanup keeps old-token (and legacy, and other-host) sessions alive and kills new-token ones.
func TestDiscardMintedTokenRevokesOnlyItsOwnSessions(t *testing.T) {
	f := newReplacementFixture(t)
	s := f.h.backend
	if err := s.DiscardMintedToken(f.newTok.ID); err != nil {
		t.Fatal(err)
	}
	if sessionLive(s, f.newSess) {
		t.Fatal("new-token session must be revoked")
	}
	if _, err := s.LookupHostToken(f.newPlain); !errors.Is(err, ErrTokenRevoked) {
		t.Fatalf("new token must be revoked, got %v", err)
	}
	for name, id := range map[string]string{"old-token": f.oldSess, "legacy-unattributed": f.legacySess, "other-host": f.otherHostSess} {
		if !sessionLive(s, id) {
			t.Fatalf("%s session must survive the cleanup", name)
		}
	}
	if _, err := s.LookupHostToken(f.oldPlain); err != nil {
		t.Fatalf("old token must stay valid: %v", err)
	}
	// The old session still works over the wire.
	if _, err := f.h.tokenClient(f.oldPlain, "lane-old").Inbox("lane-old"); err != nil {
		t.Fatalf("old token's lane must keep working: %v", err)
	}
	// Revoked rows are marked, not deleted (auditable); idempotent on retry.
	list, err := s.ListHostTokens()
	if err != nil {
		t.Fatal(err)
	}
	var seen bool
	for _, tok := range list {
		if tok.ID == f.newTok.ID {
			seen = tok.Revoked != ""
		}
	}
	if !seen {
		t.Fatal("discarded token row must remain, marked revoked")
	}
	if err := s.DiscardMintedToken(f.newTok.ID); err != nil {
		t.Fatalf("a retried cleanup must converge: %v", err)
	}
}

// A scoped token's scope rows are kept (marked-revoked, not deleted), consistent with RevokeHostToken.
func TestDiscardMintedTokenKeepsScopeRows(t *testing.T) {
	s := openTestStore(t)
	_, rec, err := s.MintScopedHostToken("cloud", "l", []string{"lane-a"})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.DiscardMintedToken(rec.ID); err != nil {
		t.Fatal(err)
	}
	agents, err := s.tokenAgents(rec.ID)
	if err != nil || len(agents) != 1 || agents[0] != "lane-a" {
		t.Fatalf("scope rows after discard = %v (%v), want kept", agents, err)
	}
}

// Negative control: the OLD cleanup callback (RevokeHostToken) signs out every session on the host. If this test
// ever stops failing the way it asserts, the positive test above no longer proves what it claims.
func TestNegativeControlRevokeHostTokenKillsEveryHostSession(t *testing.T) {
	f := newReplacementFixture(t)
	s := f.h.backend
	if err := s.RevokeHostToken(f.newTok.ID); err != nil {
		t.Fatal(err)
	}
	for name, id := range map[string]string{"old-token": f.oldSess, "legacy": f.legacySess, "new-token": f.newSess} {
		if sessionLive(s, id) {
			t.Fatalf("%s session survived RevokeHostToken: the negative control no longer reproduces the defect", name)
		}
	}
	if !sessionLive(s, f.otherHostSess) {
		t.Fatal("other-host session must survive an explicit revoke")
	}
}

// Explicit operator revoke keeps its exact behavior: the token and every session on its host, and a second
// revoke of the same id is ErrTokenUnknown.
func TestExplicitRevokeHostTokenUnchanged(t *testing.T) {
	f := newReplacementFixture(t)
	s := f.h.backend
	if err := s.RevokeHostToken(f.oldTok.ID); err != nil {
		t.Fatal(err)
	}
	for name, id := range map[string]string{"old-token": f.oldSess, "new-token": f.newSess, "legacy": f.legacySess} {
		if sessionLive(s, id) {
			t.Fatalf("%s session must be revoked by an explicit host-token revoke", name)
		}
	}
	if !sessionLive(s, f.otherHostSess) {
		t.Fatal("other host must be unaffected")
	}
	if _, err := s.LookupHostToken(f.newPlain); err != nil {
		t.Fatalf("explicit revoke of one token must not revoke the sibling token itself: %v", err)
	}
	if err := s.RevokeHostToken(f.oldTok.ID); !errors.Is(err, ErrTokenUnknown) {
		t.Fatalf("second revoke = %v, want ErrTokenUnknown", err)
	}
}

// The empty id would match every legacy session; it is refused and nothing changes. An unknown id is refused too.
func TestDiscardMintedTokenRefusesEmptyAndUnknownID(t *testing.T) {
	f := newReplacementFixture(t)
	s := f.h.backend
	for _, id := range []string{"", "   "} {
		if err := s.DiscardMintedToken(id); !errors.Is(err, ErrDiscardTokenID) {
			t.Fatalf("DiscardMintedToken(%q) = %v, want ErrDiscardTokenID", id, err)
		}
	}
	if err := s.DiscardMintedToken("no-such-token"); !errors.Is(err, ErrTokenUnknown) {
		t.Fatalf("unknown id = %v, want ErrTokenUnknown", err)
	}
	for name, id := range map[string]string{"old": f.oldSess, "new": f.newSess, "legacy": f.legacySess, "other": f.otherHostSess} {
		if !sessionLive(s, id) {
			t.Fatalf("%s session revoked by a refused discard", name)
		}
	}
}

// DiscardMintedToken is server-side only: not on the wire and not forwarded by the relay.
func TestDiscardMintedTokenIsNeverServed(t *testing.T) {
	if !notServed["DiscardMintedToken"] || !spoolRefused["DiscardMintedToken"] {
		t.Fatal("DiscardMintedToken must be in notServed and spoolRefused")
	}
	f := newReplacementFixture(t)
	body, _ := json.Marshal(map[string]any{"args": []any{f.newTok.ID}})
	for _, tok := range []string{"host-token", f.oldPlain, f.newPlain} {
		req, err := http.NewRequest(http.MethodPost, f.h.srv.URL+"/v1/call/DiscardMintedToken", bytes.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Authorization", "Bearer "+tok)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusNotFound {
			t.Fatalf("DiscardMintedToken over the wire = %d, want 404", resp.StatusCode)
		}
	}
	if !sessionLive(f.h.backend, f.newSess) {
		t.Fatal("a refused wire call changed state")
	}
}

// A v28 ledger upgrades in place: existing sessions get token_id ” and a discard never matches them.
func TestV28ToV29AddsSessionTokenID(t *testing.T) {
	path := filepath.Join(t.TempDir(), "v28.db")
	s, err := OpenPath(path)
	if err != nil {
		t.Fatal(err)
	}
	sess, err := s.MintSession("h", "a", "rt")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`ALTER TABLE sessions DROP COLUMN token_id; PRAGMA user_version=28;`); err != nil {
		t.Fatalf("rewind to v28: %v", err)
	}
	_ = s.Close()
	s2, err := OpenPath(path)
	if err != nil {
		t.Fatalf("v28 to v29 upgrade must apply cleanly: %v", err)
	}
	defer func() { _ = s2.Close() }()
	if v, e := ReadSchemaVersion(path); e != nil || v != 29 {
		t.Fatalf("post-upgrade version = %d (%v), want 29", v, e)
	}
	if got := sessionToken(t, s2, sess.ID); got != "" {
		t.Fatalf("pre-v29 session token_id = %q, want ''", got)
	}
}
