package main

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/guard"
	"github.com/SirsiMaster/sirsi-pantheon/internal/liveness"
	"github.com/SirsiMaster/sirsi-pantheon/internal/maat"
	"github.com/SirsiMaster/sirsi-pantheon/internal/setup"
	"golang.org/x/sys/unix"
)

type livenessFixtureStore struct {
	decision    maat.PreflightDecision
	manifest    maat.LivenessWatchManifest
	receipt     maat.PreflightReceipt
	attempt     maat.LivenessWatchAttempt
	started     int
	outcomes    int
	observation maat.LivenessWatchObservation
	pending     bool
	outcome     maat.LivenessWatchOutcome
	projection  *maat.LivenessWatchProjectionDebt
	projected   bool
	beginErr    error
}

func (s *livenessFixtureStore) Close() error { return nil }

func (s *livenessFixtureStore) PreflightLivenessWatch(manifest maat.LivenessWatchManifest) (maat.PreflightReceipt, error) {
	digest, err := manifest.Digest()
	if err != nil {
		return maat.PreflightReceipt{}, err
	}
	s.manifest = manifest
	s.receipt = maat.PreflightReceipt{
		Schema:               maat.MemorySchema,
		Action:               maat.Scope{Component: "pantheon", Profile: "local-macos", Operation: "install-managed-liveness-watch"},
		ActionManifestSHA256: digest, RegistrySnapshotSHA256: strings.Repeat("d", 64),
		Decision: s.decision, EvaluatedAtUTC: time.Now().UTC(),
	}
	return s.receipt, nil
}

func (s *livenessFixtureStore) BeginLivenessWatchAttempt(manifest maat.LivenessWatchManifest, receipt maat.PreflightReceipt) (maat.LivenessWatchAttempt, error) {
	if s.beginErr != nil {
		return maat.LivenessWatchAttempt{}, s.beginErr
	}
	if receipt.ActionManifestSHA256 != s.receipt.ActionManifestSHA256 || receipt.Decision != maat.PreflightPass {
		return maat.LivenessWatchAttempt{}, errors.New("fixture refused unbound attempt")
	}
	s.started++
	id := "b3c515a2-4773-4653-86d1-8a5734c2ee45"
	if s.started > 1 {
		id = "c4d626b3-5884-4764-97e2-9b684d3dff56"
	}
	s.attempt = maat.LivenessWatchAttempt{
		Schema: maat.MemorySchema, ID: id, Manifest: manifest,
		ActionManifestSHA256: receipt.ActionManifestSHA256, PreflightReceiptSHA256: strings.Repeat("e", 64),
		RegistrySnapshotSHA256: receipt.RegistrySnapshotSHA256, StartedAtUTC: time.Now().UTC(),
	}
	return s.attempt, nil
}

func (s *livenessFixtureStore) PendingLivenessWatchAttempt() (maat.LivenessWatchAttempt, error) {
	if !s.pending {
		return maat.LivenessWatchAttempt{}, maat.ErrNoIncompleteLivenessWatchAttempt
	}
	return s.attempt, nil
}

func (s *livenessFixtureStore) RecordLivenessWatchOutcome(attempt maat.LivenessWatchAttempt, observation maat.LivenessWatchObservation, installErr string) (maat.LivenessWatchOutcome, error) {
	s.outcomes++
	if attempt.ID != s.attempt.ID {
		return maat.LivenessWatchOutcome{}, errors.New("fixture attempt mismatch")
	}
	determination := "incomplete"
	if installErr == "" && observation.UID == attempt.Manifest.UID && observation.Domain == attempt.Manifest.Domain &&
		observation.Label == attempt.Manifest.ManagedLabel && observation.PlistPath == attempt.Manifest.TargetPlistPath &&
		observation.PlistSHA256 == attempt.Manifest.ExpectedPlistSHA256 && observation.LaunchdLoaded {
		determination = "resolved"
	}
	s.pending = false
	return maat.LivenessWatchOutcome{Schema: maat.MemorySchema, AttemptID: attempt.ID, Determination: determination, RecordedAtUTC: time.Now().UTC(), Observation: observation}, nil
}

func (s *livenessFixtureStore) BeginLivenessWatchProjection(attempt maat.LivenessWatchAttempt, outcome maat.LivenessWatchOutcome, decision maat.Decision) (maat.LivenessWatchProjectionDebt, error) {
	debt := maat.LivenessWatchProjectionDebt{Schema: maat.LivenessWatchProjectionSchema, Attempt: attempt, Outcome: outcome, Decision: decision, CreatedAtUTC: time.Now().UTC()}
	s.projection = &debt
	s.projected = false
	return debt, nil
}

func (s *livenessFixtureStore) PersistLivenessWatchProjectionOutcome(debt maat.LivenessWatchProjectionDebt) error {
	if s.projection == nil || debt.Attempt.ID != s.projection.Attempt.ID {
		return errors.New("fixture projection debt mismatch")
	}
	if s.outcome.AttemptID == "" {
		s.outcomes++
	}
	s.outcome = debt.Outcome
	s.pending = false
	return nil
}

func (s *livenessFixtureStore) PendingLivenessWatchProjectionDebts() ([]maat.LivenessWatchProjectionDebt, error) {
	if s.projection == nil || s.projected {
		return nil, nil
	}
	return []maat.LivenessWatchProjectionDebt{*s.projection}, nil
}

