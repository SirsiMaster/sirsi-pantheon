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
	// Trend marks event-count findings (Jetsam, panics) that recur across
	// multiple days — a sustained problem, not a one-off spike. Consumers
	// (SessionStart line, fail-loud hook) escalate only on trends to avoid
	// alert fatigue. ActiveDays is the number of distinct days in the window
	// that saw at least one event.
	Trend      bool `json:"trend,omitempty"`
	ActiveDays int  `json:"activeDays,omitempty"`
}

// DoctorReport is the complete health diagnostic.
type DoctorReport struct {
	Timestamp time.Time           `json:"timestamp"`
	Duration  string              `json:"duration"`
	Findings  []DiagnosticFinding `json:"findings"`
	Score     int                 `json:"score"` // 0-100, higher is healthier
}

// DoctorOpts configures the health diagnostic.
type DoctorOpts struct {
	// OnCheck is called after each health check completes.
	OnCheck func(checkName string, severity DiagnosticSeverity, message string, done, total int)
}

// Doctor runs a one-shot system health diagnostic.
func Doctor() (*DoctorReport, error) {
	return DoctorWithOpts(platform.Current(), DoctorOpts{})
}

// DoctorWith runs the diagnostic using the provided platform (Rule A16).
func DoctorWith(p platform.Platform) (*DoctorReport, error) {
	return DoctorWithOpts(p, DoctorOpts{})
}

// DoctorWithOpts runs the diagnostic with progress reporting.
func DoctorWithOpts(p platform.Platform, opts DoctorOpts) (*DoctorReport, error) {
	start := time.Now()
	report := &DoctorReport{
		Timestamp: start,
	}

	type checkFunc struct {
		name string
		fn   func()
	}
	checks := []checkFunc{
		{"RAM Pressure", func() { checkRAMPressure(p, report) }},
		{"Swap Usage", func() { checkSwapUsage(p, report) }},
		{"Disk Space", func() { checkDiskSpace(p, report) }},
		{"Memory Processes", func() { checkTopMemoryProcesses(p, report) }},
		{"Spotlight Storm", func() { checkSpotlightStorm(p, report) }},
		{"Crash Logs", func() { checkRecentCrashLogs(report) }},
		{"App Crashes", func() { checkAppCrashes(report) }},
		{"Sirsi Processes", func() { checkSirsiProcesses(p, report) }},
	}

	for i, c := range checks {
		prevCount := len(report.Findings)
		c.fn()
		if opts.OnCheck != nil {
			sev := SeverityOK
			msg := "healthy"
			if len(report.Findings) > prevCount {
				last := report.Findings[len(report.Findings)-1]
				sev = last.Severity
				msg = last.Message
			}
			opts.OnCheck(c.name, sev, msg, i+1, len(checks))
		}
	}

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

// crashWindowDays is the look-back window for kernel-panic / Jetsam trends.
const crashWindowDays = 7

// spotlightStormCPUThreshold is the aggregate %CPU across Spotlight indexer
// processes at or above which we flag a write-amplification storm. A quiet
// Spotlight sits near 0%; a storm (reindexing a busy dev tree after an agent
// write-burst) pins mds_stores/mdworker for sustained stretches.
const spotlightStormCPUThreshold = 30.0

// isSpotlightIndexer reports whether a process name is part of the macOS
// Spotlight indexing pipeline (the mds family).
func isSpotlightIndexer(name string) bool {
	return strings.HasPrefix(name, "mds") || strings.HasPrefix(name, "mdworker")
}

// checkSpotlightStorm detects the mds_stores write-amplification storm — the
// read-only half of the Spotlight remediation (flagship Rail B). Agent
// file-write bursts in heavy dev trees trigger Spotlight reindexing, which
// pins mds_stores and feeds the RAM-pressure → Jetsam loop. This surfaces the
// storm from the process table (no Spotlight-internals probing); the opt-in
// `~/Development` Privacy exclusion that fixes it lands behind its own confirm.
func checkSpotlightStorm(p platform.Platform, report *DoctorReport) {
	procs, err := getProcessListWith(p)
	if err != nil {
		return
	}
	var totalCPU float64
	var names []string
	for _, proc := range procs {
		if isSpotlightIndexer(proc.Name) {
			totalCPU += proc.CPUPercent
			if proc.CPUPercent > 0 {
				names = append(names, fmt.Sprintf("%s %.0f%%", proc.Name, proc.CPUPercent))
			}
		}
	}

	finding := DiagnosticFinding{Check: "Spotlight Storm"}
	if totalCPU >= spotlightStormCPUThreshold {
		finding.Severity = SeverityWarn
		finding.Message = fmt.Sprintf("Spotlight indexer busy (%.0f%% CPU) — likely reindexing a heavy-write dir; this feeds the RAM-pressure → Jetsam loop. Exclude busy dev dirs from Spotlight Privacy.", totalCPU)
		finding.Detail = strings.Join(names, " | ")
	} else {
		finding.Severity = SeverityOK
		finding.Message = "Spotlight indexer idle — no write-amplification storm"
	}
	report.Findings = append(report.Findings, finding)
}

// trendDayThreshold is the number of distinct active days within the window
// at or above which an event count is treated as a sustained TREND rather
// than a one-off TRANSIENT spike. Kept at 3 so a single bad afternoon (all
// events clustered in 1-2 days) stays a Warn, while recurrence across the
// week escalates to Critical.
const trendDayThreshold = 3

// crashEventScanFn returns the modification times of kernel-panic and Jetsam
// reports in the system DiagnosticReports dirs. Injectable (Rule A16) so the
// trend classifier can be tested without real crash logs on the host.
var crashEventScanFn = defaultCrashEventScan

// defaultCrashEventScan reads the macOS DiagnosticReports directories and
// returns the event times of kernel panics and Jetsam memory kills.
func defaultCrashEventScan() (panics, jetsams []time.Time) {
	dirs := []string{
		"/Library/Logs/DiagnosticReports",
		"/Library/Logs/DiagnosticReports/Retired",
	}
	seen := map[string]bool{}
	for _, dir := range dirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			name := e.Name()
			if seen[name] {
				continue // same report can appear in both dirs
			}
			seen[name] = true
			info, err := e.Info()
			if err != nil {
				continue
			}
			switch {
			case strings.Contains(name, "panic"):
				panics = append(panics, info.ModTime())
			case strings.Contains(name, "JetsamEvent"):
				jetsams = append(jetsams, info.ModTime())
			}
		}
	}
	return panics, jetsams
}

