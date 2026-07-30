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
	// Fix is the safe CLI command that resolves this finding (empty = informational,
	// no one-click fix). Carried on the finding so EVERY surface (menubar Horus,
	// SessionStart, dashboard) can offer resolution — not just the Insight panel.
	Fix string `json:"fix,omitempty"`
	// FixKind tells a surface how HONEST to be about the Fix button — so it never
	// promises an instant cure for a historical record (the "I clicked Fix and the
	// status stayed the same" trap). See the FixKind constants.
	FixKind FixKind `json:"fixKind,omitempty"`
}

// FixKind classifies what running a finding's Fix actually does, so surfaces can
// label the action truthfully instead of always saying "Fix it":
//
//	FixInstant   — the command provably changes this finding's state NOW (clears a
//	               report backlog, replaces a drifted binary, frees disk). Re-running
//	               diagnose after it WILL show the finding cleared or downgraded.
//	FixRelief    — the command relieves a LIVE cause (renice a hog, ease memory
//	               pressure). A finding tagged (7d) is a HISTORICAL count that cannot
//	               drop retroactively; relief stops it growing and it decays as clean
//	               days pass. Re-verify may show "still present (history)" — honestly.
//	FixGuidance  — the command only acts when the condition is live right now (a
//	               Spotlight storm in progress); otherwise it prints guidance and is
//	               a no-op. Surfaces must NOT label this "Fix it".
type FixKind string

const (
	FixInstant  FixKind = "instant"
	FixRelief   FixKind = "relief"
	FixGuidance FixKind = "guidance"
)

// remediationKind classifies a finding's Fix so surfaces label it honestly.
// Kept in lockstep with remediationCommand: same Check cases, same gate.
func remediationKind(f DiagnosticFinding) FixKind {
	switch f.Check {
	case "binary-drift":
		return FixInstant // self-update replaces the drifted binary
	case "App Crashes (7d)":
		return FixInstant // clearing the crash-report backlog drops the count
	case "Disk Space":
		return FixInstant // clean frees real bytes
	case "Local Snapshots":
		return FixInstant // thinning reclaims real disk immediately
	case "Spotlight Storm":
		return FixGuidance // acts only during a live storm; else prints guidance
	case "App Hangs (7d)":
		return FixRelief // a real user-app freeze → renice the live hog; trend decays
	case "RAM Pressure", "Top Memory Consumers", "Jetsam Events (7d)",
		"Thread Leaks", "Swap Usage", "Memory Death Spiral":
		return FixRelief // eases the live cause; trend counts decay, not drop
	case "Runaway Executor":
		return FixRelief // quarantine stops the spawner; artifacts drain via the hourly sweep
	}
	return ""
}

// remediationCommand returns the safe, already-gated CLI command that resolves a
// finding (or "" if it is informational). One mapping, read by every surface, so
// no finding dead-ends at "here's a problem" with no way to act.
func remediationCommand(f DiagnosticFinding) string {
	warn := f.Severity >= SeverityWarn
	switch f.Check {
	case "binary-drift":
		return "sirsi self-update" // heal the self-crasher
	case "App Crashes (7d)":
		if warn {
			// Crash reports are caution-tier — safe `clean` leaves them, so the
			// count never dropped (the "Fix did nothing" bug). --include-caution
			// clears the report backlog so the 7d count actually falls.
			return "sirsi clean --include-caution"
		}
	case "Spotlight Storm":
		if warn {
			return "sirsi spotlight-exclude ~/Development"
		}
	case "App Hangs (7d)":
		// Only real user-facing app freezes reach Warn+ now (background-daemon CPU
		// noise stays Info, no one-click). The remedy is to relieve the live hog —
		// renice the process currently saturating the CPU (A1-protected, reversible).
		if warn {
			return "sirsi relieve"
		}
	case "Duplicate Model Brokers":
		// Two brokers means two copies of a multi-GB model resident at once, and
		// the extra one serves nothing — nothing routes to it, so its pages get
		// swapped out and eat the swap file while it sits idle. `relieve --memory`
		// flushes caches and would not touch it; the only real remedy is to stop
		// the broker that is not the canonical one. `gemma serve --stop` targets
		// the pidfile's broker, so this points at the sweep that reaps orphans by
		// discovery instead. ADR-033: an alarming check ships a lever that acts on
		// the thing it alarmed about.
		if warn {
			return "sirsi gemma reap-orphans"
		}
	case "Process Footprint":
		// One process holding a third-to-half of RAM. `relieve --memory` flushes
		// inactive caches, which does NOT touch a single process's footprint —
		// the lever has to name the offender. Deliberately a PREVIEW verb: the
		// offender here is routinely Sirsi's own model broker, and killing the
		// local AI without asking is not a remedy an operator should discover
		// after the fact (A1: preview mutates nothing).
		return "sirsi relieve"
	case "RAM Pressure", "Top Memory Consumers", "Jetsam Events (7d)", "Memory Death Spiral":
		if warn {
			// Flush inactive caches — the safe, non-destructive memory lever
			// (renice frees CPU, not RAM). Was `sirsi guard`, which is just the
			// MONITOR — tapping "Relieve" opened a dashboard, not an action.
			// NOTE the check name: the memory-hog finding is emitted as
			// "Top Memory Consumers". This case once said "Memory Processes" —
			// the PROGRESS label, not the finding name — so the hog warning
			// dead-ended with no lever (the owner's menubar QA defect).
			return "sirsi relieve --memory"
		}
	case "Swap Usage":
		if f.Severity >= SeverityCritical {
			return "sirsi relieve --memory" // genuine pressure → flush caches
		}
	case "Thread Leaks":
		if f.Severity >= SeverityCritical {
			return "sirsi relieve" // renice the offender (a real action, not a monitor)
		}
	case "Disk Space":
		if warn {
			return "sirsi clean --include-caution"
		}
	case "Local Snapshots":
		return "sirsi reclaim-snapshots" // optional disk reclaim, offered even at Info
	case "Runaway Executor":
		if warn {
			// 𓁵 Sekhmet's kill switch (ADR-035): bootout + quarantine ONLY the
			// claude build-worker LaunchAgents; wake-loops/supervisor untouched.
			return "sirsi router quarantine-worker"
		}
	}
	return ""
}

