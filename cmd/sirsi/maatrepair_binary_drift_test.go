package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/guard"
	"github.com/SirsiMaster/sirsi-pantheon/internal/maat"
	"github.com/SirsiMaster/sirsi-pantheon/internal/selfupdate"
	modversion "github.com/SirsiMaster/sirsi-pantheon/internal/version"
)

func (j *maatTestJournal) AppendIdempotent(d maat.Decision) error {
	_, err := j.AppendOnce(d)
	return err
}

func (j *maatTestJournal) FindByEvidence(kind, evidence string) (maat.Decision, bool, error) {
	for _, d := range j.decisions {
		if d.Kind == kind && d.Evidence == evidence {
			return d, true, nil
		}
	}
	return maat.Decision{}, false, nil
}

type concurrentBinaryDriftJournal struct {
	mu   sync.Mutex
	rows []maat.Decision
}

func (j *concurrentBinaryDriftJournal) Append(d maat.Decision) error {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.rows = append(j.rows, d)
	return nil
}
func (j *concurrentBinaryDriftJournal) AppendOnce(d maat.Decision) (bool, error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	for _, row := range j.rows {
		if row.Kind == d.Kind && row.Evidence == d.Evidence {
			return false, nil
		}
	}
	j.rows = append(j.rows, d)
	return true, nil
}
func (j *concurrentBinaryDriftJournal) AppendIdempotent(d maat.Decision) error {
	_, err := j.AppendOnce(d)
	return err
}
func (j *concurrentBinaryDriftJournal) Recent(limit int) ([]maat.Decision, error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	if limit <= 0 || limit > len(j.rows) {
		limit = len(j.rows)
	}
	return append([]maat.Decision(nil), j.rows[len(j.rows)-limit:]...), nil
}
func (j *concurrentBinaryDriftJournal) FindByEvidence(kind, evidence string) (maat.Decision, bool, error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	for _, row := range j.rows {
		if row.Kind == kind && row.Evidence == evidence {
			return row, true, nil
		}
	}
	return maat.Decision{}, false, nil
}

func installMaatBinaryDriftFixture(t *testing.T) (*maatTestJournal, string, string, string) {
	t.Helper()
	oldHostname := maatTriageHostname
	maatTriageHostname = func() (string, error) { return "fixture-host", nil }
	t.Cleanup(func() { maatTriageHostname = oldHostname })

	journal := &maatTestJournal{}
	report := &guard.DoctorReport{Timestamp: time.Date(2026, 10, 24, 12, 0, 0, 0, time.UTC), Findings: []guard.DiagnosticFinding{{
		Check: maatRepairBinaryDriftCheck, Severity: guard.SeverityWarn, Message: "Sirsi binary drift detected",
	}}}
	screen, _, err := maatHostSystemOneScreen(report, "fixture-host", 1)
	if err != nil {
		t.Fatal(err)
	}
	verdict, err := maat.Screen(screen)
	if err != nil {
		t.Fatal(err)
	}
	decision, err := maat.RecordSystemOne(journal, "arbitrary requester label", verdict)
	if err != nil {
		t.Fatal(err)
	}

	root := t.TempDir()
	self := filepath.Join(root, "pantheon-sirsi")
	target := filepath.Join(root, "homebrew-sirsi")
	if err := os.WriteFile(self, []byte("fixture self binary"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte("fixture target binary"), 0o700); err != nil {
		t.Fatal(err)
	}
	return journal, decision.Evidence, self, target
}

func appendBinaryDriftMemoryIncident(t *testing.T, guardID, guardVersion string) {
	t.Helper()
	store, err := maat.OpenDefaultStore()
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	evidence, err := store.PutEvidence([]byte("binary-drift recurrence regression evidence"))
	if err != nil {
		t.Fatal(err)
	}
	sig := maat.FailureSignature{Namespace: "pantheon", Version: "v1", FailureClass: "binary-drift", Operation: "update", Component: "pantheon", Invariant: "homebrew-target-update-is-authorized", ErrorCode: "binary-drift-active"}
	key, err := sig.Digest()
	if err != nil {
		t.Fatal(err)
	}
	incident := maat.Incident{Schema: maat.MemorySchema, Key: key, Signature: sig, EvidenceSHA256: evidence, Scope: maat.Scope{Component: "pantheon", Profile: "local-macos", Operation: "update-homebrew-binary-drift"}, GuardID: guardID, GuardVersion: guardVersion, Status: maat.IncidentActive,
		RecoveryActions: []maat.RecoveryAction{{ID: "review-binary-update", Label: "Review binary update", Instruction: "Refresh the exact screen and review a new bounded plan."}}}
	if err := store.Append(incident); err != nil {
		t.Fatal(err)
	}
}

func installMaatBinaryDriftSeams(t *testing.T, journal *maatTestJournal, self, target string,
	diagnosis func() (*guard.DoctorReport, error), drift func() (*selfupdate.DriftReport, error)) *int {
	t.Helper()
	oldCommandContext := maatRepairBinaryDriftCmd.Context()
	if oldCommandContext == nil {
		maatRepairBinaryDriftCmd.SetContext(context.Background())
	}
	oldConfirm := maatRepairConfirm
	oldReconcile := maatRepairBinaryDriftReconcile
	oldEvidence, oldBinding := maatRepairBinaryDriftCaseEvidence, maatRepairBinaryDriftBinding
	oldDiagnosis, oldScan, oldScanContext, oldFinding := maatRepairReadDiagnosis, maatRepairScanDrift, maatRepairScanDriftContext, maatRepairReadBinaryFinding
	oldHash, oldMethod := maatRepairHashFile, maatRepairDetectMethod
	oldFind, oldRun, oldJournal := maatRepairFindHomebrew, maatRepairRunHomebrewUpdate, newMaatDecisionJournal
	oldOpenStore, oldSchema := maatRepairOpenBinaryDriftStore, maatRepairBinarySchemaObservation
	oldRunningInfo := maatRepairRunningBinaryInfo
	oldInspect := maatRepairDefaultStoreInspect
	home := t.TempDir()
	t.Setenv("HOME", home)
	if _, err := maat.InitializeDefaultStore(); err != nil {
		t.Fatalf("initialize isolated memory store: %v", err)
	}
	maatRepairDefaultStoreInspect = maat.InspectDefaultStore
	maatRepairOpenBinaryDriftStore = func() (maatBinaryDriftStore, error) { return maat.OpenDefaultStore() }
	maatRepairBinarySchemaObservation = func(modversion.Info) (int, error) { return 1, nil }
	runningInfo := currentMaatRunningBinaryInfo()
	runningInfo.Path, runningInfo.Version = self, "2.0.0"
	maatRepairRunningBinaryInfo = func() modversion.Info { return runningInfo }
	updateCalls := 0
	maatRepairConfirm = false
	maatRepairBinaryDriftReconcile = false
	maatRepairBinaryDriftCaseEvidence, maatRepairBinaryDriftBinding = "", ""
	maatRepairReadDiagnosis = diagnosis
	maatRepairScanDrift = drift
	maatRepairScanDriftContext = func(ctx context.Context) (*selfupdate.DriftReport, error) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		return drift()
	}
	maatRepairReadBinaryFinding = func(ctx context.Context) (guard.DiagnosticFinding, error) {
		if err := ctx.Err(); err != nil {
			return guard.DiagnosticFinding{}, err
		}
		report, err := drift()
		if err != nil {
			return guard.DiagnosticFinding{}, err
		}
		return pantheonBinaryDriftFinding(report), nil
	}
	maatRepairHashFile = selfupdate.FileHash
	maatRepairDetectMethod = func(string) selfupdate.Method { return selfupdate.MethodHomebrew }
	maatRepairFindHomebrew = func() (string, error) { return self, nil }
	maatRepairRunHomebrewUpdate = func(_ context.Context, path string) error {
		if path != self {
			t.Errorf("updater path = %q, want exact preview-bound executable %q", path, self)
		}
		updateCalls++
		return nil
	}
	newMaatDecisionJournal = func() (maat.DecisionJournal, error) { return journal, nil }
	t.Cleanup(func() {
		maatRepairBinaryDriftCmd.SetContext(oldCommandContext)
		maatRepairConfirm = oldConfirm
		maatRepairBinaryDriftReconcile = oldReconcile
		maatRepairBinaryDriftCaseEvidence, maatRepairBinaryDriftBinding = oldEvidence, oldBinding
		maatRepairReadDiagnosis, maatRepairScanDrift, maatRepairScanDriftContext, maatRepairReadBinaryFinding = oldDiagnosis, oldScan, oldScanContext, oldFinding
		maatRepairHashFile, maatRepairDetectMethod = oldHash, oldMethod
		maatRepairFindHomebrew, maatRepairRunHomebrewUpdate, newMaatDecisionJournal = oldFind, oldRun, oldJournal
		maatRepairOpenBinaryDriftStore, maatRepairBinarySchemaObservation = oldOpenStore, oldSchema
		maatRepairRunningBinaryInfo = oldRunningInfo
		maatRepairDefaultStoreInspect = oldInspect
	})
	return &updateCalls
}

