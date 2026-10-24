package routerstore

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveForCallerCopiesExactBindingWithoutAmbientMutation(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("SIRSI_AGENT_ID", "ambient-agent")
	t.Setenv("SIRSI_THREAD_ID", "ambient-thread")
	t.Setenv("SIRSI_ROUTER_TOKEN", "")
	t.Setenv("SIRSI_ROUTER_DB", "")
	spool := filepath.Join(t.TempDir(), "spool")
	setHomeWithMarker(t, "export SIRSI_ROUTER_URL='spool://"+spool+"'\n")
	store, err := ResolveForCaller("own-agent", "own-thread")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()
	rs, ok := store.(*RemoteStore)
	if !ok || rs.agent != "own-agent" || rs.threadID != "own-thread" {
		t.Fatalf("explicit binding lost: %#v", store)
	}
	transport, ok := rs.client.Transport.(*spoolTransport)
	if !ok || transport.agent != "own-agent" || transport.root != spool {
		t.Fatal("spool transport built for ambient caller")
	}
	if rs.session.ID != "" {
		t.Fatal("constructor minted a session")
	}
	if rs.sessionDir != "" {
		t.Fatal("explicit caller can touch ambient session cache")
	}
	if os.Getenv("SIRSI_AGENT_ID") != "ambient-agent" || os.Getenv("SIRSI_THREAD_ID") != "ambient-thread" || os.Getenv("SIRSI_ROUTER_URL") != "" {
		t.Fatal("resolver mutated environment")
	}
	if _, err := os.Stat(spool); !os.IsNotExist(err) {
		t.Fatalf("constructor touched spool: %v", err)
	}
}

func TestResolveForCallerRefusesMissingBindingBeforeStoreOpen(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("SIRSI_ROUTER_URL", "")
	path := filepath.Join(t.TempDir(), "never-created", "router.db")
	t.Setenv("SIRSI_ROUTER_DB", path)
	for _, c := range [][2]string{{"", "thread"}, {"agent", ""}, {" ", "thread"}} {
		if store, err := ResolveForCaller(c[0], c[1]); err == nil || store != nil {
			t.Fatal("missing binding opened store")
		}
	}
	if _, err := os.Stat(filepath.Dir(path)); !os.IsNotExist(err) {
		t.Fatalf("invalid caller created local state: %v", err)
	}
}
