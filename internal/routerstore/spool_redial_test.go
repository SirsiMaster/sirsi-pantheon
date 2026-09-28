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

// halfOpenTransport models a dead/half-open pooled connection: the FIRST
// forward fails with firstErr, later forwards succeed. It also counts calls to
// CloseIdleConnections so a test can assert the relay dropped the stale pool
// before re-dialing. Implementing CloseIdleConnections makes
// http.Client.CloseIdleConnections reach it (the closeIdler interface).
type halfOpenTransport struct {
	mu         sync.Mutex
	attempts   int
	closedIdle int
	firstErr   error // returned on attempt 1 only
}

func (t *halfOpenTransport) CloseIdleConnections() {
	t.mu.Lock()
	t.closedIdle++
	t.mu.Unlock()
}

func (t *halfOpenTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	t.mu.Lock()
	t.attempts++
	n := t.attempts
	fe := t.firstErr
	t.mu.Unlock()
	if n == 1 && fe != nil {
		return nil, fe
	}
	_, _ = io.ReadAll(req.Body) // body must be re-readable on the retry
	return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"result":[]}`)), Header: make(http.Header)}, nil
}

func (t *halfOpenTransport) count() (attempts, closedIdle int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.attempts, t.closedIdle
}

func seedInflight(t *testing.T, lane, id string) string {
	t.Helper()
	dir := filepath.Join(lane, "inflight")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(dir, id+".json")
	if err := writeAtomic(p, spoolRequest{Method: "SendGuarded", Headers: map[string]string{}, Body: []byte(`{"id":"` + id + `"}`)}, 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func discardLog() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

// rs-30: a forward whose first attempt PROVABLY never reached the service (a
// half-open pooled connection surfaces this once re-dialed) must drop the idle
// pool and re-dial ONCE on a fresh connection, and SUCCEED — not park in the
// outbox or hang. Pre-fix (no re-dial), forward returns statusHoldForRetry and
// this test is red; that is the required negative-control-on-the-fix direction.
func TestForwardRedialsOnHalfOpenAndSucceeds(t *testing.T) {
	spool := t.TempDir()
	lane := filepath.Join(spool, "x")
	p := seedInflight(t, lane, "001-x")

	// A dial-phase i/o timeout: neverReachedService=true → safe to re-dial.
	rt := &halfOpenTransport{firstErr: &net.OpError{Op: "dial", Net: "tcp", Err: errors.New("i/o timeout")}}
	rl := &Relay{Spool: spool, Base: "http://svc.invalid", Token: "t", Log: discardLog(), Client: &http.Client{Transport: rt}, now: time.Now}

	sr := rl.forward("x", "001-x", p)

	if sr.Status != http.StatusOK {
		t.Fatalf("want 200 after one safe re-dial, got status %d body=%s", sr.Status, sr.Body)
	}
	attempts, closedIdle := rt.count()
	if attempts != 2 {
		t.Fatalf("want exactly 2 attempts (fail, then re-dial), got %d", attempts)
	}
	if closedIdle != 1 {
		t.Fatalf("want idle pool dropped exactly once before the re-dial, got %d", closedIdle)
	}
}

// rs-30 NEGATIVE CONTROL: a post-send failure (response lost after the request
// left this process — neverReachedService=false) MUST NOT be auto-retried, or a
// mutation the service already committed would be double-committed. It stays
// OUTCOME UNKNOWN (502), attempted exactly once. This is the same discipline the
// durable outbox enforces on retry; the re-dial must never weaken it.
func TestForwardDoesNotRetryPostSendFailure(t *testing.T) {
	spool := t.TempDir()
	lane := filepath.Join(spool, "x")
	p := seedInflight(t, lane, "002-x")

	// A write-phase broken pipe AFTER connect: neverReachedService=false.
	rt := &halfOpenTransport{firstErr: &net.OpError{Op: "write", Net: "tcp", Err: errors.New("broken pipe")}}
	rl := &Relay{Spool: spool, Base: "http://svc.invalid", Token: "t", Log: discardLog(), Client: &http.Client{Transport: rt}, now: time.Now}

	sr := rl.forward("x", "002-x", p)

	if sr.Status != http.StatusBadGateway {
		t.Fatalf("post-send failure must stay OUTCOME UNKNOWN (502), got status %d body=%s", sr.Status, sr.Body)
	}
	if !strings.Contains(string(sr.Body), "OUTCOME UNKNOWN") {
		t.Fatalf("want OUTCOME UNKNOWN in body, got %s", sr.Body)
	}
	if attempts, closedIdle := rt.count(); attempts != 1 || closedIdle != 0 {
		t.Fatalf("post-send failure must NOT be auto-retried: got %d attempts, %d idle-closes (want 1, 0)", attempts, closedIdle)
	}
}

// rs-30: if the re-dial ALSO cannot reach the service, the request is provably
// never-committed, so it is HELD for ordered outbox retry (statusHoldForRetry),
// never reported as unknown. Confirms the retry does not corrupt the outbox path.
func TestForwardRedialStillUnreachableHolds(t *testing.T) {
	spool := t.TempDir()
	lane := filepath.Join(spool, "x")
	p := seedInflight(t, lane, "003-x")

	// down on BOTH attempts: every RoundTrip is a dial failure.
	rt := &alwaysDialFail{}
	rl := &Relay{Spool: spool, Base: "http://svc.invalid", Token: "t", Log: discardLog(), Client: &http.Client{Transport: rt}, now: time.Now}

	sr := rl.forward("x", "003-x", p)
	if sr.Status != statusHoldForRetry {
		t.Fatalf("persistent unreachability must HOLD for retry (%d), got %d", statusHoldForRetry, sr.Status)
	}
	if rt.attempts() != 2 {
		t.Fatalf("want the one safe re-dial attempted (2 total), got %d", rt.attempts())
	}
}

type alwaysDialFail struct {
	mu sync.Mutex
	n  int
}

func (t *alwaysDialFail) RoundTrip(*http.Request) (*http.Response, error) {
	t.mu.Lock()
	t.n++
	t.mu.Unlock()
	return nil, &net.OpError{Op: "dial", Net: "tcp", Err: syscall.ECONNREFUSED}
}
func (t *alwaysDialFail) attempts() int { t.mu.Lock(); defer t.mu.Unlock(); return t.n }
