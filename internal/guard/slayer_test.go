package guard

import (
	"errors"
	"testing"
)

// ── Mock killers ─────────────────────────────────────────────────────────

func mockKillerSuccess(pid int) error {
	return nil
}

func mockKillerFail(pid int) error {
	return errors.New("mock kill failed")
}

// ── SlayWith — live kill path with mock ──────────────────────────────────

func TestSlayWith_NodeDryRun(t *testing.T) {
	result, err := SlayWith(SlayNode, true, mockKillerSuccess)
	if err != nil {
		t.Fatalf("SlayWith: %v", err)
	}
	if result.DryRun != true {
		t.Error("DryRun should be true")
	}
	// Dry run tallies would-be kills
	t.Logf("Dry run: %d killed, %d skipped, %d failed", result.Killed, result.Skipped, result.Failed)
}

func TestSlayWith_NodeLive(t *testing.T) {
	// Uses mock killer — no real processes harmed
	result, err := SlayWith(SlayNode, false, mockKillerSuccess)
	if err != nil {
		t.Fatalf("SlayWith: %v", err)
	}
	if result.DryRun != false {
		t.Error("DryRun should be false")
	}
	// All kills should succeed with mock
	if result.Failed != 0 {
		t.Errorf("Failed = %d with mock killer", result.Failed)
	}
	t.Logf("Live (mock): %d killed, %d skipped", result.Killed, result.Skipped)
}

func TestSlayWith_LiveKillFailure(t *testing.T) {
	// Mock killer that always fails — tests the error accumulation path
	result, err := SlayWith(SlayNode, false, mockKillerFail)
	if err != nil {
		t.Fatalf("SlayWith: %v", err)
	}
	// If there were any node processes, they should all fail
	if result.Killed != 0 && result.Failed == 0 {
		t.Error("Mock killer should cause failures, not successes")
	}
	t.Logf("Live (fail): %d killed, %d failed, %d errors", result.Killed, result.Failed, len(result.Errors))
}

func TestSlayWith_AllTargets(t *testing.T) {
	// Test every target type with mock killer
	targets := ValidSlayTargets()
	for _, target := range targets {
		t.Run(string(target), func(t *testing.T) {
			result, err := SlayWith(target, false, mockKillerSuccess)
			if err != nil {
				t.Fatalf("SlayWith(%s): %v", target, err)
			}
			if result.Target != target {
				t.Errorf("Target = %s, want %s", result.Target, target)
			}
		})
	}
}

func TestSlayWith_AllDryRun(t *testing.T) {
	result, err := SlayWith(SlayAll, true, mockKillerSuccess)
	if err != nil {
		t.Fatalf("SlayWith: %v", err)
	}
	if !result.DryRun {
		t.Error("Should be dry run")
	}
	t.Logf("SlayAll dry run: %d matched, %d skipped", result.Killed, result.Skipped)
}

// ── StatusJSON ───────────────────────────────────────────────────────────

func TestAlertRing_RecentJSON(t *testing.T) {
	ring := NewAlertRing()

	// Push some entries
	ring.Push(AlertEntry{ProcessName: "test", PID: 1, CPUPercent: 95, Severity: "warning"})
	ring.Push(AlertEntry{ProcessName: "test2", PID: 2, CPUPercent: 200, Severity: "critical"})

	recent := ring.Recent(5)
	if len(recent) != 2 {
		t.Errorf("Recent(5) = %d entries, want 2", len(recent))
	}
	if recent[0].Severity != "critical" {
		t.Errorf("Most recent should be critical, got %q", recent[0].Severity)
	}
	current, lifetime := ring.Stats()
	if current != 2 {
		t.Errorf("current = %d, want 2", current)
	}
	if lifetime != 2 {
		t.Errorf("lifetime = %d, want 2", lifetime)
	}
}

// ── parseVMStatValue ─────────────────────────────────────────────────────

func TestParseVMStatValue_Valid(t *testing.T) {
	tests := []struct {
		line     string
		expected int64
	}{
		{"Pages free:                              123456.", 123456},
		{"Pages active:                             78901.", 78901},
		{"Pages inactive:                          456789.", 456789},
	}

	for _, tt := range tests {
		val := parseVMStatValue(tt.line)
		if val != tt.expected {
			t.Errorf("parseVMStatValue(%q) = %d, want %d", tt.line, val, tt.expected)
		}
	}
}

func TestParseVMStatValue_Invalid(t *testing.T) {
	tests := []string{
		"",
		"no colon here",
		"Pages free:    abc.",
	}

	for _, line := range tests {
		val := parseVMStatValue(line)
		if val != 0 {
			t.Errorf("parseVMStatValue(%q) = %d, want 0 for invalid", line, val)
		}
	}
}

// ── isProtectedProcess — comprehensive ──────────────────────────────────

func TestIsProtectedProcess_Root(t *testing.T) {
	p := ProcessInfo{User: "root", PID: 100, Name: "something"}
	if !isProtectedProcess(p) {
		t.Error("root processes should be protected")
	}
}

func TestIsProtectedProcess_SystemUser(t *testing.T) {
	for _, user := range []string{"_windowserver", "_coreaudiod"} {
		p := ProcessInfo{User: user, PID: 100, Name: "something"}
		if !isProtectedProcess(p) {
			t.Errorf("User %q should be protected", user)
		}
	}
}

func TestIsProtectedProcess_PID1(t *testing.T) {
	p := ProcessInfo{User: "me", PID: 1, Name: "launchd"}
	if !isProtectedProcess(p) {
		t.Error("PID 1 should be protected")
	}
}

func TestIsProtectedProcess_ProtectedNames(t *testing.T) {
	names := []string{"kernel_task", "WindowServer", "loginwindow", "Dock", "Finder"}
	for _, name := range names {
		p := ProcessInfo{User: "me", PID: 9999, Name: name}
		if !isProtectedProcess(p) {
			t.Errorf("Process %q should be protected", name)
		}
	}
}

func TestIsProtectedProcess_SafeToKill(t *testing.T) {
	p := ProcessInfo{User: "me", PID: 9999, Name: "node"}
	if isProtectedProcess(p) {
		t.Error("Regular node process should NOT be protected")
	}
}
