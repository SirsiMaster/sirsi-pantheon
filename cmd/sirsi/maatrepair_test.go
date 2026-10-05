package main

import (
	"errors"
	"strings"
	"testing"

	"github.com/SirsiMaster/sirsi-pantheon/internal/guard"
	"github.com/SirsiMaster/sirsi-pantheon/internal/maat"
	"github.com/SirsiMaster/sirsi-pantheon/internal/router"
)

func TestMaatRepairLaunchdDisabledRecordsOnlyVerifiedRecovery(t *testing.T) {
	oldConfirm, oldRead, oldRestore, oldJournal := maatRepairConfirm, maatRepairReadDiagnosis, maatRepairRestoreLaunchAgents, newMaatDecisionJournal
	t.Cleanup(func() {
		maatRepairConfirm, maatRepairReadDiagnosis, maatRepairRestoreLaunchAgents, newMaatDecisionJournal = oldConfirm, oldRead, oldRestore, oldJournal
	})
	maatRepairConfirm = true
	reads := 0
	maatRepairReadDiagnosis = func() (*guard.DoctorReport, error) {
		reads++
		if reads == 1 {
			return &guard.DoctorReport{Findings: []guard.DiagnosticFinding{{
				Check: maatRepairLaunchdDisabledCheck, Severity: guard.SeverityCritical, Message: "two managed labels are disabled",
			}}}, nil
		}
		return &guard.DoctorReport{Findings: []guard.DiagnosticFinding{{
			Check: maatRepairLaunchdDisabledCheck, Severity: guard.SeverityOK, Message: "no recoverable managed labels are disabled",
		}}}, nil
	}
	maatRepairRestoreLaunchAgents = func() (router.ManagedLaunchdRecovery, error) {
		return router.ManagedLaunchdRecovery{Enabled: []string{"ai.sirsi.menubar"}, Bootstrapped: []string{"ai.sirsi.menubar"}}, nil
	}
	journal := &maatTestJournal{}
	newMaatDecisionJournal = func() (maat.DecisionJournal, error) { return journal, nil }
	if err := maatRepairLaunchdDisabledCmd.RunE(maatRepairLaunchdDisabledCmd, nil); err != nil {
		t.Fatal(err)
	}
	if reads != 2 {
		t.Fatalf("diagnostic reads = %d, want preflight plus post-repair verification", reads)
	}
	if len(journal.decisions) != 1 {
		t.Fatalf("repair receipt count = %d, want 1", len(journal.decisions))
	}
	got := journal.decisions[0]
	if got.Kind != "diagnostic verified repair" || got.Determination != "resolved" || !strings.HasPrefix(got.Evidence, "diagnostic-repair:sha256=") {
		t.Fatalf("repair receipt = %#v", got)
	}
}

func TestMaatRepairLaunchdDisabledRequiresConfirmation(t *testing.T) {
	oldConfirm, oldRead, oldRestore := maatRepairConfirm, maatRepairReadDiagnosis, maatRepairRestoreLaunchAgents
	t.Cleanup(func() {
		maatRepairConfirm, maatRepairReadDiagnosis, maatRepairRestoreLaunchAgents = oldConfirm, oldRead, oldRestore
	})
	maatRepairConfirm = false
	called := false
	maatRepairReadDiagnosis = func() (*guard.DoctorReport, error) { called = true; return nil, errors.New("should not read") }
	maatRepairRestoreLaunchAgents = func() (router.ManagedLaunchdRecovery, error) {
		called = true
		return router.ManagedLaunchdRecovery{}, nil
	}
	err := maatRepairLaunchdDisabledCmd.RunE(maatRepairLaunchdDisabledCmd, nil)
	if err == nil || !strings.Contains(err.Error(), "--confirm") || called {
		t.Fatalf("confirmation error = %v, called=%v", err, called)
	}
}

func TestMaatRepairLivenessWatchRecordsOnlyVerifiedInstallation(t *testing.T) {
	oldConfirm, oldRead, oldInstall, oldJournal := maatRepairConfirm, maatRepairReadDiagnosis, maatRepairInstallLivenessWatch, newMaatDecisionJournal
	t.Cleanup(func() {
		maatRepairConfirm, maatRepairReadDiagnosis, maatRepairInstallLivenessWatch, newMaatDecisionJournal = oldConfirm, oldRead, oldInstall, oldJournal
	})
	maatRepairConfirm = true
	reads := 0
	maatRepairReadDiagnosis = func() (*guard.DoctorReport, error) {
		reads++
		severity, message := guard.SeverityWarn, "launchd liveness watch not installed"
		if reads > 1 {
			severity, message = guard.SeverityOK, "launchd liveness watch installed"
		}
		return &guard.DoctorReport{Findings: []guard.DiagnosticFinding{{Check: maatRepairLivenessWatchCheck, Severity: severity, Message: message}}}, nil
	}
	maatRepairInstallLivenessWatch = func() (string, error) { return "liveness watch installed and started", nil }
	journal := &maatTestJournal{}
	newMaatDecisionJournal = func() (maat.DecisionJournal, error) { return journal, nil }
	if err := maatRepairLivenessWatchCmd.RunE(maatRepairLivenessWatchCmd, nil); err != nil {
		t.Fatal(err)
	}
	if reads != 2 || len(journal.decisions) != 1 {
		t.Fatalf("reads=%d decisions=%d, want preflight/postflight and one receipt", reads, len(journal.decisions))
	}
	got := journal.decisions[0]
	if got.Determination != "resolved" || !strings.Contains(got.Why, "liveness") {
		t.Fatalf("liveness receipt = %#v", got)
	}
}

func TestMaatRepairLivenessWatchRequiresConfirmation(t *testing.T) {
	oldConfirm, oldRead, oldInstall := maatRepairConfirm, maatRepairReadDiagnosis, maatRepairInstallLivenessWatch
	t.Cleanup(func() {
		maatRepairConfirm, maatRepairReadDiagnosis, maatRepairInstallLivenessWatch = oldConfirm, oldRead, oldInstall
	})
	maatRepairConfirm = false
	called := false
	maatRepairReadDiagnosis = func() (*guard.DoctorReport, error) { called = true; return nil, errors.New("should not read") }
	maatRepairInstallLivenessWatch = func() (string, error) { called = true; return "", nil }
	err := maatRepairLivenessWatchCmd.RunE(maatRepairLivenessWatchCmd, nil)
	if err == nil || !strings.Contains(err.Error(), "--confirm") || called {
		t.Fatalf("confirmation error = %v, called=%v", err, called)
	}
}

func TestMaatRepairLivenessWatchExposesConfirmationFlag(t *testing.T) {
	if maatRepairLivenessWatchCmd.Flags().Lookup("confirm") == nil {
		t.Fatal("liveness-watch repair must expose its own --confirm flag for native and CLI callers")
	}
}
