package guard

import (
	"fmt"
	"strings"
	"testing"

	"github.com/SirsiMaster/sirsi-pantheon/internal/platform"
)

const runawayTmpDir = "/var/folders/xx/T/"

func runawayMock(psLines []string, treeCount int) *platform.Mock {
	trees := make([]string, 0, treeCount)
	for i := 0; i < treeCount; i++ {
		trees = append(trees, fmt.Sprintf("%sgo-build%06d", runawayTmpDir, i))
	}
	return &platform.Mock{
		NameStr: "mock",
		CommandResults: map[string]string{
			"ps -axo args":                 strings.Join(psLines, "\n"),
			"getconf DARWIN_USER_TEMP_DIR": runawayTmpDir + "\n",
			"find " + runawayTmpDir + " -maxdepth 1 ( -name go-build* -o -name sirsi-integration-* ) -mmin -1440": strings.Join(trees, "\n"),
		},
	}
}

func runawayFinding(t *testing.T, r *DoctorReport, check string) DiagnosticFinding {
	t.Helper()
	for _, f := range r.Findings {
		if f.Check == check {
			return f
		}
	}
	t.Fatalf("no %q finding in %+v", check, r.Findings)
	return DiagnosticFinding{}
}

func TestRunawayExecutorHealthyHostIsOK(t *testing.T) {
	// One interactive session (no -p), one short-lived headless call, a few trees.
	p := runawayMock([]string{
		"/usr/local/bin/claude",
		"/usr/local/bin/claude -p summarize this",
		"vim claude-notes.md", // 'claude' in an argument must not count
	}, 4)
	report := &DoctorReport{}
	checkRunawayExecutor(p, report)

	f := runawayFinding(t, report, "Runaway Executor")
	if f.Severity != SeverityOK {
		t.Fatalf("healthy host must be OK, got %v (%s)", f.Severity, f.Message)
	}
}

func TestRunawayExecutorSessionFloodAlarms(t *testing.T) {
	var ps []string
	for i := 0; i < runawaySessionsCritical; i++ {
		ps = append(ps, fmt.Sprintf("/usr/local/bin/claude --print item-%d", i))
	}
	p := runawayMock(ps, 0)
	report := &DoctorReport{}
	checkRunawayExecutor(p, report)

	f := runawayFinding(t, report, "Runaway Executor")
	if f.Severity != SeverityCritical {
		t.Fatalf("session flood must be CRITICAL, got %v", f.Severity)
	}
}

func TestRunawayExecutorTreeChurnAlarms(t *testing.T) {
	p := runawayMock(nil, runawayTreesWarn)
	report := &DoctorReport{}
	checkRunawayExecutor(p, report)

	f := runawayFinding(t, report, "Runaway Executor")
	if f.Severity != SeverityWarn {
		t.Fatalf("tree churn at warn threshold must WARN, got %v", f.Severity)
	}
}

// The incident's exact lesson, as a test: an alarming Runaway Executor finding
// must carry a REAL lever (ADR-033) — and that lever must be the quarantine,
// never a monitor.
func TestRunawayExecutorAlarmCarriesQuarantineLever(t *testing.T) {
	f := DiagnosticFinding{Check: "Runaway Executor", Severity: SeverityWarn}
	if got := remediationCommand(f); got != "sirsi router quarantine-worker" {
		t.Fatalf("warn-level runaway must map to the quarantine lever, got %q", got)
	}
	if got := remediationKind(f); got != FixRelief {
		t.Fatalf("quarantine stops the live cause (relief), got %q", got)
	}
}

func TestRunawayExecutorFailurePathsStayQuiet(t *testing.T) {
	// Rule A16: exercise the ps/getconf failure path — no finding noise beyond OK.
	p := &platform.Mock{NameStr: "mock", CommandError: fmt.Errorf("exec denied")}
	report := &DoctorReport{}
	checkRunawayExecutor(p, report)

	f := runawayFinding(t, report, "Runaway Executor")
	if f.Severity != SeverityOK {
		t.Fatalf("unreadable host must not fabricate an alarm, got %v", f.Severity)
	}
}
