package supervision

import (
	"testing"
	"time"
)

func lane(s Sources) LaneInput {
	return LaneInput{Agent: "claude-home", Sources: s, Routable: true}
}

// THE defect this package exists to eliminate.
//
// A lane with open work and a live process used to render as healthy, because
// liveness was measured by process existence. Heartbeats prove a session is
// alive; they prove nothing about work. LaneInput deliberately has no
// heartbeat/PID field at all, so this cannot regress by someone re-adding the
// conflation at the call site.
func TestClassify_OpenWorkAndNoMutationIsIdleWithWork_NotWorking(t *testing.T) {
	now := time.Now()
	got := Classify(lane(Sources{OpenItems: 6}), now, DefaultWorkWindow)

	if got != StateIdleWithWork {
		t.Fatalf("lane with 6 open items and no task mutation classified %q, want %q — a live process is not evidence of work", got, StateIdleWithWork)
	}
	if !NeedsWake(got) {
		t.Error("IDLE_WITH_WORK must be wakeable; that is the repair for a stalled lane")
	}
}

func TestClassify_RecentTaskMutationIsWorking(t *testing.T) {
	now := time.Now()
	in := lane(Sources{OpenItems: 3, ActionableTasks: 2})
	in.LastTaskMutation = now.Add(-2 * time.Minute)
	in.Leases = []Lease{{LeaseID: "current-fenced-claim", ItemID: "private-task", Holder: "private-worker", ExpiresAt: now.Add(time.Minute), LastMutation: in.LastTaskMutation}}

	if got := Classify(in, now, DefaultWorkWindow); got != StateWorking {
		t.Errorf("got %q, want %q — a recent task-record mutation is the evidence of working", got, StateWorking)
	}
}

// A mutation older than the window is not evidence of current work.
func TestClassify_StaleMutationFallsBackToIdleWithWork(t *testing.T) {
	now := time.Now()
	in := lane(Sources{OpenItems: 1})
	in.LastTaskMutation = now.Add(-3 * time.Hour)

	if got := Classify(in, now, DefaultWorkWindow); got != StateIdleWithWork {
		t.Errorf("got %q, want %q — work three hours ago is not work now", got, StateIdleWithWork)
	}
}

func TestClassify_ValidLeaseIsAssigned(t *testing.T) {
	now := time.Now()
	in := lane(Sources{OpenItems: 2})
	in.Leases = []Lease{{LeaseID: "l1", ItemID: "i1", Holder: "worker", ExpiresAt: now.Add(10 * time.Minute)}}

	if got := Classify(in, now, DefaultWorkWindow); got != StateAssigned {
		t.Errorf("got %q, want %q", got, StateAssigned)
	}
}

// An expired lease is orphaned work, not a claim. Counting it as ASSIGNED is
// how work stops while continuing to look attended.
func TestClassify_ExpiredLeaseIsIdleWithWork(t *testing.T) {
	now := time.Now()
	in := lane(Sources{OpenItems: 2})
	in.Leases = []Lease{{LeaseID: "l1", ItemID: "i1", Holder: "dead-worker", ExpiresAt: now.Add(-time.Minute)}}

	got := Classify(in, now, DefaultWorkWindow)
	if got != StateIdleWithWork {
		t.Errorf("got %q, want %q — an expired lease is orphaned work, not a live claim", got, StateIdleWithWork)
	}
	if !NeedsWake(got) {
		t.Error("a lane holding only expired leases must be wakeable")
	}
}

// Blocked must be EARNED by a recorded blocker covering all actionable work.
// A lane with runnable work alongside one blocked task is idle, not blocked —
// otherwise a stall acquires a legitimate-sounding excuse.
func TestClassify_PartiallyBlockedLaneIsIdleNotBlocked(t *testing.T) {
	now := time.Now()
	in := lane(Sources{ActionableTasks: 3, BlockedTasks: 1})

	if got := Classify(in, now, DefaultWorkWindow); got != StateIdleWithWork {
		t.Errorf("got %q, want %q — one blocked task does not block a lane with three runnable ones", got, StateIdleWithWork)
	}
}

func TestClassify_FullyBlockedLaneIsBlocked(t *testing.T) {
	now := time.Now()
	in := lane(Sources{ActionableTasks: 0, BlockedTasks: 2, OpenItems: 0, UnmetRequirements: 0})

	if got := Classify(in, now, DefaultWorkWindow); got != StateBlocked {
		t.Errorf("got %q, want %q", got, StateBlocked)
	}
}

func TestClassify_UnroutableEscalatesRatherThanReadingIdle(t *testing.T) {
	now := time.Now()
	in := lane(Sources{OpenItems: 4})
	in.Routable = false

	got := Classify(in, now, DefaultWorkWindow)
	if got != StateUnroutable {
		t.Fatalf("got %q, want %q", got, StateUnroutable)
	}
	if NeedsWake(got) {
		t.Error("UNROUTABLE must not be wakeable — waking harder cannot reach a lane with no wake path")
	}
	if !Escalates(got) {
		t.Error("UNROUTABLE must escalate")
	}
}

