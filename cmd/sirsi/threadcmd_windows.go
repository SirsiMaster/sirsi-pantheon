//go:build windows

package main

import (
	"errors"
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

// No ACL/handle custody implementation is qualified here. Never fall back to
// taskkill against a persisted PID; leave records and existing watchers alone.
func openRouterWatcherFiles(_ func() (string, error), _ bool) (routerWatcherFiles, error) {
	return nil, errors.New("cooperative watcher custody is not qualified on Windows; existing watcher retirement unverified; no signal sent")
}

// detachedSysProcAttr is a no-op on Windows: there is no setsid equivalent,
// and exec.Cmd children launched with default attrs already survive the parent
// in most desktop contexts. Returning nil leaves SysProcAttr unset, which the
// stdlib treats as "use default Windows process semantics".
func detachedSysProcAttr() *syscall.SysProcAttr {
	return nil
}
