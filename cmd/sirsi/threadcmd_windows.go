//go:build windows

package main

import (
	"fmt"
	"os/exec"
	"strings"
	"syscall"
)

// processAlive reports whether a process with the given PID is alive on Windows
// (no kill -0 / signal 0 equivalent). Uses `tasklist /FI "PID eq <pid>"` and
// checks for a non-empty / non-"INFO: No tasks" response. Mirrors the existing
// pattern in internal/ra/process_windows.go.
func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	out, err := exec.Command("tasklist", "/FI", fmt.Sprintf("PID eq %d", pid), "/NH").Output()
	if err != nil {
		return false
	}
	s := string(out)
	return len(strings.TrimSpace(s)) > 0 && !strings.Contains(s, "No tasks")
}

// terminateProcess sends a graceful kill (taskkill /PID <pid> /F) on Windows.
// Mirrors the existing pattern in internal/ra/process_windows.go.
func terminateProcess(pid int) error {
	return exec.Command("taskkill", "/PID", fmt.Sprintf("%d", pid), "/F").Run()
}

// detachedSysProcAttr is a no-op on Windows: there is no setsid equivalent,
// and exec.Cmd children launched with default attrs already survive the parent
// in most desktop contexts. Returning nil leaves SysProcAttr unset, which the
// stdlib treats as "use default Windows process semantics".
func detachedSysProcAttr() *syscall.SysProcAttr {
	return nil
}
