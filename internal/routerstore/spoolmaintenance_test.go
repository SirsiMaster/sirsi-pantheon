package routerstore

import (
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type maintenanceNoNetwork struct{ calls atomic.Int32 }

func (n *maintenanceNoNetwork) RoundTrip(*http.Request) (*http.Response, error) {
	n.calls.Add(1)
	return nil, errors.New("forbidden maintenance fixture network")
}

func TestRelayMaintenancePrecedesStartupDisposition(t *testing.T) {
	for _, activate := range []bool{false, true} {
		t.Run(map[bool]string{false: "cancel_keeps_inflight", true: "explicit_activation_allows_entry"}[activate], func(t *testing.T) {
			root := t.TempDir()
			inflight := filepath.Join(root, "owned", "inflight")
			if err := os.MkdirAll(inflight, 0700); err != nil {
				t.Fatal(err)
			}
			original := filepath.Join(inflight, "original-id.json")
			payload := []byte(`{"method":"opaque fixture"}`)
			if err := os.WriteFile(original, payload, 0600); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			entered, release, exited := make(chan struct{}), make(chan struct{}), make(chan error, 1)
			transport := &maintenanceNoNetwork{}
			var discovered atomic.Int32
			rl := &Relay{Spool: root, Base: "http://127.0.0.1:1", Token: "inert-owned-token", Client: &http.Client{Transport: transport}}
			rl.maintenanceStartup = func(ctx context.Context) (io.Closer, error) {
				close(entered)
				select {
				case <-ctx.Done():
					return nil, ctx.Err()
				case <-release:
					return io.NopCloser(strings.NewReader("")), nil
				}
			}
			rl.discoverFn = func(context.Context) int { discovered.Add(1); cancel(); return 0 }
			go func() { exited <- rl.Serve(ctx) }()
			defer func() {
				cancel()
				select {
				case <-exited:
				case <-time.After(2 * time.Second):
					t.Error("relay did not join")
				}
			}()
			select {
			case <-entered:
			case <-time.After(2 * time.Second):
				t.Fatal("startup hold not reached")
			}
			if _, err := os.Stat(original); err != nil || discovered.Load() != 0 || transport.calls.Load() != 0 {
				t.Fatalf("work occurred before startup hold: %v", err)
			}
			if activate {
				close(release)
			} else {
				cancel()
			}
			select {
			case err := <-exited:
				// Return the completion for the all-exits joining cleanup above.
				exited <- err
				if !activate && !errors.Is(err, context.Canceled) {
					t.Fatal(err)
				}
			case <-time.After(2 * time.Second):
				t.Fatal("relay did not finish")
			}
			if transport.calls.Load() != 0 {
				t.Fatal("startup replayed remote request")
			}
			if !activate {
				if b, err := os.ReadFile(original); err != nil || string(b) != string(payload) {
					t.Fatal("cancel disposed original inflight")
				}
				if discovered.Load() != 0 {
					t.Fatal("cancel discovered queues")
				}
			}
			if activate && discovered.Load() != 1 {
				t.Fatal("explicit activation did not reach real Serve loop")
			}
		})
	}
}
