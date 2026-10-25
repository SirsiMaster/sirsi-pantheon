package dashboard

import (
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/ledger"
	"github.com/SirsiMaster/sirsi-pantheon/internal/routerstore"
)

func TestFabricTaskEndpointContinuesSameSnapshotAndRefusesDrift(t *testing.T) {
	read := fabricReadFixture("pending", time.Date(2026, 10, 25, 12, 0, 0, 0, time.UTC))
	read.Snapshot.Agents[0].Tasks = nil
	for i := 0; i < 65; i++ {
		read.Snapshot.Agents[0].Tasks = append(read.Snapshot.Agents[0].Tasks, routerstore.Task{TaskID: fmt.Sprintf("task-%03d", i), Subject: "Owned public task", Status: "pending"})
	}
	s := New(Config{FabricReadFn: func() (ledger.FabricRead, error) { return read, nil }})
	get := func(path string, want int) ledger.FabricBoard {
		t.Helper()
		w := httptest.NewRecorder()
		s.apiFabric(w, httptest.NewRequest("GET", path, nil))
		if w.Code != want || w.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("%s: %d %s", path, w.Code, w.Body.String())
		}
		var board ledger.FabricBoard
		if want == 200 {
			if err := json.Unmarshal(w.Body.Bytes(), &board); err != nil {
				t.Fatal(err)
			}
		}
		return board
	}
	first := get("/api/fabric", 200)
	if first.Work.Open != 65 || first.Tasks.Total != 65 || len(first.Tasks.Rows) != 64 {
		t.Fatal("full aggregates or bounded real task rows lost")
	}
	path := "/api/fabric?tasks_offset=64&tasks_snapshot=" + first.Tasks.Snapshot
	last := get(path, 200)
	if len(last.Tasks.Rows) != 1 || last.Tasks.Rows[0].ID != "task-064" || last.Tasks.NextOffset != nil {
		t.Fatal("tail task not reachable through actual handler")
	}
	read.Snapshot.Agents[0].Tasks[64].Subject = "Changed"
	read.Snapshot.Agents[0].Tasks[64].Status = "in-progress"
	get(path, 409)
	fresh := get("/api/fabric", 200)
	if fresh.Tasks.Snapshot == first.Tasks.Snapshot || len(fresh.Activity) != 1 || fresh.Activity[0].TaskID != "task-064" {
		t.Fatal("fresh list missed changed row, or refused cursor polluted transition baseline")
	}
}

func TestFabricTaskEndpointRejectsMalformedSelectionBeforeAuthorityRead(t *testing.T) {
	reads := 0
	s := New(Config{FabricReadFn: func() (ledger.FabricRead, error) { reads++; return ledger.FabricRead{}, nil }})
	for _, query := range []string{"tasks_offset=64", "tasks_offset=64&tasks_snapshot=bad", "tasks_offset=64&tasks_offset=128&tasks_snapshot=" + strings.Repeat("a", 64), "tasks_offset=1&tasks_snapshot=" + strings.Repeat("a", 64), "tasks_offset=064&tasks_snapshot=" + strings.Repeat("a", 64), "Tasks_offset=64&tasks_snapshot=" + strings.Repeat("a", 64), "unexpected=secret", "tasks_offset=%XX"} {
		w := httptest.NewRecorder()
		s.apiFabric(w, httptest.NewRequest("GET", "/api/fabric?"+query, nil))
		if w.Code != 400 || reads != 0 {
			t.Fatalf("invalid query caused authority read: %q status=%d reads=%d", query, w.Code, reads)
		}
	}
}

func TestFabricPresentationPaletteAgreesWithServedBrandTokens(t *testing.T) {
	s := New(Config{})
	w := httptest.NewRecorder()
	s.handleUIAsset(w, httptest.NewRequest("GET", "/assets/tokens.css", nil))
	board := ledger.FabricFrom(ledger.Snapshot{}, nil, "fixture")
	if w.Code != 200 {
		t.Fatal(w.Code)
	}
	for role, swatch := range board.Presentation.Palette {
		if !strings.Contains(w.Body.String(), "--"+role+": "+swatch.Dark) || !strings.Contains(w.Body.String(), "--"+role+": "+swatch.Light) {
			t.Fatalf("web/native palette drift for %s", role)
		}
	}
}
