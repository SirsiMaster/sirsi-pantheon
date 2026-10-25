//go:build darwin

package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/maat"
	"github.com/SirsiMaster/sirsi-pantheon/internal/updater"
)

type appUpdateFixtureStore struct {
	pending *maat.AppUpdateAttempt
	started int
	ended   int
}

func (s *appUpdateFixtureStore) PreflightAppUpdate(manifest maat.AppUpdateManifest) (maat.PreflightReceipt, error) {
	digest, err := manifest.Digest()
	if err != nil {
		return maat.PreflightReceipt{}, err
	}
	return maat.PreflightReceipt{Schema: maat.MemorySchema,
		Action:               maat.Scope{Component: "pantheon", Profile: "local-macos", Operation: "update-whole-app"},
		ActionManifestSHA256: digest, RegistrySnapshotSHA256: strings.Repeat("d", 64),
		Decision: maat.PreflightPass, EvaluatedAtUTC: time.Now().UTC()}, nil
}

func (s *appUpdateFixtureStore) BeginAppUpdateAttempt(manifest maat.AppUpdateManifest, receipt maat.PreflightReceipt) (maat.AppUpdateAttempt, error) {
	s.started++
	if s.pending != nil {
		return maat.AppUpdateAttempt{}, errors.New("pending attempt")
	}
	attempt := maat.AppUpdateAttempt{Schema: maat.MemorySchema, ID: "fixture-app-update-attempt", Manifest: manifest,
		ActionManifestSHA256: receipt.ActionManifestSHA256, PreflightReceipt: receipt, RegistrySnapshotSHA256: receipt.RegistrySnapshotSHA256,
		StartedAtUTC: time.Now().UTC()}
	s.pending = &attempt
	return attempt, nil
}

func (s *appUpdateFixtureStore) PendingAppUpdateAttempt() (maat.AppUpdateAttempt, error) {
	if s.pending == nil {
		return maat.AppUpdateAttempt{}, maat.ErrNoIncompleteAppUpdateAttempt
	}
	return *s.pending, nil
}

func (s *appUpdateFixtureStore) RecordAppUpdateInstalled(attempt maat.AppUpdateAttempt, observation maat.AppUpdateObservation) (maat.AppUpdateOutcome, error) {
	if s.pending == nil || s.pending.ID != attempt.ID || observation.Release != attempt.Manifest.Release ||
		observation.BundleID != attempt.Manifest.TargetBundleID || observation.AppPath != attempt.Manifest.TargetAppPath ||
		observation.HelperPath != attempt.Manifest.TargetHelperPath || observation.HelperSHA256 != attempt.Manifest.ExpectedInstalledHelperSHA256 ||
		!observation.AppSignatureVerified || !observation.HelperSignatureVerified || !observation.DependenciesPresent {
		return maat.AppUpdateOutcome{}, errors.New("exact installed tuple not observed")
	}
	s.ended++
	s.pending = nil
	return maat.AppUpdateOutcome{Schema: maat.MemorySchema, AttemptID: attempt.ID, Determination: "installed", Observation: observation, RecordedAtUTC: time.Now().UTC()}, nil
}

func (*appUpdateFixtureStore) Close() error { return nil }

func appUpdatePlanFixture(t *testing.T) maintenanceInstallerPlan {
	t.Helper()
	stage := filepath.Join(t.TempDir(), "sirsi-maintenance-installer-fixture")
	if err := os.Mkdir(stage, 0700); err != nil {
		t.Fatal(err)
	}
	name := "SirsiPantheon-0.24.200-arm64.pkg"
	return maintenanceInstallerPlan{Schema: 1, Release: "0.24.200", Package: filepath.Join(stage, name),
		PackageURL: "https://github.com/SirsiMaster/sirsi-pantheon/releases/download/v0.24.200/" + name,
		SHA256:     strings.Repeat("a", 64), Device: 1, Inode: 2, Bytes: 2048, Implementation: strings.Repeat("b", 64),
		EngineSHA256: strings.Repeat("c", 64), InstallerIdentity: "Developer ID Installer: Sirsi Technologies Inc. (9D382WV988)",
		Notarization: "Notarized Developer ID", TargetBundleID: "ai.sirsi.pantheon", TargetAppPath: "/Applications/Pantheon.app",
		TargetHelperPath: "/usr/local/libexec/sirsi-pantheon/sirsi", Dependencies: []string{"/Applications/Pantheon.app", "/usr/local/libexec/sirsi-pantheon/sirsi"}}
}

