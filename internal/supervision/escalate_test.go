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

// A lane the classifier never scored must not be invented into an escalation.
func TestLaneWithoutStateIsSkipped(t *testing.T) {
	lanes := []LaneInput{escLane("ghost", 9, 0, 0, false)}
	if got := Escalations(lanes, map[string]LaneState{}); len(got) != 0 {
		t.Errorf("Escalations() = %d, want 0 for a lane with no classified state", len(got))
	}
}
