package dashboard

import (
	"context"
	"errors"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/ledger"
	"github.com/SirsiMaster/sirsi-pantheon/internal/router"
	"github.com/SirsiMaster/sirsi-pantheon/internal/routerstore"
)

func TestFleetReadinessRefreshPreservesHistory(t *testing.T) {
	ft := NewFleetTracker(map[string]bool{"lane": true})
	now := time.Now()
	ft.Observe(snapOf("lane", task("one", "pending")), now)
	got := ft.ObserveWithRoutability(snapOf("lane", task("one", "in-progress")), now, map[string]bool{"lane": false})
	if len(got.Lanes) != 1 || !got.Lanes[0].Routable || len(got.Activity) != 1 {
		t.Fatalf("fresh readiness/history lost: %+v", got)
	}
	got = ft.ObserveWithRoutability(snapOf("lane", task("one", "in-progress")), now, map[string]bool{"lane": true})
	if got.Lanes[0].Routable || len(got.Activity) != 1 {
		t.Fatalf("stale readiness/history lost: %+v", got)
	}
}

func TestFleetReadinessFailureIsUnavailable(t *testing.T) {
	for _, result := range []struct {
		value map[string]bool
		err   error
	}{{nil, errors.New("offline")}, {nil, nil}} {
		s := New(Config{FleetFn: func() (ledger.Snapshot, error) { return snapOf("lane", task("one", "pending")), nil }, RoutabilityFn: func() (map[string]bool, error) { return result.value, result.err }})
		w := httptest.NewRecorder()
		s.apiFleet(w, httptest.NewRequest("GET", "/api/fleet", nil))
		if w.Code != 502 {
			t.Fatalf("unavailable readiness returned %d: %s", w.Code, w.Body.String())
		}
	}
}

func TestFleetReadinessUnknownLaneIsNotHealthy(t *testing.T) {
	got := NewFleetTracker(nil).ObserveWithRoutability(snapOf("undeclared", task("one", "pending")), time.Now(), map[string]bool{"declared": false})
	if len(got.Lanes) != 1 || got.Lanes[0].Routable {
		t.Fatalf("unknown lane treated as routable: %+v", got)
	}
}

func TestFleetMetadataDoesNotProveWorkAndFencedWorkSurvivesUnavailableWake(t *testing.T) {
	now := time.Date(2026, time.November, 10, 18, 9, 0, 0, time.UTC)
	for _, status := range []string{"pending", "blocked", "in-progress"} {
		for _, unavailable := range []bool{false, true} {
			t.Run(status+"/unavailable="+map[bool]string{false: "false", true: "true"}[unavailable], func(t *testing.T) {
				updated := task("private-task", status)
				updated.Updated = now.Format(time.RFC3339)
				snapshot := snapOf("lane", updated)
				tracker := NewFleetTracker(nil)
				got := tracker.ObserveWithRoutability(snapshot, now, map[string]bool{"lane": unavailable})
				want := LaneIdleWithWork
				if unavailable {
					want = LaneUnroutable
				}
				if status == "blocked" {
					want = LaneBlocked
				}
				if len(got.Lanes) != 1 || got.Lanes[0].State != want || got.Lanes[0].Routable == unavailable {
					t.Fatalf("metadata-only update became work or wake evidence: %+v", got)
				}
				if len(got.Activity) != 0 || !reflect.DeepEqual(snapshot.Agents[0].Tasks[0], updated) {
					t.Fatal("passive observations changed tasks or invented a transition")
				}
			})
		}
	}
	t.Run("canonical-private-claim", func(t *testing.T) {
		store, err := routerstore.OpenPath(filepath.Join(t.TempDir(), "private.db"))
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = store.Close() })
		if err := store.AddTask(routerstore.Task{Agent: "lane", TaskID: "private-task", Subject: "inert fixture"}); err != nil {
			t.Fatal(err)
		}
		unclaimed, err := store.ClassifyLane("lane", false, time.Minute)
		if err != nil || unclaimed.Classification != routerstore.LaneUnroutable || unclaimed.RecentMutation || unclaimed.LiveLeases != 0 {
			t.Fatal("fresh unclaimed metadata is not work evidence", err)
		}
		if _, err := store.ClaimTask("lane", "private-task", "private-worker", "private-thread", 5*time.Minute); err != nil {
			t.Fatal(err)
		}
		state, err := store.ClassifyLane("lane", false, time.Minute)
		if err != nil || state.Classification != routerstore.LaneWorking || !state.RecentMutation || state.LiveLeases != 1 || state.WakeRoutable {
			t.Fatal("canonical fenced work lost behind unavailable wake", err)
		}
		before, err := store.ListTasks("lane")
		if err != nil {
			t.Fatal(err)
		}
		input := ledger.FabricRead{Snapshot: ledger.Snapshot{GeneratedAt: time.Now().UTC().Format(time.RFC3339Nano), Agents: []ledger.Agent{{AgentID: "lane", Tasks: before}}},
			Node: &router.NodeStatus{}, Evidence: ledger.FabricEvidence{Operational: map[string]routerstore.LaneOperationalState{"lane": state}}}
		producer := ledger.NewFabricGenerationProducer(func() (ledger.FabricRead, error) { return input, nil }, "private-fenced-fixture")
		t.Cleanup(producer.Close)
		generation, err := producer.Read(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		got, err := FleetFromGeneration(generation)
		if err != nil || len(got.Lanes) != 1 || got.Lanes[0].State != LaneWorking || got.Lanes[0].Routable {
			t.Fatal("fleet adapter hid canonical work or granted wake readiness", err)
		}
		after, err := store.ListTasks("lane")
		if err != nil || !reflect.DeepEqual(before, after) {
			t.Fatal("passive fleet projection changed private task state", err)
		}
	})
}
