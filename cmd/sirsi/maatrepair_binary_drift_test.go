package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/guard"
	"github.com/SirsiMaster/sirsi-pantheon/internal/maat"
	"github.com/SirsiMaster/sirsi-pantheon/internal/selfupdate"
	modversion "github.com/SirsiMaster/sirsi-pantheon/internal/version"
)

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

func installMaatBinaryDriftSeams(t *testing.T, journal *maatTestJournal, self, target string,
	diagnosis func() (*guard.DoctorReport, error), drift func() (*selfupdate.DriftReport, error)) *int {
	t.Helper()
	oldCommandContext := maatRepairBinaryDriftCmd.Context()
	if oldCommandContext == nil {
		maatRepairBinaryDriftCmd.SetContext(context.Background())
	}
	oldConfirm := maatRepairConfirm
	oldEvidence, oldBinding := maatRepairBinaryDriftCaseEvidence, maatRepairBinaryDriftBinding
	oldDiagnosis, oldScan, oldScanContext, oldFinding := maatRepairReadDiagnosis, maatRepairScanDrift, maatRepairScanDriftContext, maatRepairReadBinaryFinding
	oldHash, oldMethod := maatRepairHashFile, maatRepairDetectMethod
	oldFind, oldRun, oldJournal := maatRepairFindHomebrew, maatRepairRunHomebrewUpdate, newMaatDecisionJournal
	updateCalls := 0
	maatRepairConfirm = false
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
		maatRepairBinaryDriftCaseEvidence, maatRepairBinaryDriftBinding = oldEvidence, oldBinding
		maatRepairReadDiagnosis, maatRepairScanDrift, maatRepairScanDriftContext, maatRepairReadBinaryFinding = oldDiagnosis, oldScan, oldScanContext, oldFinding
		maatRepairHashFile, maatRepairDetectMethod = oldHash, oldMethod
		maatRepairFindHomebrew, maatRepairRunHomebrewUpdate, newMaatDecisionJournal = oldFind, oldRun, oldJournal
	})
	return &updateCalls
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
	if len(journal.decisions) != 2 || journal.decisions[1].Kind != "diagnostic repair attempt" {
		t.Fatalf("unknown execution was falsely terminalized: %+v", journal.decisions)
	}
	if err := validateMaatBinaryDriftCase(journal, evidence); err == nil || !strings.Contains(err.Error(), "durable repair attempt") {
		t.Fatalf("unresolved attempt allowed same-case replay: %v", err)
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
	if len(journal.decisions) != 3 || journal.decisions[1].Kind != "diagnostic repair attempt" || journal.decisions[2].Kind != "diagnostic verified repair" || journal.decisions[2].Determination != "resolved" {
		t.Fatalf("repair receipt = %+v", journal.decisions)
	}
	if journal.decisions[0].Kind != "system one screen" {
		t.Fatalf("original System One record was not preserved: %+v", journal.decisions[0])
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
	if *updates != 1 || len(journal.decisions) != 3 || journal.decisions[2].Determination != "failed" {
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
	if *updates != 1 || len(journal.decisions) != 3 {
		t.Fatalf("updater calls=%d decisions=%+v; want one invocation and one terminal receipt", *updates, journal.decisions)
	}
	if got := journal.decisions[2]; got.Kind != "diagnostic verified repair" || got.Determination != "failed" || got.OriginEvidence != evidence {
		t.Fatalf("canceled updater receipt = %+v; cancellation must never claim resolution", got)
	} else if !strings.Contains(got.Why, "fresh state is clear") {
		t.Fatalf("canceled updater receipt omitted the clear-but-unattributed fresh observation: %+v", got)
	}
}
