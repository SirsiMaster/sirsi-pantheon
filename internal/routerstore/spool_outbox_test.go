package routerstore

import (
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

// flakyTransport fails every request with a dial error (never-reached) while
// down, then records the forwarded body order while up. Lets the test simulate
// a cloud outage and its recovery deterministically.
type flakyTransport struct {
	mu        sync.Mutex
	down      bool
	forwarded []string
}

func (t *flakyTransport) setDown(d bool) { t.mu.Lock(); t.down = d; t.mu.Unlock() }

func (t *flakyTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.down {
		// Provably-never-sent: a dial failure. neverReachedService → hold.
		return nil, &net.OpError{Op: "dial", Net: "tcp", Err: syscall.ECONNREFUSED}
	}
	b, _ := io.ReadAll(req.Body)
	t.forwarded = append(t.forwarded, string(b))
	return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"result":[]}`)), Header: make(http.Header)}, nil
}

func (t *flakyTransport) order() []string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return append([]string(nil), t.forwarded...)
}

// ADR-069 (goal #2): a message that can't reach the cloud is HELD in the
// relay-owned outbox and re-forwarded IN ORDER on recovery — none dropped, none
// double-delivered.
func TestOutboxHoldsAndReleasesInOrder(t *testing.T) {
	spool := t.TempDir()
	lane := filepath.Join(spool, "x")
	if err := os.MkdirAll(filepath.Join(lane, "req"), 0o700); err != nil {
		t.Fatal(err)
	}
	ids := []string{"001-x", "002-x", "003-x"}
	for _, id := range ids {
		// Body carries the id so the transport can assert delivery order.
		body := []byte(`{"id":"` + id + `"}`)
		if err := writeAtomic(filepath.Join(lane, "req", id+".json"), spoolRequest{Method: "SendGuarded", Headers: map[string]string{}, Body: body}, 0o600); err != nil {
			t.Fatal(err)
		}
	}

	rt := &flakyTransport{down: true}
	rl := &Relay{Spool: spool, Base: "http://svc.invalid", Token: "t", Log: slog.New(slog.NewTextHandler(io.Discard, nil)), Client: &http.Client{Transport: rt}, now: time.Now}

	// --- cloud DOWN: drain should HOLD all three, drop none ---
	rl.serveOnce()
	held, _ := filepath.Glob(filepath.Join(lane, "outbox", "*.json"))
	if len(held) != 3 {
		t.Fatalf("cloud down: want 3 held in outbox, got %d", len(held))
	}
	if reqs, _ := filepath.Glob(filepath.Join(lane, "req", "*.json")); len(reqs) != 0 {
		t.Fatalf("cloud down: req/ should be drained into outbox, %d left", len(reqs))
	}
	if inf, _ := filepath.Glob(filepath.Join(lane, "inflight", "*.json")); len(inf) != 0 {
		t.Fatalf("cloud down: inflight/ should be empty, %d left", len(inf))
	}
	if got := rt.order(); len(got) != 0 {
		t.Fatalf("cloud down: nothing should have been forwarded, got %v", got)
	}
	// The client learned it is queued (a response exists per id), not dropped.
	if res, _ := filepath.Glob(filepath.Join(lane, "res", "*.json")); len(res) != 3 {
		t.Fatalf("cloud down: want 3 QUEUED responses for the clients, got %d", len(res))
	}

	// --- cloud UP: outbox drains, IN ORDER, exactly once ---
	rt.setDown(false)
	rl.serveOnce()
	if held, _ := filepath.Glob(filepath.Join(lane, "outbox", "*.json")); len(held) != 0 {
		t.Fatalf("cloud up: outbox should be empty, %d left", len(held))
	}
	want := []string{`{"id":"001-x"}`, `{"id":"002-x"}`, `{"id":"003-x"}`}
	got := rt.order()
	if len(got) != 3 {
		t.Fatalf("cloud up: want 3 forwards (no drop, no double), got %d: %v", len(got), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("cloud up: out of order at %d: got %v, want %v", i, got, want)
		}
	}
}

// scriptedTransport gives per-id control: dial-fail (never-reached) for a set of
// ids, or post-send failure (OUTCOME UNKNOWN) globally, else succeed. It records
// the ids it actually forwarded, in order.
type scriptedTransport struct {
	mu           sync.Mutex
	dialFailIDs  map[string]bool
	postSendFail bool
	forwarded    []string
}

func bodyID(body string) string {
	if i := strings.Index(body, `"id":"`); i >= 0 {
		if j := strings.IndexByte(body[i+6:], '"'); j >= 0 {
			return body[i+6 : i+6+j]
		}
	}
	return ""
}

