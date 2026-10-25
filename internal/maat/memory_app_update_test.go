package maat

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
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
		CurrentHelperSHA256: strings.Repeat("c", 64),
		Dependencies:        []string{"/Applications/Pantheon.app", "/usr/local/libexec/sirsi-pantheon/sirsi"},
		ObservedAtUTC:       time.Now().UTC()}
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
		"wrong target":       func(m *AppUpdateManifest) { m.TargetBundleID = "other.app" },
		"wrong dependency":   func(m *AppUpdateManifest) { m.Dependencies = []string{"/Applications/Other.app", m.TargetHelperPath} },
		"wrong signature":    func(m *AppUpdateManifest) { m.InstallerIdentity = "untrusted" },
		"wrong notarization": func(m *AppUpdateManifest) { m.Notarization = "unverified" },
		"wrong package name": func(m *AppUpdateManifest) { m.PackageName = "other.pkg" },
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
	receipt, err := store.PreflightAppUpdate(manifest)
	if err != nil || receipt.Decision != PreflightPass {
		t.Fatalf("preflight=%+v err=%v", receipt, err)
	}
	attempt, err := store.BeginAppUpdateAttempt(manifest, receipt)
	if err != nil {
		t.Fatal(err)
	}
	if attempt.PreflightReceipt.ActionManifestSHA256 != attempt.ActionManifestSHA256 || attempt.PreflightReceiptSHA256 == "" {
		t.Fatalf("durable intent lost its complete preflight receipt: %+v", attempt)
	}
	pending, err := store.PendingAppUpdateAttempt()
	if err != nil || pending.ID != attempt.ID {
		t.Fatalf("pending attempt=%+v err=%v", pending, err)
	}
	if _, err := store.BeginAppUpdateAttempt(manifest, receipt); err == nil {
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

func TestAppUpdateInstalledReadbackCompletesOnlyExactTuple(t *testing.T) {
	store, err := OpenStore(privateLivenessStoreRoot(t))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	store.storeUUID = "b3c515a2-4773-4653-86d1-8a5734c2ee45"
	manifest := fixtureAppUpdateManifest(t, store.storeUUID)
	receipt, err := store.PreflightAppUpdate(manifest)
	if err != nil {
		t.Fatal(err)
	}
	attempt, err := store.BeginAppUpdateAttempt(manifest, receipt)
	if err != nil {
		t.Fatal(err)
	}
	observation := AppUpdateObservation{Release: manifest.Release, BundleID: manifest.TargetBundleID,
		AppPath: manifest.TargetAppPath, HelperPath: manifest.TargetHelperPath,
		HelperSHA256: manifest.ExpectedInstalledHelperSHA256, AppSignatureVerified: true,
		HelperSignatureVerified: true, DependenciesPresent: true, ObservedAtUTC: time.Now().UTC()}
	outcome, err := store.RecordAppUpdateInstalled(attempt, observation)
	if err != nil || outcome.Determination != "installed" {
		t.Fatalf("outcome=%+v err=%v", outcome, err)
	}
	if _, err := store.PendingAppUpdateAttempt(); !errors.Is(err, ErrNoIncompleteAppUpdateAttempt) {
		t.Fatalf("exact installed readback left an incomplete attempt: %v", err)
	}
}
