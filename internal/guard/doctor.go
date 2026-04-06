// Package guard — doctor.go
//
// 𓁐 Isis Doctor: One-shot system health diagnostic.
//
// Runs a comprehensive health check covering RAM pressure, swap usage,
// disk space, runaway processes, orphan detection, and crash log analysis.
// Designed to be safe, read-only, and fast (< 2 seconds).
package guard

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/platform"
)

// DiagnosticSeverity indicates the urgency of a finding.
type DiagnosticSeverity int

const (
	SeverityOK DiagnosticSeverity = iota
	SeverityInfo
	SeverityWarn
	SeverityCritical
)

func (s DiagnosticSeverity) Icon() string {
	switch s {
	case SeverityOK:
		return "🟢"
	case SeverityInfo:
		return "🔵"
	case SeverityWarn:
		return "🟡"
	case SeverityCritical:
		return "🔴"
	default:
		return "⚪"
	}
}

func (s DiagnosticSeverity) String() string {
	switch s {
	case SeverityOK:
		return "OK"
	case SeverityInfo:
		return "INFO"
	case SeverityWarn:
		return "WARN"
	case SeverityCritical:
		return "CRITICAL"
	default:
		return "UNKNOWN"
	}
}

// DiagnosticFinding is a single health check result.
type DiagnosticFinding struct {
	Check    string             `json:"check"`
	Severity DiagnosticSeverity `json:"severity"`
	Message  string             `json:"message"`
	Detail   string             `json:"detail,omitempty"`
}

// DoctorReport is the complete health diagnostic.
type DoctorReport struct {
	Timestamp time.Time           `json:"timestamp"`
	Duration  string              `json:"duration"`
	Findings  []DiagnosticFinding `json:"findings"`
	Score     int                 `json:"score"` // 0-100, higher is healthier
}

// Doctor runs a one-shot system health diagnostic.
func Doctor() (*DoctorReport, error) {
	return DoctorWith(platform.Current())
}

// DoctorWith runs the diagnostic using the provided platform (Rule A16).
func DoctorWith(p platform.Platform) (*DoctorReport, error) {
	start := time.Now()
	report := &DoctorReport{
		Timestamp: start,
	}

	// Run all checks
	checkRAMPressure(p, report)
	checkSwapUsage(p, report)
	checkDiskSpace(p, report)
	checkTopMemoryProcesses(p, report)
	checkRecentCrashLogs(report)
	checkPantheonProcesses(p, report)

	// Calculate health score
	report.Score = calculateScore(report.Findings)
	report.Duration = time.Since(start).Round(time.Millisecond).String()

	return report, nil
}

// checkRAMPressure checks current memory pressure via vm_stat.
func checkRAMPressure(p platform.Platform, report *DoctorReport) {
	if p.Name() != "darwin" && p.Name() != "mock" {
		return
	}

	// Total RAM
	out, err := p.Command("sysctl", "-n", "hw.memsize")
	if err != nil {
		report.Findings = append(report.Findings, DiagnosticFinding{
			Check:    "RAM Pressure",
			Severity: SeverityWarn,
			Message:  "Could not read total RAM",
		})
		return
	}
	totalRAM, _ := strconv.ParseInt(strings.TrimSpace(string(out)), 10, 64)

	// vm_stat for pressure
	out, err = p.Command("vm_stat")
	if err != nil {
		return
	}

	pageSize := int64(16384)
	var free, active, wired, compressed int64

	for _, line := range strings.Split(string(out), "\n") {
		switch {
		case strings.Contains(line, "page size of"):
			for _, part := range strings.Fields(line) {
				if v, e := strconv.ParseInt(part, 10, 64); e == nil && v > 0 {
					pageSize = v
				}
			}
		case strings.Contains(line, "Pages free"):
			free = parseVMStatValue(line) * pageSize
		case strings.Contains(line, "Pages active"):
			active = parseVMStatValue(line) * pageSize
		case strings.Contains(line, "Pages wired"):
			wired = parseVMStatValue(line) * pageSize
		case strings.Contains(line, "compressor"):
			compressed = parseVMStatValue(line) * pageSize
		}
	}

	usedRAM := active + wired
	usedPct := float64(usedRAM) / float64(totalRAM) * 100

	finding := DiagnosticFinding{
		Check: "RAM Pressure",
		Detail: fmt.Sprintf("Total: %s | Active: %s | Wired: %s | Free: %s | Compressed: %s",
			FormatBytes(totalRAM), FormatBytes(active), FormatBytes(wired),
			FormatBytes(free), FormatBytes(compressed)),
	}

	switch {
	case usedPct > 90:
		finding.Severity = SeverityCritical
		finding.Message = fmt.Sprintf("RAM critically high at %.0f%% — Jetsam kills likely", usedPct)
	case usedPct > 75:
		finding.Severity = SeverityWarn
		finding.Message = fmt.Sprintf("RAM elevated at %.0f%% — monitor for pressure", usedPct)
	default:
		finding.Severity = SeverityOK
		finding.Message = fmt.Sprintf("RAM healthy at %.0f%%", usedPct)
	}

	report.Findings = append(report.Findings, finding)
}

