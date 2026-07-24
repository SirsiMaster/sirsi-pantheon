package main

import "testing"

// TestCcdShouldArchiveProtectsLiveSessions pins the archive-pass safety rules,
// including the one codex-pantheon's review of PR #290 caught: an old UNTAGGED
// session that still has an attributed live runner must not be archived out of
// the app's history on age alone.
func TestCcdShouldArchiveProtectsLiveSessions(t *testing.T) {
	const now = 1_800_000_000.0
	minsAgo := func(m float64) float64 { return now - m*60 }
	daysAgo := func(d float64) float64 { return now - d*86400 }

	newest := map[string]float64{"conduit": minsAgo(1)} // the running conduit instance
	live := map[string]bool{"live-untagged": true, "live-task": true}

	cases := []struct {
		name string
		s    ccdSession
		want bool
	}{
		{"old untagged but LIVE — the #290 review finding",
			ccdSession{sid: "live-untagged", last: daysAgo(30)}, false},
		{"completed task run but LIVE",
			ccdSession{sid: "live-task", sched: "conduit", last: minsAgo(90)}, false},
		{"old untagged, no live runner",
			ccdSession{sid: "stale", last: daysAgo(30)}, true},
		{"untagged idle under the stale window",
			ccdSession{sid: "recent", last: daysAgo(2)}, false},
		{"newest instance of its task",
			ccdSession{sid: "current", sched: "conduit", last: minsAgo(1)}, false},
		{"completed task run past grace",
			ccdSession{sid: "done", sched: "conduit", last: minsAgo(90)}, true},
		{"completed task run inside grace",
			ccdSession{sid: "justfinished", sched: "conduit", last: minsAgo(2)}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := ccdShouldArchive(c.s, newest, live, now); got != c.want {
				t.Fatalf("ccdShouldArchive(%+v) = %v, want %v", c.s, got, c.want)
			}
		})
	}
}