func (t *scriptedTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	b, _ := io.ReadAll(req.Body)
	id := bodyID(string(b))
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.dialFailIDs[id] {
		return nil, &net.OpError{Op: "dial", Net: "tcp", Err: syscall.ECONNREFUSED}
	}
	if t.postSendFail {
		// A write-phase error AFTER connect: neverReachedService=false, so forward
		// returns OUTCOME UNKNOWN (502) — the request may already have committed.
		return nil, &net.OpError{Op: "write", Net: "tcp", Err: errors.New("broken pipe")}
	}
	t.forwarded = append(t.forwarded, id)
	return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"result":[]}`)), Header: make(http.Header)}, nil
}

func (t *scriptedTransport) order() []string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return append([]string(nil), t.forwarded...)
}

func seedOutbox(t *testing.T, lane string, ids ...string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(lane, "outbox"), 0o700); err != nil {
		t.Fatal(err)
	}
	for _, id := range ids {
		body := []byte(`{"id":"` + id + `"}`)
		if err := writeAtomic(filepath.Join(lane, "outbox", id+".json"), spoolRequest{Method: "SendGuarded", Headers: map[string]string{}, Body: body}, 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

// SSA #794 finding 1 (2026-09-27): drainOutbox must STOP at the first
// non-delivery. If an earlier held request is still unreachable while a later
// one is reachable, forwarding the later one first violates ADR-069's
// ordered-release guarantee.
func TestOutboxStopsAtFirstUnreachable(t *testing.T) {
	spool := t.TempDir()
	lane := filepath.Join(spool, "x")
	seedOutbox(t, lane, "001-x", "002-x", "003-x")

	// 001 still unreachable; 002 and 003 WOULD succeed if attempted.
	rt := &scriptedTransport{dialFailIDs: map[string]bool{"001-x": true}}
	rl := &Relay{Spool: spool, Base: "http://svc.invalid", Token: "t", Log: slog.New(slog.NewTextHandler(io.Discard, nil)), Client: &http.Client{Transport: rt}, now: time.Now}

	rl.serveOnce()

	if got := rt.order(); len(got) != 0 {
		t.Fatalf("ordered release: nothing may be forwarded while the head is stuck, got %v", got)
	}
	if held, _ := filepath.Glob(filepath.Join(lane, "outbox", "*.json")); len(held) != 3 {
		t.Fatalf("all 3 must remain held (frontier stop), got %d", len(held))
	}
	if inf, _ := filepath.Glob(filepath.Join(lane, "inflight", "*.json")); len(inf) != 0 {
		t.Fatalf("inflight/ must be empty after a held retry, got %d", len(inf))
	}

	// Head recovers → the whole lane drains in order, exactly once.
	rt.mu.Lock()
	rt.dialFailIDs = map[string]bool{}
	rt.mu.Unlock()
	rl.serveOnce()
	want := []string{"001-x", "002-x", "003-x"}
	if got := rt.order(); len(got) != 3 || got[0] != want[0] || got[1] != want[1] || got[2] != want[2] {
		t.Fatalf("after recovery, want in-order %v, got %v", want, got)
	}
	if held, _ := filepath.Glob(filepath.Join(lane, "outbox", "*.json")); len(held) != 0 {
		t.Fatalf("outbox must be empty after recovery, got %d", len(held))
	}
}

// SSA #794 finding 2 (2026-09-27): a post-send OUTCOME-UNKNOWN result on retry
// must NOT delete the held record (the service may have committed) and must NOT
// auto-re-forward it (double-commit). It is parked in failed/ for audit, and the
// drain stops at that frontier.
func TestOutboxParksOnOutcomeUnknownNotDeleted(t *testing.T) {
	spool := t.TempDir()
	lane := filepath.Join(spool, "x")
	seedOutbox(t, lane, "001-x", "002-x")

	rt := &scriptedTransport{postSendFail: true}
	rl := &Relay{Spool: spool, Base: "http://svc.invalid", Token: "t", Log: slog.New(slog.NewTextHandler(io.Discard, nil)), Client: &http.Client{Transport: rt}, now: time.Now}

	rl.serveOnce()

	// 001 forwarded, got OUTCOME UNKNOWN → parked to failed/, NOT deleted.
	if failed, _ := filepath.Glob(filepath.Join(lane, "failed", "*.json")); len(failed) != 1 {
		t.Fatalf("OUTCOME UNKNOWN must be parked in failed/ (not deleted), got %d", len(failed))
	}
	// The drain stopped at the frontier: 002 was never attempted, still held.
	if held, _ := filepath.Glob(filepath.Join(lane, "outbox", "*.json")); len(held) != 1 {
		t.Fatalf("frontier stop: 002 must remain held, got %d held", len(held))
	}
	if inf, _ := filepath.Glob(filepath.Join(lane, "inflight", "*.json")); len(inf) != 0 {
		t.Fatalf("inflight/ must be empty, got %d", len(inf))
	}
}
