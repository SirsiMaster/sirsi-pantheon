//go:build !windows

package router

import (
	"errors"
	"os"
	"os/exec"
	"syscall"
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

// EPERM is not death. Signal 0 fails with EPERM when the process EXISTS but is
// owned by another user, so a probe that reads "any error" as gone reports a
// live root-owned process as PIDGone — and PIDGone feeds ReapDeadThreads and
// stale reconciliation directly. pid 1 (launchd/init) is the reliable subject:
// it is always running and never ours. Only ESRCH may answer gone.
// (codex-pantheon re-review of #322, item 20260726-213250.)
func TestDefaultPIDState_PermissionDeniedIsNotGone(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("running as root — signal 0 to pid 1 succeeds, no EPERM to observe")
	}
	if err := syscall.Kill(1, 0); !errors.Is(err, syscall.EPERM) {
		t.Skipf("pid 1 did not answer EPERM on this host (%v) — nothing to pin", err)
	}
	if got := defaultPIDState(1); got == PIDGone {
		t.Error("pid 1 = gone; EPERM means the process exists, never that it died")
	}
}
