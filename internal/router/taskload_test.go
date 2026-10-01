package router

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/dispatch"
	"github.com/SirsiMaster/sirsi-pantheon/internal/routercfg"
	"github.com/SirsiMaster/sirsi-pantheon/internal/routerstore"
)

func seedTask(t *testing.T, root, agent, id, party, blockedBy string) {
	t.Helper()
	f, err := dispatch.OpenRoot(root)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer func() { _ = f.Close() }()
	if err = f.Store().AddTask(routerstore.Task{Agent: agent, TaskID: id, Subject: id, ResponsibleParty: party, BlockedBy: blockedBy}); err != nil {
		t.Fatalf("add task %s: %v", id, err)
	}
}

func TestDispatchDepthAndTaskMark(t *testing.T) {
	if got := dispatchDepth(0, TaskLoad{Dispatchable: 2}); got != 2 {
		t.Fatalf("task-only lane must have depth 2, got %d", got)
	}
	if got := dispatchDepth(3, TaskLoad{Dispatchable: 2}); got != 5 {
		t.Fatalf("items + tasks = 5, got %d", got)
	}
	if got := dispatchDepth(0, TaskLoad{Leased: 4, Actionable: 4}); got != 0 {
		t.Fatalf("leased tasks are in flight, not a trigger: depth %d", got)
	}
	// A claim (dispatchable→leased) must change the progress fingerprint.
	if taskMark(TaskLoad{Dispatchable: 1, Actionable: 1}) == taskMark(TaskLoad{Leased: 1, Actionable: 1}) {
		t.Fatal("claiming a task must change the progress mark")
	}
}

// TestLaneTaskLoadReadsTheLedger: store mode counts the lane's own claimable tasks
// and nothing else; legacy mode (no durable ledger) keeps the old behavior.
func TestLaneTaskLoadReadsTheLedger(t *testing.T) {
	root := t.TempDir()
	t.Setenv(routercfg.StoreWakeEnv, "1")
	seedTask(t, root, "tl-agent", "mine", "self", "")
	seedTask(t, root, "tl-agent", "owner-step", "owner", "")
	seedTask(t, root, "tl-agent", "blocked", "self", "waiting-on-a-decision")
	seedTask(t, root, "someone-else", "theirs", "self", "")
	tl, err := LaneTaskLoad(root, "tl-agent")
	if err != nil {
		t.Fatal(err)
	}
	if tl.Dispatchable != 1 {
		t.Fatalf("dispatchable = %d, want 1 (only 'mine'): %+v", tl.Dispatchable, tl)
	}
	t.Setenv(routercfg.StoreWakeEnv, "0")
	if tl, err = LaneTaskLoad(root, "tl-agent"); err != nil || tl.Dispatchable != 0 {
		t.Fatalf("legacy mode must report no task load: %+v err=%v", tl, err)
	}
}

// TestWakeLoopDispatchesForTaskOnlyLane is the end-to-end claim: a lane with ZERO
// inbox items and one task of its own gets a consumer started. Before this, a
// task alone started nobody (2026-10-01), so work the router placed on the
// ledger sat until other mail happened to arrive.
func TestWakeLoopDispatchesForTaskOnlyLane(t *testing.T) {
	SetLoadAvgFn(func() (float64, bool) { return 0.1, true })
	defer SetLoadAvgFn(nil)
	hermeticDispatchDir(t)

	cases := []struct {
		name      string
		party     string
		blockedBy string
		want      bool
	}{
		{"own task starts a worker", "self", "", true},
		{"owner-assigned task does not", "owner", "", false},
		{"blocked task does not", "self", "waiting-on-a-decision", false},
	}
	for i, c := range cases {
		agent := "task-agent-" + string(rune('a'+i)) // own agent per case: the scratch store is shared
		t.Run(c.name, func(t *testing.T) {
			log := filepath.Join(t.TempDir(), "fired.txt")
			script := recordingConsumer(t, log, 0)
			// Fully declared (store mode validates identity on every inbox read).
			root := wakeTestRoot(t, AgentConfig{
				ID: agent, Type: "worker", Cwd: t.TempDir(), Workstream: "test",
				Wake:     WakeConfig{Mechanism: "launchagent"},
				Consumer: ConsumerConfig{Command: []string{script, "--agent", consumerAgentPlaceholder}, Prompt: "work " + consumerAgentPlaceholder},
			})
			t.Setenv(routercfg.StoreWakeEnv, "1")
			seedTask(t, root, agent, "t1", c.party, c.blockedBy)

			ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
			defer cancel()
			if err := RunWakeLoop(ctx, root, agent, 40*time.Millisecond); err != nil {
				t.Fatalf("RunWakeLoop: %v", err)
			}
			lines := awaitConsumerLines(t, log, 1)
			if c.want && len(lines) == 0 {
				t.Fatal("a lane with only a dispatchable task must have a consumer started")
			}
			if !c.want && len(lines) != 0 {
				t.Fatalf("a task the lane cannot work must NOT start a consumer, got %d dispatches", len(lines))
			}
		})
	}
}
