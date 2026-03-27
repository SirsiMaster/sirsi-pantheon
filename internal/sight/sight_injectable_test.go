package sight

import (
	"errors"
	"testing"
)

// ── Mock runners ─────────────────────────────────────────────────────────

type mockRunner struct {
	RunFunc    func(name string, args ...string) error
	OutputFunc func(name string, args ...string) ([]byte, error)
	Calls      []string
}

func (m *mockRunner) Run(name string, args ...string) error {
	m.Calls = append(m.Calls, name)
	if m.RunFunc != nil {
		return m.RunFunc(name, args...)
	}
	return nil
}

func (m *mockRunner) Output(name string, args ...string) ([]byte, error) {
	m.Calls = append(m.Calls, name)
	if m.OutputFunc != nil {
		return m.OutputFunc(name, args...)
	}
	return nil, nil
}

func mockRunnerSuccess() CommandRunner {
	return &mockRunner{}
}

func mockRunnerFail() CommandRunner {
	return &mockRunner{
		RunFunc: func(name string, args ...string) error {
			return errors.New("mock command failed")
		},
		OutputFunc: func(name string, args ...string) ([]byte, error) {
			return nil, errors.New("mock command failed")
		},
	}
}

// ── FixWith ──────────────────────────────────────────────────────────────

func TestFixWith_DryRun(t *testing.T) {
	err := FixWith(true, mockRunnerSuccess())
	if err != nil {
		t.Errorf("FixWith dry run should not error: %v", err)
	}
}

func TestFixWith_Success(t *testing.T) {
	runner := &mockRunner{}
	err := FixWith(false, runner)
	if err != nil {
		t.Errorf("FixWith success: %v", err)
	}
	// Should call lsregister and killall
	if len(runner.Calls) != 2 {
		t.Errorf("Expected 2 commands, got %d: %v", len(runner.Calls), runner.Calls)
	}
}

func TestFixWith_LsregisterFails(t *testing.T) {
	err := FixWith(false, mockRunnerFail())
	if err == nil {
		t.Error("FixWith should error when lsregister fails")
	}
}

func TestFixWith_FinderKillFails(t *testing.T) {
	// Custom runner: lsregister succeeds, killall fails (non-fatal)
	callCount := 0
	runner := &mockRunner{
		RunFunc: func(name string, args ...string) error {
			callCount++
			if callCount == 2 {
				return errors.New("killall failed")
			}
			return nil
		},
	}
	err := FixWith(false, runner)
	if err != nil {
		t.Errorf("Finder kill failure should be non-fatal: %v", err)
	}
}

// ── ReindexSpotlightWith ────────────────────────────────────────────────

func TestReindexSpotlightWith_DryRun(t *testing.T) {
	err := ReindexSpotlightWith(true, mockRunnerSuccess())
	if err != nil {
		t.Errorf("ReindexSpotlightWith dry run: %v", err)
	}
}

func TestReindexSpotlightWith_Success(t *testing.T) {
	runner := &mockRunner{}
	err := ReindexSpotlightWith(false, runner)
	if err != nil {
		t.Errorf("ReindexSpotlightWith success: %v", err)
	}
	if len(runner.Calls) != 1 {
		t.Errorf("Expected 1 command, got %d", len(runner.Calls))
	}
}

func TestReindexSpotlightWith_Failure(t *testing.T) {
	err := ReindexSpotlightWith(false, mockRunnerFail())
	if err == nil {
		t.Error("ReindexSpotlightWith should error when mdutil fails")
	}
}

// ── DefaultRunner ────────────────────────────────────────────────────────

func TestDefaultRunner_EchoSuccess(t *testing.T) {
	runner := defaultRunner{}
	err := runner.Run("echo", "test")
	if err != nil {
		t.Errorf("defaultRunner echo: %v", err)
	}
}

func TestDefaultRunner_BadCommand(t *testing.T) {
	runner := defaultRunner{}
	err := runner.Run("nonexistent-command-that-does-not-exist-xyz")
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
	if len(r.GhostRegistrations) != 1 || r.GhostRegistrations[0].BundleID != "test" {
		t.Error("GhostRegistrations mismatch")
	}
	if !r.CanFix {
		t.Error("CanFix should be true")
	}
}
