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
	"github.com/SirsiMaster/sirsi-pantheon/internal/selfupdate"
	"github.com/SirsiMaster/sirsi-pantheon/internal/setup"
	"github.com/SirsiMaster/sirsi-pantheon/internal/updater"
	modversion "github.com/SirsiMaster/sirsi-pantheon/internal/version"
)

type appUpdateFixtureStore struct {
	inner    *maat.Store
	owner    *appUpdateFixtureStore
	pending  *maat.AppUpdateAttempt
	started  int
	ended    int
	opened   int
	observed int
	last     maat.AppUpdateOutcome
}

func (s *appUpdateFixtureStore) state() *appUpdateFixtureStore {
	if s.owner != nil {
		return s.owner
	}
	return s
}

func (s *appUpdateFixtureStore) open() (maatAppUpdateStore, error) {
	if s.inner != nil {
		_ = s.inner.Close()
		s.inner = nil
	}
	inner, err := maat.OpenDefaultStore()
	if err != nil {
		return nil, err
	}
	state := s.state()
	state.opened++
	return &appUpdateFixtureStore{inner: inner, owner: state}, nil
}

func (s *appUpdateFixtureStore) PreflightAppUpdate(manifest maat.AppUpdateManifest) (maat.PreflightReceipt, error) {
	return s.inner.PreflightAppUpdate(manifest)
}

func (s *appUpdateFixtureStore) RetainPreflightReceipt(receipt maat.PreflightReceipt) (maat.StoredPreflightReceipt, error) {
	return s.inner.RetainPreflightReceipt(receipt)
}

func (s *appUpdateFixtureStore) BeginAppUpdateAttempt(manifest maat.AppUpdateManifest, receipt maat.StoredPreflightReceipt) (maat.AppUpdateAttempt, error) {
	state := s.state()
	state.started++
	if state.pending != nil {
		return maat.AppUpdateAttempt{}, errors.New("pending attempt")
	}
	attempt, err := s.inner.BeginAppUpdateAttempt(manifest, receipt)
	if err != nil {
		return maat.AppUpdateAttempt{}, err
	}
	state.pending = &attempt
	return attempt, nil
}

func (s *appUpdateFixtureStore) PendingAppUpdateAttempt() (maat.AppUpdateAttempt, error) {
	return s.inner.PendingAppUpdateAttempt()
}

func (s *appUpdateFixtureStore) RecordAppUpdateInstalled(attempt maat.AppUpdateAttempt, observation maat.AppUpdateObservation) (maat.AppUpdateOutcome, error) {
	outcome, err := s.inner.RecordAppUpdateInstalled(attempt, observation)
	if err != nil {
		return maat.AppUpdateOutcome{}, err
	}
	state := s.state()
	state.ended++
	state.pending = nil
	state.last = outcome
	return state.last, nil
}

func (s *appUpdateFixtureStore) RecordAppUpdateNotInvoked(attempt maat.AppUpdateAttempt, reason string) (maat.AppUpdateOutcome, error) {
	outcome, err := s.inner.RecordAppUpdateNotInvoked(attempt, reason)
	if err != nil {
		return maat.AppUpdateOutcome{}, err
	}
	state := s.state()
	state.pending = nil
	state.last = outcome
	return state.last, nil
}

func (s *appUpdateFixtureStore) Close() error {
	if s.inner == nil {
		return nil
	}
	err := s.inner.Close()
	s.inner = nil
	return err
}

func newAppUpdateFixtureStore(t *testing.T) *appUpdateFixtureStore {
	t.Helper()
	t.Setenv("HOME", canonicalMaatFixtureDir(t))
	if _, err := maat.InitializeDefaultStore(); err != nil {
		t.Fatalf("initialize private app-update memory fixture: %v", err)
	}
	return &appUpdateFixtureStore{}
}

