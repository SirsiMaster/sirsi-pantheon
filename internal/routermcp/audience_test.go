package routermcp

import (
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/router"
	"github.com/SirsiMaster/sirsi-pantheon/internal/routercfg"
	"github.com/SirsiMaster/sirsi-pantheon/internal/routerstore"
)

// This uses real registration/CAS and the real authenticated HTTP handler on
// owned SQLite, not a mock register returning two invented thread strings.
func TestRealSamePIDInstancesBindIndependentRemoteAudiences(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	for _, key := range []string{"SIRSI_ROUTER_URL", "SIRSI_ROUTER_TOKEN", "SIRSI_THREAD_ID", "SIRSI_ROUTER_USE_MACHINE_ID", "SIRSI_ROUTER_CLOUD_HOST", "SIRSI_TEST_PG_DSN"} {
		t.Setenv(key, "")
	}
	t.Setenv(routercfg.StoreWakeEnv, "1")
	t.Setenv("SIRSI_ALLOW_SCHEMA_MIGRATE", "1")
	t.Setenv("SIRSI_AGENT_ID", "owned-mcp")
	path := filepath.Join(t.TempDir(), "router.db")
	t.Setenv("SIRSI_ROUTER_DB", path)
	backend, err := routerstore.OpenPath(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := backend.Close(); err != nil {
			t.Error(err)
		}
	})
	handler, err := routerstore.Handler(backend, routerstore.ServerOptions{Token: "owned-fixture-token", RuleOfRa: "enforce"})
	if err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	var lastSession string
	sessions := map[string]string{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		sessions[r.URL.Path] = r.Header.Get("X-Sirsi-Session")
		if r.URL.Path == "/v1/call/Inbox" {
			lastSession = r.Header.Get("X-Sirsi-Session")
		}
		mu.Unlock()
		handler.ServeHTTP(w, r)
	}))
	t.Cleanup(srv.Close)
	t.Setenv("SIRSI_ROUTER_URL", srv.URL)
	t.Setenv("SIRSI_ROUTER_TOKEN", "owned-fixture-token")
	repo := t.TempDir()
	root := filepath.Join(repo, ".agents", "idea-router")
	if err := os.MkdirAll(root, 0700); err != nil {
		t.Fatal(err)
	}
	if err := router.SaveRegistry(root, &router.Registry{Agents: map[string]router.AgentConfig{"owned-mcp": {ID: "owned-mcp"}}}); err != nil {
		t.Fatal(err)
	}
	a, b := newServer("owned-mcp", repo), newServer("owned-mcp", repo)
	host, err := os.Hostname()
	if err != nil {
		t.Fatal(err)
	}
	a.deps.identity = func() (string, string) { return host, "owned-fixture-machine" }
	b.deps.identity = a.deps.identity
	// Keep real register/close, but no timers or heartbeat probes in this fixture.
	a.deps.ticker = func() (<-chan time.Time, func()) { return nil, func() {} }
	b.deps.ticker = a.deps.ticker
	stopA := a.registerSurface(log.New(io.Discard, "", 0))
	t.Cleanup(func() {
		if err := stopA(); err != nil {
			t.Error(err)
		}
	})
	stopB := b.registerSurface(log.New(io.Discard, "", 0))
	t.Cleanup(func() {
		if err := stopB(); err != nil {
			t.Error(err)
		}
	})
	if _, err := a.reg.gate(); err != nil {
		t.Fatal(err)
	}
	if _, err := b.reg.gate(); err != nil {
		t.Fatal(err)
	}
	if a.reg.threadID == b.reg.threadID {
		t.Fatal("same PID instances reused a registered thread")
	}
	for _, s := range []*Server{a, b} {
		record, err := backend.GetThreadRecord(s.reg.threadID)
		if err != nil {
			t.Fatal(err)
		}
		var thread router.Thread
		if err := json.Unmarshal(record.Payload, &thread); err != nil {
			t.Fatal(err)
		}
		if thread.PID != os.Getpid() || thread.ThreadID != s.reg.threadID {
			t.Fatal("not two actual same-PID registrations")
		}
	}
	// Deliberately contradict both ambient identity inputs AFTER registration.
	// Explicit local binding must win without rewriting those inputs.
	t.Setenv("SIRSI_AGENT_ID", "foreign-ambient-agent")
	t.Setenv("SIRSI_THREAD_ID", "foreign-ambient-thread")
	now := time.Now().UTC()
	for _, item := range []routerstore.Item{
		{ID: "owned-board-open", From: "fixture-sender", To: "board-peer", Title: "board package", Type: "proposal", Status: "open", Opened: now.Add(-time.Hour).Format(time.RFC3339)},
		{ID: "owned-board-closed", From: "fixture-sender", To: "board-peer", Title: "board pace", Type: "proposal", Status: "closed", Opened: now.Add(-6 * time.Hour).Format(time.RFC3339), Closed: now.Add(-2 * time.Hour).Format(time.RFC3339)},
	} {
		if err := backend.Put(item); err != nil {
			t.Fatal(err)
		}
	}
	for _, instance := range []*Server{a, b} {
		mu.Lock()
		delete(sessions, "/v1/call/ListSinceHeaders")
		delete(sessions, "/v1/call/ListThreads")
		mu.Unlock()
		result, err := instance.handleBoard(nil)
		if err != nil || result == nil || result.IsError {
			t.Fatalf("actual board handler refused: %v %v", result, err)
		}
		var board router.WorkBoard
		found := false
		for _, content := range result.Content {
			if _, raw, ok := strings.Cut(content.Text, "\nJSON:\n"); ok {
				if err := json.Unmarshal([]byte(raw), &board); err != nil {
					t.Fatal(err)
				}
				found = true
			}
		}
		if !found || board.TotalOpen != 1 || board.ClosedToday != 1 || board.Closed7d != 1 || board.AvgCloseHours != 4 {
			t.Fatalf("board packages/pace changed: %+v", board)
		}
		live := false
		for _, agent := range board.Agents {
			if agent.AgentID == "owned-mcp" && agent.Live && len(agent.Surfaces) == 1 && agent.Surfaces[0] == "mcp" {
				live = true
			}
		}
		if !live {
			t.Fatal("bound registry peers disappeared from actual board")
		}
		for _, method := range []string{"ListSinceHeaders", "ListThreads"} {
			mu.Lock()
			sid := sessions["/v1/call/"+method]
			mu.Unlock()
			session, err := backend.GetSession(sid)
			if err != nil || session.Agent != "owned-mcp" || session.ThreadID != instance.ownedThread {
				t.Fatalf("board %s used wrong audience: agent=%q thread=%q error=%v", method, session.Agent, session.ThreadID, err)
			}
		}
	}
	fa, closeA, err := a.openFacade()
	if err != nil {
		t.Fatal(err)
	}
	defer closeA()
	fb, closeB, err := b.openFacade()
	if err != nil {
		t.Fatal(err)
	}
	defer closeB()
	for i, f := range []routerstore.Store{fa.Store(), fb.Store()} {
		if _, err := f.Inbox("owned-mcp"); err != nil {
			t.Fatal(err)
		}
		mu.Lock()
		sid := lastSession
		mu.Unlock()
		sess, err := backend.GetSession(sid)
		if err != nil {
			t.Fatal(err)
		}
		want := []*Server{a, b}[i]
		if sess.Agent != "owned-mcp" || sess.ThreadID != want.reg.threadID {
			t.Fatalf("wrong actual RPC audience: agent=%q thread=%q", sess.Agent, sess.ThreadID)
		}
	}
	id, err := backend.Send("fixture-sender", "owned-mcp", "owned work", "proposal", "inert owned work")
	if err != nil {
		t.Fatal(err)
	}
	lease, err := fa.Store().ClaimNext("owned-mcp", time.Minute)
	if err != nil || lease == nil {
		t.Fatalf("claim: %v %v", lease, err)
	}
	if lease.ItemID != id {
		t.Fatal("wrong fixture item")
	}
	if err := fb.Store().RenewLease(id, lease.Token, time.Minute); !errors.Is(err, routerstore.ErrNotOwner) {
		t.Fatalf("other instance used A's lease: %v", err)
	}
	if err := fa.Store().RenewLease(id, lease.Token, time.Minute); err != nil {
		t.Fatalf("own audience lost lease: %v", err)
	}
	wrong, err := routerstore.ResolveForCaller("wrong-agent", a.reg.threadID)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = wrong.Close() }()
	if _, _, err := wrong.SendGuarded(routerstore.SendReq{From: "wrong-agent", To: "owned-mcp", Title: "must refuse", Type: "proposal", Instructions: "inert"}); !errors.Is(err, routerstore.ErrUnregistered) {
		t.Fatalf("wrong audience accepted: %v", err)
	}
	aID, bID := a.reg.threadID, b.reg.threadID
	if err := stopA(); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	closeSession := sessions["/v1/call/UpsertThreadCAS"]
	mu.Unlock()
	closedCaller, err := backend.GetSession(closeSession)
	if err != nil || closedCaller.ThreadID != aID {
		t.Fatalf("close used ambient audience: thread=%q error=%v", closedCaller.ThreadID, err)
	}
	beatB, _ := router.CallerThreadLifecycle("owned-mcp", bID)
	if _, err := beatB(root, bID, router.HeartbeatUpdate{Status: router.ThreadStatusActive}); err != nil {
		t.Fatalf("A close broke B heartbeat: %v", err)
	}
	mu.Lock()
	beatSession := sessions["/v1/call/UpsertThreadCAS"]
	mu.Unlock()
	beatCaller, err := backend.GetSession(beatSession)
	if err != nil || beatCaller.ThreadID != bID {
		t.Fatalf("heartbeat used ambient audience: thread=%q error=%v", beatCaller.ThreadID, err)
	}
	if _, err := beatB(root, aID, router.HeartbeatUpdate{Status: router.ThreadStatusActive}); err == nil {
		t.Fatal("bound lifecycle accepted another instance's target")
	}
	for _, want := range []struct{ id, status string }{{aID, "closed"}, {bID, "active"}} {
		record, err := backend.GetThreadRecord(want.id)
		if err != nil || record.Status != want.status {
			t.Fatalf("instance lifecycle state = %+v %v", record, err)
		}
	}
	if _, _, err := fa.Store().SendGuarded(routerstore.SendReq{From: "owned-mcp", To: "fixture-sender", Title: "must refuse", Type: "proposal", Instructions: "inert"}); !errors.Is(err, routerstore.ErrUnregistered) {
		t.Fatalf("closed audience mutated: %v", err)
	}
	if _, err := fb.Store().Inbox("owned-mcp"); err != nil {
		t.Fatalf("closing A broke B: %v", err)
	}
	if os.Getenv("SIRSI_AGENT_ID") != "foreign-ambient-agent" || os.Getenv("SIRSI_THREAD_ID") != "foreign-ambient-thread" {
		t.Fatal("instance changed process identity")
	}
}
