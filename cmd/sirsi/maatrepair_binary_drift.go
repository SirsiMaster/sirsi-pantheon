package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"
	"unicode/utf8"

	"github.com/SirsiMaster/sirsi-pantheon/internal/guard"
	"github.com/SirsiMaster/sirsi-pantheon/internal/maat"
	"github.com/SirsiMaster/sirsi-pantheon/internal/output"
	"github.com/SirsiMaster/sirsi-pantheon/internal/routerstore"
	"github.com/SirsiMaster/sirsi-pantheon/internal/selfupdate"
	modversion "github.com/SirsiMaster/sirsi-pantheon/internal/version"
	"github.com/spf13/cobra"
	"golang.org/x/sys/unix"
)

const maatRepairBinaryDriftCheck = "binary-drift"

var errUpdaterExecutionUnknown = errors.New("supported updater terminal state is unknown")
var updaterDirectWait = 2 * time.Second

var (
	maatRepairBinaryDriftCaseEvidence string
	maatRepairBinaryDriftBinding      string
	maatRepairBinaryDriftInspect      bool
	maatRepairBinaryDriftReconcile    bool
	maatRepairScanDrift               = selfupdate.ScanHost
	maatRepairScanDriftContext        = selfupdate.ScanHostContext
	maatRepairReadBinaryFinding       = readMaatBinaryDriftFinding
	maatRepairHashFile                = selfupdate.FileHash
	maatRepairDetectMethod            = selfupdate.DetectMethod
	maatRepairFindHomebrew            = func() (string, error) { return exec.LookPath("brew") }
	maatRepairRunHomebrewUpdate       = runSirsiHomebrewUpgradePathContext
	maatRepairOpenBinaryDriftStore    = func() (maatBinaryDriftStore, error) { return maat.OpenDefaultStore() }
	maatRepairBinarySchemaObservation = observeBinaryDriftSchemaCompatibility
	maatRepairRunningBinaryInfo       = currentMaatRunningBinaryInfo
)

type maatBinaryDriftStore interface {
	Close() error
	PreflightBinaryUpdate(maat.BinaryUpdateManifest) (maat.PreflightReceipt, error)
	RetainPreflightReceipt(maat.PreflightReceipt) (maat.StoredPreflightReceipt, error)
	BeginBinaryUpdateAttempt(maat.BinaryUpdateManifest, maat.PreflightReceipt, func() (string, error)) (maat.BinaryUpdateAttempt, error)
	PendingBinaryUpdateAttempt() (maat.BinaryUpdateAttempt, error)
	PendingBinaryUpdateProjectionDebts() ([]maat.BinaryUpdateProjectionDebt, error)
	RecordBinaryUpdateOutcome(maat.BinaryUpdateAttempt, bool, bool, string, string) (maat.BinaryUpdateOutcome, error)
	RecordBinaryUpdateInvocation(maat.BinaryUpdateAttempt) error
	RecordBinaryUpdateNotInvoked(maat.BinaryUpdateAttempt, string, string) (maat.BinaryUpdateOutcome, error)
	BinaryUpdateReconciliations(string) ([]maat.BinaryUpdateReconciliation, error)
	BeginBinaryUpdateProjection(maat.BinaryUpdateAttempt, maat.BinaryUpdateOutcome, maat.Decision) (maat.BinaryUpdateProjectionDebt, error)
	PersistBinaryUpdateProjectionOutcome(maat.BinaryUpdateProjectionDebt) error
	MarkBinaryUpdateProjectionReadBack(maat.BinaryUpdateProjectionDebt) error
}

type maatBinaryFileIdentity struct {
	OwnerUID      uint32
	Device, Inode uint64
	Mode          uint32
	SHA256        string
}

func binaryDriftFileIdentity(path string) (maatBinaryFileIdentity, error) {
	f, err := os.Open(path)
	if err != nil {
		return maatBinaryFileIdentity{}, err
	}
	defer f.Close()
	stat, err := f.Stat()
	if err != nil {
		return maatBinaryFileIdentity{}, err
	}
	if !stat.Mode().IsRegular() {
		return maatBinaryFileIdentity{}, errors.New("binary update input is not a regular file")
	}
	var id unix.Stat_t
	if err := unix.Fstat(int(f.Fd()), &id); err != nil {
		return maatBinaryFileIdentity{}, err
	}
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return maatBinaryFileIdentity{}, err
	}
	return maatBinaryFileIdentity{OwnerUID: id.Uid, Device: uint64(id.Dev), Inode: id.Ino, Mode: uint32(id.Mode), SHA256: hex.EncodeToString(h.Sum(nil))}, nil
}

func observeBinaryDriftSchemaCompatibility(info modversion.Info) (int, error) {
	dbPath, err := resolveRouterDBPath()
	if err != nil {
		return 0, err
	}
	version, err := resolveLiveSchema(dbPath)
	if err != nil {
		return 0, err
	}
	if err := selfupdate.CheckSchemaCeiling(info, version); err != nil {
		return version, err
	}
	return version, nil
}

func currentMaatRunningBinaryInfo() modversion.Info {
	info := modversion.Current("sirsi")
	info.RouterSchemaMax = routerstore.MaxSupportedSchemaVersion()
	return info
}

func buildMaatBinaryUpdateManifest(preview maatBinaryDriftPreview, storeUUID string) (maat.BinaryUpdateManifest, error) {
	if uint32(os.Getuid()) != uint32(os.Geteuid()) {
		return maat.BinaryUpdateManifest{}, errors.New("Ma'at binary-drift update requires matching real and effective user identities")
	}
	if storeUUID == "" {
		return maat.BinaryUpdateManifest{}, errors.New("Ma'at binary-drift update requires the initialized default failure-memory store")
	}
	self, err := binaryDriftFileIdentity(preview.SelfPath)
	if err != nil {
		return maat.BinaryUpdateManifest{}, fmt.Errorf("bind running binary identity: %w", err)
	}
	if self.SHA256 != preview.SelfSHA256 {
		return maat.BinaryUpdateManifest{}, errors.New("running binary changed after preview")
	}
	runningInfo := maatRepairRunningBinaryInfo()
	if runningInfo.Binary != "sirsi" || runningInfo.Path != preview.SelfPath || runningInfo.Version != preview.SelfVersion || runningInfo.RouterSchemaMax <= 0 {
		return maat.BinaryUpdateManifest{}, errors.New("complete running binary identity or supported router-schema ceiling differs from the reviewed preview")
	}
	updater, err := binaryDriftFileIdentity(preview.UpdaterPath)
	if err != nil || updater.SHA256 != preview.UpdaterSHA256 {
		return maat.BinaryUpdateManifest{}, errors.New("resolved Homebrew updater identity changed after preview")
	}
	schemaVersion, err := maatRepairBinarySchemaObservation(runningInfo)
	if err != nil {
		return maat.BinaryUpdateManifest{}, fmt.Errorf("observe binary-update schema compatibility: %w", err)
	}
	m := maat.BinaryUpdateManifest{Schema: maat.BinaryUpdateManifestSchema, CaseEvidence: preview.CaseEvidence, PreviewBinding: preview.Binding, Updater: preview.Updater,
		UpdaterPath: preview.UpdaterPath, UpdaterSHA256: preview.UpdaterSHA256, UpdaterOwnerUID: updater.OwnerUID, UpdaterDevice: updater.Device, UpdaterInode: updater.Inode, UpdaterMode: updater.Mode,
		SelfPath: preview.SelfPath, SelfVersion: preview.SelfVersion, SelfSHA256: self.SHA256, RunningInfo: runningInfo,
		SelfOwnerUID: self.OwnerUID, SelfDevice: self.Device, SelfInode: self.Inode, SelfMode: self.Mode, AuthorizedUID: uint32(os.Geteuid()),
		ObservedSchemaCompatible: true, ObservedSchemaVersion: schemaVersion, StoreUUID: storeUUID, ObservedAtUTC: time.Now().UTC()}
	for _, target := range preview.Targets {
		id, err := binaryDriftFileIdentity(target.Path)
		if err != nil {
			return maat.BinaryUpdateManifest{}, fmt.Errorf("bind reviewed binary target: %w", err)
		}
		if id.SHA256 != target.SHA256 {
			return maat.BinaryUpdateManifest{}, fmt.Errorf("reviewed binary target changed: %s", target.Path)
		}
		m.Targets = append(m.Targets, maat.BinaryUpdateTarget{Path: target.Path, Method: target.Method, Version: target.Version, SHA256: id.SHA256, OwnerUID: id.OwnerUID, Device: id.Device, Inode: id.Inode, Mode: id.Mode})
	}
	return m, nil
}

