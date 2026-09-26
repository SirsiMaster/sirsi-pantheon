package routerstore

import (
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