// DoctorReport is the complete health diagnostic.
type DoctorReport struct {
	Timestamp time.Time           `json:"timestamp"`
	Duration  string              `json:"duration"`
	Findings  []DiagnosticFinding `json:"findings"`
	Score     int                 `json:"score"`  // 0-100, higher is healthier
	Status    HealthStatus        `json:"status"` // the at-a-glance green/amber/red light
}

// HealthStatus is the single at-a-glance roll-up every surface shows (SessionStart
// line, menubar glyph, Horus). It answers "do I need to act, and how urgently" —
// NOT "how many findings exist."
//
// THE RUBRIC (canonical — surfaces must read this, never re-derive from raw
// severities, which is what made the light scream red on a usable machine):
//
//	🔴 RED   — at least one LIVE, session-threatening critical: act now.
//	           (RAM critically high → Jetsam imminent; disk full; an ACTIVE crash
//	           loop). These are happening *right now*.
//	🟡 AMBER — no live-critical, but warnings and/or HISTORICAL trends present
//	           (7-day Jetsam/crash/hang/swap patterns). Worth a look; optional fix.
//	🟢 GREEN — everything OK/Info. Nothing needs you.
//
// The crucial distinction is LIVE vs TREND: a week-old crash pattern informs
// (amber); it does not mean the machine is on fire (red).
type HealthStatus string

const (
	HealthGreen HealthStatus = "green"
	HealthAmber HealthStatus = "amber"
	HealthRed   HealthStatus = "red"
)

func (h HealthStatus) Icon() string {
	switch h {
	case HealthRed:
		return "🔴"
	case HealthAmber:
		return "🟡"
	default:
		return "🟢"
	}
}

// Label is the human status word shown next to the score.
func (h HealthStatus) Label() string {
	switch h {
	case HealthRed:
		return "Critical — act now"
	case HealthAmber:
		return "Attention"
	default:
		return "Healthy"
	}
}

// liveCriticalChecks are the checks whose Critical severity reflects a CURRENT,
// session-threatening condition — the only ones that force RED. Swap and the
// 7-day trend checks are deliberately excluded: allocated swap and past patterns
// are amber, not act-now.
var liveCriticalChecks = map[string]bool{
	"RAM Pressure":        true, // used RAM critically high → Jetsam kills imminent
	"Disk Space":          true, // volume full → saves fail, system instability
	"Memory Death Spiral": true, // swap exhausted + load runaway → the machine is dying NOW (94/100-during-spiral bug, 2026-07-16)
	// A single process at half of RAM is act-now: the kernel Jetsams SOMETHING to
	// survive it, and it does not get to pick a convenient victim. This machine
	// OOM'd three times on 2026-07-27 while every check reported green, because
	// they all sampled RSS and the offender was 39.9 GB compressed.
	"Process Footprint": true,
}

// isLiveCritical reports whether a finding is an act-now RED rather than a
// historical trend that should only inform (AMBER).
func isLiveCritical(f DiagnosticFinding) bool {
	if f.Severity != SeverityCritical {
		return false
	}
	if liveCriticalChecks[f.Check] {
		return true
	}
	// An ACTIVE crash loop (a live abort cluster) is act-now, unlike a 7-day count.
	return strings.Contains(f.Message, "crashloop") || strings.Contains(f.Message, "abort loop")
}

