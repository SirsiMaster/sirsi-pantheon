//go:build !windows

package ra

import "syscall"

// isProcessAlive checks if a process exists via signal 0.
func isProcessAlive(pid int) bool {
	return syscall.Kill(pid, 0) == nil
}

// killProcess sends SIGTERM to the given PID.
func killProcess(pid int) error {
	return syscall.Kill(pid, syscall.SIGTERM)
}
