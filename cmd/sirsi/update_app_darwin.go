//go:build darwin

package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/maat"
	"github.com/SirsiMaster/sirsi-pantheon/internal/selfupdate"
	"github.com/SirsiMaster/sirsi-pantheon/internal/setup"
	"github.com/SirsiMaster/sirsi-pantheon/internal/updater"
)

type maatAppUpdateStore interface {
	PreflightAppUpdate(maat.AppUpdateManifest) (maat.PreflightReceipt, error)
	RetainPreflightReceipt(maat.PreflightReceipt) (maat.StoredPreflightReceipt, error)
	BeginAppUpdateAttempt(maat.AppUpdateManifest, maat.StoredPreflightReceipt) (maat.AppUpdateAttempt, error)
	PendingAppUpdateAttempt() (maat.AppUpdateAttempt, error)
	RecordAppUpdateInstalled(maat.AppUpdateAttempt, maat.AppUpdateObservation) (maat.AppUpdateOutcome, error)
	RecordAppUpdateNotInvoked(maat.AppUpdateAttempt, string) (maat.AppUpdateOutcome, error)
	Close() error
}

type maatAppUpdateOps struct {
	storeStatus        func() maat.DefaultStoreStatus
	openStore          func() (maatAppUpdateStore, error)
	journal            func() (maat.DecisionJournal, error)
	stage              func(context.Context, string, *updater.Release) (maintenanceInstallerPlan, error)
	stageRollback      func(context.Context, string, *updater.Release) (maintenanceInstallerPlan, error)
	revalidate         func(context.Context, *updater.Release, maintenanceInstallerPlan) error
	revalidateRollback func(context.Context, *updater.Release, maintenanceInstallerPlan) error
	observe            func(context.Context) (maat.AppUpdateObservation, error)
	liveSchema         func() (int, error)
	handoff            func(context.Context, string) error
}

var maatAppUpdateDependencies = maatAppUpdateOps{
	storeStatus:        maat.InspectDefaultStore,
	openStore:          func() (maatAppUpdateStore, error) { return maat.OpenDefaultStore() },
	journal:            func() (maat.DecisionJournal, error) { return maat.NewDefaultDecisionJournal() },
	stage:              stageMaatAppUpdatePackage,
	stageRollback:      stageMaatAppRollbackPackage,
	revalidate:         revalidateMaatAppUpdatePackage,
	revalidateRollback: revalidateMaatAppRollbackPackage,
	observe:            observeMaatAppUpdate,
	liveSchema:         observeAppUpdateLiveSchema,
	handoff:            openMaatAppInstaller,
}

var maatAppUpdatePackageInspector = inspectAppUpdatePKG
var maatAppRollbackPackageInspector = inspectAppRollbackPKG

