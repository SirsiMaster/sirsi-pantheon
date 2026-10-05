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

func TestMaatHostSystemOneScreenMapsKnownHostRepairsToClosedActions(t *testing.T) {
	report := &guard.DoctorReport{Timestamp: time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC), Score: 63, Findings: []guard.DiagnosticFinding{
		{Check: maatRepairLaunchdDisabledCheck, Severity: guard.SeverityCritical, Message: "managed labels are disabled", Fix: "bounded native recovery"},
		{Check: maatRepairLivenessWatchCheck, Severity: guard.SeverityWarn, Message: "liveness watch is absent", Fix: "sirsi liveness-watch install"},
		{Check: "Disk Space", Severity: guard.SeverityCritical, Message: "disk is full", Fix: "sirsi clean --include-caution"},
	}}
	screen, _, err := maatHostSystemOneScreen(report, "m5", 7)
	if err != nil {
		t.Fatal(err)
	}
	repairs := map[string]string{}
	for _, finding := range screen.Findings {
		repairs[finding.Claim] = finding.RepairID
	}
	if repairs["managed labels are disabled"] != maat.SystemOneRepairLaunchdDisabled {
		t.Fatalf("disabled override repair = %q, want closed Ma'at repair", repairs["managed labels are disabled"])
	}
	if repairs["liveness watch is absent"] != maat.SystemOneRepairLivenessWatch {
		t.Fatalf("liveness repair = %q, want closed Ma'at repair", repairs["liveness watch is absent"])
	}
	if repairs["disk is full"] != "" {
		t.Fatalf("generic doctor Fix became executable repair id %q", repairs["disk is full"])
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

func TestMaatTriageRecordsOnlyAfterConfirmation(t *testing.T) {
	oldFactory, oldRead, oldHost, oldConfirm, oldJSON, oldMaatJSON := newMaatDecisionJournal, maatTriageReadDiagnosis, maatTriageHostname, maatTriageConfirm, JsonOutput, maatJSON
	t.Cleanup(func() {
		newMaatDecisionJournal, maatTriageReadDiagnosis, maatTriageHostname = oldFactory, oldRead, oldHost
		maatTriageConfirm, JsonOutput, maatJSON = oldConfirm, oldJSON, oldMaatJSON
	})
	journal := &maatTestJournal{}
	newMaatDecisionJournal = func() (maat.DecisionJournal, error) { return journal, nil }
	maatTriageReadDiagnosis = func() (*guard.DoctorReport, error) {
		return &guard.DoctorReport{Timestamp: time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC), Score: 91, Findings: []guard.DiagnosticFinding{{Check: "RAM Pressure", Severity: guard.SeverityOK, Message: "healthy"}}}, nil
	}
	maatTriageHostname = func() (string, error) { return "m5", nil }
	JsonOutput, maatJSON = false, false

	maatTriageConfirm = false
	if err := maatTriageCmd.RunE(maatTriageCmd, nil); err != nil {
		t.Fatal(err)
	}
	if len(journal.decisions) != 0 {
		t.Fatalf("preview wrote decisions: %#v", journal.decisions)
	}

	maatTriageConfirm = true
	if err := maatTriageCmd.RunE(maatTriageCmd, nil); err != nil {
		t.Fatal(err)
	}
	if len(journal.decisions) != 1 || journal.decisions[0].SystemOne == nil || journal.decisions[0].SystemOne.Subject.Kind != "host" {
		t.Fatalf("recorded decisions = %#v", journal.decisions)
	}
}