func (s *livenessFixtureStore) MarkLivenessWatchProjectionReadBack(debt maat.LivenessWatchProjectionDebt) error {
	if s.projection == nil || debt.Attempt.ID != s.projection.Attempt.ID {
		return errors.New("fixture projection debt mismatch")
	}
	s.projected = true
	return nil
}

type livenessFixtureJournal struct {
	decisions         []maat.Decision
	failAt            int
	failAfterAppend   bool
	failAfterAppendAt int
	recentErr         error
}

func (j *livenessFixtureJournal) Append(d maat.Decision) error {
	if j.failAt > 0 && len(j.decisions)+1 == j.failAt {
		return errors.New("fixture append failure")
	}
	j.decisions = append(j.decisions, d)
	if j.failAfterAppend && (j.failAfterAppendAt == 0 || len(j.decisions) == j.failAfterAppendAt) {
		j.failAfterAppend = false
		return errors.New("fixture uncertain append result")
	}
	return nil
}

func (j *livenessFixtureJournal) AppendIdempotent(d maat.Decision) error {
	for _, prior := range j.decisions {
		if prior.Evidence == d.Evidence {
			if prior != d {
				return errors.New("fixture evidence collision")
			}
			return nil
		}
	}
	return j.Append(d)
}

func (j *livenessFixtureJournal) Recent(int) ([]maat.Decision, error) { return nil, j.recentErr }

func installLivenessCommandFixtures(t *testing.T, store *livenessFixtureStore, journal *livenessFixtureJournal, installErr error) *int {
	t.Helper()
	oldConfirm, oldRead := maatRepairConfirm, maatRepairReadLivenessFinding
	oldInspect, oldInitialize, oldOpen := maatRepairDefaultStoreInspect, maatRepairDefaultStoreInitialize, maatRepairOpenDefaultStore
	oldBuild, oldObserve, oldInstall, oldRevalidate := maatRepairBuildLivenessManifest, maatRepairObserveLiveness, maatRepairInstallLiveness, maatRepairRevalidateExecutable
	oldJournal := newMaatDecisionJournal
	oldJSON, oldGlobalJSON := maatJSON, JsonOutput
	t.Cleanup(func() {
		maatRepairConfirm, maatRepairReadLivenessFinding = oldConfirm, oldRead
		maatRepairDefaultStoreInspect, maatRepairDefaultStoreInitialize, maatRepairOpenDefaultStore = oldInspect, oldInitialize, oldOpen
		maatRepairBuildLivenessManifest, maatRepairObserveLiveness, maatRepairInstallLiveness, maatRepairRevalidateExecutable = oldBuild, oldObserve, oldInstall, oldRevalidate
		newMaatDecisionJournal = oldJournal
		maatJSON, JsonOutput = oldJSON, oldGlobalJSON
	})
	maatRepairConfirm = true
	maatJSON, JsonOutput = false, false
	maatRepairDefaultStoreInspect = func() maat.DefaultStoreStatus {
		return maat.DefaultStoreStatus{State: maat.DefaultStoreReady, StoreUUID: "b3c515a2-4773-4653-86d1-8a5734c2ee45"}
	}
	maatRepairDefaultStoreInitialize = func() (string, error) { return "", errors.New("unexpected initialize") }
	maatRepairOpenDefaultStore = func() (maatLivenessStore, error) { return store, nil }
	reads := 0
	maatRepairReadLivenessFinding = func() (guard.DiagnosticFinding, bool, error) {
		reads++
		return guard.DiagnosticFinding{Check: maatRepairLivenessWatchCheck, Severity: guard.SeverityWarn, Message: "watch absent", Detail: "bounded fixture"}, true, nil
	}
	maatRepairBuildLivenessManifest = func(f guard.DiagnosticFinding, storeUUID string) (maat.LivenessWatchManifest, error) {
		manifest := fixtureCommandManifest(storeUUID, f.Message)
		return manifest, nil
	}
	maatRepairRevalidateExecutable = func(maat.LivenessWatchManifest) error { return nil }
	maatRepairObserveLiveness = func(m maat.LivenessWatchManifest) (maat.LivenessWatchObservation, error) {
		if store.observation.Label == "" {
			store.observation = exactObservation(m)
		}
		return store.observation, nil
	}
	maatRepairInstallLiveness = func(maat.LivenessWatchManifest) (string, error) { return "fixture install", installErr }
	newMaatDecisionJournal = func() (maat.DecisionJournal, error) { return journal, nil }
	return &reads
}

func TestMaatLivenessRepairPreflightsThenCallsInstallerExactlyOnceAndRequiresExactPostcondition(t *testing.T) {
	store := &livenessFixtureStore{decision: maat.PreflightPass}
	journal := &livenessFixtureJournal{}
	reads := installLivenessCommandFixtures(t, store, journal, nil)
	installs := 0
	oldInstall := maatRepairInstallLiveness
	maatRepairInstallLiveness = func(m maat.LivenessWatchManifest) (string, error) { installs++; return oldInstall(m) }
	if err := maatRepairLivenessWatchCmd.RunE(maatRepairLivenessWatchCmd, nil); err != nil {
		t.Fatal(err)
	}
	if installs != 1 || store.started != 1 || store.outcomes != 1 || *reads != 3 {
		t.Fatalf("installer=%d started=%d outcomes=%d local-watch reads=%d; want 1/1/1/3", installs, store.started, store.outcomes, *reads)
	}
	if len(journal.decisions) != 3 || journal.decisions[0].Kind != "failure memory preflight" || journal.decisions[1].Determination != "started" || journal.decisions[2].Determination != "resolved" {
		t.Fatalf("journal sequence = %#v", journal.decisions)
	}
}