// classifyHealth rolls findings up into the single green/amber/red light per the
// rubric on HealthStatus.
func classifyHealth(findings []DiagnosticFinding) HealthStatus {
	amber := false
	for _, f := range findings {
		if isLiveCritical(f) {
			return HealthRed
		}
		if f.Severity == SeverityWarn || f.Severity == SeverityCritical {
			amber = true
		}
	}
	if amber {
		return HealthAmber
	}
	return HealthGreen
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

// doctorCheck is one entry in the doctor's canonical check registry: the
// progress label surfaces show while it runs, the finding Check name(s) it can
// emit, and the runner itself.
type doctorCheck struct {
	name  string   // progress label (what's being checked)
	emits []string // every finding Check name this check can append
	run   func(p platform.Platform, report *DoctorReport)
}

// doctorChecks is THE canonical registry of health checks. DoctorWithOpts runs
// exactly this list, and FindingChecks derives the finding-name catalog from
// the same table — so a new check cannot ship without registering here, and
// registering here automatically puts it under the ADR-033 enforcement tests
// (remediation_enforcement_test.go), forcing its author to declare a
// remediation contract. The hand-maintained duplicate list this replaces is
// how "Top Memory Consumers" shipped as an alarm with no lever.
var doctorChecks = []doctorCheck{
	{"RAM Pressure", []string{"RAM Pressure"}, checkRAMPressure},
	{"Memory Death Spiral", []string{"Memory Death Spiral"}, checkMemoryDeathSpiral},
	{"Swap Usage", []string{"Swap Usage"}, checkSwapUsage},
	{"Disk Space", []string{"Disk Space"}, checkDiskSpace},
	{"Memory Processes", []string{"Top Memory Consumers"}, checkTopMemoryProcesses},
	{"Spotlight Storm", []string{"Spotlight Storm"}, checkSpotlightStorm},
	{"Process Footprint", []string{"Process Footprint"}, checkProcessFootprint},
	{"Duplicate Model Brokers", []string{"Duplicate Model Brokers"}, checkDuplicateModelBrokers},
	{"Crash Logs", []string{"Kernel Panics (7d)", "Jetsam Events (7d)"},
		func(_ platform.Platform, r *DoctorReport) { checkRecentCrashLogs(r) }},
	{"App Crashes", []string{"App Crashes (7d)"},
		func(_ platform.Platform, r *DoctorReport) { checkAppCrashes(r) }},
	{"App Hangs", []string{"App Hangs (7d)"},
		func(_ platform.Platform, r *DoctorReport) { checkAppHangs(r) }},
	{"Thread Leaks", []string{"Thread Leaks"},
		func(_ platform.Platform, r *DoctorReport) { checkThreadLeaks(r) }},
	{"Sirsi Processes", []string{"Sirsi Processes"}, checkSirsiProcesses},
	{"Runaway Executor", []string{"Runaway Executor"}, checkRunawayExecutor},
	{"Local Snapshots", []string{"Local Snapshots"},
		func(_ platform.Platform, r *DoctorReport) { checkLocalSnapshots(r) }},
}

// externalFindingChecks are finding Check names appended to the report OUTSIDE
// DoctorWithOpts but resolved through the same remediation catalog, so the
// enforcement tests must cover them too.
var externalFindingChecks = []string{
	"binary-drift", // appended by `sirsi diagnose` (cmd/sirsi/anubis.go) from selfupdate.ScanHost
}

// FindingChecks returns the canonical list of every finding Check name the
// diagnostic can emit — derived from the registry the doctor actually runs,
// never hand-maintained. The ADR-033 enforcement tests walk this list.
func FindingChecks() []string {
	out := make([]string, 0, len(doctorChecks)+len(externalFindingChecks))
	for _, c := range doctorChecks {
		out = append(out, c.emits...)
	}
	return append(out, externalFindingChecks...)
}

// DoctorWithOpts runs the diagnostic with progress reporting.
func DoctorWithOpts(p platform.Platform, opts DoctorOpts) (*DoctorReport, error) {
	start := time.Now()
	report := &DoctorReport{
		Timestamp: start,
	}

	for i, c := range doctorChecks {
		prevCount := len(report.Findings)
		c.run(p, report)
		if opts.OnCheck != nil {
			sev := SeverityOK
			msg := "healthy"
			if len(report.Findings) > prevCount {
				last := report.Findings[len(report.Findings)-1]
				sev = last.Severity
				msg = last.Message
			}
			opts.OnCheck(c.name, sev, msg, i+1, len(doctorChecks))
		}
	}

	// A 7-day trend is HISTORY, not a live alarm — demote every trend finding to
	// Info BEFORE scoring/classifying so NO surface (the score, the health light,
	// the menubar, the Horus "Attention" brief) treats it as a current issue. The
	// count + cause stay in the finding's Message for the history/dashboard view;
	// only the alarm is removed. Owner's law: an element alarms ONLY for a
	// current, actionable issue — trends inform, they don't alarm.
	demoteTrendsToInfo(report.Findings)

	// Swap is STICKY on macOS — the kernel keeps it allocated after using it — so a
	// large swap reading while RAM pressure is normal is RESIDUE, not active
	// thrashing. Cap the Swap alarm at the CURRENT memory-pressure severity so it
	// only reads red when memory is genuinely pressured NOW. (Owner's law, the one
	// that has recurred: alarm ONLY on a current, actionable condition — and you
	// cannot clear swap while memory is calm; it drains as pressure stays low.)
	gateSwapOnPressure(report.Findings)

	report.Score = calculateScore(report.Findings)
	report.Status = classifyHealth(report.Findings)
	// Attach the safe remediation + its honesty class to each finding so every
	// surface can resolve it AND label the action truthfully.
	for i := range report.Findings {
		report.Findings[i].Fix = remediationCommand(report.Findings[i])
		// Only carry an honesty-class when there is an actual action — a healthy
		// finding (e.g. swap capped to OK because pressure is normal) must NOT show
		// a "relief / 7-day pattern" banner for a fix that isn't offered.
		if report.Findings[i].Fix == "" {
			report.Findings[i].FixKind = ""
		} else {
			report.Findings[i].FixKind = remediationKind(report.Findings[i])
		}
	}
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

	// ADR-031-B: surface Hapi's NodeCapacity pressure level + its SOURCE. The
	// menubar renders finding.Detail, so this is the Hapi pressure surface with no
	// Swift rebuild / no FDA churn: it shows whether pressure is the authoritative
	// kernel DISPATCH_SOURCE_MEMORYPRESSURE level ("kernel-dispatch", once the Hapi
	// daemon's watcher is live) or the bootstrap free-% estimate ("bootstrap-snapshot").
	// Darwin-only so the mock-platform diagnose tests stay deterministic.
	if p.Name() == "darwin" {
		nc := SampleNodeCapacity()
		finding.Detail += fmt.Sprintf(" | Pressure: %s (source: %s)", nc.Pressure, nc.PressureSource)
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

	usedMB := parseSwapUsedMB(line)

	// macOS uses swap proactively — a few hundred MB with healthy RAM is normal,
	// NOT pressure. Only flag swap that is genuinely large. (Was: any swap > 0 MB
	// warned "RAM pressure present", which cried wolf on 195 MB.)
	switch {
	// GREEN STANDARD: macOS swaps PROACTIVELY — it pages idle memory to disk and
	// leaves it there even when RAM is fine, so allocated swap ≠ pressure. A busy
	// dev machine (browsers + multiple AI agents) routinely sits at a few GB of
	// swap while perfectly healthy. Only genuinely large swap signals real
	// pressure. (Was: 1 GB warned → amber, so a normal loaded Mac was never green.)
	case usedMB < 4096: // < 4 GB — routine on a loaded machine
		finding.Severity = SeverityOK
		finding.Message = fmt.Sprintf("Swap routine (%.1f GB) — normal macOS paging, no pressure", usedMB/1024)
	case usedMB < 12288: // 4–12 GB — worth a look
		finding.Severity = SeverityWarn
		finding.Message = fmt.Sprintf("Swap elevated (%.1f GB) — under memory pressure", usedMB/1024)
	default: // > 12 GB — genuinely heavy
		finding.Severity = SeverityCritical
		finding.Message = fmt.Sprintf("Heavy swapping (%.1f GB) — system is thrashing", usedMB/1024)
	}

	report.Findings = append(report.Findings, finding)
}

// parseSwapUsedMB extracts the "used = X.XXM" value from a sysctl vm.swapusage
// line. Format: "total = 2048.00M  used = 150.00M  free = 1898.00M  (encrypted)".
// Shared by checkSwapUsage and SwapUsedBytes — one parse for the one probe.
func parseSwapUsedMB(line string) float64 {
	idx := strings.Index(line, "used = ")
	if idx < 0 {
		return 0
	}
	rest := line[idx+len("used = "):]
	fields := strings.Fields(rest)
	if len(fields) == 0 {
		return 0
	}
	usedMB, _ := strconv.ParseFloat(strings.TrimSuffix(fields[0], "M"), 64)
	return usedMB
}

// SwapUsedBytes returns the swap "used" bytes via sysctl vm.swapusage — the
// SAME probe checkSwapUsage runs, exposed for the fast vitals surface (TUI
// design proof gap V1). Returns 0 when the probe or parse fails.
func SwapUsedBytes(p platform.Platform) int64 {
	out, err := p.Command("sysctl", "-n", "vm.swapusage")
	if err != nil {
		return 0
	}
	return int64(parseSwapUsedMB(strings.TrimSpace(string(out))) * 1024 * 1024)
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
		return memSize(processes[i]) > memSize(processes[j])
	})

	// Report top 5 memory consumers
	var top []string
	for i, proc := range processes {
		if i >= 5 {
			break
		}
		top = append(top, fmt.Sprintf("%s (%s)", proc.Name, FormatBytes(memSize(proc))))
	}

	// Check for any single process using > 4GB
	// Sized by PHYSICAL FOOTPRINT, not RSS. RSS counts resident pages only; on
	// Apple Silicon the compressor holds the rest. Measured 2026-07-27: the
	// gemma broker read 4.71 GB RSS against a 29.4 GB footprint and a 40.5 GB
	// peak, and the Virtualization VM read 5.4 GB RSS against 18.4 GB.
	//
	// The Gemma exemption is GONE. It read:
	//
	//   "The warm Gemma broker is an intentional, capacity-capped model
	//    reservation... RAM Pressure and Memory Death Spiral still alarm if that
	//    reservation becomes unsafe."
	//
	// That premise was falsified on 2026-07-27. The reservation became unsafe
	// three times in 24 hours — Jetsam footprints of 31, 43.9 and 38.1 GB — and
	// neither named check alarmed, because both also sampled resident memory.
	// A health surface that excuses its own components is the exact failure this
	// codebase keeps recording. Sirsi's own broker is the most likely offender on
	// a developer's machine and must be the FIRST thing named, not the one thing
	// exempted.
	//
	// The Colima VM is different: it anchors the sovereign consensus ledger.
	// Inside the reservation it was LAUNCHED with (Lima's generated record,
	// bound to a live vz.pid), it is reserved capacity, not a hog. It stays
	// visible as capacity-reserved instead of being hidden from the report.
	var hogs, reserved []string
	for _, proc := range processes {
		size := memSize(proc)
		if size <= 4*1024*1024*1024 {
			continue
		}
		if isAppleVirtVM(p, proc.PID) {
			if capBytes, ok := ColimaVMReservation(); ok && size <= capBytes {
				reserved = append(reserved, fmt.Sprintf("%s at %s of %s reserved",
					proc.Name, FormatBytes(size), FormatBytes(capBytes)))
				continue
			}
		}
		hogs = append(hogs, fmt.Sprintf("%s at %s", proc.Name, FormatBytes(size)))
	}

	finding := DiagnosticFinding{
		Check:  "Top Memory Consumers",
		Detail: strings.Join(top, " | "),
	}
	if len(reserved) > 0 {
		finding.Detail += fmt.Sprintf("  •  load-bearing, capacity-reserved: %s", strings.Join(reserved, ", "))
	}

	switch {
	case len(hogs) > 0:
		finding.Severity = SeverityWarn
		finding.Message = fmt.Sprintf("Memory hog detected: %s", strings.Join(hogs, ", "))
	case len(reserved) > 0:
		finding.Severity = SeverityOK
		finding.Message = fmt.Sprintf("No unbounded process over 4 GB (%s)", strings.Join(reserved, ", "))
	default:
		finding.Severity = SeverityOK
		finding.Message = "No individual process exceeding 4 GB"
	}

	report.Findings = append(report.Findings, finding)
}

