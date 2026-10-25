package maat

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/version"
)

func fixtureAppUpdateManifest(t *testing.T, storeUUID string) AppUpdateManifest {
	t.Helper()
	return AppUpdateManifest{
		Schema: AppUpdateManifestSchema, StoreUUID: storeUUID, UID: uint32(os.Geteuid()),
		Release: "0.24.200", PackageName: "SirsiPantheon-0.24.200-arm64.pkg",
		PackageURL:    "https://github.com/SirsiMaster/sirsi-pantheon/releases/download/v0.24.200/SirsiPantheon-0.24.200-arm64.pkg",
		PackagePath:   filepath.Join(os.TempDir(), "sirsi-maintenance-installer-fixture", "SirsiPantheon-0.24.200-arm64.pkg"),
		PackageSHA256: strings.Repeat("a", 64), PackageDevice: 1, PackageInode: 2, PackageBytes: 256,
		InstallerIdentity: "Developer ID Installer: Sirsi Technologies Inc. (9D382WV988)",
		Notarization:      "Notarized Developer ID", TargetBundleID: "ai.sirsi.pantheon",
		TargetAppPath: "/Applications/Pantheon.app", TargetHelperPath: "/usr/local/libexec/sirsi-pantheon/sirsi",
		ExpectedInstalledHelperSHA256: strings.Repeat("b", 64), CurrentRelease: "0.24.136",
		ExpectedEmbeddedHelperSHA256: strings.Repeat("b", 64), ExpectedAppCDHash: strings.Repeat("d", 40),
		CandidateVersion: version.Info{Binary: "sirsi", Version: "0.24.200", Commit: "fixture", RouterSchemaMax: 25},
		LiveRouterSchema: 25, CurrentHelperSHA256: strings.Repeat("c", 64),
		Dependencies:  []string{"/Applications/Pantheon.app", "/usr/local/libexec/sirsi-pantheon/sirsi"},
		ObservedAtUTC: time.Now().UTC()}
}

func retainAppUpdateReceipt(t *testing.T, store *Store, manifest AppUpdateManifest) StoredPreflightReceipt {
	t.Helper()
	receipt, err := store.PreflightAppUpdate(manifest)
	if err != nil || receipt.Decision != PreflightPass {
		t.Fatalf("app-update preflight=%+v err=%v", receipt, err)
	}
	stored, err := store.RetainPreflightReceipt(receipt)
	if err != nil {
		t.Fatalf("retain exact app-update preflight: %v", err)
	}
	return stored
}

func TestAppUpdateManifestBindsPackageTargetAndDependencies(t *testing.T) {
	manifest := fixtureAppUpdateManifest(t, "b3c515a2-4773-4653-86d1-8a5734c2ee45")
	first, err := manifest.Digest()
	if err != nil {
		t.Fatal(err)
	}
	changed := manifest
	changed.PackageSHA256 = strings.Repeat("d", 64)
	second, err := changed.Digest()
	if err != nil || first == second {
		t.Fatalf("manifest digest did not bind exact PKG bytes: first=%s second=%s err=%v", first, second, err)
	}
	for name, mutate := range map[string]func(*AppUpdateManifest){
		"wrong target":                      func(m *AppUpdateManifest) { m.TargetBundleID = "other.app" },
		"wrong dependency":                  func(m *AppUpdateManifest) { m.Dependencies = []string{"/Applications/Other.app", m.TargetHelperPath} },
		"wrong signature":                   func(m *AppUpdateManifest) { m.InstallerIdentity = "untrusted" },
		"wrong notarization":                func(m *AppUpdateManifest) { m.Notarization = "unverified" },
		"wrong package name":                func(m *AppUpdateManifest) { m.PackageName = "other.pkg" },
		"wrong embedded helper":             func(m *AppUpdateManifest) { m.ExpectedEmbeddedHelperSHA256 = strings.Repeat("e", 64) },
		"missing signed app identity":       func(m *AppUpdateManifest) { m.ExpectedAppCDHash = "" },
		"missing candidate schema ceiling":  func(m *AppUpdateManifest) { m.CandidateVersion.RouterSchemaMax = 0 },
		"candidate cannot open live schema": func(m *AppUpdateManifest) { m.CandidateVersion.RouterSchemaMax = 24 },
	} {
		t.Run(name, func(t *testing.T) {
			candidate := manifest
			mutate(&candidate)
			if _, err := candidate.Digest(); err == nil {
				t.Fatal("invalid reviewed tuple was accepted")
			}
		})
	}
}

