package main

import "testing"

// Router item 20260729-134030: `sirsi ccd reap` reported "exactly 2 completed-leak
// sessions per run, deterministic" for what is ONE leaked session. The app runs
// every headless session as `disclaimer <claude-path> …`, so the shim's argv
// contains the claude path and the reaper's pgrep matched both the shim and its
// child; both attributed to the same session record.

func TestDedupeSessionProcsCollapsesShimAndChild(t *testing.T) {
	// 48892 = disclaimer shim, 48893 = its claude child, same session.
	reap := []reapT{
		{pid: 48892, s: ccdSession{sid: "sess-a", sched: "router-conduit-supervisor"}, idle: 15},
		{pid: 48893, s: ccdSession{sid: "sess-a", sched: "router-conduit-supervisor"}, idle: 15},
	}
	ppid := map[int]int{48892: 2425, 48893: 48892} // shim's parent is the app, not a candidate

	got := dedupeSessionProcs(reap, func(p int) int { return ppid[p] })
	if len(got) != 1 {
		t.Fatalf("one leaked session must count once, got %d entries", len(got))
	}
	// The PARENT must survive: the kill path sweeps children via pgrep -P, so
	// keeping the child instead would orphan the shim.
	if got[0].pid != 48892 {
		t.Fatalf("dedupe must keep the parent-most pid (48892), kept %d", got[0].pid)
	}
}

func TestDedupeSessionProcsKeepsGenuinelySeparateSessions(t *testing.T) {
	// Two distinct leaked sessions, each a shim+child pair → must stay 2.
	reap := []reapT{
		{pid: 100, s: ccdSession{sid: "sess-a"}}, {pid: 101, s: ccdSession{sid: "sess-a"}},
		{pid: 200, s: ccdSession{sid: "sess-b"}}, {pid: 201, s: ccdSession{sid: "sess-b"}},
	}
	ppid := map[int]int{100: 1, 101: 100, 200: 1, 201: 200}

	got := dedupeSessionProcs(reap, func(p int) int { return ppid[p] })
	if len(got) != 2 {
		t.Fatalf("two distinct sessions must remain two, got %d", len(got))
	}
	seen := map[string]int{}
	for _, r := range got {
		seen[r.s.sid] = r.pid
	}
	if seen["sess-a"] != 100 || seen["sess-b"] != 200 {
		t.Fatalf("each session must keep its own parent, got %v", seen)
	}
}

// The dedupe must key on "parent is another CANDIDATE for the same session" —
// not merely "a parent exists", which is true of every process, and not on pid
// adjacency, which is a coincidence of spawn order rather than a relationship.
func TestDedupeSessionProcsDoesNotDropUnrelatedPids(t *testing.T) {
	reap := []reapT{
		{pid: 500, s: ccdSession{sid: "sess-a"}},
		{pid: 501, s: ccdSession{sid: "sess-b"}}, // adjacent pid, DIFFERENT session
	}
	// 501's parent happens to be 500 — but they are different sessions, so
	// dropping 501 would silently leave a real leak unreaped.
	ppid := map[int]int{500: 1, 501: 500}

	got := dedupeSessionProcs(reap, func(p int) int { return ppid[p] })
	if len(got) != 2 {
		t.Fatalf("pids in different sessions must never collapse, got %d", len(got))
	}
}

func TestDedupeSessionProcsHandlesUnreadableParent(t *testing.T) {
	// ppidOf returns 0 when ps fails (dead/denied). 0 must not match a candidate
	// and silently drop a reapable process.
	reap := []reapT{
		{pid: 700, s: ccdSession{sid: "sess-a"}},
		{pid: 701, s: ccdSession{sid: "sess-a"}},
	}
	got := dedupeSessionProcs(reap, func(int) int { return 0 })
	if len(got) != 2 {
		t.Fatalf("an unreadable parent must not drop candidates, got %d", len(got))
	}
}
