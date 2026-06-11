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
// It asks `ps` for the process state code: a leading "Z" means defunct
// (zombie) — the process is dead but unreaped, and must NOT count as a live
// agent. Empty output means the PID is gone. If `ps` is unavailable we fall
// back to signal 0, which can only distinguish exists/gone (not defunct).
func defaultPIDState(pid int) PIDState {
	out, err := exec.Command("ps", "-o", "stat=", "-p", strconv.Itoa(pid)).Output()
	state := strings.TrimSpace(string(out))
	if err != nil || state == "" {
		// ps failed or reported nothing — confirm existence via signal 0.
		if syscall.Kill(pid, 0) == nil {
			return PIDAlive
		}
		return PIDGone
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