func TestMaatLivenessRepairRefusalAndEarlyFailuresMakeZeroInstallerCalls(t *testing.T) {
	tests := []struct {
		name      string
		configure func(*livenessFixtureStore, *livenessFixtureJournal)
		want      string
	}{
		{name: "active incident reject", configure: func(s *livenessFixtureStore, _ *livenessFixtureJournal) { s.decision = maat.PreflightReject }, want: "preflight reject"},
		{name: "decision journal read failure", configure: func(_ *livenessFixtureStore, j *livenessFixtureJournal) {
			j.recentErr = errors.New("journal unavailable")
		}, want: "journal"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &livenessFixtureStore{decision: maat.PreflightPass}
			journal := &livenessFixtureJournal{}
			tt.configure(store, journal)
			installLivenessCommandFixtures(t, store, journal, nil)
			installs := 0
			oldInstall := maatRepairInstallLiveness
			maatRepairInstallLiveness = func(maat.LivenessWatchManifest) (string, error) { installs++; return "", nil }
			err := maatRepairLivenessWatchCmd.RunE(maatRepairLivenessWatchCmd, nil)
			if err == nil || !strings.Contains(strings.ToLower(err.Error()), tt.want) || installs != 0 {
				t.Fatalf("error=%v installer calls=%d", err, installs)
			}
			maatRepairInstallLiveness = oldInstall
		})
	}
}

func TestMaatLivenessRepairStaleAdmissionRefusalMakesZeroInstallerCalls(t *testing.T) {
	store := &livenessFixtureStore{decision: maat.PreflightPass, beginErr: errors.New("registry changed after reviewed preflight")}
	journal := &livenessFixtureJournal{}
	installLivenessCommandFixtures(t, store, journal, nil)
	installs := 0
	maatRepairInstallLiveness = func(maat.LivenessWatchManifest) (string, error) { installs++; return "fixture install", nil }
	err := maatRepairLivenessWatchCmd.RunE(maatRepairLivenessWatchCmd, nil)
	if err == nil || !strings.Contains(err.Error(), "registry changed") || installs != 0 || store.started != 0 || store.outcomes != 0 {
		t.Fatalf("stale admission error=%v installs=%d starts=%d outcomes=%d", err, installs, store.started, store.outcomes)
	}
}

func TestMaatLivenessRepairUsesBoundedLocalFindingForLegacyUnloadedWatch(t *testing.T) {
	store := &livenessFixtureStore{decision: maat.PreflightPass}
	journal := &livenessFixtureJournal{}
	installs := installLivenessCommandFixtures(t, store, journal, nil)
	oldEngine, oldVerify, oldHome, oldInspect, oldFinding, oldDiagnosis, oldObserve :=
		maatRepairCurrentLocalEngine, maatRepairVerifyLocalEngine, maatRepairUserHomeDir,
		maatRepairInspectLocalWatch, maatRepairReadLivenessFinding, maatRepairReadDiagnosis, maatRepairObserveLiveness
	t.Cleanup(func() {
		maatRepairCurrentLocalEngine, maatRepairVerifyLocalEngine, maatRepairUserHomeDir = oldEngine, oldVerify, oldHome
		maatRepairInspectLocalWatch, maatRepairReadLivenessFinding = oldInspect, oldFinding
		maatRepairReadDiagnosis, maatRepairObserveLiveness = oldDiagnosis, oldObserve
	})
	engine, home := "/Applications/Pantheon.app/Contents/MacOS/sirsi", t.TempDir()
	maatRepairCurrentLocalEngine = func() (string, error) { return engine, nil }
	maatRepairVerifyLocalEngine = func(path string) error {
		if path != engine {
			t.Fatalf("package verifier path = %q, want %q", path, engine)
		}
		return nil
	}
	maatRepairUserHomeDir = func() (string, error) { return home, nil }
	reads := 0
	maatRepairInspectLocalWatch = func(_ context.Context, gotEngine, gotHome string, _ liveness.LocalCommand) (liveness.LocalObservation, error) {
		reads++
		if gotEngine != engine || gotHome != home {
			t.Fatalf("bounded inspector inputs = %q/%q, want %q/%q", gotEngine, gotHome, engine, home)
		}
		return liveness.LocalObservation{
			Check: "local-watch", State: "observed", Concern: true,
			Detail: "watch configuration is legacy and exact schedule is not loaded",
		}, nil
	}
	maatRepairReadLivenessFinding = readActionableLivenessFinding
	maatRepairReadDiagnosis = func() (*guard.DoctorReport, error) {
		t.Fatal("broad doctor must not control the bounded liveness-watch repair")
		return nil, nil
	}
	maatRepairObserveLiveness = func(manifest maat.LivenessWatchManifest) (maat.LivenessWatchObservation, error) {
		return maat.LivenessWatchObservation{
			UID: manifest.UID, Domain: manifest.Domain, Label: manifest.ManagedLabel,
			PlistPath: manifest.TargetPlistPath, PlistSHA256: manifest.ExpectedPlistSHA256, LaunchdLoaded: true,
		}, nil
	}
	maatRepairRevalidateExecutable = func(maat.LivenessWatchManifest) error { return nil }
	maatRepairInstallLiveness = func(maat.LivenessWatchManifest) (string, error) {
		(*installs)++
		return "fixture install", nil
	}
	maatRepairConfirm = true
	if err := maatRepairLivenessWatchCmd.RunE(maatRepairLivenessWatchCmd, nil); err != nil {
		t.Fatal(err)
	}
	if reads != 3 || *installs != 1 || store.started != 1 || store.outcomes != 1 {
		t.Fatalf("local reads=%d installs=%d starts=%d outcomes=%d", reads, *installs, store.started, store.outcomes)
	}
}

