package platform

import (
	"os/exec"
	"testing"
	"time"
)

// Darwin.Kill now SIGTERMs first and waits up to killGrace before SIGKILL, so a
// slayed process gets a chance to exit cleanly instead of an instant SIGKILL.

func TestDarwinKill_TerminatesProcess(t *testing.T) {
	old := killGrace
	killGrace = 150 * time.Millisecond
	defer func() { killGrace = old }()

	cmd := exec.Command("sleep", "30")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	if err := (&Darwin{}).Kill(cmd.Process.Pid); err != nil {
		t.Fatalf("Kill: %v", err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case <-done: // terminated — good
	case <-time.After(3 * time.Second):
		t.Fatal("process was not terminated by Kill")
	}
}

// A process that ignores SIGTERM must still be killed — Kill escalates to SIGKILL
// after the grace window. Proves the grace is bounded (never hangs) and the
// force-kill fallback works.
func TestDarwinKill_ForcesAfterGraceWhenSigtermIgnored(t *testing.T) {
	old := killGrace
	killGrace = 200 * time.Millisecond
	defer func() { killGrace = old }()

	cmd := exec.Command("sh", "-c", `trap "" TERM; sleep 30`)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	if err := (&Darwin{}).Kill(cmd.Process.Pid); err != nil {
		t.Fatalf("Kill: %v", err)
	}
	// It ignored SIGTERM, so Kill must have waited at least the grace window
	// before escalating to SIGKILL.
	if elapsed := time.Since(start); elapsed < killGrace {
		t.Fatalf("Kill returned in %v, before the %v grace — did it skip SIGTERM?", elapsed, killGrace)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case <-done: // force-killed — good
	case <-time.After(3 * time.Second):
		t.Fatal("SIGTERM-ignoring process not force-killed after grace")
	}
}
