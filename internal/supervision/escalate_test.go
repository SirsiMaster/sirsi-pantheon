package supervision

import (
	"strings"
	"testing"
)

func escLane(agent string, open, actionable, blocked int, routable bool) LaneInput {
	return LaneInput{
		Agent:    agent,
		Sources:  Sources{OpenItems: open, ActionableTasks: actionable, BlockedTasks: blocked},
		Routable: routable,
	}
}

func escLaneThreads(agent string, open, actionable, blocked, activeThreads int, routable bool) LaneInput {
	l := escLane(agent, open, actionable, blocked, routable)
	l.ActiveThreads = activeThreads
	return l
}

// The defect this whole file exists for: an unroutable lane holding work used to
// produce nothing at all, because Escalates() had no caller.
func TestUnroutableLaneWithWorkEscalates(t *testing.T) {
	lanes := []LaneInput{escLane("codex-home", 5, 0, 0, false)}
	states := map[string]LaneState{"codex-home": StateUnroutable}

	got := Escalations(lanes, states)
	if len(got) != 1 {
		t.Fatalf("Escalations() = %d, want 1 — an unroutable lane holding work must reach the owner", len(got))
	}
	if got[0].Agent != "codex-home" || got[0].OpenItems != 5 {
		t.Errorf("got %+v, want codex-home with 5 open", got[0])
	}
	if !strings.Contains(got[0].Why, "Opening a session") {
		t.Errorf("Why must tell the owner what clears it, got: %q", got[0].Why)
	}
}

// strand.go deliberately suppressed idle mechanism:none lanes as noise. That
// call must survive here, or the escalation channel gets muted and the real
// ones are lost with it.
func TestUnroutableLaneWithoutWorkStaysQuiet(t *testing.T) {
	lanes := []LaneInput{escLane("codex-mail", 0, 0, 0, false)}
	states := map[string]LaneState{"codex-mail": StateUnroutable}

	if got := Escalations(lanes, states); len(got) != 0 {
		t.Errorf("Escalations() = %d, want 0 — an idle unwakeable lane is parked, not stuck", len(got))
	}
}

// A lane a wake CAN reach is the supervisor's problem, never the owner's.
func TestRoutableIdleLaneDoesNotEscalate(t *testing.T) {
	lanes := []LaneInput{escLane("claude-nexus", 24, 9, 6, true)}
	states := map[string]LaneState{"claude-nexus": StateIdleWithWork}

	if got := Escalations(lanes, states); len(got) != 0 {
		t.Errorf("Escalations() = %d, want 0 — IDLE_WITH_WORK is repaired by waking, not by paging", len(got))
	}
}

// The title is the dedup key against the owner's open inbox. If it embeds a
// count or an age it changes between passes, dedup silently stops working, and
// a 60s sweep mints a new item every minute.
func TestTitleIsStableAcrossChangingCounts(t *testing.T) {
	a := Escalation{Agent: "codex-finalwishes", OpenItems: 6}
	b := Escalation{Agent: "codex-finalwishes", OpenItems: 41}

	if a.Title() != b.Title() {
		t.Errorf("title drifted with count: %q vs %q — dedup would fail every pass", a.Title(), b.Title())
	}
	if !strings.Contains(a.Title(), "codex-finalwishes") {
		t.Errorf("title must name the lane, got %q", a.Title())
	}
}

func TestEscalationsSortDeepestBacklogFirst(t *testing.T) {
	lanes := []LaneInput{
		escLane("codex-assiduous", 1, 0, 0, false),
		escLane("codex-finalwishes", 6, 0, 0, false),
		escLane("codex-home", 5, 0, 0, false),
	}
	states := map[string]LaneState{
		"codex-assiduous":   StateUnroutable,
		"codex-finalwishes": StateUnroutable,
		"codex-home":        StateUnroutable,
	}

	got := Escalations(lanes, states)
	want := []string{"codex-finalwishes", "codex-home", "codex-assiduous"}
	if len(got) != len(want) {
		t.Fatalf("got %d escalations, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i].Agent != want[i] {
			t.Errorf("position %d = %s, want %s", i, got[i].Agent, want[i])
		}
	}
}

// The defect from 2026-08-06: a lane with wake:none but active threads was
// escalated as "cannot be reached". Startability and liveness are different.
// A lane with active threads must be suppressed even when UNROUTABLE.
func TestUnroutableLaneWithActiveThreadsIsNotEscalated(t *testing.T) {
	// 50% false-positive rate measured in prod: codex-home had 1 active thread,
	// claude-home had 3 — both were escalated as stopped.
	cases := []struct {
		name    string
		threads int
	}{
		{"one thread", 1},
		{"three threads", 3},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			lane := escLaneThreads("codex-home", 5, 2, 0, tc.threads, false)
			states := map[string]LaneState{"codex-home": StateUnroutable}
			if got := Escalations([]LaneInput{lane}, states); len(got) != 0 {
				t.Errorf("Escalations() = %d, want 0 — a lane with %d active thread(s) is not stopped",
					len(got), tc.threads)
			}
		})
	}
}

// Zero active threads + unroutable + work = the only correct escalation shape.
func TestUnroutableLaneWithNoThreadsEscalates(t *testing.T) {
	lane := escLaneThreads("claude-deck", 3, 0, 0, 0, false)
	states := map[string]LaneState{"claude-deck": StateUnroutable}
	got := Escalations([]LaneInput{lane}, states)
	if len(got) != 1 {
		t.Fatalf("Escalations() = %d, want 1 — stopped lane with work must reach owner", len(got))
	}
	if got[0].Agent != "claude-deck" {
		t.Errorf("escalated agent = %q, want claude-deck", got[0].Agent)
	}
	// Why text must acknowledge thread absence, not just wake:none.
	if !strings.Contains(got[0].Why, "no registered thread") {
		t.Errorf("Why should mention absent threads, got: %q", got[0].Why)
	}
}

// The re-mint regression: dedup title must not embed counts that change between
// passes. Covered independently; adding a cross-check with the thread gate.
func TestTitleStableWithThreadGate(t *testing.T) {
	a := escLaneThreads("claude-io", 1, 0, 0, 0, false)
	b := escLaneThreads("claude-io", 9, 3, 1, 0, false)
	state := map[string]LaneState{"claude-io": StateUnroutable}
	ga := Escalations([]LaneInput{a}, state)
	gb := Escalations([]LaneInput{b}, state)
	if len(ga) != 1 || len(gb) != 1 {
		t.Fatalf("both should escalate (0 threads): got %d, %d", len(ga), len(gb))
	}
	if ga[0].Title() != gb[0].Title() {
		t.Errorf("title drifted: %q vs %q", ga[0].Title(), gb[0].Title())
	}
}

// A lane the classifier never scored must not be invented into an escalation.
func TestLaneWithoutStateIsSkipped(t *testing.T) {
	lanes := []LaneInput{escLane("ghost", 9, 0, 0, false)}
	if got := Escalations(lanes, map[string]LaneState{}); len(got) != 0 {
		t.Errorf("Escalations() = %d, want 0 for a lane with no classified state", len(got))
	}
}
