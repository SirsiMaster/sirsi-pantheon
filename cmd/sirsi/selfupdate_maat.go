package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/maat"
	"github.com/SirsiMaster/sirsi-pantheon/internal/selfupdate"
	modversion "github.com/SirsiMaster/sirsi-pantheon/internal/version"
	"golang.org/x/sys/unix"
)

type maatRawSelfUpdateStore interface {
	Close() error
	PreflightRawSelfUpdate(maat.RawSelfUpdateManifest) (maat.PreflightReceipt, error)
	RetainPreflightReceipt(maat.PreflightReceipt) (maat.StoredPreflightReceipt, error)
	BeginRawSelfUpdateAttempt(maat.RawSelfUpdateManifest, maat.StoredPreflightReceipt) (maat.RawSelfUpdateAttempt, error)
	PendingRawSelfUpdateAttempt() (maat.RawSelfUpdateAttempt, error)
	RecordRawSelfUpdateInvocation(maat.RawSelfUpdateAttempt) error
	RecordRawSelfUpdateNotInvoked(maat.RawSelfUpdateAttempt, string, string) (maat.RawSelfUpdateOutcome, error)
	RecordRawSelfUpdateOutcome(maat.RawSelfUpdateAttempt, bool, string, string) (maat.RawSelfUpdateOutcome, error)
	RawSelfUpdateReconciliations() ([]maat.RawSelfUpdateReconciliation, error)
	MarkRawSelfUpdateProjectionReadBack(maat.RawSelfUpdateAttempt, maat.RawSelfUpdateOutcome, maat.Decision) error
}

type maatRawSelfUpdateDependencies struct {
	storeStatus func() maat.DefaultStoreStatus
	openStore   func() (maatRawSelfUpdateStore, error)
	journal     func() (maat.DecisionJournal, error)
	project     func(maat.DecisionJournal, maat.StoredPreflightReceipt) error
	selfInfo    func() modversion.Info
	probe       func(string) (modversion.Info, error)
	schema      func(modversion.Info) (int, error)
	identity    func(string) (maatBinaryFileIdentity, error)
	method      func(string) selfupdate.Method
	lock        func() (*selfupdate.InstallLock, error)
	replace     func(string, string) error
	now         func() time.Time
}

var maatRawSelfUpdateOps = maatRawSelfUpdateDependencies{
	storeStatus: maat.InspectDefaultStore,
	openStore:   func() (maatRawSelfUpdateStore, error) { return maat.OpenDefaultStore() },
	journal:     func() (maat.DecisionJournal, error) { return maat.NewDefaultDecisionJournal() },
	project:     maat.ProjectFailureMemoryPreflight,
	selfInfo:    currentMaatRunningBinaryInfo,
	probe:       selfupdate.CheckVersionProbe,
	schema:      observeBinaryDriftSchemaCompatibility,
	identity:    binaryDriftFileIdentity,
	method:      selfupdate.DetectMethod,
	lock:        selfupdate.AcquireInstallLock,
	replace:     selfupdate.SafeReplace,
	now:         time.Now,
}

