package main

import (
	"path/filepath"
	"testing"

	"github.com/SirsiMaster/sirsi-pantheon/internal/router"
)

func TestWakeInstallBlockedUsesArmedWatcher(t *testing.T) {
	t.Setenv("SIRSI_ROUTER_DB", filepath.Join(t.TempDir(), "router.db"))
	t.Setenv("SIRSI_ALLOW_SCHEMA_MIGRATE", "1")
	root := t.TempDir()

	// Use a test-scoped sentinel agent ID so the WatcherAliveByAgent pgrep never
	// matches a real host process (e.g. ~/.sirsi/watchers/claude-home-watcher.sh).
	// The test's semantics do not require a real agent name — it only verifies the
	// wakeInstallBlocked logic (#614 host-state pollution fix).
	const testAgent = "test-wakeinstall-sentinel-x9q"

	if _, err := router.RegisterThread(root, &router.Thread{AgentID: testAgent, Surface: "claude", PID: 6001, StartTime: "loop-dead"}); err != nil {
		t.Fatal(err)
	}
	if wakeInstallBlocked(root, testAgent, false) {
		t.Fatal("live-but-loop-dead Claude session must not block wake-install")
	}

	if _, err := router.RegisterThread(root, &router.Thread{AgentID: testAgent, Surface: "worker", PID: 6002, StartTime: "armed"}); err != nil {
		t.Fatal(err)
	}
	if !wakeInstallBlocked(root, testAgent, false) {
		t.Fatal("armed watcher must block duplicate wake-install")
	}
	if wakeInstallBlocked(root, testAgent, true) {
		t.Fatal("--force must bypass the duplicate-spawn guard")
	}
}
