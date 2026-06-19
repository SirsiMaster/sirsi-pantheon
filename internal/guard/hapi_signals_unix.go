//go:build unix

// Package guard — hapi_signals_unix.go
//
// The Hapi memory-governor's process-liveness probe + intervention primitives,
// which are POSIX-signal based (kill(2), SIGSTOP/SIGCONT/SIGTERM) and therefore
// Unix-only. Split from hapi.go so the cross-platform release build (goreleaser
// targets darwin/linux/windows) compiles — Windows gets stubs in
// hapi_signals_windows.go. macOS (the primary surface, ADR-032) and Linux use
// these real implementations. hapiCanAct (the A1 gate) is shared in hapi.go.
package guard

import "syscall"

// hapiProcessAlive reports whether pid names a live process (signal 0 probe).
func hapiProcessAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	return syscall.Kill(pid, 0) == nil
}

// hapiSuspend sends SIGSTOP — pauses the process so its memory STOPS growing.
// Reversible (hapiResume / SIGCONT). The non-destructive memory-pressure relief.
func hapiSuspend(pid int, name string) error {
	if err := hapiCanAct(pid, name); err != nil {
		return err
	}
	return syscall.Kill(pid, syscall.SIGSTOP)
}

// hapiResume sends SIGCONT — un-pauses a previously suspended process.
func hapiResume(pid int) error {
	return syscall.Kill(pid, syscall.SIGCONT)
}

// hapiKill sends SIGTERM — the last resort, only for governed compute at emergency.
func hapiKill(pid int, name string) error {
	if err := hapiCanAct(pid, name); err != nil {
		return err
	}
	return syscall.Kill(pid, syscall.SIGTERM)
}
