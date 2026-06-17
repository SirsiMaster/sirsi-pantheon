// Package guard — threadleak.go
//
// Thread-leak / pool-exhaustion detection (TRACEABILITY backlog "get #2"). This
// is the IN-process failure — a single process accumulating threads without
// bound — distinct from the Orphan Hunter (orphan.go), which reaps SEPARATE
// zombie processes. A runaway thread count starves the scheduler and is a common
// cause of the "everything got slow / beachball" pathology.
package guard

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

// procThreads is a process and its live thread count.
type procThreads struct {
	pid     int
	name    string
	threads int
}

// Thread-leak thresholds. A genuine leak / pool exhaustion runs into the
// hundreds–thousands of threads, far above any healthy app (browsers and IDEs
// sit well under ~150). Conservative so normal software never trips a warning.
const (
	threadWarnCount = 500
	threadCritCount = 1200
)

// threadCountScanFn returns live per-process thread counts. Injectable (Rule A16)
// so the classifier is testable without sampling the host.
var threadCountScanFn = defaultThreadCountScan

// defaultThreadCountScan reads live thread counts via `top` — macOS `ps` has no
// thread-count column. Skips the header and PID ≤ 1 (kernel_task/launchd, which
// legitimately hold many threads) and any row it can't parse.
func defaultThreadCountScan() ([]procThreads, error) {
	out, err := exec.Command("top", "-l", "1", "-n", "40", "-stats", "pid,th,command", "-o", "th").Output()
	if err != nil {
		return nil, err
	}
	return parseThreadCounts(string(out)), nil
}

// parseThreadCounts parses `top -stats pid,th,command` output into per-process
// thread counts. Split out from the exec so it is unit-testable (Rule A16).
func parseThreadCounts(out string) []procThreads {
	var procs []procThreads
	for _, line := range strings.Split(out, "\n") {
		f := strings.Fields(line)
		if len(f) < 3 {
			continue
		}
		pid, err := strconv.Atoi(f[0])
		if err != nil || pid <= 1 { // header row or kernel_task/launchd
			continue
		}
		// kernel_task renders threads as "running/total" (e.g. "876/19"); plain
		// processes are a bare integer. Take the count before any '/'.
		thStr := f[1]
		if i := strings.IndexByte(thStr, '/'); i >= 0 {
			thStr = thStr[:i]
		}
		th, err := strconv.Atoi(thStr)
		if err != nil {
			continue
		}
		procs = append(procs, procThreads{pid: pid, name: strings.Join(f[2:], " "), threads: th})
	}
	return procs
}

// checkThreadLeaks flags processes whose live thread count is anomalously high.
func checkThreadLeaks(report *DoctorReport) {
	procs, err := threadCountScanFn()
	f := DiagnosticFinding{Check: "Thread Leaks"}
	if err != nil {
		f.Severity = SeverityInfo
		f.Message = "Thread-count scan unavailable"
		report.Findings = append(report.Findings, f)
		return
	}

	var worst procThreads
	var offenders []string
	for _, p := range procs {
		if p.threads > worst.threads {
			worst = p
		}
		if p.threads >= threadWarnCount {
			offenders = append(offenders, fmt.Sprintf("%s (%d threads, pid %d)", p.name, p.threads, p.pid))
		}
	}

	switch {
	case len(offenders) == 0:
		f.Severity = SeverityOK
		f.Message = fmt.Sprintf("No thread leaks — busiest process holds %d threads", worst.threads)
	case worst.threads >= threadCritCount:
		f.Severity = SeverityCritical
		f.Message = fmt.Sprintf("%s holds %d threads — likely a thread leak / pool exhaustion; restart it to reclaim",
			worst.name, worst.threads)
		f.Detail = strings.Join(offenders, " | ")
	default:
		f.Severity = SeverityWarn
		f.Message = fmt.Sprintf("%s holds %d threads — unusually high; watch for a thread leak",
			worst.name, worst.threads)
		f.Detail = strings.Join(offenders, " | ")
	}
	report.Findings = append(report.Findings, f)
}