// classifyEventTrend buckets event times by calendar day within the last
// crashWindowDays and reports the in-window count, the number of distinct
// active days, and whether that constitutes a sustained trend.
func classifyEventTrend(times []time.Time, now time.Time) (count, activeDays int, isTrend bool) {
	cutoff := now.AddDate(0, 0, -crashWindowDays)
	days := map[string]bool{}
	for _, t := range times {
		if t.Before(cutoff) || t.After(now) {
			continue
		}
		count++
		days[t.Format("2006-01-02")] = true
	}
	activeDays = len(days)
	return count, activeDays, activeDays >= trendDayThreshold
}

// checkRecentCrashLogs looks for recent kernel panics and Jetsam events,
// distinguishing a one-off TRANSIENT spike from a sustained TREND so the
// SessionStart line and fail-loud hook escalate only on the latter (Rail C —
// routed flagship sequencing, claude-home 20260609-033900).
func checkRecentCrashLogs(report *DoctorReport) {
	panics, jetsams := crashEventScanFn()
	now := time.Now()

	report.Findings = append(report.Findings,
		crashEventFinding("Kernel Panics (7d)", "kernel panic", "hardware or driver issue", panics, now),
		crashEventFinding("Jetsam Events (7d)", "Jetsam memory kill", "system under RAM pressure", jetsams, now),
	)
}

