package dashboard

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/ledger"
	"github.com/SirsiMaster/sirsi-pantheon/internal/routerstore"
)

func snapOf(agent string, tasks ...routerstore.Task) ledger.Snapshot {
	return ledger.Snapshot{Agents: []ledger.Agent{{AgentID: agent, Tasks: tasks}}}
}

func task(id, status string) routerstore.Task {
	return routerstore.Task{TaskID: id, Status: status, Subject: "s-" + id}
}

// THE regression this board exists to avoid.
//
// The activity feed's entire value is that movement in it means movement in
// the ledger. If first sight of a task emitted an event, every restart would
// replay the whole ledger as a burst of fake transitions — 231 "changes" that
// never happened.
func TestFleetTracker_FirstPollSeedsAndEmitsNothing(t *testing.T) {
	ft := NewFleetTracker()
	got := ft.Observe(snapOf("claude-home", task("a", "pending"), task("b", "done")), time.Now())

	if len(got.Activity) != 0 {
		t.Fatalf("first poll emitted %d events; a fresh tracker must seed silently, not replay the ledger as fake changes: %+v", len(got.Activity), got.Activity)
	}
	if got.Summary.Total != 2 || got.Summary.Done != 1 {
		t.Errorf("summary wrong on seed poll: %+v", got.Summary)
	}
}

func TestFleetTracker_RealTransitionEmitsOneEvent(t *testing.T) {
	ft := NewFleetTracker()
	now := time.Now()
	ft.Observe(snapOf("claude-home", task("a", "pending")), now)

	got := ft.Observe(snapOf("claude-home", task("a", "in-progress")), now)

	if len(got.Activity) != 1 {
		t.Fatalf("want exactly 1 event, got %d: %+v", len(got.Activity), got.Activity)
	}
	e := got.Activity[0]
	if e.From != "pending" || e.To != "in-progress" || e.Agent != "claude-home" || e.TaskID != "a" {
		t.Errorf("event does not describe the transition: %+v", e)
	}
}

// A task sitting untouched must produce NO event, however many times it is
// polled. Silence here is honest; a heartbeat-shaped feed would drown the real
// movement it exists to show.
func TestFleetTracker_UnchangedTaskIsSilentAcrossManyPolls(t *testing.T) {
	ft := NewFleetTracker()
	now := time.Now()
	for i := 0; i < 25; i++ {
		got := ft.Observe(snapOf("claude-home", task("a", "in-progress")), now)
		if len(got.Activity) != 0 {
			t.Fatalf("poll %d emitted an event for an unchanged task: %+v", i, got.Activity)
		}
	}
}

func TestFleetTracker_ActivityIsNewestFirst(t *testing.T) {
	ft := NewFleetTracker()
	now := time.Now()
	ft.Observe(snapOf("a", task("t", "pending")), now)
	ft.Observe(snapOf("a", task("t", "in-progress")), now)
	got := ft.Observe(snapOf("a", task("t", "done")), now)

	if len(got.Activity) != 2 {
		t.Fatalf("want 2 events, got %d", len(got.Activity))
	}
	if got.Activity[0].To != "done" {
		t.Errorf("newest event is not first: %+v", got.Activity)
	}
}

// Unbounded growth on a long-lived server is a slow leak.
func TestFleetTracker_ActivityRingIsBounded(t *testing.T) {
	ft := NewFleetTracker()
	now := time.Now()
	ft.Observe(snapOf("a", task("t", "pending")), now)
	for i := 0; i < maxFleetEvents+50; i++ {
		status := "pending"
		if i%2 == 0 {
			status = "in-progress"
		}
		ft.Observe(snapOf("a", task("t", status)), now)
	}
	got := ft.Observe(snapOf("a", task("t", "done")), now)

	if len(got.Activity) > maxFleetEvents {
		t.Errorf("activity ring grew to %d, past the %d cap", len(got.Activity), maxFleetEvents)
	}
}

// A vanished task must be forgotten, or prev grows forever as ids churn — and
// a recreated id must RE-SEED rather than report a transition out of a status
// nothing currently holds.
func TestFleetTracker_VanishedTaskIsForgottenAndRecreatedOneReseeds(t *testing.T) {
	ft := NewFleetTracker()
	now := time.Now()
	ft.Observe(snapOf("a", task("t", "in-progress")), now)
	ft.Observe(snapOf("a"), now) // t is gone

	got := ft.Observe(snapOf("a", task("t", "pending")), now) // recreated

	if len(got.Activity) != 0 {
		t.Errorf("recreated task reported a phantom transition instead of re-seeding: %+v", got.Activity)
	}
	ft.mu.Lock()
	n := len(ft.prev)
	ft.mu.Unlock()
	if n != 1 {
		t.Errorf("prev holds %d keys; vanished tasks are not being forgotten", n)
	}
}

func TestFleetTracker_LaneStates(t *testing.T) {
	ft := NewFleetTracker()
	now := time.Now()
	snap := ledger.Snapshot{Agents: []ledger.Agent{
		{AgentID: "works", Tasks: []routerstore.Task{task("1", "in-progress"), task("2", "pending")}},
		{AgentID: "blocked", Tasks: []routerstore.Task{task("3", "blocked")}},
		{AgentID: "stopped", Tasks: []routerstore.Task{task("4", "done")}},
	}}
	got := ft.Observe(snap, now)

	want := map[string]string{"works": LaneWorking, "blocked": LaneBlocked, "stopped": LaneStopped}
	for _, l := range got.Lanes {
		if want[l.Agent] != l.State {
			t.Errorf("lane %s = %q, want %q", l.Agent, l.State, want[l.Agent])
		}
	}
}

// Active lanes must sort above finished ones, or the rows needing attention
// end up below a wall of completed lanes.
func TestFleetTracker_WorkingLanesSortFirst(t *testing.T) {
	ft := NewFleetTracker()
	snap := ledger.Snapshot{Agents: []ledger.Agent{
		{AgentID: "zzz-done", Tasks: []routerstore.Task{task("1", "done")}},
		{AgentID: "aaa-working", Tasks: []routerstore.Task{task("2", "in-progress")}},
	}}
	got := ft.Observe(snap, time.Now())

	if got.Lanes[0].Agent != "aaa-working" {
		t.Errorf("lanes[0] = %q; a working lane must outrank a finished one regardless of name", got.Lanes[0].Agent)
	}
}

// Empty must serialize as [], not null — a client doing .length on null throws.
func TestFleetSnapshot_EmptyCollectionsMarshalAsArrays(t *testing.T) {
	ft := NewFleetTracker()
	bs, err := json.Marshal(ft.Observe(ledger.Snapshot{}, time.Now()))
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(bs, &raw); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"activity", "lanes"} {
		if string(raw[k]) != "[]" {
			t.Errorf("%s marshaled as %s, want []", k, raw[k])
		}
	}
}

// Concurrency (Rule A21): handlers read while polls write.
func TestFleetTracker_ConcurrentObserveIsRaceFree(t *testing.T) {
	ft := NewFleetTracker()
	done := make(chan struct{})
	for i := 0; i < 8; i++ {
		go func(i int) {
			defer func() { done <- struct{}{} }()
			for j := 0; j < 40; j++ {
				status := "pending"
				if j%2 == 0 {
					status = "in-progress"
				}
				ft.Observe(snapOf("a", task("t", status)), time.Now())
			}
		}(i)
	}
	for i := 0; i < 8; i++ {
		<-done
	}
}
