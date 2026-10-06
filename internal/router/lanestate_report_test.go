package router

import (
	"testing"
	"time"
)

// A worker's own publication is projected as evidence; a worker that has
// published nothing projects nothing (unknown, never "healthy").
func TestPingProjectsWorkerReportOnlyWhenPublished(t *testing.T) {
	now := time.Date(2026, 10, 6, 4, 0, 0, 0, time.UTC)
	cfg := AgentConfig{ID: "w", Wake: WakeConfig{Mechanism: WakeLaunchAgent}}
	mk := func(lane *LaneState) *ThreadRegistry {
		return &ThreadRegistry{Threads: map[string]*Thread{"t": {ThreadID: "t", AgentID: "w", Surface: "worker", Status: ThreadStatusActive,
			ConsumerCapable: true, LastSeenAt: now, Lane: lane}}}
	}
	got := PingLane(mk(&LaneState{ConsumerDeclared: true, LastOutcome: OutcomeOK, LastDetail: "closed 2 items", PublishedAt: now.Add(-time.Minute)}), cfg, "w", now)
	if got.ReportedAt == "" || got.ReportSummary != OutcomeOK+": closed 2 items" || got.Thread != "t" {
		t.Fatalf("published state must be projected: %+v", got)
	}
	none := PingLane(mk(&LaneState{ConsumerDeclared: true}), cfg, "w", now)
	if none.ReportedAt != "" || none.ReportSummary != "" {
		t.Fatalf("an unpublished worker must project no report: %+v", none)
	}
}
