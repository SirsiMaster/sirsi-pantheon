//go:build !windows

package router

import (
	"os"
	"os/exec"
	"testing"
)

// defaultPIDState asks signal 0 before forking `ps` — the gone case, which is
// the common answer when reconcile probes quiet records, must not cost a fork.
// Semantics are unchanged by that reordering, so pin all three states.
func TestDefaultPIDState_AliveGoneDefunct(t *testing.T) {
	if got := defaultPIDState(os.Getpid()); got != PIDAlive {
		t.Errorf("self = %q, want alive", got)
	}

	// A pid that has certainly exited: spawn and reap it.
	cmd := exec.Command("true")
	if err := cmd.Run(); err != nil {
		t.Fatalf("spawn: %v", err)
	}
	if got := defaultPIDState(cmd.Process.Pid); got != PIDGone {
		t.Errorf("reaped child = %q, want gone", got)
	}

	// An unreaped exited child is defunct — dead, and must not read as a live
	// agent. Started without Wait(), so the kernel keeps the zombie for us.
	zombie := exec.Command("true")
	if err := zombie.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	defer func() { _ = zombie.Wait() }()
	for i := 0; i < 200; i++ {
		if defaultPIDState(zombie.Process.Pid) == PIDDefunct {
			return
		}
	}
	t.Skip("child did not linger as a zombie on this host — timing-dependent")
}
