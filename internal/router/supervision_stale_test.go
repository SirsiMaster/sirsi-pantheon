package router

import (
	"testing"
	"time"
)

func TestStaleActiveSupervisors(t *testing.T) {
	now := time.Date(2026, 8, 6, 0, 0, 0, 0, time.UTC)
	stale := now.Add(-time.Hour)
	fresh := now.Add(-time.Minute)
	reg := &ThreadRegistry{Threads: map[string]*Thread{
		"automation-stale": {ThreadID: "automation-stale", Surface: "automation", Status: ThreadStatusActive, LastSeenAt: stale},
		"resident-stale":   {ThreadID: "resident-stale", Surface: "worker", WakeMechanism: "resident-loop", Status: ThreadStatusActive, LastSeenAt: stale},
		"codex-stale":      {ThreadID: "codex-stale", Surface: "codex", Status: ThreadStatusActive, LastSeenAt: stale},
		"automation-fresh": {ThreadID: "automation-fresh", Surface: "automation", Status: ThreadStatusActive, LastSeenAt: fresh},
		"automation-idle":  {ThreadID: "automation-idle", Surface: "automation", Status: ThreadStatusIdle, LastSeenAt: stale},
	}}

	got := StaleActiveSupervisors(reg, now, DefaultThreadStaleAfter)
	if len(got) != 2 {
		t.Fatalf("got %d stale active supervisors, want 2: %#v", len(got), got)
	}
	if got[0].ThreadID != "automation-stale" || got[1].ThreadID != "resident-stale" {
		t.Fatalf("ids = %q, %q; want automation-stale, resident-stale", got[0].ThreadID, got[1].ThreadID)
	}
}
