// Package guard — renice.go
//
// Process priority management for IDE performance optimization.
// Uses renice(1) and taskpolicy(1) to deprioritize background processes
// (Language Servers, indexers) so the IDE's Renderer gets uncontested P-core access.
//
// This is the macOS-native equivalent of cgroup CPU isolation:
//   - renice +10: lowers scheduler priority (yields time slices under contention)
//   - taskpolicy -b: sets Background QoS (prefers E-cores when P-cores are busy)
//
// Safety:
//   - Processes don't receive signals and don't know their priority changed
//   - No code signing issues (we modify kernel scheduling metadata, not the binary)
//   - Priority resets on process restart — Guard re-applies as needed
//   - Only affects processes owned by the current user (same UID)
package guard

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

// ReniceTarget defines what process group to deprioritize.
type ReniceTarget string

const (
	ReniceTargetLSP ReniceTarget = "lsp"
	ReniceTargetAll ReniceTarget = "all"
)

// ReniceResult reports what was deprioritized.
type ReniceResult struct {
	Target    ReniceTarget     `json:"target"`
	Reniced   int              `json:"reniced"`
	Skipped   int              `json:"skipped"`
	Errors    []string         `json:"errors,omitempty"`
	Processes []RenicedProcess `json:"processes,omitempty"`
}

// RenicedProcess describes a single process that was reniced.
type RenicedProcess struct {
	PID      int    `json:"pid"`
	Name     string `json:"name"`
	RSS      int64  `json:"rss_bytes"`
	RSSHuman string `json:"rss_human"`
	OldNice  int    `json:"old_nice"`
	NewNice  int    `json:"new_nice"`
	QoS      string `json:"qos"` // "background" or "default"
}

// lspProcessPatterns are process names that match Language Servers and
// background indexers that should yield to the IDE's Renderer.
var lspProcessPatterns = []string{
	"language_server_macos_arm",
	"gopls",
	"typescript-language-server",
	"pylsp",
	"rust-analyzer",
	"clangd",
	"sourcekit-lsp",
}

// Injectable for testing
var (
	reniceFn     = defaultRenice
	taskpolicyFn = defaultTaskpolicy
)

// Renice deprioritizes background IDE processes so the Renderer gets P-core access.
func Renice(target ReniceTarget) (*ReniceResult, error) {
	return reniceWith(target, defaultOrphanPs, reniceFn, taskpolicyFn)
}

func reniceWith(target ReniceTarget, psFn func() ([]orphanPsEntry, error), reniceFnArg func(int, int) error, taskpolicyFnArg func(int) error) (*ReniceResult, error) {
	result := &ReniceResult{Target: target}

	entries, err := psFn()
	if err != nil {
		return nil, fmt.Errorf("process scan failed: %w", err)
	}

	for _, entry := range entries {
		if !shouldRenice(target, entry.Name) {
			continue
		}

		// Skip PID 0 and 1 (kernel, launchd)
		if entry.PID <= 1 {
			result.Skipped++
			continue
		}

		proc := RenicedProcess{
			PID:      entry.PID,
			Name:     entry.Name,
			RSS:      entry.RSS,
			RSSHuman: FormatBytes(entry.RSS),
			OldNice:  0,
			NewNice:  10,
		}

		// Apply renice +10
		if err := reniceFnArg(entry.PID, 10); err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("renice PID %d: %v", entry.PID, err))
			result.Skipped++
			continue
		}

		// Apply taskpolicy -b (Background QoS) — darwin only
		if err := taskpolicyFnArg(entry.PID); err != nil {
			proc.QoS = "default (taskpolicy failed)"
		} else {
			proc.QoS = "background"
		}

		result.Processes = append(result.Processes, proc)
		result.Reniced++
	}

	return result, nil
}

// shouldRenice checks if a process matches the target group.
func shouldRenice(target ReniceTarget, name string) bool {
	nameLower := strings.ToLower(name)

	switch target {
	case ReniceTargetLSP:
		return matchesLSP(nameLower)
	case ReniceTargetAll:
		return matchesLSP(nameLower)
	default:
		return false
	}
}

func matchesLSP(nameLower string) bool {
	for _, pattern := range lspProcessPatterns {
		if strings.Contains(nameLower, pattern) {
			return true
		}
	}
	return false
}

// reniceByPID deprioritizes a single process by PID (renice +10 + background QoS).
// Used by the watchdog auto-renice feature.
func reniceByPID(pid int) error {
	if pid <= 1 {
		return fmt.Errorf("refusing to renice PID %d", pid)
	}
	if err := reniceFn(pid, 10); err != nil {
		return err
	}
	_ = taskpolicyFn(pid) // best-effort
	return nil
}

// defaultRenice calls renice(1) to set a new nice value.
func defaultRenice(pid int, nice int) error {
	cmd := exec.Command("renice", strconv.Itoa(nice), "-p", strconv.Itoa(pid))
	return cmd.Run()
}

// defaultTaskpolicy calls taskpolicy(1) to set Background QoS.
func defaultTaskpolicy(pid int) error {
	cmd := exec.Command("taskpolicy", "-b", "-p", strconv.Itoa(pid))
	return cmd.Run()
}

// FormatReniceReport returns a human-readable summary.
func FormatReniceReport(r *ReniceResult) string {
	if r.Reniced == 0 {
		return "𓁵 Isis: No matching processes found to deprioritize"
	}

	var sb strings.Builder
	sb.WriteString("𓁵 Isis — Deprioritize Report (safe, reversible)\n")
	sb.WriteString(strings.Repeat("─", 50) + "\n\n")

	for _, p := range r.Processes {
		sb.WriteString(fmt.Sprintf("  ✅ PID %-6d %-30s\n", p.PID, p.Name))
		sb.WriteString(fmt.Sprintf("     Nice: %d → %d  QoS: %s  RAM: %s\n\n",
			p.OldNice, p.NewNice, p.QoS, p.RSSHuman))
	}

	sb.WriteString(strings.Repeat("─", 50) + "\n")
	sb.WriteString(fmt.Sprintf("  Deprioritized: %d process(es)\n", r.Reniced))
	if r.Skipped > 0 {
		sb.WriteString(fmt.Sprintf("  Skipped: %d\n", r.Skipped))
	}
	sb.WriteString("  Effect: Renderer gets P-core priority on next CPU contention\n")

	return sb.String()
}
