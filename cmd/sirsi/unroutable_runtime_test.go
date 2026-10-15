package main

import (
	"testing"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/router"
)

func TestFleetRoutabilityUsesLiveConsumer(t *testing.T) {
	now := time.Now().UTC()
	for _, tc := range []struct {
		name        string
		thread      *router.Thread
		unavailable bool
	}{
		{"attended no auto spawn", &router.Thread{Surface: "codex", Status: router.ThreadStatusActive, LastSeenAt: now}, false},
		{"real worker", &router.Thread{Surface: "worker", Status: router.ThreadStatusActive, LastSeenAt: now, ConsumerCapable: true}, false},
		{"watcher is not a consumer", &router.Thread{Surface: "worker", Status: router.ThreadStatusActive, LastSeenAt: now}, true},
		{"stale attended", &router.Thread{Surface: "codex", Status: router.ThreadStatusActive, LastSeenAt: now.Add(-24 * time.Hour)}, true},
		{"closed attended", &router.Thread{Surface: "codex", Status: router.ThreadStatusClosed, LastSeenAt: now}, true},
		{"authentication failure", &router.Thread{Surface: "worker", Status: router.ThreadStatusActive, LastSeenAt: now, ConsumerCapable: true, Lane: &router.LaneState{ConsumerDeclared: true, LastOutcome: router.OutcomeAuthRequired}}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tc.thread.AgentID = "lane"
			threads := &router.ThreadRegistry{Threads: map[string]*router.Thread{"t": tc.thread}}
			for _, mechanism := range []string{router.WakeNone, router.WakeLaunchAgent} {
				reg := &router.Registry{Agents: map[string]router.AgentConfig{"lane": {Wake: router.WakeConfig{Mechanism: mechanism}}}}
				if got := unroutableFromRuntime(reg, threads, now)["lane"]; got != tc.unavailable {
					t.Fatalf("wake=%s unavailable=%v want %v", mechanism, got, tc.unavailable)
				}
			}
		})
	}
}
