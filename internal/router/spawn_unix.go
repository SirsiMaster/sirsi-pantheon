//go:build unix

package router

import "syscall"

// detachedSysProcAttr detaches a fire-and-forget wake spawn from this process's
// controlling terminal/session (Setsid) so the worker survives the short-lived
// `router doctor` tick that nudged it — and, combined with os.Process.Release(),
// leaves no zombie (it reparents to init, which reaps it). Mirrors the
// established cmd/sirsi router-event spawn pattern (codex SME #89, finding 2).
func detachedSysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{Setsid: true}
}
