package router

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/routercfg"
	"github.com/SirsiMaster/sirsi-pantheon/internal/routerstore"
)

func TestCloudHostRegistrationHeartbeatClaimCloseRoundTrip120(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv(routercfg.StoreWakeEnv, "1")
	t.Setenv("SIRSI_ROUTER_DB", "")
	t.Setenv(routerstore.EnvUseMachineIDHost, "")
	t.Setenv(routerstore.EnvCloudHost, "cloud-ssa-test")
	t.Setenv("SIRSI_AGENT_ID", "sirsi-software-admin")
	t.Setenv("SIRSI_THREAD_ID", "")
	backend, err := routerstore.OpenPath(filepath.Join(t.TempDir(), "fixture.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer backend.Close()
	token, _, err := backend.MintScopedHostToken("cloud-ssa-test", "fixture", []string{"sirsi-software-admin"})
	if err != nil {
		t.Fatal(err)
	}
	h, err := routerstore.Handler(backend, routerstore.ServerOptions{Token: "fixture-admin", MaxWait: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(h)
	defer srv.Close()
	t.Setenv("SIRSI_ROUTER_URL", srv.URL)
	t.Setenv("SIRSI_ROUTER_TOKEN", token)
	root := t.TempDir()
	thread, err := RegisterThread(root, &Thread{ThreadID: "ssa-cloud-fixture", AgentID: "sirsi-software-admin", Surface: "codex", PID: os.Getpid()})
	if err != nil {
		t.Fatal(err)
	}
	if thread.Host != "cloud-ssa-test" {
		t.Fatalf("host = %q", thread.Host)
	}
	reg, err := LoadThreadRegistry(root)
	if err != nil {
		t.Fatal(err)
	}
	if reg.Threads[thread.ThreadID] == nil || reg.Threads[thread.ThreadID].Host != thread.Host {
		t.Fatal("registration silently lost")
	}

	// Heartbeat must save under the same logical ownership as registration.
	if _, err := Heartbeat(root, thread.ThreadID, HeartbeatUpdate{}); err != nil {
		t.Fatalf("heartbeat: %v", err)
	}
	t.Setenv("SIRSI_THREAD_ID", thread.ThreadID)
	store, err := routerstore.Resolve()
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err := store.AddTask(routerstore.Task{Agent: "sirsi-software-admin", TaskID: "cloud-roundtrip", Subject: "fixture", Status: "pending", ResponsibleParty: "sirsi-software-admin"}); err != nil {
		t.Fatal(err)
	}
	lease, err := store.ClaimTask("sirsi-software-admin", "cloud-roundtrip", "fixture", thread.ThreadID, time.Minute)
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	held, err := store.TaskEligibility("sirsi-software-admin", "cloud-roundtrip")
	if err != nil || !held.LeaseHeld || held.ThreadID != thread.ThreadID {
		t.Fatal("claim ownership not retained", err)
	}
	if err := store.CompleteTaskLease("sirsi-software-admin", "cloud-roundtrip", lease.Token, "fixture-result"); err != nil {
		t.Fatalf("complete: %v", err)
	}
	if _, err := CloseThread(root, thread.ThreadID); err != nil {
		t.Fatalf("close: %v", err)
	}
	rows, err := backend.ListThreads()
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, row := range rows {
		if row.ThreadID == thread.ThreadID {
			found = true
			var payload Thread
			if err := json.Unmarshal(row.Payload, &payload); err != nil {
				t.Fatal(err)
			}
			if row.Host != "cloud-ssa-test" || row.Status != "closed" || payload.Host != row.Host {
				t.Fatal("closed record ownership mismatch")
			}
		}
	}
	if !found {
		t.Fatal("closed record absent")
	}
	// A session carrying a different logical host must still be refused by server.
	t.Setenv(routerstore.EnvCloudHost, "cloud-foreign")
	t.Setenv("HOME", t.TempDir()) // force a fresh mint rather than reusing an existing scoped session
	foreign := routerstore.NewRemoteStore(srv.URL, token)
	if _, err := foreign.Inbox("sirsi-software-admin"); err == nil {
		t.Fatal("foreign scoped host accepted")
	}
	t.Setenv(routerstore.EnvCloudHost, "cloud-ssa-test")
	if _, err := RegisterThread(root, &Thread{ThreadID: "foreign-host", AgentID: "sirsi-software-admin", Surface: "codex", Host: "cloud-ra"}); err == nil {
		t.Fatal("foreign cloud host accepted")
	}
	if _, err := RegisterThread(root, &Thread{ThreadID: "foreign-lane", AgentID: "ra", Surface: "codex"}); err == nil {
		t.Fatal("foreign lane accepted")
	}
}