// checkSwapUsage checks if the system is swapping.
func checkSwapUsage(p platform.Platform, report *DoctorReport) {
	out, err := p.Command("sysctl", "-n", "vm.swapusage")
	if err != nil {
		return
	}

	line := strings.TrimSpace(string(out))
	finding := DiagnosticFinding{
		Check:  "Swap Usage",
		Detail: line,
	}

	// Parse "used = X.XXM" specifically from the swap usage line
	// Format: "total = 2048.00M  used = 150.00M  free = 1898.00M  (encrypted)"
	usedMB := 0.0
	if idx := strings.Index(line, "used = "); idx >= 0 {
		rest := line[idx+len("used = "):]
		rest = strings.TrimSuffix(strings.Fields(rest)[0], "M")
		usedMB, _ = strconv.ParseFloat(rest, 64)
	}

	switch {
	case usedMB == 0:
		finding.Severity = SeverityOK
		finding.Message = "No swap in use"
	case usedMB > 1000:
		finding.Severity = SeverityCritical
		finding.Message = "Heavy swapping detected — system is thrashing"
	default:
		finding.Severity = SeverityWarn
		finding.Message = fmt.Sprintf("Swap active (%.0f MB used) — RAM pressure present", usedMB)
	}

	report.Findings = append(report.Findings, finding)
}

// checkDiskSpace checks available disk space on the boot volume.
func checkDiskSpace(p platform.Platform, report *DoctorReport) {
	out, err := p.Command("df", "-h", "/")
	if err != nil {
		return
	}

	lines := strings.Split(string(out), "\n")
	if len(lines) < 2 {
		return
	}

	fields := strings.Fields(lines[1])
	if len(fields) < 5 {
		return
	}

	available := fields[3]
	capacityStr := strings.TrimSuffix(fields[4], "%")
	capacity, _ := strconv.Atoi(capacityStr)

	finding := DiagnosticFinding{
		Check:  "Disk Space",
		Detail: fmt.Sprintf("Available: %s | Capacity: %s%%", available, capacityStr),
	}

	switch {
	case capacity > 95:
		finding.Severity = SeverityCritical
		finding.Message = fmt.Sprintf("Disk critically full at %d%% — %s remaining", capacity, available)
	case capacity > 85:
		finding.Severity = SeverityWarn
		finding.Message = fmt.Sprintf("Disk usage high at %d%% — %s remaining", capacity, available)
	default:
		finding.Severity = SeverityOK
		finding.Message = fmt.Sprintf("Disk healthy at %d%% — %s available", capacity, available)
	}

	report.Findings = append(report.Findings, finding)
}

// checkTopMemoryProcesses identifies the top RAM consumers.
func checkTopMemoryProcesses(p platform.Platform, report *DoctorReport) {
	processes, err := getProcessListWith(p)
	if err != nil {
		return
	}

	// Sort by RSS descending
	sort.Slice(processes, func(i, j int) bool {
		return processes[i].RSS > processes[j].RSS
	})

	// Report top 5 memory consumers
	var top []string
	for i, proc := range processes {
		if i >= 5 {
			break
		}
		top = append(top, fmt.Sprintf("%s (%s)", proc.Name, FormatBytes(proc.RSS)))
	}

	// Check for any single process using > 4GB
	var hogs []string
	for _, proc := range processes {
		if proc.RSS > 4*1024*1024*1024 {
			hogs = append(hogs, fmt.Sprintf("%s at %s", proc.Name, FormatBytes(proc.RSS)))
		}
	}

	finding := DiagnosticFinding{
		Check:  "Top Memory Consumers",
		Detail: strings.Join(top, " | "),
	}

	if len(hogs) > 0 {
		finding.Severity = SeverityWarn
		finding.Message = fmt.Sprintf("Memory hog detected: %s", strings.Join(hogs, ", "))
	} else {
		finding.Severity = SeverityOK
		finding.Message = "No individual process exceeding 4 GB"
	}

	report.Findings = append(report.Findings, finding)
}

