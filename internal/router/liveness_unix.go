//go:build !windows

package router

import (
	"errors"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
)

// defaultPIDState reports OS-truth liveness on unix-like systems.
//
// Signal 0 is asked FIRST because it is a syscall, not a fork: it settles the
// gone/not-gone question outright, and "gone" is the common answer on the hot
// path (reconcile probes records precisely because they have gone quiet, and
// most such sessions really did exit). Only a process that still exists needs
// `ps`, whose state code is the one thing the syscall cannot report: a leading
// "Z" means defunct — dead but unreaped, and must NOT count as a live agent.
//
// The prior order forked `ps` for every probe including the gone ones. Once
// reconcile began probing each stale record (rather than suspending on the
// clock alone) that became a fork per record per pass, which measurably slowed
// a loaded host — the cmd/sirsi integration test crossed its 60s deadline.
// If `ps` is unavailable we keep the old fallback: exists, defunctness unknown.
//
// The error is read, not merely tested for nil: only ESRCH proves the PID is
// gone. Signal 0 returns EPERM when the process EXISTS but belongs to another
// user — routine on this host, where root-owned launchd services sit next to
// user sessions. Treating every error as gone would feed a false PIDGone
// straight into ReapDeadThreads and stale reconciliation, which is the one
// direction this probe must never fail in. EPERM and any other indeterminate
// errno therefore answer PIDUnknown, which callers already handle
// conservatively (ReconcileExits falls through to idle-based behavior rather
// than acting on it).
func defaultPIDState(pid int) PIDState {
	switch err := syscall.Kill(pid, 0); {
	case err == nil: // exists — ps refines alive vs defunct below
	case errors.Is(err, syscall.ESRCH):
		return PIDGone
	default: // EPERM and friends: it exists, or we cannot tell. Never "gone".
		return PIDUnknown
	}
	out, err := exec.Command("ps", "-o", "stat=", "-p", strconv.Itoa(pid)).Output()
	state := strings.TrimSpace(string(out))
	if err != nil || state == "" {
		return PIDAlive // exists per signal 0; ps could not refine it
	}
	if strings.HasPrefix(strings.ToUpper(state), "Z") {
		return PIDDefunct
	}
	return PIDAlive
}

// defaultPIDStart returns the process start signature on unix-like systems via
// `ps -o lstart=` — a stable per-process boot timestamp that disambiguates a
// recycled PID (a different process reusing the number carries a newer lstart).
// One cheap shell-out, same shape as the stat= probe. Empty when ps fails or the
// PID is gone (caller treats empty as "no discriminator" and keeps bare-PID
// semantics — never a false reap).
func defaultPIDStart(pid int) string {
	out, err := exec.Command("ps", "-o", "lstart=", "-p", strconv.Itoa(pid)).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// defaultPIDCommand returns the full command line for pid. Empty means
// unavailable/unknowable; callers must treat that as non-fatal.
func defaultPIDCommand(pid int) string {
	out, err := exec.Command("ps", "-o", "command=", "-p", strconv.Itoa(pid)).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
