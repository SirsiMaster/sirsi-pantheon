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

	if _, err := router.RegisterThread(root, &router.Thread{AgentID: "claude-home", Surface: "claude", PID: 6001, StartTime: "loop-dead"}); err != nil {
		t.Fatal(err)
	}
	if wakeInstallBlocked(root, "claude-home", false) {
		t.Fatal("live-but-loop-dead Claude session must not block wake-install")
	}

	if _, err := router.RegisterThread(root, &router.Thread{AgentID: "claude-home", Surface: "worker", PID: 6002, StartTime: "armed"}); err != nil {
		t.Fatal(err)
	}
	if !wakeInstallBlocked(root, "claude-home", false) {
		t.Fatal("armed watcher must block duplicate wake-install")
	}
	if wakeInstallBlocked(root, "claude-home", true) {
		t.Fatal("--force must bypass the duplicate-spawn guard")
	}
}