func appUpdatePlanFixture(t *testing.T) maintenanceInstallerPlan {
	t.Helper()
	stage := filepath.Join(canonicalMaatFixtureDir(t), "sirsi-maintenance-installer-fixture")
	if err := os.Mkdir(stage, 0700); err != nil {
		t.Fatal(err)
	}
	name := "SirsiPantheon-0.24.200-arm64.pkg"
	return maintenanceInstallerPlan{Schema: 1, Release: "0.24.200", Package: filepath.Join(stage, name),
		PackageURL: "https://github.com/SirsiMaster/sirsi-pantheon/releases/download/v0.24.200/" + name,
		SHA256:     strings.Repeat("a", 64), Device: 1, Inode: 2, Bytes: 2048, Implementation: strings.Repeat("b", 64),
		EngineSHA256: strings.Repeat("c", 64), InstallerIdentity: "Developer ID Installer: Sirsi Technologies Inc. (9D382WV988)",
		AppUpdateIdentity: &maintenanceAppUpdateIdentity{EmbeddedHelperSHA256: strings.Repeat("c", 64), AppCDHash: strings.Repeat("d", 40),
			CandidateVersion: modversion.Info{Binary: "sirsi", Version: "0.24.200", Commit: "fixture", RouterSchemaMax: 25}},
		Notarization: "Notarized Developer ID", TargetBundleID: "ai.sirsi.pantheon", TargetAppPath: "/Applications/Pantheon.app",
		TargetHelperPath: "/usr/local/libexec/sirsi-pantheon/sirsi", Dependencies: []string{"/Applications/Pantheon.app", "/usr/local/libexec/sirsi-pantheon/sirsi"}}
}

func installAppUpdateFixture(t *testing.T, plan maintenanceInstallerPlan, handoffErr error, installed bool) (*appUpdateFixtureStore, *maat.FileDecisionJournal, *int, *int) {
	t.Helper()
	store := newAppUpdateFixtureStore(t)
	journal, err := maat.NewDefaultDecisionJournal()
	if err != nil {
		t.Fatalf("open private FileDecisionJournal fixture: %v", err)
	}
	handoffs, stages := 0, 0
	previous := maatAppUpdateDependencies
	maatAppUpdateDependencies = maatAppUpdateOps{
		storeStatus: maat.InspectDefaultStore,
		openStore:   store.open,
		journal:     func() (maat.DecisionJournal, error) { return journal, nil },
		stage: func(context.Context, string, *updater.Release) (maintenanceInstallerPlan, error) {
			stages++
			return plan, nil
		},
		revalidate: func(context.Context, *updater.Release, maintenanceInstallerPlan) error { return nil },
		observe: func(context.Context) (maat.AppUpdateObservation, error) {
			store.observed++
			if installed && handoffs > 0 && handoffErr == nil {
				return maat.AppUpdateObservation{Release: plan.Release, BundleID: plan.TargetBundleID, AppPath: plan.TargetAppPath,
					HelperPath: plan.TargetHelperPath, HelperSHA256: plan.EngineSHA256, EmbeddedHelperSHA256: plan.AppUpdateIdentity.EmbeddedHelperSHA256,
					AppCDHash: plan.AppUpdateIdentity.AppCDHash, AppSignatureVerified: true,
					HelperSignatureVerified: true, DependenciesPresent: true, ObservedAtUTC: time.Now().UTC()}, nil
			}
			return maat.AppUpdateObservation{Release: "0.24.136", BundleID: plan.TargetBundleID, AppPath: plan.TargetAppPath,
				HelperPath: plan.TargetHelperPath, HelperSHA256: strings.Repeat("e", 64), AppSignatureVerified: true,
				EmbeddedHelperSHA256: strings.Repeat("e", 64), AppCDHash: strings.Repeat("f", 40),
				HelperSignatureVerified: true, DependenciesPresent: true, ObservedAtUTC: time.Now().UTC()}, nil
		},
		liveSchema: func() (int, error) { return 25, nil },
		handoff:    func(context.Context, string) error { handoffs++; return handoffErr },
	}
	t.Cleanup(func() { maatAppUpdateDependencies = previous })
	return store, journal, &handoffs, &stages
}