func TestReadActionableLivenessFindingRefusesUnknownOrDisabledState(t *testing.T) {
	oldEngine, oldVerify, oldHome, oldInspect :=
		maatRepairCurrentLocalEngine, maatRepairVerifyLocalEngine, maatRepairUserHomeDir, maatRepairInspectLocalWatch
	t.Cleanup(func() {
		maatRepairCurrentLocalEngine, maatRepairVerifyLocalEngine = oldEngine, oldVerify
		maatRepairUserHomeDir, maatRepairInspectLocalWatch = oldHome, oldInspect
	})
	maatRepairCurrentLocalEngine = func() (string, error) { return "/Applications/Pantheon.app/Contents/MacOS/sirsi", nil }
	maatRepairVerifyLocalEngine = func(string) error { return nil }
	maatRepairUserHomeDir = func() (string, error) { return t.TempDir(), nil }
	for _, tc := range []struct {
		name        string
		observation liveness.LocalObservation
		inspectErr  error
	}{
		{name: "operator-disabled", observation: liveness.LocalObservation{Check: "local-watch", State: "not-applicable"}},
		{name: "unknown-state", observation: liveness.LocalObservation{Check: "local-watch", State: "unavailable", Detail: "unknown"}},
		{name: "wrong-check", observation: liveness.LocalObservation{Check: "menubar", State: "observed", Concern: true}},
		{name: "inspector-error", inspectErr: errors.New("launchd state unavailable")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			maatRepairInspectLocalWatch = func(context.Context, string, string, liveness.LocalCommand) (liveness.LocalObservation, error) {
				return tc.observation, tc.inspectErr
			}
			_, found, err := readActionableLivenessFinding()
			if found || err == nil {
				t.Fatalf("unknown/disabled observation accepted: found=%v err=%v", found, err)
			}
		})
	}
}

func TestMaAtRepairUsesInstalledPackagedEngineAuthorityByDefault(t *testing.T) {
	if maatRepairCurrentLocalEngine == nil || maatRepairVerifyLocalEngine == nil {
		t.Fatal("packaged engine authority is not wired")
	}
	if _, err := setup.CurrentPackagedMaatEngine(); err == nil {
		t.Fatal("repository test executable unexpectedly qualified as an installed packaged engine")
	}
}

func TestMaatLivenessRepairFindingDriftAndPostconditionFailureNeverClaimSuccess(t *testing.T) {
	t.Run("finding drift before preflight", func(t *testing.T) {
		store := &livenessFixtureStore{decision: maat.PreflightPass}
		journal := &livenessFixtureJournal{}
		installLivenessCommandFixtures(t, store, journal, nil)
		reads := 0
		maatRepairReadLivenessFinding = func() (guard.DiagnosticFinding, bool, error) {
			reads++
			return guard.DiagnosticFinding{Check: maatRepairLivenessWatchCheck, Severity: guard.SeverityWarn, Message: fmt.Sprintf("finding-%d", reads), Detail: "bounded fixture"}, true, nil
		}
		installs := 0
		maatRepairInstallLiveness = func(maat.LivenessWatchManifest) (string, error) { installs++; return "", nil }
		err := maatRepairLivenessWatchCmd.RunE(maatRepairLivenessWatchCmd, nil)
		if err == nil || !strings.Contains(err.Error(), "changed after review") || installs != 0 || store.started != 0 {
			t.Fatalf("error=%v installs=%d starts=%d", err, installs, store.started)
		}
	})

	t.Run("negative exact observation", func(t *testing.T) {
		store := &livenessFixtureStore{decision: maat.PreflightPass}
		journal := &livenessFixtureJournal{}
		installLivenessCommandFixtures(t, store, journal, nil)
		store.observation = maat.LivenessWatchObservation{UID: 1, Label: "wrong"}
		installs := 0
		maatRepairInstallLiveness = func(maat.LivenessWatchManifest) (string, error) { installs++; return "fixture install", nil }
		err := maatRepairLivenessWatchCmd.RunE(maatRepairLivenessWatchCmd, nil)
		if err == nil || installs != 1 || store.outcomes != 1 || len(journal.decisions) != 3 || journal.decisions[2].Determination != "failed" {
			t.Fatalf("error=%v installs=%d outcomes=%d journal=%#v", err, installs, store.outcomes, journal.decisions)
		}
	})
}

