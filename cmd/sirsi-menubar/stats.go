// Package main — sirsi-menubar
//
// stats.go — Live stats collection for the Sirsi menu bar.
//
// Collects system metrics from across the Sirsi ecosystem:
//   - RAM pressure (via guard.Audit)
//   - Git status (via osiris.Assess)
//   - Accelerator profile (via hapi.DetectAccelerators)
//   - Active deities (process detection)
//   - Disk waste (last scan result)
//
// Stats are refreshed on a configurable interval and formatted
// for menu bar display as single-line status items.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

// StatsSnapshot is a point-in-time collection of system metrics.
type StatsSnapshot struct {
	// RAM
	TotalRAM    int64   `json:"total_ram"`
	UsedRAM     int64   `json:"used_ram"`
	FreeRAM     int64   `json:"free_ram"`
	RAMPercent  float64 `json:"ram_percent"`
	RAMPressure string  `json:"ram_pressure"` // "low", "medium", "high"
	RAMIcon     string  `json:"ram_icon"`

	// Git / Osiris
	UncommittedFiles int    `json:"uncommitted_files"`
	TimeSinceCommit  string `json:"time_since_commit"`
	GitBranch        string `json:"git_branch"`
	OsirisRisk       string `json:"osiris_risk"`
	OsirisIcon       string `json:"osiris_icon"`

	// Accelerator
	PrimaryAccelerator string `json:"primary_accelerator"`
	AccelIcon          string `json:"accel_icon"`

	// Active Deities
	ActiveDeities []string `json:"active_deities"`
	DeityCount    int      `json:"deity_count"`

	// Ra Deployment
	RaDeployed bool            `json:"ra_deployed"`
	RaScopes   []RaScopeStatus `json:"ra_scopes"`
	RaIcon     string          `json:"ra_icon"`

	// Disk
	DiskWasteEstimate string `json:"disk_waste_estimate"`

	// Meta
	Timestamp   time.Time `json:"timestamp"`
	CollectedIn string    `json:"collected_in"`
}

// RaScopeStatus tracks one Ra-deployed agent window.
type RaScopeStatus struct {
	Name  string `json:"name"`
	State string `json:"state"` // "running", "completed", "failed", "idle"
	Icon  string `json:"icon"`
}

// StatsConfig configures what to collect and how often.
type StatsConfig struct {
	RepoDir  string
	Interval time.Duration
}

// DefaultStatsConfig returns sensible defaults.
func DefaultStatsConfig() StatsConfig {
	return StatsConfig{
		RepoDir:  ".",
		Interval: 60 * time.Second,
	}
}

// CollectStats gathers a fresh stats snapshot.
// This is designed to be fast (< 500ms) and safe to call frequently.
func CollectStats(cfg StatsConfig) *StatsSnapshot {
	start := time.Now()
	snap := &StatsSnapshot{
		Timestamp: time.Now(),
	}

	// RAM pressure (lightweight — sysctl + vm_stat)
	collectRAM(snap)

	// Git status (lightweight — git status --porcelain)
	collectGit(snap, cfg.RepoDir)

	// Accelerator (cached after first call)
	collectAccelerator(snap)

	// Active deities (process scan)
	collectDeities(snap)

	// Ra deployment status
	collectRa(snap)

	snap.CollectedIn = time.Since(start).Round(time.Millisecond).String()
	return snap
}

// ── RAM Collection ──────────────────────────────────────────────────────

func collectRAM(snap *StatsSnapshot) {
	// Get total RAM
	out, err := exec.Command("sysctl", "-n", "hw.memsize").Output()
	if err != nil {
		snap.RAMPressure = "unknown"
		snap.RAMIcon = "⚪"
		return
	}

	var total int64
	_, _ = fmt.Sscanf(strings.TrimSpace(string(out)), "%d", &total)
	snap.TotalRAM = total

	// Use vm_stat for memory info (lightweight — avoids expensive memory_pressure command)
	collectRAMFromVMStat(snap)

	// Set pressure level
	switch {
	case snap.RAMPercent > 85:
		snap.RAMPressure = "high"
		snap.RAMIcon = "🔴"
	case snap.RAMPercent > 65:
		snap.RAMPressure = "medium"
		snap.RAMIcon = "🟡"
	default:
		snap.RAMPressure = "low"
		snap.RAMIcon = "🟢"
	}
}