func revalidateMaatBinaryUpdateManifest(m maat.BinaryUpdateManifest) error {
	if err := m.ValidateFresh(); err != nil {
		return fmt.Errorf("binary-update review is no longer fresh: %w", err)
	}
	self, err := binaryDriftFileIdentity(m.SelfPath)
	if err != nil || self.Device != m.SelfDevice || self.Inode != m.SelfInode || self.OwnerUID != m.SelfOwnerUID || self.Mode != m.SelfMode || self.SHA256 != m.SelfSHA256 {
		return errors.New("running binary identity changed after review")
	}
	runningInfo := maatRepairRunningBinaryInfo()
	if runningInfo != m.RunningInfo {
		return errors.New("complete running binary build identity or schema ceiling changed after review")
	}
	updater, err := binaryDriftFileIdentity(m.UpdaterPath)
	if err != nil || updater.Device != m.UpdaterDevice || updater.Inode != m.UpdaterInode || updater.OwnerUID != m.UpdaterOwnerUID || updater.Mode != m.UpdaterMode || updater.SHA256 != m.UpdaterSHA256 {
		return errors.New("Homebrew updater identity changed after review")
	}
	for _, target := range m.Targets {
		id, err := binaryDriftFileIdentity(target.Path)
		if err != nil || id.Device != target.Device || id.Inode != target.Inode || id.OwnerUID != target.OwnerUID || id.Mode != target.Mode || id.SHA256 != target.SHA256 {
			return fmt.Errorf("reviewed binary target identity changed: %s", target.Path)
		}
	}
	version, err := maatRepairBinarySchemaObservation(runningInfo)
	if err != nil || version != m.ObservedSchemaVersion {
		return errors.New("binary-update schema compatibility observation changed after review")
	}
	return nil
}

type maatBinaryDriftTarget struct {
	Path    string `json:"path"`
	Method  string `json:"method"`
	Version string `json:"version,omitempty"`
	SHA256  string `json:"sha256"`
}

type maatBinaryDriftPreview struct {
	CaseEvidence  string                  `json:"case_evidence"`
	Updater       string                  `json:"updater"`
	UpdaterPath   string                  `json:"updater_path"`
	UpdaterSHA256 string                  `json:"updater_sha256"`
	SelfPath      string                  `json:"self_path"`
	SelfVersion   string                  `json:"self_version"`
	SelfSHA256    string                  `json:"self_sha256"`
	Targets       []maatBinaryDriftTarget `json:"targets"`
	Binding       string                  `json:"binding"`
}

type maatBinaryDriftBinding struct {
	CaseEvidence  string                  `json:"case_evidence"`
	Updater       string                  `json:"updater"`
	UpdaterPath   string                  `json:"updater_path"`
	UpdaterSHA256 string                  `json:"updater_sha256"`
	SelfPath      string                  `json:"self_path"`
	SelfVersion   string                  `json:"self_version"`
	SelfSHA256    string                  `json:"self_sha256"`
	Targets       []maatBinaryDriftTarget `json:"targets"`
}