// checkRecentCrashLogs looks for recent kernel panics and Jetsam events.
func checkRecentCrashLogs(report *DoctorReport) {
	diagDir := "/Library/Logs/DiagnosticReports/Retired"

	entries, err := os.ReadDir(diagDir)
	if err != nil {
		// Try non-retired
		entries, err = os.ReadDir("/Library/Logs/DiagnosticReports")
		if err != nil {
			return
		}
		diagDir = "/Library/Logs/DiagnosticReports"
	}

	var recentPanics int
	var recentJetsams int
	cutoff := time.Now().AddDate(0, 0, -7)

	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		if info.ModTime().Before(cutoff) {
			continue
		}

		name := e.Name()
		if strings.Contains(name, "panic") {
			recentPanics++
		}
		if strings.Contains(name, "JetsamEvent") {
			recentJetsams++
		}
	}

	// Also check the Retired subdirectory if we started from the parent
	if diagDir == "/Library/Logs/DiagnosticReports" {
		retiredEntries, err := os.ReadDir(filepath.Join(diagDir, "Retired"))
		if err == nil {
			for _, e := range retiredEntries {
				info, err := e.Info()
				if err != nil || info.ModTime().Before(cutoff) {
					continue
				}
				name := e.Name()
				if strings.Contains(name, "panic") {
					recentPanics++
				}
				if strings.Contains(name, "JetsamEvent") {
					recentJetsams++
				}
			}
		}
	}

	// Panic finding
	panicFinding := DiagnosticFinding{
		Check: "Kernel Panics (7d)",
	}
	switch {
	case recentPanics > 2:
		panicFinding.Severity = SeverityCritical
		panicFinding.Message = fmt.Sprintf("%d kernel panics in the last 7 days — hardware or driver issue", recentPanics)
	case recentPanics > 0:
		panicFinding.Severity = SeverityWarn
		panicFinding.Message = fmt.Sprintf("%d kernel panic(s) in the last 7 days", recentPanics)
	default:
		panicFinding.Severity = SeverityOK
		panicFinding.Message = "No kernel panics in the last 7 days"
	}
	report.Findings = append(report.Findings, panicFinding)

	// Jetsam finding
	jetsamFinding := DiagnosticFinding{
		Check: "Jetsam Events (7d)",
	}
	switch {
	case recentJetsams > 5:
		jetsamFinding.Severity = SeverityCritical
		jetsamFinding.Message = fmt.Sprintf("%d Jetsam memory kills in 7 days — system under severe RAM pressure", recentJetsams)
	case recentJetsams > 0:
		jetsamFinding.Severity = SeverityWarn
		jetsamFinding.Message = fmt.Sprintf("%d Jetsam event(s) in the last 7 days — RAM pressure present", recentJetsams)
	default:
		jetsamFinding.Severity = SeverityOK
		jetsamFinding.Message = "No Jetsam memory kills in the last 7 days"
	}
	report.Findings = append(report.Findings, jetsamFinding)
}

// checkPantheonProcesses checks for running Pantheon daemons and their health.
func checkPantheonProcesses(p platform.Platform, report *DoctorReport) {
	out, err := p.Command("ps", "-axo", "pid,rss,comm")
	if err != nil {
		return
	}

	pantheonProcs := map[string]struct {
		pid int
		rss int64
	}{}

	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		lower := strings.ToLower(line)
		if strings.Contains(lower, "pantheon") {
			fields := strings.Fields(line)
			if len(fields) >= 3 {
				pid, _ := strconv.Atoi(fields[0])
				rss, _ := strconv.ParseInt(fields[1], 10, 64)
				name := filepath.Base(strings.Join(fields[2:], " "))
				pantheonProcs[name] = struct {
					pid int
					rss int64
				}{pid: pid, rss: rss * 1024}
			}
		}
	}

	finding := DiagnosticFinding{
		Check: "Pantheon Processes",
	}

	if len(pantheonProcs) == 0 {
		finding.Severity = SeverityInfo
		finding.Message = "No Pantheon background processes running"
	} else {
		var details []string
		var totalRSS int64
		for name, info := range pantheonProcs {
			details = append(details, fmt.Sprintf("%s (PID %d, %s)", name, info.pid, FormatBytes(info.rss)))
			totalRSS += info.rss
		}
		finding.Detail = strings.Join(details, " | ")

		if totalRSS > 500*1024*1024 {
			finding.Severity = SeverityWarn
			finding.Message = fmt.Sprintf("%d Pantheon process(es) using %s total", len(pantheonProcs), FormatBytes(totalRSS))
		} else {
			finding.Severity = SeverityOK
			finding.Message = fmt.Sprintf("%d Pantheon process(es) healthy (%s total)", len(pantheonProcs), FormatBytes(totalRSS))
		}
	}

	report.Findings = append(report.Findings, finding)
}

// calculateScore derives a 0-100 health score from findings.
func calculateScore(findings []DiagnosticFinding) int {
	score := 100
	for _, f := range findings {
		switch f.Severity {
		case SeverityCritical:
			score -= 20
		case SeverityWarn:
			score -= 10
		case SeverityInfo:
			score -= 2
		}
	}
	if score < 0 {
		score = 0
	}
	return score
}
