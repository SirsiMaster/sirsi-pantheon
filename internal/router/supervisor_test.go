package router

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

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
	if got := byAgent["claude-pantheon"].Status; got != SupervisorStatusWakeable {
		t.Fatalf("claude status = %s, want wakeable", got)
	}
	if got := byAgent["codex-pantheon"].Status; got != SupervisorStatusBlocked {
		t.Fatalf("codex status = %s, want blocked for pending work with missing command", got)
	}
	if got := byAgent["manual-pantheon"].Status; got != SupervisorStatusBlocked {
		t.Fatalf("manual status = %s, want blocked for pending work with missing cwd/wake", got)
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
		if agent.Status != SupervisorStatusStale {
			t.Fatalf("claude status = %s, want stale", agent.Status)
		}
		if len(agent.StaleThreads) != 1 || agent.StaleThreads[0] != "thr-stale-agent" {
			t.Fatalf("stale threads = %#v", agent.StaleThreads)
		}
		return
	}
	t.Fatal("claude-pantheon status not found")
}
