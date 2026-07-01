package guard

import (
	"strings"
	"testing"
)

// ADR-033 "Three-Outcome Law" enforced by test: every finding resolves to a real
// ACTION, honest GUIDANCE, or plain INFO — never a passive monitor. These tests
// are the guardrail so the "tapping Fix opened a dashboard" bug can't come back.

// bannedFixCommands are passive/monitor verbs that must NEVER be a finding's fix —
// they observe, they don't remediate. (`sirsi guard` == `sirsi monitor` was the
// original offender.)
var bannedFixCommands = map[string]bool{
	"guard": true, "monitor": true, "status": true,
	"diagnose": true, "scan": true, "watch": true,
}

// allFindingChecks is the canonical set of finding Check names the doctor emits.
// Keep in sync with the checks in DoctorWithOpts (a new finding must be added
// here, which forces its author to declare its remediation contract below).
var allFindingChecks = []string{
	"RAM Pressure", "Swap Usage", "Memory Processes", "Disk Space",
	"Spotlight Storm", "Kernel Panics (7d)", "Jetsam Events (7d)",
	"App Crashes (7d)", "App Hangs (7d)", "Thread Leaks", "binary-drift",
	"Local Snapshots", "Sirsi Processes", "Crash Logs",
}

// noLeverRequired findings legitimately have no required one-click lever — either
// GUIDANCE-only (name the cause + manual steps) or INFO-only (never alarms). An
// empty remediationCommand is correct for them. Anything NOT here that can alarm
// MUST have a real lever.
var noLeverRequired = map[string]bool{
	"Kernel Panics (7d)": true, // guidance: hardware/driver — nothing safe to auto-do
	"Crash Logs":         true, // info: raw log pointer
	"Sirsi Processes":    true, // info
	"Local Snapshots":    true, // info: always SeverityInfo; carries an OPTIONAL reclaim (added with the reclaim lever), never alarms
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
	for _, check := range allFindingChecks {
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
func TestEveryAlarmingCheckHasLeverOrGuidance(t *testing.T) {
	for _, check := range allFindingChecks {
		if noLeverRequired[check] {
			continue
		}
		f := DiagnosticFinding{Check: check, Severity: SeverityCritical}
		if remediationCommand(f) == "" {
			t.Errorf("%q can alarm but has no lever AND isn't guidance/info-only — every alarming finding needs a real action (ADR-033)", check)
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