func rawSelfUpdateManifest(selfPath, selfHash string, targets []selfupdate.DriftTarget, storeUUID string, info modversion.Info, liveSchema int, observedAt time.Time) (maat.RawSelfUpdateManifest, error) {
	if uint32(os.Getuid()) != uint32(os.Geteuid()) {
		return maat.RawSelfUpdateManifest{}, errors.New("Ma'at self-update requires matching real and effective user identities")
	}
	if len(targets) == 0 || len(targets) > 32 {
		return maat.RawSelfUpdateManifest{}, errors.New("Ma'at self-update requires a bounded nonempty exact target set")
	}
	self, err := maatRawSelfUpdateOps.identity(selfPath)
	if err != nil || self.SHA256 != selfHash {
		return maat.RawSelfUpdateManifest{}, errors.New("Ma'at self-update running binary identity changed after preview")
	}
	if info.Binary != "sirsi" || info.Path != selfPath || info.RouterSchemaMax <= 0 || info.Version == "dev" || info.Commit == "none" || info.Dirty {
		return maat.RawSelfUpdateManifest{}, errors.New("Ma'at self-update requires the complete current stamped version identity and schema ceiling")
	}
	current := maatRawSelfUpdateOps.selfInfo()
	current.Path = info.Path
	if current != info {
		return maat.RawSelfUpdateManifest{}, errors.New("version probe does not match the complete current runtime identity and schema ceiling")
	}
	if err := selfupdate.CheckSchemaCeiling(info, liveSchema); err != nil {
		return maat.RawSelfUpdateManifest{}, fmt.Errorf("candidate schema ceiling: %w", err)
	}
	ordered := append([]selfupdate.DriftTarget(nil), targets...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].Path < ordered[j].Path })
	manifest := maat.RawSelfUpdateManifest{
		Schema: maat.RawSelfUpdateManifestSchema, SelfPath: selfPath, SelfSHA256: self.SHA256,
		SelfOwnerUID: self.OwnerUID, SelfDevice: self.Device, SelfInode: self.Inode, SelfMode: self.Mode,
		RunningInfo: info, AuthorizedUID: uint32(os.Geteuid()), LiveRouterSchema: liveSchema,
		ObservedSchemaCompatible: true, StoreUUID: storeUUID, ObservedAtUTC: observedAt.UTC(),
	}
	for _, target := range ordered {
		method := maatRawSelfUpdateOps.method(target.Path)
		if method != selfupdate.MethodRaw {
			return maat.RawSelfUpdateManifest{}, fmt.Errorf("Ma'at self-update does not own %s target %s; use its owning updater", method, target.Path)
		}
		id, err := maatRawSelfUpdateOps.identity(target.Path)
		if err != nil || id.SHA256 != target.Present || target.Expected != selfHash {
			return maat.RawSelfUpdateManifest{}, fmt.Errorf("Ma'at self-update target changed after preview: %s", target.Path)
		}
		manifest.Targets = append(manifest.Targets, maat.RawSelfUpdateTarget{Path: target.Path, Method: method.String(), SHA256: id.SHA256,
			OwnerUID: id.OwnerUID, Device: id.Device, Inode: id.Inode, Mode: id.Mode})
	}
	binding, err := selfUpdatePreviewBinding(selfPath, selfHash, info, manifest.Targets)
	if err != nil {
		return maat.RawSelfUpdateManifest{}, err
	}
	manifest.PreviewBinding = binding
	if err := manifest.ValidateFresh(); err != nil {
		return maat.RawSelfUpdateManifest{}, err
	}
	return manifest, nil
}

func selfUpdatePreviewBinding(selfPath, selfHash string, info modversion.Info, targets []maat.RawSelfUpdateTarget) (string, error) {
	value := struct {
		SelfPath string                     `json:"self_path"`
		SelfHash string                     `json:"self_sha256"`
		Info     modversion.Info            `json:"running_info"`
		Targets  []maat.RawSelfUpdateTarget `json:"ordered_targets"`
	}{selfPath, selfHash, info, targets}
	b, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:]), nil
}

func revalidateRawSelfUpdateManifest(manifest maat.RawSelfUpdateManifest) error {
	if err := manifest.ValidateFresh(); err != nil {
		return err
	}
	self, err := maatRawSelfUpdateOps.identity(manifest.SelfPath)
	if err != nil || self.SHA256 != manifest.SelfSHA256 || self.OwnerUID != manifest.SelfOwnerUID || self.Device != manifest.SelfDevice || self.Inode != manifest.SelfInode || self.Mode != manifest.SelfMode {
		return errors.New("running binary identity changed after Ma'at self-update review")
	}
	info := maatRawSelfUpdateOps.selfInfo()
	info.Path = manifest.SelfPath
	if info != manifest.RunningInfo {
		return errors.New("complete running binary build identity or router-schema ceiling changed after review")
	}
	binding, err := selfUpdatePreviewBinding(manifest.SelfPath, manifest.SelfSHA256, manifest.RunningInfo, manifest.Targets)
	if err != nil || binding != manifest.PreviewBinding {
		return errors.New("raw self-update preview binding no longer matches its ordered manifest")
	}
	liveSchema, err := maatRawSelfUpdateOps.schema(manifest.RunningInfo)
	if err != nil || liveSchema != manifest.LiveRouterSchema {
		return errors.New("live router schema or compatibility observation changed after review")
	}
	for _, target := range manifest.Targets {
		if maatRawSelfUpdateOps.method(target.Path).String() != target.Method {
			return fmt.Errorf("target installation owner changed after review: %s", target.Path)
		}
		identity, err := maatRawSelfUpdateOps.identity(target.Path)
		if err != nil || identity.OwnerUID != target.OwnerUID || identity.Device != target.Device || identity.Inode != target.Inode || identity.Mode != target.Mode || identity.SHA256 != target.SHA256 {
			return fmt.Errorf("target identity changed after review: %s", target.Path)
		}
	}
	return nil
}