var maatRepairBinaryDriftCmd = &cobra.Command{
	Use:   "binary-drift",
	Short: "Preview or apply the supported Homebrew binary-drift update",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		if maatRepairBinaryDriftReconcile {
			journal, err := newMaatDecisionJournal()
			if err != nil {
				return fmt.Errorf("Ma'at binary-drift reconciliation: open Casebook: %w", err)
			}
			store, err := maatRepairOpenBinaryDriftStore()
			if err != nil {
				return fmt.Errorf("Ma'at binary-drift reconciliation: open existing failure-memory store: %w", err)
			}
			defer store.Close()
			return reconcileMaatBinaryDrift(journal, store)
		}
		if maatRepairBinaryDriftInspect {
			if maatRepairBinaryDriftCaseEvidence == "" {
				return errors.New("Ma'at binary-drift attempt inspection requires the exact System One case evidence")
			}
			journal, err := newMaatDecisionJournal()
			if err != nil {
				return fmt.Errorf("Ma'at binary-drift attempt inspection: open Casebook: %w", err)
			}
			status, err := inspectMaatBinaryDriftAttempt(journal, maatRepairBinaryDriftCaseEvidence)
			if err != nil {
				return err
			}
			if JsonOutput || maatJSON {
				return emitJSON(status)
			}
			fmt.Printf("Unresolved updater attempt %s for %s\n", status.AttemptEvidence, status.CaseEvidence)
			fmt.Printf("Fresh binary observation: %s\n", status.Observation)
			if status.ObservationError != "" {
				fmt.Printf("Observation unavailable: %s\n", status.ObservationError)
			}
			if status.MemoryReceipt != nil {
				fmt.Printf("Canonical failure-memory receipt: %s\n", status.MemoryReceipt.EvidenceReference())
			}
			fmt.Println("This read-only inspection does not prove the prior updater stopped or authorize replay. Review the retained attempt and take a fresh System One screen before any new consent.")
			return nil
		}
		if maatRepairBinaryDriftCaseEvidence == "" {
			return errors.New("Ma'at binary-drift recovery requires the exact open System One case evidence")
		}
		journal, err := newMaatDecisionJournal()
		if err != nil {
			return fmt.Errorf("Ma'at binary-drift recovery: open Casebook: %w", err)
		}
		if err = validateMaatBinaryDriftCase(journal, maatRepairBinaryDriftCaseEvidence); err != nil {
			return err
		}
		preview, err := currentMaatBinaryDriftPreview(maatRepairBinaryDriftCaseEvidence)
		if err != nil {
			return err
		}
		if !maatRepairConfirm {
			if JsonOutput || maatJSON {
				return emitJSON(preview)
			}
			fmt.Printf("Ma'at updater preview: %s\n", preview.Updater)
			fmt.Printf("  executable: %s sha256:%s\n", preview.UpdaterPath, preview.UpdaterSHA256)
			fmt.Printf("  running Sirsi: %s · %s · sha256:%s\n", preview.SelfPath, preview.SelfVersion, preview.SelfSHA256)
			for _, target := range preview.Targets {
				fmt.Printf("  target: %s · %s · %s · sha256:%s\n", target.Path, target.Method, target.Version, target.SHA256)
			}
			fmt.Printf("Preview binding: %s\nNo files were changed. Review this exact plan, then confirm it in Ma'at.\n", preview.Binding)
			return nil
		}
		if !isSHA256Hex(maatRepairBinaryDriftBinding) {
			return errors.New("Ma'at binary-drift recovery requires the binding from a fresh read-only preview")
		}
		if preview.Binding != maatRepairBinaryDriftBinding {
			return errors.New("Ma'at binary-drift recovery preview changed; no updater was run. Refresh the case and review a new preview")
		}
		storeStatus := maatRepairDefaultStoreInspect()
		if storeStatus.State != maat.DefaultStoreReady {
			return errors.New("Ma'at binary-drift update requires the existing initialized failure-memory store; no updater was run")
		}
		store, err := maatRepairOpenBinaryDriftStore()
		if err != nil {
			return fmt.Errorf("Ma'at binary-drift update: open existing failure-memory store: %w", err)
		}
		defer store.Close()
		if pending, pendingErr := store.PendingBinaryUpdateAttempt(); pendingErr == nil {
			return fmt.Errorf("Ma'at binary-drift update attempt %s is unresolved; inspect and reconcile it before any new consent", pending.ID)
		} else if !errors.Is(pendingErr, maat.ErrNoIncompleteBinaryUpdateAttempt) {
			return fmt.Errorf("Ma'at binary-drift attempt history is unverifiable: %w", pendingErr)
		}
		if debts, err := store.PendingBinaryUpdateProjectionDebts(); err != nil || len(debts) != 0 {
			return errors.New("Ma'at binary-drift terminal projection debt must be reconciled before new consent")
		}
		manifest, err := buildMaatBinaryUpdateManifest(preview, storeStatus.StoreUUID)
		if err != nil {
			return fmt.Errorf("Ma'at binary-drift update manifest: %w", err)
		}
		receipt, preflightErr := store.PreflightBinaryUpdate(manifest)
		if preflightErr != nil && (receipt.Decision != maat.PreflightUnverifiable || receipt.RegistrySnapshotSHA256 == "") {
			return fmt.Errorf("Ma'at binary-drift failure-memory preflight is unverifiable: %w", preflightErr)
		}
		stored, err := store.RetainPreflightReceipt(receipt)
		if err != nil {
			return fmt.Errorf("retain complete Ma'at binary-drift preflight receipt: %w", err)
		}
		if err := maat.ProjectFailureMemoryPreflight(journal, stored); err != nil {
			return fmt.Errorf("retain Ma'at binary-drift preflight projection before action: %w", err)
		}
		if preflightErr != nil {
			return fmt.Errorf("Ma'at binary-drift failure-memory preflight is unverifiable; updater refused: %w", preflightErr)
		}
		if receipt.Decision != maat.PreflightPass {
			return fmt.Errorf("Ma'at binary-drift failure-memory preflight %s; updater refused", receipt.Decision)
		}
		if err := revalidateMaatBinaryUpdateManifest(manifest); err != nil {
			return fmt.Errorf("Ma'at binary-drift action changed before admission: %w", err)
		}
		actionDigest, err := manifest.Digest()
		if err != nil {
			return fmt.Errorf("Ma'at binary-drift action digest: %w", err)
		}
		attempt, err := store.BeginBinaryUpdateAttempt(manifest, receipt, func() (string, error) {
			if err := recordMaatBinaryDriftAttemptBound(journal, preview, stored.Reference(), actionDigest); err != nil {
				return "", err
			}
			return maatBinaryDriftAttemptEvidence(preview.CaseEvidence), nil
		})
		if err != nil {
			if attempt.ID != "" {
				if reconcileErr := reconcileMaatBinaryDrift(journal, store); reconcileErr != nil {
					return fmt.Errorf("binary-drift claim did not invoke updater; retained fact/projection needs reconciliation: claim=%v reconciliation=%w", err, reconcileErr)
				}
			}
			return err
		}
		if err := revalidateMaatBinaryUpdateManifest(manifest); err != nil {
			outcome, factErr := store.RecordBinaryUpdateNotInvoked(attempt, "prelaunch_revalidation_failed", boundedMaatRepairDetail(err.Error(), 4096))
			if factErr != nil {
				return fmt.Errorf("Ma'at binary-drift attempt %s is durable but action identity changed before updater launch; execution remains unresolved: validation=%v fact=%w", attempt.ID, err, factErr)
			}
			preview := maatBinaryDriftPreviewFromManifest(manifest)
			if _, projectErr := retainMaatBinaryDriftOutcomeProjection(journal, store, attempt, outcome, preview, "failed", "updater was not invoked: "+outcome.Detail, outcome.Detail); projectErr != nil {
				return fmt.Errorf("Ma'at binary-drift prelaunch refusal was retained; projection reconciliation required: validation=%v projection=%w", err, projectErr)
			}
			return fmt.Errorf("Ma'at binary-drift attempt %s was not launched because its action identity changed: %w", attempt.ID, err)
		}
		if err := store.RecordBinaryUpdateInvocation(attempt); err != nil {
			if outcome, factErr := store.RecordBinaryUpdateNotInvoked(attempt, "invocation_record_failed", boundedMaatRepairDetail(err.Error(), 4096)); factErr == nil {
				if _, projectErr := retainMaatBinaryDriftOutcomeProjection(journal, store, attempt, outcome, preview, "failed", "updater was not invoked: "+outcome.Detail, outcome.Detail); projectErr != nil {
					return fmt.Errorf("binary-drift invocation barrier failed before updater call; projection reconciliation required: barrier=%v projection=%w", err, projectErr)
				}
				return fmt.Errorf("binary-drift invocation barrier failed; updater was not called: %w", err)
			}
			return fmt.Errorf("binary-drift invocation boundary is uncertain; updater was not called by this process; do not replay: %w", err)
		}

		ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		fmt.Fprintln(cmd.ErrOrStderr(), "Ma'at is applying the reviewed Homebrew update (maximum 2 minutes); it will verify a fresh diagnostic before recording an outcome.")
		updaterErr := maatRepairRunHomebrewUpdate(ctx, preview.UpdaterPath)
		if errors.Is(updaterErr, errUpdaterExecutionUnknown) {
			return fmt.Errorf("Ma'at binary-drift recovery remains unresolved; its durable launch claim is retained and updater replay is refused: %w", updaterErr)
		}
		// Verification is a fresh factual read, independent of the updater's
		// canceled context, but has its own strict bound so cancellation cannot
		// skip or hang the terminal receipt.
		verifyCtx, verifyCancel := context.WithTimeout(context.Background(), maatBinaryDriftObservationTimeout)
		afterFinding, findingErr := maatRepairReadBinaryFinding(verifyCtx)
		afterDrift, driftErr := maatRepairScanDriftContext(verifyCtx)
		verifyCancel()
		cleared := updaterErr == nil && findingErr == nil && maatBinaryDriftFindingClear(afterFinding) && driftErr == nil && maatBinaryDriftTargetsObserved(preview.Targets, afterDrift) && afterDrift.Healthy
		detail := "the fresh diagnostic and binary inventory are clear after the updater returned successfully"
		if updaterErr != nil {
			detail = "Homebrew updater returned an error: " + updaterErr.Error()
			if findingErr == nil && driftErr == nil && afterDrift != nil && afterDrift.Healthy && maatBinaryDriftFindingClear(afterFinding) {
				detail += "; fresh state is clear, but Ma'at does not attribute that change to the failed updater"
			}
		} else if findingErr != nil {
			detail = "post-update binary-drift observation unavailable: " + findingErr.Error()
		} else if driftErr != nil {
			detail = "post-update binary inventory unavailable: " + driftErr.Error()
		} else if !cleared {
			detail = "the fresh diagnostic still reports binary drift, a reviewed target was not completely observed, or the inventory is not healthy"
		}
		obs, _ := json.Marshal(struct {
			Finding        guard.DiagnosticFinding `json:"finding"`
			FindingError   string                  `json:"finding_error,omitempty"`
			Inventory      *selfupdate.DriftReport `json:"inventory,omitempty"`
			InventoryError string                  `json:"inventory_error,omitempty"`
		}{afterFinding, errorString(findingErr), afterDrift, errorString(driftErr)})
		observationHash := sha256.Sum256(obs)
		outcome, outcomeErr := store.RecordBinaryUpdateOutcome(attempt, true, cleared, hex.EncodeToString(observationHash[:]), detail)
		if outcomeErr != nil {
			return fmt.Errorf("binary-drift updater returned but terminal outcome is not durable; do not replay: %w", outcomeErr)
		}
		return recordMaatBinaryDriftMemoryOutcome(journal, store, attempt, outcome, preview, cleared, detail, updaterErr)
	},
}

