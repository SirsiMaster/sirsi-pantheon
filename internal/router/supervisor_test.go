package router

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/dispatch"
	"github.com/SirsiMaster/sirsi-pantheon/internal/routerstore"
	"github.com/SirsiMaster/sirsi-pantheon/internal/work"
)

func writeSupervisorRegistry(t *testing.T, routerRoot, goodCwd string) {
	t.Helper()
	reg := Registry{Agents: map[string]AgentConfig{
		"claude-pantheon": {
			Type:       "claude",
			Command:    []string{"sh", "-c", "true"},
			Cwd:        goodCwd,
			Workstream: "pantheon",
		},
		"codex-pantheon": {
			Type:       "codex",
			Command:    []string{"definitely-not-a-real-sirsi-test-binary"},
			Cwd:        goodCwd,
			Workstream: "pantheon",
		},
		"manual-pantheon": {
			Type:       "worker",
			Cwd:        filepath.Join(goodCwd, "missing"),
			Workstream: "pantheon",
		},
	}}
	data, err := json.MarshalIndent(reg, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(routerRoot, "agents.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestSuperviseOnceIncludesUnregisteredDurableWorkOwner(t *testing.T) {
	repoRoot := t.TempDir()
	routerRoot := filepath.Join(repoRoot, ".agents", "idea-router")
	if err := os.MkdirAll(filepath.Join(routerRoot, "items"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeSupervisorRegistry(t, routerRoot, repoRoot)
	f, err := dispatch.Open(repoRoot)
	if err != nil {
		t.Fatal(err)
	}
	if addErr := f.Store().AddTask(routerstore.Task{Agent: "future-llm", TaskID: "work", Subject: "must not disappear"}); addErr != nil {
		t.Fatal(addErr)
	}
	_ = f.Close()

	report, err := SuperviseOnce(SuperviseOptions{RepoRoot: repoRoot, AgentID: "horus-supervisor-test", PID: os.Getpid(), Now: time.Now().UTC()})
	if err != nil {
		t.Fatal(err)
	}
	for _, lane := range report.Agents {
		if lane.AgentID == "future-llm" {
			if lane.Operational.Classification != routerstore.LaneUnroutable {
				t.Fatalf("unregistered work owner=%+v, want UNROUTABLE", lane)
			}
			return
		}
	}
	t.Fatal("unregistered durable work owner was omitted from supervision")
}

// The supervisor reconciles every lane on every pass, but for a long time it
// read only reconcile.State and dropped the counters — so a repair and a no-op
// left identical records. Scope of this test (A35): a real repair reaches the
// report with the right count, AND the whole counter set survives serialization.
// It does NOT re-prove that any individual counter increments correctly; that
// is routerstore's reconcile_test.
func TestSuperviseOnceSurfacesReconcileCounters(t *testing.T) {
	repoRoot := t.TempDir()
	routerRoot := filepath.Join(repoRoot, ".agents", "idea-router")
	if err := os.MkdirAll(filepath.Join(routerRoot, "items"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeSupervisorRegistry(t, routerRoot, repoRoot)
	f, err := dispatch.Open(repoRoot)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.Store().AddRequirement("counters reach a surface", "ADR-057", "R6", "claude-pantheon"); err != nil {
		t.Fatal(err)
	}
	_ = f.Close()

	lane := func(report *SuperviseReport) AgentSurfaceStatus {
		t.Helper()
		for _, a := range report.Agents {
			if a.AgentID == "claude-pantheon" {
				return a
			}
		}
		t.Fatal("claude-pantheon absent from supervision")
		return AgentSurfaceStatus{}
	}

	now := time.Now().UTC().Add(time.Minute).Truncate(time.Second)
	report, err := SuperviseOnce(SuperviseOptions{RepoRoot: repoRoot, AgentID: "horus-supervisor-test", PID: os.Getpid(), Now: now})
	if err != nil {
		t.Fatal(err)
	}
	if got := lane(report).Reconcile.RequirementTasksCreated; got != 1 {
		t.Fatalf("RequirementTasksCreated=%d, want 1 — repair happened but never reached the report", got)
	}

	// FalseDoneRejected is the counter that motivated this, but a false `done`
	// cannot be forged through the store API — the gate rejects it, which is the
	// point — and routerstore's own TestReconcileCreatesTaskForRequirementAnd
	// RejectsFalseDone already proves it increments. What is NOT proven there,
	// and is proven here, is that it survives onto the wire: the counters are a
	// serialized contract, not just a Go field a renderer could reach.
	encoded, err := json.Marshal(lane(report))
	if err != nil {
		t.Fatal(err)
	}
	var decoded struct {
		Reconcile map[string]json.RawMessage `json:"reconcile"`
	}
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"false_done_rejected", "requirement_tasks_created", "repaired_non_active_leases"} {
		if _, ok := decoded.Reconcile[key]; !ok {
			t.Fatalf("%q absent from the lane contract: %s", key, encoded)
		}
	}
}

func TestSuperviseOnceRegistersThreadAndClassifiesSurfaces(t *testing.T) {
	repoRoot := t.TempDir()
	routerRoot := filepath.Join(repoRoot, ".agents", "idea-router")
	if err := os.MkdirAll(filepath.Join(routerRoot, "items"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeSupervisorRegistry(t, routerRoot, repoRoot)
	if _, err := work.Send(routerRoot, "claude-pantheon", "codex-pantheon", "needs codex", "work"); err != nil {
		t.Fatal(err)
	}
	if _, err := work.Send(routerRoot, "claude-pantheon", "manual-pantheon", "needs manual", "work"); err != nil {
		t.Fatal(err)
	}

	report, err := SuperviseOnce(SuperviseOptions{
		RepoRoot: repoRoot,
		AgentID:  "horus-supervisor-test",
		PID:      os.Getpid(),
		Now:      time.Now().UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if report.ThreadID == "" {
		t.Fatal("expected supervisor thread id")
	}
	if report.Status != ThreadStatusActive {
		t.Fatalf("status = %s, want active when pending work exists", report.Status)
	}
	if report.PendingTotal != 2 {
		t.Fatalf("pending total = %d, want 2", report.PendingTotal)
	}

	byAgent := map[string]AgentSurfaceStatus{}
	for _, agent := range report.Agents {
		byAgent[agent.AgentID] = agent
	}
	if got := byAgent["claude-pantheon"].Status; got != SupervisorStatusBlocked {
		t.Fatalf("claude status = %s, want blocked because operational state is UNROUTABLE", got)
	}
	if got := byAgent["codex-pantheon"].Status; got != SupervisorStatusBlocked {
		t.Fatalf("codex status = %s, want blocked for pending work with missing command", got)
	}
	if got := byAgent["manual-pantheon"].Status; got != SupervisorStatusBlocked {
		t.Fatalf("manual status = %s, want blocked for pending work with missing cwd/wake", got)
	}
	if got := byAgent["codex-pantheon"].Operational.Classification; got != routerstore.LaneUnroutable {
		t.Fatalf("codex operational state = %s, want UNROUTABLE", got)
	}
	if got := byAgent["claude-pantheon"].Operational.Classification; got != routerstore.LaneUnroutable {
		t.Fatalf("claude operational state = %s, want UNROUTABLE without an explicit wake adapter", got)
	}

	threads, err := LoadThreadRegistry(routerRoot)
	if err != nil {
		t.Fatal(err)
	}
	thr := threads.Threads[report.ThreadID]
	if thr == nil {
		t.Fatalf("supervisor thread %s was not saved", report.ThreadID)
	}
	if thr.AgentID != "horus-supervisor-test" || thr.Surface != SupervisorSurface {
		t.Fatalf("thread = %+v", thr)
	}
}

func TestSuperviseOnceStampsSchemaAndDrillableItems(t *testing.T) {
	repoRoot := t.TempDir()
	routerRoot := filepath.Join(repoRoot, ".agents", "idea-router")
	if err := os.MkdirAll(filepath.Join(routerRoot, "items"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeSupervisorRegistry(t, routerRoot, repoRoot)
	id, err := work.SendTyped(routerRoot, "claude-pantheon", "codex-pantheon", "wire the board", "review", "Enrich board.json to 1.1.0")
	if err != nil {
		t.Fatal(err)
	}

	now := time.Now().UTC()
	report, err := SuperviseOnce(SuperviseOptions{
		RepoRoot: repoRoot,
		AgentID:  "horus-supervisor-test",
		PID:      os.Getpid(),
		Now:      now,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Contract stamps present — renderers gate on these.
	if report.SchemaVersion != SupervisorSchemaVersion {
		t.Fatalf("schema_version = %q, want %q", report.SchemaVersion, SupervisorSchemaVersion)
	}
	if report.GeneratedAt == "" {
		t.Fatal("generated_at must be stamped")
	}

	// pending_items is drillable: the codex row carries the enriched record, not
	// a bare id.
	var codex AgentSurfaceStatus
	for _, a := range report.Agents {
		if a.AgentID == "codex-pantheon" {
			codex = a
		}
	}
	if len(codex.PendingItems) != 1 {
		t.Fatalf("codex pending_items = %d, want 1", len(codex.PendingItems))
	}
	pi := codex.PendingItems[0]
	if pi.ID != id {
		t.Errorf("pending_item id = %q, want %q", pi.ID, id)
	}
	if pi.Title != "wire the board" {
		t.Errorf("pending_item title = %q, want the item title", pi.Title)
	}
	if pi.Type != "review" {
		t.Errorf("pending_item type = %q, want review", pi.Type)
	}
	if pi.From != "claude-pantheon" {
		t.Errorf("pending_item from = %q, want claude-pantheon", pi.From)
	}
	if codex.OldestPendingAgeSeconds < 0 {
		t.Errorf("oldest_pending_age_seconds = %v, must never be negative", codex.OldestPendingAgeSeconds)
	}
}

func TestSuperviseOnceWritesBoardJSON(t *testing.T) {
	repoRoot := t.TempDir()
	routerRoot := filepath.Join(repoRoot, ".agents", "idea-router")
	if err := os.MkdirAll(filepath.Join(routerRoot, "items"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeSupervisorRegistry(t, routerRoot, repoRoot)

	report, err := SuperviseOnce(SuperviseOptions{
		RepoRoot: repoRoot,
		AgentID:  "horus-supervisor-test",
		PID:      os.Getpid(),
		Now:      time.Now().UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}

	// The board is persisted for thin renderers and round-trips to the same shape.
	data, err := os.ReadFile(filepath.Join(routerRoot, BoardFileName))
	if err != nil {
		t.Fatalf("board.json not written: %v", err)
	}
	var board SuperviseReport
	if err := json.Unmarshal(data, &board); err != nil {
		t.Fatalf("board.json does not round-trip: %v", err)
	}
	if board.SchemaVersion != report.SchemaVersion {
		t.Errorf("board schema = %q, want %q", board.SchemaVersion, report.SchemaVersion)
	}
	if board.ThreadID != report.ThreadID {
		t.Errorf("board thread_id = %q, want %q", board.ThreadID, report.ThreadID)
	}
	// No temp files leak into the router root after an atomic write.
	entries, _ := os.ReadDir(routerRoot)
	for _, e := range entries {
		if len(e.Name()) > 7 && e.Name()[:7] == ".board-" {
			t.Errorf("leaked board temp file: %s", e.Name())
		}
	}
}

func TestSuperviseOnceMarksStaleAgentThread(t *testing.T) {
	repoRoot := t.TempDir()
	routerRoot := filepath.Join(repoRoot, ".agents", "idea-router")
	if err := os.MkdirAll(filepath.Join(routerRoot, "items"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeSupervisorRegistry(t, routerRoot, repoRoot)
	now := time.Now().UTC()
	_, err := RegisterThread(routerRoot, &Thread{
		ThreadID:   "thr-stale-agent",
		AgentID:    "claude-pantheon",
		Surface:    "claude",
		Repo:       repoRoot,
		Status:     ThreadStatusActive,
		StartedAt:  now.Add(-2 * time.Hour),
		LastSeenAt: now.Add(-time.Hour),
		PID:        os.Getpid(),
	})
	if err != nil {
		t.Fatal(err)
	}
	reg, err := LoadThreadRegistry(routerRoot)
	if err != nil {
		t.Fatal(err)
	}
	reg.Threads["thr-stale-agent"].StartedAt = now.Add(-2 * time.Hour)
	reg.Threads["thr-stale-agent"].LastSeenAt = now.Add(-time.Hour)
	if err = SaveThreadRegistry(routerRoot, reg); err != nil {
		t.Fatal(err)
	}

	report, err := SuperviseOnce(SuperviseOptions{
		RepoRoot: repoRoot,
		AgentID:  "horus-supervisor-test",
		PID:      os.Getpid(),
		Now:      now,
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, agent := range report.Agents {
		if agent.AgentID != "claude-pantheon" {
			continue
		}
		if agent.Status != SupervisorStatusBlocked {
			t.Fatalf("claude status = %s, want blocked because runnable work is UNROUTABLE", agent.Status)
		}
		if len(agent.StaleThreads) != 1 || agent.StaleThreads[0] != "thr-stale-agent" {
			t.Fatalf("stale threads = %#v", agent.StaleThreads)
		}
		return
	}
	t.Fatal("claude-pantheon status not found")
}
