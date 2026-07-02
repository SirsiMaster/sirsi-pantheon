package main

import (
	"os"
	"testing"
)

// The no-args launch decision must keep every non-interactive context printing
// help byte-for-byte unchanged. In the test harness stdin/stdout are pipes (not
// TTYs), so shouldLaunchTUI must return false regardless of flags — the console
// only ever opens on a genuine terminal. These tests also pin the explicit
// opt-outs (SIRSI_NO_TUI, --json, --quiet). They swap package globals, so they
// must NOT run in parallel.

func TestShouldNotLaunchTUIWhenNotATerminal(t *testing.T) {
	// Under `go test`, stdin/stdout are not TTYs → never launch.
	if shouldLaunchTUI() {
		t.Error("shouldLaunchTUI() = true in a non-terminal test harness; want false (help path)")
	}
}

func TestShouldLaunchTUIRespectsOptOuts(t *testing.T) {
	// Even if we could not observe the TTY, the explicit opt-outs must force the
	// help path. We assert each guard independently.
	t.Setenv("SIRSI_NO_TUI", "1")
	if shouldLaunchTUI() {
		t.Error("SIRSI_NO_TUI=1 must force the help path")
	}
	os.Unsetenv("SIRSI_NO_TUI")

	oldJSON, oldQuiet := JsonOutput, quietMode
	defer func() { JsonOutput, quietMode = oldJSON, oldQuiet }()

	JsonOutput, quietMode = true, false
	if shouldLaunchTUI() {
		t.Error("--json must force the help path")
	}
	JsonOutput, quietMode = false, true
	if shouldLaunchTUI() {
		t.Error("--quiet must force the help path")
	}
}

// isTerminal must classify a real pipe (os.Pipe) as non-interactive — the exact
// case that decides help-vs-TUI for `echo | sirsi` and `sirsi > file`.
func TestIsTerminalRejectsPipe(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	defer r.Close()
	defer w.Close()
	if isTerminal(r.Fd()) {
		t.Error("isTerminal(pipe) = true; want false")
	}
}