func TestObserveBinaryDriftSchemaCompatibilityUsesRealRunningCeiling(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "router.db")
	if err := os.WriteFile(dbPath, []byte("controlled existing database fixture"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SIRSI_ROUTER_DB", dbPath)
	info := currentMaatRunningBinaryInfo()
	if info.RouterSchemaMax <= 0 {
		t.Fatalf("running binary has no real router schema ceiling: %+v", info)
	}
	oldReadSchema := readSchemaFn
	t.Cleanup(func() { readSchemaFn = oldReadSchema })
	reads := 0
	readSchemaFn = func(path string) (int, error) {
		reads++
		if path != dbPath {
			t.Fatalf("schema observer path=%q, want controlled default database %q", path, dbPath)
		}
		return info.RouterSchemaMax, nil
	}
	live, err := observeBinaryDriftSchemaCompatibility(info)
	if err != nil || live != info.RouterSchemaMax || reads != 1 {
		t.Fatalf("supported live schema=%d err=%v reads=%d ceiling=%d", live, err, reads, info.RouterSchemaMax)
	}
	readSchemaFn = func(string) (int, error) { return info.RouterSchemaMax + 1, nil }
	if _, err := observeBinaryDriftSchemaCompatibility(info); !errors.Is(err, selfupdate.ErrSchemaIncompatible) {
		t.Fatalf("live schema above the actual running ceiling error=%v", err)
	}
}

func maatBinaryDriftFixtureReport(active bool) *guard.DoctorReport {
	severity, message := guard.SeverityOK, "Sirsi binaries in sync"
	if active {
		severity, message = guard.SeverityWarn, "Sirsi binary drift detected"
	}
	return &guard.DoctorReport{Findings: []guard.DiagnosticFinding{{
		Check: maatRepairBinaryDriftCheck, Severity: severity, Message: message,
	}}}
}

func maatBinaryDriftFixtureInventory(self, target string, active bool) *selfupdate.DriftReport {
	report := &selfupdate.DriftReport{Self: modversion.Info{Binary: "sirsi", Version: "2.0.0", Path: self}, Healthy: !active}
	report.Siblings = []selfupdate.Sibling{{Info: modversion.Info{Binary: "sirsi", Version: "2.0.0", Path: target}, Method: selfupdate.MethodHomebrew}}
	if active {
		report.Siblings[0].Version = "1.0.0"
		report.D2Mismatch = []selfupdate.Sibling{report.Siblings[0]}
	}
	return report
}

func TestMaatBinaryDriftPostcheckRequiresEveryReviewedTargetObservation(t *testing.T) {
	_, _, self, target := installMaatBinaryDriftFixture(t)
	preview := []maatBinaryDriftTarget{{Path: target}}
	if !maatBinaryDriftTargetsObserved(preview, maatBinaryDriftFixtureInventory(self, target, false)) {
		t.Fatal("complete fresh target observation was not accepted")
	}
	for name, mutate := range map[string]func(*selfupdate.DriftReport){
		"missing":         func(report *selfupdate.DriftReport) { report.Siblings = nil },
		"probe error":     func(report *selfupdate.DriftReport) { report.Siblings[0].Err = "probe failed" },
		"unknown version": func(report *selfupdate.DriftReport) { report.Siblings[0].Version = "dev" },
	} {
		t.Run(name, func(t *testing.T) {
			report := maatBinaryDriftFixtureInventory(self, target, false)
			mutate(report)
			if maatBinaryDriftTargetsObserved(preview, report) {
				t.Fatalf("incomplete target report accepted: %+v", report)
			}
		})
	}
}

func TestMaatBinaryDriftPostcheckRequiresUnambiguousClearDiagnosis(t *testing.T) {
	if maatBinaryDriftDiagnosisClear(nil) || !maatBinaryDriftDiagnosisClear(&guard.DoctorReport{}) {
		t.Fatal("nil diagnosis or a complete finding-free report had the wrong clear status")
	}
	if !maatBinaryDriftDiagnosisClear(maatBinaryDriftFixtureReport(false)) || maatBinaryDriftDiagnosisClear(maatBinaryDriftFixtureReport(true)) {
		t.Fatal("single non-actionable or active binary-drift finding had the wrong clear status")
	}
	duplicate := maatBinaryDriftFixtureReport(false)
	duplicate.Findings = append(duplicate.Findings, duplicate.Findings[0])
	if maatBinaryDriftDiagnosisClear(duplicate) {
		t.Fatal("ambiguous duplicate diagnostic findings were accepted as a clear postcheck")
	}
}

func TestMaatBinaryDriftObservationHonorsCallerCancellation(t *testing.T) {
	old := maatRepairScanDriftContext
	maatRepairScanDriftContext = func(ctx context.Context) (*selfupdate.DriftReport, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	t.Cleanup(func() { maatRepairScanDriftContext = old })
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	_, err := readMaatBinaryDriftFinding(ctx)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("stalled binary-only observation error = %v, want deadline", err)
	}
}

func TestMaatBinaryDriftPreviewBindsOnlyReviewedHomebrewTargets(t *testing.T) {
	journal, evidence, self, target := installMaatBinaryDriftFixture(t)
	diagnosis := func() (*guard.DoctorReport, error) { return maatBinaryDriftFixtureReport(true), nil }
	inventory := func() (*selfupdate.DriftReport, error) {
		return maatBinaryDriftFixtureInventory(self, target, true), nil
	}
	installMaatBinaryDriftSeams(t, journal, self, target, diagnosis, inventory)
	if err := validateMaatBinaryDriftCase(journal, evidence); err != nil {
		t.Fatal(err)
	}
	preview, err := currentMaatBinaryDriftPreview(evidence)
	if err != nil {
		t.Fatal(err)
	}
	if preview.CaseEvidence != evidence || preview.Updater != "brew upgrade sirsi-pantheon" || preview.UpdaterPath != self || preview.UpdaterSHA256 == "" || len(preview.Targets) != 1 || preview.Targets[0].Path != target || preview.Targets[0].Method != "homebrew" || preview.Binding == "" {
		t.Fatalf("typed updater preview = %+v", preview)
	}
	if err := os.WriteFile(target, []byte("changed after preview"), 0o700); err != nil {
		t.Fatal(err)
	}
	changed, err := currentMaatBinaryDriftPreview(evidence)
	if err != nil {
		t.Fatal(err)
	}
	if changed.Binding == preview.Binding {
		t.Fatal("target bytes changed but preview binding stayed the same")
	}
	maatRepairFindHomebrew = func() (string, error) { return "brew", nil }
	if _, err := currentMaatBinaryDriftPreview(evidence); err == nil || !strings.Contains(err.Error(), "not absolute") {
		t.Fatalf("relative updater path preview error = %v", err)
	}
}

func TestMaatBinaryDriftRefusesUnmanagedTarget(t *testing.T) {
	journal, evidence, self, target := installMaatBinaryDriftFixture(t)
	read := func() (*guard.DoctorReport, error) { return maatBinaryDriftFixtureReport(true), nil }
	scan := func() (*selfupdate.DriftReport, error) {
		return maatBinaryDriftFixtureInventory(self, target, true), nil
	}
	installMaatBinaryDriftSeams(t, journal, self, target, read, scan)
	maatRepairScanDrift = func() (*selfupdate.DriftReport, error) {
		report := maatBinaryDriftFixtureInventory(self, target, true)
		report.D2Mismatch[0].Method = selfupdate.MethodRaw
		return report, nil
	}
	maatRepairScanDriftContext = func(context.Context) (*selfupdate.DriftReport, error) { return maatRepairScanDrift() }
	if _, err := currentMaatBinaryDriftPreview(evidence); err == nil || !strings.Contains(err.Error(), "unsupported or unmanaged") {
		t.Fatalf("unmanaged target preview error = %v", err)
	}
	if len(journal.decisions) != 1 {
		t.Fatalf("unmanaged refusal changed journal: %d rows", len(journal.decisions))
	}
}

func TestMaatBinaryDriftRefusesSeparateOwnerDecisionForOriginalCase(t *testing.T) {
	journal, evidence, _, _ := installMaatBinaryDriftFixture(t)
	journal.decisions = append(journal.decisions, maat.Decision{
		Kind: "diagnostic owner review", OriginEvidence: evidence, Evidence: "diagnostic-review:sha256:separate",
	})
	if err := validateMaatBinaryDriftCase(journal, evidence); err == nil || !strings.Contains(err.Error(), "separate owner decision") {
		t.Fatalf("case with existing owner review was accepted for repair: %v", err)
	}
}

func TestMaatBinaryDriftDurableAttemptBlocksSameCaseReplay(t *testing.T) {
	journal, evidence, _, _ := installMaatBinaryDriftFixture(t)
	journal.decisions = append(journal.decisions, maat.Decision{Kind: "diagnostic repair attempt", OriginEvidence: evidence, Evidence: "diagnostic-repair-attempt:sha256=" + strings.Repeat("a", 64)})
	if err := validateMaatBinaryDriftCase(journal, evidence); err == nil || !strings.Contains(err.Error(), "durable repair attempt") {
		t.Fatalf("same-case retry after pre-launch claim = %v", err)
	}
}

func TestMaatBinaryDriftConcurrentClaimsAppendOnlyOneLaunchPermit(t *testing.T) {
	path := filepath.Join(t.TempDir(), "maat", "decisions.jsonl")
	journal := &maat.FileDecisionJournal{Path: path}
	preview := maatBinaryDriftPreview{CaseEvidence: "maat-system-one:sha256=" + strings.Repeat("b", 64), Binding: strings.Repeat("c", 64)}
	start := make(chan struct{})
	results := make(chan error, 2)
	var callers sync.WaitGroup
	for _, binding := range []string{strings.Repeat("c", 64), strings.Repeat("d", 64)} {
		callers.Add(1)
		go func(binding string) {
			defer callers.Done()
			<-start
			candidate := preview
			candidate.Binding = binding
			results <- recordMaatBinaryDriftAttempt(journal, candidate)
		}(binding)
	}
	close(start)
	callers.Wait()
	close(results)
	successes, refusals := 0, 0
	for err := range results {
		if err == nil {
			successes++
		} else {
			refusals++
		}
	}
	if successes != 1 || refusals != 1 {
		t.Fatalf("concurrent same-case launch claims: success=%d refusal=%d", successes, refusals)
	}
	rows, err := journal.Recent(10)
	if err != nil || len(rows) != 1 || rows[0].Kind != "diagnostic repair attempt" {
		t.Fatalf("durable launch claim rows=%+v err=%v", rows, err)
	}
}

func TestMaatBinaryDriftAttemptInspectionIsReadOnlyAndUnattributed(t *testing.T) {
	caseEvidence := "maat-system-one:sha256=" + strings.Repeat("b", 64)
	journal := &maatTestJournal{decisions: []maat.Decision{{
		Kind: "diagnostic repair attempt", Resource: maatRepairBinaryDriftCheck,
		OriginEvidence: caseEvidence, Evidence: maatBinaryDriftAttemptEvidence(caseEvidence),
	}}}
	old := maatRepairReadBinaryFinding
	maatRepairReadBinaryFinding = func(context.Context) (guard.DiagnosticFinding, error) {
		return pantheonBinaryDriftFinding(&selfupdate.DriftReport{Healthy: true}), nil
	}
	t.Cleanup(func() { maatRepairReadBinaryFinding = old })
	status, err := inspectMaatBinaryDriftAttempt(journal, caseEvidence)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(status.Observation, "does not attribute") || len(journal.decisions) != 1 {
		t.Fatalf("inspection changed journal or attributed the prior attempt: status=%+v rows=%d", status, len(journal.decisions))
	}
}

func TestMaatBinaryDriftUnknownUpdaterRetainsPendingClaimWithoutTerminalReceipt(t *testing.T) {
	journal, evidence, self, target := installMaatBinaryDriftFixture(t)
	read := func() (*guard.DoctorReport, error) { return maatBinaryDriftFixtureReport(true), nil }
	scan := func() (*selfupdate.DriftReport, error) {
		return maatBinaryDriftFixtureInventory(self, target, true), nil
	}
	updates := installMaatBinaryDriftSeams(t, journal, self, target, read, scan)
	preview, err := currentMaatBinaryDriftPreview(evidence)
	if err != nil {
		t.Fatal(err)
	}
	maatRepairConfirm = true
	maatRepairBinaryDriftCaseEvidence, maatRepairBinaryDriftBinding = evidence, preview.Binding
	maatRepairRunHomebrewUpdate = func(context.Context, string) error {
		(*updates)++
		return errUpdaterExecutionUnknown
	}
	err = maatRepairBinaryDriftCmd.RunE(maatRepairBinaryDriftCmd, nil)
	if !errors.Is(err, errUpdaterExecutionUnknown) || *updates != 1 {
		t.Fatalf("unknown updater result=%v calls=%d", err, *updates)
	}
	if len(journal.decisions) != 3 || journal.decisions[2].Kind != "diagnostic repair attempt" {
		t.Fatalf("unknown execution was falsely terminalized: %+v", journal.decisions)
	}
	if err := validateMaatBinaryDriftCase(journal, evidence); err == nil || !strings.Contains(err.Error(), "durable repair attempt") {
		t.Fatalf("unresolved attempt allowed same-case replay: %v", err)
	}
	status, inspectErr := inspectMaatBinaryDriftAttempt(journal, evidence)
	if inspectErr != nil || status.MemoryReceipt == nil || status.MemoryReceipt.Action.Operation != "update-homebrew-binary-drift" {
		t.Fatalf("read-only CLI inspection did not follow canonical typed receipt projection: status=%+v err=%v", status, inspectErr)
	}
	store, storeErr := maat.OpenDefaultStore()
	if storeErr != nil {
		t.Fatal(storeErr)
	}
	defer store.Close()
	if pending, pendingErr := store.PendingBinaryUpdateAttempt(); pendingErr != nil || pending.Manifest.CaseEvidence != evidence {
		t.Fatalf("unknown memory attempt=%+v err=%v", pending, pendingErr)
	}
	maatRepairBinaryDriftReconcile = true
	if err := maatRepairBinaryDriftCmd.RunE(maatRepairBinaryDriftCmd, nil); err == nil || !strings.Contains(err.Error(), "completion is unknown") {
		t.Fatalf("unknown invocation was incorrectly reconciled: %v", err)
	}
	if *updates != 1 {
		t.Fatalf("unknown reconciliation replayed updater: calls=%d", *updates)
	}
}

func TestMaatBinaryDriftConfirmRefusesChangedPreviewBeforeUpdater(t *testing.T) {
	journal, evidence, self, target := installMaatBinaryDriftFixture(t)
	active := true
	read := func() (*guard.DoctorReport, error) { return maatBinaryDriftFixtureReport(active), nil }
	scan := func() (*selfupdate.DriftReport, error) {
		return maatBinaryDriftFixtureInventory(self, target, active), nil
	}
	updates := installMaatBinaryDriftSeams(t, journal, self, target, read, scan)
	maatRepairConfirm = true
	maatRepairBinaryDriftCaseEvidence = evidence
	maatRepairBinaryDriftBinding = strings.Repeat("0", 64)
	if err := maatRepairBinaryDriftCmd.RunE(maatRepairBinaryDriftCmd, nil); err == nil || !strings.Contains(err.Error(), "preview changed") {
		t.Fatalf("changed preview confirmation error = %v", err)
	}
	if *updates != 0 || len(journal.decisions) != 1 {
		t.Fatalf("changed preview calls=%d journal rows=%d; want no updater and only original case", *updates, len(journal.decisions))
	}
}

func TestMaatBinaryDriftCommandRetainsRejectAndUnverifiableBeforeZeroUpdaterCalls(t *testing.T) {
	for _, tc := range []struct {
		name, guardID, guardVersion string
		want                        maat.PreflightDecision
	}{
		{"exact recurrence", maat.ActiveIncidentGuardID, maat.ActiveIncidentGuardVersion, maat.PreflightReject},
		{"unsupported exact guard", "unknown.binary-drift-guard", "v1", maat.PreflightUnverifiable},
	} {
		t.Run(tc.name, func(t *testing.T) {
			journal, evidence, self, target := installMaatBinaryDriftFixture(t)
			updates := installMaatBinaryDriftSeams(t, journal, self, target, func() (*guard.DoctorReport, error) { return maatBinaryDriftFixtureReport(true), nil }, func() (*selfupdate.DriftReport, error) {
				return maatBinaryDriftFixtureInventory(self, target, true), nil
			})
			appendBinaryDriftMemoryIncident(t, tc.guardID, tc.guardVersion)
			preview, err := currentMaatBinaryDriftPreview(evidence)
			if err != nil {
				t.Fatal(err)
			}
			maatRepairConfirm = true
			maatRepairBinaryDriftCaseEvidence = evidence
			maatRepairBinaryDriftBinding = preview.Binding
			err = maatRepairBinaryDriftCmd.RunE(maatRepairBinaryDriftCmd, nil)
			if err == nil || !strings.Contains(err.Error(), string(tc.want)) || *updates != 0 {
				t.Fatalf("decision=%s err=%v updater calls=%d", tc.want, err, *updates)
			}
			found := false
			for _, d := range journal.decisions {
				if d.Kind == "failure memory preflight" && d.FailureMemoryReceipt != nil && d.FailureMemoryReceipt.Action.Operation == "update-homebrew-binary-drift" {
					found = true
				}
			}
			if !found {
				t.Fatalf("negative preflight receipt was not projected: %+v", journal.decisions)
			}
		})
	}
}

func TestMaatBinaryDriftCobraSerializesConcurrentPreviewConfirmations(t *testing.T) {
	journal, evidence, self, target := installMaatBinaryDriftFixture(t)
	safeJournal := &concurrentBinaryDriftJournal{rows: append([]maat.Decision(nil), journal.decisions...)}
	newMaatDecisionJournal = func() (maat.DecisionJournal, error) { return safeJournal, nil }
	var calls atomic.Int32
	maatRepairRunHomebrewUpdate = func(context.Context, string) error { calls.Add(1); return nil }
	maatRepairReadDiagnosis = func() (*guard.DoctorReport, error) { return maatBinaryDriftFixtureReport(true), nil }
	maatRepairScanDrift = func() (*selfupdate.DriftReport, error) {
		return maatBinaryDriftFixtureInventory(self, target, true), nil
	}
	maatRepairScanDriftContext = func(ctx context.Context) (*selfupdate.DriftReport, error) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		return maatBinaryDriftFixtureInventory(self, target, true), nil
	}
	maatRepairReadBinaryFinding = func(ctx context.Context) (guard.DiagnosticFinding, error) {
		if err := ctx.Err(); err != nil {
			return guard.DiagnosticFinding{}, err
		}
		return pantheonBinaryDriftFinding(maatBinaryDriftFixtureInventory(self, target, true)), nil
	}
	maatRepairConfirm = true
	maatRepairBinaryDriftCaseEvidence = evidence
	preview, err := currentMaatBinaryDriftPreview(evidence)
	if err != nil {
		t.Fatal(err)
	}
	maatRepairBinaryDriftBinding = preview.Binding
	start := make(chan struct{})
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			command := *maatRepairBinaryDriftCmd
			command.SetContext(context.Background())
			errs <- command.RunE(&command, nil)
		}()
	}
	close(start)
	wg.Wait()
	close(errs)
	if calls.Load() != 1 {
		results := []error{}
		for err := range errs {
			results = append(results, err)
		}
		t.Fatalf("concurrent Cobra confirmations launched updater %d times; results=%v, journal=%+v", calls.Load(), results, journal.decisions)
	}
}

