package main

import (
	"strings"
	"testing"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/guard"
	"github.com/SirsiMaster/sirsi-pantheon/internal/maat"
)

func TestMaatHostSystemOneScreenMapsLiveDiagnosticsToClosedEvidence(t *testing.T) {
	report := &guard.DoctorReport{Timestamp: time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC), Score: 63, Findings: []guard.DiagnosticFinding{
		{Check: "Disk Space", Severity: guard.SeverityCritical, Message: "disk is critically full", Fix: "sirsi clean --include-caution"},
		{Check: "RAM Pressure", Severity: guard.SeverityWarn, Message: "memory pressure is elevated", Fix: "sirsi relieve --memory"},
		{Check: "Sirsi Processes", Severity: guard.SeverityOK, Message: "healthy"},
	}}
	screen, evidence, err := maatHostSystemOneScreen(report, "m5", 7)
	if err != nil {
		t.Fatal(err)
	}
	if screen.Subject.Kind != "host" || screen.Subject.Repo != "m5" || !strings.HasPrefix(evidence, "diagnostic:sha256=") {
		t.Fatalf("screen identity = %#v, evidence = %q", screen.Subject, evidence)
	}
	if len(screen.Findings) != 2 || screen.Findings[0].Severity != "block" || !strings.Contains(screen.Findings[0].FixHint, "Level 3") {
		t.Fatalf("screen findings = %#v", screen.Findings)
	}
	verdict, err := maat.Screen(screen)
	if err != nil {
		t.Fatal(err)
	}
	if verdict.Gate != maat.GateBlock {
		t.Fatalf("gate = %q, want block", verdict.Gate)
	}
}

func TestMaatHostSystemOneScreenRejectsMissingAuthority(t *testing.T) {
	if _, _, err := maatHostSystemOneScreen(nil, "m5", 0); err == nil {
		t.Fatal("nil diagnostic report accepted")
	}
	if _, _, err := maatHostSystemOneScreen(&guard.DoctorReport{}, "", 0); err == nil {
		t.Fatal("empty host accepted")
	}
}
