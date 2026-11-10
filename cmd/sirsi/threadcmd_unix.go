//go:build !windows

package main

import "syscall"

// processAlive reports whether a process with the given PID is alive
// (responds to signal 0). Used only for parent liveness, never as authority
// to terminate a process named by a persisted watcher record.
func processAlive(pid int) bool {
	return pid > 0 && syscall.Kill(pid, 0) == nil
}

// detachedSysProcAttr returns the SysProcAttr that detaches a child process
// from this controlling terminal/session (Setsid on Unix) so it survives
// our exit. Returned value is passed to (*exec.Cmd).SysProcAttr.
func detachedSysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{Setsid: true}
}
