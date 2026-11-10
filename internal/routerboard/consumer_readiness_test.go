package routerboard

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/ledger"
	"github.com/SirsiMaster/sirsi-pantheon/internal/router"
	"github.com/SirsiMaster/sirsi-pantheon/internal/routerstore"
)

// In-memory source fixtures only: no store, native probe, listener or consumer.
func TestGenerationWatcherArmedDoesNotInventConsumerReadiness(t *testing.T) {
	now := time.Date(2026, time.November, 10, 18, 9, 0, 0, time.UTC)
	for _, tc := range []struct {
		name           string
		capable        bool
		hold, outcome  string
		attended       string
		missingRuntime bool
		verdict        string
		wantAvailable  bool
	}{
		{name: "watch-only", verdict: router.VerdictWatchOnly},
		{name: "stale attended plus watch-only", attended: "stale", verdict: router.VerdictWatchOnly},
		{name: "fresh attended needs no worker grant", attended: "fresh", verdict: router.VerdictLive, wantAvailable: true},
		{name: "declared consumer", capable: true, verdict: router.VerdictWakeable, wantAvailable: true},
		{name: "attended hold without fresh session", hold: router.HoldAttended, verdict: router.VerdictHeld},
		{name: "consumer auth failure", capable: true, outcome: router.OutcomeAuthRequired, verdict: router.VerdictAuthRequired},
		{name: "missing canonical readiness", missingRuntime: true, capable: true, verdict: router.VerdictWakeable},
	} {
		t.Run(tc.name, func(t *testing.T) {
			worker := &router.Thread{ThreadID: "private-watcher", AgentID: "lane", Surface: "worker",
				Status: router.ThreadStatusActive, LastSeenAt: now, ConsumerCapable: tc.capable,
				Lane: &router.LaneState{ConsumerDeclared: tc.capable, Hold: tc.hold, LastOutcome: tc.outcome}}
			threads := &router.ThreadRegistry{Threads: map[string]*router.Thread{worker.ThreadID: worker}}
			node := &router.NodeStatus{LiveThreads: []router.ThreadSummary{{ThreadID: worker.ThreadID,
				AgentID: "lane", Surface: "worker", Armed: true, LoopState: "na"}}}
			if tc.attended != "" {
				attended := &router.Thread{ThreadID: "private-attended", AgentID: "lane", Surface: "codex",
					Status: router.ThreadStatusActive, LastSeenAt: now}
				if tc.attended == "stale" {
					attended.LastSeenAt = now.Add(-2 * router.DefaultThreadStaleAfter)
					node.StaleThreads = []router.ThreadSummary{{ThreadID: attended.ThreadID, AgentID: "lane", Surface: "codex", Stale: true}}
				} else {
					node.LiveThreads = append(node.LiveThreads, router.ThreadSummary{ThreadID: attended.ThreadID,
						AgentID: "lane", Surface: "codex", Armed: true, LoopState: "na"})
				}
				threads.Threads[attended.ThreadID] = attended
			}
			ping := router.PingLane(threads, router.AgentConfig{ID: "lane"}, "lane", now)
			if ping.Verdict != tc.verdict {
				t.Fatalf("source availability changed: got %+v, want %s", ping, tc.verdict)
			}
			if ping.Verdict == router.VerdictLive && !strings.Contains(ping.Detail, "consumption is unproven") {
				t.Fatal("attended availability falsely promised inbox consumption")
			}
			input := ledger.FabricRead{
				Snapshot: ledger.Snapshot{GeneratedAt: now.Format(time.RFC3339), Agents: []ledger.Agent{{AgentID: "lane"}}},
				Node:     node,
				Metadata: ledger.FabricMetadata{Lanes: map[string]ledger.FabricLaneMetadata{"lane": {Type: "codex"}}},
			}
			if !tc.missingRuntime {
				input.Evidence.Operational = map[string]routerstore.LaneOperationalState{"lane": {
					Agent: "lane", Classification: routerstore.LaneIdleWithWork, WakeRoutable: ping.Workable,
				}}
			}
			before, err := json.Marshal(input)
			if err != nil {
				t.Fatal(err)
			}
			threadBefore, err := json.Marshal(threads)
			if err != nil {
				t.Fatal(err)
			}
			producer := ledger.NewFabricGenerationProducer(func() (ledger.FabricRead, error) { return input, nil }, "private-source-fixture")
			t.Cleanup(producer.Close)
			generation, err := producer.Read(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			payload, err := FromGeneration(generation, "private-ui-fixture")
			if err != nil {
				t.Fatal(err)
			}
			if len(payload.Fleet) != 1 || payload.Fleet[0].Consumer == nil {
				t.Fatal("watcher observation lost")
			}
			consumer := payload.Fleet[0].Consumer
			if !consumer.Armed || consumer.Loop != "na" || consumer.Consumption != "unknown" {
				t.Fatalf("watcher liveness and consumer availability conflated: %+v", consumer)
			}
			if tc.missingRuntime {
				if consumer.RoutingAvailable != nil {
					t.Fatal("missing availability was fabricated")
				}
			} else if consumer.RoutingAvailable == nil || *consumer.RoutingAvailable != tc.wantAvailable {
				t.Fatalf("routing availability must not imply consumption or change watcher state: %+v", consumer)
			}
			// Preserve the canonical operational state, not a heartbeat-derived WORKING.
			wantState := routerstore.LaneIdleWithWork
			if tc.missingRuntime {
				wantState = "UNKNOWN"
			}
			if payload.Fleet[0].State != wantState {
				t.Fatalf("projection invented work progress: %+v", payload.Fleet[0])
			}
			after, err := json.Marshal(input)
			if err != nil {
				t.Fatal(err)
			}
			threadAfter, err := json.Marshal(threads)
			if err != nil {
				t.Fatal(err)
			}
			if string(before) != string(after) || string(threadBefore) != string(threadAfter) {
				t.Fatal("passive projection changed source state, capability or role")
			}
		})
	}
}