func reconcilePendingMaatAppUpdate() (bool, error) {
	status := maatAppUpdateDependencies.storeStatus()
	if status.State != maat.DefaultStoreReady {
		return true, errors.New("Ma'at app update requires the existing initialized failure-memory store; no network check or Installer action occurred")
	}
	store, err := maatAppUpdateDependencies.openStore()
	if err != nil {
		return true, fmt.Errorf("Ma'at app update: open existing failure-memory store: %w", err)
	}
	defer store.Close()
	pending, err := store.PendingAppUpdateAttempt()
	if errors.Is(err, maat.ErrNoIncompleteAppUpdateAttempt) {
		return false, nil
	}
	if err != nil {
		return true, fmt.Errorf("Ma'at app update attempt history is unverifiable; network and Installer replay refused: %w", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	return true, reconcileMaatAppUpdate(ctx, store, pending)
}

func appUpdateRecoveryCapability() (maat.AppUpdateRecoveryCapability, error) {
	if maatAppUpdateDependencies.storeStatus == nil || maatAppUpdateDependencies.openStore == nil || maatAppUpdateDependencies.observe == nil {
		return maat.AppUpdateRecoveryCapability{}, errors.New("whole-app attempt reader or installed-pair observer is not wired")
	}
	return maat.CurrentAppUpdateRecoveryCapability(), nil
}

func reconcileMaatAppUpdateReport() (appUpdateReconciliationReport, error) {
	report := appUpdateReconciliationReport{Schema: appUpdateReconciliationSchema, State: "unverifiable"}
	status := maatAppUpdateDependencies.storeStatus()
	if status.State != maat.DefaultStoreReady {
		err := errors.New("Ma'at app-attempt readback requires the existing initialized failure-memory store")
		report.Reason = err.Error()
		return report, err
	}
	store, err := maatAppUpdateDependencies.openStore()
	if err != nil {
		report.Reason = err.Error()
		return report, err
	}
	defer store.Close()
	attempt, err := store.PendingAppUpdateAttempt()
	if errors.Is(err, maat.ErrNoIncompleteAppUpdateAttempt) {
		report.State = "no_pending_attempt"
		return report, nil
	}
	if err != nil {
		report.Reason = err.Error()
		return report, err
	}
	report.AttemptID = attempt.ID
	report.TargetRelease = attempt.Manifest.Release
	report.CurrentRelease = attempt.Manifest.CurrentRelease
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	observation, reconcileErr := reconcileMaatAppUpdateObserved(ctx, store, attempt)
	if observation != (maat.AppUpdateObservation{}) {
		report.Observation = &observation
	}
	if reconcileErr != nil {
		report.State = "unresolved"
		report.Reason = reconcileErr.Error()
		return report, reconcileErr
	}
	report.State = "resolved"
	return report, nil
}

func installAppReleaseWithMaat(rel *updater.Release) error {
	return installAppReleaseWithMaatDirection(rel, false)
}

func installAppRollbackWithMaat(rel *updater.Release) error {
	return installAppReleaseWithMaatDirection(rel, true)
}

func installAppReleaseWithMaatDirection(rel *updater.Release, rollback bool) error {
	if os.Getuid() == 0 || os.Geteuid() == 0 {
		return errors.New("Ma'at app update must use the signed package from the unprivileged user; no installer was opened")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	status := maatAppUpdateDependencies.storeStatus()
	if status.State != maat.DefaultStoreReady {
		return errors.New("Ma'at app update requires the existing initialized failure-memory store; no installer was opened")
	}
	store, err := maatAppUpdateDependencies.openStore()
	if err != nil {
		return fmt.Errorf("Ma'at app update: open existing failure-memory store: %w", err)
	}
	defer store.Close()
	if pending, pendingErr := store.PendingAppUpdateAttempt(); pendingErr == nil {
		return reconcileMaatAppUpdate(ctx, store, pending)
	} else if !errors.Is(pendingErr, maat.ErrNoIncompleteAppUpdateAttempt) {
		return fmt.Errorf("Ma'at app update attempt history is unverifiable; Installer replay refused: %w", pendingErr)
	}
	if rel == nil {
		return errors.New("Ma'at app update: exact commercial release is unavailable; no installer was opened")
	}
	journal, err := maatAppUpdateDependencies.journal()
	if err != nil {
		return fmt.Errorf("Ma'at app update: open Casebook: %w", err)
	}
	before, err := maatAppUpdateDependencies.observe(ctx)
	if err != nil {
		return fmt.Errorf("Ma'at app update: current installed identity is unverifiable: %w", err)
	}
	if before.Release == strings.TrimPrefix(rel.TagName, "v") {
		return errors.New("Ma'at app update: exact release is already installed; no Installer handoff or replay occurred")
	}
	validateVersion := updater.ValidateUpdateVersion
	stage := maatAppUpdateDependencies.stage
	revalidate := maatAppUpdateDependencies.revalidate
	if rollback {
		validateVersion = updater.ValidateRollbackVersion
		stage = maatAppUpdateDependencies.stageRollback
		revalidate = maatAppUpdateDependencies.revalidateRollback
	}
	if err := validateVersion(before.Release, rel.TagName); err != nil {
		return fmt.Errorf("Ma'at app rollback selection is not an exact compatible older release: %w", err)
	}
	if stage == nil || revalidate == nil {
		return errors.New("Ma'at app rollback adapter is unavailable; no Installer handoff")
	}
	liveSchema, err := maatAppUpdateDependencies.liveSchema()
	if err != nil {
		return fmt.Errorf("Ma'at app update: live router schema is unavailable; no Installer handoff: %w", err)
	}
	plan, err := stage(ctx, before.Release, rel)
	if err != nil {
		return fmt.Errorf("Ma'at app update package was not staged as a verified exact tuple: %w", err)
	}
	if plan.Release != strings.TrimPrefix(rel.TagName, "v") || plan.CurrentRelease != before.Release {
		return errors.New("Ma'at app release selection or observed source identity changed during staging; no Installer handoff")
	}
	if plan.AppUpdateIdentity == nil {
		return errors.New("Ma'at app update package lacks its inspected app/helper/version identity; no Installer handoff")
	}
	if rollback && (plan.AppUpdateRecoveryCapability == nil || *plan.AppUpdateRecoveryCapability != maat.CurrentAppUpdateRecoveryCapability()) {
		return errors.New("Ma'at rollback target does not prove the whole-app attempt recovery reader/schema; no Installer handoff")
	}
	if err := selfupdate.CheckSchemaCeiling(plan.AppUpdateIdentity.CandidateVersion, liveSchema); err != nil {
		return fmt.Errorf("Ma'at app update candidate cannot open the live router schema; no Installer handoff: %w", err)
	}
	manifest := maat.AppUpdateManifest{
		Schema: maat.AppUpdateManifestSchema, StoreUUID: status.StoreUUID, UID: uint32(os.Geteuid()),
		Release: plan.Release, PackageName: filepath.Base(plan.Package), PackageURL: plan.PackageURL,
		PackagePath: plan.Package, PackageSHA256: plan.SHA256, PackageDevice: plan.Device, PackageInode: plan.Inode,
		PackageBytes: plan.Bytes, InstallerIdentity: plan.InstallerIdentity, Notarization: plan.Notarization,
		TargetBundleID: plan.TargetBundleID, TargetAppPath: plan.TargetAppPath, TargetHelperPath: plan.TargetHelperPath,
		ExpectedInstalledHelperSHA256: plan.EngineSHA256, ExpectedEmbeddedHelperSHA256: plan.AppUpdateIdentity.EmbeddedHelperSHA256,
		ExpectedAppCDHash: plan.AppUpdateIdentity.AppCDHash, CandidateVersion: plan.AppUpdateIdentity.CandidateVersion, LiveRouterSchema: liveSchema,
		CurrentRelease: before.Release, CurrentHelperSHA256: before.HelperSHA256,
		Dependencies: append([]string(nil), plan.Dependencies...), ObservedAtUTC: time.Now().UTC(),
	}
	receipt, preflightErr := store.PreflightAppUpdate(manifest)
	if preflightErr != nil && (receipt.Decision != maat.PreflightUnverifiable || receipt.RegistrySnapshotSHA256 == "") {
		return fmt.Errorf("Ma'at app update failure-memory preflight is unverifiable: %w", preflightErr)
	}
	stored, err := store.RetainPreflightReceipt(receipt)
	if err != nil {
		return fmt.Errorf("retain and read back complete Ma'at app update preflight before Installer handoff: %w", err)
	}
	if err := maat.ProjectFailureMemoryPreflight(journal, stored); err != nil {
		return fmt.Errorf("retain Ma'at app update preflight projection before Installer handoff: %w", err)
	}
	if preflightErr != nil {
		return fmt.Errorf("Ma'at app update failure-memory preflight is unverifiable; Installer refused: %w", preflightErr)
	}
	if receipt.Decision != maat.PreflightPass {
		return fmt.Errorf("Ma'at app update failure-memory preflight %s; Installer refused", receipt.Decision)
	}
	if err := revalidate(ctx, rel, plan); err != nil {
		return fmt.Errorf("Ma'at app update package changed before durable intent; Installer refused: %w", err)
	}
	if err := recheckAppUpdateSchema(liveSchema); err != nil {
		return fmt.Errorf("Ma'at app update live schema changed before durable intent; Installer refused: %w", err)
	}
	current, err := maatAppUpdateDependencies.observe(ctx)
	if err != nil || !sameInstalledAppUpdateObservation(before, current) {
		return errors.Join(errors.New("Ma'at app update current target changed before durable intent; Installer refused"), err)
	}
	attempt, err := store.BeginAppUpdateAttempt(manifest, stored)
	if err != nil {
		return fmt.Errorf("Ma'at app update durable intent was not admitted; Installer was not opened: %w", err)
	}
	// Revalidate all mutable observations after durable intent. If any check
	// refuses, persist an exact not-invoked fact before allowing a fresh review.
	if err := recheckAppUpdateSchema(liveSchema); err != nil {
		if _, dispositionErr := store.RecordAppUpdateNotInvoked(attempt, "schema-revalidation-refused"); dispositionErr != nil {
			return fmt.Errorf("Ma'at app update attempt %s remains retained; schema revalidation refused before Installer handoff and exact disposition failed: %w (schema: %v)", attempt.ID, dispositionErr, err)
		}
		return fmt.Errorf("Ma'at app update attempt %s was retained as not-invoked after schema changed; review a compatible release: %w", attempt.ID, err)
	}
	current, err = maatAppUpdateDependencies.observe(ctx)
	if err != nil || !sameInstalledAppUpdateObservation(before, current) {
		if _, dispositionErr := store.RecordAppUpdateNotInvoked(attempt, "target-revalidation-refused"); dispositionErr != nil {
			return fmt.Errorf("Ma'at app update attempt %s remains retained; target revalidation refused before Installer handoff and exact disposition failed: %w (readback: %v)", attempt.ID, dispositionErr, err)
		}
		if err == nil {
			err = errors.New("installed app/helper identity changed")
		}
		return fmt.Errorf("Ma'at app update attempt %s was retained as not-invoked after installed target changed; review fresh state: %w", attempt.ID, err)
	}
	// This final no-follow package hash follows the trust inspection and the
	// other observations, immediately before the only native Installer call.
	if err := revalidate(ctx, rel, plan); err != nil {
		if _, dispositionErr := store.RecordAppUpdateNotInvoked(attempt, "package-revalidation-refused"); dispositionErr != nil {
			return fmt.Errorf("Ma'at app update attempt %s remains retained; package revalidation refused before Installer handoff and exact disposition failed: %w (revalidation: %v)", attempt.ID, dispositionErr, err)
		}
		return fmt.Errorf("Ma'at app update attempt %s was retained as not-invoked after package revalidation refused; stage and review a fresh exact package: %w", attempt.ID, err)
	}
	if err := maatAppUpdateDependencies.handoff(ctx, plan.Package); err != nil {
		return fmt.Errorf("Ma'at app update attempt %s is retained; Installer handoff is ambiguous and must be read back before retry: %w", attempt.ID, err)
	}
	observation, observeErr := maatAppUpdateDependencies.observe(ctx)
	if observeErr != nil {
		return fmt.Errorf("Ma'at app update attempt %s is retained; Installer returned but installed identity readback is unavailable; do not replay: %w", attempt.ID, observeErr)
	}
	if _, err := store.RecordAppUpdateInstalled(attempt, observation); err != nil {
		return fmt.Errorf("Ma'at app update attempt %s remains unresolved after Installer handoff; use a later data-only readback, do not replay: %w", attempt.ID, err)
	}
	if rollback {
		fmt.Printf("Ma'at verified rollback to Pantheon %s; retained package and installed app/helper identity were read back.\n", plan.Release)
	} else {
		fmt.Printf("Ma'at verified installed Pantheon %s; package and installed app/helper identity were read back.\n", plan.Release)
	}
	return nil
}

func sameInstalledAppUpdateObservation(a, b maat.AppUpdateObservation) bool {
	return a.Release == b.Release && a.BundleID == b.BundleID && a.AppPath == b.AppPath && a.HelperPath == b.HelperPath &&
		a.HelperSHA256 == b.HelperSHA256 && a.EmbeddedHelperSHA256 == b.EmbeddedHelperSHA256 && a.AppCDHash == b.AppCDHash &&
		a.AppSignatureVerified && b.AppSignatureVerified && a.HelperSignatureVerified && b.HelperSignatureVerified &&
		a.DependenciesPresent && b.DependenciesPresent
}

func observeAppUpdateLiveSchema() (int, error) {
	dbPath, err := resolveRouterDBPath()
	if err != nil {
		return 0, err
	}
	return resolveLiveSchema(dbPath)
}

func recheckAppUpdateSchema(reviewed int) error {
	current, err := maatAppUpdateDependencies.liveSchema()
	if err != nil {
		return err
	}
	if current != reviewed {
		return fmt.Errorf("live router schema changed from %d to %d", reviewed, current)
	}
	return nil
}

func reconcileMaatAppUpdate(ctx context.Context, store maatAppUpdateStore, attempt maat.AppUpdateAttempt) error {
	if _, err := reconcileMaatAppUpdateObserved(ctx, store, attempt); err != nil {
		return err
	}
	fmt.Printf("Ma'at reconciled installed Pantheon %s from retained attempt %s; no Installer was run.\n", attempt.Manifest.Release, attempt.ID)
	return nil
}

func reconcileMaatAppUpdateObserved(ctx context.Context, store maatAppUpdateStore, attempt maat.AppUpdateAttempt) (maat.AppUpdateObservation, error) {
	observation, err := maatAppUpdateDependencies.observe(ctx)
	if err != nil {
		return observation, fmt.Errorf("Ma'at app update attempt %s remains unresolved; readback unavailable, Installer was not replayed: %w", attempt.ID, err)
	}
	if _, err := store.RecordAppUpdateInstalled(attempt, observation); err != nil {
		return observation, fmt.Errorf("Ma'at app update attempt %s remains unresolved; current installed identity does not match the retained package, Installer was not replayed: %w", attempt.ID, err)
	}
	return observation, nil
}

func stageMaatAppUpdatePackage(ctx context.Context, current string, rel *updater.Release) (maintenanceInstallerPlan, error) {
	return stageMaintenanceInstaller(ctx, current, maintenanceInstallerOps{
		release: func() (*updater.Release, error) { return rel, nil }, download: maintenanceDownload, inspect: inspectAppUpdatePKG,
	})
}

func stageMaatAppRollbackPackage(ctx context.Context, current string, rel *updater.Release) (maintenanceInstallerPlan, error) {
	return stageMaintenanceRollbackInstaller(ctx, current, maintenanceInstallerOps{
		release: func() (*updater.Release, error) { return rel, nil }, download: maintenanceDownload, inspect: inspectAppRollbackPKG,
	})
}

func revalidateMaatAppUpdatePackage(ctx context.Context, rel *updater.Release, plan maintenanceInstallerPlan) error {
	return revalidateMaatAppPackageDirection(ctx, rel, plan, false)
}

func revalidateMaatAppRollbackPackage(ctx context.Context, rel *updater.Release, plan maintenanceInstallerPlan) error {
	return revalidateMaatAppPackageDirection(ctx, rel, plan, true)
}

func revalidateMaatAppPackageDirection(ctx context.Context, rel *updater.Release, plan maintenanceInstallerPlan, rollback bool) error {
	if plan.CurrentRelease == "" {
		return errors.New("staged PKG is missing the exact observed installed release")
	}
	asset, err := maintenancePackageAssetDirection(plan.CurrentRelease, rel, rollback)
	if err != nil {
		return err
	}
	if asset.Name != filepath.Base(plan.Package) || asset.BrowserDownloadURL != plan.PackageURL ||
		asset.Size != plan.Bytes || strings.TrimPrefix(asset.Digest, "sha256:") != plan.SHA256 {
		return errors.New("published package tuple differs from the retained update manifest")
	}
	current, err := maintenanceInstallerFileHash(plan.Package, true)
	if err != nil || current.SHA256 != plan.SHA256 || current.Device != plan.Device || current.Inode != plan.Inode || current.Bytes != plan.Bytes {
		return errors.Join(errors.New("staged package bytes or file identity changed"), err)
	}
	inspector := maatAppUpdatePackageInspector
	if rollback {
		inspector = maatAppRollbackPackageInspector
	}
	inspection, err := inspector(ctx, plan.Package, plan.Release)
	if err != nil {
		return err
	}
	if plan.AppUpdateIdentity == nil || inspection.Capability.Implementation != plan.Implementation || inspection.EngineSHA256 != plan.EngineSHA256 ||
		inspection.EmbeddedHelperSHA256 != plan.AppUpdateIdentity.EmbeddedHelperSHA256 ||
		(rollback && (plan.AppUpdateRecoveryCapability == nil || inspection.AppUpdateRecoveryCapability != *plan.AppUpdateRecoveryCapability)) || inspection.AppCDHash != plan.AppUpdateIdentity.AppCDHash ||
		inspection.CandidateVersion != plan.AppUpdateIdentity.CandidateVersion ||
		inspection.InstallerIdentity != plan.InstallerIdentity || inspection.Notarization != plan.Notarization ||
		inspection.TargetBundleID != plan.TargetBundleID || inspection.TargetAppPath != plan.TargetAppPath ||
		inspection.TargetHelperPath != plan.TargetHelperPath || !sameMaintenanceStrings(inspection.Dependencies, plan.Dependencies) {
		return errors.New("staged package signature, target, or dependency plan changed")
	}
	final, err := maintenanceInstallerFileHash(plan.Package, true)
	if err != nil || final.SHA256 != plan.SHA256 || final.Device != plan.Device || final.Inode != plan.Inode || final.Bytes != plan.Bytes {
		return errors.Join(errors.New("staged package changed during final trust assessment"), err)
	}
	return nil
}

func observeMaatAppUpdate(ctx context.Context) (maat.AppUpdateObservation, error) {
	const appPath = "/Applications/Pantheon.app"
	const helperPath = "/usr/local/libexec/sirsi-pantheon/sirsi"
	if _, err := setup.CurrentPackagedMaatEngine(); err != nil {
		return maat.AppUpdateObservation{}, err
	}
	return observeMaatAppUpdateAt(ctx, appPath, helperPath, maintenanceInstallerRead, maintenanceInstallerFileHash, signedCodeDirectoryHash)
}

func observeMaatAppUpdateAt(ctx context.Context, appPath, helperPath string,
	read func(context.Context, string, ...string) ([]byte, error),
	hash func(string, bool) (maintenanceInstallerPlan, error),
	cdhash func(context.Context, string) (string, error),
) (maat.AppUpdateObservation, error) {
	var observation maat.AppUpdateObservation
	const appRequirement = `=anchor apple generic and certificate leaf[subject.OU] = "9D382WV988" and identifier "ai.sirsi.pantheon"`
	const helperRequirement = `=anchor apple generic and certificate leaf[subject.OU] = "9D382WV988" and identifier "sirsi"`
	versionBytes, err := read(ctx, "/usr/libexec/PlistBuddy", "-c", "Print :CFBundleVersion", filepath.Join(appPath, "Contents", "Info.plist"))
	if err != nil {
		return observation, err
	}
	version := strings.TrimSpace(string(versionBytes))
	if !maintenanceVersionRE.MatchString(version) {
		return observation, errors.New("installed app bundle version is not canonical")
	}
	help, err := hash(helperPath, false)
	if err != nil {
		return observation, err
	}
	if _, err := read(ctx, "/usr/bin/codesign", "--verify", "--strict", "--deep", "-R", appRequirement, appPath); err != nil {
		return observation, errors.New("installed Pantheon app signature readback failed")
	}
	if _, err := read(ctx, "/usr/bin/codesign", "--verify", "--strict", "-R", helperRequirement, helperPath); err != nil {
		return observation, errors.New("installed Pantheon helper signature readback failed")
	}
	embeddedHelperPath := filepath.Join(appPath, "Contents", "MacOS", "sirsi")
	embeddedHelper, err := hash(embeddedHelperPath, false)
	if err != nil || embeddedHelper.SHA256 != help.SHA256 {
		return observation, errors.New("installed app/helper payload pair is missing or differs")
	}
	appCDHash, err := cdhash(ctx, appPath)
	if err != nil {
		return observation, err
	}
	if info, err := os.Lstat(appPath); err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return observation, errors.New("installed Pantheon app dependency is unavailable")
	}
	observation = maat.AppUpdateObservation{Release: version, BundleID: "ai.sirsi.pantheon", AppPath: appPath,
		HelperPath: helperPath, HelperSHA256: help.SHA256, EmbeddedHelperSHA256: embeddedHelper.SHA256, AppCDHash: appCDHash, AppSignatureVerified: true,
		HelperSignatureVerified: true, DependenciesPresent: true, ObservedAtUTC: time.Now().UTC()}
	return observation, nil
}

func openMaatAppInstaller(ctx context.Context, pkg string) error {
	if !filepath.IsAbs(pkg) || filepath.Clean(pkg) != pkg {
		return errors.New("canonical staged package path required")
	}
	cmd := exec.CommandContext(ctx, "/usr/bin/open", pkg)
	cmd.Env = []string{"PATH=/usr/bin:/bin:/usr/sbin:/sbin", "LC_ALL=C"}
	return cmd.Run()
}
