package dashboard

import (
	"encoding/json"
	"errors"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/ledger"
	"github.com/SirsiMaster/sirsi-pantheon/internal/router"
	"github.com/SirsiMaster/sirsi-pantheon/internal/routerstore"
)

func fabricReadFixture(status string, at time.Time) ledger.FabricRead {
	zero := int64(0)
	total := int64(48 << 30)
	pct := float64(23)
	cores := 12
	dirty := false
	return ledger.FabricRead{Snapshot: ledger.Snapshot{GeneratedAt: at.UTC().Format(time.RFC3339), Agents: []ledger.Agent{{AgentID: "lane", Items: []ledger.Item{{ID: "message", AgeSeconds: 30}}, Tasks: []routerstore.Task{{TaskID: "task", Subject: "Inert open task", Status: status, Stage: "build", TestState: "untested", ResponsibleParty: "fixture-owner", CommissionedBy: "fixture-lead", CommissionedAt: at.UTC().Format(time.RFC3339), Updated: at.UTC().Format(time.RFC3339)}, {TaskID: "done", Status: "done"}}}}}, Node: &router.NodeStatus{}, Evidence: ledger.FabricEvidence{Operational: map[string]routerstore.LaneOperationalState{"lane": {Agent: "lane", Classification: routerstore.LaneIdleWithWork, WakeRoutable: true, Runnable: routerstore.RunnableState{Agent: "lane", OpenRouterItems: 1, ActionableLedgerTasks: 1, Runnable: true}}}, Health: &ledger.FabricHealth{RAMPct: &pct, Swap: &zero, TotalRAM: &total, CPUCores: &cores, GitDirty: &dirty, Engine: "inert-engine", EngineState: "observed_session", Issues: []string{}, ObservedAt: at.UTC().Format(time.RFC3339)}}}
}
func TestFabricJoinedEndpointRealTransitionAndCrossSurfaceWire(t *testing.T) {
	at := time.Date(2026, 10, 25, 12, 0, 0, 0, time.UTC)
	status := "pending"
	s := New(Config{FabricBuild: "fixture137", FabricReadFn: func() (ledger.FabricRead, error) { return fabricReadFixture(status, at), nil }})
	read := func() (ledger.FabricBoard, []byte) {
		t.Helper()
		w := httptest.NewRecorder()
		s.apiFabric(w, httptest.NewRequest("GET", "/api/fabric", nil))
		if w.Code != 200 {
			t.Fatalf("endpoint %d: %s", w.Code, w.Body.String())
		}
		var board ledger.FabricBoard
		if err := json.Unmarshal(w.Body.Bytes(), &board); err != nil {
			t.Fatal(err)
		}
		return board, w.Body.Bytes()
	}
	first, _ := read()
	if first.ActivitySeeded || len(first.Activity) != 0 {
		t.Fatal("first read invented events/coverage")
	}
	status = "in-progress"
	at = at.Add(time.Second)
	board, raw := read()
	if !board.ActivitySeeded || len(board.Activity) != 1 || board.Activity[0].From != "pending" || board.Activity[0].To != "in-progress" {
		t.Fatalf("real transition missing: %+v", board)
	}
	if board.Work.Total != 2 || board.Work.Done != 1 || board.Work.Open != 1 || board.Messages.Open != 1 || board.Lanes[0].State == "WORKING" {
		t.Fatalf("stream/runtime mismatch: %+v", board)
	}
	if board.Health.Swap == nil || *board.Health.Swap != 0 || board.Health.Engine != "inert-engine" {
		t.Fatal("known zero or engine fact lost")
	}
	quiet, _ := read()
	if len(quiet.Activity) != 1 {
		t.Fatal("unchanged poll invented another event")
	}
	if path := os.Getenv("PANTHEON_TEST_FABRIC_WIRE_OUTPUT"); path != "" {
		f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err != nil {
			t.Fatal(err)
		}
		_, writeErr := f.Write(raw)
		syncErr := f.Sync()
		closeErr := f.Close()
		if writeErr != nil || syncErr != nil || closeErr != nil {
			t.Fatal(writeErr, syncErr, closeErr)
		}
	}
}
func TestFabricFailedReadDoesNotPolluteTransitionBaseline(t *testing.T) {
	at := time.Date(2026, 10, 25, 12, 0, 0, 0, time.UTC)
	calls := 0
	s := New(Config{FabricReadFn: func() (ledger.FabricRead, error) {
		calls++
		if calls == 2 {
			return ledger.FabricRead{}, errors.New("inert unavailable")
		}
		status := "pending"
		if calls > 2 {
			status = "done"
		}
		return fabricReadFixture(status, at), nil
	}})
	for i := 1; i <= 3; i++ {
		w := httptest.NewRecorder()
		s.apiFabric(w, httptest.NewRequest("GET", "/api/fabric", nil))
		if i == 2 {
			if w.Code != 502 {
				t.Fatal(w.Code)
			}
			continue
		}
		if w.Code != 200 {
			t.Fatal(w.Code)
		}
		var b ledger.FabricBoard
		if err := json.Unmarshal(w.Body.Bytes(), &b); err != nil {
			t.Fatal(err)
		}
		if i == 3 && (len(b.Activity) != 1 || b.Activity[0].From != "pending" || b.Activity[0].To != "done") {
			t.Fatalf("failure reseeded/faked transition: %+v", b.Activity)
		}
	}
}
func TestFabricConcurrentReadsKeepOneRealTransition(t *testing.T) {
	at := time.Date(2026, 10, 25, 12, 0, 0, 0, time.UTC)
	calls := 0
	s := New(Config{FabricReadFn: func() (ledger.FabricRead, error) {
		calls++
		state := "pending"
		if calls > 1 {
			state = "in-progress"
		}
		return fabricReadFixture(state, at), nil
	}})
	done := make(chan struct{}, 8)
	boards := make(chan ledger.FabricBoard, 8)
	for i := 0; i < 8; i++ {
		go func() {
			defer func() { done <- struct{}{} }()
			w := httptest.NewRecorder()
			s.apiFabric(w, httptest.NewRequest("GET", "/api/fabric", nil))
			if w.Code != 200 {
				t.Errorf("endpoint: %d", w.Code)
			}
			var board ledger.FabricBoard
			if err := json.Unmarshal(w.Body.Bytes(), &board); err != nil {
				t.Error(err)
			}
			boards <- board
		}()
	}
	for i := 0; i < 8; i++ {
		<-done
	}
	if calls != 8 {
		t.Fatal(calls)
	}
	observed := 0
	for i := 0; i < 8; i++ {
		board := <-boards
		if len(board.Activity) > 1 {
			t.Fatal("concurrent reads fabricated transitions")
		}
		if len(board.Activity) == 1 {
			observed++
		}
	}
	if observed != 7 {
		t.Fatalf("real transition missing from later generations: %d", observed)
	}
}
