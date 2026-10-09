package router

import (
	"testing"
	"time"
)

// TestRunCensus pins the no-misses contract: every matched agent-class process
// becomes a thread; already-tracked pids are recognized; system processes are
// never touched; a second pass is idempotent.
func TestRunCensus(t *testing.T) {
	root := t.TempDir()

	// Pre-register the worker pid so census must recognize it as tracked.
	if _, err := RegisterThread(root, &Thread{
		AgentID: "gemma", Surface: "worker", PID: 90001, Status: ThreadStatusActive,
	}); err != nil {
		t.Fatal(err)
	}

	procs := []CensusProc{
		{PID: 90001, Command: "/bin/bash /Users/x/.local/bin/sirsi-gemma-worker.sh"},
		{PID: 90002, Command: "Python /Users/x/.sirsi/gemma-capped-server.py 22320611328 --model m"},
		{PID: 90003, Command: "/usr/libexec/some-system-daemon"},
		{PID: 90004, Command: "/Users/x/.local/share/apollo-observer/apollo-observer-agent --addr 100.88.242.95:8490 --dir /Users/x/.local/share/apollo-observer/telemetry --machine M1"},
		{PID: 90005, Command: "/Users/x/Development/Apollo/artifacts/development/apollo-dashboard/apollo-dashboard --repo /Users/x/Development/Apollo --addr 127.0.0.1:8488"},
	}
	actions := RunCensus(root, procs)
	if len(actions) != 4 {
		t.Fatalf("actions = %+v, want 4 (system daemon untouched)", actions)
	}
	byPID := map[int]CensusAction{}
	for _, a := range actions {
		byPID[a.Proc.PID] = a
	}
	if a := byPID[90001]; a.Outcome != CensusKnown {
		t.Errorf("worker = %+v, want already-tracked", a)
	}
	a := byPID[90002]
	if a.Outcome != CensusRegistered || a.AgentID != "gemma" || a.Surface != "gpu-server" || a.Thread == "" {
		t.Fatalf("gpu server = %+v, want registered as gemma/gpu-server", a)
	}

	// Registered thread really exists, on this machine, active.
	reg, _ := LoadThreadRegistry(root)
	thr := reg.Threads[a.Thread]
	if thr == nil || thr.PID != 90002 || thr.Status != ThreadStatusActive || thr.MachineID != MachineID() {
		t.Fatalf("registered thread wrong: %+v", thr)
	}

	b := byPID[90004]
	if b.Outcome != CensusRegistered || b.AgentID != "apollo-observer" || b.Surface != "worker" || b.Thread == "" {
		t.Fatalf("apollo observer = %+v, want registered as apollo-observer/worker", b)
	}

	c := byPID[90005]
	if c.Outcome != CensusRegistered || c.AgentID != "apollo-dashboard" || c.Surface != "worker" || c.Thread == "" {
		t.Fatalf("apollo dashboard = %+v, want registered as apollo-dashboard/worker", c)
	}

	// Second pass: idempotent — the gpu server is now tracked, nothing new.
	again := RunCensus(root, procs)
	for _, act := range again {
		if act.Outcome != CensusKnown {
			t.Fatalf("second pass %+v, want everything already-tracked", act)
		}
	}
}

// TestRunCensus_ForeignMachineRecordsIgnored: a foreign record with the same
// pid must not mask a LOCAL unregistered process.
func TestRunCensus_ForeignMachineRecordsIgnored(t *testing.T) {
	root := t.TempDir()
	reg, _ := LoadThreadRegistry(root)
	id := NewThreadID()
	reg.Threads[id] = &Thread{
		ThreadID: id, AgentID: "gemma", Surface: "gpu-server", PID: 91000,
		Status: ThreadStatusActive, MachineID: "OTHER-MACHINE", LastSeenAt: time.Now().UTC(),
	}
	if err := SaveThreadRegistry(root, reg); err != nil {
		t.Fatal(err)
	}
	actions := RunCensus(root, []CensusProc{{PID: 91000, Command: "Python gemma-capped-server.py"}})
	if len(actions) != 1 || actions[0].Outcome != CensusRegistered {
		t.Fatalf("actions = %+v, want local registration despite foreign record", actions)
	}
}
