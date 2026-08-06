package router

import (
	"testing"
	"time"
)

// TestSortedThreads_IsATotalOrderWhenTimestampsTie is the regression for the
// flake that randomly reddened CI for every PR in this repo.
//
// SortedThreads used sort.Slice ordering ONLY by LastSeenAt. sort.Slice is
// unstable, so any two threads sharing a timestamp came back in arbitrary order
// that varied run to run. Shared timestamps are not an edge case: heartbeats land
// in the same second, and a fixed test clock makes every record identical.
//
// TestStaleActiveSupervisors builds five threads on exactly two timestamps and
// asserts a fixed order, so it passed or failed at random — it failed once here
// and then passed five consecutive runs, which is exactly how a flake hides.
func TestSortedThreads_IsATotalOrderWhenTimestampsTie(t *testing.T) {
	now := time.Date(2026, 8, 6, 0, 0, 0, 0, time.UTC)
	stale := now.Add(-time.Hour)

	// All five share ONE timestamp: ordering must fall entirely to the tiebreak.
	reg := &ThreadRegistry{Threads: map[string]*Thread{}}
	for _, id := range []string{"echo", "alpha", "delta", "bravo", "charlie"} {
		reg.Threads[id] = &Thread{ThreadID: id, Status: ThreadStatusActive, LastSeenAt: stale}
	}

	want := []string{"alpha", "bravo", "charlie", "delta", "echo"}

	// Repeat: Go randomizes map iteration per run, so a single pass can pass by
	// luck. This is the assertion the old code could not satisfy.
	for round := 0; round < 200; round++ {
		got := reg.SortedThreads()
		if len(got) != len(want) {
			t.Fatalf("round %d: got %d threads, want %d", round, len(got), len(want))
		}
		for i := range want {
			if got[i].ThreadID != want[i] {
				t.Fatalf("round %d: position %d = %q, want %q — SortedThreads is not a total order",
					round, i, got[i].ThreadID, want[i])
			}
		}
	}
}

// TestSortedThreads_RecencyStillWins guards the tiebreak from inverting the
// primary contract: newer records must still sort first.
func TestSortedThreads_RecencyStillWins(t *testing.T) {
	now := time.Date(2026, 8, 6, 0, 0, 0, 0, time.UTC)
	reg := &ThreadRegistry{Threads: map[string]*Thread{
		// "zulu" sorts LAST alphabetically but is the most recent, so recency
		// must put it first. If the tiebreak ever became the primary key this
		// fails.
		"zulu":  {ThreadID: "zulu", LastSeenAt: now},
		"alpha": {ThreadID: "alpha", LastSeenAt: now.Add(-time.Hour)},
		"bravo": {ThreadID: "bravo", LastSeenAt: now.Add(-time.Hour)},
	}}
	for round := 0; round < 50; round++ {
		got := reg.SortedThreads()
		if got[0].ThreadID != "zulu" {
			t.Fatalf("round %d: most recent thread is %q, want zulu — recency must remain the primary key",
				round, got[0].ThreadID)
		}
		if got[1].ThreadID != "alpha" || got[2].ThreadID != "bravo" {
			t.Fatalf("round %d: tied pair ordered %q,%q; want alpha,bravo",
				round, got[1].ThreadID, got[2].ThreadID)
		}
	}
}
