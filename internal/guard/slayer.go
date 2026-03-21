package guard

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// SlayTarget defines what group of processes to terminate.
type SlayTarget string

const (
	SlayNode     SlayTarget = "node"
	SlayLSP      SlayTarget = "lsp"
	SlayDocker   SlayTarget = "docker"
	SlayElectron SlayTarget = "electron"
	SlayBuild    SlayTarget = "build"
	SlayAI       SlayTarget = "ai"
	SlayAll      SlayTarget = "all"
)

// ValidSlayTargets returns all valid slay targets.
func ValidSlayTargets() []SlayTarget {
	return []SlayTarget{SlayNode, SlayLSP, SlayDocker, SlayElectron, SlayBuild, SlayAI, SlayAll}
}

// IsValidTarget checks if a target string is valid.
func IsValidTarget(target string) bool {
	for _, t := range ValidSlayTargets() {
		if string(t) == target {
			return true
		}
	}
	return false
}

// SlayResult contains the results of a slay operation.
type SlayResult struct {
	Target     SlayTarget
	Killed     int
	Failed     int
	Skipped    int
	BytesFreed int64
	Errors     []error
	DryRun     bool
}

// Slay terminates processes matching the target group.
// Rule A1: NEVER kills without confirmation (dryRun must be explicitly false).
// Safety: SIGTERM first, SIGKILL after 5s timeout. Never kills root/system processes.
func Slay(target SlayTarget, dryRun bool) (*SlayResult, error) {
	result := &SlayResult{
		Target: target,
		DryRun: dryRun,
	}

	// Get current process list
	processes, err := getProcessList()
	if err != nil {
		return nil, fmt.Errorf("guard slay: %w", err)
	}

	// Classify and filter
	var targets []ProcessInfo
	for i := range processes {
		p := &processes[i]
		group := classifyProcess(p)
		p.Group = group

		if target == SlayAll {
			// "all" only targets known orphan groups, not "other"
			if group != "other" && group != "app_helper" {
				targets = append(targets, *p)
			}
		} else if group == string(target) {
			targets = append(targets, *p)
		}
	}

	// Safety: filter out protected processes
	var safeTargets []ProcessInfo
	for _, p := range targets {
		if isProtectedProcess(p) {
			result.Skipped++
			continue
		}
		safeTargets = append(safeTargets, p)
	}

	// Execute kills
	for _, p := range safeTargets {
		if dryRun {
			result.Killed++
			result.BytesFreed += p.RSS
			continue
		}

		err := killProcess(p.PID)
		if err != nil {
			result.Failed++
			result.Errors = append(result.Errors, fmt.Errorf("PID %d (%s): %w", p.PID, p.Name, err))
		} else {
			result.Killed++
			result.BytesFreed += p.RSS
		}
	}

	return result, nil
}

// isProtectedProcess returns true for processes that should never be killed.
func isProtectedProcess(p ProcessInfo) bool {
	// Never kill root or system processes
	if p.User == "root" || p.User == "_windowserver" || p.User == "_coreaudiod" {
		return true
	}

	// Never kill ourselves
	if p.PID == os.Getpid() {
		return true
	}

	// Never kill PID 1 (init/launchd)
	if p.PID <= 1 {
		return true
	}

	// Protected process names
	protectedNames := []string{
		"kernel_task", "launchd", "WindowServer", "loginwindow",
		"coreaudiod", "dock", "finder", "systemuiserver",
		"cfprefsd", "distnoted", "syslogd", "notifyd",
		"securityd", "trustd", "tccd", "locationd",
	}
	nameLower := strings.ToLower(p.Name)
	for _, protected := range protectedNames {
		if strings.ToLower(protected) == nameLower {
			return true
		}
	}

	return false
}

// killProcess sends SIGTERM, waits 5s, then SIGKILL if still running.
func killProcess(pid int) error {
	// Send SIGTERM (graceful)
	proc, err := os.FindProcess(pid)
	if err != nil {
		return fmt.Errorf("find process: %w", err)
	}

	if err := proc.Signal(syscall.SIGTERM); err != nil {
		return fmt.Errorf("SIGTERM: %w", err)
	}

	// Wait up to 5 seconds for graceful shutdown
	for i := 0; i < 10; i++ {
		time.Sleep(500 * time.Millisecond)
		if !isProcessRunning(pid) {
			return nil
		}
	}

	// Still running — force kill
	if err := proc.Signal(syscall.SIGKILL); err != nil {
		// Process may have died between check and kill
		if !isProcessRunning(pid) {
			return nil
		}
		return fmt.Errorf("SIGKILL: %w", err)
	}

	return nil
}

// isProcessRunning checks if a process is still alive.
func isProcessRunning(pid int) bool {
	out, err := exec.Command("kill", "-0", strconv.Itoa(pid)).CombinedOutput()
	_ = out
	return err == nil
}
