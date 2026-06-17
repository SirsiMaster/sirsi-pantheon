package guard

import (
	"fmt"
	"strings"
	"testing"
)

func TestParseThreadCounts(t *testing.T) {
	// Real `top -l 1 -stats pid,th,command -o th` shape, incl. header + the
	// kernel_task "running/total" thread render that must be skipped/parsed.
	out := `Processes: 400 total
PID    #TH    COMMAND
0      876/19 kernel_task
77521  78     Claude
1502   55     Google Chrome
1      4      launchd`
	procs := parseThreadCounts(out)
	// kernel_task (pid 0) and launchd (pid 1) are skipped; 2 real rows remain.
	if len(procs) != 2 {
		t.Fatalf("got %d procs, want 2 (kernel_task + launchd skipped): %+v", len(procs), procs)
	}
	if procs[0].name != "Claude" || procs[0].threads != 78 {
		t.Errorf("first proc = %+v, want Claude/78", procs[0])
	}
	if procs[1].name != "Google Chrome" || procs[1].threads != 55 {
		t.Errorf("second proc = %+v, want 'Google Chrome'/55", procs[1])
	}
}

func TestCheckThreadLeaks(t *testing.T) {
	old := threadCountScanFn
	defer func() { threadCountScanFn = old }()

	// Healthy: everything well under the warn threshold.
	threadCountScanFn = func() ([]procThreads, error) {
		return []procThreads{{100, "Chrome", 80}, {200, "gopls", 30}}, nil
	}
	report := &DoctorReport{}
	checkThreadLeaks(report)
	if f := findByCheck(report.Findings, "Thread Leaks"); f == nil || f.Severity != SeverityOK {
		t.Fatalf("healthy: got %+v, want OK", f)
	}

	// Above warn, below critical → Warn, names the offender.
	threadCountScanFn = func() ([]procThreads, error) {
		return []procThreads{{100, "LeakyApp", 600}, {200, "Chrome", 90}}, nil
	}
	report = &DoctorReport{}
	checkThreadLeaks(report)
	f := findByCheck(report.Findings, "Thread Leaks")
	if f.Severity != SeverityWarn {
		t.Errorf("warn: got severity=%v, want Warn", f.Severity)
	}
	if !strings.Contains(f.Detail, "LeakyApp (600 threads") {
		t.Errorf("warn detail %q should name the offender", f.Detail)
	}

	// Above critical → Critical.
	threadCountScanFn = func() ([]procThreads, error) {
		return []procThreads{{100, "RunawayApp", 1500}, {200, "Chrome", 90}}, nil
	}
	report = &DoctorReport{}
	checkThreadLeaks(report)
	f = findByCheck(report.Findings, "Thread Leaks")
	if f.Severity != SeverityCritical {
		t.Errorf("crit: got severity=%v, want Critical", f.Severity)
	}
	if !strings.Contains(f.Message, "RunawayApp holds 1500 threads") {
		t.Errorf("crit message %q should name the offender + count", f.Message)
	}

	// Scan failure → Info, never a crash.
	threadCountScanFn = func() ([]procThreads, error) { return nil, fmt.Errorf("top failed") }
	report = &DoctorReport{}
	checkThreadLeaks(report)
	if f := findByCheck(report.Findings, "Thread Leaks"); f == nil || f.Severity != SeverityInfo {
		t.Errorf("scan error: got %+v, want a single Info finding", f)
	}
}
