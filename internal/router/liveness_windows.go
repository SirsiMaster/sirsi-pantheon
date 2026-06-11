//go:build windows

package router

import "os"

// defaultPIDState reports OS-truth liveness on Windows. Windows has no zombie
// concept, so liveness collapses to exists/gone: os.FindProcess opens a handle
// to the PID and fails if no such process exists.
func defaultPIDState(pid int) PIDState {
	if _, err := os.FindProcess(pid); err != nil {
		return PIDGone
	}
	return PIDAlive
}

// defaultPIDStart returns "" on Windows: there is no cheap, dependency-free
// lstart equivalent, and Windows has neither the zombie nor the routine PID-
// reuse pressure this discriminator defends against. Empty makes PIDStateOf
// fall back to bare-PID semantics (the pre-Amendment behavior) — correct here.
func defaultPIDStart(pid int) string {
	return ""
}

func defaultPIDCommand(pid int) string {
	return ""
}