func applyRawSelfUpdate(manifest maat.RawSelfUpdateManifest) error {
	status := maatRawSelfUpdateOps.storeStatus()
	if status.State != maat.DefaultStoreReady || status.StoreUUID != manifest.StoreUUID {
		return errors.New("Ma'at self-update requires the existing initialized failure-memory store; no target was changed")
	}
	if err := manifest.ValidateFresh(); err != nil {
		return fmt.Errorf("Ma'at self-update reviewed plan expired before admission: %w", err)
	}
	store, err := maatRawSelfUpdateOps.openStore()
	if err != nil {
		return fmt.Errorf("Ma'at self-update: open existing failure-memory store: %w", err)
	}
	defer store.Close()
	if err := revalidateRawSelfUpdateManifest(manifest); err != nil {
		return fmt.Errorf("Ma'at self-update reviewed plan changed before preflight: %w", err)
	}
	if pending, pendingErr := store.PendingRawSelfUpdateAttempt(); pendingErr == nil {
		return fmt.Errorf("Ma'at self-update attempt %s is unresolved; inspect and reconcile retained state before another confirmation", pending.ID)
	} else if !errors.Is(pendingErr, maat.ErrNoIncompleteRawSelfUpdateAttempt) {
		return fmt.Errorf("Ma'at self-update attempt history is unverifiable; replacement refused: %w", pendingErr)
	}
	journal, err := maatRawSelfUpdateOps.journal()
	if err != nil {
		return fmt.Errorf("Ma'at self-update: open Casebook decision journal: %w", err)
	}
	receipt, preflightErr := store.PreflightRawSelfUpdate(manifest)
	if preflightErr != nil && (receipt.Decision != maat.PreflightUnverifiable || receipt.RegistrySnapshotSHA256 == "") {
		return fmt.Errorf("Ma'at self-update failure-memory preflight is unverifiable: %w", preflightErr)
	}
	stored, err := store.RetainPreflightReceipt(receipt)
	if err != nil {
		return fmt.Errorf("retain and read back complete Ma'at self-update preflight receipt before replacement: %w", err)
	}
	project := maatRawSelfUpdateOps.project
	if project == nil {
		project = maat.ProjectFailureMemoryPreflight
	}
	if err := project(journal, stored); err != nil {
		return fmt.Errorf("retain canonical Ma'at self-update preflight projection before replacement: %w", err)
	}
	if preflightErr != nil {
		return fmt.Errorf("Ma'at self-update failure-memory preflight is unverifiable; no target was changed: %w", preflightErr)
	}
	if receipt.Decision != maat.PreflightPass {
		return fmt.Errorf("Ma'at self-update failure-memory preflight %s; no target was changed", receipt.Decision)
	}
	lock, err := maatRawSelfUpdateOps.lock()
	if err != nil {
		return fmt.Errorf("Ma'at self-update serialization gate: %w", err)
	}
	if lock != nil {
		defer lock.Close()
	}
	attempt, err := store.BeginRawSelfUpdateAttempt(manifest, stored)
	if err != nil {
		return fmt.Errorf("Ma'at self-update durable intent was not admitted; no target was changed: %w", err)
	}
	if err := revalidateRawSelfUpdateManifest(manifest); err != nil {
		_, factErr := store.RecordRawSelfUpdateNotInvoked(attempt, "prelaunch_action_changed", boundedMaatRepairDetail(err.Error(), 4096))
		if factErr != nil {
			return fmt.Errorf("Ma'at self-update attempt %s remains retained after prelaunch refusal; execution is unresolved: validation=%v fact=%w", attempt.ID, err, factErr)
		}
		return fmt.Errorf("Ma'at self-update attempt %s was retained as not-invoked after exact action revalidation failed: %w", attempt.ID, err)
	}
	if err := store.RecordRawSelfUpdateInvocation(attempt); err != nil {
		if _, factErr := store.RecordRawSelfUpdateNotInvoked(attempt, "invocation_not_recorded", boundedMaatRepairDetail(err.Error(), 4096)); factErr != nil {
			return fmt.Errorf("Ma'at self-update attempt %s is retained; invocation boundary is uncertain and SafeReplace was not called: barrier=%v disposition=%w", attempt.ID, err, factErr)
		}
		return fmt.Errorf("Ma'at self-update invocation barrier failed; SafeReplace was not called: %w", err)
	}
	var observations []maatRawSelfUpdatePostcheck
	var failures []string
	for _, target := range manifest.Targets {
		callErr := maatRawSelfUpdateOps.replace(manifest.SelfPath, target.Path)
		observation, observeErr := observeRawSelfUpdateTarget(target)
		observations = append(observations, observation)
		if callErr != nil {
			failures = append(failures, target.Path+": "+boundedMaatRepairDetail(callErr.Error(), 512))
		}
		if observeErr != nil {
			failures = append(failures, target.Path+" postcheck: "+boundedMaatRepairDetail(observeErr.Error(), 512))
		}
	}
	postLiveSchema, schemaErr := maatRawSelfUpdateOps.schema(manifest.RunningInfo)
	schemaObservation := maatRawSelfUpdatePostcheck{Path: "router-schema", LiveRouterSchema: postLiveSchema,
		SchemaCompatible: schemaErr == nil && postLiveSchema == manifest.LiveRouterSchema, Verified: schemaErr == nil && postLiveSchema == manifest.LiveRouterSchema}
	if schemaErr != nil {
		schemaObservation.Error = boundedMaatRepairDetail(schemaErr.Error(), 512)
	}
	observations = append(observations, schemaObservation)
	if schemaErr != nil || postLiveSchema != manifest.LiveRouterSchema {
		failures = append(failures, "router-schema postcheck: live compatibility observation changed")
	}
	postcheckPassed := len(failures) == 0 && len(observations) == len(manifest.Targets)+1
	observationJSON, _ := json.Marshal(observations)
	observationHash := sha256.Sum256(observationJSON)
	detail := "all reviewed targets were replaced once and the exact fresh inode/hash postchecks passed"
	if !postcheckPassed {
		detail = "one or more exact target replacement or postcheck steps failed: " + boundedMaatRepairDetail(fmt.Sprint(failures), 3072)
	}
	outcome, err := store.RecordRawSelfUpdateOutcome(attempt, postcheckPassed, hex.EncodeToString(observationHash[:]), detail)
	if err != nil {
		return fmt.Errorf("Ma'at self-update attempt %s remains unresolved after the replacement sequence; do not replay: %w", attempt.ID, err)
	}
	if err := reconcileRawSelfUpdateProjections(journal, store); err != nil {
		return fmt.Errorf("Ma'at self-update terminal outcome is retained with projection debt; reconcile without replacement: %w", err)
	}
	if !postcheckPassed {
		return fmt.Errorf("Ma'at self-update did not verify every reviewed target; attempt %s is retained", attempt.ID)
	}
	_ = outcome
	return nil
}

