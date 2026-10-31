package main

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/SirsiMaster/sirsi-pantheon/internal/maat"
	"github.com/spf13/cobra"
)

func localMemoryCommandFixture(t *testing.T) (*maat.Store, maat.LivenessFailureView, string) {
	t.Helper()
	home := t.TempDir()
	if err := os.Chmod(home, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	uuid, err := maat.InitializeDefaultStore()
	if err != nil {
		t.Fatal(err)
	}
	s, err := maat.OpenDefaultStore()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	j, err := maat.NewDefaultDecisionJournal()
	if err != nil {
		t.Fatal(err)
	}
	m := fixtureCommandManifest(uuid, "inert closed local installation")
	r, err := s.PreflightLivenessWatch(m)
	if err != nil {
		t.Fatal(err)
	}
	h, err := s.RetainPreflightReceipt(r)
	if err != nil {
		t.Fatal(err)
	}
	if err := maat.ProjectFailureMemoryPreflight(j, h); err != nil {
		t.Fatal(err)
	}
	a, err := s.BeginLivenessWatchAttempt(m, r)
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	result, err := s.RunLivenessWatchDirectAttempt(a, func(maat.LivenessWatchManifest) (string, error) {
		calls++
		return "inert fixture install returned successfully", nil
	}, func(m maat.LivenessWatchManifest) (maat.LivenessWatchObservation, error) {
		return maat.LivenessWatchObservation{UID: m.UID, Domain: m.Domain, Label: m.ManagedLabel,
			PlistPath: m.TargetPlistPath, PlistSHA256: m.ExpectedPlistSHA256, Postcondition: "known-unloaded"}, nil
	})
	if err != nil || calls != 1 {
		t.Fatalf("completion: %v calls=%d", err, calls)
	}
	intake, err := s.RememberLivenessFailure(a.ID)
	if err != nil {
		t.Fatal(err)
	}
	decision, err := maat.LivenessCompletionDecision(result)
	if err != nil {
		t.Fatal(err)
	}
	debt, err := s.BeginLivenessWatchProjection(a, result.Outcome, decision)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.PersistLivenessWatchProjectionOutcome(debt); err != nil {
		t.Fatal(err)
	}
	if err := j.AppendIdempotent(debt.Decision); err != nil {
		t.Fatal(err)
	}
	if err := s.MarkLivenessWatchProjectionReadBack(debt); err != nil {
		t.Fatal(err)
	}
	r, err = s.Preflight(maat.Scope{Component: "pantheon", Profile: "local-macos", Operation: "install-managed-liveness-watch"})
	if err != nil || r.Decision != maat.PreflightReject {
		t.Fatalf("remember/prevent: %+v %v", r, err)
	}
	h, err = s.RetainPreflightReceipt(r)
	if err != nil {
		t.Fatal(err)
	}
	if err := maat.ProjectFailureMemoryPreflight(j, h); err != nil {
		t.Fatal(err)
	}
	evidence, err := r.EvidenceReference()
	if err != nil {
		t.Fatal(err)
	}
	view, err := s.ReadLocalPreventionDisposition(intake.IncidentKey)
	if err != nil {
		t.Fatal(err)
	}
	return s, view, evidence
}

func runLocalMemoryFixtureCommand(t *testing.T, command *cobra.Command, args ...string) (string, error) {
	t.Helper()
	command.SetArgs(args)
	command.SilenceUsage, command.SilenceErrors = true, true
	var actionErr error
	output := captureStdout(t, func() { actionErr = command.Execute() })
	return output, actionErr
}

func TestLocalOwnerDispositionCLIUsesActualDefaultJournalAndSeparateConfirmedEffect(t *testing.T) {
	s, view, evidence := localMemoryCommandFixture(t)
	output, err := runLocalMemoryFixtureCommand(t, newMaatLocalFailureReadCmd(), "--incident-key", view.Incident.Key, "--json")
	var read maat.LivenessFailureView
	if err != nil || json.Unmarshal([]byte(output), &read) != nil || read.Head.Key != view.Head.Key || read.ActionAuthorized {
		t.Fatalf("actual failure read: %s %v", output, err)
	}
	args := []string{"preview", "--incident-key", view.Incident.Key, "--occurrence-key", view.Head.Key,
		"--body-sha256", view.Head.BodySHA256, "--case-evidence", evidence, "--effect", "retire-local-prevention",
		"--conclusion", "Owner accepts this exact known unhealthy occurrence", "--json"}
	output, err = runLocalMemoryFixtureCommand(t, newMaatLocalDispositionCmd(), args...)
	var preview maat.LocalPreventionDispositionPreview
	if err != nil || json.Unmarshal([]byte(output), &preview) != nil || !preview.CanCommit {
		t.Fatalf("actual preview: %s %v", output, err)
	}
	if _, err := runLocalMemoryFixtureCommand(t, newMaatLocalDispositionCmd(), "commit", "--plan-sha256", preview.PlanSHA256, "--json"); err == nil {
		t.Fatal("missing explicit consent changed memory")
	}
	if current, err := s.ReadLocalPreventionDisposition(view.Incident.Key); err != nil || current.Head.Status != maat.IncidentActive {
		t.Fatalf("preview/unconfirmed commit retired prevention: %+v %v", current, err)
	}
	output, err = runLocalMemoryFixtureCommand(t, newMaatLocalDispositionCmd(), "commit", "--plan-sha256", preview.PlanSHA256, "--confirm", "--json")
	var receipt maat.LocalPreventionDispositionReceipt
	if err != nil || json.Unmarshal([]byte(output), &receipt) != nil || receipt.EventKey == "" || receipt.ActionAuthorized {
		t.Fatalf("actual confirmed receipt: %s %v", output, err)
	}
	repeated, err := runLocalMemoryFixtureCommand(t, newMaatLocalDispositionCmd(), "reconcile", "--plan-sha256", preview.PlanSHA256, "--json")
	if err != nil || repeated != output {
		t.Fatalf("exact historical reconciliation changed bytes: %v", err)
	}
	current, err := s.ReadLocalPreventionDisposition(view.Incident.Key)
	if err != nil || current.Head.Status != maat.IncidentRetired || current.Health != "historical-known-unhealthy; current-health-unobserved" {
		t.Fatalf("owner disposition misrepresented repair/health: %+v %v", current, err)
	}
}

func TestLocalOwnerDispositionCLIRefusesRawAuthorityAndForeignOperands(t *testing.T) {
	_, view, _ := localMemoryCommandFixture(t)
	for _, args := range [][]string{
		{"commit", "--plan-sha256", strings.Repeat("f", 64), "--confirm", "--json"},
		{"commit", "--plan-sha256", strings.Repeat("f", 64), "--approved"},
		{"reconcile", "--plan-sha256", strings.Repeat("f", 64), "--json"},
		{"pending", "--incident-key", view.Incident.Key, "--store", "/tmp/foreign"},
	} {
		if _, err := runLocalMemoryFixtureCommand(t, newMaatLocalDispositionCmd(), args...); err == nil {
			t.Fatalf("raw/unissued authority accepted: %v", args)
		}
	}
}
