package main

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"
	"testing"
)

func ancestryLookup(entries map[int]anchorProcess) anchorProcessLookup {
	return func(pid int) (anchorProcess, error) {
		proc, ok := entries[pid]
		if !ok {
			return anchorProcess{}, fmt.Errorf("pid %d not found", pid)
		}
		return proc, nil
	}
}

func TestResolveDurableAnchorWalksPastTransientCodexHelpers(t *testing.T) {
	lookup := ancestryLookup(map[int]anchorProcess{
		900: {parentPID: 800, command: "zsh"},
		800: {parentPID: 700, command: "unified-exec-helper"},
		700: {parentPID: 600, command: "Codex Helper (Renderer)"},
		600: {parentPID: 1, command: "/Applications/ChatGPT.app/Contents/Resources/codex-code-mode-host"},
	})
	got, err := resolveDurableAnchor(900, "codex", lookup)
	if err != nil {
		t.Fatalf("resolveDurableAnchor: %v", err)
	}
	if got != 600 {
		t.Fatalf("anchor=%d, want persistent codex host 600", got)
	}
}

func TestResolveDurableAnchorFindsClaudeAtVariableDepth(t *testing.T) {
	lookup := ancestryLookup(map[int]anchorProcess{
		50: {parentPID: 40, command: "python3"},
		40: {parentPID: 30, command: "sh"},
		30: {parentPID: 20, command: "Claude Helper (GPU)"},
		20: {parentPID: 1, command: "/usr/local/bin/claude-code"},
	})
	got, err := resolveDurableAnchor(50, "claude", lookup)
	if err != nil {
		t.Fatalf("resolveDurableAnchor: %v", err)
	}
	if got != 20 {
		t.Fatalf("anchor=%d, want durable claude host 20", got)
	}
}

func TestResolveDurableAnchorSupportsEveryNamedInteractiveSurface(t *testing.T) {
	tests := []struct {
		surface string
		command string
	}{
		{"codex", "codex"},
		{"claude", "Claude"},
		{"gemini", "gemini-cli"},
		{"gemma", "sirsi-gemma-worker"},
		{"qwen", "qwen-agent"},
	}
	for _, tt := range tests {
		t.Run(tt.surface, func(t *testing.T) {
			lookup := ancestryLookup(map[int]anchorProcess{10: {parentPID: 1, command: tt.command}})
			got, err := resolveDurableAnchor(10, tt.surface, lookup)
			if err != nil || got != 10 {
				t.Fatalf("anchor=%d err=%v, want 10", got, err)
			}
		})
	}
}

func TestRefineDesktopAnchorSelectsUniqueCodeModeHost(t *testing.T) {
	original := lookupAnchorProcess
	lookupAnchorProcess = func(pid int) (anchorProcess, error) {
		if pid != 100 {
			return anchorProcess{}, fmt.Errorf("unexpected pid %d", pid)
		}
		return anchorProcess{parentPID: 1, command: "/Applications/ChatGPT.app/Contents/Resources/codex"}, nil
	}
	t.Cleanup(func() { lookupAnchorProcess = original })

	children := func(parentPID int) ([]anchorChild, error) {
		return []anchorChild{
			{pid: 101, process: anchorProcess{parentPID: parentPID, command: "crashpad_handler"}},
			{pid: 102, process: anchorProcess{parentPID: parentPID, command: "/Applications/ChatGPT.app/Contents/Resources/codex-code-mode-host"}},
		}, nil
	}
	got, err := refineDesktopAnchor(100, "codex", "Codex Desktop", children)
	if err != nil || got != 102 {
		t.Fatalf("anchor=%d err=%v, want task host 102", got, err)
	}
}

func TestRefineDesktopAnchorFailsClosedOnAmbiguousHosts(t *testing.T) {
	original := lookupAnchorProcess
	lookupAnchorProcess = func(pid int) (anchorProcess, error) {
		return anchorProcess{parentPID: 1, command: "codex"}, nil
	}
	t.Cleanup(func() { lookupAnchorProcess = original })

	children := func(parentPID int) ([]anchorChild, error) {
		return []anchorChild{
			{pid: 101, process: anchorProcess{parentPID: parentPID, command: "codex-code-mode-host"}},
			{pid: 102, process: anchorProcess{parentPID: parentPID, command: "codex-code-mode-host"}},
		}, nil
	}
	if got, err := refineDesktopAnchor(100, "codex", "Codex Desktop", children); err == nil || got != 0 {
		t.Fatalf("anchor=%d err=%v, want fail-closed ambiguity", got, err)
	}
}

