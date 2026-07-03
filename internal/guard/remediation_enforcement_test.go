package guard

import (
	"strings"
	"testing"
)

// ADR-033 "Three-Outcome Law" enforced by test: every finding resolves to a real
// ACTION, honest GUIDANCE, or plain INFO — never a passive monitor. These tests
// are the guardrail so the "tapping Fix opened a dashboard" bug can't come back.
//
// The check list is NOT hand-maintained: it is FindingChecks(), derived from the
// doctorChecks registry the doctor actually runs (doctor.go). A hand-copied list
// here is exactly how "Top Memory Consumers" shipped as a warn-level alarm with
// no lever while the phantom name "Memory Processes" held its mapping — the list
// had rotted and nothing could notice.

// bannedFixCommands are passive/monitor verbs that must NEVER be a finding's fix —
// they observe, they don't remediate. (`sirsi guard` == `sirsi monitor` was the
// original offender.)
var bannedFixCommands = map[string]bool{
	"guard": true, "monitor": true, "status": true,
	"diagnose": true, "scan": true, "watch": true,
}

// noLeverRequired findings legitimately have no required one-click lever — either
// GUIDANCE-only (name the cause + manual steps) or INFO-only (never alarms). An
// empty remediationCommand is correct for them. Anything NOT here that can alarm
// MUST have a real lever.
var noLeverRequired = map[string]bool{
	"Kernel Panics (7d)": true, // guidance: hardware/driver — nothing safe to auto-do
	"Sirsi Processes":    true, // info
	"Local Snapshots":    true, // info: always SeverityInfo; carries an OPTIONAL reclaim, never alarms
}

// firstToken returns the verb from a "sirsi <verb> …" remediation command — the
// token the banned-monitor check applies to.
func firstToken(s string) string {
	parts := strings.Fields(s)
	if len(parts) >= 2 && parts[0] == "sirsi" {
		return parts[1]
	}
	if len(parts) >= 1 {
		return parts[0]
	}
	return ""
}

// TestRemediationNeverAMonitor: no finding's fix, at any severity, is a passive
// monitor command. This is the durable fix for "tapping Relieve opened a dashboard."
func TestRemediationNeverAMonitor(t *testing.T) {
	for _, check := range FindingChecks() {
		for _, sev := range []DiagnosticSeverity{SeverityWarn, SeverityCritical} {
			f := DiagnosticFinding{Check: check, Severity: sev}
			cmd := remediationCommand(f)
			if cmd == "" {
				continue
			}
			if verb := firstToken(cmd); bannedFixCommands[verb] {
				t.Errorf("%q/%v maps its fix to a MONITOR (%q) — a monitor is never a fix (ADR-033)", check, sev, cmd)
			}
		}
	}
}

// TestEveryAlarmingCheckHasLeverOrGuidance: a finding that can reach Warn+ must
// offer a real lever, unless it is explicitly guidance-only. No silent dead-ends.
// "Top Memory Consumers" is deliberately NOT exempt: a warn-level memory hog gets
// `sirsi relieve --memory` (the #124 lever), never "Informational".
func TestEveryAlarmingCheckHasLeverOrGuidance(t *testing.T) {
	for _, check := range FindingChecks() {
		if noLeverRequired[check] {
			continue
		}
		f := DiagnosticFinding{Check: check, Severity: SeverityCritical}
		if remediationCommand(f) == "" {
			t.Errorf("%q can alarm but has no lever AND isn't guidance/info-only — every alarming finding needs a real action (ADR-033)", check)
		}
	}
}

// TestFindingChecksMatchEmittedFindings closes the registry loop: run the real
// doctor against the mock platform and assert every finding it emits carries a
// Check name declared in the registry. A check whose emits declaration is wrong
// or missing fails here — the list cannot silently rot again.
func TestFindingChecksMatchEmittedFindings(t *testing.T) {
	registered := map[string]bool{}
	for _, c := range FindingChecks() {
		registered[c] = true
	}

	report, err := DoctorWith(healthyMock())
	if err != nil {
		t.Fatalf("DoctorWith() error = %v", err)
	}
	if len(report.Findings) == 0 {
		t.Fatal("mock doctor emitted no findings — registry test has nothing to verify")
	}
	for _, f := range report.Findings {
		if !registered[f.Check] {
			t.Errorf("doctor emitted finding %q which is not declared in the doctorChecks registry (doctor.go) — declare it in the check's emits so ADR-033 enforcement covers it", f.Check)
		}
	}

	// The exemption list must not accumulate stale names either: every entry
	// must be a real, registered check.
	for check := range noLeverRequired {
		if !registered[check] {
			t.Errorf("noLeverRequired lists %q, which no registered check emits — remove the stale exemption", check)
		}
	}
}

// TestGuardrailCatchesMonitor is the self-proof: the guardrail flags a monitor
// verb and passes a real action — so the tests above have teeth.
func TestGuardrailCatchesMonitor(t *testing.T) {
	if !bannedFixCommands[firstToken("sirsi guard")] {
		t.Fatal("guardrail must flag 'sirsi guard' (a monitor) as banned")
	}
	if !bannedFixCommands[firstToken("sirsi monitor")] {
		t.Fatal("guardrail must flag 'sirsi monitor' as banned")
	}
	if bannedFixCommands[firstToken("sirsi relieve --memory")] {
		t.Fatal("guardrail must NOT flag a real action ('sirsi relieve --memory')")
	}
	if bannedFixCommands[firstToken("sirsi reclaim-snapshots")] {
		t.Fatal("guardrail must NOT flag a real action ('sirsi reclaim-snapshots')")
	}
}

// TestMemoryHogLever pins the owner-reported defect: a warn-level memory hog
// ("Top Memory Consumers") must map to the memory-relief lever with an honest
// relief classification, and the healthy (OK) reading must offer no command.
func TestMemoryHogLever(t *testing.T) {
	warn := DiagnosticFinding{
		Check:    "Top Memory Consumers",
		Severity: SeverityWarn,
		Message:  "Memory hog detected: Python at 6.2 GB",
		Detail:   "Python (6.2 GB) | node (1.1 GB) | Safari (900 MB)",
	}
	if got := remediationCommand(warn); got != "sirsi relieve --memory" {
		t.Errorf("remediationCommand(Top Memory Consumers/WARN) = %q, want %q", got, "sirsi relieve --memory")
	}
	if got := remediationKind(warn); got != FixRelief {
		t.Errorf("remediationKind(Top Memory Consumers/WARN) = %q, want %q", got, FixRelief)
	}

	ok := DiagnosticFinding{Check: "Top Memory Consumers", Severity: SeverityOK}
	if got := remediationCommand(ok); got != "" {
		t.Errorf("remediationCommand(Top Memory Consumers/OK) = %q, want none (healthy reading offers no fix)", got)
	}
}
