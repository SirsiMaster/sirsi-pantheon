package dashboard

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/ledger"
	"github.com/SirsiMaster/sirsi-pantheon/internal/router"
	"github.com/SirsiMaster/sirsi-pantheon/internal/routerboard"
	"github.com/SirsiMaster/sirsi-pantheon/internal/routerstore"
	"github.com/SirsiMaster/sirsi-pantheon/internal/work"
)

type fixedFabricGeneration struct {
	generation *ledger.FabricGeneration
	err        error
}

func (f fixedFabricGeneration) Read(context.Context) (*ledger.FabricGeneration, error) {
	return f.generation, f.err
}
func (f fixedFabricGeneration) ReadPage(ctx context.Context, offset int, digest string) (*ledger.FabricGeneration, error) {
	g, err := f.Read(ctx)
	if err != nil {
		return nil, err
	}
	v, err := g.View()
	if err != nil {
		return nil, err
	}
	_, err = ledger.FabricTasksFrom(v.Read.Snapshot, offset, digest)
	return g, err
}

func mixedFabricGeneration(t *testing.T) (*ledger.FabricGeneration, *int) {
	t.Helper()
	stamp := "2026-10-25T12:00:00Z"
	charter := "inert legacy charter"
	outline := `[{"id":"one","step":"Inert step","done":false}]`
	input := ledger.FabricRead{Snapshot: ledger.Snapshot{GeneratedAt: stamp, Agents: []ledger.Agent{
		{AgentID: "worker", Items: []ledger.Item{{ID: "message-a", Blocked: true}, {ID: "message-b", Blocked: true}}, Tasks: []routerstore.Task{
			{Agent: "worker", TaskID: "pending", Subject: "Pending", Status: "pending", Liveness: "unknown", Updated: "2000-01-01T00:00:00Z", Phase: "Build", Charter: &charter, Outline: &outline, Timeline: []routerstore.TimelineEntry{{Day: "1", Owner: "fixture", Hours: 1, Label: "inert"}}, Links: []routerstore.TaskLink{{Kind: "evidence", Label: "fixture", URL: "https://github.com/SirsiMaster/sirsi-pantheon/pull/1"}}},
			{Agent: "worker", TaskID: "active", Subject: "Active", Status: "in-progress", Liveness: "active", Updated: stamp, Phase: "Build"},
			{Agent: "worker", TaskID: "stalled", Subject: "Stalled", Status: "in-progress", Liveness: "stalled", Updated: stamp},
			{Agent: "worker", TaskID: "blocked", Subject: "Blocked", Status: "blocked", Liveness: "blocked", Updated: stamp, BlockedBy: "pending"},
			{Agent: "worker", TaskID: "done", Subject: "Done", Status: "done", Liveness: "unknown", Updated: stamp},
		}},
		{AgentID: "orphan", Tasks: []routerstore.Task{{Agent: "orphan", TaskID: "unregistered", Subject: "Unregistered", Status: "pending", Liveness: "unknown", Updated: stamp}}},
	}}, Node: &router.NodeStatus{RegisteredAgents: []string{"worker", "horus"}, LiveThreads: []router.ThreadSummary{{AgentID: "worker", ThreadID: "worker-thread", Armed: true, LoopState: "na"}, {AgentID: "horus", ThreadID: "service-thread", Armed: true, LoopState: "na"}}, StaleThreads: []router.ThreadSummary{{AgentID: "orphan", ThreadID: "stale-thread", Stale: true}}},
		Metadata: ledger.FabricMetadata{Lanes: map[string]ledger.FabricLaneMetadata{"worker": {Type: "codex", WakeMechanism: "none"}, "horus": {Type: "service", WakeMechanism: "none"}}, Threads: []ledger.FabricThreadMetadata{{ID: "worker-thread", Agent: "worker", Workstream: "fixture"}, {ID: "service-thread", Agent: "horus", Workstream: "service", Stale: false}, {ID: "stale-thread", Agent: "orphan", Stale: true}}, Errors: []string{"inert-consumer-probe-unavailable"}},
		Evidence: ledger.FabricEvidence{Operational: map[string]routerstore.LaneOperationalState{"worker": {Agent: "worker", Classification: routerstore.LaneIdleWithWork}, "orphan": {Agent: "orphan", Classification: routerstore.LaneUnroutable}, "horus": {Agent: "horus", Classification: routerstore.LaneComplete}}}}
	// Real acquisition shape: BuildFrom omits taskless registry-only Horus.
	// The shared producer, not this fixture or a legacy adapter, must add it.
	tasks := []routerstore.Task{}
	for _, agent := range input.Snapshot.Agents {
		tasks = append(tasks, agent.Tasks...)
	}
	at, e := time.Parse(time.RFC3339Nano, stamp)
	if e != nil {
		t.Fatal(e)
	}
	input.Snapshot = ledger.BuildFrom([]work.Item{
		{ID: "message-a", To: "worker", Status: "open", Opened: stamp, BlockedBy: "absent-a"},
		{ID: "message-b", To: "worker", Status: "open", Opened: stamp, BlockedBy: "absent-b"},
	}, tasks, nil, "", at, ledger.DefaultStaleAfter)
	for i := range input.Snapshot.Agents {
		if input.Snapshot.Agents[i].AgentID == "horus" {
			t.Fatal("fixture unexpectedly supplied registry-only service")
		}
	}
	calls := new(int)
	p := ledger.NewFabricGenerationProducer(func() (ledger.FabricRead, error) {
		*calls++
		if *calls > 1 {
			return ledger.FabricRead{}, errors.New("adapter reacquired authority")
		}
		return input, nil
	}, "fixture")
	g, err := p.Read(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return g, calls
}

func TestFabricGenerationMixedExistingConsumersShareFactsAndNoReacquisition(t *testing.T) {
	g, calls := mixedFabricGeneration(t)
	v, err := g.View()
	if err != nil {
		t.Fatal(err)
	}
	legacy, err := routerboard.FromGeneration(g, "ui-fixture")
	if err != nil {
		t.Fatal(err)
	}
	fleet, err := FleetFromGeneration(g)
	if err != nil {
		t.Fatal(err)
	}
	horus, err := BindRouterFabric(RouterSnapshot{Version: "Ra-fixture", Consumers: RouterConsumers{Running: 1, Max: 2}}, g)
	if err != nil {
		t.Fatal(err)
	}
	w := v.Board.Work
	if w.Total != 6 || w.Done != 1 || w.Open != 5 || w.InProgress != 1 || w.AssignedNotStarted != 2 || w.Stalled != 1 || w.Blocked != 1 {
		t.Fatalf("mixed common facts wrong: %+v", w)
	}
	if !reflect.DeepEqual(*legacy.Fabric, v.Board) || !reflect.DeepEqual(*horus.Fabric, v.Board) || legacy.Generation != v.ID || horus.FabricGeneration != v.ID || legacy.GeneratedAt != v.Board.GeneratedAt {
		t.Fatal("adapters mixed producer generations")
	}
	if legacy.Board.ActiveTasks != w.Open || v.Facts.Ledger.ActiveTasks != w.Open || legacy.Board.BlockedTasks != 1 || !reflect.DeepEqual(legacy.Board, legacy.Ledger) {
		t.Fatal("inclusive active double counted or diverged")
	}
	if v.Facts.Ledger.BlockedItems != 2 || legacy.Board.BlockedItems != 1 {
		t.Fatal("task and inbox blocker scopes relabeled")
	}
	if fleet.Summary.Active != w.InProgress || fleet.Summary.Stalled != w.Stalled || fleet.Summary.Assigned != w.AssignedNotStarted || fleet.Summary.LanesWork != 0 || fleet.Summary.IdleLanes != 1 || ledger.IdleFabricLanes(v.Board.Lanes) != 1 {
		t.Fatal("fleet/menu inferred runtime or recounted work")
	}
	byID := map[string]routerboard.TaskDetail{}
	for _, task := range legacy.Tasks {
		byID[task.TaskID] = task
	}
	if byID["stalled"].Liveness != "stalled" || byID["pending"].Liveness != "unknown" {
		t.Fatal("adapter recalculated canonical liveness from local age")
	}
	if byID["pending"].Charter == nil || byID["pending"].Outline == nil || len(byID["pending"].Timeline) != 1 || len(byID["pending"].Links) != 1 || byID["pending"].Phase != "Build" {
		t.Fatal("full Nexus detail was replaced with safe-row subset")
	}
	serviceSeen, orphanSeen := false, false
	for _, lane := range legacy.Fleet {
		serviceSeen = serviceSeen || lane.Agent == "horus"
		orphanSeen = orphanSeen || lane.Agent == "orphan"
		for _, status := range []string{"done", "in-progress", "pending", "blocked"} {
			if _, known := lane.Counts[status]; !known {
				t.Fatalf("legacy captured lane %s omitted known-zero status %s", lane.Agent, status)
			}
		}
		if lane.Agent == "orphan" && !reflect.DeepEqual(lane.Counts, map[string]int{"done": 0, "in-progress": 0, "pending": 1, "blocked": 0}) {
			t.Fatalf("pending-only legacy lane needs explicit 0 / 1 completed output: %+v", lane.Counts)
		}
		if lane.Agent == "horus" && !reflect.DeepEqual(lane.Counts, map[string]int{"done": 0, "in-progress": 0, "pending": 0, "blocked": 0}) {
			t.Fatalf("taskless legacy service needs explicit 0 / 0 completed output: %+v", lane.Counts)
		}
		if lane.Agent == "orphan" && (lane.Registered.Router || lane.State != "UNROUTABLE" || lane.Consumer == nil || lane.Consumer.Armed || lane.Consumer.Loop != "unknown") {
			t.Fatal("missing/stale consumer promoted to registered/healthy")
		}
		if lane.Agent == "horus" && (lane.Type != "service" || lane.State != "COMPLETE" || !lane.Registered.Router || !lane.Registered.Thread || lane.Registered.Ledger || lane.TasksTotal != 0 || len(lane.Tasks) != 0) {
			t.Fatal("service became worker/actor")
		}
	}
	if !serviceSeen || !orphanSeen || len(legacy.Fleet) != 3 || len(v.Board.Lanes) != 3 {
		t.Fatal("registry/ledger lane union lost taskless service or unregistered work")
	}
	if len(legacy.DataErrors) != 1 || horus.Consumers.Running != 1 || horus.Consumers.Max != 2 {
		t.Fatal("consumer errors/authority plane changed")
	}
	for _, id := range legacy.RegistrationGaps {
		if id == "horus" {
			t.Fatal("service was treated as missing worker task/registration coverage")
		}
	}
	if *calls != 1 {
		t.Fatal("adapter reacquired authority")
	}
	if path := os.Getenv("PANTHEON_TEST_FABRIC_GENERATION_WIRE_OUTPUT"); path != "" {
		file, e := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if e != nil {
			t.Fatal(e)
		}
		e = json.NewEncoder(file).Encode(map[string]any{"fabric": v.Board, "legacy": legacy, "ledger": v.Facts.Ledger, "fleet": fleet, "horus": horus})
		syncErr := file.Sync()
		closeErr := file.Close()
		if e != nil || syncErr != nil || closeErr != nil {
			t.Fatal(e, syncErr, closeErr)
		}
	}
}

func TestFabricGenerationExistingHTTPShapesAndReadOnlyBoundary(t *testing.T) {
	g, calls := mixedFabricGeneration(t)
	frozen := fixedFabricGeneration{generation: g}
	s := New(Config{FabricSource: frozen, RouterFn: func() (RouterSnapshot, error) { return RouterSnapshot{Version: "Ra-fixture"}, nil }})
	for _, path := range []string{"/api/fabric", "/api/ledger", "/api/fleet", "/api/router/v1/snapshot"} {
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, path, nil)
		r.Header.Set("Origin", "http://localhost:5173")
		s.handler.ServeHTTP(w, r)
		if w.Code != 200 || w.Header().Get("Cache-Control") != "no-store" || w.Header().Get("X-Sirsi-Fabric-Generation") == "" {
			t.Fatalf("existing route %s failed: %d %s", path, w.Code, w.Body.String())
		}
	}
	board := routerboard.NewWithProducer("/must-not-execute", "/must-not-read", "ui-fixture", frozen.Read)
	board.Poll(context.Background())
	mux := http.NewServeMux()
	routerboard.NewHandler(board, "../routerboard/ui").RegisterReadOnly(mux)
	for _, path := range []string{"/api/router/v1/snapshot", "/api/router/v1/ledger", "/api/router/v1/tasks", "/api/ledger", "/api/tasks"} {
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, path, nil)
		r.Header.Set("Origin", "http://localhost:5173")
		mux.ServeHTTP(w, r)
		if w.Code != 200 || w.Header().Get("X-Sirsi-Router-Schema") != "router-board.v1" || w.Header().Get("Cache-Control") != "no-store" || w.Header().Get("Access-Control-Allow-Origin") != "http://localhost:5173" {
			t.Fatalf("legacy route %s wrong: %d", path, w.Code)
		}
		post := httptest.NewRecorder()
		mux.ServeHTTP(post, httptest.NewRequest(http.MethodPost, path, nil))
		if post.Code != 405 {
			t.Fatal("read-only method widened", path, post.Code)
		}
	}
	arm := httptest.NewRecorder()
	mux.ServeHTTP(arm, httptest.NewRequest(http.MethodGet, "/api/arm?agent=worker", nil))
	if arm.Code != 404 {
		t.Fatal("read-only adapter exposed arm mutation")
	}
	manifest := httptest.NewRecorder()
	s.handler.ServeHTTP(manifest, httptest.NewRequest(http.MethodGet, "/api/router/v1/manifest", nil))
	if !strings.Contains(manifest.Body.String(), `"schema":"router-surface.v1"`) {
		t.Fatal("Horus schema relabeled")
	}
	if *calls != 1 {
		t.Fatal("HTTP shape adapter reacquired source")
	}
}

func TestFabricGenerationUnavailableReadDoesNotServeHealthyZero(t *testing.T) {
	frozen := fixedFabricGeneration{err: errors.New("inert source unavailable")}
	s := New(Config{FabricSource: frozen, RouterFn: func() (RouterSnapshot, error) { return RouterSnapshot{}, nil }})
	for _, path := range []string{"/api/ledger", "/api/fleet", "/api/router/v1/snapshot"} {
		w := httptest.NewRecorder()
		s.handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
		if w.Code != 503 {
			t.Fatal("unavailable producer became success", path, w.Code)
		}
	}
}