// crashEventFinding builds a trend-aware finding for a class of crash events.
// Transient (clustered) spikes stay Warn; sustained trends across
// trendDayThreshold+ days escalate to Critical.
func crashEventFinding(check, noun, trendCause string, times []time.Time, now time.Time) DiagnosticFinding {
	count, activeDays, isTrend := classifyEventTrend(times, now)
	f := DiagnosticFinding{Check: check, ActiveDays: activeDays, Trend: isTrend}
	switch {
	case count == 0:
		f.Severity = SeverityOK
		f.Message = fmt.Sprintf("No %ss in the last %d days", noun, crashWindowDays)
	case isTrend:
		f.Severity = SeverityCritical
		f.Message = fmt.Sprintf("%d %ss across %d of %d days — sustained trend, %s",
			count, noun, activeDays, crashWindowDays, trendCause)
	default:
		f.Severity = SeverityWarn
		f.Message = fmt.Sprintf("%d %s(s) in the last %d days, clustered in %d day(s) — transient spike, watch for a trend",
			count, noun, crashWindowDays, activeDays)
	}
	return f
}

// appCrash is a single userspace crash report (EXC_CRASH / SIGABRT etc.).
type appCrash struct {
	process string
	when    time.Time
}

// appCrashDirsFn returns the DiagnosticReports directories to scan for app
// crashes. Injectable (Rule A16) so tests can point at a temp dir.
var appCrashDirsFn = defaultAppCrashDirs

func defaultAppCrashDirs() []string {
	roots := []string{"/Library/Logs/DiagnosticReports"}
	if home, err := os.UserHomeDir(); err == nil {
		// User app crashes (e.g. Chrome SIGABRT) land here, NOT in the system dir.
		roots = append([]string{filepath.Join(home, "Library/Logs/DiagnosticReports")}, roots...)
	}
	// macOS moves older reports to a Retired/ subdir under load — scan both so
	// the 7-day window doesn't lose crashes that were just retired (parity with
	// the kernel-panic/Jetsam check).
	dirs := make([]string, 0, len(roots)*2)
	for _, r := range roots {
		dirs = append(dirs, r, filepath.Join(r, "Retired"))
	}
	return dirs
}

// parseCrashReportName extracts the process name and timestamp from a crash
// report filename of the form "Process Name-YYYY-MM-DD-HHMMSS.ips".
func parseCrashReportName(name string) (proc string, ts time.Time, ok bool) {
	base := strings.TrimSuffix(name, ".ips")
	// Crashpad writes multi-fragment reports for ONE crash event with a trailing
	// ".NNNN" suffix (e.g. "...-164451.0004"). Strip it so the timestamp parses
	// and so all fragments collapse to the same second (see scanAppCrashes dedup).
	if i := strings.LastIndex(base, "."); i >= 0 {
		if _, err := strconv.Atoi(base[i+1:]); err == nil {
			base = base[:i]
		}
	}
	parts := strings.Split(base, "-")
	if len(parts) < 5 {
		return "", time.Time{}, false
	}
	n := len(parts)
	dateStr := strings.Join(parts[n-4:], "-")
	t, err := time.ParseInLocation("2006-01-02-150405", dateStr, time.Local)
	if err != nil {
		return "", time.Time{}, false
	}
	return strings.Join(parts[:n-4], "-"), t, true
}

// scanAppCrashes collects userspace crash reports (.ips) from the given dirs,
// excluding kernel panics and Jetsam events (counted separately), newer than
// cutoff. Read-only; tolerant of missing/unreadable dirs.
func scanAppCrashes(dirs []string, cutoff time.Time) []appCrash {
	var crashes []appCrash
	// Collapse multi-fragment reports of one event: same process + same second.
	seen := map[string]bool{}
	for _, dir := range dirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			name := e.Name()
			if !strings.HasSuffix(name, ".ips") {
				continue
			}
			if strings.Contains(strings.ToLower(name), "panic") || strings.Contains(name, "JetsamEvent") {
				continue
			}
			info, err := e.Info()
			if err != nil || info.ModTime().Before(cutoff) {
				continue
			}
			proc, ts, ok := parseCrashReportName(name)
			if !ok {
				proc, ts = strings.TrimSuffix(name, ".ips"), info.ModTime()
			}
			key := proc + "|" + ts.Truncate(time.Second).String()
			if seen[key] {
				continue
			}
			seen[key] = true
			crashes = append(crashes, appCrash{process: proc, when: ts})
		}
	}
	return crashes
}

