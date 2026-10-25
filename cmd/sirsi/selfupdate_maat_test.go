package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/maat"
	"github.com/SirsiMaster/sirsi-pantheon/internal/routerstore"
	"github.com/SirsiMaster/sirsi-pantheon/internal/selfupdate"
	modversion "github.com/SirsiMaster/sirsi-pantheon/internal/version"
)

type rawSelfUpdateProjectionOutageJournal struct{ maat.DecisionJournal }

func (rawSelfUpdateProjectionOutageJournal) AppendIdempotent(maat.Decision) error {
	return errors.New("injected raw self-update projection outage")
}

func (j rawSelfUpdateProjectionOutageJournal) FindByEvidence(kind, evidence string) (maat.Decision, bool, error) {
	reader, ok := j.DecisionJournal.(interface {
		FindByEvidence(string, string) (maat.Decision, bool, error)
	})
	if !ok {
		return maat.Decision{}, false, errors.New("fixture journal lacks exact readback")
	}
	return reader.FindByEvidence(kind, evidence)
}

func canonicalMaatFixtureDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	canonical, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatalf("resolve canonical private Ma'at fixture path: %v", err)
	}
	return canonical
}

func configureMaatSelfUpdateCommandFixture(t *testing.T, forceHomebrew bool) (string, string, *int) {
	t.Helper()
	home := canonicalMaatFixtureDir(t)
	t.Setenv("HOME", home)
	if _, err := maat.InitializeDefaultStore(); err != nil {
		t.Fatalf("initialize private Ma'at fixture: %v", err)
	}
	binDir := canonicalMaatFixtureDir(t)
	selfPath, targetPath := filepath.Join(binDir, "sirsi-self"), filepath.Join(binDir, "sirsi-target")
	if err := os.WriteFile(selfPath, []byte("running stamped sirsi"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(targetPath, []byte("stale sirsi"), 0o755); err != nil {
		t.Fatal(err)
	}
	selfHash, err := selfupdate.FileHash(selfPath)
	if err != nil {
		t.Fatal(err)
	}
	targetHash, err := selfupdate.FileHash(targetPath)
	if err != nil {
		t.Fatal(err)
	}
	info := modversion.Info{Binary: "sirsi", Version: "0.24.137", Commit: "fixture", Date: "2026-10-25",
		Path: selfPath, GoVer: "go-fixture", RouterSchemaMax: routerstore.MaxSupportedSchemaVersion()}
	oldOps := maatRawSelfUpdateOps
	oldExecutable, oldHash, oldDetect := selfUpdateExecutable, selfUpdateHash, selfUpdateDetect
	oldSchemaGate, oldPrompt := selfUpdateSchemaGate, selfUpdateConfirmPrompt
	oldConfirm, oldJSON, oldReconcile := selfUpdateConfirm, selfUpdateJSON, selfUpdateReconcile
	replacements := 0
	maatRawSelfUpdateOps = maatRawSelfUpdateDependencies{
		storeStatus: maat.InspectDefaultStore,
		openStore:   func() (maatRawSelfUpdateStore, error) { return maat.OpenDefaultStore() },
		journal:     func() (maat.DecisionJournal, error) { return maat.NewDefaultDecisionJournal() },
		project:     maat.ProjectFailureMemoryPreflight,
		selfInfo:    func() modversion.Info { return info },
		probe:       func(string) (modversion.Info, error) { return info, nil },
		schema:      func(modversion.Info) (int, error) { return 0, nil },
		identity:    binaryDriftFileIdentity,
		method: func(string) selfupdate.Method {
			if forceHomebrew {
				return selfupdate.MethodHomebrew
			}
			return selfupdate.MethodRaw
		},
		lock: selfupdate.AcquireInstallLock,
		replace: func(src, dst string) error {
			replacements++
			body, err := os.ReadFile(src)
			if err != nil {
				return err
			}
			staged, err := os.CreateTemp(filepath.Dir(dst), ".sirsi-inert-replace-")
			if err != nil {
				return err
			}
			stagedPath := staged.Name()
			defer os.Remove(stagedPath)
			if err := staged.Chmod(0o755); err != nil {
				_ = staged.Close()
				return err
			}
			if _, err := staged.Write(body); err != nil {
				_ = staged.Close()
				return err
			}
			if err := staged.Close(); err != nil {
				return err
			}
			return os.Rename(stagedPath, dst)
		},
		now: time.Now,
	}
	selfUpdateExecutable = func() (string, error) { return selfPath, nil }
	selfUpdateHash = selfupdate.FileHash
	selfUpdateDetect = func(string, string) []selfupdate.DriftTarget {
		return []selfupdate.DriftTarget{{Path: targetPath, Present: targetHash, Expected: selfHash}}
	}
	selfUpdateSchemaGate = func(modversion.Info) error { return nil }
	selfUpdateConfirmPrompt = func(string) bool { return true }
	selfUpdateConfirm, selfUpdateJSON, selfUpdateReconcile = true, false, false
	t.Cleanup(func() {
		maatRawSelfUpdateOps = oldOps
		selfUpdateExecutable, selfUpdateHash, selfUpdateDetect = oldExecutable, oldHash, oldDetect
		selfUpdateSchemaGate, selfUpdateConfirmPrompt = oldSchemaGate, oldPrompt
		selfUpdateConfirm, selfUpdateJSON, selfUpdateReconcile = oldConfirm, oldJSON, oldReconcile
	})
	return selfPath, targetPath, &replacements
}

func TestRunSelfUpdateCobraRouteRetainsReceiptAttemptAndExactTerminalReadback(t *testing.T) {
	_, _, replacements := configureMaatSelfUpdateCommandFixture(t, false)
	if err := selfUpdateCmd.RunE(selfUpdateCmd, nil); err != nil {
		t.Fatalf("confirmed self-update command: %v", err)
	}
	if *replacements != 1 {
		t.Fatalf("SafeReplace seam calls=%d, want one", *replacements)
	}
	store, err := maat.OpenDefaultStore()
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	history, err := store.RawSelfUpdateReconciliations()
	if err != nil || len(history) != 1 || history[0].Outcome == nil || !history[0].Outcome.PostcheckPassed || !history[0].ProjectionReadBack {
		t.Fatalf("exact attempt/outcome/projection not retained: history=%+v err=%v", history, err)
	}
	journal, err := maat.NewDefaultDecisionJournal()
	if err != nil {
		t.Fatal(err)
	}
	decisions, err := journal.Recent(32)
	if err != nil {
		t.Fatal(err)
	}
	var preflight, terminal bool
	for _, decision := range decisions {
		preflight = preflight || decision.Kind == "failure memory preflight" && decision.Assessed == "update-cli-binary-drift"
		terminal = terminal || decision.Kind == "diagnostic verified repair" && decision.Determination == "resolved"
	}
	if !preflight || !terminal {
		t.Fatalf("canonical preflight or exact terminal Casebook projection missing: preflight=%t terminal=%t", preflight, terminal)
	}
}

func TestRunSelfUpdateCobraRouteDoesNotReplaceHomebrewOwnedTarget(t *testing.T) {
	_, _, replacements := configureMaatSelfUpdateCommandFixture(t, true)
	err := selfUpdateCmd.RunE(selfUpdateCmd, nil)
	if err == nil {
		t.Fatal("raw self-update accepted a Homebrew-owned target")
	}
	if errors.Is(err, selfupdate.ErrHomebrewManaged) {
		t.Fatal("route delegated to SafeReplace instead of refusing before mutation")
	}
	if *replacements != 0 {
		t.Fatalf("Homebrew-owned target reached replacement seam %d times", *replacements)
	}
}

func TestRunSelfUpdateCobraRoutePostcheckFailureIsRetainedFailedAndNeverRetried(t *testing.T) {
	_, _, _ = configureMaatSelfUpdateCommandFixture(t, false)
	replacements := 0
	maatRawSelfUpdateOps.replace = func(_, _ string) error {
		replacements++
		return nil // Simulate a nominal updater return without installing the reviewed bytes.
	}
	err := selfUpdateCmd.RunE(selfUpdateCmd, nil)
	if err == nil {
		t.Fatal("a no-op updater return with unchanged target was reported as success")
	}
	if replacements != 1 {
		t.Fatalf("raw updater calls=%d, want exactly one failed-postcheck attempt", replacements)
	}
	store, err := maat.OpenDefaultStore()
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	history, err := store.RawSelfUpdateReconciliations()
	if err != nil || len(history) != 1 || history[0].Outcome == nil || history[0].Outcome.Determination != "failed" ||
		history[0].Outcome.PostcheckPassed || !history[0].ProjectionReadBack {
		t.Fatalf("failed exact postcheck was not retained/projected as failure: history=%+v err=%v", history, err)
	}
	if err := reconcilePendingRawSelfUpdate(); err != nil {
		t.Fatalf("data-only failed-outcome readback: %v", err)
	}
	if replacements != 1 {
		t.Fatalf("failed outcome reconciliation retried the updater; calls=%d", replacements)
	}
}

func TestRunSelfUpdateCobraRouteProjectionDebtNeverRetriesReplacement(t *testing.T) {
	_, _, replacements := configureMaatSelfUpdateCommandFixture(t, false)
	originalJournal := maatRawSelfUpdateOps.journal
	maatRawSelfUpdateOps.journal = func() (maat.DecisionJournal, error) {
		journal, err := originalJournal()
		if err != nil {
			return nil, err
		}
		return rawSelfUpdateProjectionOutageJournal{DecisionJournal: journal}, nil
	}
	err := selfUpdateCmd.RunE(selfUpdateCmd, nil)
	if err == nil {
		t.Fatal("injected terminal projection outage was hidden")
	}
	if *replacements != 1 {
		t.Fatalf("first confirmed run called replacement %d times, want once", *replacements)
	}
	store, err := maat.OpenDefaultStore()
	if err != nil {
		t.Fatal(err)
	}
	history, err := store.RawSelfUpdateReconciliations()
	if err != nil || len(history) != 1 || history[0].Outcome == nil || history[0].ProjectionReadBack {
		_ = store.Close()
		t.Fatalf("terminal projection debt was not retained: history=%+v err=%v", history, err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	if err := reconcilePendingRawSelfUpdate(); err == nil {
		t.Fatal("injected projection outage unexpectedly reconciled")
	}
	if *replacements != 1 {
		t.Fatalf("projection reconciliation retried replacement; calls=%d", *replacements)
	}
}

func TestRunSelfUpdateCobraRouteRecurrenceRejectHasZeroReplacementEffects(t *testing.T) {
	_, _, replacements := configureMaatSelfUpdateCommandFixture(t, false)
	store, err := maat.OpenDefaultStore()
	if err != nil {
		t.Fatal(err)
	}
	evidence, err := store.PutEvidence([]byte("inert raw-self-update recurrence fixture"))
	if err != nil {
		t.Fatal(err)
	}
	incident, err := maat.NewIncident(
		maat.FailureSignature{Namespace: "pantheon", Version: "v1", FailureClass: "binary-drift", Operation: "self-update", Component: "sirsi", Invariant: "cli-targets-converge", ErrorCode: "stale-target"},
		evidence,
		maat.Scope{Component: "pantheon", Profile: "local-macos", Operation: "update-cli-binary-drift"},
		maat.ActiveIncidentGuardID, maat.ActiveIncidentGuardVersion,
		[]maat.RecoveryAction{{ID: "review", Label: "Review the target", Instruction: "Keep the target unchanged until reviewed."}},
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Append(incident); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	err = selfUpdateCmd.RunE(selfUpdateCmd, nil)
	if err == nil {
		t.Fatal("active recurrence unexpectedly authorized raw self-update")
	}
	if *replacements != 0 {
		t.Fatalf("rejected recurrence reached replacement seam %d times", *replacements)
	}
	store, err = maat.OpenDefaultStore()
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	journal, err := maat.NewDefaultDecisionJournal()
	if err != nil {
		t.Fatal(err)
	}
	decisions, err := journal.Recent(32)
	if err != nil {
		t.Fatal(err)
	}
	var receiptDigest string
	for _, decision := range decisions {
		if decision.Kind == "failure memory preflight" && decision.Assessed == "update-cli-binary-drift" {
			marker := "receipt-sha256="
			if index := strings.Index(decision.Evidence, marker); index >= 0 {
				receiptDigest = strings.SplitN(decision.Evidence[index+len(marker):], ";", 2)[0]
			}
		}
	}
	readback, err := store.ReadPreflightReceipt(receiptDigest)
	if err != nil || readback.Receipt.Decision != maat.PreflightReject {
		t.Fatalf("rejected receipt was not retained: receipt=%+v err=%v", readback, err)
	}
}

func retainRawSelfUpdateIntentForReconciliation(t *testing.T, selfPath string) (*maat.Store, maat.RawSelfUpdateAttempt) {
	t.Helper()
	store, err := maat.OpenDefaultStore()
	if err != nil {
		t.Fatal(err)
	}
	selfHash, err := selfupdate.FileHash(selfPath)
	if err != nil {
		_ = store.Close()
		t.Fatal(err)
	}
	info, err := maatRawSelfUpdateOps.probe(selfPath)
	if err != nil {
		_ = store.Close()
		t.Fatal(err)
	}
	status := maatRawSelfUpdateOps.storeStatus()
	manifest, err := rawSelfUpdateManifest(selfPath, selfHash, selfUpdateDetect(selfPath, selfHash), status.StoreUUID, info, 0, time.Now().UTC())
	if err != nil {
		_ = store.Close()
		t.Fatal(err)
	}
	receipt, err := store.PreflightRawSelfUpdate(manifest)
	if err != nil {
		_ = store.Close()
		t.Fatal(err)
	}
	stored, err := store.RetainPreflightReceipt(receipt)
	if err != nil {
		_ = store.Close()
		t.Fatal(err)
	}
	journal, err := maatRawSelfUpdateOps.journal()
	if err != nil {
		_ = store.Close()
		t.Fatal(err)
	}
	if err := maatRawSelfUpdateOps.project(journal, stored); err != nil {
		_ = store.Close()
		t.Fatal(err)
	}
	attempt, err := store.BeginRawSelfUpdateAttempt(manifest, stored)
	if err != nil {
		_ = store.Close()
		t.Fatal(err)
	}
	return store, attempt
}

func TestRunSelfUpdateReconcileClosesKnownPreInvocationGapWithoutReplacement(t *testing.T) {
	selfPath, _, replacements := configureMaatSelfUpdateCommandFixture(t, false)
	store, _ := retainRawSelfUpdateIntentForReconciliation(t, selfPath)
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	if err := reconcilePendingRawSelfUpdate(); err != nil {
		t.Fatalf("data-only pre-invocation reconciliation: %v", err)
	}
	if *replacements != 0 {
		t.Fatalf("reconciliation reached replacement seam %d times", *replacements)
	}
	store, err := maat.OpenDefaultStore()
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	history, err := store.RawSelfUpdateReconciliations()
	if err != nil || len(history) != 1 || history[0].InvocationRecorded || history[0].Outcome == nil ||
		history[0].Outcome.Determination != "not_invoked" || history[0].Outcome.FactCode != "reconcile_no_invocation_marker" || !history[0].ProjectionReadBack {
		t.Fatalf("pre-invocation gap was not closed by exact retained terminal projection: history=%+v err=%v", history, err)
	}
}

func TestRunSelfUpdateReconcileLeavesUnknownInvocationUnresolvedWithoutReplacement(t *testing.T) {
	selfPath, _, replacements := configureMaatSelfUpdateCommandFixture(t, false)
	store, attempt := retainRawSelfUpdateIntentForReconciliation(t, selfPath)
	if err := store.RecordRawSelfUpdateInvocation(attempt); err != nil {
		_ = store.Close()
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	if err := reconcilePendingRawSelfUpdate(); err == nil {
		t.Fatal("reconciliation treated an invocation without a terminal outcome as safe to retry")
	}
	if *replacements != 0 {
		t.Fatalf("unknown invocation reached replacement seam %d times", *replacements)
	}
	store, err := maat.OpenDefaultStore()
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	history, err := store.RawSelfUpdateReconciliations()
	if err != nil || len(history) != 1 || !history[0].InvocationRecorded || history[0].Outcome != nil {
		t.Fatalf("unknown invocation was not preserved unresolved: history=%+v err=%v", history, err)
	}
}
