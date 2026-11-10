package ledger

import (
	"testing"

	"github.com/SirsiMaster/sirsi-pantheon/internal/routerstore"
)

// Canceled and withdrawn tasks are closed, not done: off the board totals entirely.
func TestClosedNotDoneTasksLeaveTheBoard(t *testing.T) {
	f := fabricFacts(Snapshot{Agents: []Agent{{AgentID: "lane", Tasks: []routerstore.Task{
		{Status: "done"}, {Status: "pending"}, {Status: "canceled"}, {Status: "withdrawn"},
	}}}})
	if f.Ledger.TotalTasks != 2 || f.Ledger.DoneTasks != 1 || f.Ledger.ActiveTasks != 1 || f.Ledger.PctDone != 50 {
		t.Fatalf("board = %+v", f.Ledger)
	}
	if c := f.Lanes["lane"].Counts; c["withdrawn"] != 1 || c["canceled"] != 1 {
		t.Fatalf("lane counts lost the closed tasks: %v", c)
	}
}