func TestMaatLivenessRepairProjectionDebtReconcilesAfterUncertainAppendWithoutReplay(t *testing.T) {
	store := &livenessFixtureStore{decision: maat.PreflightPass}
	journal := &livenessFixtureJournal{failAfterAppend: true, failAfterAppendAt: 3}
	installLivenessCommandFixtures(t, store, journal, nil)
	installs := 0
	maatRepairInstallLiveness = func(maat.LivenessWatchManifest) (string, error) { installs++; return "fixture install", nil }
	err := maatRepairLivenessWatchCmd.RunE(maatRepairLivenessWatchCmd, nil)
	if err == nil || !strings.Contains(err.Error(), "Casebook projection remains pending") || installs != 1 || store.outcomes != 1 || store.projection == nil || store.projected {
		t.Fatalf("error=%v installs=%d outcomes=%d", err, installs, store.outcomes)
	}

	oldConfirm, oldReobserve := maatRepairConfirm, maatRepairReobserveFailureMemory
	t.Cleanup(func() { maatRepairConfirm, maatRepairReobserveFailureMemory = oldConfirm, oldReobserve })
	maatRepairConfirm, maatRepairReobserveFailureMemory = true, true
	if err := maatRepairLivenessWatchCmd.RunE(maatRepairLivenessWatchCmd, nil); err != nil {
		t.Fatal(err)
	}
	if installs != 1 || store.outcomes != 1 || !store.projected || len(journal.decisions) != 4 ||
		journal.decisions[2].Determination != "resolved" || journal.decisions[2].Time != store.projection.Decision.Time ||
		journal.decisions[2].Evidence != store.projection.Decision.Evidence ||
		journal.decisions[3].Kind != "liveness-watch observation" || journal.decisions[3].Determination != "exact" {
		t.Fatalf("reconcile replayed or duplicated: installs=%d outcomes=%d projected=%v journal=%#v", installs, store.outcomes, store.projected, journal.decisions)
	}
}

func TestMaatResolvedProjectionDebtReconcilesHistoryWhenWatchIsGoneThenAllowsFreshConsent(t *testing.T) {
	store := &livenessFixtureStore{decision: maat.PreflightPass}
	journal := &livenessFixtureJournal{failAfterAppend: true, failAfterAppendAt: 3}
	reads := installLivenessCommandFixtures(t, store, journal, nil)
	installs := 0
	oldInstall := maatRepairInstallLiveness
	maatRepairInstallLiveness = func(maat.LivenessWatchManifest) (string, error) { installs++; return "fixture install", nil }
	t.Cleanup(func() { maatRepairInstallLiveness = oldInstall })
	if err := maatRepairLivenessWatchCmd.RunE(maatRepairLivenessWatchCmd, nil); err == nil || store.projection == nil || store.projection.Outcome.Determination != "resolved" {
		t.Fatalf("initial result err=%v debt=%+v", err, store.projection)
	}
	historicalTime := store.projection.Outcome.RecordedAtUTC
	historicalDecisionTime := store.projection.Decision.Time
	oldConfirm, oldReobserve, oldObserve := maatRepairConfirm, maatRepairReobserveFailureMemory, maatRepairObserveLiveness
	t.Cleanup(func() {
		maatRepairConfirm, maatRepairReobserveFailureMemory, maatRepairObserveLiveness = oldConfirm, oldReobserve, oldObserve
	})
	maatRepairConfirm, maatRepairReobserveFailureMemory = true, true
	maatRepairObserveLiveness = func(maat.LivenessWatchManifest) (maat.LivenessWatchObservation, error) {
		return maat.LivenessWatchObservation{}, errors.New("fixture watch disappeared after original resolved outcome")
	}
	if err := maatRepairLivenessWatchCmd.RunE(maatRepairLivenessWatchCmd, nil); err == nil || !strings.Contains(err.Error(), "current watch is not verified") {
		t.Fatalf("reconciliation error=%v, want current-state refusal after historical completion", err)
	}
	historicalAt, historicalTimeErr := time.Parse(time.RFC3339Nano, historicalDecisionTime)
	currentAt, currentTimeErr := time.Parse(time.RFC3339Nano, journal.decisions[3].Time)
	if installs != 1 || !store.projected ||
		store.projection.Outcome.Determination != "resolved" || !store.projection.Outcome.RecordedAtUTC.Equal(historicalTime) ||
		store.projection.Decision.Time != historicalDecisionTime || len(journal.decisions) != 4 ||
		journal.decisions[2].Determination != "resolved" || journal.decisions[3].Determination != "unverifiable" ||
		!strings.Contains(journal.decisions[3].Why, "current watch could not be verified") || historicalTimeErr != nil || currentTimeErr != nil || !currentAt.After(historicalAt) {
		t.Fatalf("historical/current split or no-replay invariant failed: installs=%d projected=%v debt=%+v journal=%#v", installs, store.projected, store.projection, journal.decisions)
	}
	// A later repair must pass through the ordinary fresh diagnosis/consent
	// path; clearing projection debt itself never calls the installer.
	maatRepairReobserveFailureMemory = false
	maatRepairObserveLiveness = oldObserve
	*reads = 0
	if err := maatRepairLivenessWatchCmd.RunE(maatRepairLivenessWatchCmd, nil); err != nil {
		t.Fatalf("freshly consented repair after historical reconciliation: %v", err)
	}
	if installs != 2 || store.started != 2 {
		t.Fatalf("fresh consent did not begin one new attempt: installs=%d started=%d", installs, store.started)
	}
}

