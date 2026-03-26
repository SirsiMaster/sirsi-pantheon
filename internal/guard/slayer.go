package guard

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/SirsiMaster/sirsi-pantheon/internal/platform"
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

// Slay terminates processes matching the target group using the current platform.
func Slay(target SlayTarget, dryRun bool) (*SlayResult, error) {
	return SlayWith(platform.Current(), target, dryRun)
}

// SlayWith terminates processes matching the target group using the provided platform (Rule A16).
// Rule A1: NEVER kills without confirmation (dryRun must be explicitly false).
// Safety: SIGTERM first (via Platform.Kill), Never kills root/system processes.
func SlayWith(p platform.Platform, target SlayTarget, dryRun bool) (*SlayResult, error) {
	result := &SlayResult{
		Target: target,
		DryRun: dryRun,
	}

	// Get current process list
	processes, err := getProcessListWith(p)
	if err != nil {
		return nil, fmt.Errorf("guard slay: %w", err)
	}

	// Classify and filter
	var targets []ProcessInfo
	for i := range processes {
		proc := &processes[i]
		group := classifyProcess(proc)
		proc.Group = group

		if target == SlayAll {
			// "all" only targets known orphan groups, not "other"
			if group != "other" && group != "app_helper" {
				targets = append(targets, *proc)
			}
		} else if group == string(target) {
			targets = append(targets, *proc)
		}
	}

	// Safety: filter out protected processes
	var safeTargets []ProcessInfo
	for _, proc := range targets {
		if isProtectedProcessWith(p, proc) {
			result.Skipped++
			continue
		}
		safeTargets = append(safeTargets, proc)
	}

	// Execute kills
	for _, proc := range safeTargets {
		if dryRun {
			result.Killed++
			result.BytesFreed += proc.RSS
			continue
		}

		err := p.Kill(proc.PID)
		if err != nil {
			result.Failed++
			result.Errors = append(result.Errors, fmt.Errorf("PID %d (%s): %w", proc.PID, proc.Name, err))
		} else {
			result.Killed++
			result.BytesFreed += proc.RSS
		}
	}

	return result, nil
}

// isProtectedProcessWith returns true for processes that should never be killed.
func isProtectedProcessWith(p platform.Platform, proc ProcessInfo) bool {
	// Never kill root or system processes
	if proc.User == "root" || proc.User == "_windowserver" || proc.User == "_coreaudiod" {
		return true
	}

	// Never kill ourselves
	if proc.PID == os.Getpid() {
		return true
	}

	// Never kill PID 1 (init/launchd)
	if proc.PID <= 1 {
		return true
	}

	// Protected process names (exact match)
	protectedNames := []string{
		"kernel_task", "launchd", "WindowServer", "loginwindow",
		"coreaudiod", "dock", "finder", "systemuiserver",
		"cfprefsd", "distnoted", "syslogd", "notifyd",
		"securityd", "trustd", "tccd", "locationd",
	}
	nameLower := strings.ToLower(proc.Name)
	for _, protected := range protectedNames {
		if strings.ToLower(protected) == nameLower {
			return true
		}
	}

	// Antigravity core — killing this crashes the IDE.
	// Uses Contains because the full path is in proc.Name.
	if strings.Contains(nameLower, "language_server_macos_arm") {
		return true
	}

	return false
}

// isProcessRunning checks if a process is still alive.
func isProcessRunningWith(p platform.Platform, pid int) bool {
	out, err := p.Command("kill", "-0", strconv.Itoa(pid))
	_ = out
	return err == nil
}