func TestAppUpdateAttemptRetainsReceiptAndUnknownHandoffNeverReplays(t *testing.T) {
	root := privateLivenessStoreRoot(t)
	store, err := OpenStore(root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	store.storeUUID = "b3c515a2-4773-4653-86d1-8a5734c2ee45"
	manifest := fixtureAppUpdateManifest(t, store.storeUUID)
	stored := retainAppUpdateReceipt(t, store, manifest)
	attempt, err := store.BeginAppUpdateAttempt(manifest, stored)
	if err != nil {
		t.Fatal(err)
	}
	if attempt.PreflightReceipt.ActionSHA256 != attempt.ActionManifestSHA256 || attempt.PreflightReceipt.ReceiptSHA256 == "" {
		t.Fatalf("durable intent lost its exact retained preflight reference: %+v", attempt)
	}
	readback, err := store.ReadPreflightReceipt(attempt.PreflightReceipt.ReceiptSHA256)
	if err != nil || readback.Receipt.ActionManifestSHA256 != attempt.ActionManifestSHA256 || readback.Receipt.Decision != PreflightPass {
		t.Fatalf("durable intent does not link to the exact committed passing receipt: receipt=%+v err=%v", readback, err)
	}
	pending, err := store.PendingAppUpdateAttempt()
	if err != nil || pending.ID != attempt.ID {
		t.Fatalf("pending attempt=%+v err=%v", pending, err)
	}
	if _, err := store.BeginAppUpdateAttempt(manifest, stored); err == nil {
		t.Fatal("second update was admitted while first Installer handoff was unresolved")
	}
	wrong := AppUpdateObservation{Release: manifest.CurrentRelease, BundleID: manifest.TargetBundleID,
		AppPath: manifest.TargetAppPath, HelperPath: manifest.TargetHelperPath,
		HelperSHA256: manifest.CurrentHelperSHA256, AppSignatureVerified: true,
		HelperSignatureVerified: true, DependenciesPresent: true, ObservedAtUTC: time.Now().UTC()}
	if _, err := store.RecordAppUpdateInstalled(attempt, wrong); err == nil {
		t.Fatal("old installed version was mislabeled as completed update")
	}
	if pending, err := store.PendingAppUpdateAttempt(); err != nil || pending.ID != attempt.ID {
		t.Fatalf("mismatched installed readback cleared handoff debt: pending=%+v err=%v", pending, err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = OpenStore(root)
	if err != nil {
		t.Fatal(err)
	}
	store.storeUUID = "b3c515a2-4773-4653-86d1-8a5734c2ee45"
	defer store.Close()
	if pending, err := store.PendingAppUpdateAttempt(); err != nil || pending.ID != attempt.ID {
		t.Fatalf("reopened unknown handoff was cleared or replayable: pending=%+v err=%v", pending, err)
	}
}

func TestAppAndRawSelfUpdateAdmissionsFenceEachOther(t *testing.T) {
	for _, first := range []string{"app", "raw"} {
		t.Run(first, func(t *testing.T) {
			store, rawManifest, rawReceipt := rawSelfUpdateFixture(t)
			appManifest := fixtureAppUpdateManifest(t, store.storeUUID)
			appReceipt := retainAppUpdateReceipt(t, store, appManifest)
			if first == "app" {
				if _, err := store.BeginAppUpdateAttempt(appManifest, appReceipt); err != nil {
					t.Fatalf("begin confirmed app attempt: %v", err)
				}
				if _, err := store.BeginRawSelfUpdateAttempt(rawManifest, rawReceipt); err == nil {
					t.Fatal("raw self-update entered while whole-app Installer handoff was unresolved")
				}
				return
			}
			if _, err := store.BeginRawSelfUpdateAttempt(rawManifest, rawReceipt); err != nil {
				t.Fatalf("begin raw self-update attempt: %v", err)
			}
			if _, err := store.BeginAppUpdateAttempt(appManifest, appReceipt); err == nil {
				t.Fatal("whole-app Installer entered while raw self-update was unresolved")
			}
		})
	}
}

func TestAppUpdateInstalledReadbackCompletesOnlyExactTuple(t *testing.T) {
	root := privateLivenessStoreRoot(t)
	store, err := OpenStore(root)
	if err != nil {
		t.Fatal(err)
	}
	store.storeUUID = "b3c515a2-4773-4653-86d1-8a5734c2ee45"
	manifest := fixtureAppUpdateManifest(t, store.storeUUID)
	stored := retainAppUpdateReceipt(t, store, manifest)
	attempt, err := store.BeginAppUpdateAttempt(manifest, stored)
	if err != nil {
		t.Fatal(err)
	}
	for name, observation := range map[string]AppUpdateObservation{
		"different code identity": func() AppUpdateObservation {
			o := exactAppUpdateObservation(manifest, time.Now().UTC())
			o.AppCDHash = strings.Repeat("e", 40)
			return o
		}(),
		"different embedded helper": func() AppUpdateObservation {
			o := exactAppUpdateObservation(manifest, time.Now().UTC())
			o.EmbeddedHelperSHA256 = strings.Repeat("e", 64)
			return o
		}(),
	} {
		if _, err := store.RecordAppUpdateInstalled(attempt, observation); err == nil {
			t.Fatalf("%s was accepted as installed", name)
		}
		if pending, err := store.PendingAppUpdateAttempt(); err != nil || pending.ID != attempt.ID {
			t.Fatalf("%s refusal cleared attempt: %+v %v", name, pending, err)
		}
	}
	observation := exactAppUpdateObservation(manifest, time.Now().UTC())
	outcome, err := store.RecordAppUpdateInstalled(attempt, observation)
	if err != nil || outcome.Determination != "installed" {
		t.Fatalf("outcome=%+v err=%v", outcome, err)
	}
	if _, err := store.PendingAppUpdateAttempt(); !errors.Is(err, ErrNoIncompleteAppUpdateAttempt) {
		t.Fatalf("exact installed readback left an incomplete attempt: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = OpenStore(root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	store.storeUUID = "b3c515a2-4773-4653-86d1-8a5734c2ee45"
	if _, err := store.PendingAppUpdateAttempt(); !errors.Is(err, ErrNoIncompleteAppUpdateAttempt) {
		t.Fatalf("exact historical outcome failed reopen validation: %v", err)
	}
}

func TestAppUpdateAdmissionsFenceBothOwnersAcrossIndependentHandles(t *testing.T) {
	for _, first := range []string{"app", "liveness"} {
		t.Run(first, func(t *testing.T) {
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
			appManifest := fixtureAppUpdateManifest(t, storeUUID)
			appReceipt := retainAppUpdateReceipt(t, storeA, appManifest)
			livenessManifest := fixtureLivenessManifest(t, storeUUID)
			livenessReceipt, err := storeB.PreflightLivenessWatch(livenessManifest)
			if err != nil {
				t.Fatal(err)
			}
			if first == "app" {
				if _, err := storeA.BeginAppUpdateAttempt(appManifest, appReceipt); err != nil {
					t.Fatal(err)
				}
				if _, err := storeB.BeginLivenessWatchAttempt(livenessManifest, livenessReceipt); err == nil {
					t.Fatal("liveness installation entered while app update attempt was pending")
				}
			} else {
				if _, err := storeB.BeginLivenessWatchAttempt(livenessManifest, livenessReceipt); err != nil {
					t.Fatal(err)
				}
				if _, err := storeA.BeginAppUpdateAttempt(appManifest, appReceipt); err == nil {
					t.Fatal("app update entered while liveness installation attempt was pending")
				}
			}
		})
	}
}

func TestAppAndLivenessConcurrentAdmissionHasOneSharedWinner(t *testing.T) {
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
	appManifest := fixtureAppUpdateManifest(t, storeUUID)
	appReceipt := retainAppUpdateReceipt(t, storeA, appManifest)
	livenessManifest := fixtureLivenessManifest(t, storeUUID)
	livenessReceipt, err := storeB.PreflightLivenessWatch(livenessManifest)
	if err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	results := make(chan error, 2)
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		<-start
		_, err := storeA.BeginAppUpdateAttempt(appManifest, appReceipt)
		results <- err
	}()
	go func() {
		defer wg.Done()
		<-start
		_, err := storeB.BeginLivenessWatchAttempt(livenessManifest, livenessReceipt)
		results <- err
	}()
	close(start)
	wg.Wait()
	close(results)
	passed := 0
	for err := range results {
		if err == nil {
			passed++
		}
	}
	if passed != 1 {
		t.Fatalf("cross-owner admission passes=%d, want exactly one", passed)
	}
}

func TestAppUpdateOutcomeRejectsCallerMutationAndBadObservationTime(t *testing.T) {
	store, err := OpenStore(privateLivenessStoreRoot(t))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	store.storeUUID = "b3c515a2-4773-4653-86d1-8a5734c2ee45"
	manifest := fixtureAppUpdateManifest(t, store.storeUUID)
	stored := retainAppUpdateReceipt(t, store, manifest)
	attempt, err := store.BeginAppUpdateAttempt(manifest, stored)
	if err != nil {
		t.Fatal(err)
	}
	mutated := attempt
	mutated.Manifest.CurrentRelease = "0.24.135"
	mutated.ActionManifestSHA256, _ = mutated.Manifest.Digest()
	mutated.PreflightReceipt.ActionSHA256 = mutated.ActionManifestSHA256
	observation := exactAppUpdateObservation(manifest, time.Now().UTC())
	if _, err := store.RecordAppUpdateInstalled(mutated, observation); err == nil {
		t.Fatal("same-ID caller-modified attempt was accepted")
	}
	for name, stamp := range map[string]time.Time{
		"future": time.Now().UTC().Add(time.Minute), "stale": time.Now().UTC().Add(-3 * time.Minute),
		"non-utc": time.Now().In(time.FixedZone("offset", 3600)), "before-start": attempt.StartedAtUTC.Add(-time.Second),
	} {
		t.Run(name, func(t *testing.T) {
			bad := exactAppUpdateObservation(manifest, stamp)
			if _, err := store.RecordAppUpdateInstalled(attempt, bad); err == nil {
				t.Fatal("invalid observation time was accepted")
			}
			if pending, err := store.PendingAppUpdateAttempt(); err != nil || pending.ID != attempt.ID {
				t.Fatalf("refused outcome poisoned retained attempt: %+v %v", pending, err)
			}
		})
	}
	if _, err := store.RecordAppUpdateInstalled(attempt, observation); err != nil {
		t.Fatal(err)
	}
	if _, err := store.PendingAppUpdateAttempt(); !errors.Is(err, ErrNoIncompleteAppUpdateAttempt) {
		t.Fatalf("exact outcome did not reopen as complete: %v", err)
	}
}

func TestAppUpdateKnownNotInvokedClosesOnlyExactPreHandoffAttempt(t *testing.T) {
	root := privateLivenessStoreRoot(t)
	store, err := OpenStore(root)
	if err != nil {
		t.Fatal(err)
	}
	store.storeUUID = "b3c515a2-4773-4653-86d1-8a5734c2ee45"
	manifest := fixtureAppUpdateManifest(t, store.storeUUID)
	stored := retainAppUpdateReceipt(t, store, manifest)
	attempt, err := store.BeginAppUpdateAttempt(manifest, stored)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.RecordAppUpdateNotInvoked(attempt, "unknown-handoff"); err == nil {
		t.Fatal("arbitrary/ambiguous failure was reclassified as known not-invoked")
	}
	if pending, err := store.PendingAppUpdateAttempt(); err != nil || pending.ID != attempt.ID {
		t.Fatalf("unknown attempt was cleared: %+v %v", pending, err)
	}
	outcome, err := store.RecordAppUpdateNotInvoked(attempt, "package-revalidation-refused")
	if err != nil || outcome.Determination != "not-invoked" {
		t.Fatalf("disposition=%+v err=%v", outcome, err)
	}
	if _, err := store.PendingAppUpdateAttempt(); !errors.Is(err, ErrNoIncompleteAppUpdateAttempt) {
		t.Fatalf("terminal not-invoked still pending: %v", err)
	}
	data, err := readExact(store.attemptsFD, attempt.ID+".app-update-outcome.json")
	if err != nil {
		t.Fatal(err)
	}
	var retained AppUpdateOutcome
	if json.Unmarshal(data, &retained) != nil || validateAppUpdateOutcome(retained, attempt) != nil || retained.Determination != "not-invoked" || retained.NotInvokedReason != "package-revalidation-refused" {
		t.Fatalf("retained known-not-invoked fact is not exact: %+v", retained)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = OpenStore(root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	store.storeUUID = "b3c515a2-4773-4653-86d1-8a5734c2ee45"
	if _, err := store.PendingAppUpdateAttempt(); !errors.Is(err, ErrNoIncompleteAppUpdateAttempt) {
		t.Fatalf("reopened not-invoked fact did not close attempt: %v", err)
	}
	if _, err := store.BeginAppUpdateAttempt(manifest, stored); err != nil {
		t.Fatalf("fresh exact attempt could not follow retained not-invoked fact: %v", err)
	}
}

func TestAppUpdatePreparedExactLeavesRecoverTargetAndCommitPhasesOnly(t *testing.T) {
	root := privateLivenessStoreRoot(t)
	store, err := OpenStore(root)
	if err != nil {
		t.Fatal(err)
	}
	store.storeUUID = "b3c515a2-4773-4653-86d1-8a5734c2ee45"
	manifest := fixtureAppUpdateManifest(t, store.storeUUID)
	stored := retainAppUpdateReceipt(t, store, manifest)
	reference := stored.Reference()
	attempt := AppUpdateAttempt{Schema: MemorySchema, ID: "b3c515a2-4773-4653-86d1-8a5734c2ee45", Manifest: manifest,
		ActionManifestSHA256: reference.ActionSHA256, PreflightReceipt: reference,
		RegistrySnapshotSHA256: reference.RegistrySHA256, StartedAtUTC: time.Now().UTC()}
	if err := retainPreparedAppUpdateFixtureLeaf(store, attempt.ID+".app-update-started.json", attempt); err != nil {
		t.Fatal(err)
	}
	pending, err := store.PendingAppUpdateAttempt()
	if err != nil || pending.ID != attempt.ID {
		t.Fatalf("prepared started record did not recover exact target+commit: %+v %v", pending, err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = OpenStore(root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	store.storeUUID = "b3c515a2-4773-4653-86d1-8a5734c2ee45"
	if pending, err := store.PendingAppUpdateAttempt(); err != nil || pending.ID != attempt.ID {
		t.Fatalf("reopened exact started attempt differs: %+v %v", pending, err)
	}
	outcome := AppUpdateOutcome{Schema: MemorySchema, AttemptID: attempt.ID, Determination: "not-invoked",
		NotInvokedReason: "package-revalidation-refused", RecordedAtUTC: time.Now().UTC()}
	if err := retainPreparedAppUpdateFixtureLeaf(store, attempt.ID+".app-update-outcome.json", outcome); err != nil {
		t.Fatal(err)
	}
	if _, err := store.PendingAppUpdateAttempt(); !errors.Is(err, ErrNoIncompleteAppUpdateAttempt) {
		t.Fatalf("prepared terminal target+commit did not recover: %v", err)
	}
}

func TestAppUpdatePreparedMissingTargetAndUnrelatedWALRemainFailClosed(t *testing.T) {
	for _, leaf := range []string{"b3c515a2-4773-4653-86d1-8a5734c2ee45.app-update-started.json", "unrelated.json"} {
		t.Run(leaf, func(t *testing.T) {
			store, err := OpenStore(privateLivenessStoreRoot(t))
			if err != nil {
				t.Fatal(err)
			}
			defer store.Close()
			store.storeUUID = "b3c515a2-4773-4653-86d1-8a5734c2ee45"
			if err := retainPreparedAppUpdateFixtureIntentOnly(store, leaf, []byte("prepared")); err != nil {
				t.Fatal(err)
			}
			if _, err := store.PendingAppUpdateAttempt(); err == nil {
				t.Fatal("missing exact target or unrelated prepared WAL was bypassed")
			}
		})
	}
}

func retainPreparedAppUpdateFixtureLeaf(store *Store, leaf string, value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	intent, err := newWriteIntent("attempts", leaf, data)
	if err != nil {
		return err
	}
	intentBytes, err := json.Marshal(intent)
	if err != nil {
		return err
	}
	if err := createOrVerifyRaw(store.writesFD, intent.Key+".prepared.json", intentBytes); err != nil {
		return err
	}
	return createOrVerifyRaw(store.attemptsFD, leaf, data)
}

func retainPreparedAppUpdateFixtureIntentOnly(store *Store, leaf string, data []byte) error {
	intent, err := newWriteIntent("attempts", leaf, data)
	if err != nil {
		return err
	}
	intentBytes, err := json.Marshal(intent)
	if err != nil {
		return err
	}
	return createOrVerifyRaw(store.writesFD, intent.Key+".prepared.json", intentBytes)
}

func exactAppUpdateObservation(manifest AppUpdateManifest, stamp time.Time) AppUpdateObservation {
	return AppUpdateObservation{Release: manifest.Release, BundleID: manifest.TargetBundleID,
		AppPath: manifest.TargetAppPath, HelperPath: manifest.TargetHelperPath,
		HelperSHA256: manifest.ExpectedInstalledHelperSHA256, EmbeddedHelperSHA256: manifest.ExpectedEmbeddedHelperSHA256,
		AppCDHash: manifest.ExpectedAppCDHash, AppSignatureVerified: true, HelperSignatureVerified: true,
		DependenciesPresent: true, ObservedAtUTC: stamp}
}