func errorString(err error) string {
	if err == nil {
		return ""
	}
	return boundedMaatRepairDetail(err.Error(), 1024)
}

func recordMaatBinaryDriftMemoryOutcome(journal maat.DecisionJournal, store maatBinaryDriftStore, attempt maat.BinaryUpdateAttempt, outcome maat.BinaryUpdateOutcome, preview maatBinaryDriftPreview, resolved bool, detail string, updaterErr error) error {
	determination := "failed"
	if resolved && updaterErr == nil {
		determination = "resolved"
	}
	after := "post-update observation: " + detail
	if determination == "resolved" {
		after = "binary-drift diagnostic and local inventory are clear"
	}
	debt, err := retainMaatBinaryDriftOutcomeProjection(journal, store, attempt, outcome, preview, determination, after, detail)
	if err != nil {
		return fmt.Errorf("binary-drift result remains retained for projection reconciliation: %w", err)
	}
	summary := "The supported Homebrew update and fresh diagnostic result were retained."
	if determination != "resolved" {
		summary = "The Homebrew update or fresh diagnostic did not verify; Ma'at retained an incomplete outcome."
	}
	result := &output.CommandResult{Command: "sirsi maat repair binary-drift", BriefTitle: "Ma'at binary-drift recovery", Status: determination, Summary: summary,
		Evidence:    []output.Evidence{{Label: "Original case", Value: preview.CaseEvidence}, {Label: "Updater binding", Value: preview.Binding}, {Label: "Fresh result", Value: after}, {Label: "Ma'at receipt", Value: debt.Decision.Evidence}},
		NextActions: []output.NextAction{{Label: "Open Ma'at Casebook", Command: "sirsi maat casebook", Description: "Inspect the original observation and the separate retained repair outcome."}}}
	result.Render()
	if determination != "resolved" {
		if updaterErr != nil {
			return fmt.Errorf("Ma'at binary-drift update did not verify: %w", updaterErr)
		}
		return errors.New("Ma'at binary-drift update did not clear the fresh diagnostic; the original case remains open")
	}
	return nil
}

func retainMaatBinaryDriftOutcomeProjection(journal maat.DecisionJournal, store maatBinaryDriftStore, attempt maat.BinaryUpdateAttempt, outcome maat.BinaryUpdateOutcome, preview maatBinaryDriftPreview, determination, after, detail string) (maat.BinaryUpdateProjectionDebt, error) {
	decision, err := buildMaatBinaryDriftDecision(attempt, outcome, preview, determination, after, detail)
	if err != nil {
		return maat.BinaryUpdateProjectionDebt{}, fmt.Errorf("build terminal projection: %w", err)
	}
	debt, err := store.BeginBinaryUpdateProjection(attempt, outcome, decision)
	if err != nil {
		return maat.BinaryUpdateProjectionDebt{}, fmt.Errorf("retain projection intent: %w", err)
	}
	if err := completeMaatBinaryDriftProjection(journal, store, debt); err != nil {
		return debt, fmt.Errorf("complete projection: %w", err)
	}
	return debt, nil
}

func buildMaatBinaryDriftDecision(attempt maat.BinaryUpdateAttempt, outcome maat.BinaryUpdateOutcome, preview maatBinaryDriftPreview, determination, after, detail string) (maat.Decision, error) {
	plan, _ := json.Marshal(preview)
	repair := maat.DiagnosticRepair{Check: maatRepairBinaryDriftCheck, Operation: preview.Updater + "; confirmed preview sha256=" + preview.Binding,
		OriginEvidence: preview.CaseEvidence, Before: preview.Binding, After: after, Determination: determination,
		Detail: "reviewed preview=" + string(plan) + "\npost-update result=" + detail + "\nretained attempt=" + attempt.ID + "; memory outcome=" + outcome.Determination}
	decision, err := maat.BuildDiagnosticRepairDecision("sirsi maat repair binary-drift", repair)
	if err != nil {
		return maat.Decision{}, err
	}
	decision.Time = outcome.RecordedAtUTC.UTC().Format(time.RFC3339Nano)
	return decision, nil
}

func completeMaatBinaryDriftProjection(journal maat.DecisionJournal, store maatBinaryDriftStore, debt maat.BinaryUpdateProjectionDebt) error {
	if err := store.PersistBinaryUpdateProjectionOutcome(debt); err != nil {
		return fmt.Errorf("persist terminal outcome projection: %w", err)
	}
	idempotent, ok := journal.(interface{ AppendIdempotent(maat.Decision) error })
	if !ok {
		return errors.New("journal idempotence is unavailable")
	}
	if err := idempotent.AppendIdempotent(debt.Decision); err != nil {
		return fmt.Errorf("append terminal outcome projection: %w", err)
	}
	reader, ok := journal.(interface {
		FindByEvidence(string, string) (maat.Decision, bool, error)
	})
	if !ok {
		return errors.New("exact journal readback is unavailable")
	}
	readback, found, err := reader.FindByEvidence(debt.Decision.Kind, debt.Decision.Evidence)
	if err != nil {
		return fmt.Errorf("read back terminal outcome projection: %w", err)
	}
	if !found {
		return errors.New("terminal projection readback is missing")
	}
	expectedBytes, _ := json.Marshal(debt.Decision)
	actualBytes, _ := json.Marshal(readback)
	if string(expectedBytes) != string(actualBytes) {
		return errors.New("terminal projection readback differs")
	}
	return store.MarkBinaryUpdateProjectionReadBack(debt)
}