func TestMaatCurrentObservationAppendFailureClosesHistoryAndAllowsFreshConsent(t *testing.T) {
	store := &livenessFixtureStore{decision: maat.PreflightPass}
	journal := &livenessFixtureJournal{failAfterAppend: true, failAfterAppendAt: 3}
	reads := installLivenessCommandFixtures(t, store, journal, nil)
	installs := 0
	oldInstall := maatRepairInstallLiveness
	maatRepairInstallLiveness = func(maat.LivenessWatchManifest) (string, error) { installs++; return "fixture install", nil }
	t.Cleanup(func() { maatRepairInstallLiveness = oldInstall })
	if err := maatRepairLivenessWatchCmd.RunE(maatRepairLivenessWatchCmd, nil); err == nil || store.projection == nil || store.projected {
		t.Fatalf("initial projection result err=%v projected=%v", err, store.projected)
	}
	oldConfirm, oldReobserve := maatRepairConfirm, maatRepairReobserveFailureMemory
	t.Cleanup(func() { maatRepairConfirm, maatRepairReobserveFailureMemory = oldConfirm, oldReobserve })
	maatRepairConfirm, maatRepairReobserveFailureMemory = true, true
	journal.failAt = len(journal.decisions) + 1
	if err := maatRepairLivenessWatchCmd.RunE(maatRepairLivenessWatchCmd, nil); err == nil || !strings.Contains(err.Error(), "current observation is unknown") {
		t.Fatalf("current append error=%v, want explicit unknown/unrecorded result", err)
	}
	if installs != 1 || !store.projected || len(journal.decisions) != 3 {
		t.Fatalf("current append failure reopened history or replayed installer: installs=%d projected=%v journal=%#v", installs, store.projected, journal.decisions)
	}
	maatRepairReobserveFailureMemory = false
	journal.failAt = 0
	*reads = 0
	if err := maatRepairLivenessWatchCmd.RunE(maatRepairLivenessWatchCmd, nil); err != nil {
		t.Fatalf("freshly consented repair after current-fact append failure: %v", err)
	}
	if installs != 2 || store.started != 2 {
		t.Fatalf("fresh consent did not start exactly one new attempt: installs=%d started=%d", installs, store.started)
	}
}

func TestMaatLivenessProjectionJSONFailureReturnsNonzero(t *testing.T) {
	oldJSON, oldGlobalJSON := maatJSON, JsonOutput
	maatJSON, JsonOutput = true, false
	t.Cleanup(func() { maatJSON, JsonOutput = oldJSON, oldGlobalJSON })
	outcome := maat.LivenessWatchOutcome{Determination: "incomplete"}
	if err := renderLivenessProjectionResult("b3c515a2-4773-4653-86d1-8a5734c2ee45", outcome, "fixture"); err == nil {
		t.Fatal("JSON rendering returned success for a failed liveness-watch outcome")
	}
	debt := maat.LivenessWatchProjectionDebt{
		Attempt: maat.LivenessWatchAttempt{ID: "b3c515a2-4773-4653-86d1-8a5734c2ee45"},
		Outcome: outcome,
	}
	if err := renderLivenessProjectionCurrentResult(debt, "exact", time.Now().UTC(), "current-evidence", true, nil); err == nil {
		t.Fatal("an exact later observation upgraded an incomplete historical repair outcome")
	}
}

func TestMaatLivenessMemoryInitializationIsExplicitFirstUseAndJournaled(t *testing.T) {
	oldConfirm, oldInitialize, oldInspect := maatRepairConfirm, maatRepairDefaultStoreInitialize, maatRepairDefaultStoreInspect
	oldJournal, oldJSON, oldGlobalJSON := newMaatDecisionJournal, maatJSON, JsonOutput
	oldInitializeFlag := maatRepairInitializeFailureMemory
	t.Cleanup(func() {
		maatRepairConfirm, maatRepairDefaultStoreInitialize, maatRepairDefaultStoreInspect = oldConfirm, oldInitialize, oldInspect
		newMaatDecisionJournal, maatJSON, JsonOutput = oldJournal, oldJSON, oldGlobalJSON
		maatRepairInitializeFailureMemory = oldInitializeFlag
	})
	maatRepairConfirm, maatRepairInitializeFailureMemory = true, true
	maatJSON, JsonOutput = false, false
	maatRepairDefaultStoreInspect = func() maat.DefaultStoreStatus { return maat.DefaultStoreStatus{State: maat.DefaultStoreUninitialized} }
	initializations := 0
	maatRepairDefaultStoreInitialize = func() (string, error) {
		initializations++
		return "b3c515a2-4773-4653-86d1-8a5734c2ee45", nil
	}
	journal := &livenessFixtureJournal{}
	newMaatDecisionJournal = func() (maat.DecisionJournal, error) { return journal, nil }
	if err := maatRepairLivenessWatchCmd.RunE(maatRepairLivenessWatchCmd, nil); err != nil {
		t.Fatal(err)
	}
	if initializations != 1 || len(journal.decisions) != 1 || journal.decisions[0].Determination != "initialized" || !strings.Contains(journal.decisions[0].Why, "no incidents imported") {
		t.Fatalf("initializations=%d journal=%#v", initializations, journal.decisions)
	}
}