func collectRAMFromVMStat(snap *StatsSnapshot) {
	out, err := exec.Command("vm_stat").Output()
	if err != nil {
		return
	}

	var pageSize int64 = 16384
	var free, active, wired int64

	for _, line := range strings.Split(string(out), "\n") {
		switch {
		case strings.Contains(line, "page size of"):
			_, _ = fmt.Sscanf(line, "Mach Virtual Memory Statistics: (page size of %d bytes)", &pageSize)
		case strings.Contains(line, "Pages free"):
			free = parseVMStatLine(line) * pageSize
		case strings.Contains(line, "Pages active"):
			active = parseVMStatLine(line) * pageSize
		case strings.Contains(line, "Pages wired"):
			wired = parseVMStatLine(line) * pageSize
		}
	}

	snap.UsedRAM = active + wired
	snap.FreeRAM = free
	if snap.TotalRAM > 0 {
		snap.RAMPercent = float64(snap.UsedRAM) / float64(snap.TotalRAM) * 100
	}
}

func parseVMStatLine(line string) int64 {
	parts := strings.Split(line, ":")
	if len(parts) < 2 {
		return 0
	}
	valStr := strings.TrimSpace(parts[1])
	valStr = strings.TrimSuffix(valStr, ".")
	var v int64
	_, _ = fmt.Sscanf(valStr, "%d", &v)
	return v
}

// ── Git Collection ──────────────────────────────────────────────────────

func collectGit(snap *StatsSnapshot, repoDir string) {
	if repoDir == "" {
		repoDir = "."
	}

	// Branch
	branchOut, err := exec.Command("git", "-C", repoDir, "rev-parse", "--abbrev-ref", "HEAD").Output()
	if err != nil {
		snap.GitBranch = "—"
		return
	}
	snap.GitBranch = strings.TrimSpace(string(branchOut))

	// Uncommitted files count
	statusOut, err := exec.Command("git", "-C", repoDir, "status", "--porcelain").Output()
	if err != nil {
		return
	}
	status := strings.TrimSpace(string(statusOut))
	if status != "" {
		snap.UncommittedFiles = len(strings.Split(status, "\n"))
	}

	// Time since last commit
	timeOut, err := exec.Command("git", "-C", repoDir, "log", "-1", "--format=%aI").Output()
	if err != nil {
		return
	}
	if t, err := time.Parse(time.RFC3339, strings.TrimSpace(string(timeOut))); err == nil {
		dur := time.Since(t)
		snap.TimeSinceCommit = formatDuration(dur)
	}

	// Risk assessment
	switch {
	case snap.UncommittedFiles == 0:
		snap.OsirisRisk = "none"
		snap.OsirisIcon = "✅"
	case snap.UncommittedFiles <= 5:
		snap.OsirisRisk = "low"
		snap.OsirisIcon = "🟢"
	case snap.UncommittedFiles <= 15:
		snap.OsirisRisk = "moderate"
		snap.OsirisIcon = "🟡"
	case snap.UncommittedFiles <= 30:
		snap.OsirisRisk = "high"
		snap.OsirisIcon = "🟠"
	default:
		snap.OsirisRisk = "critical"
		snap.OsirisIcon = "🔴"
	}
}

// ── Accelerator Collection ──────────────────────────────────────────────

func collectAccelerator(snap *StatsSnapshot) {
	// Check for Apple Silicon
	out, err := exec.Command("sysctl", "-n", "machdep.cpu.brand_string").Output()
	if err != nil {
		snap.PrimaryAccelerator = "CPU"
		snap.AccelIcon = "💻"
		return
	}

	cpuBrand := strings.TrimSpace(string(out))
	switch {
	case strings.Contains(cpuBrand, "Apple"):
		snap.PrimaryAccelerator = "ANE + Metal"
		snap.AccelIcon = "⚡"
	case strings.Contains(cpuBrand, "Intel"):
		snap.PrimaryAccelerator = "CPU (Intel)"
		snap.AccelIcon = "💻"
	default:
		snap.PrimaryAccelerator = "CPU"
		snap.AccelIcon = "💻"
	}
}

// ── Deity Detection ─────────────────────────────────────────────────────

var knownDeities = map[string]string{
	"sirsi":       "☥ Sirsi",
	"anubis":      "𓁢 Anubis",
	"sirsi-agent": "🤖 Agent",
	"guard":       "🛡 Guard",
	"maat":        "🪶 Ma'at",
	"scarab":      "🪲 Scarab",
	"thoth":       "𓁟 Thoth",
}

func collectDeities(snap *StatsSnapshot) {
	out, err := exec.Command("ps", "-eo", "comm").Output()
	if err != nil {
		return
	}

	procs := strings.ToLower(string(out))
	for binary, label := range knownDeities {
		if strings.Contains(procs, binary) {
			snap.ActiveDeities = append(snap.ActiveDeities, label)
		}
	}
	snap.DeityCount = len(snap.ActiveDeities)
}

