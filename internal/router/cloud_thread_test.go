package router

import (
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/routercfg"
	"github.com/SirsiMaster/sirsi-pantheon/internal/routerstore"
)

func TestSSACloudThreadRegistrationPersistsScopedHost(t *testing.T) {
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
	thread, err := RegisterThread(root, &Thread{ThreadID: "ssa-cloud-fixture", AgentID: "sirsi-software-admin", Surface: "codex"})
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
	if _, err := RegisterThread(root, &Thread{ThreadID: "foreign-host", AgentID: "sirsi-software-admin", Surface: "codex", Host: "cloud-ra"}); err == nil {
		t.Fatal("foreign cloud host accepted")
	}
	if _, err := RegisterThread(root, &Thread{ThreadID: "foreign-lane", AgentID: "ra", Surface: "codex"}); err == nil {
		t.Fatal("foreign lane accepted")
	}
}
