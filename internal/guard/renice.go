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
	"sync"
)

// ReniceTarget defines what process group to deprioritize.
type ReniceTarget string

const (
	ReniceTargetLSP ReniceTarget = "lsp"
	ReniceTargetAll ReniceTarget = "all"
	// ReniceTargetAgents deprioritizes background AI-agent CLIs (claude, codex,
	// local mlx/gemma runners) so the foreground session keeps P-core access.
	// QoS only — core-pinning is not possible (ANE is CoreML-only); taskpolicy -b
	// prefers E-cores, which is the whole cross-agent QoS story (ADR-031-B).
	ReniceTargetAgents ReniceTarget = "agents"
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

// PreviewRenice reports what Renice(target) WOULD deprioritize without
// applying anything — the dry-run half of the A1 preview-then-confirm shape.
func PreviewRenice(target ReniceTarget) (*ReniceResult, error) {
	return reniceWith(target, defaultOrphanPs,
		func(int, int) error { return nil }, func(int) error { return nil })
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

		// A1: the protected list holds for EVERY target, not just per-PID
		// callers — a pattern that ever grows to overlap a protected name
		// (e.g. "sirsi-gemma-worker" under the agents target) must be
		// skipped here, not trusted to pattern hygiene.
		if isProtectedReniceTarget(entry.Name) {
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
	case ReniceTargetAgents:
		return matchesAgent(nameLower)
	case ReniceTargetAll:
		return matchesLSP(nameLower) || matchesAgent(nameLower)
	default:
		return false
	}
}

// agentProcessPatterns match background AI-agent CLIs and local model runners.
// "sirsi" processes (incl. sirsi-gemma-worker) are excluded by the protected
// list — Pantheon never deprioritizes itself.
var agentProcessPatterns = []string{
	"claude",
	"codex",
	"mlx_lm",
	"gemma",
}

func matchesAgent(nameLower string) bool {
	// The foreground GUI is NOT a background agent: "claude" must match the
	// claude-code runners, never /Applications/Claude.app (the owner's desktop
	// app and its Electron helpers) — deprioritizing that harms the exact
	// foreground this lever protects. The exclusion keys on GUI install
	// locations, not ".app" itself: claude-code's headless runner also lives
	// inside an .app-shaped directory (under Application Support) and MUST
	// still match.
	if strings.Contains(nameLower, "/applications/") || strings.Contains(nameLower, "/frameworks/") {
		return false
	}
	for _, pattern := range agentProcessPatterns {
		if strings.Contains(nameLower, pattern) {
			return true
		}
	}
	return false
}

func matchesLSP(nameLower string) bool {
	for _, pattern := range lspProcessPatterns {
		if strings.Contains(nameLower, pattern) {
			return true
		}
	}
	return false
}

// protectedReniceNames are process-name fragments that MUST NEVER be reniced.
// Deprioritizing any of these can freeze the UI, starve audio/input, or harm
// Sirsi itself — the opposite of relief. HARDCODED and not overridable by flag
// or config (A1), mirroring internal/cleaner safety.go's protected-path model.
// Matched case-insensitively as a substring of the process name.
var protectedReniceNames = []string{
	"windowserver",   // the compositor — renicing it IS the freeze
	"kernel_task",    // kernel
	"launchd",        // pid 1 / job manager
	"loginwindow",    // session
	"systemuiserver", // menu bar / system UI
	"windowmanager",  // Stage Manager / window management
	"coreaudiod",     // audio — never starve
	"sirsi",          // never renice ourselves (sirsi/-agent/-menubar/-gemma)
}

// isProtectedReniceTarget reports whether a process name must never be reniced.
func isProtectedReniceTarget(name string) bool {
	n := strings.ToLower(name)
	for _, p := range protectedReniceNames {
		if strings.Contains(n, p) {
			return true
		}
	}
	return false
}

// reniceByPID deprioritizes a single process by PID (renice +10 + background QoS).
// Used by the watchdog auto-renice feature. A1: refuses pid<=1 AND any protected
// process (compositor, kernel, audio, session UI, sirsi itself) so an auto or
// one-click renice can never starve a critical process and make a freeze worse.
func reniceByPID(pid int, name string) error {
	return reniceByPIDWith(pid, name, reniceFn, taskpolicyFn)
}

// reniceByPIDFn is the A21-safe seam the Isis watchdog's auto-renice goroutine
// calls. The Watch loop runs it from a `go func`, so a test that swaps it to
// capture calls MUST go through set/getReniceByPIDFn (under reniceByPIDMu) —
// raw global assignment would be a data race against the live goroutine.
var (
	reniceByPIDMu sync.RWMutex
	reniceByPIDFn = reniceByPID
)

func getReniceByPIDFn() func(int, string) error {
	reniceByPIDMu.RLock()
	defer reniceByPIDMu.RUnlock()
	return reniceByPIDFn
}

func setReniceByPIDFn(fn func(int, string) error) {
	reniceByPIDMu.Lock()
	defer reniceByPIDMu.Unlock()
	reniceByPIDFn = fn
}

// reniceByPIDWith is the injectable core (A16/A21) so the protected-target
// refusal is unit-tested without touching real processes.
func reniceByPIDWith(pid int, name string, reniceFnArg func(int, int) error, taskpolicyFnArg func(int) error) error {
	if pid <= 1 {
		return fmt.Errorf("refusing to renice PID %d", pid)
	}
	if isProtectedReniceTarget(name) {
		return fmt.Errorf("refusing to renice protected process %q (pid %d)", name, pid)
	}
	if err := reniceFnArg(pid, 10); err != nil {
		return err
	}
	_ = taskpolicyFnArg(pid) // best-effort
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
