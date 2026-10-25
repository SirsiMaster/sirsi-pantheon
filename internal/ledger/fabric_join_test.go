package ledger

import (
	"encoding/json"
	"github.com/SirsiMaster/sirsi-pantheon/internal/router"
	"github.com/SirsiMaster/sirsi-pantheon/internal/routerstore"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestFabricCanonicalRuntimeOverridesStatusAndHeartbeat(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "owned")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	store, err := routerstore.OpenPath(filepath.Join(dir, "fixture.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Error(err)
		}
	})
	if err := store.AddTask(routerstore.Task{Agent: "lane", TaskID: "work", Subject: "inert", Status: "in-progress"}); err != nil {
		t.Fatal(err)
	}
	task, err := store.GetTask("lane", "work")
	if err != nil {
		t.Fatal(err)
	}
	if task.Liveness != "active" {
		t.Fatalf("fixture must reproduce status-derived active: %s", task.Liveness)
	}
	runtime, err := store.ClassifyLane("lane", true, 5*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	snap := Snapshot{GeneratedAt: time.Now().UTC().Format(time.RFC3339), Agents: []Agent{{AgentID: "lane", Tasks: []routerstore.Task{task}}}}
	ns := &router.NodeStatus{LiveThreads: []router.ThreadSummary{{AgentID: "lane", ThreadID: "live"}}}
	board := FabricFrom(snap, ns, "fixture", FabricEvidence{Operational: map[string]routerstore.LaneOperationalState{"lane": runtime}})
	if board.Lanes[0].State == "WORKING" || board.Lanes[0].State != runtime.Classification {
		t.Fatalf("heartbeat/status was promoted over canonical runtime: %+v", board.Lanes[0])
	}
	if _, err := store.ClaimTask("lane", "work", "worker", "thread", time.Minute); err != nil {
		t.Fatal(err)
	}
	runtime, err = store.ClassifyLane("lane", true, 5*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if !runtime.RecentMutation || runtime.LiveLeases == 0 || runtime.Classification != routerstore.LaneWorking {
		t.Fatalf("positive not backed by canonical leased mutation: %+v", runtime)
	}
	board = FabricFrom(snap, ns, "fixture", FabricEvidence{Operational: map[string]routerstore.LaneOperationalState{"lane": runtime}})
	if board.Lanes[0].State != "WORKING" || board.Lanes[0].Runtime == nil {
		t.Fatal("real runtime working evidence lost")
	}
}
func TestFabricMissingRuntimeIsUnknownNotWorking(t *testing.T) {
	snap := Snapshot{Agents: []Agent{{AgentID: "lane", Tasks: []routerstore.Task{{TaskID: "work", Status: "in-progress", Liveness: "active"}}}}}
	board := FabricFrom(snap, &router.NodeStatus{}, "fixture")
	if board.RuntimeAvailable || board.Lanes[0].State != "UNKNOWN" || board.Work.InProgress != 1 {
		t.Fatalf("task status and work evidence conflated: %+v", board)
	}
}
func TestFabricEmptySerializesKnownEmptyArraysAndUnknownHealthNull(t *testing.T) {
	board := FabricFrom(Snapshot{Agents: []Agent{}}, &router.NodeStatus{}, "fixture", FabricEvidence{Operational: map[string]routerstore.LaneOperationalState{}})
	raw, err := json.Marshal(board)
	if err != nil {
		t.Fatal(err)
	}
	var value map[string]any
	if err := json.Unmarshal(raw, &value); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"activity", "lanes"} {
		if rows, ok := value[name].([]any); !ok || len(rows) != 0 {
			t.Fatalf("%s is not []: %s", name, raw)
		}
	}
	health := value["health"].(map[string]any)
	for _, name := range []string{"ram_pct", "swap", "git_dirty", "total_ram", "cpu_cores"} {
		if health[name] != nil {
			t.Fatalf("unknown %s became a value: %s", name, raw)
		}
	}
	if board.ActivitySeeded {
		t.Fatal("one-shot empty output claimed transition coverage")
	}
}
