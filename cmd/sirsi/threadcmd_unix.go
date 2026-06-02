//go:build !windows

package main

import "syscall"

// processAlive reports whether a process with the given PID is alive
// (responds to signal 0). Used for fs-watcher pidfile dedup + parent-PID
// liveness ticking in spawnRouterWatcher / watchRouterCmd.
func processAlive(pid int) bool {
	return pid > 0 && syscall.Kill(pid, 0) == nil
}

// terminateProcess sends SIGTERM to the given PID (graceful stop).
// Used by killRouterWatcher to retire an existing fs-watcher.
func terminateProcess(pid int) error {
	return syscall.Kill(pid, syscall.SIGTERM)
}

// detachedSysProcAttr returns the SysProcAttr that detaches a child process
// from this controlling terminal/session (Setsid on Unix) so it survives
// our exit. Returned value is passed to (*exec.Cmd).SysProcAttr.
func detachedSysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{Setsid: true}
}
