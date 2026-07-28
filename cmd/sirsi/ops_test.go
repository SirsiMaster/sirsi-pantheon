package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/router"
	"github.com/SirsiMaster/sirsi-pantheon/internal/routercfg"
	"github.com/SirsiMaster/sirsi-pantheon/internal/work"
)

func TestOpsCapabilitiesCoverAgentOperationsParity(t *testing.T) {
	caps := opsCapabilities()
	byID := map[string]opsCapability{}
	for _, cap := range caps {
		byID[cap.ID] = cap
		if cap.DeterministicCommand == "" {
			t.Fatalf("%s missing deterministic command", cap.ID)
		}
		if cap.MenubarSurface == "" {
			t.Fatalf("%s missing menubar surface", cap.ID)
		}
	}

	required := []string{
		"wake",
		"queue",
		"respond",
		"review",
		"ask",
		"memory",
		"watch",
		"reap",
		"supervise",
		"insight",
	}
	for _, id := range required {
		if _, ok := byID[id]; !ok {
			t.Fatalf("missing %s capability", id)
		}
	}

	if byID["wake"].LocalAICommand != "sirsi ctr --reconcile" {
		t.Fatalf("wake local AI command = %q", byID["wake"].LocalAICommand)
	}
	if byID["ask"].LocalAICommand == "" {
		t.Fatal("ask capability must expose local AI command")
	}
}

func TestNonEmptyPendingAgents(t *testing.T) {
	got := nonEmptyPendingAgents(map[string][]string{
		"claude-home":    {"a"},
		"codex-pantheon": nil,
		" ":              {"ignored"},
		"claude-nexus":   {"b", "c"},
	})
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2 (%v)", len(got), got)
	}
}

func TestCollectOpsLiveSummaryIsCheapAndStrandedAware(t *testing.T) {
	t.Setenv(routercfg.StoreWakeEnv, "0")
	root := t.TempDir()
	routerRoot := filepath.Join(root, ".agents", "idea-router")
	if err := os.MkdirAll(filepath.Join(routerRoot, "items"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(routerRoot, "state.json"), []byte(`{"pending":{}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(routerRoot, "agents.json"), []byte(`{"agents":{}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := work.Send(routerRoot, "claude-home", "codex-pantheon", "review", "please review"); err != nil {
		t.Fatal(err)
	}
	if _, err := router.RegisterThread(routerRoot, &router.Thread{
		AgentID:    "codex-pantheon",
		Surface:    "codex",
		LastSeenAt: time.Now().UTC(),
		Watches:    []string{"codex-pantheon"},
	}); err != nil {
		t.Fatal(err)
	}

	got, err := collectOpsLiveSummary(root, routerRoot)
	if err != nil {
		t.Fatal(err)
	}
	if got.PendingTotal != 1 || got.AgentsPending != 1 || got.LiveThreads != 1 || got.Stranded != 0 {
		t.Fatalf("summary = %+v", got)
	}
}
