package ledger

import (
	"testing"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/router"
	"github.com/SirsiMaster/sirsi-pantheon/internal/routerstore"
	"github.com/SirsiMaster/sirsi-pantheon/internal/work"
)

func TestBuildFromClassifiesDependenciesStalenessAndPickup(t *testing.T) {
	now := time.Date(2026, 8, 2, 16, 0, 0, 0, time.UTC)
	items := []work.Item{
		{ID: "dep-open", To: "agent-a", Status: "open", Opened: now.Add(-3 * time.Hour).Format(time.RFC3339), Title: "dependency"},
		{ID: "blocked", To: "agent-a", From: "owner", Type: "decision", Status: "open", Opened: now.Add(-2 * time.Hour).Format(time.RFC3339), Title: "blocked task", BlockedBy: "dep-open"},
		{ID: "picked", To: "agent-a", Status: "open", Opened: now.Add(-time.Hour).Format(time.RFC3339), Title: "picked task"},
		{ID: "dep-done", To: "agent-b", Status: "closed", Opened: now.Add(-5 * time.Hour).Format(time.RFC3339), Title: "done"},
		{ID: "released", To: "agent-b", Status: "open", Opened: now.Add(-30 * time.Minute).Format(time.RFC3339), Title: "released", BlockedBy: "dep-done"},
	}
	threads := &router.ThreadRegistry{Threads: map[string]*router.Thread{
		"t": {ThreadID: "t", AgentID: "agent-a", LastSeenAt: now.Add(-time.Minute), Status: router.ThreadStatusActive, CurrentItem: "picked"},
	}}
	tasks := []routerstore.Task{{Agent: "agent-a", TaskID: "registry-1", Subject: "registered", Status: "pending", ResponsibleParty: "self"}}
	s := BuildFrom(items, tasks, threads, "", now, 4*time.Hour)
	if len(s.Agents) != 2 {
		t.Fatalf("agents = %d", len(s.Agents))
	}
	a := s.Agents[0]
	if a.AgentID != "agent-a" || a.BlockedCount != 1 || a.UnblockedUnpicked != 1 || a.Stale || len(a.Tasks) != 1 {
		t.Fatalf("agent-a = %+v", a)
	}
	if !a.Items[2].Picked { // deterministic id order: blocked, dep-open, picked
		t.Fatalf("picked item not classified: %+v", a.Items)
	}
	b := s.Agents[1]
	if !b.Stale || b.BlockedCount != 0 || b.UnblockedUnpicked != 1 {
		t.Fatalf("agent-b = %+v", b)
	}
}

func TestDependencyCycleAndMissingFailClosed(t *testing.T) {
	byID := map[string]work.Item{
		"a": {ID: "a", Status: "open", BlockedBy: "b"},
		"b": {ID: "b", Status: "open", BlockedBy: "a"},
	}
	if chain, blocked := dependencyChain("a", byID); !blocked || chain[len(chain)-1] != "cycle" {
		t.Fatalf("cycle = %v blocked=%v", chain, blocked)
	}
	if chain, blocked := dependencyChain("missing-id", byID); !blocked || chain[len(chain)-1] != "missing" {
		t.Fatalf("missing = %v blocked=%v", chain, blocked)
	}
}
