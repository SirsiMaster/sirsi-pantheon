package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/SirsiMaster/sirsi-pantheon/internal/router"
)

func sampleNodeStatus() *router.NodeStatus {
	return &router.NodeStatus{
		PendingByAgent: map[string][]string{
			"claude-pantheon": {"i1", "i2"},
			"claude-nexus":    {"i3"},
		},
		TotalPending:    3,
		LiveThreadCount: 4,
		StaleThreads:    []router.ThreadSummary{{ThreadID: "t1"}, {ThreadID: "t2"}},
		StrandedInbox:   []router.StrandedAgent{{AgentID: "claude-nexus", OpenItems: 1}},
	}
}

func sampleWakePass() router.WakePassReport {
	return router.WakePassReport{
		Attempted:   []router.WakeOutcome{{ItemID: "i3", AgentID: "claude-nexus", Adapter: "launchagent"}},
		Armed:       []router.WakeOutcome{{ItemID: "i1", AgentID: "claude-pantheon"}, {ItemID: "i2", AgentID: "claude-pantheon"}},
		Unavailable: []router.WakeOutcome{{ItemID: "iX", AgentID: "user", Detail: "agent \"user\" not registered"}},
	}
}

// Unscoped: every agent's pending, wake outcomes, and stranded rows are folded
// into the result with correct counts.
func TestBuildCtrResultUnscoped(t *testing.T) {
	res := buildCtrResult("/repo/sirsi-pantheon", "", sampleNodeStatus(), sampleWakePass())

	if res.Repo != "sirsi-pantheon" {
		t.Errorf("repo = %q, want basename sirsi-pantheon", res.Repo)
	}
	if res.PendingTotal != 3 || res.AgentsPending != 2 {
		t.Errorf("pending = %d across %d agents, want 3 across 2", res.PendingTotal, res.AgentsPending)
	}
	if res.LiveThreads != 4 || res.StaleThreads != 2 {
		t.Errorf("live=%d stale=%d, want 4/2", res.LiveThreads, res.StaleThreads)
	}
	if len(res.Woke) != 1 || len(res.AlreadyLive) != 2 || len(res.NeedsOwner) != 1 {
		t.Errorf("woke=%d live=%d needs-owner=%d, want 1/2/1", len(res.Woke), len(res.AlreadyLive), len(res.NeedsOwner))
	}
	if len(res.Stranded) != 1 || res.Stranded[0].AgentID != "claude-nexus" {
		t.Errorf("stranded = %#v, want one claude-nexus", res.Stranded)
	}
}

// Scoped to one agent: pending, wake outcomes, and stranded rows for OTHER
// agents are excluded, so a scoped `ctr <agent>` shows only that agent.
func TestBuildCtrResultScopedFilters(t *testing.T) {
	res := buildCtrResult("/repo/sirsi-pantheon", "claude-pantheon", sampleNodeStatus(), sampleWakePass())

	if res.Scope != "claude-pantheon" {
		t.Errorf("scope = %q", res.Scope)
	}
	if res.PendingTotal != 2 || res.AgentsPending != 1 {
		t.Errorf("scoped pending = %d across %d, want 2 across 1", res.PendingTotal, res.AgentsPending)
	}
	if len(res.AlreadyLive) != 2 {
		t.Errorf("scoped already-live = %d, want 2 (pantheon)", len(res.AlreadyLive))
	}
	if len(res.Woke) != 0 || len(res.NeedsOwner) != 0 {
		t.Errorf("scoped woke=%d needs-owner=%d, want 0/0 (those are other agents)", len(res.Woke), len(res.NeedsOwner))
	}
	if len(res.Stranded) != 0 {
		t.Errorf("scoped stranded = %d, want 0 (nexus filtered out)", len(res.Stranded))
	}
}

// Per-item wake outcomes collapse to per-agent counts for display.
func TestByAgentCount(t *testing.T) {
	got := byAgentCount([]router.WakeOutcome{
		{AgentID: "a", Adapter: "launchagent"},
		{AgentID: "a"},
		{AgentID: "b"},
	})
	if len(got) != 2 {
		t.Fatalf("agents = %d, want 2", len(got))
	}
	if got[0].agent != "a" || got[0].count != 2 || got[0].adapter != "launchagent" {
		t.Errorf("first = %+v, want a×2 via launchagent", got[0])
	}
	if got[1].agent != "b" || got[1].count != 1 {
		t.Errorf("second = %+v, want b×1", got[1])
	}
}

// The shim invokes `sirsi ctr`, prefers PATH, and lands in ~/.local/bin with the
// right filename for the platform.
func TestCtrShim(t *testing.T) {
	path, body := ctrShim("/home/u", "/abs/sirsi")
	if !strings.Contains(body, "ctr") || !strings.Contains(body, "/abs/sirsi") {
		t.Errorf("shim body missing ctr invocation or fallback path:\n%s", body)
	}
	wantName := "ctr"
	if runtime.GOOS == "windows" {
		wantName = "ctr.cmd"
	}
	if filepath.Base(path) != wantName {
		t.Errorf("shim path = %q, want basename %q", path, wantName)
	}
	if filepath.Dir(path) != filepath.Join("/home/u", ".local", "bin") {
		t.Errorf("shim dir = %q, want ~/.local/bin", filepath.Dir(path))
	}
}

// The repo-shipped skill (.claude/skills/ctr/SKILL.md) must be byte-identical to
// what `sirsi ctr --install` writes, so the two copies never drift.
func TestCtrSkillRepoMatchesInstaller(t *testing.T) {
	root := repoRootFromTest(t)
	repoSkill := filepath.Join(root, ".claude", "skills", "ctr", "SKILL.md")
	data, err := os.ReadFile(repoSkill)
	if err != nil {
		t.Fatalf("read repo skill: %v", err)
	}
	if string(data) != ctrSkillBody() {
		t.Errorf("%s has drifted from ctrSkillBody(); regenerate it", repoSkill)
	}
}

func repoRootFromTest(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod not found walking up from test cwd")
		}
		dir = parent
	}
}
