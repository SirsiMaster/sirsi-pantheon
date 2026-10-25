package routerstore

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// realHandshakeLeaf returns the genuine error net/http produces when a TLS
// handshake stalls: a TCP listener accepts but never speaks TLS, and the client's
// handshake timeout fires. It returns the Transport-level leaf (what a
// RoundTripper returns); http.Client.Do wraps it in a *url.Error.
func realHandshakeLeaf(t *testing.T) error {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	var held []net.Conn
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			mu.Lock()
			held = append(held, c)
			mu.Unlock()
		}
	}()
	defer func() {
		_ = ln.Close()
		mu.Lock()
		for _, c := range held {
			_ = c.Close()
		}
		mu.Unlock()
	}()
	c := &http.Client{Transport: &http.Transport{TLSHandshakeTimeout: 100 * time.Millisecond, DisableKeepAlives: true}}
	_, err = c.Get("https://" + ln.Addr().String() + "/")
	var ue *url.Error
	if !errors.As(err, &ue) || ue.Err == nil || ue.Err.Error() != "net/http: TLS handshake timeout" {
		t.Fatalf("could not produce a real TLS handshake timeout, got %v", err)
	}
	return ue.Err
}

// handshakeTimeout is the exact shape http.Client.Do returns for a stalled handshake.
func handshakeTimeout(t *testing.T) error {
	return &url.Error{Op: "Post", URL: "https://svc.invalid/v1/call/MaatRead", Err: realHandshakeLeaf(t)}
}

// A stalled TLS handshake happens before any request byte is written, so the
// request never reached the service. Only the real net/http leaf counts; text
// that merely mentions the phrase is post-send or ambiguous and stays UNKNOWN.
func TestNeverReachedServiceClassifiesTLSHandshakeTimeout(t *testing.T) {
	const phrase = "net/http: TLS handshake timeout"
	headers := errors.New("net/http: timeout awaiting response headers")
	for _, tc := range []struct {
		name string
		err  error
		want bool
	}{
		{"real handshake timeout is never-reached", handshakeTimeout(t), true},
		// Negative controls: post-send, or not provably pre-send.
		{"response header timeout is post-send", &url.Error{Op: "Post", URL: "https://x", Err: headers}, false},
		{"response header timeout with misleading wrapper text", &url.Error{Op: "Post", URL: "https://x", Err: fmt.Errorf("%s (retrying): %w", phrase, headers)}, false},
		{"misleading URL text", &url.Error{Op: "Post", URL: "https://x/" + url.PathEscape(phrase), Err: headers}, false},
		{"longer leaf containing the phrase", &url.Error{Op: "Post", URL: "https://x", Err: errors.New(phrase + " after request write")}, false},
		{"exact phrase but not the net/http leaf", &url.Error{Op: "Post", URL: "https://x", Err: errors.New(phrase)}, false},
		{"joined: handshake plus post-send cause", errors.Join(handshakeTimeout(t), headers), false},
		{"url error around a joined cause", &url.Error{Op: "Post", URL: "https://x", Err: errors.Join(realHandshakeLeaf(t), headers)}, false},
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
	rt := &halfOpenTransport{firstErr: realHandshakeLeaf(t)}
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
	rt := &alwaysHandshakeStall{leaf: realHandshakeLeaf(t)}
	rl := &Relay{Spool: spool, Base: "http://svc.invalid", Token: "t", Log: discardLog(), Client: &http.Client{Transport: rt}, now: time.Now}

	sr := rl.forward("x", "011-x", p)

	if sr.Status != statusHoldForRetry {
		t.Fatalf("a persistent handshake stall must HOLD for retry (%d), got %d body=%s", statusHoldForRetry, sr.Status, sr.Body)
	}
	if rt.attempts() != 2 {
		t.Fatalf("want exactly one safe re-dial (2 attempts), got %d", rt.attempts())
	}
}

// Negative controls at the forward level: a timeout awaiting response headers
// means the request WAS sent, with or without misleading text naming a handshake.
// It must not be retried (the service may have committed) and stays OUTCOME UNKNOWN.
func TestForwardDoesNotRetryResponseHeaderTimeout(t *testing.T) {
	headers := errors.New("net/http: timeout awaiting response headers")
	for i, leaf := range []error{
		headers,
		fmt.Errorf("net/http: TLS handshake timeout (retrying): %w", headers),
		errors.New("net/http: TLS handshake timeout"),
	} {
		spool := t.TempDir()
		id := fmt.Sprintf("01%d-x", 2+i)
		p := seedInflight(t, filepath.Join(spool, "x"), id)
		rt := &halfOpenTransport{firstErr: leaf}
		rl := &Relay{Spool: spool, Base: "http://svc.invalid", Token: "t", Log: discardLog(), Client: &http.Client{Transport: rt}, now: time.Now}

		sr := rl.forward("x", id, p)

		if sr.Status != http.StatusBadGateway || !strings.Contains(string(sr.Body), "OUTCOME UNKNOWN") {
			t.Fatalf("%q: post-send timeout must stay OUTCOME UNKNOWN, got status %d body=%s", leaf, sr.Status, sr.Body)
		}
		if attempts, _ := rt.count(); attempts != 1 {
			t.Fatalf("%q: post-send timeout must not be retried, got %d attempts", leaf, attempts)
		}
	}
}

type alwaysHandshakeStall struct {
	mu   sync.Mutex
	n    int
	leaf error
}

func (t *alwaysHandshakeStall) RoundTrip(*http.Request) (*http.Response, error) {
	t.mu.Lock()
	t.n++
	t.mu.Unlock()
	return nil, t.leaf
}
func (t *alwaysHandshakeStall) attempts() int { t.mu.Lock(); defer t.mu.Unlock(); return t.n }
