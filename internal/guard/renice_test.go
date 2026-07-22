package guard

import (
	"fmt"
	"testing"
)

func TestRenice_LSP(t *testing.T) {
	// Mock ps output with a language_server_macos_arm process
	mockPs := func() ([]orphanPsEntry, error) {
		return []orphanPsEntry{
			{PID: 10789, PPID: 10316, RSS: 2900 * 1024 * 1024, CPU: 10.1, Name: "/path/to/language_server_macos_arm", ElapsedTime: "2:30:00"},
			{PID: 10333, PPID: 10316, RSS: 900 * 1024 * 1024, CPU: 78.0, Name: "Antigravity Helper (Renderer)", ElapsedTime: "2:30:00"},
			{PID: 500, PPID: 1, RSS: 100 * 1024 * 1024, CPU: 5.0, Name: "gopls", ElapsedTime: "1:00:00"},
		}, nil
	}

	reniceCalled := 0
	mockRenice := func(pid int, nice int) error {
		reniceCalled++
		if nice != 10 {
			t.Errorf("expected nice=10, got %d", nice)
		}
		return nil
	}

	taskpolicyCalled := 0
	mockTaskpolicy := func(pid int) error {
		taskpolicyCalled++
		return nil
	}

	result, err := reniceWith(ReniceTargetLSP, mockPs, mockRenice, mockTaskpolicy)
	if err != nil {
		t.Fatalf("Renice failed: %v", err)
	}

	// Should renice language_server_macos_arm and gopls, but NOT the Renderer
	if result.Reniced != 2 {
		t.Errorf("expected 2 processes reniced, got %d", result.Reniced)
	}
	if reniceCalled != 2 {
		t.Errorf("expected renice called 2 times, got %d", reniceCalled)
	}
	if taskpolicyCalled != 2 {
		t.Errorf("expected taskpolicy called 2 times, got %d", taskpolicyCalled)
	}

	// Verify the right PIDs
	pids := map[int]bool{}
	for _, p := range result.Processes {
		pids[p.PID] = true
	}
	if !pids[10789] {
		t.Error("language_server_macos_arm (PID 10789) should have been reniced")
	}
	if !pids[500] {
		t.Error("gopls (PID 500) should have been reniced")
	}
	if pids[10333] {
		t.Error("Renderer (PID 10333) should NOT have been reniced")
	}
}

func TestRenice_ReniceFails(t *testing.T) {
	mockPs := func() ([]orphanPsEntry, error) {
		return []orphanPsEntry{
			{PID: 100, PPID: 1, RSS: 50 * 1024 * 1024, CPU: 5.0, Name: "gopls"},
		}, nil
	}

	mockRenice := func(pid int, nice int) error {
		return fmt.Errorf("permission denied")
	}

	mockTaskpolicy := func(pid int) error {
		return nil
	}

	result, err := reniceWith(ReniceTargetLSP, mockPs, mockRenice, mockTaskpolicy)
	if err != nil {
		t.Fatalf("Renice should not return error on per-process failure: %v", err)
	}

	if result.Reniced != 0 {
		t.Errorf("expected 0 reniced, got %d", result.Reniced)
	}
	if result.Skipped != 1 {
		t.Errorf("expected 1 skipped, got %d", result.Skipped)
	}
	if len(result.Errors) != 1 {
		t.Errorf("expected 1 error, got %d", len(result.Errors))
	}
}

