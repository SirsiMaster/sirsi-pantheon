package router

import (
	"encoding/json"
	"fmt"
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
	if ns.GeneratedAt == "" {
		t.Error("GeneratedAt must be stamped by CollectNodeStatus")
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
	// /bin/sh exists on macOS AND the Linux release runner (goreleaser job runs on
	// ubuntu); /bin/zsh is absent on Linux → ProgramFound:false → release-blocking
	// test failure (the reason v0.23.0/v0.23.1-beta release runs failed at this step).
	plist := testLaunchPlist("/bin/sh")
	if err := os.WriteFile(filepath.Join(agentDir, "ai.sirsi.pantheon.plist"), []byte(plist), 0o644); err != nil {
		t.Fatal(err)
	}

	// Deterministic stub: nothing loaded. (nil now means "shell the real
	// launchctl", which would make this test depend on the host's launchd.)
	agents := CollectLaunchAgents("/tmp/repo", func(args ...string) error {
		return fmt.Errorf("stub: not loaded")
	})
	var found bool
	for _, h := range agents {
		if h.Label == "ai.sirsi.pantheon" {
			found = true
			if !h.Installed || h.Role != "menubar" || h.Program != "/bin/sh" || !h.ProgramFound {
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
			// A legacy command-only agent (no explicit wake.mechanism). The honest
			// view must report it NOT ready — it would only be blind-spawnable, which
			// the wake pass refuses (PR #89).
			"legacy-agent": map[string]interface{}{
				"type":    "gemini",
				"command": []string{"echo"},
				"cwd":     repoRoot,
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
	// api-call with an endpoint is honestly wakeable.
	if got["api-agent"].Mechanism != WakeAPICall || !got["api-agent"].Ready {
		t.Fatalf("api wake health = %+v", got["api-agent"])
	}
	// mcp-notification is NOT yet wired, so the honest view reports not-ready
	// (the surface must match the acted-on readiness — PR #89 finding 3).
	if got["mcp-agent"].Mechanism != WakeMCPNotification || got["mcp-agent"].Ready {
		t.Fatalf("mcp wake health should be not-ready (unwired) = %+v", got["mcp-agent"])
	}
	// A legacy command-only agent has no explicit wake mechanism → not ready
	// (never blind-spawned). This is the honesty the permissive view masked.
	if got["legacy-agent"].Ready || got["legacy-agent"].Mechanism != "" {
		t.Fatalf("legacy command agent must be not-ready with no explicit mechanism = %+v", got["legacy-agent"])
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

func TestCollectNodeStatus_PendingByAgentUsesItemsQueue(t *testing.T) {
	repoRoot := setupNodeTestRouter(t)
	routerRoot := filepath.Join(repoRoot, ".agents", "idea-router")
	itemsDir := filepath.Join(routerRoot, "items")
	if err := os.MkdirAll(itemsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	item := `---
from: "claude-pantheon"
to: "codex-pantheon"
title: "canonical queue item"
status: open
opened: 2026-06-05T23:20:00Z
---

## Instructions

Use items/ as the canonical queue.
`
	if err := os.WriteFile(filepath.Join(itemsDir, "live-item.md"), []byte(item), 0o644); err != nil {
		t.Fatal(err)
	}

	ns, err := CollectNodeStatus(repoRoot, nil, mockAuthProbe(true, false, ""))
	if err != nil {
		t.Fatalf("CollectNodeStatus: %v", err)
	}

	if ns.TotalPending != 1 {
		t.Fatalf("TotalPending = %d, want 1", ns.TotalPending)
	}
	if got := ns.PendingByAgent["codex-pantheon"]; len(got) != 1 || got[0] != "live-item" {
		t.Fatalf("PendingByAgent[codex-pantheon] = %v, want [live-item]", got)
	}
	if _, ok := ns.PendingByAgent["claude-pantheon"]; ok {
		t.Fatal("stale state.json pending should not be surfaced when items/ has live work")
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

// TestDefaultAuthProbe_TimeoutIsDegradedNotLoggedOut is the regression test for
// the 8s-timeout false-positive: a cold Claude CLI that takes longer than the
// probe deadline to answer must be reported as INCONCLUSIVE (needsLogin=false),
// never as "logged out". It drives probeAuthWithTimeout — the
// timeout-parameterized core DefaultAuthProbe wraps — against a fake `claude`
// that sleeps past a short deadline the test controls, so the REAL
// deadline-exceeded branch is exercised in milliseconds instead of waiting out
// the production 30s budget. The contract asserted: a deadline-exceeded
// returns (false, false, "…timed out…").
func TestDefaultAuthProbe_TimeoutIsDegradedNotLoggedOut(t *testing.T) {
	binDir := t.TempDir()
	fake := filepath.Join(binDir, "claude-slow")
	// Sleep past the probe deadline, then print an auth-looking line. The probe
	// must NEVER read that line — it must time out first and report inconclusive,
	// proving a timeout is not conflated with a logout. `exec sleep` replaces the
	// shell so the context's kill terminates the sleep directly (no lingering
	// shell parent holding the output pipe open past the deadline).
	script := "#!/bin/sh\nexec sleep 5\n"
	if err := os.WriteFile(fake, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	// Run the probe with a short deadline via a wrapper so the test is fast. We
	// exercise the exact deadline-exceeded branch DefaultAuthProbe uses.
	authOK, needsLogin, detail := probeAuthWithTimeout(fake, "claude", 300*time.Millisecond)
	if authOK {
		t.Error("authOK should be false on a probe timeout")
	}
	if needsLogin {
		t.Error("needsLogin must be FALSE on a timeout — a timeout is inconclusive, not a logout")
	}
	if !strings.Contains(strings.ToLower(detail), "timed out") {
		t.Errorf("detail should say the probe timed out, got %q", detail)
	}

	// And the collector must classify it as Degraded, NOT count blocked items.
	installFakeAgentCLIs(t)
	repoRoot := setupNodeTestRouter(t)
	// Probe that mimics a timeout outcome: not ok, not needs-login, timeout detail.
	timeoutProbe := mockAuthProbe(false, false, "auth probe timed out after 30s (cold CLI start — inconclusive, not logged out)")
	ns, err := CollectNodeStatus(repoRoot, nil, timeoutProbe)
	if err != nil {
		t.Fatalf("CollectNodeStatus: %v", err)
	}
	for _, h := range ns.AgentHealth {
		if !h.CLIFound {
			continue
		}
		if h.NeedsLogin {
			t.Errorf("%s: NeedsLogin must be false on an inconclusive probe", h.AgentType)
		}
		if !h.Degraded {
			t.Errorf("%s: Degraded must be true on an inconclusive probe", h.AgentType)
		}
		if h.BlockedItems != 0 {
			t.Errorf("%s: BlockedItems = %d, want 0 — an inconclusive probe must not strand work", h.AgentType, h.BlockedItems)
		}
	}
}

// TestDefaultAuthProbe_EnvTimeoutOverride proves SIRSI_AUTH_PROBE_TIMEOUT_MS
// caps the FULL DefaultAuthProbe path (not just the parameterized core): with
// the env set to 500ms and an injected slow `claude` (sleeps far past the
// deadline), DefaultAuthProbe must return promptly with the DEGRADED
// (inconclusive) verdict — never NeedsLogin, and never the production 30s
// budget. This is the override's only behavioral contract; without this test
// a regression could silently pin the probe back to 30s and consume a test
// suite's entire timeout on one cold probe.
func TestDefaultAuthProbe_EnvTimeoutOverride(t *testing.T) {
	binDir := t.TempDir()
	fake := filepath.Join(binDir, "claude-slow")
	// The injectable slow probe: blocks well past the overridden deadline.
	// `exec sleep` replaces the shell so the context kill reaps it directly.
	script := "#!/bin/sh\nexec sleep 30\n"
	if err := os.WriteFile(fake, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SIRSI_AUTH_PROBE_TIMEOUT_MS", "500")

	start := time.Now()
	authOK, needsLogin, detail := DefaultAuthProbe(fake, "claude")
	elapsed := time.Since(start)

	if elapsed > 5*time.Second {
		t.Fatalf("DefaultAuthProbe took %v — the 500ms env override was not honored", elapsed)
	}
	if authOK {
		t.Error("authOK must be false on a probe timeout")
	}
	if needsLogin {
		t.Error("needsLogin must be FALSE — a timeout is Degraded/inconclusive, never a logout")
	}
	if !strings.Contains(strings.ToLower(detail), "timed out") {
		t.Errorf("detail should say the probe timed out, got %q", detail)
	}
}

// TestClaudeProbeTimeout_EnvParsing pins the override's parse rules: a positive
// integer (ms) is honored; junk, zero, and negatives fall back to the 30s
// production budget.
func TestClaudeProbeTimeout_EnvParsing(t *testing.T) {
	cases := []struct {
		env  string
		want time.Duration
	}{
		{"500", 500 * time.Millisecond},
		{"", claudeAuthProbeTimeout},
		{"0", claudeAuthProbeTimeout},
		{"-100", claudeAuthProbeTimeout},
		{"not-a-number", claudeAuthProbeTimeout},
	}
	for _, tc := range cases {
		t.Setenv("SIRSI_AUTH_PROBE_TIMEOUT_MS", tc.env)
		if got := claudeProbeTimeout(); got != tc.want {
			t.Errorf("claudeProbeTimeout() with env %q = %v, want %v", tc.env, got, tc.want)
		}
	}
}

// TestDefaultAuthProbe_LoginSignatureNeedsLogin proves the honest opposite: a
// real /login signature in the CLI output IS a logout (needsLogin=true), so a
// genuine re-auth blocker is still surfaced (and still counts blocked items).
func TestDefaultAuthProbe_LoginSignatureNeedsLogin(t *testing.T) {
	binDir := t.TempDir()
	fake := filepath.Join(binDir, "claude-loggedout")
	// Answer instantly with an unambiguous /login signature and a non-zero exit.
	script := "#!/bin/sh\necho 'Not logged in · Please run /login'\nexit 1\n"
	if err := os.WriteFile(fake, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	// Ensure env is present so this is classified as a real logout, not demoted
	// to the missing-env inconclusive branch.
	t.Setenv("USER", "tester")
	t.Setenv("HOME", binDir)

	authOK, needsLogin, detail := DefaultAuthProbe(fake, "claude")
	if authOK {
		t.Error("authOK should be false when the CLI reports /login")
	}
	if !needsLogin {
		t.Errorf("needsLogin should be TRUE on a real /login signature, got detail %q", detail)
	}

	// And the collector must count it as a blocker (the fixture has 1 claude item).
	installFakeAgentCLIs(t)
	repoRoot := setupNodeTestRouter(t)
	loginProbe := mockAuthProbe(false, true, "Not logged in · Please run /login")
	ns, err := CollectNodeStatus(repoRoot, nil, loginProbe)
	if err != nil {
		t.Fatalf("CollectNodeStatus: %v", err)
	}
	for _, h := range ns.AgentHealth {
		if h.AgentType == "claude" && h.CLIFound {
			if !h.NeedsLogin {
				t.Error("claude NeedsLogin should be true")
			}
			if h.Degraded {
				t.Error("claude Degraded should be false when it's a real logout")
			}
			if h.BlockedItems != 1 {
				t.Errorf("claude BlockedItems = %d, want 1 (a real logout blocks the pending item)", h.BlockedItems)
			}
		}
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
	// A stale thread is one with a LIVE PID but an old heartbeat — it must be
	// surfaced as stale, not reaped. (A pid-less phantom that is stale is DEAD
	// and gets reaped per the #29 policy, so it would not be a "stale" row.)
	// Report its PID alive so the OS-truth reaper leaves it be.
	const stalePID = 424242
	oldState := getPIDStateFn()
	setPIDStateFn(func(pid int) PIDState {
		if pid == stalePID {
			return PIDAlive
		}
		return PIDGone
	})
	defer setPIDStateFn(oldState)

	stale, err := RegisterThread(routerRoot, &Thread{
		AgentID: "codex-pantheon", Surface: "codex", Repo: repoRoot, PID: stalePID,
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

func TestCollectNodeStatus_DoesNotCountSuspendedThreadsAsLive(t *testing.T) {
	installFakeAgentCLIs(t)
	repoRoot := setupNodeTestRouter(t)
	routerRoot := filepath.Join(repoRoot, ".agents", "idea-router")

	thr, err := RegisterThread(routerRoot, &Thread{
		AgentID: "claude-pantheon", Surface: "claude", Repo: repoRoot,
		Watches: []string{"claude-pantheon"},
	})
	if err != nil {
		t.Fatalf("RegisterThread: %v", err)
	}
	if _, err = SuspendThread(routerRoot, thr.ThreadID, &SuspendPayload{ResumePrompt: "resume later"}); err != nil {
		t.Fatalf("SuspendThread: %v", err)
	}

	ns, err := CollectNodeStatus(repoRoot, nil, mockAuthProbe(true, false, ""))
	if err != nil {
		t.Fatalf("CollectNodeStatus: %v", err)
	}
	if ns.LiveThreadCount != 0 || len(ns.LiveThreads) != 0 || len(ns.StaleThreads) != 0 {
		t.Fatalf("suspended thread surfaced as live/stale: live=%d stale=%d count=%d",
			len(ns.LiveThreads), len(ns.StaleThreads), ns.LiveThreadCount)
	}
}

// TestCollectNodeStatus_HonestLiveness locks the owner-priority "honest liveness"
// contract (2026-06-19): node-status must not report a thread armed when its
// surface-native loop is actually dead, and must not false-flag surfaces that
// prove liveness by heartbeat. It also confirms OS-dead threads auto-reap on the
// live CollectNodeStatus path (criterion 3).
func TestCollectNodeStatus_HonestLiveness(t *testing.T) {
	repoRoot := setupNodeTestRouter(t)
	routerRoot := filepath.Join(repoRoot, ".agents", "idea-router")
	host, _ := os.Hostname()

	// OS truth: pid 10004 is gone (reapable); the rest are alive with matching start.
	oldState, oldStart, oldCmd := getPIDStateFn(), getPIDStartFn(), getPIDCommandFn()
	setPIDStateFn(func(pid int) PIDState {
		if pid == 10004 {
			return PIDGone
		}
		return PIDAlive
	})
	setPIDStartFn(func(int) string { return "sig" })
	setPIDCommandFn(func(int) string { return "" })
	defer func() { setPIDStateFn(oldState); setPIDStartFn(oldStart); setPIDCommandFn(oldCmd) }()

	mk := func(agent, surface string, pid int) *Thread {
		thr, err := RegisterThread(routerRoot, &Thread{AgentID: agent, Surface: surface, PID: pid, Host: host, StartTime: "sig"})
		if err != nil {
			t.Fatalf("register %s: %v", agent, err)
		}
		return thr
	}
	aLive := mk("claude-a", "claude", 10001)   // loop-monitor, loop ALIVE
	bDead := mk("claude-b", "claude", 10002)   // loop-monitor, heartbeat-fresh but loop DEAD
	cCodex := mk("codex-x", "codex", 10003)    // app-heartbeat — loop-evidence N/A
	eMenu := mk("menubar-x", "menubar", 10005) // native-runloop resident — loop-evidence N/A
	fGemma := mk("gemma-x", "gemma", 10006)    // surface-loop — loop-evidence N/A (NOT pgrep-gated)
	dGone := mk("claude-d", "claude", 10004)   // OS-dead → must auto-reap

	// Only thread A has a live watcher loop.
	setWatcherAliveFn(func(id string) bool { return id == aLive.ThreadID })
	defer func() { setWatcherAliveFn(nil) }()

	ns, err := CollectNodeStatus(repoRoot, nil, mockAuthProbe(true, false, ""))
	if err != nil {
		t.Fatalf("CollectNodeStatus: %v", err)
	}
	find := func(id string) *ThreadSummary {
		for i := range ns.LiveThreads {
			if ns.LiveThreads[i].ThreadID == id {
				return &ns.LiveThreads[i]
			}
		}
		for i := range ns.StaleThreads {
			if ns.StaleThreads[i].ThreadID == id {
				return &ns.StaleThreads[i]
			}
		}
		return nil
	}

	// codex's 5-test matrix:
	// (1) loop-monitor + live loop → armed, loop_state alive.
	if s := find(aLive.ThreadID); s == nil || !s.Armed || s.LoopState != "alive" || s.ArmedReason != "loop-alive" || s.WatcherType != "loop-monitor" {
		t.Errorf("live-looping claude: got %+v, want armed=true loop_state=alive reason=loop-alive type=loop-monitor", s)
	}
	// (2) loop-monitor + DEAD loop (heartbeat fresh) → NOT armed, loop_state dead — the false-live bug.
	if s := find(bDead.ThreadID); s == nil || s.Armed || s.LoopState != "dead" || s.ArmedReason != "loop-dead" {
		t.Errorf("loop-dead claude: got %+v, want armed=false loop_state=dead reason=loop-dead", s)
	}
	// (3) app-heartbeat (codex) → armed by heartbeat, loop_state na — NEVER false-flagged.
	if s := find(cCodex.ThreadID); s == nil || !s.Armed || s.LoopState != "na" || s.ArmedReason != "app-heartbeat-fresh" {
		t.Errorf("codex app-heartbeat: got %+v, want armed=true loop_state=na reason=app-heartbeat-fresh", s)
	}
	// (4) native-runloop resident (menubar) → armed by heartbeat, loop_state na, no inbox-worker expectation.
	if s := find(eMenu.ThreadID); s == nil || !s.Armed || s.LoopState != "na" || s.ArmedReason != "resident-runloop-fresh" {
		t.Errorf("menubar native-runloop: got %+v, want armed=true loop_state=na reason=resident-runloop-fresh", s)
	}
	// (5) OS-dead → auto-reaped off the live path (absent from live + stale), ADR-022 safety preserved.
	if s := find(dGone.ThreadID); s != nil {
		t.Errorf("OS-dead thread must auto-reap, still present: %+v", s)
	}
	// (6) surface-loop (gemma) → NOT pgrep-gated (loop-monitor-only verdict): armed by
	// heartbeat, loop_state na. Under a broadened gate this would be armed:false/dead
	// (no watcher proc) — false-negativing a healthy worker. Locks codex-home's #79 verdict.
	if s := find(fGemma.ThreadID); s == nil || !s.Armed || s.LoopState != "na" || s.ArmedReason != "heartbeat-fresh" {
		t.Errorf("gemma surface-loop: got %+v, want armed=true loop_state=na reason=heartbeat-fresh", s)
	}
}
