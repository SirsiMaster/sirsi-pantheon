package ledger

import (
	"testing"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/brand"
	"github.com/SirsiMaster/sirsi-pantheon/internal/routerstore"
)

// Canceled and withdrawn tasks remain historical rows, but are neither done nor open work.
func TestClosedNotDoneTasksLeaveTheBoard(t *testing.T) {
	tasks := []routerstore.Task{
		{Agent: "lane", TaskID: "done", Status: "done", Phase: "Build"},
		{Agent: "lane", TaskID: "pending", Status: "pending", Phase: "Build"},
		{Agent: "lane", TaskID: "active", Status: "in-progress", Phase: "Build"},
		{Agent: "lane", TaskID: "blocked", Status: "blocked", Phase: "Build"},
		{Agent: "lane", TaskID: "canceled", Status: "canceled", Liveness: "stalled", Phase: "Build"},
		{Agent: "lane", TaskID: "withdrawn", Status: "withdrawn", Liveness: "stalled", Phase: "Build"},
	}
	s := BuildFrom(nil, tasks, nil, "", time.Now().UTC(), time.Hour)
	f := fabricFacts(s)
	if f.Work.Total != 4 || f.Work.Done != 1 || f.Work.Open != 3 || f.Work.InProgress != 1 || f.Work.AssignedNotStarted != 1 || f.Work.Blocked != 1 {
		t.Fatalf("fabric work counts include a terminal disposition: %+v", f.Work)
	}
	if f.Ledger.TotalTasks != 4 || f.Ledger.DoneTasks != 1 || f.Ledger.ActiveTasks != 3 || f.Ledger.BlockedTasks != 1 || f.Ledger.PctDone != 25 {
		t.Fatalf("board = %+v", f.Ledger)
	}
	if len(f.Ledger.Phases) != 1 || f.Ledger.Phases[0].Total != 4 || f.Ledger.Phases[0].Active != 3 || f.Ledger.Phases[0].Done != 1 {
		t.Fatalf("phase counts include terminal dispositions: %+v", f.Ledger.Phases)
	}
	if s.Agents[0].OpenTasks != 3 || s.Agents[0].BlockedTasks != 1 {
		t.Fatalf("agent counts = open %d blocked %d", s.Agents[0].OpenTasks, s.Agents[0].BlockedTasks)
	}
	if c := f.Lanes["lane"].Counts; c["withdrawn"] != 1 || c["canceled"] != 1 {
		t.Fatalf("lane status counts lost terminal rows: %v", c)
	}
	if c := f.Lanes["lane"].Counts; c["pending"] != 1 || c["in-progress"] != 1 || c["blocked"] != 1 || c["done"] != 1 {
		t.Fatalf("lane counts lost nonterminal rows: %v", c)
	}
	all, err := FabricAllTasksFrom(s)
	if err != nil || len(all) != len(tasks) {
		t.Fatalf("all-task projection rows=%d err=%v", len(all), err)
	}
	statuses := map[string]string{}
	roles := map[string]string{}
	for _, row := range all {
		statuses[row.ID] = row.Status
		roles[row.ID] = row.Role
	}
	if statuses["canceled"] != "canceled" || statuses["withdrawn"] != "withdrawn" || roles["canceled"] != brand.Dim.Name() || roles["withdrawn"] != brand.Dim.Name() {
		t.Fatalf("terminal dispositions were lost or rewritten in task projection: %v", statuses)
	}
	for _, filter := range []string{"", "open", "in_progress", "assigned_not_started", "stalled", "blocked"} {
		page, err := FabricTasksFilteredFrom(s, 0, "", filter)
		if err != nil {
			t.Fatalf("filter %q: %v", filter, err)
		}
		for _, row := range page.Rows {
			if row.Status == "canceled" || row.Status == "withdrawn" {
				t.Fatalf("terminal row %q leaked into %q filter", row.ID, filter)
			}
		}
	}
	done, err := FabricTasksFilteredFrom(s, 0, "", "done")
	if err != nil || done.Total != 1 || len(done.Rows) != 1 || done.Rows[0].Status != "done" {
		t.Fatalf("done filter = %+v err=%v", done, err)
	}
}
