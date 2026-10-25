package routerstore

import (
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// This owned target accepts TCP but never speaks TLS. Old automatic redirect
// following therefore produces a genuine handshake timeout after the original
// service has already received its POST. Closing and joining the accept loop
// precedes the final connection cleanup; no accepted socket escapes the list.
func stalledRedirectTarget(t *testing.T) (string, *atomic.Int32) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	var count atomic.Int32
	var mu sync.Mutex
	var held []net.Conn
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			mu.Lock()
			held = append(held, conn)
			count.Add(1)
			mu.Unlock()
		}
	}()
	t.Cleanup(func() {
		_ = ln.Close()
		<-done
		mu.Lock()
		for _, conn := range held {
			_ = conn.Close()
		}
		mu.Unlock()
	})
	return "https://" + ln.Addr().String() + "/redirected", &count
}

func TestForwardDoesNotReplayOriginalPOSTAfterRedirect(t *testing.T) {
	for _, status := range []int{http.StatusTemporaryRedirect, http.StatusPermanentRedirect} {
		t.Run(strconv.Itoa(status), func(t *testing.T) {
			target, targetConnections := stalledRedirectTarget(t)
			var originals atomic.Int32
			var authenticatedOriginals atomic.Int32
			service := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				originals.Add(1)
				if r.Method == http.MethodPost && r.Header.Get("Authorization") == "Bearer fixture-only" {
					authenticatedOriginals.Add(1)
				}
				w.Header().Set("Location", target)
				w.WriteHeader(status)
			}))
			t.Cleanup(service.Close)
			client := newRelayHTTPClient()
			client.Timeout = 2 * time.Second
			client.Transport.(*http.Transport).TLSHandshakeTimeout = 50 * time.Millisecond
			t.Cleanup(client.CloseIdleConnections)
			spool := t.TempDir()
			path := seedInflight(t, filepath.Join(spool, "x"), "redirect-x")
			relay := &Relay{Spool: spool, Base: service.URL, Token: "fixture-only", Log: discardLog(), Client: client, now: time.Now}

			result := relay.forward("x", "redirect-x", path)

			if result.Status != status || result.Status == statusHoldForRetry {
				t.Errorf("received original redirect must not become a never-reached hold: got %d, want %d", result.Status, status)
			}
			if got := originals.Load(); got != 1 {
				t.Errorf("original service received %d requests, want exactly one", got)
			}
			if got := authenticatedOriginals.Load(); got != 1 {
				t.Errorf("original authenticated POST count=%d, want1", got)
			}
			if got := targetConnections.Load(); got != 0 {
				t.Errorf("relay connected to redirected target %d times; must not send redirected auth or body", got)
			}
		})
	}
}

func TestForwardRedirectFencePreservesSharedClient(t *testing.T) {
	var policyCalls atomic.Int32
	client := newRelayHTTPClient()
	client.Timeout = 3 * time.Second
	client.CheckRedirect = func(*http.Request, []*http.Request) error {
		policyCalls.Add(1)
		return http.ErrUseLastResponse
	}
	transport := client.Transport
	service := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", "/not-followed")
		w.WriteHeader(http.StatusTemporaryRedirect)
	}))
	t.Cleanup(service.Close)
	t.Cleanup(client.CloseIdleConnections)
	spool := t.TempDir()
	path := seedInflight(t, filepath.Join(spool, "x"), "client-x")
	relay := &Relay{Spool: spool, Base: service.URL, Token: "fixture-only", Log: discardLog(), Client: client, now: time.Now}
	result := relay.forward("x", "client-x", path)
	if result.Status != http.StatusTemporaryRedirect || policyCalls.Load() != 0 {
		t.Fatalf("forward must use its own no-redirect policy, got status%d/shared calls%d", result.Status, policyCalls.Load())
	}
	if client.Timeout != 3*time.Second || client.Transport != transport {
		t.Fatal("forward changed shared timeout or transport/TLS policy")
	}
	if err := client.CheckRedirect(nil, nil); err != http.ErrUseLastResponse || policyCalls.Load() != 1 {
		t.Fatal("forward replaced the shared client's redirect policy")
	}
}
