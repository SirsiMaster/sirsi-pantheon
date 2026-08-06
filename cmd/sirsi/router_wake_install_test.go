package main

import (
	"testing"

	"github.com/SirsiMaster/sirsi-pantheon/internal/router"
	"github.com/SirsiMaster/sirsi-pantheon/internal/routercfg"
)

func TestWakeInstallBlockedUsesArmedWatcher(t *testing.T) {
	t.Setenv(routercfg.StoreWakeEnv, "0")
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
