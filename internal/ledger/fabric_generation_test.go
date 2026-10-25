package ledger

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/SirsiMaster/sirsi-pantheon/internal/router"
)

func TestFabricGenerationSealsInputsAndDetachedViews(t *testing.T) {
	input := FabricRead{Snapshot: fabricTaskFixture(1), Node: &router.NodeStatus{}, Metadata: FabricMetadata{Lanes: map[string]FabricLaneMetadata{"lane": {Type: "worker"}}}}
	secret := "PRIVATE-CHARTER-CANARY"
	input.Snapshot.Agents[0].Tasks[0].Charter = &secret
	calls := 0
	p := NewFabricGenerationProducer(func() (FabricRead, error) { calls++; return input, nil }, "fixture")
	g, err := p.Read(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	input.Snapshot.Agents[0].Tasks[0].Subject = "changed caller input"
	input.Metadata.Lanes["lane"] = FabricLaneMetadata{Type: "service"}
	v, err := g.View()
	if err != nil {
		t.Fatal(err)
	}
	if v.Board.Tasks.Rows[0].Subject != "Inert task 0" || v.Read.Metadata.Lanes["lane"].Type != "worker" {
		t.Fatal("input alias changed sealed generation")
	}
	v.Read.Snapshot.Agents[0].Tasks[0].Subject = "changed returned view"
	v.Board.Tasks.Rows[0].Subject = "changed board"
	v.Facts.Lanes["lane"].Counts["pending"] = 100
	again, err := g.View()
	if err != nil {
		t.Fatal(err)
	}
	if again.Board.Tasks.Rows[0].Subject != "Inert task 0" || again.Facts.Lanes["lane"].Counts["pending"] != 1 || calls != 1 {
		t.Fatal("view alias/reacquisition changed generation")
	}
	raw, err := json.Marshal(again.Board)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(raw, []byte(secret)) {
		t.Fatal("private detail widened public Fabric")
	}
	for _, value := range []any{g, again} {
		raw, err = json.Marshal(value)
		if err != nil || string(raw) != "{}" {
			t.Fatal("private generation accidentally serialized", string(raw), err)
		}
	}
}

func TestFabricGenerationMetadataKeepsCanonicalThreadObservationAndNoCredentials(t *testing.T) {
	secret := "PRIVATE-REGISTRY-CREDENTIAL-CANARY"
	reg := &router.Registry{Agents: map[string]router.AgentConfig{"horus": {Type: "service", Command: []string{secret}, Env: map[string]string{"TOKEN": secret}}}}
	threads := &router.ThreadRegistry{Threads: map[string]*router.Thread{"thread": {ThreadID: "thread", AgentID: "horus", Workstream: "service"}}}
	node := &router.NodeStatus{StaleThreads: []router.ThreadSummary{{ThreadID: "thread", AgentID: "horus", IdleSeconds: 7, Stale: true}}}
	m := captureFabricMetadata(reg, threads, node)
	if m.Lanes["horus"].Type != "service" || len(m.Threads) != 1 || !m.Threads[0].Stale || m.Threads[0].IdleSeconds == nil || *m.Threads[0].IdleSeconds != 7 {
		t.Fatal("metadata reclassified service or re-derived thread freshness")
	}
	raw, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), secret) {
		t.Fatal("registry commands/environment leaked into generation metadata")
	}
	m = captureFabricMetadata(nil, threads, nil)
	if len(m.Errors) != 3 || m.Threads[0].IdleSeconds != nil || !m.Threads[0].Stale {
		t.Fatal("missing metadata became healthy/known zero")
	}
}