func TestAppUpdateSchemaRefusalAndDriftNeverReachInstaller(t *testing.T) {
	for _, mode := range []string{"candidate-below-live", "live-changed-before-intent", "live-changed-before-handoff", "target-changed-before-handoff"} {
		t.Run(mode, func(t *testing.T) {
			plan := appUpdatePlanFixture(t)
			store := newAppUpdateFixtureStore(t)
			journal, err := maat.NewDefaultDecisionJournal()
			if err != nil {
				t.Fatalf("open private FileDecisionJournal fixture: %v", err)
			}
			handoffs, liveCalls := 0, 0
			previous := maatAppUpdateDependencies
			maatAppUpdateDependencies = maatAppUpdateOps{
				storeStatus: maat.InspectDefaultStore,
				openStore:   store.open, journal: func() (maat.DecisionJournal, error) { return journal, nil },
				stage:      func(context.Context, string, *updater.Release) (maintenanceInstallerPlan, error) { return plan, nil },
				revalidate: func(context.Context, *updater.Release, maintenanceInstallerPlan) error { return nil },
				observe: func(context.Context) (maat.AppUpdateObservation, error) {
					if store.pending != nil && handoffs == 0 && mode == "target-changed-before-handoff" {
						return maat.AppUpdateObservation{Release: "0.24.136", BundleID: plan.TargetBundleID, AppPath: plan.TargetAppPath,
							HelperPath: plan.TargetHelperPath, HelperSHA256: strings.Repeat("e", 64), EmbeddedHelperSHA256: strings.Repeat("e", 64),
							AppCDHash: strings.Repeat("a", 40), AppSignatureVerified: true, HelperSignatureVerified: true, DependenciesPresent: true, ObservedAtUTC: time.Now().UTC()}, nil
					}
					return maat.AppUpdateObservation{Release: "0.24.136", BundleID: plan.TargetBundleID, AppPath: plan.TargetAppPath,
						HelperPath: plan.TargetHelperPath, HelperSHA256: strings.Repeat("e", 64), EmbeddedHelperSHA256: strings.Repeat("e", 64),
						AppCDHash: strings.Repeat("f", 40), AppSignatureVerified: true, HelperSignatureVerified: true, DependenciesPresent: true, ObservedAtUTC: time.Now().UTC()}, nil
				},
				liveSchema: func() (int, error) {
					liveCalls++
					if mode == "candidate-below-live" {
						return 26, nil
					}
					if (mode == "live-changed-before-intent" && liveCalls > 1) || (mode == "live-changed-before-handoff" && liveCalls > 2) {
						return 26, nil
					}
					return 25, nil
				},
				handoff: func(context.Context, string) error { handoffs++; return nil },
			}
			t.Cleanup(func() { maatAppUpdateDependencies = previous })
			if err := installAppReleaseWithMaat(&updater.Release{TagName: "v0.24.200"}); err == nil {
				t.Fatal("schema or installed-target drift passed")
			}
			if handoffs != 0 {
				t.Fatalf("refusal reached Installer %d times", handoffs)
			}
			if mode == "live-changed-before-handoff" || mode == "target-changed-before-handoff" {
				if store.pending != nil {
					t.Fatal("known pre-handoff refusal left unresolvable pending attempt")
				}
				if store.started != 1 {
					t.Fatalf("attempt count=%d, want one retained then terminalized", store.started)
				}
				if mode == "target-changed-before-handoff" && store.last.NotInvokedReason != "target-revalidation-refused" {
					t.Fatalf("target drift disposition=%+v", store.last)
				}
			}
		})
	}
}

func TestKnownPostIntentPackageRefusalIsRetainedNotInvokedAndDoesNotHandOff(t *testing.T) {
	plan := appUpdatePlanFixture(t)
	store, _, handoffs, _ := installAppUpdateFixture(t, plan, nil, false)
	revalidations := 0
	maatAppUpdateDependencies.revalidate = func(context.Context, *updater.Release, maintenanceInstallerPlan) error {
		revalidations++
		if revalidations == 2 {
			return errors.New("fixture package identity changed before handoff")
		}
		return nil
	}
	if err := installAppReleaseWithMaat(&updater.Release{TagName: "v0.24.200"}); err == nil {
		t.Fatal("known pre-handoff refusal returned success")
	}
	if revalidations != 2 || store.started != 1 || store.pending != nil || *handoffs != 0 ||
		store.last.Determination != "not-invoked" || store.last.NotInvokedReason != "package-revalidation-refused" {
		t.Fatalf("pre-handoff refusal state: checks=%d starts=%d pending=%v handoffs=%d outcome=%+v", revalidations, store.started, store.pending != nil, *handoffs, store.last)
	}
}

