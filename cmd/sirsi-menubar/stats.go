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

	"github.com/SirsiMaster/sirsi-pantheon/internal/deity"
	"github.com/SirsiMaster/sirsi-pantheon/internal/vitals"
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

	// Shared vitals (RAM, Git, Accelerator) — single source of truth
	v := vitals.Collect()
	snap.TotalRAM = v.RAMTotalBytes
	snap.UsedRAM = v.RAMUsedBytes
	snap.FreeRAM = v.RAMFreeBytes
	snap.RAMPercent = v.RAMPercent
	snap.RAMPressure = v.RAMPressure
	snap.RAMIcon = v.RAMIcon
	snap.GitBranch = v.GitBranch
	snap.UncommittedFiles = v.Uncommitted
	snap.TimeSinceCommit = v.LastCommit
	snap.PrimaryAccelerator = v.Accelerator
	if strings.Contains(v.Accelerator, "Apple") || strings.Contains(v.Accelerator, "M") {
		snap.AccelIcon = "⚡"
	} else {
		snap.AccelIcon = "💻"
	}

	// Risk assessment from uncommitted count
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

	// Active deities (process scan)
	collectDeities(snap)

	// Ra deployment status
	collectRa(snap)

	snap.CollectedIn = time.Since(start).Round(time.Millisecond).String()
	return snap
}

// ── Deity Detection ─────────────────────────────────────────────────────

// knownDeities builds the process-name-to-label map from the shared deity registry.
// Also includes non-deity process names that appear in ps output.
var knownDeities = func() map[string]string {
	m := map[string]string{
		"sirsi":       "☥ Sirsi",
		"sirsi-agent": "🤖 Agent",
	}
	for _, d := range deity.Roster {
		m[d.Key] = d.Glyph + " " + d.Name
	}
	return m
}()

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

	// Enrich with TUI deity state (success/failed/hasData indicators)
	if state, err := deity.LoadState(); err == nil {
		for key, runState := range state.DeityState {
			d := deity.Lookup(key)
			var indicator string
			switch runState {
			case deity.StateSucceeded:
				indicator = "✓"
			case deity.StateFailed:
				indicator = "✗"
			case deity.StateHasData:
				indicator = "◆"
			default:
				continue
			}
			// Update matching active deity or add if not present
			found := false
			label := d.Glyph + " " + d.Name
			for i, a := range snap.ActiveDeities {
				if a == label {
					snap.ActiveDeities[i] = indicator + " " + label
					found = true
					break
				}
			}
			if !found && runState != deity.StateNeverRun {
				snap.ActiveDeities = append(snap.ActiveDeities, indicator+" "+label)
			}
		}
		snap.DeityCount = len(snap.ActiveDeities)
	}
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
	// Memory leads — plain English, no jargon (pressure/swap stay in the CLI).
	// This is the pre-eminent line of the menubar.
	memWord := "healthy"
	switch s.RAMPressure {
	case "medium":
		memWord = "getting full"
	case "high":
		memWord = "running low"
	}
	items := []string{
		fmt.Sprintf("%s Memory — %s (%.0f%% in use)", s.RAMIcon, memWord, s.RAMPercent),
		fmt.Sprintf("%s %d files with unsaved changes", s.OsirisIcon, s.UncommittedFiles),
	}

	if s.TimeSinceCommit != "" {
		items = append(items, fmt.Sprintf("⏱ Last saved: %s ago", s.TimeSinceCommit))
	}

	items = append(items, fmt.Sprintf("🌿 Working on: %s", s.GitBranch))

	if s.DeityCount > 0 {
		items = append(items, fmt.Sprintf("🏛 Running: %s", strings.Join(s.ActiveDeities, ", ")))
	} else {
		items = append(items, "🏛 Nothing running in the background")
	}

	items = append(items, fmt.Sprintf("%s Graphics: %s", s.AccelIcon, s.PrimaryAccelerator))

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