func TestMaatBinaryDriftManifestSchemaDriftBlocksBeforeLaunchClaim(t *testing.T) {
	journal, evidence, self, target := installMaatBinaryDriftFixture(t)
	updates := installMaatBinaryDriftSeams(t, journal, self, target, func() (*guard.DoctorReport, error) { return maatBinaryDriftFixtureReport(true), nil }, func() (*selfupdate.DriftReport, error) {
		return maatBinaryDriftFixtureInventory(self, target, true), nil
	})
	preview, err := currentMaatBinaryDriftPreview(evidence)
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	maatRepairBinarySchemaObservation = func(modversion.Info) (int, error) {
		calls++
		if calls == 1 {
			return 7, nil
		}
		return 8, nil
	}
	maatRepairConfirm = true
	maatRepairBinaryDriftCaseEvidence = evidence
	maatRepairBinaryDriftBinding = preview.Binding
	err = maatRepairBinaryDriftCmd.RunE(maatRepairBinaryDriftCmd, nil)
	if err == nil || !strings.Contains(err.Error(), "schema compatibility observation changed") || *updates != 0 || calls < 2 {
		t.Fatalf("schema drift err=%v updater calls=%d observations=%d", err, *updates, calls)
	}
	for _, decision := range journal.decisions {
		if decision.Kind == "diagnostic repair attempt" {
			t.Fatal("launch claim was written before manifest recheck")
		}
	}
}