// isAppleVirtVM reports whether pid is an Apple Virtualization VM process. This
// is IDENTITY ONLY — deliberately no longer a combined identity+cap predicate.
// The first pass proved "an Apple-Virt process whose RSS fits a number read out
// of a config file" and then treated that conjunction as identity, so a VM that
// grew past the number silently stopped being recognized as a VM at all. The
// reservation is now a separate, separately-sourced fact (ColimaVMReservation),
// which keeps "what is this process" and "what was it promised" independent.
func isAppleVirtVM(p platform.Platform, pid int) bool {
	out, err := p.Command("ps", "-p", strconv.Itoa(pid), "-o", "command=")
	if err != nil {
		return false
	}
	return strings.Contains(string(out), "com.apple.Virtualization.VirtualMachine")
}

// memSize is the honest size of a process: physical footprint when available,
// RSS only as a last resort. Every caller that sizes a process must use this —
// reading .RSS directly is how the broker stayed invisible through three OOMs.
func memSize(pr ProcessInfo) int64 {
	if pr.Footprint > 0 {
		return pr.Footprint
	}
	return pr.RSS
}

// isCapacityCappedGemmaBroker was DELETED 2026-07-27, not left unused.
//
// It exempted Sirsi's own broker from the memory-hog check, on the premise
// that RAM Pressure and Memory Death Spiral would alarm if the reservation
// became unsafe. It became unsafe three times that day — Jetsam footprints of
// 31, 43.9 and 38.1 GB — and neither alarmed, because both also sampled
// resident memory while 39.9 GB sat compressed.
//
// Deleted rather than left unused so the exemption cannot be quietly
// re-applied: there is nothing left to call.

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