func installAppUpdateFixture(t *testing.T, plan maintenanceInstallerPlan, handoffErr error, installed bool) (*appUpdateFixtureStore, *maatTestJournal, *int, *int) {
	t.Helper()
	store := &appUpdateFixtureStore{}
	journal := &maatTestJournal{}
	handoffs, stages := 0, 0
	previous := maatAppUpdateDependencies
	maatAppUpdateDependencies = maatAppUpdateOps{
		storeStatus: func() maat.DefaultStoreStatus {
			return maat.DefaultStoreStatus{State: maat.DefaultStoreReady, StoreUUID: "b3c515a2-4773-4653-86d1-8a5734c2ee45"}
		},
		openStore: func() (maatAppUpdateStore, error) { return store, nil },
		journal:   func() (maat.DecisionJournal, error) { return journal, nil },
		stage: func(context.Context, string, *updater.Release) (maintenanceInstallerPlan, error) {
			stages++
			return plan, nil
		},
		revalidate: func(context.Context, *updater.Release, maintenanceInstallerPlan) error { return nil },
		observe: func(context.Context) (maat.AppUpdateObservation, error) {
			if installed && handoffs > 0 && handoffErr == nil {
				return maat.AppUpdateObservation{Release: plan.Release, BundleID: plan.TargetBundleID, AppPath: plan.TargetAppPath,
					HelperPath: plan.TargetHelperPath, HelperSHA256: plan.EngineSHA256, AppSignatureVerified: true,
					HelperSignatureVerified: true, DependenciesPresent: true, ObservedAtUTC: time.Now().UTC()}, nil
			}
			return maat.AppUpdateObservation{Release: "0.24.136", BundleID: plan.TargetBundleID, AppPath: plan.TargetAppPath,
				HelperPath: plan.TargetHelperPath, HelperSHA256: strings.Repeat("e", 64), AppSignatureVerified: true,
				HelperSignatureVerified: true, DependenciesPresent: true, ObservedAtUTC: time.Now().UTC()}, nil
		},
		handoff: func(context.Context, string) error { handoffs++; return handoffErr },
	}
	t.Cleanup(func() { maatAppUpdateDependencies = previous })
	return store, journal, &handoffs, &stages
}

func TestInstallAppReleaseWithMaatRefusesInvalidPKGTuplesBeforeIntentOrHandoff(t *testing.T) {
	for _, name := range []string{"tuple", "hash", "signature", "dependencies"} {
		t.Run(name, func(t *testing.T) {
			plan := appUpdatePlanFixture(t)
			switch name {
			case "tuple":
				plan.PackageURL = "https://untrusted.invalid/package.pkg"
			case "hash":
				plan.SHA256 = ""
			case "signature":
				plan.InstallerIdentity = "untrusted signer"
			case "dependencies":
				plan.Dependencies = []string{"/Applications/Other.app"}
			}
			store, _, handoffs, stages := installAppUpdateFixture(t, plan, nil, false)
			err := installAppReleaseWithMaat(&updater.Release{TagName: "v0.24.200"})
			if err == nil || store.started != 0 || *handoffs != 0 || *stages != 1 {
				t.Fatalf("invalid %s tuple passed: err=%v starts=%d handoffs=%d stages=%d", name, err, store.started, *handoffs, *stages)
			}
		})
	}
}

func TestInstallAppReleaseWithMaatHandsOffOnceAndRequiresInstalledReadback(t *testing.T) {
	plan := appUpdatePlanFixture(t)
	store, journal, handoffs, stages := installAppUpdateFixture(t, plan, nil, true)
	if err := installAppReleaseWithMaat(&updater.Release{TagName: "v0.24.200"}); err != nil {
		t.Fatal(err)
	}
	if store.started != 1 || store.ended != 1 || *handoffs != 1 || *stages != 1 || store.pending != nil {
		t.Fatalf("handoff/readback counts start=%d terminal=%d handoffs=%d stages=%d pending=%v", store.started, store.ended, *handoffs, *stages, store.pending != nil)
	}
	if len(journal.decisions) != 1 || journal.decisions[0].Kind != "failure memory preflight" || journal.decisions[0].Determination != "pass" {
		t.Fatalf("full scoped preflight was not projected before handoff: %+v", journal.decisions)
	}
}

func TestAmbiguousAppInstallerHandoffReconcilesDataOnlyWithoutReplay(t *testing.T) {
	plan := appUpdatePlanFixture(t)
	store, _, handoffs, stages := installAppUpdateFixture(t, plan, errors.New("handoff outcome unknown"), false)
	if err := installAppReleaseWithMaat(&updater.Release{TagName: "v0.24.200"}); err == nil || store.pending == nil {
		t.Fatalf("ambiguous Installer handoff lost its durable pending intent: err=%v pending=%v", err, store.pending != nil)
	}
	if handled, err := reconcilePendingMaatAppUpdate(); !handled || err == nil {
		t.Fatalf("data-only reconciliation did not retain the unresolved install fact: handled=%v err=%v", handled, err)
	}
	oldClient := newUpdateClient
	oldCLI, oldApp := updateInstallCLI, updateInstallApp
	newUpdateClient = func() *updater.Client { t.Fatal("pending readback performed a network update check"); return nil }
	updateInstallCLI, updateInstallApp = false, true
	defer func() { newUpdateClient, updateInstallCLI, updateInstallApp = oldClient, oldCLI, oldApp }()
	if err := runUpdate(updateCmd, nil); err == nil {
		t.Fatal("unresolved app update was hidden by a network check")
	}
	if store.started != 1 || *handoffs != 1 || *stages != 1 || store.pending == nil {
		t.Fatalf("ambiguous handoff was replayed or debt lost: starts=%d handoffs=%d stages=%d pending=%v", store.started, *handoffs, *stages, store.pending != nil)
	}
}
