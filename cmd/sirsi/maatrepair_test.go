package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/SirsiMaster/sirsi-pantheon/internal/guard"
	"github.com/SirsiMaster/sirsi-pantheon/internal/maat"
	"github.com/SirsiMaster/sirsi-pantheon/internal/router"
)

func fixtureMaatLaunchdPreview(t *testing.T) router.ManagedLaunchdPreview {
	t.Helper()
	targets := []router.ManagedLaunchdPreviewTarget{{
		Label: "ai.sirsi.menubar", Plist: "ai.sirsi.menubar.plist",
		SHA256: strings.Repeat("a", 64), WasLoaded: true,
	}}
	canonical := struct {
		Schema  string                               `json:"schema"`
		Targets []router.ManagedLaunchdPreviewTarget `json:"targets"`
	}{Schema: "sirsi.maat.launchd-disabled-preview.v1", Targets: targets}
	raw, err := json.Marshal(canonical)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(raw)
	return router.ManagedLaunchdPreview{Schema: canonical.Schema, Targets: targets, Binding: hex.EncodeToString(sum[:])}
}

func TestMaatRepairLaunchdDisabledRecordsOnlyVerifiedRecovery(t *testing.T) {
	oldConfirm, oldBinding := maatRepairConfirm, maatRepairLaunchdPreviewBinding
	oldRead, oldPreview, oldRestore, oldJournal := maatRepairReadDiagnosis, maatRepairPreviewLaunchAgents, maatRepairRestoreReviewedLaunchAgents, newMaatDecisionJournal
	t.Cleanup(func() {
		maatRepairConfirm, maatRepairLaunchdPreviewBinding = oldConfirm, oldBinding
		maatRepairReadDiagnosis, maatRepairPreviewLaunchAgents, maatRepairRestoreReviewedLaunchAgents, newMaatDecisionJournal = oldRead, oldPreview, oldRestore, oldJournal
	})
	maatRepairConfirm = true
	preview := fixtureMaatLaunchdPreview(t)
	maatRepairLaunchdPreviewBinding = preview.Binding
	maatRepairPreviewLaunchAgents = func() (router.ManagedLaunchdPreview, error) { return preview, nil }
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
	maatRepairRestoreReviewedLaunchAgents = func(binding string) (router.ManagedLaunchdRecovery, error) {
		if binding != preview.Binding {
			t.Fatalf("restore binding = %q, want reviewed %q", binding, preview.Binding)
		}
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

func TestMaatRepairLaunchdDisabledPreviewIsReadOnly(t *testing.T) {
	oldConfirm, oldJSON, oldRead, oldPreview, oldRestore := maatRepairConfirm, maatJSON, maatRepairReadDiagnosis, maatRepairPreviewLaunchAgents, maatRepairRestoreReviewedLaunchAgents
	t.Cleanup(func() {
		maatRepairConfirm, maatJSON = oldConfirm, oldJSON
		maatRepairReadDiagnosis, maatRepairPreviewLaunchAgents, maatRepairRestoreReviewedLaunchAgents = oldRead, oldPreview, oldRestore
	})
	maatRepairConfirm = false
	maatJSON = true
	readCalled, restoreCalled := false, false
	maatRepairReadDiagnosis = func() (*guard.DoctorReport, error) {
		readCalled = true
		return nil, errors.New("diagnostic is not part of preview")
	}
	preview := fixtureMaatLaunchdPreview(t)
	maatRepairPreviewLaunchAgents = func() (router.ManagedLaunchdPreview, error) { return preview, nil }
	maatRepairRestoreReviewedLaunchAgents = func(string) (router.ManagedLaunchdRecovery, error) {
		restoreCalled = true
		return router.ManagedLaunchdRecovery{}, nil
	}
	err := maatRepairLaunchdDisabledCmd.RunE(maatRepairLaunchdDisabledCmd, nil)
	if err != nil || readCalled || restoreCalled {
		t.Fatalf("preview error=%v diagnostic=%v restore=%v", err, readCalled, restoreCalled)
	}
}

func TestMaatRepairLaunchdDisabledStaleBindingCannotReachDiagnosisOrActuation(t *testing.T) {
	oldConfirm, oldBinding, oldRead, oldPreview, oldRestore := maatRepairConfirm, maatRepairLaunchdPreviewBinding, maatRepairReadDiagnosis, maatRepairPreviewLaunchAgents, maatRepairRestoreReviewedLaunchAgents
	t.Cleanup(func() {
		maatRepairConfirm, maatRepairLaunchdPreviewBinding = oldConfirm, oldBinding
		maatRepairReadDiagnosis, maatRepairPreviewLaunchAgents, maatRepairRestoreReviewedLaunchAgents = oldRead, oldPreview, oldRestore
	})
	maatRepairConfirm = true
	maatRepairLaunchdPreviewBinding = strings.Repeat("b", 64)
	preview := fixtureMaatLaunchdPreview(t)
	maatRepairPreviewLaunchAgents = func() (router.ManagedLaunchdPreview, error) { return preview, nil }
	readCalled, restoreCalled := false, false
	maatRepairReadDiagnosis = func() (*guard.DoctorReport, error) { readCalled = true; return nil, nil }
	maatRepairRestoreReviewedLaunchAgents = func(string) (router.ManagedLaunchdRecovery, error) {
		restoreCalled = true
		return router.ManagedLaunchdRecovery{}, nil
	}
	err := maatRepairLaunchdDisabledCmd.RunE(maatRepairLaunchdDisabledCmd, nil)
	if err == nil || !strings.Contains(err.Error(), "preview changed") || readCalled || restoreCalled {
		t.Fatalf("stale confirmation error=%v diagnostic=%v restore=%v", err, readCalled, restoreCalled)
	}
}

func TestMaatRepairLivenessWatchExposesConfirmationFlag(t *testing.T) {
	if maatRepairLivenessWatchCmd.Flags().Lookup("confirm") == nil {
		t.Fatal("liveness-watch repair must expose its own --confirm flag for native and CLI callers")
	}
}

func TestMaatRepairLivenessWatchExposesOnlyClosedFailureMemoryActions(t *testing.T) {
	for _, name := range []string{"initialize-failure-memory", "inspect-failure-memory", "reobserve-incomplete"} {
		if maatRepairLivenessWatchCmd.Flags().Lookup(name) == nil {
			t.Errorf("liveness-watch repair is missing closed local action flag --%s", name)
		}
	}
	if maatRepairLivenessWatchCmd.Flags().Lookup("store") != nil {
		t.Fatal("liveness-watch failure-memory actions must not accept a caller-selected store path")
	}
}