// transientEventTolerance is THE GREEN STANDARD for 7-day trend checks (Jetsam,
// kernel panics, app hangs): a handful of ISOLATED events over a week is normal
// background on a busy machine — informational history, NOT a current alarm. At
// or below this count (and not a multi-day trend) the finding stays SeverityOK so
// the at-a-glance light can be GREEN. Above it (a real spike) → Warn (amber);
// recurring across trendDayThreshold+ days → Critical (amber). Without this, a
// single Jetsam kill 6 days ago pinned the machine amber for a week and green was
// unreachable on any real dev machine (the owner's "it's always yellow" report).
const transientEventTolerance = 3

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
		// A "(7d)" trend is HISTORY (demoteTrendsToInfo greens it), so the message
		// must read as a record, not a present-tense alarm — "%s" is what was true
		// AT THE TIME, not now. A green dot saying "system under RAM pressure" was
		// the exact contradiction the owner flagged.
		f.Message = fmt.Sprintf("%d %ss on %d of the last %d days — a 7-day record (%s at the time); not happening now",
			count, noun, activeDays, crashWindowDays, trendCause)
	case count <= transientEventTolerance:
		// GREEN STANDARD: a few isolated events over a week is normal background,
		// not a current problem — informational, keeps the light green.
		f.Severity = SeverityOK
		f.Message = fmt.Sprintf("%d %s(s) in the last %d days — isolated, no sustained trend (normal background)",
			count, noun, crashWindowDays)
	default:
		f.Severity = SeverityWarn
		f.Message = fmt.Sprintf("%d %ss in the last %d days, clustered in %d day(s) — elevated, watch for a trend",
			count, noun, crashWindowDays, activeDays)
	}
	return f
}

