package main

import (
	"testing"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/router"
)

// A service (Horus) and a human have no inbox consumer by design, so neither may
// ever be reported unroutable. A lane with the same shape (wake none, no live
// session) is the control: it must still read unroutable, or this test would pass
// for a function that had simply stopped judging anyone.
func TestServiceAndHumanAreNeverUnroutable(t *testing.T) {
	now := time.Now().UTC()
	reg := &router.Registry{Agents: map[string]router.AgentConfig{
		"horus": {Type: "service", Wake: router.WakeConfig{Mechanism: router.WakeNone}},
		"owner": {Type: "human", Wake: router.WakeConfig{Mechanism: router.WakeNone}},
		"lane":  {Type: "claude", Wake: router.WakeConfig{Mechanism: router.WakeNone}},
	}}
	got := unroutableFromRuntime(reg, &router.ThreadRegistry{Threads: map[string]*router.Thread{}}, now)
	if got["horus"] || got["owner"] {
		t.Fatalf("service/human judged unroutable: horus=%v owner=%v", got["horus"], got["owner"])
	}
	if !got["lane"] {
		t.Fatal("control failed: a wake-none lane with no live session must read unroutable")
	}
}

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
