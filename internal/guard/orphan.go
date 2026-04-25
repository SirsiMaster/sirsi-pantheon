// Package guard — orphan.go
//
// Orphan Process Detection: Finds zombie child processes from tools like
// Playwright, Puppeteer, Electron, and other automation frameworks.
//
// These are processes that were spawned by a parent (e.g., an IDE, CI runner,
// or test suite) but the parent died or disconnected, leaving the children
// running indefinitely. They consume RAM, hold file handles, and can block
// ports.
//
// This is distinct from Ka's ghost detection:
//   - Ka finds remnants of UNINSTALLED APPS (file-level ghosts)
//   - Isis finds orphaned RUNNING PROCESSES (process-level ghosts)
//
// The orphan detector runs as part of the watchdog cycle or on-demand
// via `sirsi guard --orphans`.
package guard

import (
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// OrphanPattern defines a known orphan-producing tool.
type OrphanPattern struct {
	Name         string   // Human-readable name
	ProcessNames []string // Process substrings to match
	ParentHints  []string // Expected parent processes
	Category     string   // "browser_automation", "ide", "build_tool", etc.
}

// KnownOrphanPatterns defines tools that commonly leak child processes.
var KnownOrphanPatterns = []OrphanPattern{
	{
		Name:         "Playwright",
		ProcessNames: []string{"ms-playwright", "playwright", "run-driver"},
		ParentHints:  []string{"node", "npm", "npx"},
		Category:     "browser_automation",
	},
	{
		Name:         "Puppeteer",
		ProcessNames: []string{"puppeteer", "chromium", "headless_shell"},
		ParentHints:  []string{"node", "npm"},
		Category:     "browser_automation",
	},
	{
		Name:         "Chrome DevTools",
		ProcessNames: []string{"chrome-devtools", "remote-debugging"},
		ParentHints:  []string{"Google Chrome"},
		Category:     "browser_automation",
	},
	{
		Name:         "Electron Helper",
		ProcessNames: []string{"Electron Helper", "Helper (GPU)", "Helper (Renderer)", "Helper (Plugin)"},
		ParentHints:  []string{"Electron"},
		Category:     "ide",
	},
	{
		Name:         "LSP Servers",
		ProcessNames: []string{"gopls", "typescript-language-server", "pylsp", "rust-analyzer", "clangd", "language_server_macos_arm", "sirsi"},
		ParentHints:  []string{"code", "cursor", "windsurf", "Antigravity", "Electron"},
		Category:     "ide",
	},
	{
		Name:         "Build Watchers",
		ProcessNames: []string{"fswatch", "watchman", "chokidar", "esbuild", "webpack"},
		ParentHints:  []string{"node", "npm"},
		Category:     "build_tool",
	},
	{
		Name:         "Stale Chrome Profiles",
		ProcessNames: []string{"antigravity-browser-profile", "puppeteer-profile", "test-profile"},
		ParentHints:  []string{"Google Chrome"},
		Category:     "browser_automation",
	},
}

// OrphanProcess is a detected orphan.
type OrphanProcess struct {
	ProcessInfo
	Pattern    string `json:"pattern"`     // Which OrphanPattern matched
	Category   string `json:"category"`    // browser_automation, ide, etc.
	ParentPID  int    `json:"parent_pid"`  // PPID — 1 means truly orphaned
	ParentName string `json:"parent_name"` // Parent process name
	IsOrphaned bool   `json:"is_orphaned"` // True if PPID=1 (adopted by launchd)
	IsStale    bool   `json:"is_stale"`    // True if running longer than stale threshold
	RunningFor string `json:"running_for"` // Human-readable age
}

// OrphanReport summarizes the orphan scan results.
type OrphanReport struct {
	TotalOrphans  int             `json:"total_orphans"`
	TotalRAM      int64           `json:"total_ram_bytes"`
	TotalRAMHuman string          `json:"total_ram_human"`
	ByCategory    map[string]int  `json:"by_category"`
	Orphans       []OrphanProcess `json:"orphans"`
	ScanDuration  string          `json:"scan_duration"`
}

// Injectable for testing
var orphanPsFn = defaultOrphanPs

// ScanOrphans finds orphaned processes matching known patterns.
// A process is considered orphaned if:
//  1. Its name matches a known orphan pattern, AND
//  2. Its PPID is 1 (adopted by launchd/init), OR
//  3. Its parent doesn't match an expected parent hint
func ScanOrphans() (*OrphanReport, error) {
	return scanOrphansWithFn(orphanPsFn)
}

func scanOrphansWithFn(psFn func() ([]orphanPsEntry, error)) (*OrphanReport, error) {
	start := time.Now()

	entries, err := psFn()
	if err != nil {
		return nil, err
	}

	// Build PID → name lookup for parent resolution
	pidNames := make(map[int]string)
	for _, e := range entries {
		pidNames[e.PID] = e.Name
	}

	report := &OrphanReport{
		ByCategory: make(map[string]int),
	}

	for _, entry := range entries {
		for _, pattern := range KnownOrphanPatterns {
			if matchesPattern(entry.Name, pattern.ProcessNames) {
				parentName := pidNames[entry.PPID]
				isOrphaned := entry.PPID <= 1
				isStale := !isOrphaned && !matchesPattern(parentName, pattern.ParentHints)

				if isOrphaned || isStale {
					orphan := OrphanProcess{
						ProcessInfo: ProcessInfo{
							PID:        entry.PID,
							Name:       entry.Name,
							RSS:        entry.RSS,
							CPUPercent: entry.CPU,
						},
						Pattern:    pattern.Name,
						Category:   pattern.Category,
						ParentPID:  entry.PPID,
						ParentName: parentName,
						IsOrphaned: isOrphaned,
						IsStale:    isStale,
						RunningFor: entry.ElapsedTime,
					}
					report.Orphans = append(report.Orphans, orphan)
					report.TotalOrphans++
					report.TotalRAM += entry.RSS
					report.ByCategory[pattern.Category]++
				}
				break // Only match first pattern
			}
		}
	}

	report.TotalRAMHuman = FormatBytes(report.TotalRAM)
	report.ScanDuration = time.Since(start).Truncate(time.Millisecond).String()
	return report, nil
}

// matchesPattern checks if a process name contains any of the pattern strings.
func matchesPattern(name string, patterns []string) bool {
	nameLower := strings.ToLower(name)
	for _, p := range patterns {
		if strings.Contains(nameLower, strings.ToLower(p)) {
			return true
		}
	}
	return false
}

// orphanPsEntry is a raw ps output row with PPID and elapsed time.
type orphanPsEntry struct {
	PID         int
	PPID        int
	RSS         int64
	CPU         float64
	Name        string
	ElapsedTime string
}

// defaultOrphanPs uses ps to get process info including PPID and elapsed time.
func defaultOrphanPs() ([]orphanPsEntry, error) {
	out, err := exec.Command("ps", "-axo", "pid,ppid,rss,%cpu,etime,comm").Output()
	if err != nil {
		return nil, err
	}

	var entries []orphanPsEntry
	lines := strings.Split(string(out), "\n")
	for i, line := range lines {
		if i == 0 {
			continue // header
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) < 6 {
			continue
		}

		pid, _ := strconv.Atoi(fields[0])
		ppid, _ := strconv.Atoi(fields[1])
		rss, _ := strconv.ParseInt(fields[2], 10, 64)
		cpu, _ := strconv.ParseFloat(fields[3], 64)
		etime := fields[4]
		name := strings.Join(fields[5:], " ")

		entries = append(entries, orphanPsEntry{
			PID:         pid,
			PPID:        ppid,
			RSS:         rss * 1024, // KB → bytes
			CPU:         cpu,
			Name:        name,
			ElapsedTime: etime,
		})
	}

	return entries, nil
}

// FormatOrphanReport returns a human-readable summary.
func FormatOrphanReport(r *OrphanReport) string {
	if r.TotalOrphans == 0 {
		return "𓁵 Isis: No orphaned processes detected ✨"
	}

	var sb strings.Builder
	sb.WriteString("𓁵 Isis Orphan Report\n")
	sb.WriteString(strings.Repeat("─", 50) + "\n")
	sb.WriteString("\n")

	for _, o := range r.Orphans {
		status := "🟡 stale parent"
		if o.IsOrphaned {
			status = "🔴 orphaned (PPID=1)"
		}
		sb.WriteString("  " + status + "\n")
		sb.WriteString("    PID: " + strconv.Itoa(o.PID) + " (" + o.Name + ")\n")
		sb.WriteString("    Pattern: " + o.Pattern + " [" + o.Category + "]\n")
		sb.WriteString("    RAM: " + FormatBytes(o.ProcessInfo.RSS) + "  CPU: " + strconv.FormatFloat(o.CPUPercent, 'f', 1, 64) + "%\n")
		sb.WriteString("    Running: " + o.RunningFor + "  Parent: " + o.ParentName + " (PID " + strconv.Itoa(o.ParentPID) + ")\n")
		sb.WriteString("\n")
	}

	sb.WriteString(strings.Repeat("─", 50) + "\n")
	sb.WriteString("  Total: " + strconv.Itoa(r.TotalOrphans) + " orphan(s) using " + r.TotalRAMHuman + "\n")

	if len(r.ByCategory) > 0 {
		sb.WriteString("  By type: ")
		parts := make([]string, 0, len(r.ByCategory))
		for cat, count := range r.ByCategory {
			parts = append(parts, cat+"="+strconv.Itoa(count))
		}
		sb.WriteString(strings.Join(parts, ", ") + "\n")
	}

	sb.WriteString("  Run `sirsi guard --slay` to clean up\n")
	return sb.String()
}
