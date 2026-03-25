package sight

import (
	"errors"
	"testing"
)

// ── Mock runners ─────────────────────────────────────────────────────────

func mockRunnerSuccess(name string, args ...string) error {
	return nil
}

func mockRunnerFail(name string, args ...string) error {
	return errors.New("mock command failed")
}

// Track which commands were called
type callTracker struct {
	calls []string
}

func (ct *callTracker) runner(name string, args ...string) error {
	ct.calls = append(ct.calls, name)
	return nil
}

// ── FixWith ──────────────────────────────────────────────────────────────

func TestFixWith_DryRun(t *testing.T) {
	err := FixWith(true, mockRunnerSuccess)
	if err != nil {
		t.Errorf("FixWith dry run should not error: %v", err)
	}
}

func TestFixWith_Success(t *testing.T) {
	tracker := &callTracker{}
	err := FixWith(false, tracker.runner)
	if err != nil {
		t.Errorf("FixWith success: %v", err)
	}
	// Should call lsregister and killall
	if len(tracker.calls) != 2 {
		t.Errorf("Expected 2 commands, got %d: %v", len(tracker.calls), tracker.calls)
	}
}

func TestFixWith_LsregisterFails(t *testing.T) {
	err := FixWith(false, mockRunnerFail)
	if err == nil {
		t.Error("FixWith should error when lsregister fails")
	}
}

func TestFixWith_FinderKillFails(t *testing.T) {
	// Custom runner: lsregister succeeds, killall fails (non-fatal)
	callCount := 0
	runner := func(name string, args ...string) error {
		callCount++
		if callCount == 2 {
			return errors.New("killall failed")
		}
		return nil
	}
	err := FixWith(false, runner)
	if err != nil {
		t.Errorf("Finder kill failure should be non-fatal: %v", err)
	}
}

// ── ReindexSpotlightWith ────────────────────────────────────────────────

func TestReindexSpotlightWith_DryRun(t *testing.T) {
	err := ReindexSpotlightWith(true, mockRunnerSuccess)
	if err != nil {
		t.Errorf("ReindexSpotlightWith dry run: %v", err)
	}
}

func TestReindexSpotlightWith_Success(t *testing.T) {
	tracker := &callTracker{}
	err := ReindexSpotlightWith(false, tracker.runner)
	if err != nil {
		t.Errorf("ReindexSpotlightWith success: %v", err)
	}
	if len(tracker.calls) != 1 {
		t.Errorf("Expected 1 command, got %d", len(tracker.calls))
	}
}

func TestReindexSpotlightWith_Failure(t *testing.T) {
	err := ReindexSpotlightWith(false, mockRunnerFail)
	if err == nil {
		t.Error("ReindexSpotlightWith should error when mdutil fails")
	}
}

// ── DefaultRunner ────────────────────────────────────────────────────────

func TestDefaultRunner_EchoSuccess(t *testing.T) {
	err := defaultRunner("echo", "test")
	if err != nil {
		t.Errorf("defaultRunner echo: %v", err)
	}
}

func TestDefaultRunner_BadCommand(t *testing.T) {
	err := defaultRunner("nonexistent-command-that-does-not-exist-xyz")
	if err == nil {
		t.Error("defaultRunner should error for bad command")
	}
}

// ── GhostRegistration struct ─────────────────────────────────────────────

func TestGhostRegistration_AllFields(t *testing.T) {
	g := GhostRegistration{
		BundleID: "com.example.test",
		Path:     "/Applications/Test.app",
		Name:     "Test App",
	}
	if g.BundleID == "" || g.Path == "" || g.Name == "" {
		t.Error("All fields should be set")
	}
}

func TestSightResult_AllFields(t *testing.T) {
	r := SightResult{
		GhostRegistrations: []GhostRegistration{{BundleID: "test"}},
		TotalGhosts:        1,
		CanFix:             true,
	}
	if r.TotalGhosts != 1 {
		t.Error("TotalGhosts mismatch")
	}
}