func TestClassify_WorkEvidencePrecedesUnavailableWake(t *testing.T) {
	now := time.Date(2026, time.November, 10, 18, 9, 0, 0, time.UTC)
	for _, tc := range []struct {
		name           string
		mutation       time.Time
		lease          time.Time
		fencedMutation bool
		want           LaneState
	}{
		{"metadata without claim", now.Add(-time.Minute), time.Time{}, false, StateUnroutable},
		{"valid claim", time.Time{}, now.Add(time.Minute), false, StateAssigned},
		{"fenced mutation before unavailable wake", now.Add(-time.Minute), now.Add(time.Minute), true, StateWorking},
		{"metadata with unrelated claim", now.Add(-time.Minute), now.Add(time.Minute), false, StateAssigned},
		{"old claimed-row mutation", now.Add(-time.Hour), now.Add(time.Minute), true, StateAssigned},
		{"stale evidence", now.Add(-time.Hour), now.Add(-time.Minute), true, StateUnroutable},
		{"no evidence", time.Time{}, time.Time{}, false, StateUnroutable},
	} {
		t.Run(tc.name, func(t *testing.T) {
			in := lane(Sources{ActionableTasks: 1})
			in.Routable = false
			in.LastTaskMutation = tc.mutation
			if !tc.lease.IsZero() {
				in.Leases = []Lease{{LeaseID: "private-claim", ExpiresAt: tc.lease}}
				if tc.fencedMutation {
					in.Leases[0].LastMutation = tc.mutation
				}
			}
			if got := Classify(in, now, DefaultWorkWindow); got != tc.want {
				t.Fatalf("unavailable wake hid work evidence: got %q, want %q", got, tc.want)
			}
			if in.Routable {
				t.Fatal("classification must not grant wake readiness")
			}
		})
	}
	if got := Classify(LaneInput{Sources: Sources{BlockedTasks: 1}}, now, DefaultWorkWindow); got != StateBlocked {
		t.Fatalf("recorded blocked-only work is not runnable: got %q", got)
	}
	if got := Classify(LaneInput{LastTaskMutation: now}, now, DefaultWorkWindow); got != StateComplete {
		t.Fatalf("empty work sources must not become synthetic WORKING: got %q", got)
	}
	in := LaneInput{Sources: Sources{ActionableTasks: 2}, Leases: []Lease{
		{LeaseID: "older-live-claim", ExpiresAt: now.Add(time.Minute), LastMutation: now.Add(-time.Hour)},
		{LeaseID: "recent-live-claim", ExpiresAt: now.Add(time.Minute), LastMutation: now.Add(-time.Minute)},
	}}
	if got := Classify(in, now, DefaultWorkWindow); got != StateWorking {
		t.Fatalf("first old claim hid a different current claimed-row mutation: got %q", got)
	}
}

// COMPLETE is the ONLY legitimate parked state, and it requires all three
// sources drained simultaneously.
func TestClassify_CompleteRequiresAllThreeSourcesEmpty(t *testing.T) {
	now := time.Now()
	if got := Classify(lane(Sources{}), now, DefaultWorkWindow); got != StateComplete {
		t.Errorf("drained lane got %q, want %q", got, StateComplete)
	}

	// Each source alone must defeat COMPLETE.
	for name, s := range map[string]Sources{
		"open item":         {OpenItems: 1},
		"actionable task":   {ActionableTasks: 1},
		"unmet requirement": {UnmetRequirements: 1},
	} {
		if got := Classify(lane(s), now, DefaultWorkWindow); got == StateComplete {
			t.Errorf("lane with an %s classified COMPLETE — parking with work outstanding is the failure this predicate forbids", name)
		}
	}
}

// The predicate itself, independent of classification.
func TestRunnable_AnySourceMakesALaneRunnable(t *testing.T) {
	cases := map[string]Sources{
		"open item":         {OpenItems: 1},
		"actionable task":   {ActionableTasks: 1},
		"unmet requirement": {UnmetRequirements: 1},
	}
	for name, s := range cases {
		if !s.Runnable() {
			t.Errorf("%s did not make the lane runnable", name)
		}
	}
	if (Sources{}).Runnable() {
		t.Error("an empty lane must not be runnable")
	}
}

// A blocked task is real work but not RUNNABLE work — counting it would make a
// fully-blocked lane look runnable forever and never reach a terminal state.
func TestRunnable_BlockedWorkAloneIsNotRunnable(t *testing.T) {
	if (Sources{BlockedTasks: 5}).Runnable() {
		t.Error("blocked-only work reported runnable; a fully blocked lane would never settle")
	}
	if (Sources{BlockedTasks: 5}).Empty() {
		t.Error("blocked work must still defeat Empty() — a completion claim over blocked work is false")
	}
}