func TestResolveDurableAnchorFailsClosedForUnknownResidentSurface(t *testing.T) {
	lookup := ancestryLookup(map[int]anchorProcess{
		10: {parentPID: 9, command: "sh"},
		9:  {parentPID: 1, command: "resident-custom-daemon"},
	})
	if got, err := resolveDurableAnchor(10, "worker", lookup); err == nil || got != 0 {
		t.Fatalf("anchor=%d err=%v, want fail-closed", got, err)
	}
}

func TestResolveDurableAnchorFailsOnAmbiguousOrBrokenAncestry(t *testing.T) {
	t.Run("cycle", func(t *testing.T) {
		lookup := ancestryLookup(map[int]anchorProcess{
			10: {parentPID: 9, command: "sh"},
			9:  {parentPID: 10, command: "python3"},
		})
		if _, err := resolveDurableAnchor(10, "codex", lookup); err == nil {
			t.Fatal("expected ancestry-cycle error")
		}
	})
	t.Run("lookup failure", func(t *testing.T) {
		if _, err := resolveDurableAnchor(10, "codex", ancestryLookup(nil)); err == nil {
			t.Fatal("expected lookup error")
		}
	})
}

// TestResolveAnchorPIDFallsBackToResidentAnchorWhenPsIsSandboxDenied covers
// sandboxed-consumer-session-binding: a Codex-style sandbox refuses fork/exec
// of `ps` entirely (EPERM), so the ancestry walk can never answer. Thread
// registration must still succeed, anchored at the immediate parent, rather
// than refusing to register at all.
func TestResolveAnchorPIDFallsBackToResidentAnchorWhenPsIsSandboxDenied(t *testing.T) {
	original := lookupAnchorProcess
	lookupAnchorProcess = func(pid int) (anchorProcess, error) {
		return anchorProcess{}, &exec.Error{Name: "ps", Err: &os.SyscallError{Syscall: "fork/exec", Err: syscall.EPERM}}
	}
	t.Cleanup(func() { lookupAnchorProcess = original })

	got, err := resolveAnchorPID("codex")
	if err != nil {
		t.Fatalf("resolveAnchorPID: unexpected error %v", err)
	}
	if want := os.Getppid(); got != want {
		t.Fatalf("anchor=%d, want resident fallback to parent pid %d", got, want)
	}
}

// TestResolveAnchorPIDStillFailsClosedOnNonPermissionErrors confirms the
// sandbox fallback is scoped to permission denial only — an ordinary lookup
// failure (pid gone, ambiguous ancestry, etc.) must still fail closed rather
// than silently anchoring somewhere unproven.
func TestResolveAnchorPIDStillFailsClosedOnNonPermissionErrors(t *testing.T) {
	original := lookupAnchorProcess
	lookupAnchorProcess = func(pid int) (anchorProcess, error) {
		return anchorProcess{}, fmt.Errorf("inspect pid %d: no such process", pid)
	}
	t.Cleanup(func() { lookupAnchorProcess = original })

	if got, err := resolveAnchorPID("codex"); err == nil {
		t.Fatalf("anchor=%d, want error for a non-permission lookup failure", got)
	}
}

func TestDurableRuntimeForSurfaceRejectsHelpersAndCrossSurfaceMatches(t *testing.T) {
	for _, command := range []string{"Claude Helper (Renderer)", "codex-helper", "codex-exec", "claude-wrapper", "python3", "zsh"} {
		if durableRuntimeForSurface("claude", command) {
			t.Fatalf("claude matcher accepted transient/cross-surface command %q", command)
		}
	}
	if durableRuntimeForSurface("codex", "codex-exec") {
		t.Fatal("codex matcher accepted prefix-only helper identity")
	}
	if durableRuntimeForSurface("codex", "claude") {
		t.Fatal("codex matcher accepted claude")
	}
}
