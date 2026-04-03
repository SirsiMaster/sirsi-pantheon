//go:build windows

package ra

import (
	"fmt"
	"os/exec"
)

// isProcessAlive checks if a process exists via tasklist on Windows.
func isProcessAlive(pid int) bool {
	cmd := exec.Command("tasklist", "/FI", fmt.Sprintf("PID eq %d", pid), "/NH")
	out, err := cmd.Output()
	if err != nil {
		return false
	}
	return len(out) > 0 && !contains(string(out), "No tasks")
}

// killProcess terminates a process via taskkill on Windows.
func killProcess(pid int) error {
	return exec.Command("taskkill", "/PID", fmt.Sprintf("%d", pid), "/F").Run()
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
