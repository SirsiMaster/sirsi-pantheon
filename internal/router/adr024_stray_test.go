package router

// ADR-024 supersession reaper — ReapStrayThreads enforces "one live watcher per
// (agent, surface, machine)": once a live watcher holds a surface, its non-live
// siblings (superseded suspends + dead-PID ghosts) are swept, while an
// un-superseded suspend (no live sibling) survives (ADR-025). Salvageable state
// is captured before the sweep so nothing is lost.

import "testing"

// perPIDProbe stubs liveness so alivePIDs report PIDAlive (with a matching start
// signature so the composite key does not false-reap) and every other PID reports
// PIDGone. Returns a restore func.
func perPIDProbe(t *testing.T, alivePIDs ...int) func() {
	t.Helper()
	alive := map[int]bool{}
	for _, p := range alivePIDs {
		alive[p] = true
	}
	oldState, oldStart, oldCommand := getPIDStateFn(), getPIDStartFn(), getPIDCommandFn()
	setPIDStateFn(func(pid int) PIDState {
		if alive[pid] {
			return PIDAlive
		}
		return PIDGone
	})
	setPIDStartFn(func(int) string { return "sig" }) // recorded StartTime is "sig" too → match
	setPIDCommandFn(func(int) string { return "" })
	return func() { setPIDStateFn(oldState); setPIDStartFn(oldStart); setPIDCommandFn(oldCommand) }
}

func TestReapStrayThreads_SupersedesGhostsKeepsLonelySuspend(t *testing.T) {
	const machine = "THIS-MACHINE"
	oldMID := getMachineIDFn()
	setMachineIDFn(func() string { return machine })
	defer setMachineIDFn(oldMID)
	defer perPIDProbe(t, 60000)() // only the live anchor's PID is alive

	tmp := t.TempDir()
	reg := &ThreadRegistry{Threads: map[string]*Thread{
		// the one live watcher for claude-home/claude — the anchor
		"thr-live": {ThreadID: "thr-live", AgentID: "claude-home", Surface: "claude",
			PID: 60000, StartTime: "sig", MachineID: machine, Status: ThreadStatusActive},
		// empty tombstone suspend — superseded, no salvageable state → swept, no ledger
		"thr-susp-empty": {ThreadID: "thr-susp-empty", AgentID: "claude-home", Surface: "claude",
			MachineID: machine, Status: ThreadStatusSuspended,
			SuspendPayload: &SuspendPayload{ResumePrompt: "session ended"}},
		// suspend with real owned work — superseded → swept, salvage captured
		"thr-susp-work": {ThreadID: "thr-susp-work", AgentID: "claude-home", Surface: "claude",
			MachineID: machine, Status: ThreadStatusSuspended,
			SuspendPayload: &SuspendPayload{ResumePrompt: "finish PR #99", OwnedOpenItems: []string{"item-1"}}},
		// dead-PID active ghost — superseded → swept
		"thr-ghost": {ThreadID: "thr-ghost", AgentID: "claude-home", Surface: "claude",
			PID: 60001, StartTime: "sig", MachineID: machine, Status: ThreadStatusActive},
		// lonely suspend for a DIFFERENT agent with no live sibling → MUST survive (ADR-025)
		"thr-lonely": {ThreadID: "thr-lonely", AgentID: "claude-nexus", Surface: "claude",
			MachineID: machine, Status: ThreadStatusSuspended,
			SuspendPayload: &SuspendPayload{ResumePrompt: "resume nexus work"}},
		// foreign-machine suspend of the same surface → unobservable, untouched
		"thr-foreign": {ThreadID: "thr-foreign", AgentID: "claude-home", Surface: "claude",
			MachineID: "OTHER-MACHINE", Status: ThreadStatusSuspended,
			SuspendPayload: &SuspendPayload{ResumePrompt: "elsewhere"}},
	}}
	if err := SaveThreadRegistry(tmp, reg); err != nil {
		t.Fatal(err)
	}

	retired, err := ReapStrayThreads(tmp)
	if err != nil {
		t.Fatal(err)
	}
	if len(retired) != 3 {
		t.Fatalf("expected 3 strays retired, got %d: %+v", len(retired), retired)
	}

	out, err := LoadThreadRegistry(tmp)
	if err != nil {
		t.Fatal(err)
	}
	wantStatus := map[string]ThreadStatus{
		"thr-live":       ThreadStatusActive,    // live anchor untouched
		"thr-susp-empty": ThreadStatusClosed,    // superseded
		"thr-susp-work":  ThreadStatusClosed,    // superseded
		"thr-ghost":      ThreadStatusClosed,    // superseded dead-PID ghost
		"thr-lonely":     ThreadStatusSuspended, // no live sibling → ADR-025 preserved
		"thr-foreign":    ThreadStatusSuspended, // foreign machine → untouched
	}
	for id, want := range wantStatus {
		if got := out.Threads[id].Status; got != want {
			t.Errorf("%s status = %q, want %q", id, got, want)
		}
	}
}

func TestReapStrayThreads_NoLiveAnchorIsNoOp(t *testing.T) {
	const machine = "THIS-MACHINE"
	oldMID := getMachineIDFn()
	setMachineIDFn(func() string { return machine })
	defer setMachineIDFn(oldMID)
	defer perPIDProbe(t /* nothing alive */)()

	tmp := t.TempDir()
	reg := &ThreadRegistry{Threads: map[string]*Thread{
		"thr-a": {ThreadID: "thr-a", AgentID: "claude-home", Surface: "claude",
			MachineID: machine, Status: ThreadStatusSuspended,
			SuspendPayload: &SuspendPayload{ResumePrompt: "session ended"}},
		"thr-b": {ThreadID: "thr-b", AgentID: "claude-home", Surface: "claude",
			MachineID: machine, Status: ThreadStatusSuspended,
			SuspendPayload: &SuspendPayload{ResumePrompt: "session ended"}},
	}}
	if err := SaveThreadRegistry(tmp, reg); err != nil {
		t.Fatal(err)
	}
	retired, err := ReapStrayThreads(tmp)
	if err != nil {
		t.Fatal(err)
	}
	if len(retired) != 0 {
		t.Fatalf("no live anchor → must not retire any suspend, got %d", len(retired))
	}
}

func TestStraySalvage_OnlySavesRealWork(t *testing.T) {
	cases := []struct {
		name string
		t    *Thread
		want bool
	}{
		{"boilerplate resume is empty tombstone",
			&Thread{SuspendPayload: &SuspendPayload{ResumePrompt: "session ended"}}, false},
		{"blank payload is empty tombstone",
			&Thread{SuspendPayload: &SuspendPayload{}}, false},
		{"nil payload is empty tombstone", &Thread{}, false},
		{"real resume prompt is salvageable",
			&Thread{SuspendPayload: &SuspendPayload{ResumePrompt: "finish the migration"}}, true},
		{"owned open items are salvageable",
			&Thread{SuspendPayload: &SuspendPayload{ResumePrompt: "session ended", OwnedOpenItems: []string{"x"}}}, true},
		{"thoth ref is salvageable",
			&Thread{SuspendPayload: &SuspendPayload{ThothRef: "stele-abc"}}, true},
		{"in-flight current item is salvageable", &Thread{CurrentItem: "item-42"}, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, ok := straySalvage(c.t, "thr-anchor")
			if ok != c.want {
				t.Errorf("straySalvage ok = %v, want %v", ok, c.want)
			}
		})
	}
}