type maatRawSelfUpdatePostcheck struct {
	Path             string `json:"path"`
	PreviousSHA256   string `json:"previous_sha256"`
	InstalledSHA256  string `json:"installed_sha256,omitempty"`
	InstalledDevice  uint64 `json:"installed_device,omitempty"`
	InstalledInode   uint64 `json:"installed_inode,omitempty"`
	InstalledOwner   uint32 `json:"installed_owner_uid,omitempty"`
	InstalledMode    uint32 `json:"installed_mode,omitempty"`
	FreshInode       bool   `json:"fresh_inode"`
	ContentChanged   bool   `json:"content_changed"`
	LiveRouterSchema int    `json:"live_router_schema,omitempty"`
	SchemaCompatible bool   `json:"schema_compatible"`
	Verified         bool   `json:"verified"`
	Error            string `json:"error,omitempty"`
}

func observeRawSelfUpdateTarget(target maat.RawSelfUpdateTarget) (maatRawSelfUpdatePostcheck, error) {
	result := maatRawSelfUpdatePostcheck{Path: target.Path, PreviousSHA256: target.SHA256}
	identity, err := maatRawSelfUpdateOps.identity(target.Path)
	if err != nil {
		result.Error = boundedMaatRepairDetail(err.Error(), 512)
		return result, err
	}
	result.InstalledSHA256, result.InstalledDevice, result.InstalledInode = identity.SHA256, identity.Device, identity.Inode
	result.InstalledOwner, result.InstalledMode = identity.OwnerUID, identity.Mode
	result.FreshInode = identity.Device != target.Device || identity.Inode != target.Inode
	result.ContentChanged = identity.SHA256 != target.SHA256
	ownerMatches := identity.OwnerUID == target.OwnerUID || (target.OwnerUID == 0 && identity.OwnerUID == uint32(os.Geteuid()))
	result.Verified = result.FreshInode && result.ContentChanged && ownerMatches && identity.Mode&uint32(unix.S_IFMT) == uint32(unix.S_IFREG) &&
		identity.Mode&0o111 != 0 && identity.Mode&0o022 == 0 && identity.Mode&uint32(unix.S_ISUID|unix.S_ISGID) == 0
	if !result.Verified {
		err = errors.New("installed target did not converge to a fresh owned regular-file identity")
		result.Error = err.Error()
		return result, err
	}
	return result, nil
}