type projectionFailMaatJournal struct{ *maatTestJournal }

func (j *projectionFailMaatJournal) AppendIdempotent(maat.Decision) error {
	return errors.New("injected projection outage")
}

type projectionReadbackFailMaatJournal struct{ *maatTestJournal }

func (j *projectionReadbackFailMaatJournal) FindByEvidence(string, string) (maat.Decision, bool, error) {
	return maat.Decision{}, false, errors.New("injected exact readback outage")
}

func TestMaatBinaryDriftProjectionDebtDoesNotRetryUpdater(t *testing.T) {
	base, evidence, self, target := installMaatBinaryDriftFixture(t)
	updates := installMaatBinaryDriftSeams(t, base, self, target, func() (*guard.DoctorReport, error) { return maatBinaryDriftFixtureReport(false), nil }, func() (*selfupdate.DriftReport, error) {
		return maatBinaryDriftFixtureInventory(self, target, false), nil
	})
	journal := &projectionFailMaatJournal{maatTestJournal: base}
	newMaatDecisionJournal = func() (maat.DecisionJournal, error) { return journal, nil }
	preview, err := currentMaatBinaryDriftPreview(evidence)
	if err != nil {
		t.Fatal(err)
	}
	maatRepairConfirm = true
	maatRepairBinaryDriftCaseEvidence = evidence
	maatRepairBinaryDriftBinding = preview.Binding
	if err := maatRepairBinaryDriftCmd.RunE(maatRepairBinaryDriftCmd, nil); err == nil || !strings.Contains(err.Error(), "projection debt") {
		t.Fatalf("projection outage result=%v", err)
	}
	if *updates != 1 {
		t.Fatalf("first updater count=%d", *updates)
	}
	store, err := maat.OpenDefaultStore()
	if err != nil {
		t.Fatal(err)
	}
	debts, err := store.PendingBinaryUpdateProjectionDebts()
	_ = store.Close()
	if err != nil || len(debts) != 1 {
		t.Fatalf("projection debts=%d err=%v", len(debts), err)
	}
	if err := maatRepairBinaryDriftCmd.RunE(maatRepairBinaryDriftCmd, nil); err == nil {
		t.Fatal("retry unexpectedly proceeded through existing attempt")
	}
	if *updates != 1 {
		t.Fatalf("projection debt caused automatic updater replay: calls=%d", *updates)
	}
	newMaatDecisionJournal = func() (maat.DecisionJournal, error) {
		return &projectionReadbackFailMaatJournal{maatTestJournal: base}, nil
	}
	if err := maatRepairBinaryDriftCmd.RunE(maatRepairBinaryDriftCmd, nil); err == nil || !strings.Contains(err.Error(), "read back terminal outcome projection") {
		t.Fatalf("injected projection readback outage was not retained as debt: %v", err)
	}
	store, err = maat.OpenDefaultStore()
	if err != nil {
		t.Fatal(err)
	}
	debts, err = store.PendingBinaryUpdateProjectionDebts()
	_ = store.Close()
	if err != nil || len(debts) != 1 {
		t.Fatalf("readback outage lost projection debt: debts=%d err=%v", len(debts), err)
	}
	newMaatDecisionJournal = func() (maat.DecisionJournal, error) { return base, nil }
	maatRepairBinaryDriftReconcile = true
	if err := maatRepairBinaryDriftCmd.RunE(maatRepairBinaryDriftCmd, nil); err != nil {
		t.Fatalf("data-only terminal projection reconciliation failed: %v", err)
	}
	if *updates != 1 {
		t.Fatalf("projection reconciliation replayed updater: calls=%d", *updates)
	}
	store, err = maat.OpenDefaultStore()
	if err != nil {
		t.Fatal(err)
	}
	debts, err = store.PendingBinaryUpdateProjectionDebts()
	_ = store.Close()
	if err != nil || len(debts) != 0 {
		t.Fatalf("reconciled projection debts=%d err=%v", len(debts), err)
	}
}