func TestFabricGenerationRegistryOnlyServiceWithoutRuntimeRemainsUnknown(t *testing.T) {
	input := FabricRead{Snapshot: Snapshot{GeneratedAt: "2026-10-25T12:00:00Z"},
		Node:     &router.NodeStatus{RegisteredAgents: []string{"horus"}},
		Metadata: FabricMetadata{Lanes: map[string]FabricLaneMetadata{"horus": {Type: "service", WakeMechanism: "none"}}}}
	p := NewFabricGenerationProducer(func() (FabricRead, error) { return input, nil }, "fixture")
	g, err := p.Read(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	v, err := g.View()
	if err != nil {
		t.Fatal(err)
	}
	if len(v.Board.Lanes) != 1 || v.Board.Lanes[0].Agent != "horus" || v.Board.Lanes[0].State != "UNKNOWN" || !v.Board.Lanes[0].Registered.Router || v.Board.Work.Total != 0 || v.Board.RuntimeAvailable {
		t.Fatal("registered taskless service disappeared or became a healthy worker", v.Board)
	}
}

func TestFabricGenerationFailureAndCancellationDoNotAdvanceHistory(t *testing.T) {
	input := FabricRead{Snapshot: fabricTaskFixture(1)}
	calls := 0
	fail := false
	ctx, cancel := context.WithCancel(context.Background())
	cancelOnRead := false
	p := NewFabricGenerationProducer(func() (FabricRead, error) {
		calls++
		if fail {
			return FabricRead{}, errors.New("inert read failure")
		}
		if cancelOnRead {
			cancel()
		}
		return input, nil
	}, "fixture")
	if _, err := p.Read(context.Background()); err != nil {
		t.Fatal(err)
	}
	input.Snapshot.Agents[0].Tasks[0].Status = "done"
	fail = true
	if _, err := p.Read(context.Background()); err == nil {
		t.Fatal("failed read accepted")
	}
	fail = false
	cancelOnRead = true
	if _, err := p.Read(ctx); !errors.Is(err, context.Canceled) {
		t.Fatal("cancelled acquisition accepted", err)
	}
	cancelOnRead = false
	input.Snapshot.Agents[0].Tasks[0].Status = "in-progress"
	g, err := p.Read(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	v, err := g.View()
	if err != nil {
		t.Fatal(err)
	}
	if calls != 4 || len(v.Events) != 1 || v.Events[0].From != "pending" || v.Events[0].To != "in-progress" || v.Completed != 0 {
		t.Fatal("failed/cancelled frame advanced history", v.Events, v.Completed, calls)
	}
}

func TestFabricGenerationRejectedContinuationDoesNotAdvanceHistory(t *testing.T) {
	input := FabricRead{Snapshot: fabricTaskFixture(65)}
	p := NewFabricGenerationProducer(func() (FabricRead, error) { return input, nil }, "fixture")
	g, err := p.Read(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	first, err := g.View()
	if err != nil {
		t.Fatal(err)
	}
	input.Snapshot.Agents[0].Tasks[64].Status = "done"
	if _, err := p.ReadPage(context.Background(), 64, first.Board.Tasks.Snapshot); !errors.Is(err, ErrFabricTasksChanged) || !errors.Is(err, ErrFabricPageRejected) {
		t.Fatal("changed continuation accepted", err)
	}
	input.Snapshot.Agents[0].Tasks[64].Status = "blocked"
	g, err = p.Read(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	v, err := g.View()
	if err != nil {
		t.Fatal(err)
	}
	if len(v.Events) != 1 || v.Events[0].From != "pending" || v.Events[0].To != "blocked" || v.Completed != 0 {
		t.Fatal("refused page polluted transition baseline", v.Events)
	}
}

func TestFabricFactsInclusiveActiveAndCanonicalLiveness(t *testing.T) {
	s := fabricTaskFixture(4)
	s.Agents[0].Tasks[0].Status = "done"
	s.Agents[0].Tasks[1].Status = "in-progress"
	s.Agents[0].Tasks[1].Liveness = "stalled" // deliberately fresh timestamp
	s.Agents[0].Tasks[2].Status = "blocked"
	f := fabricFacts(s)
	if f.Work.Total != 4 || f.Work.Done != 1 || f.Work.Open != 3 || f.Work.InProgress != 0 || f.Work.Stalled != 1 || f.Work.Blocked != 1 || f.Ledger.ActiveTasks != 3 || f.Ledger.Phases[0].Active != 3 {
		t.Fatalf("shared inclusive/liveness facts differ: %+v", f)
	}
	if IdleFabricLanes([]FabricLane{{State: "IDLE_WITH_WORK"}, {State: "IDLE with work"}, {State: "WORKING"}}) != 1 {
		t.Fatal("menu used display label instead of canonical enum")
	}
}
