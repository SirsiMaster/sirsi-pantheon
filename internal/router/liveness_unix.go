//go:build !windows

package router

import (
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
func defaultPIDState(pid int) PIDState {
	if syscall.Kill(pid, 0) != nil {
		return PIDGone
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
