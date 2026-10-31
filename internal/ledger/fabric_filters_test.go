package ledger

import (
	"errors"
	"fmt"
	"testing"

	"github.com/SirsiMaster/sirsi-pantheon/internal/routerstore"
)

func TestFabricFilteredCountsEqualCanonicalHeadlines(t *testing.T) {
	s := Snapshot{Agents: []Agent{{AgentID: "lane"}}}
	for i, status := range []string{"done", "pending", "in-progress", "blocked", "", "unknown"} {
		for j, liveness := range []string{"active", "stalled", "unknown"} {
			s.Agents[0].Tasks = append(s.Agents[0].Tasks, routerstore.Task{TaskID: fmt.Sprintf("task-%d-%d", i, j), Status: status, Liveness: liveness, BlockedBy: "private prose"})
		}
	}
	w := fabricFacts(s).Work
	counts := map[string]int{"": w.Open, "open": w.Open, "done": w.Done, "in_progress": w.InProgress, "assigned_not_started": w.AssignedNotStarted, "stalled": w.Stalled, "blocked": w.Blocked}
	for filter, want := range counts {
		p, err := FabricTasksFilteredFrom(s, 0, "", filter)
		if err != nil || p.Total != want || len(p.Rows) != want || p.Filter != filter {
			t.Fatalf("filter=%q: page=%+v err=%v want=%d", filter, p, err, want)
		}
		if filter == "done" {
			for _, row := range p.Rows {
				if row.Status != "done" {
					t.Fatal("completed task projected with a different status")
				}
			}
		}
	}
}

func TestFabricFilterPrecedesPagingAndBindsSelectionAndMutation(t *testing.T) {
	s := fabricTaskFixture(260)
	for i := 130; i < 260; i++ {
		s.Agents[0].Tasks[i].Status = "blocked"
	}
	first, err := FabricTasksFilteredFrom(s, 0, "", "blocked")
	if err != nil || first.Total != 130 || first.Rows[0].ID != "task-130" || first.NextOffset == nil {
		t.Fatalf("filter applied to a page instead of full authority: %+v %v", first, err)
	}
	second, err := FabricTasksFilteredFrom(s, 64, first.Snapshot, "blocked")
	if err != nil || second.Rows[0].ID != "task-194" {
		t.Fatalf("filtered continuation skipped rows: %+v %v", second, err)
	}
	if _, err := FabricTasksFilteredFrom(s, 64, first.Snapshot, "open"); !errors.Is(err, ErrFabricTasksChanged) {
		t.Fatal("cursor crossed filter boundary", err)
	}
	s.Agents[0].Tasks[259].Subject = "late same-selection change"
	if _, err := FabricTasksFilteredFrom(s, 64, first.Snapshot, "blocked"); !errors.Is(err, ErrFabricTasksChanged) {
		t.Fatal("tail substitution accepted", err)
	}
	if _, err := FabricTasksFilteredFrom(s, 0, "", "status=blocked"); err == nil {
		t.Fatal("arbitrary filter accepted")
	}
}
