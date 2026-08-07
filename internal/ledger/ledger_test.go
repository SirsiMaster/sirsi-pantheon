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

func TestSummarizePhaseGroups(t *testing.T) {
	tasks := []routerstore.Task{
		{Agent: "a", TaskID: "1", Subject: "s1", Status: "done", Phase: "Infra", ResponsibleParty: "self"},
		{Agent: "a", TaskID: "2", Subject: "s2", Status: "in-progress", Phase: "Infra", ResponsibleParty: "self"},
		{Agent: "a", TaskID: "3", Subject: "s3", Status: "blocked", Phase: "Cross-Repo", ResponsibleParty: "self"},
		{Agent: "a", TaskID: "4", Subject: "s4", Status: "pending", Phase: "", ResponsibleParty: "self"},
	}
	s := BuildFrom(nil, tasks, nil, "", time.Now().UTC(), time.Hour)
	bs := Summarize(s)

	if bs.TotalTasks != 4 || bs.DoneTasks != 1 || bs.BlockedTasks != 1 {
		t.Fatalf("counts wrong: %+v", bs)
	}
	if len(bs.Phases) != 3 {
		t.Fatalf("expected 3 phases (Infra, Cross-Repo, General), got %d: %v", len(bs.Phases), bs.Phases)
	}
	if bs.Phases[0].Name != "Infra" || bs.Phases[0].Total != 2 || bs.Phases[0].Done != 1 || bs.Phases[0].PctDone != 50 {
		t.Fatalf("Infra phase wrong: %+v", bs.Phases[0])
	}
	if bs.Phases[1].Name != "Cross-Repo" || bs.Phases[1].Blocked != 1 {
		t.Fatalf("Cross-Repo phase wrong: %+v", bs.Phases[1])
	}
	if bs.Phases[2].Name != "General" {
		t.Fatalf("General phase wrong: %+v", bs.Phases[2])
	}
}

// Regression: claude-home rendered "0 open · blocked 0 · unblocked/unpicked 0"
// while 41 non-done task rows sat in the SAME output. All three header counters
// are item-only, so an empty inbox zeroed the header regardless of ledger depth
// and a conduit run reading it concluded there was no work. The task counters
// must be non-zero exactly when the registry has non-done rows, independently of
// whether any inbox item exists.
func TestOpenTaskCountsAreIndependentOfInboxItems(t *testing.T) {
	tasks := []routerstore.Task{
		{Agent: "claude-home", TaskID: "1", Subject: "s1", Status: "done", ResponsibleParty: "self"},
		{Agent: "claude-home", TaskID: "2", Subject: "s2", Status: "pending", ResponsibleParty: "self"},
		{Agent: "claude-home", TaskID: "3", Subject: "s3", Status: "in-progress", ResponsibleParty: "self"},
		{Agent: "claude-home", TaskID: "4", Subject: "s4", Status: "blocked", ResponsibleParty: "owner"},
	}
	// nil items == the exact production condition: inbox zero, ledger full.
	s := BuildFrom(nil, tasks, nil, "", time.Now().UTC(), time.Hour)
	if len(s.Agents) != 1 {
		t.Fatalf("expected 1 agent, got %d", len(s.Agents))
	}
	a := s.Agents[0]
	if len(a.Items) != 0 || a.BlockedCount != 0 || a.UnblockedUnpicked != 0 {
		t.Fatalf("item counters must stay item-scoped: %+v", a)
	}
	if a.OpenTasks != 3 {
		t.Fatalf("OpenTasks = %d, want 3 (pending+in-progress+blocked, done excluded)", a.OpenTasks)
	}
	if a.BlockedTasks != 1 {
		t.Fatalf("BlockedTasks = %d, want 1", a.BlockedTasks)
	}
}

