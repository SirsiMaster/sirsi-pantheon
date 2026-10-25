package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/guard"
	"github.com/SirsiMaster/sirsi-pantheon/internal/maat"
)

func TestMaatHostV1WarningRepairsRemainDeterministicWithoutCalibration(t *testing.T) {
	for _, tc := range []struct {
		name       string
		check      string
		fix        string
		route      guard.ResolutionRoute
		wantRepair string
	}{
		{"closed launchd fallback", maatRepairLaunchdDisabledCheck, "", guard.ResolutionRepair, maat.SystemOneRepairLaunchdDisabled},
		{"closed binary-drift updater", "binary-drift", "producer hint is not action authority", guard.ResolutionRepair, maat.SystemOneRepairBinaryDrift},
		// The real liveness warning producer supplies this Fix after doctor's
		// remediation pass. Its check name alone does not establish eligibility.
		{"closed producer-owned liveness", maatRepairLivenessWatchCheck, "sirsi liveness-watch install", guard.ResolutionRepair, maat.SystemOneRepairLivenessWatch},
		{"generic actionable warning", "RAM Pressure", "", guard.ResolutionRepair, ""},
		{"liveness missing fix", maatRepairLivenessWatchCheck, "", guard.ResolutionMaatReview, ""},
		{"liveness blank fix", maatRepairLivenessWatchCheck, " \t", guard.ResolutionMaatReview, ""},
		{"unknown review-only warning", "owned unknown check", "", guard.ResolutionMaatReview, ""},
		{"unknown fix is not a closed action", "owned unknown check", "owned inert fix text", guard.ResolutionRepair, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			finding := guard.DiagnosticFinding{Check: tc.check, Severity: guard.SeverityWarn, Message: "owned fixture warning", Fix: tc.fix}
			if got := guard.ResolutionFor(finding); got != tc.route {
				t.Fatalf("fixture route=%q want=%q", got, tc.route)
			}
			report := &guard.DoctorReport{Timestamp: time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC), Score: 92, Findings: []guard.DiagnosticFinding{finding}}
			s, _, err := maatHostSystemOneScreen(report, "fixture-host", 0)
			if err != nil {
				t.Fatal(err)
			}
			v, err := maat.Screen(s)
			if err != nil {
				t.Fatal(err)
			}
			if v.SchemaVersion != 1 || v.Contract != "" || v.Model.Calibration != "" || v.Gate != maat.GateChanges {
				t.Fatalf("host behavior changed: %+v", v)
			}
			if len(v.Findings) != 1 || v.Findings[0].RepairID != tc.wantRepair {
				t.Fatalf("host repair mapping=%+v want=%q", v.Findings, tc.wantRepair)
			}
		})
	}
}

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
	if screen.Subject.Kind != "host" || !strings.HasPrefix(screen.Subject.Repo, "local-host-") || !strings.HasPrefix(evidence, "diagnostic:sha256=") {
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
		repairs[finding.ID] = finding.RepairID
	}
	if repairs[maat.CanonicalHostFindingID(maatRepairLaunchdDisabledCheck)] != maat.SystemOneRepairLaunchdDisabled {
		t.Fatalf("disabled override repair = %q, want closed Ma'at repair", repairs[maat.CanonicalHostFindingID(maatRepairLaunchdDisabledCheck)])
	}
	if repairs[maat.CanonicalHostFindingID(maatRepairLivenessWatchCheck)] != maat.SystemOneRepairLivenessWatch {
		t.Fatalf("liveness repair = %q, want closed Ma'at repair", repairs[maat.CanonicalHostFindingID(maatRepairLivenessWatchCheck)])
	}
	if repairs[maat.CanonicalHostFindingID("Disk Space")] != "" {
		t.Fatalf("generic doctor Fix became executable repair id %q", repairs[maat.CanonicalHostFindingID("Disk Space")])
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
	home := t.TempDir()
	if err := os.Chmod(home, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	if _, err := maat.InitializeDefaultStore(); err != nil {
		t.Fatal(err)
	}
	oldFactory, oldRead, oldHost, oldConfirm, oldJSON, oldMaatJSON := newMaatDecisionJournal, maatTriageReadDiagnosis, maatTriageHostname, maatTriageConfirm, JsonOutput, maatJSON
	t.Cleanup(func() {
		newMaatDecisionJournal, maatTriageReadDiagnosis, maatTriageHostname = oldFactory, oldRead, oldHost
		maatTriageConfirm, JsonOutput, maatJSON = oldConfirm, oldJSON, oldMaatJSON
	})
	journal := &maat.FileDecisionJournal{Path: filepath.Join(home, "owned-diagnostics.jsonl")}
	newMaatDecisionJournal = func() (maat.DecisionJournal, error) { return journal, nil }
	maatTriageReadDiagnosis = func() (*guard.DoctorReport, error) {
		return &guard.DoctorReport{Timestamp: time.Now().UTC(), Score: 91, Findings: []guard.DiagnosticFinding{{Check: "RAM Pressure", Severity: guard.SeverityOK, Message: "healthy"}}}, nil
	}
	maatTriageHostname = func() (string, error) { return "m5", nil }
	JsonOutput, maatJSON = false, false

	maatTriageConfirm = false
	if err := maatTriageCmd.RunE(maatTriageCmd, nil); err != nil {
		t.Fatal(err)
	}
	if decisions, err := journal.Recent(100); err != nil || len(decisions) != 0 {
		t.Fatalf("preview wrote decisions: %#v %v", decisions, err)
	}

	maatTriageConfirm = true
	if err := maatTriageCmd.RunE(maatTriageCmd, nil); err != nil {
		t.Fatal(err)
	}
	if decisions, err := journal.Recent(100); err != nil || len(decisions) != 1 || decisions[0].SystemOne == nil || decisions[0].SystemOne.Subject.Kind != "host" || !strings.HasPrefix(decisions[0].OriginEvidence, "maat-diagnostic-record:") {
		t.Fatalf("recorded decisions = %#v %v", decisions, err)
	}
}
