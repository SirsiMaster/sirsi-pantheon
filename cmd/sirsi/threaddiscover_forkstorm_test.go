package main

import (
	"os"
	"strings"
	"testing"
)

// TestDiscoverNeverForksAWatcher guards the fork-storm that took the
// workstation down on 2026-07-27.
//
// `thread discover` adopted each unregistered agent process AND forked a
// `watch-router` bridge for it. watch-router runs the agent's spawn command,
// which starts a NEW agent process — unregistered, so the next discover pass
// adopted THAT and forked again. On the supervisor's cadence the population
// multiplied every pass: 358 `claude` processes, 267 zombies, load average
// 436, swap 48.5 GB of 49 GB, macOS reporting "system has run out of
// application memory".
//
// ADR-024 had already made `register` a pure handshake that RETURNS the
// watcher spec instead of spawning it. discover was the caller that kept the
// old behavior — the same shape as the cutover call site #315 missed.
//
// Source-level because the failure needs a live supervisor, a real process
// table and several cadence ticks to manifest; by the time it is observable
// the machine is already unusable. The invariant worth pinning is simply that
// the router never launches an agent it merely discovered.
func TestDiscoverNeverForksAWatcher(t *testing.T) {
	src, err := os.ReadFile("threaddiscover.go")
	if err != nil {
		t.Fatalf("read threaddiscover.go: %v", err)
	}

	var code []string
	for _, line := range strings.Split(string(src), "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "//") {
			continue // the explanation names the removed symbol
		}
		code = append(code, line)
	}
	body := strings.Join(code, "\n")

	for _, forbidden := range []string{"spawnRouterWatcher", "watch-router"} {
		if strings.Contains(body, forbidden) {
			t.Errorf("thread discover references %q — discover must REGISTER, never arm. "+
				"A discovered process is already running: it needs watching, not launching. "+
				"Forking a watcher here restarts the 2026-07-27 fork storm.", forbidden)
		}
	}

	if !strings.Contains(body, "WatcherFor(") {
		t.Error("discover no longer reports the watcher spec — the surface needs to know what to arm")
	}
}