func maatBinaryDriftPreviewFromManifest(manifest maat.BinaryUpdateManifest) maatBinaryDriftPreview {
	preview := maatBinaryDriftPreview{CaseEvidence: manifest.CaseEvidence, Updater: manifest.Updater, UpdaterPath: manifest.UpdaterPath, UpdaterSHA256: manifest.UpdaterSHA256,
		SelfPath: manifest.SelfPath, SelfVersion: manifest.SelfVersion, SelfSHA256: manifest.SelfSHA256, Binding: manifest.PreviewBinding}
	for _, target := range manifest.Targets {
		preview.Targets = append(preview.Targets, maatBinaryDriftTarget{Path: target.Path, Method: target.Method, Version: target.Version, SHA256: target.SHA256})
	}
	return preview
}

// reconcileMaatBinaryDrift completes only persisted data/projections. A new
// invocation protocol marks the ordering guarantee that the updater call is
// strictly after the invocation fact; older attempts remain unresolved.
func reconcileMaatBinaryDrift(journal maat.DecisionJournal, store maatBinaryDriftStore) error {
	history, err := store.BinaryUpdateReconciliations("")
	if err != nil {
		return fmt.Errorf("read retained binary-update facts: %w", err)
	}
	var unresolved []string
	for _, entry := range history {
		attempt := entry.Attempt
		if entry.Outcome == nil && !entry.InvocationRecorded && attempt.InvocationProtocol == maat.BinaryUpdateInvocationProtocol {
			// This is the typed pre-invocation generation. Both claim append and
			// updater invocation are ordered after the durable attempt; the updater
			// cannot have been reached before its independently read-back marker.
			if _, err := store.RecordBinaryUpdateNotInvoked(attempt, "claim_not_invoked_before_barrier", "reconciled a typed attempt whose updater invocation barrier was never persisted"); err != nil {
				unresolved = append(unresolved, attempt.ID+": no-invocation fact could not be retained: "+err.Error())
				continue
			}
			refreshed, err := store.BinaryUpdateReconciliations(attempt.Manifest.CaseEvidence)
			if err != nil {
				unresolved = append(unresolved, attempt.ID+": retained no-invocation fact cannot be read back: "+err.Error())
				continue
			}
			for _, candidate := range refreshed {
				if candidate.Attempt.ID == attempt.ID {
					entry = candidate
					break
				}
			}
		}
		if entry.Outcome == nil {
			unresolved = append(unresolved, attempt.ID+": updater completion is unknown; replay refused")
			continue
		}
		if entry.ProjectionReadBack {
			continue
		}
		debt := maat.BinaryUpdateProjectionDebt{}
		if entry.Projection != nil {
			debt = *entry.Projection
		} else {
			preview := maatBinaryDriftPreviewFromManifest(attempt.Manifest)
			determination := entry.Outcome.Determination
			if determination == "not_invoked" {
				determination = "failed"
			}
			after := "post-update observation: " + entry.Outcome.Detail
			if entry.Outcome.Determination == "not_invoked" {
				after = "updater was not invoked: " + entry.Outcome.Detail
			} else if determination == "resolved" {
				after = "binary-drift diagnostic and local inventory are clear"
			}
			decision, err := buildMaatBinaryDriftDecision(attempt, *entry.Outcome, preview, determination, after, entry.Outcome.Detail)
			if err != nil {
				unresolved = append(unresolved, attempt.ID+": terminal projection cannot be reconstructed: "+err.Error())
				continue
			}
			debt, err = store.BeginBinaryUpdateProjection(attempt, *entry.Outcome, decision)
			if err != nil {
				unresolved = append(unresolved, attempt.ID+": terminal projection intent cannot be retained: "+err.Error())
				continue
			}
		}
		if err := completeMaatBinaryDriftProjection(journal, store, debt); err != nil {
			unresolved = append(unresolved, attempt.ID+": "+err.Error())
		}
	}
	if len(unresolved) != 0 {
		return fmt.Errorf("binary-drift reconciliation retained unresolved facts: %s", strings.Join(unresolved, "; "))
	}
	return nil
}

type maatBinaryDriftAttemptStatus struct {
	AttemptEvidence  string                          `json:"attempt_evidence"`
	CaseEvidence     string                          `json:"case_evidence"`
	Observation      string                          `json:"observation"`
	ObservationError string                          `json:"observation_error,omitempty"`
	MemoryReceipt    *maat.PreflightReceiptReference `json:"memory_receipt,omitempty"`
}

func inspectMaatBinaryDriftAttempt(journal maat.DecisionJournal, caseEvidence string) (maatBinaryDriftAttemptStatus, error) {
	if !strings.HasPrefix(caseEvidence, "maat-system-one:sha256=") || !isSHA256Hex(strings.TrimPrefix(caseEvidence, "maat-system-one:sha256=")) {
		return maatBinaryDriftAttemptStatus{}, errors.New("Ma'at binary-drift attempt inspection: invalid System One case evidence")
	}
	if fileJournal, ok := journal.(*maat.FileDecisionJournal); ok {
		attempt, found, err := fileJournal.FindByEvidence("diagnostic repair attempt", maatBinaryDriftAttemptEvidence(caseEvidence))
		if err != nil {
			return maatBinaryDriftAttemptStatus{}, fmt.Errorf("Ma'at binary-drift attempt inspection: read retained journal: %w", err)
		}
		if !found || attempt.Resource != maatRepairBinaryDriftCheck || attempt.OriginEvidence != caseEvidence {
			return maatBinaryDriftAttemptStatus{}, errors.New("Ma'at binary-drift attempt inspection: no exact durable attempt is present in the retained journal")
		}
		terminal, err := fileJournal.HasOriginDecision("diagnostic verified repair", maatRepairBinaryDriftCheck, caseEvidence)
		if err != nil {
			return maatBinaryDriftAttemptStatus{}, fmt.Errorf("Ma'at binary-drift attempt inspection: read retained terminal outcome: %w", err)
		}
		if terminal {
			return maatBinaryDriftAttemptStatus{}, errors.New("Ma'at binary-drift attempt inspection: this attempt already has a terminal repair outcome; review its retained Casebook receipt")
		}
		status := observeMaatBinaryDriftAttempt(attempt)
		status.MemoryReceipt, err = projectedMaatBinaryDriftReceipt(fileJournal, attempt)
		if err != nil {
			return maatBinaryDriftAttemptStatus{}, fmt.Errorf("Ma'at binary-drift attempt inspection: follow canonical failure-memory receipt: %w", err)
		}
		return status, nil
	}
	decisions, err := journal.Recent(100)
	if err != nil {
		return maatBinaryDriftAttemptStatus{}, fmt.Errorf("Ma'at binary-drift attempt inspection: read retained journal: %w", err)
	}
	for i := len(decisions) - 1; i >= 0; i-- {
		attempt := decisions[i]
		if attempt.Kind != "diagnostic repair attempt" || attempt.Resource != maatRepairBinaryDriftCheck || attempt.OriginEvidence != caseEvidence || attempt.Evidence != maatBinaryDriftAttemptEvidence(caseEvidence) {
			continue
		}
		terminal := false
		for _, decision := range decisions {
			if decision.Kind == "diagnostic verified repair" && decision.Resource == maatRepairBinaryDriftCheck && decision.OriginEvidence == attempt.OriginEvidence {
				terminal = true
				break
			}
		}
		if terminal {
			continue
		}
		status := observeMaatBinaryDriftAttempt(attempt)
		status.MemoryReceipt, err = projectedMaatBinaryDriftReceipt(journal, attempt)
		if err != nil {
			return maatBinaryDriftAttemptStatus{}, fmt.Errorf("Ma'at binary-drift attempt inspection: follow canonical failure-memory receipt: %w", err)
		}
		return status, nil
	}
	return maatBinaryDriftAttemptStatus{}, errors.New("Ma'at binary-drift attempt inspection: no unresolved durable attempt is present in the retained journal")
}