func TestAppUpdateDefaultLiveSchemaObserverUsesExistingCeilingPredicate(t *testing.T) {
	db := filepath.Join(t.TempDir(), "router.db")
	if err := os.WriteFile(db, []byte("fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SIRSI_ROUTER_URL", "")
	t.Setenv("SIRSI_ROUTER_DB", db)
	oldRead := readSchemaFn
	readSchemaFn = func(path string) (int, error) {
		if path != db {
			t.Fatalf("schema read path=%q, want %q", path, db)
		}
		return 25, nil
	}
	t.Cleanup(func() { readSchemaFn = oldRead })
	live, err := observeAppUpdateLiveSchema()
	if err != nil || live != 25 {
		t.Fatalf("default live schema=%d err=%v", live, err)
	}
	candidate := appUpdatePlanFixture(t).AppUpdateIdentity.CandidateVersion
	if err := selfupdate.CheckSchemaCeiling(candidate, live); err != nil {
		t.Fatalf("compatible candidate refused: %v", err)
	}
	candidate.RouterSchemaMax = 24
	if err := selfupdate.CheckSchemaCeiling(candidate, live); err == nil {
		t.Fatal("candidate below live schema passed")
	}
	candidate.RouterSchemaMax = 0
	if err := selfupdate.CheckSchemaCeiling(candidate, live); err == nil {
		t.Fatal("missing schema ceiling passed")
	}
}

func TestMaatAppUpdateRevalidatorChecksPackageAfterTrustInspection(t *testing.T) {
	for _, mutate := range []bool{false, true} {
		t.Run(map[bool]string{false: "unchanged", true: "replaced-during-inspection"}[mutate], func(t *testing.T) {
			plan := appUpdatePlanFixture(t)
			original := []byte("exact reviewed package payload")
			if err := os.WriteFile(plan.Package, original, 0600); err != nil {
				t.Fatal(err)
			}
			stat, err := maintenanceInstallerFileHash(plan.Package, true)
			if err != nil {
				t.Fatal(err)
			}
			plan.SHA256, plan.Device, plan.Inode, plan.Bytes = stat.SHA256, stat.Device, stat.Inode, stat.Bytes
			release := &updater.Release{TagName: "v0.24.200", Assets: []updater.Asset{
				{Name: filepath.Base(plan.Package), BrowserDownloadURL: plan.PackageURL, Size: plan.Bytes, Digest: "sha256:" + plan.SHA256},
				{Name: "SirsiPantheon-0.24.200-arm64.dmg", BrowserDownloadURL: "https://github.com/SirsiMaster/sirsi-pantheon/releases/download/v0.24.200/SirsiPantheon-0.24.200-arm64.dmg", Size: 8, Digest: "sha256:" + strings.Repeat("a", 64)},
			}}
			oldInspector := maatAppUpdatePackageInspector
			maatAppUpdatePackageInspector = func(context.Context, string, string) (maintenancePackageInspection, error) {
				if mutate {
					if err := os.WriteFile(plan.Package, []byte("different package payload"), 0600); err != nil {
						t.Fatal(err)
					}
				}
				return maintenancePackageInspection{Capability: setup.LegacyAppCapability{Implementation: plan.Implementation}, EngineSHA256: plan.EngineSHA256,
					EmbeddedHelperSHA256: plan.AppUpdateIdentity.EmbeddedHelperSHA256, AppCDHash: plan.AppUpdateIdentity.AppCDHash,
					CandidateVersion:  plan.AppUpdateIdentity.CandidateVersion,
					InstallerIdentity: plan.InstallerIdentity, Notarization: plan.Notarization, TargetBundleID: plan.TargetBundleID,
					TargetAppPath: plan.TargetAppPath, TargetHelperPath: plan.TargetHelperPath, Dependencies: plan.Dependencies}, nil
			}
			t.Cleanup(func() { maatAppUpdatePackageInspector = oldInspector })
			err = revalidateMaatAppUpdatePackage(context.Background(), release, plan)
			if mutate && err == nil {
				t.Fatal("package changed during trust inspection passed final identity readback")
			}
			if !mutate && err != nil {
				t.Fatalf("unchanged package refused: %v", err)
			}
		})
	}
}

func TestInstalledAppObserverHashesActualSignedAppHelperPair(t *testing.T) {
	for _, mode := range []string{"exact", "different-embedded-helper", "linked-helper"} {
		t.Run(mode, func(t *testing.T) {
			root := filepath.Join(t.TempDir(), "Pantheon.app")
			embedded := filepath.Join(root, "Contents", "MacOS", "sirsi")
			protected := filepath.Join(t.TempDir(), "sirsi")
			if err := os.MkdirAll(filepath.Dir(embedded), 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(root, "Contents", "Info.plist"), []byte("fixture"), 0600); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(protected, []byte("identical signed helper"), 0700); err != nil {
				t.Fatal(err)
			}
			if mode == "different-embedded-helper" {
				if err := os.WriteFile(embedded, []byte("different helper"), 0700); err != nil {
					t.Fatal(err)
				}
			} else if mode == "linked-helper" {
				if err := os.Symlink(protected, embedded); err != nil {
					t.Fatal(err)
				}
			} else if err := os.WriteFile(embedded, []byte("identical signed helper"), 0700); err != nil {
				t.Fatal(err)
			}
			read := func(_ context.Context, exe string, args ...string) ([]byte, error) {
				if exe == "/usr/libexec/PlistBuddy" {
					return []byte("0.24.200"), nil
				}
				return []byte("signature fixture"), nil
			}
			cdhash := func(context.Context, string) (string, error) { return strings.Repeat("d", 40), nil }
			observation, err := observeMaatAppUpdateAt(context.Background(), root, protected, read, maintenanceInstallerFileHash, cdhash)
			if mode == "exact" {
				if err != nil {
					t.Fatal(err)
				}
				if !observation.DependenciesPresent || observation.HelperSHA256 != observation.EmbeddedHelperSHA256 || observation.AppCDHash != strings.Repeat("d", 40) {
					t.Fatalf("actual file pair is not bound: %+v", observation)
				}
			} else if err == nil {
				t.Fatalf("%s helper pair passed observer", mode)
			}
		})
	}
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
	decisions, err := journal.Recent(10)
	if err != nil || len(decisions) != 1 || decisions[0].Kind != "failure memory preflight" || decisions[0].Determination != "pass" {
		t.Fatalf("full scoped preflight was not projected through the real journal before handoff: %+v err=%v", decisions, err)
	}
}

func TestAmbiguousAppInstallerHandoffReconcilesDataOnlyWithoutReplay(t *testing.T) {
	plan := appUpdatePlanFixture(t)
	store, _, handoffs, stages := installAppUpdateFixture(t, plan, errors.New("handoff outcome unknown"), false)
	if err := installAppReleaseWithMaat(&updater.Release{TagName: "v0.24.200"}); err == nil || store.pending == nil {
		t.Fatalf("ambiguous Installer handoff lost its durable pending intent: err=%v pending=%v", err, store.pending != nil)
	}
	attemptID := store.pending.ID
	openedBefore, observedBefore := store.opened, store.observed
	if handled, err := reconcilePendingMaatAppUpdate(); !handled || err == nil ||
		!strings.Contains(err.Error(), attemptID) || !strings.Contains(err.Error(), "current installed identity does not match the retained package") {
		t.Fatalf("reopened data-only reconciliation did not reach exact retained identity debt: handled=%v err=%v", handled, err)
	}
	if store.opened != openedBefore+1 || store.observed != observedBefore+1 {
		t.Fatalf("reconciliation did not open a fresh Store and perform exactly one readback: opened %d->%d observed %d->%d", openedBefore, store.opened, observedBefore, store.observed)
	}
	reopened, err := store.open()
	if err != nil {
		t.Fatalf("reopen fixture Store for exact debt readback: %v", err)
	}
	pending, err := reopened.PendingAppUpdateAttempt()
	if err != nil || pending.ID != attemptID {
		t.Fatalf("reopened fixture lost or changed exact unresolved attempt: want=%s got=%+v err=%v", attemptID, pending, err)
	}
	if err := reopened.Close(); err != nil {
		t.Fatal(err)
	}
	oldClient := newUpdateClient
	oldCLI, oldApp := updateInstallCLI, updateInstallApp
	newUpdateClient = func() *updater.Client { t.Fatal("pending readback performed a network update check"); return nil }
	updateInstallCLI, updateInstallApp = false, true
	defer func() { newUpdateClient, updateInstallCLI, updateInstallApp = oldClient, oldCLI, oldApp }()
	if err := runUpdate(updateCmd, nil); err == nil {
		t.Fatal("unresolved app update was hidden by a network check")
	}
	if store.started != 1 || *handoffs != 1 || *stages != 1 || store.pending == nil || store.pending.ID != attemptID {
		t.Fatalf("ambiguous handoff was replayed or exact debt lost: starts=%d handoffs=%d stages=%d pending=%+v", store.started, *handoffs, *stages, store.pending)
	}
}
