package routerstore

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWorkerSessionCacheIsScopedWithoutChangingAuthority(t *testing.T) {
	home, scoped := t.TempDir(), t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("SIRSI_ROUTER_SESSION_DIR", scoped)
	t.Setenv("SIRSI_AGENT_ID", "codex-compasspoint-contract")
	t.Setenv("SIRSI_THREAD_ID", "thr-test-scoped")
	rs := NewRemoteStore("https://canonical.example", "fixture")
	if rs.sessionDir != scoped || rs.agent != "codex-compasspoint-contract" || rs.threadID != "thr-test-scoped" {
		t.Fatalf("scoped cache changed identity: dir=%s agent=%s thread=%s", rs.sessionDir, rs.agent, rs.threadID)
	}
	for _, p := range rs.sessionPaths() {
		if filepath.Dir(p) != scoped {
			t.Fatal("cache escaped own directory")
		}
	}
	if _, err := os.Stat(filepath.Join(home, ".sirsi", "sessions")); !os.IsNotExist(err) {
		t.Fatal("touched shared session directory")
	}
	t.Setenv("SIRSI_ROUTER_SESSION_DIR", "")
	if got := NewRemoteStore("https://canonical.example", "fixture").sessionDir; got != filepath.Join(home, ".sirsi", "sessions") {
		t.Fatalf("legacy default changed: %s", got)
	}
}
