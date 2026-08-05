package guard

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/platform"
)

func findFinding(r *DoctorReport, check string) (DiagnosticFinding, bool) {
	for _, f := range r.Findings {
		if f.Check == check {
			return f, true
		}
	}
	return DiagnosticFinding{}, false
}

func writeLedger(t *testing.T, home string, age time.Duration, labels ...string) string {
	t.Helper()
	dir := filepath.Join(home, "Library", "LaunchAgents")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, ".sirsi-paused")
	body := ""
	for _, l := range labels {
		body += l + "\n"
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	when := time.Now().Add(-age)
	if err := os.Chtimes(path, when, when); err != nil {
		t.Fatal(err)
	}
	return path
}

// The real incident: a pause ledger written 2026-07-31 sat unresumed for five
// days. Pantheon's heal duty re-enabled 30 of 34 labels piecemeal, so the
// disabled-label check went quiet while four CI runners stayed dead — and the
// surviving ledger meant the next pause would re-disable everything.
func TestCheckPauseLedger_StaleLedgerIsCritical(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeLedger(t, home, 5*24*time.Hour,
		"actions.runner.SirsiMaster-Assiduous.m5-sirsi",
		"ai.sirsi.gemma-broker")

	var r DoctorReport
	checkPauseLedger(&platform.Mock{}, &r)

	f, ok := findFinding(&r, "Agent Pause Ledger")
	if !ok {
		t.Fatal("no Agent Pause Ledger finding produced")
	}
	if f.Severity != SeverityCritical {
		t.Errorf("severity = %v, want Critical for a 5-day-old ledger", f.Severity)
	}
	if f.Detail == "" {
		t.Error("finding carries no Detail — this check is guidance-only, so the steps ARE the remediation")
	}
}

// A pause that is minutes old is a benchmark in progress, not an incident.
// Alarming on it would train the operator to ignore this check.
func TestCheckPauseLedger_FreshLedgerIsOnlyAWarning(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeLedger(t, home, 10*time.Minute, "ai.sirsi.gemma-broker")

	var r DoctorReport
	checkPauseLedger(&platform.Mock{}, &r)

	f, _ := findFinding(&r, "Agent Pause Ledger")
	if f.Severity != SeverityWarn {
		t.Errorf("severity = %v, want Warn for a 10-minute-old pause", f.Severity)
	}
}

// No ledger is the healthy state and must be reported as OK, not silence — a
// check that says nothing when healthy cannot be distinguished from one that
// never ran.
func TestCheckPauseLedger_NoLedgerReportsOK(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	var r DoctorReport
	checkPauseLedger(&platform.Mock{}, &r)

	f, ok := findFinding(&r, "Agent Pause Ledger")
	if !ok {
		t.Fatal("healthy state produced no finding at all")
	}
	if f.Severity != SeverityOK {
		t.Errorf("severity = %v, want OK with no ledger present", f.Severity)
	}
}

func TestCheckPauseLedger_CountsLabels(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeLedger(t, home, 7*24*time.Hour, "a", "b", "c", "", "d")

	var r DoctorReport
	checkPauseLedger(&platform.Mock{}, &r)

	f, _ := findFinding(&r, "Agent Pause Ledger")
	if want := "4 agent(s)"; !strings.Contains(f.Message, want) {
		t.Errorf("message = %q, want it to report %q (blank lines must not count)", f.Message, want)
	}
}
