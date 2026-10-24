package maat

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func TestLivenessExecutableIdentityAllowsInstalledRootOwnerAndRejectsUnsafeFiles(t *testing.T) {
	actor := uint32(os.Geteuid())
	regular0755 := uint32(unix.S_IFREG) | 0o755
	for _, tc := range []struct {
		name               string
		actor, owner, mode uint32
		want               bool
	}{
		{"actor-owned", actor, actor, regular0755, true},
		{"root-owned-installed-app", actor, 0, regular0755, true},
		{"unrelated-owner", actor, actor + 1, regular0755, false},
		{"group-writable", actor, 0, uint32(unix.S_IFREG) | 0o775, false},
		{"other-writable", actor, actor, uint32(unix.S_IFREG) | 0o757, false},
		{"not-executable", actor, 0, uint32(unix.S_IFREG) | 0o644, false},
		{"setuid", actor, 0, regular0755 | uint32(unix.S_ISUID), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := LivenessExecutableIdentityAllowed(tc.actor, tc.owner, tc.mode); got != tc.want {
				t.Fatalf("identity allowed = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestLivenessWatchManifestBindsTargetAndCurrentOwnerBeyondScope(t *testing.T) {
	storeUUID := "b3c515a2-4773-4653-86d1-8a5734c2ee45"
	first := fixtureLivenessManifest(t, storeUUID)
	second := first
	second.TargetPlistPath = filepath.Join(t.TempDir(), "other.plist")
	firstDigest, err := first.Digest()
	if err != nil {
		t.Fatal(err)
	}
	secondDigest, err := second.Digest()
	if err != nil {
		t.Fatal(err)
	}
	if firstDigest == secondDigest {
		t.Fatal("action digest ignored a changed installation target")
	}
	first.UID++
	if _, err := first.Digest(); err == nil {
		t.Fatal("manifest accepted a different current UID")
	}
}

func TestLivenessWatchManifestBindsInstalledExecutableOwnerSeparatelyFromActor(t *testing.T) {
	manifest := fixtureLivenessManifest(t, "b3c515a2-4773-4653-86d1-8a5734c2ee45")
	manifest.ExecutableOwnerBound = true
	manifest.ExecutableOwnerUID = 0
	if _, err := manifest.Digest(); err != nil {
		t.Fatalf("root-owned installed executable should validate for the current actor: %v", err)
	}

	unsafe := manifest
	unsafe.ExecutableMode |= 0o020
	if _, err := unsafe.Digest(); err == nil {
		t.Fatal("group-writable root-owned executable was accepted")
	}

	legacy := fixtureLivenessManifest(t, "b3c515a2-4773-4653-86d1-8a5734c2ee45")
	if _, err := legacy.Digest(); err != nil {
		t.Fatalf("legacy actor-owned v1 manifest should retain its interpretation: %v", err)
	}
}

func TestLivenessWatchAttemptIsDurableSingleAndExact(t *testing.T) {
	store, err := OpenStore(privateLivenessStoreRoot(t))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	store.storeUUID = "b3c515a2-4773-4653-86d1-8a5734c2ee45"
	manifest := fixtureLivenessManifest(t, store.storeUUID)
	receipt, err := store.PreflightLivenessWatch(manifest)
	if err != nil || receipt.Decision != PreflightPass {
		t.Fatalf("preflight = %+v, err=%v", receipt, err)
	}
	attempt, err := store.BeginLivenessWatchAttempt(manifest, receipt)
	if err != nil {
		t.Fatal(err)
	}
	if pending, err := store.PendingLivenessWatchAttempt(); err != nil || pending.ID != attempt.ID {
		t.Fatalf("pending = %+v, err=%v", pending, err)
	}
	if _, err := store.BeginLivenessWatchAttempt(manifest, receipt); err == nil {
		t.Fatal("second installer attempt passed while first attempt was incomplete")
	}
	wrong := LivenessWatchObservation{UID: manifest.UID, Domain: manifest.Domain, Label: manifest.ManagedLabel,
		PlistPath: filepath.Join(t.TempDir(), "other.plist"), PlistSHA256: manifest.ExpectedPlistSHA256, LaunchdLoaded: true}
	outcome, err := store.RecordLivenessWatchOutcome(attempt, wrong, "")
	if err != nil || outcome.Determination != "incomplete" {
		t.Fatalf("wrong-target outcome = %+v, err=%v", outcome, err)
	}
	if _, err := store.PendingLivenessWatchAttempt(); !errors.Is(err, ErrNoIncompleteLivenessWatchAttempt) {
		t.Fatalf("terminal incomplete attempt should not be replayed: %v", err)
	}
}

func TestLivenessWatchOutcomeRequiresExactPositiveObservation(t *testing.T) {
	store, err := OpenStore(privateLivenessStoreRoot(t))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	store.storeUUID = "b3c515a2-4773-4653-86d1-8a5734c2ee45"
	manifest := fixtureLivenessManifest(t, store.storeUUID)
	receipt, err := store.PreflightLivenessWatch(manifest)
	if err != nil {
		t.Fatal(err)
	}
	attempt, err := store.BeginLivenessWatchAttempt(manifest, receipt)
	if err != nil {
		t.Fatal(err)
	}
	observation := LivenessWatchObservation{UID: manifest.UID, Domain: manifest.Domain, Label: manifest.ManagedLabel,
		PlistPath: manifest.TargetPlistPath, PlistSHA256: manifest.ExpectedPlistSHA256, LaunchdLoaded: true}
	outcome, err := store.RecordLivenessWatchOutcome(attempt, observation, "")
	if err != nil || outcome.Determination != "resolved" {
		t.Fatalf("exact observation = %+v, err=%v", outcome, err)
	}
}

func TestLivenessWatchProjectionDebtSurvivesRestartAndClosesOnlyAfterReadBack(t *testing.T) {
	root := privateLivenessStoreRoot(t)
	storeUUID := "b3c515a2-4773-4653-86d1-8a5734c2ee45"
	store, err := OpenStore(root)
	if err != nil {
		t.Fatal(err)
	}
	store.storeUUID = storeUUID
	manifest := fixtureLivenessManifest(t, storeUUID)
	receipt, err := store.PreflightLivenessWatch(manifest)
	if err != nil {
		t.Fatal(err)
	}
	attempt, err := store.BeginLivenessWatchAttempt(manifest, receipt)
	if err != nil {
		t.Fatal(err)
	}
	observation := LivenessWatchObservation{UID: manifest.UID, Domain: manifest.Domain, Label: manifest.ManagedLabel,
		PlistPath: manifest.TargetPlistPath, PlistSHA256: manifest.ExpectedPlistSHA256, LaunchdLoaded: true}
	outcome, err := NewLivenessWatchOutcome(attempt, observation, "")
	if err != nil {
		t.Fatal(err)
	}
	decision, err := BuildDiagnosticRepairDecision("sirsi maat repair liveness-watch", DiagnosticRepair{
		Check: "liveness-watch", Operation: "install exact watch", Before: "watch absent", After: "watch present",
		Determination: "resolved", Detail: "attempt " + attempt.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	decision.Evidence = "maat-liveness-watch-attempt:" + attempt.ID + ":" + decision.Evidence
	debt, err := store.BeginLivenessWatchProjection(attempt, outcome, decision)
	if err != nil {
		t.Fatal(err)
	}
	if pending, err := store.PendingLivenessWatchProjectionDebts(); err != nil || len(pending) != 1 || pending[0].Attempt.ID != attempt.ID {
		t.Fatalf("projection debt = %#v, err=%v", pending, err)
	}
	if err := store.PersistLivenessWatchProjectionOutcome(debt); err != nil {
		t.Fatal(err)
	}
	if _, err := store.BeginLivenessWatchAttempt(manifest, receipt); err == nil {
		t.Fatal("new repair consent passed while projection debt remained")
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	store, err = OpenStore(root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	store.storeUUID = storeUUID
	pending, err := store.PendingLivenessWatchProjectionDebts()
	if err != nil || len(pending) != 1 || pending[0].Decision.Evidence != decision.Evidence {
		t.Fatalf("reopened projection debt = %#v, err=%v", pending, err)
	}
	changed := pending[0]
	changed.Decision.Why += " drift"
	if err := store.MarkLivenessWatchProjectionReadBack(changed); err == nil {
		t.Fatal("changed decision was accepted as projection readback")
	}
	if err := store.MarkLivenessWatchProjectionReadBack(pending[0]); err != nil {
		t.Fatal(err)
	}
	if debts, err := store.PendingLivenessWatchProjectionDebts(); err != nil || len(debts) != 0 {
		t.Fatalf("completed projection debt = %#v, err=%v", debts, err)
	}
}

func TestLivenessProjectionDeterminationKeepsDiagnosticPostconditionIndependent(t *testing.T) {
	if !validLivenessProjectionDetermination("failed", "resolved") {
		t.Fatal("a failed diagnostic postcondition must remain representable when the exact watch observation succeeded")
	}
	if validLivenessProjectionDetermination("resolved", "incomplete") {
		t.Fatal("an unresolved watch observation must not produce a resolved Casebook decision")
	}
}

func TestLivenessWatchAttemptRefusesManifestDriftAfterPreflight(t *testing.T) {
	store, err := OpenStore(privateLivenessStoreRoot(t))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	store.storeUUID = "b3c515a2-4773-4653-86d1-8a5734c2ee45"
	manifest := fixtureLivenessManifest(t, store.storeUUID)
	receipt, err := store.PreflightLivenessWatch(manifest)
	if err != nil {
		t.Fatal(err)
	}
	manifest.ManagedLabel = "ai.sirsi.other"
	if _, err := store.BeginLivenessWatchAttempt(manifest, receipt); err == nil {
		t.Fatal("attempt accepted a manifest that drifted after preflight")
	}
}

func TestLivenessWatchAttemptRechecksSnapshotUnderAdmissionLock(t *testing.T) {
	for _, mode := range []string{"matching-active", "unsupported-changed", "corrupt-changed", "unrelated-changed"} {
		t.Run(mode, func(t *testing.T) {
			root := privateLivenessStoreRoot(t)
			storeA, err := OpenStore(root)
			if err != nil {
				t.Fatal(err)
			}
			defer storeA.Close()
			storeB, err := OpenStore(root)
			if err != nil {
				t.Fatal(err)
			}
			defer storeB.Close()
			const storeUUID = "b3c515a2-4773-4653-86d1-8a5734c2ee45"
			storeA.storeUUID, storeB.storeUUID = storeUUID, storeUUID
			manifest := fixtureLivenessManifest(t, storeUUID)
			reviewed, err := storeA.PreflightLivenessWatch(manifest)
			if err != nil || reviewed.Decision != PreflightPass {
				t.Fatalf("initial preflight = %+v, err=%v", reviewed, err)
			}
			switch mode {
			case "matching-active", "unsupported-changed", "unrelated-changed":
				evidence, err := storeB.PutEvidence([]byte("later matching liveness-watch incident"))
				if err != nil {
					t.Fatal(err)
				}
				guardID, guardVersion := ActiveIncidentGuardID, ActiveIncidentGuardVersion
				if mode == "unsupported-changed" {
					guardID, guardVersion = "unsupported.fixture", "v1"
				}
				scope := Scope{Component: "pantheon", Profile: "local-macos", Operation: "install-managed-liveness-watch"}
				if mode == "unrelated-changed" {
					// This valid incident changes the reviewed registry snapshot but
					// does not match this action, so fresh preflight still passes.
					scope = Scope{Component: "other", Profile: "unrelated", Operation: "different-action"}
				}
				incident, err := NewIncident(testSignature(), evidence, scope, guardID, guardVersion, testActions())
				if err != nil {
					t.Fatal(err)
				}
				if err := storeB.Append(incident); err != nil {
					t.Fatal(err)
				}
			case "corrupt-changed":
				bad := filepath.Join(root, "incidents", strings.Repeat("f", 64)+".json")
				if err := os.WriteFile(bad, []byte("{}\n"), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			if mode == "unrelated-changed" {
				fresh, err := storeA.PreflightLivenessWatch(manifest)
				if err != nil || fresh.Decision != PreflightPass || fresh.RegistrySnapshotSHA256 == reviewed.RegistrySnapshotSHA256 {
					t.Fatalf("unrelated change must preserve a passing decision with a changed snapshot: old=%+v fresh=%+v err=%v", reviewed, fresh, err)
				}
			}
			if _, err := storeA.BeginLivenessWatchAttempt(manifest, reviewed); err == nil {
				t.Fatal("attempt admission accepted a changed registry snapshot")
			}
			if _, err := storeA.PendingLivenessWatchAttempt(); !errors.Is(err, ErrNoIncompleteLivenessWatchAttempt) {
				t.Fatalf("refused admission left an attempt: %v", err)
			}
		})
	}
}

func privateLivenessStoreRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := os.Chmod(root, 0o700); err != nil {
		t.Fatalf("set private liveness store fixture mode: %v", err)
	}
	return root
}

func fixtureLivenessManifest(t *testing.T, storeUUID string) LivenessWatchManifest {
	t.Helper()
	root := t.TempDir()
	return LivenessWatchManifest{
		Schema: LivenessWatchManifestSchema, UID: uint32(os.Geteuid()), Domain: "gui/" + strconv.Itoa(os.Geteuid()),
		ExecutablePath: filepath.Join(root, "sirsi"), ExecutableDevice: 1, ExecutableInode: 2,
		ExecutableMode: uint32(unix.S_IFREG) | 0o755, ExecutableSize: 1, ExecutableSHA256: strings.Repeat("a", 64),
		WorkDirectory: root, ManagedLabel: "ai.sirsi.liveness-watch", TargetPlistPath: filepath.Join(root, "Library", "LaunchAgents", "ai.sirsi.liveness-watch.plist"),
		ExpectedPlistSHA256: strings.Repeat("b", 64), FindingCheck: "liveness-watch", FindingSeverity: 2,
		FindingEvidenceSHA256: strings.Repeat("c", 64), FindingObservedAtUTC: time.Now().UTC(), StoreUUID: storeUUID,
	}
}
