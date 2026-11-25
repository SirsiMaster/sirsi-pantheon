package ledger

import (
	"testing"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/routerstore"
)

// Terminal task statuses are done, canceled and withdrawn. A reader that tests only "done" counts the other two as
// open work (observed: 737 shown vs 370 for a lane carrying 367 withdrawn tasks).
func TestOpenTaskCountExcludesEveryTerminalStatus(t *testing.T) {
	tasks := []routerstore.Task{
		{Agent: "lane", TaskID: "a", Status: "pending"},
		{Agent: "lane", TaskID: "b", Status: "blocked"},
		{Agent: "lane", TaskID: "c", Status: "in-progress"},
		{Agent: "lane", TaskID: "d", Status: "done"},
		{Agent: "lane", TaskID: "e", Status: "canceled"},
		{Agent: "lane", TaskID: "f", Status: "withdrawn"},
		{Agent: "lane", TaskID: "g", Status: "withdrawn"},
	}
	s := BuildFrom(nil, tasks, nil, "lane", time.Now().UTC(), DefaultStaleAfter)
	if len(s.Agents) != 1 {
		t.Fatalf("agents = %d", len(s.Agents))
	}
	a := s.Agents[0]
	if a.OpenTasks != 3 || a.BlockedTasks != 1 {
		t.Fatalf("OpenTasks=%d BlockedTasks=%d, want 3 and 1 (done/canceled/withdrawn are closed)", a.OpenTasks, a.BlockedTasks)
	}
	if len(a.Tasks) != 7 {
		t.Fatalf("history must keep all %d rows, got %d", 7, len(a.Tasks))
	}
	for _, status := range []string{"done", "canceled", "withdrawn"} {
		if fabricTaskMatches(routerstore.Task{Status: status}, "open") {
			t.Fatalf("%s task matched the open drill-down filter", status)
		}
	}
	if !fabricTaskMatches(routerstore.Task{Status: "pending"}, "open") {
		t.Fatal("pending task must match the open filter")
	}
}