func TestRenice_NoMatches(t *testing.T) {
	mockPs := func() ([]orphanPsEntry, error) {
		return []orphanPsEntry{
			{PID: 10333, PPID: 10316, RSS: 900 * 1024 * 1024, CPU: 78.0, Name: "Antigravity Helper (Renderer)"},
		}, nil
	}

	result, err := reniceWith(ReniceTargetLSP, mockPs,
		func(int, int) error { return nil },
		func(int) error { return nil },
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Reniced != 0 {
		t.Errorf("expected 0 reniced, got %d", result.Reniced)
	}
}

func TestRenice_SkipsKernel(t *testing.T) {
	mockPs := func() ([]orphanPsEntry, error) {
		return []orphanPsEntry{
			{PID: 0, PPID: 0, RSS: 0, CPU: 0, Name: "language_server_macos_arm"}, // PID 0 = kernel
			{PID: 1, PPID: 0, RSS: 0, CPU: 0, Name: "gopls"},                     // PID 1 = launchd
		}, nil
	}

	result, err := reniceWith(ReniceTargetLSP, mockPs,
		func(int, int) error { t.Error("should not renice PID 0 or 1"); return nil },
		func(int) error { return nil },
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Reniced != 0 {
		t.Errorf("expected 0 reniced (kernel/launchd skipped), got %d", result.Reniced)
	}
	if result.Skipped != 2 {
		t.Errorf("expected 2 skipped, got %d", result.Skipped)
	}
}

func TestFormatReniceReport_Empty(t *testing.T) {
	r := &ReniceResult{Target: ReniceTargetLSP}
	report := FormatReniceReport(r)
	if report == "" {
		t.Error("expected non-empty report")
	}
}

func TestFormatReniceReport_WithProcesses(t *testing.T) {
	r := &ReniceResult{
		Target:  ReniceTargetLSP,
		Reniced: 1,
		Processes: []RenicedProcess{
			{PID: 10789, Name: "language_server_macos_arm", RSS: 2900 * 1024 * 1024, RSSHuman: "2.9 GB", OldNice: 0, NewNice: 10, QoS: "background"},
		},
	}
	report := FormatReniceReport(r)
	if report == "" {
		t.Error("expected non-empty report")
	}
	if !contains(report, "10789") {
		t.Error("report should contain PID")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// TestShouldRenice_AgentsTarget locks the cross-agent QoS matcher (ADR-031-B):
// agent CLIs and local model runners match; sirsi itself must never (A1).
func TestShouldRenice_AgentsTarget(t *testing.T) {
	for name, want := range map[string]bool{
		"claude":        true,
		"codex":         true,
		"mlx_lm.server": true,
		"gemma-runner":  true,
		"Safari":        false,
		"gopls":         false,
	} {
		if got := shouldRenice(ReniceTargetAgents, name); got != want {
			t.Errorf("shouldRenice(agents, %q) = %v, want %v", name, got, want)
		}
	}
	// "all" now includes agents alongside LSPs.
	if !shouldRenice(ReniceTargetAll, "codex") {
		t.Error("shouldRenice(all, codex) = false, want true")
	}
}

// TestReniceWith_SkipsProtected proves the A1 protected list holds at the
// reniceWith layer for every target: sirsi-gemma-worker matches the agents
// pattern set ("gemma") but MUST be skipped, never reniced.
func TestReniceWith_SkipsProtected(t *testing.T) {
	ps := func() ([]orphanPsEntry, error) {
		return []orphanPsEntry{
			{PID: 4242, Name: "sirsi-gemma-worker.sh", RSS: 1 << 20},
			{PID: 4243, Name: "codex", RSS: 1 << 20},
		}, nil
	}
	var reniced []int
	res, err := reniceWith(ReniceTargetAgents, ps,
		func(pid, _ int) error { reniced = append(reniced, pid); return nil },
		func(int) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	if res.Reniced != 1 || res.Skipped != 1 {
		t.Fatalf("reniced=%d skipped=%d, want 1/1", res.Reniced, res.Skipped)
	}
	if len(reniced) != 1 || reniced[0] != 4243 {
		t.Fatalf("reniced pids = %v, want [4243] only (protected worker skipped)", reniced)
	}
}

// TestMatchesAgent_ExcludesGUIBundles: the claude CLI matches; the Claude
// DESKTOP app (an .app bundle path — the owner's foreground GUI) never does.
func TestMatchesAgent_ExcludesGUIBundles(t *testing.T) {
	if matchesAgent("/applications/claude.app/contents/macos/claude") {
		t.Error("Claude.app (GUI bundle) must not match the agents target")
	}
	if !matchesAgent("claude") {
		t.Error("claude CLI must match the agents target")
	}
	// claude-code's headless runner lives in an .app-shaped dir under
	// Application Support — it IS a background agent and must match.
	if !matchesAgent("/users/x/library/application support/claude/claude-code/2.1.215/claude.app/contents/macos/claude") {
		t.Error("claude-code runner under Application Support must match")
	}
}