func TestMaatBinaryDriftReconcileClosesTypedPreInvocationGapWithoutUpdater(t *testing.T) {
	journal, evidence, self, target := installMaatBinaryDriftFixture(t)
	updates := installMaatBinaryDriftSeams(t, journal, self, target, func() (*guard.DoctorReport, error) { return maatBinaryDriftFixtureReport(true), nil }, func() (*selfupdate.DriftReport, error) {
		return maatBinaryDriftFixtureInventory(self, target, true), nil
	})
	preview, err := currentMaatBinaryDriftPreview(evidence)
	if err != nil {
		t.Fatal(err)
	}
	status := maatRepairDefaultStoreInspect()
	store, err := maatRepairOpenBinaryDriftStore()
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := buildMaatBinaryUpdateManifest(preview, status.StoreUUID)
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := store.PreflightBinaryUpdate(manifest)
	if err != nil {
		t.Fatal(err)
	}
	stored, err := store.RetainPreflightReceipt(receipt)
	if err != nil {
		t.Fatal(err)
	}
	if err := maat.ProjectFailureMemoryPreflight(journal, stored); err != nil {
		t.Fatal(err)
	}
	digest, err := manifest.Digest()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.BeginBinaryUpdateAttempt(manifest, receipt, func() (string, error) {
		if err := recordMaatBinaryDriftAttemptBound(journal, preview, stored.Reference(), digest); err != nil {
			return "", err
		}
		return maatBinaryDriftAttemptEvidence(evidence), nil
	}); err != nil {
		_ = store.Close()
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	maatRepairBinaryDriftReconcile = true
	if err := maatRepairBinaryDriftCmd.RunE(maatRepairBinaryDriftCmd, nil); err != nil {
		t.Fatalf("typed pre-invocation reconciliation failed: %v", err)
	}
	if *updates != 0 {
		t.Fatalf("data-only reconciliation invoked updater %d times", *updates)
	}
	store, err = maat.OpenDefaultStore()
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	history, err := store.BinaryUpdateReconciliations(evidence)
	if err != nil || len(history) != 1 || history[0].Outcome == nil || history[0].Outcome.Determination != "not_invoked" || history[0].InvocationRecorded || !history[0].ProjectionReadBack {
		t.Fatalf("reconciled typed gap=%+v err=%v", history, err)
	}
}

func TestMaatBinaryDriftReconcileRebuildsMissingProjectionIntentWithoutUpdater(t *testing.T) {
	journal, evidence, self, target := installMaatBinaryDriftFixture(t)
	updates := installMaatBinaryDriftSeams(t, journal, self, target, func() (*guard.DoctorReport, error) { return maatBinaryDriftFixtureReport(false), nil }, func() (*selfupdate.DriftReport, error) {
		return maatBinaryDriftFixtureInventory(self, target, false), nil
	})
	preview, err := currentMaatBinaryDriftPreview(evidence)
	if err != nil {
		t.Fatal(err)
	}
	status := maatRepairDefaultStoreInspect()
	store, err := maatRepairOpenBinaryDriftStore()
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := buildMaatBinaryUpdateManifest(preview, status.StoreUUID)
	if err != nil {
		_ = store.Close()
		t.Fatal(err)
	}
	receipt, err := store.PreflightBinaryUpdate(manifest)
	if err != nil {
		_ = store.Close()
		t.Fatal(err)
	}
	stored, err := store.RetainPreflightReceipt(receipt)
	if err != nil {
		_ = store.Close()
		t.Fatal(err)
	}
	if err := maat.ProjectFailureMemoryPreflight(journal, stored); err != nil {
		_ = store.Close()
		t.Fatal(err)
	}
	digest, err := manifest.Digest()
	if err != nil {
		_ = store.Close()
		t.Fatal(err)
	}
	attempt, err := store.BeginBinaryUpdateAttempt(manifest, receipt, func() (string, error) {
		if err := recordMaatBinaryDriftAttemptBound(journal, preview, stored.Reference(), digest); err != nil {
			return "", err
		}
		return maatBinaryDriftAttemptEvidence(evidence), nil
	})
	if err != nil {
		_ = store.Close()
		t.Fatal(err)
	}
	if err := store.RecordBinaryUpdateInvocation(attempt); err != nil {
		_ = store.Close()
		t.Fatal(err)
	}
	if _, err := store.RecordBinaryUpdateOutcome(attempt, true, false, strings.Repeat("e", 64), "postcheck remains active"); err != nil {
		_ = store.Close()
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	maatRepairBinaryDriftReconcile = true
	if err := maatRepairBinaryDriftCmd.RunE(maatRepairBinaryDriftCmd, nil); err != nil {
		t.Fatalf("reconstructing missing projection intent failed: %v", err)
	}
	if *updates != 0 {
		t.Fatalf("missing-intent reconciliation invoked updater %d times", *updates)
	}
	store, err = maat.OpenDefaultStore()
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	history, err := store.BinaryUpdateReconciliations(evidence)
	if err != nil || len(history) != 1 || history[0].Outcome == nil || history[0].Projection == nil || !history[0].ProjectionReadBack {
		t.Fatalf("reconstructed terminal projection=%+v err=%v", history, err)
	}
}

func TestMaatBinaryDriftRecordsResolvedOnlyAfterFreshClearObservation(t *testing.T) {
	journal, evidence, self, target := installMaatBinaryDriftFixture(t)
	reads, scans := 0, 0
	read := func() (*guard.DoctorReport, error) {
		reads++
		return maatBinaryDriftFixtureReport(reads <= 2), nil
	}
	scan := func() (*selfupdate.DriftReport, error) {
		scans++
		return maatBinaryDriftFixtureInventory(self, target, scans <= 4), nil
	}
	updates := installMaatBinaryDriftSeams(t, journal, self, target, read, scan)
	maatRepairRunHomebrewUpdate = func(context.Context, string) error {
		(*updates)++
		store, err := maat.OpenDefaultStore()
		if err != nil {
			return err
		}
		defer store.Close()
		attempt, err := store.PendingBinaryUpdateAttempt()
		if err != nil {
			return fmt.Errorf("effect reached before durable memory attempt: %w", err)
		}
		readback, err := store.ReadPreflightReceipt(attempt.PreflightReceiptSHA256)
		if err != nil || readback.Receipt.Decision != maat.PreflightPass || readback.Receipt.ActionManifestSHA256 != attempt.ActionManifestSHA256 {
			return fmt.Errorf("effect reached before exact passing receipt readback: %v", err)
		}
		preflightFound, claimFound := false, false
		for _, row := range journal.decisions {
			if row.Kind == "failure memory preflight" && row.FailureMemoryReceipt != nil && row.FailureMemoryReceipt.ReceiptSHA256 == attempt.PreflightReceiptSHA256 {
				preflightFound = true
			}
			if row.Kind == "diagnostic repair attempt" && row.FailureMemoryReceipt == nil && strings.Contains(row.Why, "retained failure-memory receipt="+attempt.PreflightReceiptSHA256) && strings.Contains(row.Why, "action="+attempt.ActionManifestSHA256) && strings.Contains(row.Why, "registry="+attempt.RegistrySnapshotSHA256) {
				claimFound = true
			}
		}
		if !preflightFound || !claimFound {
			return errors.New("effect reached before canonical typed preflight projection and linked journal launch claim")
		}
		return nil
	}
	preview, err := currentMaatBinaryDriftPreview(evidence)
	if err != nil {
		t.Fatal(err)
	}
	maatRepairConfirm = true
	maatRepairBinaryDriftCaseEvidence = evidence
	maatRepairBinaryDriftBinding = preview.Binding
	if err := maatRepairBinaryDriftCmd.RunE(maatRepairBinaryDriftCmd, nil); err != nil {
		t.Fatal(err)
	}
	if *updates != 1 || reads != 0 || scans != 6 {
		t.Fatalf("updates=%d diagnosis reads=%d inventory reads=%d", *updates, reads, scans)
	}
	if len(journal.decisions) != 4 || journal.decisions[2].Kind != "diagnostic repair attempt" || journal.decisions[3].Kind != "diagnostic verified repair" || journal.decisions[3].Determination != "resolved" {
		t.Fatalf("repair receipt = %+v", journal.decisions)
	}
	if journal.decisions[0].Kind != "system one screen" {
		t.Fatalf("original System One record was not preserved: %+v", journal.decisions[0])
	}
	store, err := maat.OpenDefaultStore()
	if err != nil {
		t.Fatal(err)
	}
	debts, err := store.PendingBinaryUpdateProjectionDebts()
	_ = store.Close()
	if err != nil || len(debts) != 0 {
		t.Fatalf("completed projection readback left debts=%d err=%v", len(debts), err)
	}
}

func TestMaatBinaryDriftRealFileJournalAcceptsTypedPreflightAndClaim(t *testing.T) {
	fixtureJournal, evidence, self, target := installMaatBinaryDriftFixture(t)
	active := true
	read := func() (*guard.DoctorReport, error) { return maatBinaryDriftFixtureReport(active), nil }
	scan := func() (*selfupdate.DriftReport, error) {
		return maatBinaryDriftFixtureInventory(self, target, active), nil
	}
	installMaatBinaryDriftSeams(t, fixtureJournal, self, target, read, scan)
	journal := &maat.FileDecisionJournal{Path: filepath.Join(t.TempDir(), "casebook", "decisions.jsonl")}
	for _, row := range fixtureJournal.decisions {
		if err := journal.Append(row); err != nil {
			t.Fatalf("seed real journal: %v", err)
		}
	}
	newMaatDecisionJournal = func() (maat.DecisionJournal, error) { return journal, nil }
	preview, err := currentMaatBinaryDriftPreview(evidence)
	if err != nil {
		t.Fatal(err)
	}
	maatRepairConfirm = true
	maatRepairBinaryDriftCaseEvidence, maatRepairBinaryDriftBinding = evidence, preview.Binding
	maatRepairRunHomebrewUpdate = func(context.Context, string) error {
		store, err := maat.OpenDefaultStore()
		if err != nil {
			return err
		}
		defer store.Close()
		attempt, err := store.PendingBinaryUpdateAttempt()
		if err != nil {
			return err
		}
		history, err := store.BinaryUpdateReconciliations(evidence)
		if err != nil || len(history) != 1 || !history[0].InvocationRecorded {
			return fmt.Errorf("real store did not retain exact invocation barrier before updater: history=%+v err=%v", history, err)
		}
		rows, err := journal.Recent(20)
		if err != nil {
			return err
		}
		var preflight, claim *maat.Decision
		for i := range rows {
			row := &rows[i]
			if row.Kind == "failure memory preflight" && row.FailureMemoryReceipt != nil && row.FailureMemoryReceipt.ReceiptSHA256 == attempt.PreflightReceiptSHA256 {
				preflight = row
			}
			if row.Kind == "diagnostic repair attempt" && row.OriginEvidence == evidence {
				claim = row
			}
		}
		if preflight == nil || claim == nil || claim.FailureMemoryReceipt != nil || !strings.Contains(claim.Why, "retained failure-memory receipt="+attempt.PreflightReceiptSHA256) {
			return errors.New("real journal did not retain the typed preflight projection and case-bound launch claim")
		}
		active = false
		return nil
	}
	if err := maatRepairBinaryDriftCmd.RunE(maatRepairBinaryDriftCmd, nil); err != nil {
		t.Fatal(err)
	}
	rows, err := journal.Recent(20)
	if err != nil {
		t.Fatal(err)
	}
	var preflight, claim *maat.Decision
	for i := range rows {
		row := &rows[i]
		if row.Kind == "failure memory preflight" && row.FailureMemoryReceipt != nil && row.FailureMemoryReceipt.Action.Operation == "update-homebrew-binary-drift" {
			preflight = row
		}
		if row.Kind == "diagnostic repair attempt" && row.OriginEvidence == evidence {
			claim = row
		}
	}
	if preflight == nil || claim == nil || claim.FailureMemoryReceipt != nil {
		t.Fatalf("real file journal rows lost canonical typed receipt/claim boundary: %+v", rows)
	}
}

func TestMaatBinaryDriftPostcheckStillActiveNeverRecordsSuccess(t *testing.T) {
	journal, evidence, self, target := installMaatBinaryDriftFixture(t)
	reads, scans := 0, 0
	read := func() (*guard.DoctorReport, error) {
		reads++
		return maatBinaryDriftFixtureReport(true), nil
	}
	scan := func() (*selfupdate.DriftReport, error) {
		scans++
		return maatBinaryDriftFixtureInventory(self, target, true), nil
	}
	updates := installMaatBinaryDriftSeams(t, journal, self, target, read, scan)
	preview, err := currentMaatBinaryDriftPreview(evidence)
	if err != nil {
		t.Fatal(err)
	}
	maatRepairConfirm = true
	maatRepairBinaryDriftCaseEvidence = evidence
	maatRepairBinaryDriftBinding = preview.Binding
	err = maatRepairBinaryDriftCmd.RunE(maatRepairBinaryDriftCmd, nil)
	if err == nil || !strings.Contains(err.Error(), "did not clear") {
		t.Fatalf("still-active postcheck result = %v", err)
	}
	if *updates != 1 || len(journal.decisions) != 4 || journal.decisions[3].Determination != "failed" {
		t.Fatalf("updates=%d decisions=%+v", *updates, journal.decisions)
	}
}

func TestMaatBinaryDriftCancellationRetainsFailedReceiptWithoutResolvedOutcome(t *testing.T) {
	journal, evidence, self, target := installMaatBinaryDriftFixture(t)
	active := true
	read := func() (*guard.DoctorReport, error) { return maatBinaryDriftFixtureReport(active), nil }
	scan := func() (*selfupdate.DriftReport, error) {
		return maatBinaryDriftFixtureInventory(self, target, active), nil
	}
	updates := installMaatBinaryDriftSeams(t, journal, self, target, read, scan)
	preview, err := currentMaatBinaryDriftPreview(evidence)
	if err != nil {
		t.Fatal(err)
	}
	maatRepairConfirm = true
	maatRepairBinaryDriftCaseEvidence = evidence
	maatRepairBinaryDriftBinding = preview.Binding

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	command := *maatRepairBinaryDriftCmd
	command.SetContext(ctx)
	started := make(chan struct{})
	maatRepairRunHomebrewUpdate = func(ctx context.Context, _ string) error {
		(*updates)++
		close(started)
		<-ctx.Done()
		active = false // Fresh postcheck clears, but cancellation means updater success is unproven.
		return ctx.Err()
	}
	result := make(chan error, 1)
	joined := false
	defer func() {
		cancel()
		if !joined {
			select {
			case <-result:
			case <-time.After(3 * time.Second):
				t.Errorf("cancellation fixture did not join the command goroutine")
			}
		}
	}()
	go func() { result <- command.RunE(&command, nil) }()
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("confirmed updater did not reach its injected in-progress seam")
	}
	cancel()
	select {
	case err := <-result:
		joined = true
		if err == nil || !strings.Contains(err.Error(), "did not verify") {
			t.Fatalf("canceled updater terminal result = %v, want an explicit failed outcome", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("canceled updater did not produce a bounded terminal receipt")
	}
	if *updates != 1 || len(journal.decisions) != 4 {
		t.Fatalf("updater calls=%d decisions=%+v; want one invocation and one terminal receipt", *updates, journal.decisions)
	}
	if got := journal.decisions[3]; got.Kind != "diagnostic verified repair" || got.Determination != "failed" || got.OriginEvidence != evidence {
		t.Fatalf("canceled updater receipt = %+v; cancellation must never claim resolution", got)
	} else if !strings.Contains(got.Why, "fresh state is clear") {
		t.Fatalf("canceled updater receipt omitted the clear-but-unattributed fresh observation: %+v", got)
	}
}
