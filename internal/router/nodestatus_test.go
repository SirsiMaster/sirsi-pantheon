package router

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// mockAuthProbe returns a fake auth probe for testing.
func mockAuthProbe(authOK, needsLogin bool, detail string) AuthProbeFunc {
	return func(cliPath, agentType string) (bool, bool, string) {
		return authOK, needsLogin, detail
	}
}

// installFakeAgentCLIs makes CLI discovery deterministic without invoking real agents.
func installFakeAgentCLIs(t *testing.T) {
	t.Helper()
	binDir := t.TempDir()
	for _, name := range []string{"claude", "codex"} {
		path := filepath.Join(binDir, name)
		if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", binDir)
}

// setupNodeTestRouter creates a minimal router directory for node-status testing.
func setupNodeTestRouter(t *testing.T) string {
	t.Helper()
	tmp := t.TempDir()
	routerRoot := filepath.Join(tmp, ".agents", "idea-router")
	for _, dir := range []string{"proposals", "reviews", "decisions"} {
		if err := os.MkdirAll(filepath.Join(routerRoot, dir), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	// state.json
	state := State{
		Version:         1,
		ActiveTopics:    []string{"topic-alpha", "topic-beta"},
		CompletedTopics: []string{"topic-done"},
		LastClaudeRead:  "2026-05-19T12:00:00Z",
		LastCodexRead:   "2026-05-19T10:00:00Z",
		Rules:           map[string]bool{"require_plan": true},
		Pending: map[string][]string{
			"claude-pantheon": {"item-1"},
			"codex-pantheon":  {},
		},
		PendingForClaude: []string{"item-1"},
		PendingForCodex:  []string{},
	}
	data, _ := json.MarshalIndent(state, "", "  ")
	if err := os.WriteFile(filepath.Join(routerRoot, "state.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}

	// agents.json
	reg := map[string]interface{}{
		"agents": map[string]interface{}{
			"claude-pantheon": map[string]interface{}{
				"type":    "claude",
				"command": []string{"claude", "--print"},
				"cwd":     tmp,
			},
			"codex-pantheon": map[string]interface{}{
				"type":    "codex",
				"command": []string{"codex", "exec"},
				"cwd":     tmp,
			},
		},
	}
	regData, _ := json.MarshalIndent(reg, "", "  ")
	if err := os.WriteFile(filepath.Join(routerRoot, "agents.json"), regData, 0o644); err != nil {
		t.Fatal(err)
	}

	return tmp
}

func TestCollectNodeStatus_BasicFields(t *testing.T) {
	repoRoot := setupNodeTestRouter(t)

	ns, err := CollectNodeStatus(repoRoot, nil, mockAuthProbe(true, false, ""))
	if err != nil {
		t.Fatalf("CollectNodeStatus: %v", err)
	}

	if ns.AgentCount != 2 {
		t.Errorf("AgentCount = %d, want 2", ns.AgentCount)
	}
	if ns.TotalPending != 1 {
		t.Errorf("TotalPending = %d, want 1", ns.TotalPending)
	}
	if len(ns.ActiveTopics) != 2 {
		t.Errorf("ActiveTopics = %d, want 2", len(ns.ActiveTopics))
	}
	if ns.CompletedCount != 1 {
		t.Errorf("CompletedCount = %d, want 1", ns.CompletedCount)
	}
	if ns.LastClaudeRead != "2026-05-19T12:00:00Z" {
		t.Errorf("LastClaudeRead = %q", ns.LastClaudeRead)
	}
	if ns.RouterHome == "" {
		t.Error("RouterHome is empty")
	}
	if len(ns.WakeHealth) != 2 {
		t.Fatalf("WakeHealth = %d, want 2", len(ns.WakeHealth))
	}
}

func TestCollectLaunchAgentsInventoriesKnownHelpers(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	agentDir := filepath.Join(home, "Library", "LaunchAgents")
	if err := os.MkdirAll(agentDir, 0o755); err != nil {
		t.Fatal(err)
	}
	plist := RenderLaunchAgentPlist(ServiceOptions{
		Label:      "ai.sirsi.pantheon",
		RepoRoot:   "/tmp/repo",
		BinaryPath: "/bin/zsh",
		LogPath:    "/tmp/out.log",
		ErrPath:    "/tmp/err.log",
		PathEnv:    "/bin",
	})
	if err := os.WriteFile(filepath.Join(agentDir, "ai.sirsi.pantheon.plist"), []byte(plist), 0o644); err != nil {
		t.Fatal(err)
	}

	agents := CollectLaunchAgents("/tmp/repo", nil)
	var found bool
	for _, h := range agents {
		if h.Label == "ai.sirsi.pantheon" {
			found = true
			if !h.Installed || h.Role != "menubar" || h.Program != "/bin/zsh" || !h.ProgramFound {
				t.Fatalf("unexpected menubar LaunchAgent health: %+v", h)
			}
		}
		if h.Label == "com.sirsi.router.repo" && !h.Legacy {
			t.Fatalf("legacy router helper must be marked legacy: %+v", h)
		}
	}
	if !found {
		t.Fatalf("known menubar LaunchAgent not inventoried: %+v", agents)
	}
}

func TestCollectNodeStatus_WakeHealthIncludesMechanisms(t *testing.T) {
	repoRoot := setupNodeTestRouter(t)
	routerRoot := filepath.Join(repoRoot, ".agents", "idea-router")
	reg := map[string]interface{}{
		"agents": map[string]interface{}{
			"api-agent": map[string]interface{}{
				"type": "gemini",
				"wake": map[string]interface{}{
					"mechanism": "api-call",
					"endpoint":  "http://127.0.0.1:9999/wake",
				},
			},
			"mcp-agent": map[string]interface{}{
				"type": "ide-extension",
				"wake": map[string]interface{}{
					"mechanism":  "mcp-notification",
					"mcp_server": "sirsi",
				},
			},
		},
	}
	regData, _ := json.MarshalIndent(reg, "", "  ")
	if err := os.WriteFile(filepath.Join(routerRoot, "agents.json"), regData, 0o644); err != nil {
		t.Fatal(err)
	}

	ns, err := CollectNodeStatus(repoRoot, nil, mockAuthProbe(true, false, ""))
	if err != nil {
		t.Fatalf("CollectNodeStatus: %v", err)
	}

	got := make(map[string]AgentWakeHealth)
	for _, h := range ns.WakeHealth {
		got[h.AgentID] = h
	}
	if got["api-agent"].Mechanism != WakeAPICall || !got["api-agent"].Ready {
		t.Fatalf("api wake health = %+v", got["api-agent"])
	}
	if got["mcp-agent"].Mechanism != WakeMCPNotification || got["mcp-agent"].Detail != "sirsi" {
		t.Fatalf("mcp wake health = %+v", got["mcp-agent"])
	}
}

func TestCollectNodeStatus_PendingByAgent(t *testing.T) {
	repoRoot := setupNodeTestRouter(t)

	ns, err := CollectNodeStatus(repoRoot, nil, mockAuthProbe(true, false, ""))
	if err != nil {
		t.Fatalf("CollectNodeStatus: %v", err)
	}

	ids, ok := ns.PendingByAgent["claude-pantheon"]
	if !ok || len(ids) != 1 || ids[0] != "item-1" {
		t.Errorf("PendingByAgent[claude-pantheon] = %v, want [item-1]", ids)
	}
	if _, ok := ns.PendingByAgent["codex-pantheon"]; ok {
		t.Error("codex-pantheon should not appear (empty)")
	}
}

func TestCollectNodeStatus_RegisteredAgentsSorted(t *testing.T) {
	repoRoot := setupNodeTestRouter(t)

	ns, err := CollectNodeStatus(repoRoot, nil, mockAuthProbe(true, false, ""))
	if err != nil {
		t.Fatalf("CollectNodeStatus: %v", err)
	}

	if len(ns.RegisteredAgents) != 2 {
		t.Fatalf("RegisteredAgents = %d, want 2", len(ns.RegisteredAgents))
	}
	if ns.RegisteredAgents[0] != "claude-pantheon" || ns.RegisteredAgents[1] != "codex-pantheon" {
		t.Errorf("RegisteredAgents not sorted: %v", ns.RegisteredAgents)
	}
}

func TestCollectNodeStatus_DaemonNotInstalled(t *testing.T) {
	repoRoot := setupNodeTestRouter(t)

	ns, err := CollectNodeStatus(repoRoot, nil, mockAuthProbe(true, false, ""))
	if err != nil {
		t.Fatalf("CollectNodeStatus: %v", err)
	}

	if ns.DaemonInstalled {
		t.Error("DaemonInstalled should be false when no plist exists")
	}
	if ns.DaemonLoaded {
		t.Error("DaemonLoaded should be false with nil checker")
	}
}

func TestCollectNodeStatus_WorkQueueSummary(t *testing.T) {
	repoRoot := setupNodeTestRouter(t)
	routerRoot := filepath.Join(repoRoot, ".agents", "idea-router")

	// Write a work queue with mixed statuses
	wq := struct {
		Items []WorkItem `json:"items"`
	}{
		Items: []WorkItem{
			{ID: "a:1", Status: StatusPending},
			{ID: "b:2", Status: StatusCompleted},
			{ID: "c:3", Status: StatusFailed, TargetAgentID: "codex-pantheon",
				Attempts: []Attempt{{Error: "CLI not found"}}},
		},
	}
	data, _ := json.MarshalIndent(wq, "", "  ")
	if err := os.WriteFile(filepath.Join(routerRoot, "work-queue.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}

	ns, err := CollectNodeStatus(repoRoot, nil, mockAuthProbe(true, false, ""))
	if err != nil {
		t.Fatalf("CollectNodeStatus: %v", err)
	}

	if ns.WorkItemSummary["pending"] != 1 {
		t.Errorf("pending = %d, want 1", ns.WorkItemSummary["pending"])
	}
	if ns.WorkItemSummary["completed"] != 1 {
		t.Errorf("completed = %d, want 1", ns.WorkItemSummary["completed"])
	}
	if len(ns.RecentFailures) != 1 {
		t.Fatalf("RecentFailures = %d, want 1", len(ns.RecentFailures))
	}
	if ns.RecentFailures[0].Error != "CLI not found" {
		t.Errorf("failure error = %q", ns.RecentFailures[0].Error)
	}
}

func TestCollectNodeStatus_BlockedItemsAppearInRecentFailures(t *testing.T) {
	repoRoot := setupNodeTestRouter(t)
	routerRoot := filepath.Join(repoRoot, ".agents", "idea-router")
	blockedAt := time.Now()

	wq := struct {
		Items []WorkItem `json:"items"`
	}{
		Items: []WorkItem{
			{
				ID:            "claude-pantheon:item-1",
				Status:        StatusBlocked,
				TargetAgentID: "claude-pantheon",
				LastError:     "claude CLI not authenticated",
				CompletedAt:   blockedAt,
			},
		},
	}
	data, _ := json.MarshalIndent(wq, "", "  ")
	if err := os.WriteFile(filepath.Join(routerRoot, "work-queue.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}

	ns, err := CollectNodeStatus(repoRoot, nil, mockAuthProbe(true, false, ""))
	if err != nil {
		t.Fatalf("CollectNodeStatus: %v", err)
	}

	if ns.WorkItemSummary["blocked"] != 1 {
		t.Errorf("blocked = %d, want 1", ns.WorkItemSummary["blocked"])
	}
	if len(ns.RecentFailures) != 1 {
		t.Fatalf("RecentFailures = %d, want 1", len(ns.RecentFailures))
	}
	if ns.RecentFailures[0].Error != "claude CLI not authenticated" {
		t.Errorf("failure error = %q", ns.RecentFailures[0].Error)
	}
}

func TestCollectNodeStatus_AuthProbeNeedsLogin(t *testing.T) {
	installFakeAgentCLIs(t)
	repoRoot := setupNodeTestRouter(t)

	probe := mockAuthProbe(false, true, "Not logged in · Please run /login")
	ns, err := CollectNodeStatus(repoRoot, nil, probe)
	if err != nil {
		t.Fatalf("CollectNodeStatus: %v", err)
	}

	for _, h := range ns.AgentHealth {
		if !h.CLIFound {
			// CLI not in PATH during test — skip
			continue
		}
		if h.AuthOK {
			t.Errorf("%s: AuthOK should be false when probe says needs login", h.AgentType)
		}
		if !h.NeedsLogin {
			t.Errorf("%s: NeedsLogin should be true", h.AgentType)
		}
		if h.AuthError == "" {
			t.Errorf("%s: AuthError should be set", h.AgentType)
		}
	}
}

func TestCollectNodeStatus_AuthProbeOK(t *testing.T) {
	installFakeAgentCLIs(t)
	repoRoot := setupNodeTestRouter(t)

	probe := mockAuthProbe(true, false, "")
	ns, err := CollectNodeStatus(repoRoot, nil, probe)
	if err != nil {
		t.Fatalf("CollectNodeStatus: %v", err)
	}

	for _, h := range ns.AgentHealth {
		if !h.CLIFound {
			continue
		}
		if !h.AuthOK {
			t.Errorf("%s: AuthOK should be true", h.AgentType)
		}
		if h.NeedsLogin {
			t.Errorf("%s: NeedsLogin should be false", h.AgentType)
		}
	}
}

func TestCollectNodeStatus_BlockedItemsCount(t *testing.T) {
	installFakeAgentCLIs(t)
	repoRoot := setupNodeTestRouter(t)

	// The test fixture has 1 pending item for claude-pantheon
	probe := mockAuthProbe(false, true, "not logged in")
	ns, err := CollectNodeStatus(repoRoot, nil, probe)
	if err != nil {
		t.Fatalf("CollectNodeStatus: %v", err)
	}

	for _, h := range ns.AgentHealth {
		if !h.CLIFound {
			continue
		}
		if h.AgentType == "claude" && h.BlockedItems != 1 {
			t.Errorf("claude BlockedItems = %d, want 1 (pending item-1 for claude-pantheon)", h.BlockedItems)
		}
	}
}

func TestDefaultAuthProbe_FlagsMissingClaudeEnv(t *testing.T) {
	binDir := t.TempDir()
	fake := filepath.Join(binDir, "claude-fake")
	script := "#!/bin/sh\necho 'Not logged in · Please run /login'\nexit 1\n"
	if err := os.WriteFile(fake, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	t.Setenv("USER", "")
	t.Setenv("HOME", "")

	authOK, needsLogin, detail := DefaultAuthProbe(fake, "claude")
	if authOK {
		t.Fatal("authOK should be false when CLI reports not logged in")
	}
	if needsLogin {
		t.Error("needsLogin should be false when the failure is actually env propagation")
	}
	if !strings.Contains(detail, "missing env") {
		t.Errorf("detail should call out missing env, got %q", detail)
	}
}

func TestIsAuthError(t *testing.T) {
	tests := []struct {
		output string
		want   bool
	}{
		{"Not logged in · Please run /login", true},
		{"please log in to continue", true},
		{"Authentication required", true},
		{"Unauthorized", true},
		{"claude v1.2.3", false},
		{"some other error", false},
		{"", false},
	}
	for _, tt := range tests {
		got := isAuthError(tt.output)
		if got != tt.want {
			t.Errorf("isAuthError(%q) = %v, want %v", tt.output, got, tt.want)
		}
	}
}

func TestCollectNodeStatus_SurfacesLiveAndStaleThreads(t *testing.T) {
	installFakeAgentCLIs(t)
	repoRoot := setupNodeTestRouter(t)
	routerRoot := filepath.Join(repoRoot, ".agents", "idea-router")

	// Register a fresh (live) thread.
	if _, err := RegisterThread(routerRoot, &Thread{
		AgentID: "claude-pantheon", Surface: "claude", Repo: repoRoot,
		Watches: []string{"claude-pantheon"},
	}); err != nil {
		t.Fatalf("RegisterThread live: %v", err)
	}
	// Register a thread, then back-date its LastSeenAt to make it stale.
	stale, err := RegisterThread(routerRoot, &Thread{
		AgentID: "codex-pantheon", Surface: "codex", Repo: repoRoot,
	})
	if err != nil {
		t.Fatalf("RegisterThread stale: %v", err)
	}
	tr, _ := LoadThreadRegistry(routerRoot)
	tr.Threads[stale.ThreadID].LastSeenAt = time.Now().Add(-1 * time.Hour)
	if saveErr := SaveThreadRegistry(routerRoot, tr); saveErr != nil {
		t.Fatal(saveErr)
	}

	ns, err := CollectNodeStatus(repoRoot, nil, mockAuthProbe(true, false, ""))
	if err != nil {
		t.Fatalf("CollectNodeStatus: %v", err)
	}
	if len(ns.LiveThreads) != 1 {
		t.Errorf("LiveThreads = %d, want 1", len(ns.LiveThreads))
	}
	if len(ns.StaleThreads) != 1 {
		t.Errorf("StaleThreads = %d, want 1", len(ns.StaleThreads))
	}
	if ns.LiveThreadCount != 1 {
		t.Errorf("LiveThreadCount = %d, want 1", ns.LiveThreadCount)
	}
	if ns.LiveThreads[0].AgentID != "claude-pantheon" {
		t.Errorf("live thread agent = %q, want claude-pantheon", ns.LiveThreads[0].AgentID)
	}
	if !ns.StaleThreads[0].Stale {
		t.Error("stale thread not marked stale=true in summary")
	}
}
