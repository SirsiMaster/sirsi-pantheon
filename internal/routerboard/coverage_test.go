package routerboard

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/ledger"
	"github.com/SirsiMaster/sirsi-pantheon/internal/routerstore"
)

func TestProjectionAndPayloadHelpers(t *testing.T) {
	if indexAny([]byte("prefix{value"), '{', '[') != 6 || indexAny([]byte("plain"), '{', '[') != -1 {
		t.Fatal("indexAny")
	}
	if got, ok := ageSeconds("not-a-time"); ok || got != 0 {
		t.Fatalf("bad age = %v/%v", got, ok)
	}
	if ageStr("not-a-time") != "?" {
		t.Fatal("bad age string")
	}
	if strOr(nil, "fallback") != "fallback" || strOr("", "fallback") != "fallback" || strOr("ok", "fallback") != "ok" {
		t.Fatal("strOr")
	}
	if intOr(float64(3)) != 3 || intOr("3") != 0 {
		t.Fatal("intOr")
	}
	if got := sliceOf([]interface{}{1}); len(got) != 1 || len(sliceOf(nil)) != 0 {
		t.Fatal("sliceOf")
	}
	if typeOf(map[string]interface{}{"type": "worker"}) != "worker" || typeOf(nil) != "?" {
		t.Fatal("typeOf")
	}
	if mech, label := wakeOf(map[string]interface{}{"wake": map[string]interface{}{"mechanism": "launchagent", "launch_agent_label": "x"}}); mech != "launchagent" || label != "x" {
		t.Fatal("wakeOf")
	}
	if mech, label := wakeOf(nil); mech != "" || label != "" {
		t.Fatal("empty wakeOf")
	}

	p := Payload{Board: BoardSummary{TotalTasks: 5, DoneTasks: 2, PctDone: 40}, Counters: Counters{InProgressNow: 1, Pending: 2}, Fleet: []Lane{
		{Agent: "working", Activity: "active now", Counts: map[string]int{"in-progress": 1}, TasksTotal: 1, OpenItems: 1},
		{Agent: "blocked", Counts: map[string]int{"blocked": 1}, TasksTotal: 1, OpenItems: 1, UnblockedOpen: 0},
		{Agent: "idle", IdleWithWork: true, Counts: map[string]int{}, TasksTotal: 0, OpenItems: 0},
		{Agent: "none", Counts: map[string]int{}, TasksTotal: 0, OpenItems: 0},
	}}
	out := ToFleetShape(p)
	if len(out.Lanes) != 4 || out.Lanes[0].State == "" {
		t.Fatalf("projection = %+v", out)
	}
	for _, lane := range out.Lanes {
		if lane.State == "" {
			t.Fatal("empty lane state")
		}
	}
	if b, v, err := New("sirsi", "missing", "build").SnapshotFleetShape(); err != nil || b != nil || v != 0 {
		t.Fatalf("empty snapshot = %s/%d/%v", b, v, err)
	}
	if _, err := json.Marshal(out); err != nil {
		t.Fatal(err)
	}
}

func TestPollUsesOneGenerationWithoutCLIReacquisition(t *testing.T) {
	tmp := t.TempDir()
	cli := filepath.Join(tmp, "sirsi")
	script := "#!/bin/sh\nexit 99\n"
	if err := os.WriteFile(cli, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	agents := filepath.Join(tmp, "agents.json")
	if err := os.WriteFile(agents, []byte(`{"agents":{"a":{"type":"software","wake":{"mechanism":"launchagent","launch_agent_label":"a"}}}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	read := ledger.FabricRead{Snapshot: ledger.Snapshot{GeneratedAt: time.Now().UTC().Format(time.RFC3339Nano), Agents: []ledger.Agent{{AgentID: "a", Items: []ledger.Item{{ID: "i", Title: "item", Stale: true}}, Tasks: []routerstore.Task{{TaskID: "t1", Status: "in-progress", Phase: "Build", Subject: "compile", Liveness: "active"}}}}}}
	calls := 0
	producer := ledger.NewFabricGenerationProducer(func() (ledger.FabricRead, error) { calls++; return read, nil }, "fixture")
	b := NewWithProducer(cli, agents, "test-build", producer.Read)
	b.Poll(context.Background())
	if b.Version() == 0 {
		t.Fatal("poll did not publish a payload")
	}
	body, version, err := b.SnapshotFleetShape()
	if err != nil || version == 0 || len(body) == 0 {
		t.Fatalf("fleet snapshot = %s/%d/%v", body, version, err)
	}
	var shape FleetShape
	if err := json.Unmarshal(body, &shape); err != nil || len(shape.Lanes) != 1 {
		t.Fatalf("fleet shape = %s/%v", body, err)
	}
	read.Snapshot.Agents[0].Tasks[0].Status = "done"
	b.Poll(context.Background())
	encoded, _, valid := b.SnapshotState()
	var payload Payload
	if err := json.Unmarshal(encoded, &payload); err != nil {
		t.Fatal(err)
	}
	if !valid || len(payload.Activity) != 1 || payload.Counters.Completed != 1 || calls != 2 {
		t.Fatalf("real shared transition missing: %+v calls=%d", payload, calls)
	}
	// Invalid retained payloads are surfaced, never converted to an empty board.
	b.mu.Lock()
	oldPayload, oldVersion := b.payload, b.version
	b.payload, b.version = []byte("not json"), 1
	b.mu.Unlock()
	if _, _, err := b.SnapshotFleetShape(); err == nil {
		t.Fatal("invalid payload was accepted")
	}
	b.mu.Lock()
	b.payload, b.version = oldPayload, oldVersion
	b.mu.Unlock()
}
