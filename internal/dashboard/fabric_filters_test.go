package dashboard

import (
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/ledger"
	"github.com/SirsiMaster/sirsi-pantheon/internal/routerstore"
)

func TestFabricFilterEndpointUsesFullSelectionAndRejectsCursorDrift(t *testing.T) {
	read := fabricReadFixture("pending", time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC))
	read.Snapshot.Agents[0].Tasks = nil
	for i := 0; i < 140; i++ {
		status := "pending"
		if i >= 70 {
			status = "blocked"
		}
		read.Snapshot.Agents[0].Tasks = append(read.Snapshot.Agents[0].Tasks, routerstore.Task{TaskID: fmt.Sprintf("task-%03d", i), Status: status})
	}
	s := New(Config{FabricReadFn: func() (ledger.FabricRead, error) { return read, nil }})
	get := func(path string, want int) ledger.FabricBoard {
		t.Helper()
		w := httptest.NewRecorder()
		s.apiFabric(w, httptest.NewRequest("GET", path, nil))
		if w.Code != want {
			t.Fatalf("%s: status%d body%s", path, w.Code, w.Body.String())
		}
		var b ledger.FabricBoard
		if want == 200 {
			if err := json.Unmarshal(w.Body.Bytes(), &b); err != nil {
				t.Fatal(err)
			}
		}
		return b
	}
	first := get("/api/fabric?tasks_filter=blocked", 200)
	if first.Tasks.Total != first.Work.Blocked || first.Tasks.Total != 70 || first.Tasks.Rows[0].ID != "task-070" || len(first.Tasks.Rows) != 64 {
		t.Fatal("headline did not drill into complete blocked selection", first.Tasks)
	}
	path := "/api/fabric?tasks_filter=blocked&tasks_offset=64&tasks_snapshot=" + first.Tasks.Snapshot
	last := get(path, 200)
	if len(last.Tasks.Rows) != 6 || last.Tasks.Rows[5].ID != "task-139" {
		t.Fatal("last six blocked rows inaccessible")
	}
	get("/api/fabric?tasks_filter=open&tasks_offset=64&tasks_snapshot="+first.Tasks.Snapshot, 409)
	read.Snapshot.Agents[0].Tasks[139].Status = "in-progress"
	get(path, 409)
	fresh := get("/api/fabric?tasks_filter=blocked", 200)
	if fresh.Tasks.Total != 69 || len(fresh.Activity) != 1 || fresh.Activity[0].TaskID != "task-139" {
		t.Fatal("rejected selection corrupted transition baseline or refresh totals")
	}
}

func TestFabricFilterMalformedBeforeAuthorityRead(t *testing.T) {
	reads := 0
	s := New(Config{FabricReadFn: func() (ledger.FabricRead, error) { reads++; return ledger.FabricRead{}, nil }})
	for _, query := range []string{"tasks_filter=", "tasks_filter=unknown", "tasks_filter=blocked&tasks_filter=done", "tasks_filter=blocked&extra=1", "tasks_filter=blocked&tasks_offset=0", "tasks_filter=blocked&tasks_offset=64", "tasks_filter=%XX"} {
		w := httptest.NewRecorder()
		s.apiFabric(w, httptest.NewRequest("GET", "/api/fabric?"+query, nil))
		if w.Code != 400 || reads != 0 {
			t.Fatalf("invalid filter read authority: query%q status%d reads%d", query, w.Code, reads)
		}
	}
}

func TestFabricFilterRefusesProducerWithoutCompleteSelectionSupport(t *testing.T) {
	reads := 0
	s := New(Config{FabricFn: func() (ledger.FabricBoard, error) {
		reads++
		return ledger.FabricBoard{}, nil
	}})
	w := httptest.NewRecorder()
	s.apiFabric(w, httptest.NewRequest("GET", "/api/fabric?tasks_filter=blocked", nil))
	if w.Code != 503 || reads != 0 {
		t.Fatalf("partial producer claimed a filtered selection: status%d reads%d", w.Code, reads)
	}
}