// projectedMaatBinaryDriftReceipt follows the canonical typed preflight row
// for read-only inspection. Claim Why hashes are correlation context only;
// neither this lookup nor the text grants action authority.
func projectedMaatBinaryDriftReceipt(journal maat.DecisionJournal, claim maat.Decision) (*maat.PreflightReceiptReference, error) {
	const marker = "; retained failure-memory receipt="
	_, suffix, ok := strings.Cut(claim.Why, marker)
	if !ok {
		return nil, nil
	}
	receiptHash, suffix, ok := strings.Cut(suffix, "; action=")
	if !ok || !isSHA256Hex(receiptHash) {
		return nil, errors.New("malformed receipt correlation in attempt context")
	}
	actionHash, registryHash, ok := strings.Cut(suffix, "; registry=")
	if !ok || !isSHA256Hex(actionHash) || !isSHA256Hex(registryHash) || strings.Contains(registryHash, ";") {
		return nil, errors.New("malformed action or registry correlation in attempt context")
	}
	reader, ok := journal.(interface {
		FindByEvidence(string, string) (maat.Decision, bool, error)
	})
	if !ok {
		return nil, errors.New("exact decision-journal lookup is unavailable")
	}
	evidence := "maat-failure-memory:receipt-sha256=" + receiptHash + ";action-sha256=" + actionHash + ";registry-sha256=" + registryHash
	decision, found, err := reader.FindByEvidence("failure memory preflight", evidence)
	if err != nil {
		return nil, err
	}
	if !found || decision.FailureMemoryReceipt == nil || decision.FailureMemoryReceipt.EvidenceReference() != evidence || decision.FailureMemoryReceipt.Action.Operation != "update-homebrew-binary-drift" {
		return nil, nil
	}
	reference := *decision.FailureMemoryReceipt
	return &reference, nil
}

func observeMaatBinaryDriftAttempt(attempt maat.Decision) maatBinaryDriftAttemptStatus {
	status := maatBinaryDriftAttemptStatus{AttemptEvidence: attempt.Evidence, CaseEvidence: attempt.OriginEvidence, Observation: "unknown"}
	ctx, cancel := context.WithTimeout(context.Background(), maatBinaryDriftObservationTimeout)
	finding, readErr := maatRepairReadBinaryFinding(ctx)
	cancel()
	if readErr != nil {
		status.ObservationError = readErr.Error()
	} else if maatBinaryDriftFindingActive(finding, nil) {
		status.Observation = "binary drift remains active"
	} else if maatBinaryDriftFindingClear(finding) {
		status.Observation = "binary drift is currently clear; this does not attribute the change to the prior attempt"
	} else {
		status.Observation = "binary state is not a verified clear result"
	}
	return status
}

const maatBinaryDriftObservationTimeout = 3 * time.Second

func readMaatBinaryDriftFinding(ctx context.Context) (guard.DiagnosticFinding, error) {
	drift, err := maatRepairScanDriftContext(ctx)
	if err != nil {
		return guard.DiagnosticFinding{}, err
	}
	return pantheonBinaryDriftFinding(drift), nil
}

func maatBinaryDriftFindingActive(finding guard.DiagnosticFinding, err error) bool {
	return err == nil && finding.Check == maatRepairBinaryDriftCheck && finding.Severity >= guard.SeverityWarn && guard.ResolutionFor(finding) == guard.ResolutionRepair
}

func maatBinaryDriftFindingClear(finding guard.DiagnosticFinding) bool {
	return finding.Check == maatRepairBinaryDriftCheck && finding.Severity < guard.SeverityWarn && guard.ResolutionFor(finding) != guard.ResolutionRepair
}

func recordMaatBinaryDriftAttempt(journal maat.DecisionJournal, preview maatBinaryDriftPreview) error {
	return recordMaatBinaryDriftAttemptBound(journal, preview, maat.PreflightReceiptReference{}, "")
}

func recordMaatBinaryDriftAttemptBound(journal maat.DecisionJournal, preview maatBinaryDriftPreview, memoryReceipt maat.PreflightReceiptReference, manifestDigest string) error {
	// One source case gets one launch claim even if concurrent previews observed
	// different target bytes. The reviewed binding remains recorded as context,
	// but cannot split the replay fence into multiple permits.
	decision := maat.Decision{
		Kind: "diagnostic repair attempt", Requester: "sirsi maat repair binary-drift", Resource: maatRepairBinaryDriftCheck,
		Assessed: "confirmed supported Homebrew updater", Determination: "in_progress",
		Why:      "durable pre-launch claim for preview sha256=" + preview.Binding,
		Evidence: maatBinaryDriftAttemptEvidence(preview.CaseEvidence), OriginEvidence: preview.CaseEvidence,
	}
	if memoryReceipt.ReceiptSHA256 != "" {
		decision.Why += "; retained failure-memory receipt=" + memoryReceipt.ReceiptSHA256 + "; action=" + manifestDigest + "; registry=" + memoryReceipt.RegistrySHA256
	}
	if fileJournal, ok := journal.(*maat.FileDecisionJournal); ok {
		appended, err := fileJournal.AppendBinaryDriftAttemptOnce(decision)
		if err != nil {
			return fmt.Errorf("Ma'at binary-drift recovery: retain operation-wide durable launch claim: %w", err)
		}
		if !appended {
			return errors.New("Ma'at binary-drift recovery: this exact screen already has a launch claim; inspect the retained attempt and create a fresh System One screen before another action")
		}
		return nil
	}
	appender, ok := journal.(interface {
		AppendOnce(maat.Decision) (bool, error)
	})
	if !ok {
		return errors.New("Ma'at binary-drift recovery: journal does not support an atomic durable launch claim; no updater was run")
	}
	appended, err := appender.AppendOnce(decision)
	if err != nil {
		return fmt.Errorf("Ma'at binary-drift recovery: retain durable pre-launch claim: %w", err)
	}
	if !appended {
		return errors.New("Ma'at binary-drift recovery: this exact preview already has a launch claim; inspect the retained attempt and create a fresh System One screen before another action")
	}
	return nil
}

