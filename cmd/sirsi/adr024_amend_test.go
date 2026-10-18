package main

// ADR-024 Amendment 1 — (2) worker-lifecycle gate.
// In-package test (not main_test) so it can stub the unexported oneShotProbe and
// exercise the gate decision directly — proving it is SELECTIVE (interactive /
// resident surfaces still register under the same path that refuses a worker),
// per claude-home's review note, without spawning real `--print` processes.

import "testing"

func TestADR024Amend_OnlyAgentPrintModeIsEphemeral(t *testing.T) {
	for _, tc := range []struct {
		command string
		want    bool
	}{
		{"/usr/local/bin/claude --print hello", true},
		{"claude -p hello", true},
		{"claude --print=hello", true},
		{"claude --resume session", false},
		{"python3 sirsi-maat-run --work test -- sh -c go test -p 2", false},
		{"/bin/zsh -c echo --print", false},
		{"claude -- --print", false},
	} {
		if got := oneShotCommand(tc.command); got != tc.want {
			t.Errorf("%q: %v want %v", tc.command, got, tc.want)
		}
	}
}

func TestADR024Amend_WorkerLifecycleGate_Selective(t *testing.T) {
	old := oneShotProbe
	defer func() { oneShotProbe = old }()

	// Stub: pid 111 is a one-shot (--print/-p) worker; pid 222 is an
	// interactive/resident surface (claude REPL, menubar, …).
	oneShotProbe = func(pid int) bool { return pid == 111 }

	if !ephemeralWorkerSkip(111) {
		t.Error("a one-shot worker (pid 111) must be refused persistent registration")
	}
	if ephemeralWorkerSkip(222) {
		t.Error("an interactive/resident surface (pid 222) must register under the SAME gate — selective, not blanket")
	}
	if ephemeralWorkerSkip(0) || ephemeralWorkerSkip(-1) {
		t.Error("a non-positive anchor is unverifiable and must never be skipped")
	}
}
