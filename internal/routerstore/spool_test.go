package routerstore

import (
	"context"
	"log/slog"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// End to end through files: a real Handler behind httptest stands in for the
// service, the Relay forwards with the host token, and a RemoteStore over
// spool:// (no token) mints a session, signs and works the ledger.
func TestSpoolRelayEndToEnd(t *testing.T) {
	backend := newDst(t)
	h, err := Handler(backend, ServerOptions{Token: "h0st", MaxWait: 3 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	spool := t.TempDir()
	logBuf := &strings.Builder{}
	rl := &Relay{Spool: spool, Base: srv.URL, Token: "h0st", Log: slog.New(slog.NewTextHandler(logBuf, nil))}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go func() { _ = rl.Serve(ctx) }()

	t.Setenv("SIRSI_AGENT_ID", "lane-x")
	t.Setenv("HOME", t.TempDir()) // session cache dir
	rs := NewRemoteStore("spool://"+spool, "")
	if rs.token != "" {
		t.Fatal("spool client must hold no token")
	}
	id, _, err := rs.SendGuarded(SendReq{From: "lane-x", To: "b", Title: "via spool", Type: "proposal", Instructions: "x"})
	if err != nil {
		t.Fatalf("send through spool: %v\nrelay log:\n%s", err, logBuf.String())
	}
	it, err := rs.Get(id)
	if err != nil || it.Title != "via spool" {
		t.Fatalf("get through spool: %v %+v", err, it)
	}
	// Files are cleaned up after each round trip.
	if left, _ := filepath.Glob(filepath.Join(spool, "lane-x", "*", "*.json")); len(left) != 0 {
		t.Fatalf("spool not cleaned: %v", left)
	}
	// The relay log never carries a token or a signature.
	if l := logBuf.String(); strings.Contains(l, "h0st") || strings.Contains(l, "X-Sirsi-Signature") || !strings.Contains(l, "method=SendGuarded") {
		t.Fatalf("relay log must be secret-free and name the method:\n%s", l)
	}
	// Token methods are refused by name, never forwarded.
	var out []any
	if err := rs.call("ListHostTokens", nil, &out); err == nil || !strings.Contains(err.Error(), "never forwarded") {
		t.Fatalf("ListHostTokens must be refused by the relay: %v", err)
	}
}

// Without a relay the lane fails loudly and leaves nothing behind.
func TestSpoolWithoutRelayTimesOut(t *testing.T) {
	spool := t.TempDir()
	tr := newSpoolTransport(spool, "lane-y")
	tr.wait = 400 * time.Millisecond
	t.Setenv("SIRSI_AGENT_ID", "lane-y")
	t.Setenv("HOME", t.TempDir())
	rs := NewRemoteStore("spool://"+spool, "")
	rs.client.Transport = tr
	_, err := rs.Get("nope")
	if err == nil || !strings.Contains(err.Error(), "relay serve") {
		t.Fatalf("want a loud no-relay error, got %v", err)
	}
	if left, _ := filepath.Glob(filepath.Join(spool, "lane-y", "req", "*.json")); len(left) != 0 {
		t.Fatalf("request left behind after timeout: %v", left)
	}
}

// Resolve accepts spool:// without a token and still refuses http(s) without one.
func TestResolveSpoolNeedsNoToken(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("SIRSI_ROUTER_TOKEN", "")
	t.Setenv("SIRSI_ROUTER_URL", "spool://"+t.TempDir())
	s, err := Resolve()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := s.(*RemoteStore); !ok {
		t.Fatalf("got %T", s)
	}
	t.Setenv("SIRSI_ROUTER_URL", "https://router.invalid")
	if _, err := Resolve(); err == nil {
		t.Fatal("https without token must still be refused")
	}
}