// hangReportScanFn returns one record per macOS UI-hang / CPU-saturation report
// (process + time) — the system's record of main-thread stalls (beachballs,
// frozen UI, dropped frames) and processes that pegged the CPU. Injectable (Rule
// A16) so the trend classifier is testable without real reports on the host.
var hangReportScanFn = defaultHangReportScan

// isHangReport matches the report kinds that signal a stall or saturation:
//   - .hang / .spin   — spindumps: the main thread stalled (a UI hang/beachball)
//   - .cpu_resource.diag — a process exceeded a sustained-CPU budget (the
//     "main-thread bulging instead of multithread dispersion" signature; on
//     macOS 26 this is the dominant on-disk evidence of a runaway/saturating app)
func isHangReport(name string) bool {
	return strings.HasSuffix(name, ".hang") ||
		strings.HasSuffix(name, ".spin") ||
		strings.HasSuffix(name, ".cpu_resource.diag")
}

// hangReportProcess pulls the offending process from a report filename of the
// form "<process>_<YYYY-MM-DD-HHMMSS>_<host>.<ext>" or "<process>-<date>.<ext>".
func hangReportProcess(name string) string {
	for _, s := range []string{".cpu_resource.diag", ".wakeups_resource.diag", ".hang", ".spin", ".diag"} {
		name = strings.TrimSuffix(name, s)
	}
	// The process is the leading token before the first date stamp (_20.. / -20..).
	for _, sep := range []string{"_20", "-20"} {
		if i := strings.Index(name, sep); i > 0 {
			return name[:i]
		}
	}
	if i := strings.IndexAny(name, "_-"); i > 0 {
		return name[:i]
	}
	return name
}

// hangEvent is one hang/spin/CPU-saturation report: which process, and when.
// Tagging the process per event lets checkAppHangs separate REAL user-facing app
// freezes from background OS housekeeping (Spotlight indexing, cloud sync) that
// routinely trips CPU budgets without the user ever seeing a beachball.
type hangEvent struct {
	process string
	when    time.Time
}

// backgroundCPUDaemons are macOS system daemons that legitimately exceed CPU
// budgets during routine housekeeping (indexing, cloud/file sync, media
// analysis, backup). A .cpu_resource.diag from one of these is NOT the user's app
// hanging — surfacing it as a "freeze/beachball" alarms about something the user
// never experienced. Matched case-insensitively as a substring. Deliberately
// omits short ambiguous tokens (e.g. "bird") that could collide with app names.
var backgroundCPUDaemons = []string{
	"spotlightknowledged", "corespotlightd", "knowledge-agent",
	"mds_stores", "mdworker", "mdsync", "mdbulkimport",
	"fileproviderd", "cloudd", "brctld", "syncdefaultsd",
	"photoanalysisd", "mediaanalysisd", "photolibraryd", "amplibraryagent",
	"backupd", "suggestd", "parsecd", "assistantd", "nsurlsessiond", "triald",
}

// isBackgroundCPUDaemon reports whether a process is OS housekeeping rather than
// a user-facing app the user would feel hang.
func isBackgroundCPUDaemon(process string) bool {
	p := strings.ToLower(process)
	for _, d := range backgroundCPUDaemons {
		if strings.Contains(p, d) {
			return true
		}
	}
	return false
}

// defaultHangReportScan reads the same DiagnosticReports dirs as the crash scan
// and returns one hangEvent per report (process + modtime).
func defaultHangReportScan() []hangEvent {
	var events []hangEvent
	seen := map[string]bool{}
	for _, dir := range appCrashDirsFn() {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			name := e.Name()
			if !isHangReport(name) || seen[name] {
				continue
			}
			seen[name] = true
			info, err := e.Info()
			if err != nil {
				continue
			}
			events = append(events, hangEvent{process: hangReportProcess(name), when: info.ModTime()})
		}
	}
	return events
}