// detectCrashloop returns the worst offender if any single process crashed
// 3+ times within a 5-minute sliding window AND the cluster is still active
// (its latest abort is within activeRecency of now) — the signature of a LIVE
// abort loop worth acting on. Historical clusters (e.g. a lockup days ago) are
// excluded here; they still show up in the 7-day count.
func detectCrashloop(crashes []appCrash, now time.Time) (proc string, count int, looping bool) {
	const window = 5 * time.Minute
	const threshold = 3
	const activeRecency = 30 * time.Minute
	cutoff := now.Add(-activeRecency)
	byProc := map[string][]time.Time{}
	for _, c := range crashes {
		if c.when.Before(cutoff) {
			continue
		}
		byProc[c.process] = append(byProc[c.process], c.when)
	}
	for p, times := range byProc {
		sort.Slice(times, func(i, j int) bool { return times[i].Before(times[j]) })
		for i := range times {
			j := i
			for j < len(times) && times[j].Sub(times[i]) <= window {
				j++
			}
			if n := j - i; n >= threshold && n > count {
				proc, count, looping = p, n, true
			}
		}
	}
	return proc, count, looping
}

// checkAppCrashes surfaces recent userspace application crashes (the class
// that kernel-panic / Jetsam checks miss — e.g. a SIGABRT in an agent-spawned
// Chrome). Surface + crashloop guard only; never auto-kills (Rule A1/A5).
func checkAppCrashes(report *DoctorReport) {
	now := time.Now()
	crashes := scanAppCrashes(appCrashDirsFn(), now.AddDate(0, 0, -7))

	finding := DiagnosticFinding{Check: "App Crashes (7d)"}

	if loopProc, loopN, looping := detectCrashloop(crashes, now); looping {
		finding.Severity = SeverityCritical
		finding.Message = fmt.Sprintf("%q crashloop — %d aborts in 5 min (active)", loopProc, loopN)
		finding.Detail = fmt.Sprintf("ACTIVE: %q is aborting repeatedly right now — investigate/stop it. Then reclaim the report backlog: sirsi clean", loopProc)
		report.Findings = append(report.Findings, finding)
		return
	}

	// Rank processes by crash count for an actionable detail line.
	counts := map[string]int{}
	for _, c := range crashes {
		counts[c.process]++
	}
	var top string
	var topN int
	for p, n := range counts {
		if n > topN {
			top, topN = p, n
		}
	}

	// Resolution pointer — these are mostly retired logs; Clean reclaims them
	// (the crash_reports rule now reaches Retired/). The count is history, not a
	// live emergency unless detectCrashloop fired above.
	resolution := fmt.Sprintf("%d crash report(s) accumulated (top: %q ×%d). These are diagnostic logs, not live failures — reclaim them with: sirsi clean", len(crashes), top, topN)
	switch {
	case len(crashes) > 10:
		finding.Severity = SeverityWarn // history, not critical — was over-alarming
		finding.Message = fmt.Sprintf("%d crash reports (7d) — top: %q (%d) · clear with `sirsi clean`", len(crashes), top, topN)
		finding.Detail = resolution
	case len(crashes) > 0:
		finding.Severity = SeverityWarn
		finding.Message = fmt.Sprintf("%d crash report(s) (7d) — top: %q (%d) · clear with `sirsi clean`", len(crashes), top, topN)
		finding.Detail = resolution
	default:
		finding.Severity = SeverityOK
		finding.Message = "No app crashes in the last 7 days"
	}
	report.Findings = append(report.Findings, finding)
}

// checkSirsiProcesses checks for running Sirsi daemons and their health.
func checkSirsiProcesses(p platform.Platform, report *DoctorReport) {
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
		if strings.Contains(lower, "sirsi") {
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
		Check: "Sirsi Processes",
	}

	if len(pantheonProcs) == 0 {
		finding.Severity = SeverityInfo
		finding.Message = "No Sirsi background processes running"
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
			finding.Message = fmt.Sprintf("%d Sirsi process(es) using %s total", len(pantheonProcs), FormatBytes(totalRSS))
		} else {
			finding.Severity = SeverityOK
			finding.Message = fmt.Sprintf("%d Sirsi process(es) healthy (%s total)", len(pantheonProcs), FormatBytes(totalRSS))
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
