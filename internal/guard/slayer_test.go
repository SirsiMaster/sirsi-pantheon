package guard

import (
	"testing"

	"github.com/SirsiMaster/sirsi-pantheon/internal/platform"
)

// ── SlayWith — live kill path with mock ──────────────────────────────────

func TestSlayWith_NodeDryRun(t *testing.T) {
	result, err := SlayWith(platform.Current(), SlayNode, true)
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
	// Uses mock platform — no real processes harmed
	m := &platform.Mock{
		// Simulate a node process
		CommandResults: map[string]string{
			"ps -axo pid,rss,vsz,%cpu,user,command": "  PID   RSS   VSZ  %CPU USER     COMM\n 9999 51200 81920 1.5 user node",
		},
	}
	result, err := SlayWith(m, SlayNode, false)
	if err != nil {
		t.Fatalf("SlayWith: %v", err)
	}
	if result.DryRun != false {
		t.Error("DryRun should be false")
	}
	t.Logf("Live (mock): %d killed, %d skipped", result.Killed, result.Skipped)
}

func TestSlayWith_AllTargets(t *testing.T) {
	// Test every target type with mock platform
	m := &platform.Mock{}
	targets := ValidSlayTargets()
	for _, target := range targets {
		t.Run(string(target), func(t *testing.T) {
			result, err := SlayWith(m, target, false)
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
	result, err := SlayWith(platform.Current(), SlayAll, true)
	if err != nil {
		t.Fatalf("SlayWith: %v", err)
	}
	if !result.DryRun {
		t.Error("Should be dry run")
	}
	t.Logf("SlayAll dry run: %d matched, %d skipped", result.Killed, result.Skipped)
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
