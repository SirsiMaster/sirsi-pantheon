// Package main — pantheon-menubar
//
// stats.go — Live stats collection for the Pantheon menu bar.
//
// Collects system metrics from across the Pantheon ecosystem:
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
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// StatsSnapshot is a point-in-time collection of system metrics.
type StatsSnapshot struct {
	// RAM
	TotalRAM     int64   `json:"total_ram"`
	UsedRAM      int64   `json:"used_ram"`
	FreeRAM      int64   `json:"free_ram"`
	RAMPercent   float64 `json:"ram_percent"`
	RAMPressure  string  `json:"ram_pressure"` // "low", "medium", "high"
	RAMIcon      string  `json:"ram_icon"`

	// Git / Osiris
	UncommittedFiles  int    `json:"uncommitted_files"`
	TimeSinceCommit   string `json:"time_since_commit"`
	GitBranch         string `json:"git_branch"`
	OsirisRisk        string `json:"osiris_risk"`
	OsirisIcon        string `json:"osiris_icon"`

	// Accelerator
	PrimaryAccelerator string `json:"primary_accelerator"`
	AccelIcon          string `json:"accel_icon"`

	// Active Deities
	ActiveDeities []string `json:"active_deities"`
	DeityCount    int      `json:"deity_count"`

	// Disk
	DiskWasteEstimate string `json:"disk_waste_estimate"`

	// Meta
	Timestamp   time.Time `json:"timestamp"`
	CollectedIn string    `json:"collected_in"`
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
		Interval: 10 * time.Second,
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
	fmt.Sscanf(strings.TrimSpace(string(out)), "%d", &total)
	snap.TotalRAM = total

	// Get memory pressure
	out, err = exec.Command("memory_pressure").Output()
	if err == nil {
		pressure := strings.TrimSpace(string(out))
		switch {
		case strings.Contains(pressure, "System-wide memory free percentage:"):
			// Parse free percentage
			for _, line := range strings.Split(pressure, "\n") {
				if strings.Contains(line, "free percentage") {
					var pct int
					fmt.Sscanf(line, "System-wide memory free percentage: %d%%", &pct)
					snap.RAMPercent = float64(100 - pct)
					snap.FreeRAM = total * int64(pct) / 100
					snap.UsedRAM = total - snap.FreeRAM
				}
			}
		}
	}

	// Fallback to vm_stat if memory_pressure didn't work
	if snap.RAMPercent == 0 {
		collectRAMFromVMStat(snap)
	}

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
			fmt.Sscanf(line, "Mach Virtual Memory Statistics: (page size of %d bytes)", &pageSize)
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
	fmt.Sscanf(valStr, "%d", &v)
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
	"pantheon":        "🏛 Pantheon",
	"anubis":          "𓂀 Anubis",
	"pantheon-agent":  "🤖 Agent",
	"guard":           "🛡 Guard",
	"maat":            "🪶 Ma'at",
	"scarab":          "🪲 Scarab",
	"thoth":           "𓁟 Thoth",
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

	return items
}

// StatusLine returns the bottom status line for the menu.
func (s *StatsSnapshot) StatusLine() string {
	return fmt.Sprintf("Pantheon Active — collected in %s", s.CollectedIn)
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

// formatBytes returns a human-readable byte count.
func formatBytes(bytes int64) string {
	const (
		KB = 1024
		MB = 1024 * KB
		GB = 1024 * MB
	)
	switch {
	case bytes >= GB:
		return fmt.Sprintf("%.1f GB", float64(bytes)/float64(GB))
	case bytes >= MB:
		return fmt.Sprintf("%.1f MB", float64(bytes)/float64(MB))
	case bytes >= KB:
		return fmt.Sprintf("%.1f KB", float64(bytes)/float64(KB))
	default:
		return fmt.Sprintf("%d B", bytes)
	}
}