func TestMaatLivenessMemoryReobserveDoesNotReplayInstaller(t *testing.T) {
	store := &livenessFixtureStore{decision: maat.PreflightPass, pending: true}
	manifest := fixtureCommandManifest("b3c515a2-4773-4653-86d1-8a5734c2ee45", "watch absent")
	store.attempt = maat.LivenessWatchAttempt{Schema: maat.MemorySchema, ID: "b3c515a2-4773-4653-86d1-8a5734c2ee45", Manifest: manifest}
	journal := &livenessFixtureJournal{}
	oldConfirm, oldReobserve, oldOpen, oldObserve, oldJournal, oldJSON, oldGlobalJSON := maatRepairConfirm, maatRepairReobserveFailureMemory, maatRepairOpenDefaultStore, maatRepairObserveLiveness, newMaatDecisionJournal, maatJSON, JsonOutput
	t.Cleanup(func() {
		maatRepairConfirm, maatRepairReobserveFailureMemory, maatRepairOpenDefaultStore = oldConfirm, oldReobserve, oldOpen
		maatRepairObserveLiveness, newMaatDecisionJournal, maatJSON, JsonOutput = oldObserve, oldJournal, oldJSON, oldGlobalJSON
	})
	maatRepairConfirm, maatRepairReobserveFailureMemory = true, true
	maatJSON, JsonOutput = false, false
	maatRepairOpenDefaultStore = func() (maatLivenessStore, error) { return store, nil }
	maatRepairObserveLiveness = func(m maat.LivenessWatchManifest) (maat.LivenessWatchObservation, error) {
		return exactObservation(m), nil
	}
	newMaatDecisionJournal = func() (maat.DecisionJournal, error) { return journal, nil }
	installs := 0
	oldInstall := maatRepairInstallLiveness
	maatRepairInstallLiveness = func(maat.LivenessWatchManifest) (string, error) { installs++; return "", nil }
	t.Cleanup(func() { maatRepairInstallLiveness = oldInstall })
	if err := maatRepairLivenessWatchCmd.RunE(maatRepairLivenessWatchCmd, nil); err != nil {
		t.Fatal(err)
	}
	if installs != 0 || store.outcomes != 1 || len(journal.decisions) != 1 || journal.decisions[0].Determination != "resolved" {
		t.Fatalf("installs=%d outcomes=%d journal=%#v", installs, store.outcomes, journal.decisions)
	}
}

func TestMaatLivenessMemoryInitializationRequiresItsOwnConfirmation(t *testing.T) {
	oldConfirm, oldInitializeFlag, oldInspect, oldInit := maatRepairConfirm, maatRepairInitializeFailureMemory, maatRepairDefaultStoreInspect, maatRepairDefaultStoreInitialize
	t.Cleanup(func() {
		maatRepairConfirm, maatRepairInitializeFailureMemory = oldConfirm, oldInitializeFlag
		maatRepairDefaultStoreInspect, maatRepairDefaultStoreInitialize = oldInspect, oldInit
	})
	maatRepairConfirm, maatRepairInitializeFailureMemory = false, true
	called := false
	maatRepairDefaultStoreInspect = func() maat.DefaultStoreStatus {
		called = true
		return maat.DefaultStoreStatus{State: maat.DefaultStoreUninitialized}
	}
	maatRepairDefaultStoreInitialize = func() (string, error) { called = true; return "", nil }
	err := maatRepairLivenessWatchCmd.RunE(maatRepairLivenessWatchCmd, nil)
	if err == nil || !strings.Contains(err.Error(), "--confirm") || called {
		t.Fatalf("error=%v called=%v", err, called)
	}
}

func fixtureCommandManifest(storeUUID, finding string) maat.LivenessWatchManifest {
	uid := uint32(os.Geteuid())
	root := "/tmp/pantheon-maat-fixture"
	findingDigest := fmt.Sprintf("%064x", sha256.Sum256([]byte(finding)))
	return maat.LivenessWatchManifest{
		Schema: maat.LivenessWatchManifestSchema, UID: uid, Domain: fmt.Sprintf("gui/%d", uid),
		ExecutablePath: root + "/sirsi", ExecutableDevice: 1, ExecutableInode: 2, ExecutableMode: uint32(unix.S_IFREG) | 0o755,
		ExecutableSize: 1, ExecutableSHA256: strings.Repeat("a", 64), WorkDirectory: root,
		ManagedLabel: "ai.sirsi.liveness-watch", TargetPlistPath: root + "/watch.plist",
		ExpectedPlistSHA256: strings.Repeat("b", 64), FindingCheck: "liveness-watch", FindingSeverity: 2,
		FindingEvidenceSHA256: findingDigest, FindingObservedAtUTC: time.Now().UTC(), StoreUUID: storeUUID,
	}
}

func exactObservation(m maat.LivenessWatchManifest) maat.LivenessWatchObservation {
	return maat.LivenessWatchObservation{UID: m.UID, Domain: m.Domain, Label: m.ManagedLabel, PlistPath: m.TargetPlistPath, PlistSHA256: m.ExpectedPlistSHA256, LaunchdLoaded: true}
}

