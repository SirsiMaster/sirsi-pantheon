package routerstore

// rs-13 outage finding: while Postgres was down for 30 s, every node saw
// "HTTP 401: missing or invalid bearer token" because the token lookup failed
// and the handler treated the failure as an auth verdict. A dead database is
// 503 — retry — never 401.

import (
	"errors"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"
)

// flakyStore wraps a real store and fails the auth-chain lookups on demand.
type flakyStore struct {
	*SQLiteStore
	down bool
}

var errDBDown = errors.New("FATAL: terminating connection due to administrator command")

func (f *flakyStore) LookupHostToken(tok string) (HostToken, error) {
	if f.down {
		return HostToken{}, errDBDown
	}
	return f.SQLiteStore.LookupHostToken(tok)
}

func (f *flakyStore) GetSession(id string) (Session, error) {
	if f.down {
		return Session{}, errDBDown
	}
	return f.SQLiteStore.GetSession(id)
}

func TestOutageIs503NotUnauthorized(t *testing.T) {
	backend := openBackendStore(t, filepath.Join(t.TempDir(), "router.db"))
	t.Cleanup(func() { _ = backend.Close() })
	backend.notifyDir = t.TempDir()
	fs := &flakyStore{SQLiteStore: backend}
	plain, _, err := backend.MintHostToken("m1", "")
	if err != nil {
		t.Fatal(err)
	}
	h, err := Handler(fs, ServerOptions{Token: "boot"})
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)

	rs := NewRemoteStore(srv.URL, plain)
	rs.sessionDir = ""
	rs.host, rs.agent = "m1", "claude-m1"
	if _, ierr := rs.Inbox("claude-m1"); ierr != nil {
		t.Fatal("positive control (db up):", ierr)
	}
	sid := rs.session.ID

	fs.down = true
	_, err = rs.Inbox("claude-m1")
	if !errors.Is(err, ErrServiceUnavailable) {
		t.Fatalf("db down: want ErrServiceUnavailable (503), got %v", err)
	}
	// The client must NOT have thrown its session away: 503 is not an auth verdict.
	if rs.session.ID != sid {
		t.Fatal("client dropped its session on a 503")
	}

	fs.down = false
	if _, err := rs.Inbox("claude-m1"); err != nil {
		t.Fatalf("db back: same session must work again without a re-mint: %v", err)
	}
	if rs.session.ID != sid {
		t.Fatal("session changed across the outage")
	}
	_ = time.Second
}
