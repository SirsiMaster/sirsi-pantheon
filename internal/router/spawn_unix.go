//go:build unix

package router

import (
	"syscall"
	"time"
)

// consumerKillGrace is how long a SIGTERMed consumer group gets before SIGKILL.
var consumerKillGrace = 20 * time.Second

// detachedSysProcAttr detaches a fire-and-forget wake spawn from this process's
// controlling terminal/session (Setsid) so the worker survives the short-lived
// `router doctor` tick that nudged it — and, combined with os.Process.Release(),
// leaves no zombie (it reparents to init, which reaps it). Mirrors the
// established cmd/sirsi router-event spawn pattern (codex SME #89, finding 2).
func detachedSysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{Setsid: true}
}

// terminateConsumer ends a stalled consumer and everything it spawned: the
// child is Setsid-detached, so its pid is its process-group id and a negative
// pid addresses the whole group. SIGTERM first; SIGKILL follows after a short
// grace only if the group is still there — one termination, never a loop.
func terminateConsumer(pid int) error {
	if pid <= 0 {
		return nil
	}
	if err := syscall.Kill(-pid, syscall.SIGTERM); err != nil {
		return err
	}
	go func() {
		time.Sleep(consumerKillGrace)
		_ = syscall.Kill(-pid, syscall.SIGKILL)
	}()
	return nil
}