func maatBinaryDriftTargetsObserved(targets []maatBinaryDriftTarget, report *selfupdate.DriftReport) bool {
	if report == nil || len(targets) == 0 {
		return false
	}
	observed := make(map[string]selfupdate.Sibling, len(report.Siblings))
	for _, sibling := range report.Siblings {
		path := filepath.Clean(sibling.Path)
		if _, duplicate := observed[path]; duplicate {
			return false
		}
		observed[path] = sibling
	}
	for _, target := range targets {
		sibling, ok := observed[filepath.Clean(target.Path)]
		if !ok || sibling.Err != "" || strings.TrimSpace(sibling.Version) == "" || sibling.Version == "dev" {
			return false
		}
	}
	return true
}

func maatBinaryDriftAttemptEvidence(caseEvidence string) string {
	sum := sha256.Sum256([]byte(caseEvidence))
	return "diagnostic-repair-attempt:sha256=" + hex.EncodeToString(sum[:])
}

func currentMaatBinaryDriftPreview(caseEvidence string) (maatBinaryDriftPreview, error) {
	ctx, cancel := context.WithTimeout(context.Background(), maatBinaryDriftObservationTimeout)
	defer cancel()
	finding, err := maatRepairReadBinaryFinding(ctx)
	if err != nil {
		return maatBinaryDriftPreview{}, fmt.Errorf("Ma'at binary-drift preview: bounded binary inventory failed: %w", err)
	}
	if !maatBinaryDriftFindingActive(finding, nil) {
		return maatBinaryDriftPreview{}, errors.New("Ma'at binary-drift preview: the exact finding is no longer active; refresh System One and review the current evidence")
	}
	drift, err := maatRepairScanDriftContext(ctx)
	if err != nil {
		return maatBinaryDriftPreview{}, fmt.Errorf("Ma'at binary-drift preview: inspect local binary inventory: %w", err)
	}
	if drift == nil || drift.Healthy {
		return maatBinaryDriftPreview{}, errors.New("Ma'at binary-drift preview: no current drifted target is available; no updater was run")
	}
	brewPath, err := maatRepairFindHomebrew()
	if err != nil {
		return maatBinaryDriftPreview{}, fmt.Errorf("Ma'at binary-drift preview: Homebrew updater is unavailable; no target was changed: %w", err)
	}
	if !filepath.IsAbs(brewPath) {
		return maatBinaryDriftPreview{}, errors.New("Ma'at binary-drift preview: resolved Homebrew executable path is not absolute; no updater was run")
	}
	brewHash, err := maatRepairHashFile(brewPath)
	if err != nil {
		return maatBinaryDriftPreview{}, fmt.Errorf("Ma'at binary-drift preview: hash the resolved Homebrew updater: %w", err)
	}

	type targetInput struct {
		path, method, version string
	}
	inputs := make(map[string]targetInput)
	for _, sibling := range drift.D2Mismatch {
		inputs[sibling.Path] = targetInput{path: sibling.Path, method: sibling.Method.String(), version: sibling.Version}
	}
	if drift.D3PathBin != "" {
		path := drift.D3PathBin
		if _, exists := inputs[path]; !exists {
			inputs[path] = targetInput{path: path, method: maatRepairDetectMethod(path).String(), version: "selected PATH binary"}
		}
	}
	if len(inputs) == 0 || len(inputs) > 32 {
		return maatBinaryDriftPreview{}, errors.New("Ma'at binary-drift preview: target set is empty or exceeds the bounded 32-target review; no updater was run")
	}
	paths := make([]string, 0, len(inputs))
	for path := range inputs {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	targets := make([]maatBinaryDriftTarget, 0, len(paths))
	for _, path := range paths {
		input := inputs[path]
		if path == "" || input.method != selfupdate.MethodHomebrew.String() {
			return maatBinaryDriftPreview{}, fmt.Errorf("Ma'at binary-drift preview: refusing unsupported or unmanaged target %q (method %q); use its normal installer, no file was changed", path, input.method)
		}
		hash, hashErr := maatRepairHashFile(path)
		if hashErr != nil {
			return maatBinaryDriftPreview{}, fmt.Errorf("Ma'at binary-drift preview: hash exact target %q: %w", path, hashErr)
		}
		targets = append(targets, maatBinaryDriftTarget{Path: path, Method: input.method, Version: input.version, SHA256: hash})
	}
	selfHash, err := maatRepairHashFile(drift.Self.Path)
	if err != nil {
		return maatBinaryDriftPreview{}, fmt.Errorf("Ma'at binary-drift preview: hash running Sirsi executable: %w", err)
	}
	preview := maatBinaryDriftPreview{
		CaseEvidence:  caseEvidence,
		Updater:       "brew upgrade sirsi-pantheon",
		UpdaterPath:   brewPath,
		UpdaterSHA256: brewHash,
		SelfPath:      drift.Self.Path,
		SelfVersion:   drift.Self.Version,
		SelfSHA256:    selfHash,
		Targets:       targets,
	}
	canonical := maatBinaryDriftBinding{
		CaseEvidence:  preview.CaseEvidence,
		Updater:       preview.Updater,
		UpdaterPath:   preview.UpdaterPath,
		UpdaterSHA256: preview.UpdaterSHA256,
		SelfPath:      preview.SelfPath,
		SelfVersion:   preview.SelfVersion,
		SelfSHA256:    preview.SelfSHA256,
		Targets:       preview.Targets,
	}
	raw, err := json.Marshal(canonical)
	if err != nil {
		return maatBinaryDriftPreview{}, fmt.Errorf("Ma'at binary-drift preview: bind exact updater plan: %w", err)
	}
	sum := sha256.Sum256(raw)
	preview.Binding = hex.EncodeToString(sum[:])
	plan, err := json.Marshal(preview)
	if err != nil || len(plan) > 10*1024 {
		return maatBinaryDriftPreview{}, errors.New("Ma'at binary-drift preview: exact plan exceeds the bounded retained-evidence size; no updater was run")
	}
	return preview, nil
}

func validateMaatBinaryDriftCase(journal maat.DecisionJournal, evidence string) error {
	if !strings.HasPrefix(evidence, "maat-system-one:sha256=") || !isSHA256Hex(strings.TrimPrefix(evidence, "maat-system-one:sha256=")) {
		return errors.New("Ma'at binary-drift recovery: invalid System One evidence reference")
	}
	decisions, err := journal.Recent(100)
	if err != nil {
		return fmt.Errorf("Ma'at binary-drift recovery: read retained System One case: %w", err)
	}
	var selected *maat.Decision
	for i := range decisions {
		if decisions[i].Evidence != evidence {
			continue
		}
		if selected != nil {
			return errors.New("Ma'at binary-drift recovery: evidence resolves to multiple journal rows; no updater was run")
		}
		selected = &decisions[i]
	}
	if selected == nil || selected.Kind != "system one screen" || selected.SystemOne == nil {
		return errors.New("Ma'at binary-drift recovery: exact retained System One case is unavailable; refresh Casebook")
	}
	verdict := selected.SystemOne
	if err := maat.ValidateMaatVerdict(*verdict); err != nil {
		return fmt.Errorf("Ma'at binary-drift recovery: retained System One case is invalid: %w", err)
	}
	host, hostErr := maatTriageHostname()
	if hostErr != nil || verdict.Subject.Kind != "host" || verdict.Subject.Ref != "diagnostic" || verdict.Subject.Repo != strings.TrimSpace(host) {
		return errors.New("Ma'at binary-drift recovery: the retained evidence is not this host's diagnostic screen")
	}
	raw, err := json.Marshal(verdict)
	if err != nil {
		return fmt.Errorf("Ma'at binary-drift recovery: bind retained screen: %w", err)
	}
	sum := sha256.Sum256(raw)
	if evidence != "maat-system-one:sha256="+hex.EncodeToString(sum[:]) {
		return errors.New("Ma'at binary-drift recovery: retained screen digest does not match its evidence reference")
	}
	count := 0
	for _, finding := range verdict.Findings {
		closed := finding.RepairID == maat.SystemOneRepairBinaryDrift
		legacy := finding.RepairID == "" && maat.IsCanonicalHostDiagnosticFinding(*verdict, finding, maatRepairBinaryDriftCheck)
		if !closed && !legacy {
			continue
		}
		count++
		if !maat.IsCanonicalHostDiagnosticFinding(*verdict, finding, maatRepairBinaryDriftCheck) {
			return errors.New("Ma'at binary-drift recovery: closed repair reference is not bound to the canonical diagnostic finding")
		}
	}
	if count != 1 {
		return errors.New("Ma'at binary-drift recovery: exact screen must contain one closed binary-drift action")
	}
	if fileJournal, ok := journal.(*maat.FileDecisionJournal); ok {
		for _, kind := range []string{"diagnostic verified repair", "diagnostic repair attempt"} {
			found, err := fileJournal.HasOriginDecision(kind, maatRepairBinaryDriftCheck, evidence)
			if err != nil {
				return fmt.Errorf("Ma'at binary-drift recovery: read exact repair history: %w", err)
			}
			if found {
				return errors.New("Ma'at binary-drift recovery: this exact screen already has a durable repair attempt or receipt; inspect the retained attempt and create a fresh System One screen before another action")
			}
		}
		for _, kind := range []string{"diagnostic owner acceptance", "diagnostic owner review"} {
			found, err := fileJournal.HasRelatedEvidence(kind, evidence)
			if err != nil {
				return fmt.Errorf("Ma'at binary-drift recovery: read exact owner-decision history: %w", err)
			}
			if found {
				return errors.New("Ma'at binary-drift recovery: this case already has a separate owner decision; refresh and review it")
			}
		}
	} else {
		for _, decision := range decisions {
			if (decision.Kind == "diagnostic verified repair" || decision.Kind == "diagnostic repair attempt") && decision.OriginEvidence == evidence {
				return errors.New("Ma'at binary-drift recovery: this exact screen already has a durable repair attempt or receipt; inspect the retained attempt and create a fresh System One observation before another action")
			}
			if (decision.Kind == "diagnostic owner acceptance" && decision.ResolutionFor == evidence) ||
				(decision.Kind == "diagnostic owner review" && (decision.Evidence == evidence || decision.OriginEvidence == evidence)) {
				return errors.New("Ma'at binary-drift recovery: this case already has a separate owner decision; refresh and review it")
			}
		}
	}
	return nil
}

func maatBinaryDriftDiagnosisClear(report *guard.DoctorReport) bool {
	if report == nil {
		return false
	}
	count := 0
	for _, finding := range report.Findings {
		if finding.Check != maatRepairBinaryDriftCheck {
			continue
		}
		count++
		if count > 1 || finding.Severity >= guard.SeverityWarn {
			return false
		}
	}
	return true
}

func maatRecordBinaryDriftOutcome(journal maat.DecisionJournal, preview maatBinaryDriftPreview, resolved bool, detail string, updaterErr error) error {
	detail = boundedMaatRepairDetail(detail, 4*1024)
	operation := preview.Updater + "; confirmed preview sha256=" + preview.Binding
	determination := "failed"
	if resolved && updaterErr == nil {
		determination = "resolved"
	}
	after := "post-update observation: " + detail
	if determination == "resolved" {
		after = "binary-drift diagnostic and local inventory are clear"
	}
	plan, _ := json.Marshal(preview)
	recordedDetail := "reviewed preview=" + string(plan) + "\npost-update result=" + detail
	outcome := maat.DiagnosticRepair{
		Check: maatRepairBinaryDriftCheck, Operation: operation, OriginEvidence: preview.CaseEvidence, Before: preview.Binding,
		After: after, Determination: determination, Detail: recordedDetail,
	}
	decision, err := maat.RecordDiagnosticRepair(journal, "sirsi maat repair binary-drift", outcome)
	if err != nil {
		return fmt.Errorf("Ma'at binary-drift recovery outcome could not be retained; do not replay without a fresh preview: %w", err)
	}
	summary := "The supported Homebrew update and fresh diagnostic result were retained."
	if determination != "resolved" {
		summary = "The Homebrew update or fresh diagnostic did not verify; Ma'at retained an incomplete outcome."
	}
	result := &output.CommandResult{
		Command: "sirsi maat repair binary-drift", BriefTitle: "Ma'at binary-drift recovery",
		Status: determination, Summary: summary,
		Evidence: []output.Evidence{
			{Label: "Original case", Value: preview.CaseEvidence},
			{Label: "Updater binding", Value: preview.Binding},
			{Label: "Fresh result", Value: after},
			{Label: "Ma'at receipt", Value: decision.Evidence},
		},
		NextActions: []output.NextAction{{Label: "Open Ma'at Casebook", Command: "sirsi maat casebook", Description: "Inspect the original observation and the separate retained repair outcome."}},
	}
	result.Render()
	if determination != "resolved" {
		if updaterErr != nil {
			return fmt.Errorf("Ma'at binary-drift update did not verify: %w", updaterErr)
		}
		return errors.New("Ma'at binary-drift update did not clear the fresh diagnostic; the original case remains open")
	}
	return nil
}

func boundedMaatRepairDetail(value string, maxBytes int) string {
	if len(value) <= maxBytes {
		return value
	}
	cut := maxBytes
	for cut > 0 && !utf8.ValidString(value[:cut]) {
		cut--
	}
	return value[:cut] + " …[truncated]"
}

func isSHA256Hex(value string) bool {
	if len(value) != sha256.Size*2 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func init() {
	maatRepairBinaryDriftCmd.Flags().StringVar(&maatRepairBinaryDriftCaseEvidence, "case-evidence", "", "exact open System One case evidence shown in Ma'at")
	maatRepairBinaryDriftCmd.Flags().StringVar(&maatRepairBinaryDriftBinding, "preview-binding", "", "exact binding returned by the latest read-only updater preview")
	maatRepairBinaryDriftCmd.Flags().BoolVar(&maatRepairConfirm, "confirm", false, "confirm the exact Homebrew updater preview")
	maatRepairBinaryDriftCmd.Flags().BoolVar(&maatRepairBinaryDriftInspect, "inspect-attempt", false, "read-only inspection of an unresolved updater attempt and current binary observation")
	maatRepairBinaryDriftCmd.Flags().BoolVar(&maatRepairBinaryDriftReconcile, "reconcile", false, "reconcile retained binary-update facts and Casebook projection without running the updater")
	maatRepairCmd.AddCommand(maatRepairBinaryDriftCmd)
}