func reconcileRawSelfUpdateProjections(journal maat.DecisionJournal, store maatRawSelfUpdateStore) error {
	entries, err := store.RawSelfUpdateReconciliations()
	if err != nil {
		return err
	}
	idempotent, ok := journal.(interface{ AppendIdempotent(maat.Decision) error })
	if !ok {
		return errors.New("Ma'at self-update requires idempotent journal append for projection recovery")
	}
	reader, ok := journal.(interface {
		FindByEvidence(string, string) (maat.Decision, bool, error)
	})
	if !ok {
		return errors.New("Ma'at self-update requires exact journal readback for projection recovery")
	}
	for _, entry := range entries {
		if entry.Outcome == nil || entry.ProjectionReadBack {
			continue
		}
		decision := entry.Outcome.Decision
		if err := idempotent.AppendIdempotent(decision); err != nil {
			return fmt.Errorf("append retained raw self-update terminal projection: %w", err)
		}
		readback, found, err := reader.FindByEvidence(decision.Kind, decision.Evidence)
		if err != nil {
			return fmt.Errorf("read back retained raw self-update terminal projection: %w", err)
		}
		if !found {
			return errors.New("raw self-update terminal projection readback is missing")
		}
		expectedBytes, _ := json.Marshal(decision)
		actualBytes, _ := json.Marshal(readback)
		if string(expectedBytes) != string(actualBytes) {
			return errors.New("raw self-update terminal projection readback differs")
		}
		if err := store.MarkRawSelfUpdateProjectionReadBack(entry.Attempt, *entry.Outcome, readback); err != nil {
			return fmt.Errorf("retain exact raw self-update projection readback: %w", err)
		}
	}
	return nil
}

func reconcilePendingRawSelfUpdate() error {
	status := maatRawSelfUpdateOps.storeStatus()
	if status.State != maat.DefaultStoreReady {
		return errors.New("Ma'at self-update reconciliation requires the existing initialized failure-memory store")
	}
	store, err := maatRawSelfUpdateOps.openStore()
	if err != nil {
		return err
	}
	defer store.Close()
	lock, err := maatRawSelfUpdateOps.lock()
	if err != nil {
		return fmt.Errorf("Ma'at self-update reconciliation serialization gate: %w", err)
	}
	if lock != nil {
		defer lock.Close()
	}
	entries, err := store.RawSelfUpdateReconciliations()
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.Outcome != nil || entry.InvocationRecorded {
			continue
		}
		if _, err := store.RecordRawSelfUpdateNotInvoked(entry.Attempt, "reconcile_no_invocation_marker",
			"the serialized raw self-update protocol has no durable invocation marker; SafeReplace was not entered"); err != nil {
			return fmt.Errorf("attempt %s could not retain the exact pre-invocation recovery fact: %w", entry.Attempt.ID, err)
		}
	}
	journal, err := maatRawSelfUpdateOps.journal()
	if err != nil {
		return err
	}
	if err := reconcileRawSelfUpdateProjections(journal, store); err != nil {
		return err
	}
	entries, err := store.RawSelfUpdateReconciliations()
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.Outcome == nil {
			return fmt.Errorf("attempt %s remains unresolved; invocation state is unknown and replacement will not be replayed", entry.Attempt.ID)
		}
		if !entry.ProjectionReadBack {
			return fmt.Errorf("attempt %s still has terminal projection debt; no replacement was run", entry.Attempt.ID)
		}
	}
	fmt.Println("Ma'at raw self-update history read back; no replacement was run.")
	return nil
}