// ── Ra Deployment Collection ────────────────────────────────────────────

func collectRa(snap *StatsSnapshot) {
	home, err := os.UserHomeDir()
	if err != nil {
		return
	}
	raDir := filepath.Join(home, ".config", "ra")

	// Read deployment.json for scope names
	metaPath := filepath.Join(raDir, "deployment.json")
	metaData, err := os.ReadFile(metaPath)
	if err != nil {
		snap.RaIcon = "⚫"
		return
	}

	// Parse scope names from deployment.json
	var meta struct {
		Scopes []string `json:"scopes"`
	}
	if err := json.Unmarshal(metaData, &meta); err != nil || len(meta.Scopes) == 0 {
		snap.RaIcon = "⚫"
		return
	}

	snap.RaDeployed = true
	allDone := true
	anyRunning := false
	anyFailed := false

	for _, scope := range meta.Scopes {
		ss := RaScopeStatus{Name: scope}

		// Check if PID is alive
		pidFile := filepath.Join(raDir, "pids", scope+".pid")
		pidData, err := os.ReadFile(pidFile)
		if err != nil {
			ss.State = "idle"
			ss.Icon = "⚫"
			snap.RaScopes = append(snap.RaScopes, ss)
			continue
		}

		var pid int
		_, _ = fmt.Sscanf(strings.TrimSpace(string(pidData)), "%d", &pid)

		// Check if process is alive (signal 0)
		if pid > 0 && isAlive(pid) {
			ss.State = "running"
			ss.Icon = "🔄"
			anyRunning = true
			allDone = false
		} else {
			// Check exit code
			exitFile := filepath.Join(raDir, "exits", scope+".exit")
			exitData, err := os.ReadFile(exitFile)
			if err != nil {
				ss.State = "crashed"
				ss.Icon = "💀"
				anyFailed = true
			} else {
				var code int
				_, _ = fmt.Sscanf(strings.TrimSpace(string(exitData)), "%d", &code)
				if code == 0 {
					ss.State = "completed"
					ss.Icon = "✅"
				} else {
					ss.State = "failed"
					ss.Icon = "❌"
					anyFailed = true
				}
			}
		}
		snap.RaScopes = append(snap.RaScopes, ss)
	}

	switch {
	case anyRunning:
		snap.RaIcon = "𓇶"
	case anyFailed:
		snap.RaIcon = "⚠️"
	case allDone:
		snap.RaIcon = "✅"
	default:
		snap.RaIcon = "⚫"
	}
}

// isAlive checks if a process exists via signal 0.
func isAlive(pid int) bool {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	err = proc.Signal(syscall.Signal(0))
	return err == nil
}

// ── Formatters ──────────────────────────────────────────────────────────

// FormatMenuItems returns the stats as menu item strings.
func (s *StatsSnapshot) FormatMenuItems() []string {
	items := []string{
		fmt.Sprintf("%s RAM: %.0f%% (%s)", s.RAMIcon, s.RAMPercent, s.RAMPressure),
		fmt.Sprintf("%s Files: %d uncommitted", s.OsirisIcon, s.UncommittedFiles),
	}

	if s.TimeSinceCommit != "" {
		items = append(items, fmt.Sprintf("⏱ Last commit: %s ago", s.TimeSinceCommit))
	}

	items = append(items, fmt.Sprintf("🌿 Branch: %s", s.GitBranch))

	if s.DeityCount > 0 {
		items = append(items, fmt.Sprintf("🏛 Active: %s", strings.Join(s.ActiveDeities, ", ")))
	} else {
		items = append(items, "🏛 No deities running")
	}

	items = append(items, fmt.Sprintf("%s Accelerator: %s", s.AccelIcon, s.PrimaryAccelerator))

	// Ra deployment status
	if s.RaDeployed && len(s.RaScopes) > 0 {
		items = append(items, "─── Ra Deployment ───")
		for _, scope := range s.RaScopes {
			items = append(items, fmt.Sprintf("  %s %s — %s", scope.Icon, scope.Name, scope.State))
		}
	} else {
		items = append(items, "𓇶 Ra: idle")
	}

	return items
}

// StatusLine returns the bottom status line for the menu.
func (s *StatsSnapshot) StatusLine() string {
	return fmt.Sprintf("Sirsi Active — collected in %s", s.CollectedIn)
}

// formatDuration returns a human-friendly duration.
func formatDuration(d time.Duration) string {
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		h := int(d.Hours())
		m := int(d.Minutes()) % 60
		if m > 0 {
			return fmt.Sprintf("%dh%dm", h, m)
		}
		return fmt.Sprintf("%dh", h)
	default:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	}
}