func TestBuildLivenessWatchManifestBindsCanonicalEngineAndHomeLocalPlist(t *testing.T) {
	home := t.TempDir()
	engine := filepath.Join(t.TempDir(), "sirsi")
	if err := os.WriteFile(engine, []byte("inert canonical engine fixture"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(engine, 0o755); err != nil {
		t.Fatal(err)
	}
	oldEngine, oldVerify, oldHome := maatRepairCurrentLocalEngine, maatRepairVerifyLocalEngine, maatRepairUserHomeDir
	maatRepairCurrentLocalEngine = func() (string, error) { return engine, nil }
	maatRepairVerifyLocalEngine = func(path string) error {
		if path != engine {
			t.Fatalf("signature verifier received %q, want %q", path, engine)
		}
		return nil
	}
	maatRepairUserHomeDir = func() (string, error) { return home, nil }
	t.Cleanup(func() {
		maatRepairCurrentLocalEngine, maatRepairVerifyLocalEngine, maatRepairUserHomeDir = oldEngine, oldVerify, oldHome
	})
	finding := guard.DiagnosticFinding{Check: maatRepairLivenessWatchCheck, Severity: guard.SeverityWarn, Message: "fixture watch absent", Detail: "bounded fixture"}
	manifest, err := buildLivenessWatchManifest(finding, "b3c515a2-4773-4653-86d1-8a5734c2ee45")
	if err != nil {
		t.Fatal(err)
	}
	wantPath := filepath.Join(home, "Library", "LaunchAgents", liveness.Label+".plist")
	wantDigest := sha256.Sum256([]byte(liveness.LocalPlistContent(engine, home)))
	if manifest.ExecutablePath != engine || manifest.WorkDirectory != home || manifest.TargetPlistPath != wantPath || manifest.ExpectedPlistSHA256 != fmt.Sprintf("%x", wantDigest) {
		t.Fatalf("manifest is not bound to local engine/HOME contract: %+v", manifest)
	}
}

func TestBuildLivenessWatchManifestRejectsSymlinkedHome(t *testing.T) {
	parent := t.TempDir()
	engine := filepath.Join(t.TempDir(), "sirsi")
	if err := os.WriteFile(engine, []byte("inert canonical engine fixture"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(engine, 0o755); err != nil {
		t.Fatal(err)
	}
	realHome := t.TempDir()
	homeLink := filepath.Join(parent, "home-link")
	if err := os.Symlink(realHome, homeLink); err != nil {
		t.Fatal(err)
	}
	oldEngine, oldVerify, oldHome := maatRepairCurrentLocalEngine, maatRepairVerifyLocalEngine, maatRepairUserHomeDir
	maatRepairCurrentLocalEngine = func() (string, error) { return engine, nil }
	maatRepairVerifyLocalEngine = func(path string) error {
		if path != engine {
			t.Fatalf("signature verifier received %q, want %q", path, engine)
		}
		return nil
	}
	maatRepairUserHomeDir = func() (string, error) { return homeLink, nil }
	t.Cleanup(func() {
		maatRepairCurrentLocalEngine, maatRepairVerifyLocalEngine, maatRepairUserHomeDir = oldEngine, oldVerify, oldHome
	})

	finding := guard.DiagnosticFinding{Check: maatRepairLivenessWatchCheck, Severity: guard.SeverityWarn, Message: "fixture watch absent", Detail: "bounded fixture"}
	if _, err := buildLivenessWatchManifest(finding, "b3c515a2-4773-4653-86d1-8a5734c2ee45"); err == nil || !strings.Contains(err.Error(), "inspect local liveness-watch HOME") {
		t.Fatalf("symlinked HOME was not refused at no-follow inspection: %v", err)
	}
}

func TestMaatLivenessAdapterDelegatesObserveAndInstallToLocalModeAPIs(t *testing.T) {
	manifest := fixtureCommandManifest("b3c515a2-4773-4653-86d1-8a5734c2ee45", "adapter fixture")
	manifest.WorkDirectory = "/tmp/maat-local-home"
	manifest.TargetPlistPath = filepath.Join(manifest.WorkDirectory, "Library", "LaunchAgents", liveness.Label+".plist")
	digest := sha256.Sum256([]byte(liveness.LocalPlistContent(manifest.ExecutablePath, manifest.WorkDirectory)))
	manifest.ExpectedPlistSHA256 = fmt.Sprintf("%x", digest)
	called := false
	observation, err := observeLivenessWatchWithLocalInspector(context.Background(), manifest, func(_ context.Context, engine, home string, command liveness.LocalCommand) (liveness.LocalObservation, error) {
		called = true
		if engine != manifest.ExecutablePath || home != manifest.WorkDirectory || command != nil {
			t.Fatalf("inspector received noncanonical local-mode identity: engine=%q home=%q command=%v", engine, home, command != nil)
		}
		return liveness.LocalObservation{Check: "local-watch", State: "observed", Detail: "exact local metadata schedule is loaded", ConfigurationSHA256: manifest.ExpectedPlistSHA256}, nil
	})
	if err != nil || !called || observation != exactObservation(manifest) {
		t.Fatalf("local API observation = %+v called=%v err=%v", observation, called, err)
	}
	_, err = observeLivenessWatchWithLocalInspector(context.Background(), manifest, func(context.Context, string, string, liveness.LocalCommand) (liveness.LocalObservation, error) {
		return liveness.LocalObservation{Check: "local-watch", State: "unavailable", Detail: "fixture unavailable"}, nil
	})
	if err == nil {
		t.Fatal("unknown local watch state was accepted as an exact postcondition")
	}
	_, err = observeLivenessWatchWithLocalInspector(context.Background(), manifest, func(context.Context, string, string, liveness.LocalCommand) (liveness.LocalObservation, error) {
		return liveness.LocalObservation{Check: "local-watch", State: "observed", Detail: "loaded but changed bytes", ConfigurationSHA256: strings.Repeat("c", 64)}, nil
	})
	if err == nil {
		t.Fatal("local watch with changed plist bytes was accepted as the reviewed postcondition")
	}

	installCalled := false
	message, err := installLivenessWatchWithLocalInstaller(manifest, func(_ context.Context, engine, home string, command liveness.LocalCommand) (string, error) {
		installCalled = true
		if engine != manifest.ExecutablePath || home != manifest.WorkDirectory || command != nil {
			t.Fatalf("installer received noncanonical local-mode identity: engine=%q home=%q command=%v", engine, home, command != nil)
		}
		return "local fixture install", nil
	})
	if err != nil || !installCalled || message != "local fixture install" {
		t.Fatalf("local API install = %q called=%v err=%v", message, installCalled, err)
	}
}
