package routerstore

import (
	"errors"
	"net"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// handshakeTimeout is what net/http returns when the TLS handshake stalls, as
// the Client wraps it: a *url.Error around net/http's unexported timeout error.
func handshakeTimeout() error {
	return &url.Error{Op: "Post", URL: "https://svc.invalid/v1/call/MaatRead", Err: errors.New("net/http: TLS handshake timeout")}
}

// A stalled TLS handshake happens before any request byte is written, so the
// request never reached the service. The relay's own doc comment said so; the
// code did not detect it, and the stall surfaced as OUTCOME UNKNOWN on a lossy
// link (Mercury and Ra, 2026-10-24, M1 on a phone hotspot).
func TestNeverReachedServiceClassifiesTLSHandshakeTimeout(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		want bool
	}{
		{"handshake timeout is never-reached", handshakeTimeout(), true},
		{"handshake timeout, wrapped again", errors.Join(errors.New("relay"), handshakeTimeout()), true},
		// Negative controls: these happen AFTER the request was sent.
		{"response header timeout is post-send", &url.Error{Op: "Post", URL: "https://x", Err: errors.New("net/http: timeout awaiting response headers")}, false},
		{"body read deadline is post-send", errors.New("context deadline exceeded (Client.Timeout or context cancellation while reading body)"), false},
		{"write-phase broken pipe is post-send", &net.OpError{Op: "write", Net: "tcp", Err: errors.New("broken pipe")}, false},
		{"nil", nil, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := neverReachedService(tc.err); got != tc.want {
				t.Fatalf("neverReachedService(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}

// One stalled handshake is retried once on a fresh connection and succeeds,
// instead of being reported as an unknown outcome.
func TestForwardRedialsAfterTLSHandshakeTimeout(t *testing.T) {
	spool := t.TempDir()
	p := seedInflight(t, filepath.Join(spool, "x"), "010-x")
	rt := &halfOpenTransport{firstErr: handshakeTimeout()}
	rl := &Relay{Spool: spool, Base: "http://svc.invalid", Token: "t", Log: discardLog(), Client: &http.Client{Transport: rt}, now: time.Now}

	sr := rl.forward("x", "010-x", p)

	if sr.Status != http.StatusOK {
		t.Fatalf("a stalled handshake must be re-dialed and succeed, got status %d body=%s", sr.Status, sr.Body)
	}
	if attempts, closedIdle := rt.count(); attempts != 2 || closedIdle != 1 {
		t.Fatalf("want 2 attempts and 1 idle-close, got %d and %d", attempts, closedIdle)
	}
}

// If the handshake stalls on the retry too, the request is held in the outbox
// (queued, delivered in order later), never reported as an unknown outcome.
func TestForwardHoldsWhenTLSHandshakeKeepsStalling(t *testing.T) {
	spool := t.TempDir()
	p := seedInflight(t, filepath.Join(spool, "x"), "011-x")
	rt := &alwaysHandshakeStall{}
	rl := &Relay{Spool: spool, Base: "http://svc.invalid", Token: "t", Log: discardLog(), Client: &http.Client{Transport: rt}, now: time.Now}

	sr := rl.forward("x", "011-x", p)

	if sr.Status != statusHoldForRetry {
		t.Fatalf("a persistent handshake stall must HOLD for retry (%d), got %d body=%s", statusHoldForRetry, sr.Status, sr.Body)
	}
	if rt.attempts() != 2 {
		t.Fatalf("want exactly one safe re-dial (2 attempts), got %d", rt.attempts())
	}
}

// Negative control at the forward level: a timeout awaiting response headers
// means the request WAS sent. It must not be retried (the service may have
// committed) and stays OUTCOME UNKNOWN.
func TestForwardDoesNotRetryResponseHeaderTimeout(t *testing.T) {
	spool := t.TempDir()
	p := seedInflight(t, filepath.Join(spool, "x"), "012-x")
	rt := &halfOpenTransport{firstErr: &url.Error{Op: "Post", URL: "https://x", Err: errors.New("net/http: timeout awaiting response headers")}}
	rl := &Relay{Spool: spool, Base: "http://svc.invalid", Token: "t", Log: discardLog(), Client: &http.Client{Transport: rt}, now: time.Now}

	sr := rl.forward("x", "012-x", p)

	if sr.Status != http.StatusBadGateway || !strings.Contains(string(sr.Body), "OUTCOME UNKNOWN") {
		t.Fatalf("post-send timeout must stay OUTCOME UNKNOWN, got status %d body=%s", sr.Status, sr.Body)
	}
	if attempts, _ := rt.count(); attempts != 1 {
		t.Fatalf("post-send timeout must not be retried, got %d attempts", attempts)
	}
}

type alwaysHandshakeStall struct {
	mu sync.Mutex
	n  int
}

func (t *alwaysHandshakeStall) RoundTrip(*http.Request) (*http.Response, error) {
	t.mu.Lock()
	t.n++
	t.mu.Unlock()
	return nil, handshakeTimeout()
}
func (t *alwaysHandshakeStall) attempts() int { t.mu.Lock(); defer t.mu.Unlock(); return t.n }