// topOffenders renders the worst-N process tally as "name ×count | name ×count".
func topOffenders(byProcess map[string]int, n int) string {
	type pc struct {
		p string
		c int
	}
	list := make([]pc, 0, len(byProcess))
	for p, c := range byProcess {
		list = append(list, pc{p, c})
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].c != list[j].c {
			return list[i].c > list[j].c
		}
		return list[i].p < list[j].p
	})
	parts := make([]string, 0, n)
	for i := 0; i < len(list) && i < n; i++ {
		parts = append(parts, fmt.Sprintf("%s ×%d", list[i].p, list[i].c))
	}
	return strings.Join(parts, " | ")
}

// checkAppHangs surfaces UI hangs and CPU-saturation spikes — the user-facing
// "freeze / beachball / stutter / dropped-frame" pathology that hits gamers and
// productivity users as hard as developers. Trend-aware (parity with the crash
// checks): a clustered one-off stays Warn; recurrence across the window escalates
// to Critical. The Detail names the worst offenders so the cause is actionable.
func checkAppHangs(report *DoctorReport) {
	events := hangReportScanFn()
	now := time.Now()

	// Separate REAL user-facing app freezes from background OS housekeeping. Only
	// the former is a beachball the user actually felt; the latter (Spotlight
	// indexing, cloud sync) routinely trips CPU budgets and must NOT be reported as
	// "your apps are freezing" — that would alarm a user about a non-event.
	var userTimes []time.Time
	userByProc := map[string]int{}
	daemonByProc := map[string]int{}
	for _, e := range events {
		if isBackgroundCPUDaemon(e.process) {
			daemonByProc[e.process]++
		} else {
			userByProc[e.process]++
			userTimes = append(userTimes, e.when)
		}
	}
	daemonTotal := 0
	for _, c := range daemonByProc {
		daemonTotal += c
	}

	count, activeDays, isTrend := classifyEventTrend(userTimes, now)
	f := DiagnosticFinding{Check: "App Hangs (7d)", ActiveDays: activeDays, Trend: isTrend}
	switch {
	case count == 0 && daemonTotal == 0:
		f.Severity = SeverityOK
		f.Message = fmt.Sprintf("No UI hangs or CPU-saturation spikes in the last %d days", crashWindowDays)
	case count == 0:
		// Background daemons only — routine housekeeping, not app freezes. Stays
		// informational (no alarm, scores zero) and is honest about the cause.
		f.Severity = SeverityInfo
		f.Message = fmt.Sprintf("No app freezes — %d background CPU-budget event(s) from system housekeeping (indexing/sync), not your apps", daemonTotal)
		f.Detail = "Background only: " + topOffenders(daemonByProc, 3) + " — normal macOS maintenance. If Spotlight churns often, exclude busy folders from indexing."
	case isTrend:
		f.Severity = SeverityCritical
		// History (demoted to green), not a present-tense claim of current saturation.
		f.Message = fmt.Sprintf("%d app freeze event(s) on %d of the last %d days — a 7-day record of main-thread stalls (beachballs); not happening now",
			count, activeDays, crashWindowDays)
		f.Detail = topOffenders(userByProc, 3)
	case count <= transientEventTolerance:
		// GREEN STANDARD: a few isolated stalls over a week are normal — keep green.
		f.Severity = SeverityInfo
		f.Message = fmt.Sprintf("%d app freeze event(s) in the last %d days — isolated, no sustained trend (normal background)",
			count, crashWindowDays)
		f.Detail = topOffenders(userByProc, 3)
	default:
		f.Severity = SeverityWarn
		f.Message = fmt.Sprintf("%d app freeze event(s) in the last %d days, clustered in %d day(s) — elevated, watch for a trend",
			count, crashWindowDays, activeDays)
		f.Detail = topOffenders(userByProc, 3)
	}
	// When real hangs coexist with background noise, say so — don't let daemon
	// events inflate the offender list the user acts on.
	if count > 0 && daemonTotal > 0 {
		f.Detail += fmt.Sprintf("  (+%d background housekeeping events ignored: %s)", daemonTotal, topOffenders(daemonByProc, 2))
	}
	report.Findings = append(report.Findings, f)
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
	resolution := fmt.Sprintf("%d crash report(s) accumulated (top: %q ×%d). These are diagnostic logs, not live failures — crash reports are caution-tier, so clear the backlog with: sirsi clean --include-caution", len(crashes), top, topN)
	switch {
	case len(crashes) > 10:
		finding.Severity = SeverityWarn // history, not critical — was over-alarming
		finding.Message = fmt.Sprintf("%d crash reports (7d) — top: %q (%d) · clear with `sirsi clean --include-caution`", len(crashes), top, topN)
		finding.Detail = resolution
	case len(crashes) > 0:
		finding.Severity = SeverityWarn
		finding.Message = fmt.Sprintf("%d crash report(s) (7d) — top: %q (%d) · clear with `sirsi clean --include-caution`", len(crashes), top, topN)
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

// demoteTrendsToInfo strips the alarm from EVERY 7-day-window finding
// (Jetsam/kernel-panic/app-crash/app-hang counts). The whole "(7d)" category is
// HISTORY — last week's events can't be acted on now and no click can clear them
// — so none of it may read as "attention", whether or not it crossed the
// sustained-trend threshold (3 clustered crashes is still history). Demoting to
// Info keeps each finding visible (its Message carries the count + cause for the
// history/dashboard view) while removing the yellow/red on every surface. The
// CURRENT signals (live RAM/Swap/Disk/etc.) have no "(7d)" suffix and aren't
// trends, so they keep their severity and still alarm.
func demoteTrendsToInfo(findings []DiagnosticFinding) {
	for i := range findings {
		f := &findings[i]
		historical := f.Trend || strings.HasSuffix(f.Check, "(7d)")
		if historical && f.Severity > SeverityInfo {
			f.Severity = SeverityInfo
		}
	}
}

// gateSwapOnPressure caps swap-residue alarms at the CURRENT memory-pressure
// severity. macOS keeps swap ALLOCATED after using it, so a large swap reading
// while RAM pressure is normal is residue — not active thrashing. "Heavy
// swapping — system is thrashing" in red, while RAM Pressure reads healthy, is
// the recurring false alarm: nothing the user can do clears swap while memory is
// calm (it drains only as pressure stays low). So swap may never alarm higher
// than the live RAM Pressure finding; when capped, its message is rewritten to
// the truth. If there is no RAM Pressure reading, swap is left untouched.
func gateSwapOnPressure(findings []DiagnosticFinding) {
	pressureSev := SeverityOK
	havePressure := false
	for i := range findings {
		if findings[i].Check == "RAM Pressure" {
			pressureSev = findings[i].Severity
			havePressure = true
		}
	}
	if !havePressure {
		return
	}
	for i := range findings {
		f := &findings[i]
		if (f.Check != "Swap Usage" && f.Check != "Memory Death Spiral") || f.Severity <= pressureSev {
			continue
		}
		f.Severity = pressureSev
		if f.Check == "Memory Death Spiral" {
			if pressureSev <= SeverityInfo {
				f.Message = "No active memory death spiral — memory pressure is normal; high swap allocation is retained history"
			} else {
				f.Message = "Memory pressure elevated, but no active death spiral is confirmed"
			}
			continue
		}
		usedGB := parseSwapUsedGB(f.Detail)
		if pressureSev <= SeverityInfo {
			f.Message = fmt.Sprintf("Swap in use (%.1f GB) — memory pressure is normal; macOS keeps swap allocated after using it", usedGB)
		} else {
			f.Message = fmt.Sprintf("Swap elevated (%.1f GB) — memory is under some pressure", usedGB)
		}
	}
}

// parseSwapUsedGB extracts the "used = X.XXM" value (in GB) from a vm.swapusage
// detail line: "total = …  used = 12986.31M  free = …  (encrypted)".
func parseSwapUsedGB(detail string) float64 {
	if idx := strings.Index(detail, "used = "); idx >= 0 {
		if fields := strings.Fields(detail[idx+len("used = "):]); len(fields) > 0 {
			mb, _ := strconv.ParseFloat(strings.TrimSuffix(fields[0], "M"), 64)
			return mb / 1024
		}
	}
	return 0
}

// calculateScore derives a 0-100 health score from the CURRENT state.
//
// The score is point-in-time — "how healthy is this Mac RIGHT NOW" — not a
// credit-score-style rap sheet of the past week. Historical 7-day trend
// findings (Jetsam/crash/hang/panic counts) are therefore EXCLUDED from the
// score: a machine that is currently fine reads as fine even if last week was
// rough. Those trends are still produced as findings and belong on a dashboard
// for longitudinal analysis — they inform, they don't deduct.
func calculateScore(findings []DiagnosticFinding) int {
	score := 100
	for _, f := range findings {
		if f.Trend {
			continue // historical 7-day trend → dashboard, never the live score
		}
		switch {
		case isLiveCritical(f):
			score -= 25 // a live, session-threatening problem
		case f.Severity == SeverityCritical:
			score -= 12 // a current critical condition
		case f.Severity == SeverityWarn:
			score -= 6
		}
		// Info/OK do not reduce health.
	}
	if score < 0 {
		score = 0
	}
	return score
}

// RemediationFor exposes a finding's fix command to callers outside this
// package. `sirsi ask` needs it so the heuristic rung can answer with an ACTION
// and not just a diagnosis — a measured problem with no offered remedy is half
// an answer (ADR-033).
func RemediationFor(f DiagnosticFinding) string { return remediationCommand(f) }