func TestSummarizeSemantics(t *testing.T) {
	tests := []struct {
		name        string
		tasks       []routerstore.Task
		wantTotal   int
		wantDone    int
		wantActive  int
		wantBlocked int
		wantPct     int
	}{
		{name: "zero tasks"},
		{
			name: "all done",
			tasks: []routerstore.Task{
				{Agent: "a", TaskID: "1", Status: "done", ResponsibleParty: "self"},
				{Agent: "a", TaskID: "2", Status: "done", ResponsibleParty: "self"},
			},
			wantTotal: 2, wantDone: 2, wantPct: 100,
		},
		{
			name: "blocked not in active",
			tasks: []routerstore.Task{
				{Agent: "a", TaskID: "1", Status: "done", ResponsibleParty: "self"},
				{Agent: "a", TaskID: "2", Status: "in-progress", ResponsibleParty: "self"},
				{Agent: "a", TaskID: "3", Status: "blocked", ResponsibleParty: "self"},
			},
			wantTotal: 3, wantDone: 1, wantActive: 1, wantBlocked: 1, wantPct: 33,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := BuildFrom(nil, tc.tasks, nil, "", time.Now().UTC(), time.Hour)
			bs := Summarize(s)
			if bs.TotalTasks != tc.wantTotal {
				t.Errorf("TotalTasks = %d, want %d", bs.TotalTasks, tc.wantTotal)
			}
			if bs.DoneTasks != tc.wantDone {
				t.Errorf("DoneTasks = %d, want %d", bs.DoneTasks, tc.wantDone)
			}
			if bs.ActiveTasks != tc.wantActive {
				t.Errorf("ActiveTasks = %d, want %d", bs.ActiveTasks, tc.wantActive)
			}
			if bs.BlockedTasks != tc.wantBlocked {
				t.Errorf("BlockedTasks = %d, want %d", bs.BlockedTasks, tc.wantBlocked)
			}
			if bs.PctDone != tc.wantPct {
				t.Errorf("PctDone = %d, want %d", bs.PctDone, tc.wantPct)
			}
		})
	}
}

func TestFabricFromKeepsWorkMessagesAndRegistrationDistinct(t *testing.T) {
	s := Snapshot{GeneratedAt: "2026-08-05T12:00:00Z", Agents: []Agent{
		{AgentID: "agent-a", Items: []Item{{ID: "msg", AgeSeconds: 90}}, Tasks: []routerstore.Task{
			{TaskID: "active", Status: "in-progress", Updated: "2026-08-05T11:59:00Z"},
			{TaskID: "done", Status: "done"},
		}},
		{AgentID: "agent-b", Tasks: []routerstore.Task{{TaskID: "stalled", Status: "pending", Liveness: "stalled"}}},
	}}
	ns := &router.NodeStatus{
		RegisteredAgents: []string{"agent-a", "agent-b"},
		LiveThreads:      []router.ThreadSummary{{ThreadID: "thread-a", AgentID: "agent-a"}},
		StaleThreads:     []router.ThreadSummary{{ThreadID: "thread-b", AgentID: "agent-b"}},
		StrandedInbox:    []router.StrandedAgent{{AgentID: "agent-b", OpenItems: 2}},
	}

	b := FabricFrom(s, ns, "v1")
	if b.Work.Total != 3 || b.Work.Done != 1 || b.Work.Open != 2 || b.Work.InProgress != 1 || b.Work.Stalled != 1 {
		t.Fatalf("work = %+v", b.Work)
	}
	if b.Messages.Open != 1 || b.Messages.OldestAgeSeconds != 90 || b.Messages.Stranded != 2 {
		t.Fatalf("messages = %+v", b.Messages)
	}
	if len(b.Lanes) != 2 || b.Lanes[0].State != "WORKING" || b.Lanes[1].State != "IDLE with work" {
		t.Fatalf("lanes = %+v", b.Lanes)
	}
	if !b.Lanes[0].Registered.Router || !b.Lanes[0].Registered.Thread || !b.Lanes[0].Registered.Ledger {
		t.Fatalf("registration = %+v", b.Lanes[0].Registered)
	}
	if b.Health.RAMPct != nil || b.Health.Swap != nil || b.Health.GitDirty != nil || len(b.Health.Issues) != 1 {
		t.Fatalf("health must preserve unknown values and stale issue: %+v", b.Health)
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
