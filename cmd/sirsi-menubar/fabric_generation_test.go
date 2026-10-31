package main

import (
	"context"
	"testing"

	"github.com/SirsiMaster/sirsi-pantheon/internal/ledger"
	"github.com/SirsiMaster/sirsi-pantheon/internal/routerstore"
)

func TestFabricMenuUsesGenerationCanonicalIdleEnum(t *testing.T) {
	producer := ledger.NewFabricGenerationProducer(func() (ledger.FabricRead, error) {
		return ledger.FabricRead{Snapshot: ledger.Snapshot{GeneratedAt: "2026-10-25T12:00:00Z", Agents: []ledger.Agent{{AgentID: "idle"}, {AgentID: "service"}}}, Evidence: ledger.FabricEvidence{Operational: map[string]routerstore.LaneOperationalState{"idle": {Agent: "idle", Classification: routerstore.LaneIdleWithWork}, "service": {Agent: "service", Classification: routerstore.LaneComplete}}}}, nil
	}, "fixture")
	g, err := producer.Read(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	v, err := g.View()
	if err != nil {
		t.Fatal(err)
	}
	if countIdleLanes(v.Board.Lanes) != 1 {
		t.Fatal("actual menu helper failed canonical IDLE_WITH_WORK")
	}
	if countIdleLanes([]ledger.FabricLane{{State: "IDLE with work"}, {State: "WORKING"}}) != 0 {
		t.Fatal("display labels/working conflated with canonical idle")
	}
}
