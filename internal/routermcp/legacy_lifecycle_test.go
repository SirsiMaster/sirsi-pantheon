package routermcp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/router"
	"github.com/SirsiMaster/sirsi-pantheon/internal/routercfg"
)

// JSON remains a compatibility registry, not authenticated durable isolation.
// These fixtures intentionally do not create/open a router database. Any HTTP
// submission is counted/refused by the owned inert listener, never a live URL.
func TestRealLegacyRegistrationHeartbeatEOFClosePreservesSibling(t *testing.T) {
	for _, mode := range []string{"no-store", "explicit-sandbox-and-service"} {
		t.Run(mode, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			for _, key := range []string{"SIRSI_ROUTER_URL", "SIRSI_ROUTER_TOKEN", "SIRSI_ROUTER_DB", "SIRSI_THREAD_ID", "SIRSI_ROUTER_CLOUD_HOST", "SIRSI_ROUTER_USE_MACHINE_ID"} {
				t.Setenv(key, "")
			}
			t.Setenv(routercfg.StoreWakeEnv, "0")
			t.Setenv("SIRSI_AGENT_ID", "owned-json")
			var calls atomic.Int32
			sandbox := filepath.Join(t.TempDir(), "must-not-open.db")
			if mode == "explicit-sandbox-and-service" {
				srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					calls.Add(1)
					http.Error(w, "unexpected fixture submission", http.StatusServiceUnavailable)
				}))
				t.Cleanup(srv.Close)
				t.Setenv("SIRSI_ROUTER_URL", srv.URL)
				t.Setenv("SIRSI_ROUTER_TOKEN", "owned-inert-token")
				t.Setenv("SIRSI_ROUTER_DB", sandbox)
			}
			repo := t.TempDir()
			root := filepath.Join(repo, ".agents", "idea-router")
			if err := os.MkdirAll(filepath.Join(root, "items"), 0700); err != nil {
				t.Fatal(err)
			}
			// A real sibling record must survive registration, heartbeat and EOF.
			sibling, err := router.RegisterThread(root, &router.Thread{ThreadID: router.NewThreadID(), AgentID: "owned-sibling", Surface: "mcp", PID: os.Getpid(), Host: "owned-json-host", MachineID: "owned-json-machine", WakeMechanism: "none"})
			if err != nil {
				t.Fatal(err)
			}
			before, _ := json.Marshal(sibling)
			opened := time.Now().UTC().Add(-time.Hour).Format(time.RFC3339)
			item := fmt.Sprintf("---\nfrom: fixture\nto: owned-json\ntitle: legacy package\nstatus: open\nopened: %s\n---\n\nowned fixture\n", opened)
			if err := os.WriteFile(filepath.Join(root, "items", "owned-legacy-item.md"), []byte(item), 0600); err != nil {
				t.Fatal(err)
			}
			s := newServer("owned-json", repo)
			s.deps.identity = func() (string, string) { return "owned-json-host", "owned-json-machine" }
			registered := make(chan *router.Thread, 1)
			s.deps.register = func(root string, tr *router.Thread) (*router.Thread, error) {
				actual, err := router.RegisterThread(root, tr)
				if err == nil {
					copy := *actual
					registered <- &copy
				}
				return actual, err
			}
			ticks := make(chan time.Time, 1)
			s.deps.ticker = func() (<-chan time.Time, func()) { return ticks, func() {} }
			beaten := make(chan error, 1)
			s.deps.bind = func(agent, id string) (func(string, string, router.HeartbeatUpdate) (*router.Thread, error), func(string, string) (*router.Thread, error)) {
				beat, closeThread := router.CallerThreadLifecycle(agent, id)
				return func(root, id string, update router.HeartbeatUpdate) (*router.Thread, error) {
					actual, err := beat(root, id, update)
					beaten <- err
					return actual, err
				}, closeThread
			}
			reader, writer := io.Pipe()
			ctx, cancel := context.WithCancel(context.Background())
			exited := make(chan struct{})
			var runErr error
			go func() { defer close(exited); runErr = s.Run(ctx, reader, io.Discard) }()
			t.Cleanup(func() {
				cancel()
				_ = writer.Close()
				_ = reader.Close()
				select {
				case <-exited:
				case <-time.After(2 * time.Second):
					t.Error("owned JSON lifecycle did not join")
				}
			})
			var actual *router.Thread
			select {
			case actual = <-registered:
			case <-time.After(2 * time.Second):
				t.Fatal("no real JSON registration")
			}
			if actual.ThreadID != s.ownedThread || actual.ThreadID == sibling.ThreadID {
				t.Fatal("legacy registration did not retain exact owned ID")
			}
			ticks <- time.Now()
			select {
			case err := <-beaten:
				if err != nil {
					t.Fatal(err)
				}
			case <-time.After(2 * time.Second):
				t.Fatal("no real JSON heartbeat")
			}
			reg, err := router.LoadThreadRegistry(root)
			if err != nil {
				t.Fatal(err)
			}
			if tr := reg.Threads[actual.ThreadID]; tr == nil || tr.Status != router.ThreadStatusActive || !tr.LastSeenAt.After(actual.LastSeenAt) {
				t.Fatal("heartbeat did not refresh actual JSON row")
			}
			// Actual board must stay on the file readers, even with a service URL.
			board, err := s.handleBoard(nil)
			if err != nil || board == nil || board.IsError || len(board.Content) == 0 || !strings.Contains(board.Content[0].Text, "legacy package") {
				t.Fatalf("legacy board did not retain file mode: %v %v", board, err)
			}
			beat, closeThread := router.CallerThreadLifecycle("owned-json", actual.ThreadID)
			if _, err := beat(root, sibling.ThreadID, router.HeartbeatUpdate{}); err == nil {
				t.Fatal("legacy heartbeat accepted sibling target")
			}
			if _, err := closeThread(root, sibling.ThreadID); err == nil {
				t.Fatal("legacy close accepted sibling target")
			}
			if err := writer.Close(); err != nil {
				t.Fatal(err)
			}
			select {
			case <-exited:
				if runErr != nil {
					t.Fatal(runErr)
				}
			case <-time.After(2 * time.Second):
				t.Fatal("EOF did not close real JSON registration")
			}
			reg, err = router.LoadThreadRegistry(root)
			if err != nil {
				t.Fatal(err)
			}
			if tr := reg.Threads[actual.ThreadID]; tr == nil || tr.Status != router.ThreadStatusClosed {
				t.Fatal("EOF left owned JSON row live")
			}
			after, _ := json.Marshal(reg.Threads[sibling.ThreadID])
			if string(before) != string(after) {
				t.Fatal("sibling JSON record changed")
			}
			if calls.Load() != 0 {
				t.Fatal("legacy lifecycle or board submitted a durable RPC")
			}
			if _, err := os.Stat(sandbox); !os.IsNotExist(err) {
				t.Fatalf("legacy mode created durable database: %v", err)
			}
		})
	}
}
