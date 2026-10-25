package maat

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	internalversion "github.com/SirsiMaster/sirsi-pantheon/internal/version"
	"golang.org/x/sys/unix"
)

const RawSelfUpdateManifestSchema = "sirsi.maat.raw-self-update-action.v1"

// RawSelfUpdateTarget is one exact raw-owned target from the existing
// self-update preview. Other installation owners are rejected before a
// manifest can enter Store admission.
type RawSelfUpdateTarget struct {
	Path     string `json:"path"`
	Method   string `json:"method"`
	SHA256   string `json:"sha256"`
	OwnerUID uint32 `json:"owner_uid"`
	Device   uint64 `json:"device"`
	Inode    uint64 `json:"inode"`
	Mode     uint32 `json:"mode"`
}

// RawSelfUpdateManifest binds the confirmed SafeReplace route to the running
// signed release, exact ordered sibling targets, current schema observation,
// and the initialized local store. It contains no caller-supplied command.
type RawSelfUpdateManifest struct {
	Schema                   string                `json:"schema"`
	PreviewBinding           string                `json:"preview_binding"`
	SelfPath                 string                `json:"self_path"`
	SelfSHA256               string                `json:"self_sha256"`
	SelfOwnerUID             uint32                `json:"self_owner_uid"`
	SelfDevice               uint64                `json:"self_device"`
	SelfInode                uint64                `json:"self_inode"`
	SelfMode                 uint32                `json:"self_mode"`
	RunningInfo              internalversion.Info  `json:"running_info"`
	Targets                  []RawSelfUpdateTarget `json:"targets"`
	AuthorizedUID            uint32                `json:"authorized_uid"`
	LiveRouterSchema         int                   `json:"live_router_schema"`
	ObservedSchemaCompatible bool                  `json:"observed_schema_compatible"`
	StoreUUID                string                `json:"store_uuid"`
	ObservedAtUTC            time.Time             `json:"observed_at_utc"`
}

func rawSelfUpdateScope() Scope {
	return Scope{Component: "pantheon", Profile: "local-macos", Operation: "update-cli-binary-drift"}
}

func (m RawSelfUpdateManifest) Validate() error {
	if m.Schema != RawSelfUpdateManifestSchema || !validDigest(m.PreviewBinding) || m.AuthorizedUID != uint32(os.Geteuid()) ||
		!filepathIsCanonicalAbsolute(m.SelfPath) || !validDigest(m.SelfSHA256) || m.SelfInode == 0 ||
		m.SelfMode&uint32(unix.S_IFMT) != uint32(unix.S_IFREG) || !validOwnerMode(m.AuthorizedUID, m.SelfOwnerUID, m.SelfMode) ||
		m.RunningInfo.Binary != "sirsi" || m.RunningInfo.Path != m.SelfPath || strings.TrimSpace(m.RunningInfo.Version) == "" || m.RunningInfo.Version == "dev" ||
		m.RunningInfo.Commit == "" || m.RunningInfo.Commit == "none" || m.RunningInfo.Dirty || m.RunningInfo.RouterSchemaMax <= 0 ||
		m.LiveRouterSchema < 0 || !m.ObservedSchemaCompatible || m.RunningInfo.RouterSchemaMax < m.LiveRouterSchema ||
		!validUUID(m.StoreUUID) || m.ObservedAtUTC.IsZero() || m.ObservedAtUTC.Location() != time.UTC || len(m.Targets) == 0 || len(m.Targets) > 32 {
		return errors.New("maat memory: invalid raw self-update action manifest")
	}
	last := ""
	for _, target := range m.Targets {
		if !filepathIsCanonicalAbsolute(target.Path) || target.Path <= last || target.Method != "raw" ||
			!validDigest(target.SHA256) || target.Inode == 0 || target.Mode&uint32(unix.S_IFMT) != uint32(unix.S_IFREG) ||
			!validOwnerMode(m.AuthorizedUID, target.OwnerUID, target.Mode) {
			return errors.New("maat memory: invalid or unordered raw self-update target")
		}
		last = target.Path
	}
	return nil
}

// ValidateFresh is admission-only; historical attempts remain valid after the
// short preview window expires.
func (m RawSelfUpdateManifest) ValidateFresh() error {
	if err := m.Validate(); err != nil {
		return err
	}
	now := binaryUpdateNow()
	if now.Sub(m.ObservedAtUTC) > 2*time.Minute || m.ObservedAtUTC.Sub(now) > 10*time.Second {
		return errors.New("maat memory: raw self-update preview is outside its admission window")
	}
	return nil
}

func (m RawSelfUpdateManifest) Digest() (string, error) {
	if err := m.Validate(); err != nil {
		return "", err
	}
	data, err := json.Marshal(m)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

type RawSelfUpdateAttempt struct {
	Schema                 string                    `json:"schema"`
	ID                     string                    `json:"id"`
	Manifest               RawSelfUpdateManifest     `json:"manifest"`
	ActionManifestSHA256   string                    `json:"action_manifest_sha256"`
	PreflightReceipt       PreflightReceiptReference `json:"preflight_receipt"`
	PreflightReceiptSHA256 string                    `json:"preflight_receipt_sha256"`
	RegistrySnapshotSHA256 string                    `json:"registry_snapshot_sha256"`
	StartedAtUTC           time.Time                 `json:"started_at_utc"`
}

type RawSelfUpdateInvocation struct {
	Schema          string    `json:"schema"`
	AttemptID       string    `json:"attempt_id"`
	AttemptSHA256   string    `json:"attempt_sha256"`
	AuthorizedAtUTC time.Time `json:"authorized_at_utc"`
}

type RawSelfUpdateOutcome struct {
	Schema            string    `json:"schema"`
	AttemptID         string    `json:"attempt_id"`
	RecordedAtUTC     time.Time `json:"recorded_at_utc"`
	Determination     string    `json:"determination"`
	UpdaterReturned   bool      `json:"updater_returned"`
	PostcheckPassed   bool      `json:"postcheck_passed"`
	ObservationSHA256 string    `json:"observation_sha256"`
	FactCode          string    `json:"fact_code,omitempty"`
	Detail            string    `json:"detail"`
	Decision          Decision  `json:"decision"`
	DecisionSHA256    string    `json:"decision_sha256"`
}

// RawSelfUpdateProjectionReceipt closes only an exact idempotent journal
// append/readback. The outcome itself is the durable projection intent, so a
// missing receipt is data-only debt and never permission to rerun SafeReplace.
type RawSelfUpdateProjectionReceipt struct {
	Schema        string    `json:"schema"`
	AttemptID     string    `json:"attempt_id"`
	OutcomeSHA256 string    `json:"outcome_sha256"`
	JournalKey    string    `json:"journal_key"`
	ReadBackAtUTC time.Time `json:"readback_at_utc"`
}

type RawSelfUpdateReconciliation struct {
	Attempt            RawSelfUpdateAttempt  `json:"attempt"`
	InvocationRecorded bool                  `json:"invocation_recorded"`
	Outcome            *RawSelfUpdateOutcome `json:"outcome,omitempty"`
	ProjectionReadBack bool                  `json:"projection_read_back"`
}

var ErrNoIncompleteRawSelfUpdateAttempt = errors.New("maat memory: no incomplete raw self-update attempt")

func rawSelfUpdateEvidenceName(kind, id string) string {
	return "raw-self-update-" + kind + "-" + id + ".json"
}

func rawSelfUpdateProjectionReceiptName(id string) string {
	return "raw-self-update-projection-receipt-" + id + ".json"
}

func (s *Store) PreflightRawSelfUpdate(manifest RawSelfUpdateManifest) (PreflightReceipt, error) {
	if s == nil || s.storeUUID == "" || manifest.StoreUUID != s.storeUUID {
		return PreflightReceipt{}, errors.New("maat memory: raw self-update action is not bound to this initialized store")
	}
	if err := manifest.ValidateFresh(); err != nil {
		return PreflightReceipt{}, err
	}
	digest, err := manifest.Digest()
	if err != nil {
		return PreflightReceipt{}, err
	}
	return s.preflightBound(rawSelfUpdateScope(), digest, resolveMemoryGuard)
}

// BeginRawSelfUpdateAttempt requires the opaque G02 retained-receipt handle,
// reopens its committed bytes, rechecks the registry and other update owners
// under one Store lock, then records the no-replay intent before any .new file.
func (s *Store) BeginRawSelfUpdateAttempt(manifest RawSelfUpdateManifest, stored StoredPreflightReceipt) (RawSelfUpdateAttempt, error) {
	receipt, receiptErr := stored.Receipt()
	reference := stored.Reference()
	if s == nil || s.evidenceFD < 0 || receiptErr != nil || manifest.StoreUUID != s.storeUUID || manifest.ValidateFresh() != nil ||
		receipt.validate() != nil || receipt.Decision != PreflightPass {
		return RawSelfUpdateAttempt{}, errors.New("maat memory: raw self-update requires a passing exact retained preflight")
	}
	digest, err := manifest.Digest()
	if err != nil || receipt.Action != rawSelfUpdateScope() || receipt.ActionManifestSHA256 != digest {
		return RawSelfUpdateAttempt{}, errors.New("maat memory: preflight does not bind the exact raw self-update plan")
	}
	receiptDigest, err := receipt.Digest()
	if err != nil {
		return RawSelfUpdateAttempt{}, err
	}
	if reference.ReceiptSHA256 != receiptDigest || reference.ActionSHA256 != digest || reference.Action != rawSelfUpdateScope() ||
		reference.RegistrySHA256 != receipt.RegistrySnapshotSHA256 || reference.RegistrySnapshotVersion != receipt.RegistrySnapshotVersion ||
		reference.StoreBinding.State != "initialized" || reference.StoreBinding.StoreUUID != s.storeUUID {
		return RawSelfUpdateAttempt{}, errors.New("maat memory: stored preflight handle is not bound to this raw self-update and initialized store")
	}
	if err := s.lock(unix.LOCK_EX); err != nil {
		return RawSelfUpdateAttempt{}, err
	}
	defer s.unlock()
	if err := manifest.ValidateFresh(); err != nil {
		return RawSelfUpdateAttempt{}, err
	}
	retained, err := s.readPreflightReceiptLocked(reference.ReceiptSHA256)
	if err != nil || digestEqual(retained, receipt) != nil {
		return RawSelfUpdateAttempt{}, errors.New("maat memory: exact committed raw self-update preflight receipt is unavailable or changed")
	}
	current, evalErr := s.preflightBoundLocked(rawSelfUpdateScope(), digest, resolveMemoryGuard)
	if evalErr != nil || current.Decision != PreflightPass || current.RegistrySnapshotSHA256 != receipt.RegistrySnapshotSHA256 {
		return RawSelfUpdateAttempt{}, errors.New("maat memory: raw self-update preflight or registry changed before admission")
	}
	if _, err := s.pendingRawSelfUpdateAttemptLocked(); err == nil {
		return RawSelfUpdateAttempt{}, errors.New("maat memory: unresolved raw self-update attempt requires inspection; replay refused")
	} else if !errors.Is(err, ErrNoIncompleteRawSelfUpdateAttempt) {
		return RawSelfUpdateAttempt{}, err
	}
	if _, err := s.pendingBinaryUpdateAttemptLocked(); err == nil {
		return RawSelfUpdateAttempt{}, errors.New("maat memory: unresolved Homebrew binary update must be reconciled before raw self-update")
	} else if !errors.Is(err, ErrNoIncompleteBinaryUpdateAttempt) {
		return RawSelfUpdateAttempt{}, err
	}
	if _, err := s.pendingLivenessWatchAttemptLocked(); err == nil {
		return RawSelfUpdateAttempt{}, errors.New("maat memory: unresolved liveness-watch installation must be inspected before raw self-update")
	} else if !errors.Is(err, ErrNoIncompleteLivenessWatchAttempt) {
		return RawSelfUpdateAttempt{}, err
	}
	if _, err := s.pendingAppUpdateAttemptLocked(); err == nil {
		return RawSelfUpdateAttempt{}, errors.New("maat memory: unresolved whole-app Installer handoff must be inspected before raw self-update")
	} else if !errors.Is(err, ErrNoIncompleteAppUpdateAttempt) {
		return RawSelfUpdateAttempt{}, fmt.Errorf("maat memory: shared attempt history is unverifiable: %w", err)
	}
	if err := s.ensureNoConflictingRawSelfUpdateLocked(); err != nil {
		return RawSelfUpdateAttempt{}, err
	}
	id, err := newStoreUUID()
	if err != nil {
		return RawSelfUpdateAttempt{}, err
	}
	attempt := RawSelfUpdateAttempt{Schema: MemorySchema, ID: id, Manifest: manifest, ActionManifestSHA256: digest,
		PreflightReceipt: reference, PreflightReceiptSHA256: receiptDigest, RegistrySnapshotSHA256: receipt.RegistrySnapshotSHA256,
		StartedAtUTC: binaryUpdateNow().UTC()}
	if err := s.writeRawSelfUpdateRecord("started", id, attempt); err != nil {
		return RawSelfUpdateAttempt{}, fmt.Errorf("maat memory: persist raw self-update intent before SafeReplace: %w", err)
	}
	retainedAttempt, err := s.readRawSelfUpdateAttemptLocked(id)
	if err != nil || digestEqual(retainedAttempt, attempt) != nil {
		return RawSelfUpdateAttempt{}, errors.New("maat memory: raw self-update durable intent readback differs")
	}
	return attempt, nil
}

func (s *Store) RecordRawSelfUpdateInvocation(attempt RawSelfUpdateAttempt) error {
	if s == nil || s.evidenceFD < 0 || validateRawSelfUpdateAttempt(attempt, s.storeUUID) != nil {
		return errors.New("maat memory: invalid raw self-update invocation identity")
	}
	if err := s.lock(unix.LOCK_EX); err != nil {
		return err
	}
	defer s.unlock()
	pending, err := s.pendingRawSelfUpdateAttemptLocked()
	if err != nil || pending.ID != attempt.ID || digestEqual(pending, attempt) != nil {
		return errors.New("maat memory: invocation requires the unique incomplete raw self-update attempt")
	}
	if err := attempt.Manifest.ValidateFresh(); err != nil {
		return err
	}
	if _, err := s.readPreflightReceiptLocked(attempt.PreflightReceiptSHA256); err != nil {
		return errors.New("maat memory: raw self-update invocation requires the exact retained receipt")
	}
	current, evalErr := s.preflightBoundLocked(rawSelfUpdateScope(), attempt.ActionManifestSHA256, resolveMemoryGuard)
	if evalErr != nil || current.Decision != PreflightPass || current.RegistrySnapshotSHA256 != attempt.RegistrySnapshotSHA256 {
		return errors.New("maat memory: raw self-update registry or action changed before SafeReplace")
	}
	if _, err := s.readRawSelfUpdateInvocationLocked(attempt.ID); err == nil {
		return errors.New("maat memory: raw self-update invocation was already recorded")
	} else if !errors.Is(err, unix.ENOENT) {
		return err
	}
	digest, err := digestJSON(attempt)
	if err != nil {
		return err
	}
	invocation := RawSelfUpdateInvocation{Schema: MemorySchema, AttemptID: attempt.ID, AttemptSHA256: digest, AuthorizedAtUTC: binaryUpdateNow().UTC()}
	if err := s.writeRawSelfUpdateRecord("invocation", attempt.ID, invocation); err != nil {
		return err
	}
	readback, err := s.readRawSelfUpdateInvocationLocked(attempt.ID)
	if err != nil || digestEqual(readback, invocation) != nil {
		return errors.New("maat memory: raw self-update invocation readback differs; execution remains unresolved")
	}
	return nil
}

func (s *Store) RecordRawSelfUpdateNotInvoked(attempt RawSelfUpdateAttempt, factCode, detail string) (RawSelfUpdateOutcome, error) {
	if s == nil || s.evidenceFD < 0 || validateRawSelfUpdateAttempt(attempt, s.storeUUID) != nil || len(detail) > 4096 ||
		(factCode != "prelaunch_action_changed" && factCode != "prelaunch_schema_changed" && factCode != "prelaunch_target_changed" && factCode != "invocation_not_recorded" && factCode != "reconcile_no_invocation_marker") {
		return RawSelfUpdateOutcome{}, errors.New("maat memory: invalid raw self-update known-not-invoked fact")
	}
	if err := s.lock(unix.LOCK_EX); err != nil {
		return RawSelfUpdateOutcome{}, err
	}
	defer s.unlock()
	pending, err := s.pendingRawSelfUpdateAttemptLocked()
	if err != nil || pending.ID != attempt.ID || digestEqual(pending, attempt) != nil {
		return RawSelfUpdateOutcome{}, errors.New("maat memory: known-not-invoked fact requires the unique incomplete raw self-update attempt")
	}
	if _, err := s.readRawSelfUpdateInvocationLocked(attempt.ID); err == nil {
		return RawSelfUpdateOutcome{}, errors.New("maat memory: raw self-update invocation exists; leave its completion unresolved")
	} else if !errors.Is(err, unix.ENOENT) {
		return RawSelfUpdateOutcome{}, err
	}
	sum := sha256.Sum256([]byte("raw-self-update-not-invoked\x00" + factCode + "\x00" + detail))
	outcome := RawSelfUpdateOutcome{Schema: MemorySchema, AttemptID: attempt.ID, RecordedAtUTC: binaryUpdateNow().UTC(),
		Determination: "not_invoked", ObservationSHA256: hex.EncodeToString(sum[:]), FactCode: factCode, Detail: detail}
	decision, err := rawSelfUpdateOutcomeDecision(attempt, outcome)
	if err != nil {
		return RawSelfUpdateOutcome{}, err
	}
	outcome.Decision = decision
	outcome.DecisionSHA256, err = digestJSON(decision)
	if err != nil {
		return RawSelfUpdateOutcome{}, err
	}
	if err := s.writeRawSelfUpdateRecord("outcome", attempt.ID, outcome); err != nil {
		return RawSelfUpdateOutcome{}, err
	}
	readback, err := s.readRawSelfUpdateOutcomeLocked(attempt.ID)
	if err != nil || digestEqual(readback, outcome) != nil {
		return RawSelfUpdateOutcome{}, errors.New("maat memory: raw self-update known-not-invoked outcome readback differs")
	}
	return outcome, nil
}

func (s *Store) RecordRawSelfUpdateOutcome(attempt RawSelfUpdateAttempt, postcheckPassed bool, observationSHA256, detail string) (RawSelfUpdateOutcome, error) {
	if s == nil || s.evidenceFD < 0 || validateRawSelfUpdateAttempt(attempt, s.storeUUID) != nil || len(detail) > 4096 || !validDigest(observationSHA256) {
		return RawSelfUpdateOutcome{}, errors.New("maat memory: invalid raw self-update outcome")
	}
	if err := s.lock(unix.LOCK_EX); err != nil {
		return RawSelfUpdateOutcome{}, err
	}
	defer s.unlock()
	pending, err := s.pendingRawSelfUpdateAttemptLocked()
	if err != nil || pending.ID != attempt.ID || digestEqual(pending, attempt) != nil {
		return RawSelfUpdateOutcome{}, errors.New("maat memory: raw self-update outcome requires the unique incomplete attempt")
	}
	invocation, err := s.readRawSelfUpdateInvocationLocked(attempt.ID)
	attemptDigest, digestErr := digestJSON(attempt)
	if err != nil || digestErr != nil || invocation.AttemptSHA256 != attemptDigest {
		return RawSelfUpdateOutcome{}, errors.New("maat memory: raw self-update returned without its exact durable invocation fact")
	}
	determination := "failed"
	if postcheckPassed {
		determination = "resolved"
	}
	outcome := RawSelfUpdateOutcome{Schema: MemorySchema, AttemptID: attempt.ID, RecordedAtUTC: binaryUpdateNow().UTC(),
		Determination: determination, UpdaterReturned: true, PostcheckPassed: postcheckPassed,
		ObservationSHA256: observationSHA256, Detail: detail}
	decision, err := rawSelfUpdateOutcomeDecision(attempt, outcome)
	if err != nil {
		return RawSelfUpdateOutcome{}, err
	}
	outcome.Decision = decision
	outcome.DecisionSHA256, err = digestJSON(decision)
	if err != nil {
		return RawSelfUpdateOutcome{}, err
	}
	if err := s.writeRawSelfUpdateRecord("outcome", attempt.ID, outcome); err != nil {
		return RawSelfUpdateOutcome{}, err
	}
	readback, err := s.readRawSelfUpdateOutcomeLocked(attempt.ID)
	if err != nil || digestEqual(readback, outcome) != nil {
		return RawSelfUpdateOutcome{}, errors.New("maat memory: raw self-update terminal outcome readback differs; do not replay")
	}
	return outcome, nil
}

func (s *Store) PendingRawSelfUpdateAttempt() (RawSelfUpdateAttempt, error) {
	if err := s.lock(unix.LOCK_SH); err != nil {
		return RawSelfUpdateAttempt{}, err
	}
	defer s.unlock()
	return s.pendingRawSelfUpdateAttemptLocked()
}

func (s *Store) RawSelfUpdateReconciliations() ([]RawSelfUpdateReconciliation, error) {
	if err := s.lock(unix.LOCK_SH); err != nil {
		return nil, err
	}
	defer s.unlock()
	if _, err := s.verifyWriteJournal(); err != nil {
		return nil, err
	}
	names, err := readDirectoryNames(s.evidenceFD)
	if err != nil {
		return nil, err
	}
	return s.rawSelfUpdateReconciliationsLocked(names)
}

func (s *Store) pendingRawSelfUpdateAttemptLocked() (RawSelfUpdateAttempt, error) {
	if _, err := s.verifyWriteJournal(); err != nil {
		return RawSelfUpdateAttempt{}, err
	}
	names, err := readDirectoryNames(s.evidenceFD)
	if err != nil {
		return RawSelfUpdateAttempt{}, err
	}
	var pending *RawSelfUpdateAttempt
	for _, name := range names {
		if !strings.HasPrefix(name, "raw-self-update-") {
			continue
		}
		if strings.HasPrefix(name, "raw-self-update-projection-receipt-") {
			id := strings.TrimSuffix(strings.TrimPrefix(name, "raw-self-update-projection-receipt-"), ".json")
			if name != rawSelfUpdateProjectionReceiptName(id) || !validUUID(id) {
				return RawSelfUpdateAttempt{}, errors.New("maat memory: malformed raw self-update projection receipt name")
			}
			continue
		}
		if !strings.HasSuffix(name, ".json") {
			return RawSelfUpdateAttempt{}, errors.New("maat memory: malformed raw self-update evidence name")
		}
		validName := false
		for _, kind := range []string{"started", "invocation", "outcome"} {
			prefix := "raw-self-update-" + kind + "-"
			if !strings.HasPrefix(name, prefix) {
				continue
			}
			id := strings.TrimSuffix(strings.TrimPrefix(name, prefix), ".json")
			if validUUID(id) && name == rawSelfUpdateEvidenceName(kind, id) {
				validName = true
				break
			}
		}
		if !validName {
			return RawSelfUpdateAttempt{}, errors.New("maat memory: unrecognized raw self-update evidence name")
		}
	}
	entries, err := s.rawSelfUpdateReconciliationsLocked(names)
	if err != nil {
		return RawSelfUpdateAttempt{}, err
	}
	for _, entry := range entries {
		if entry.Outcome != nil {
			continue
		}
		if pending != nil {
			return RawSelfUpdateAttempt{}, errors.New("maat memory: multiple unresolved raw self-update attempts require inspection")
		}
		copy := entry.Attempt
		pending = &copy
	}
	if pending == nil {
		return RawSelfUpdateAttempt{}, ErrNoIncompleteRawSelfUpdateAttempt
	}
	return *pending, nil
}

func (s *Store) rawSelfUpdateReconciliationsLocked(names []string) ([]RawSelfUpdateReconciliation, error) {
	if names == nil {
		var err error
		names, err = readDirectoryNames(s.evidenceFD)
		if err != nil {
			return nil, err
		}
	}
	entries := []RawSelfUpdateReconciliation{}
	startedIDs := map[string]bool{}
	projectionIDs := map[string]bool{}
	for _, name := range names {
		if !strings.HasPrefix(name, "raw-self-update-") {
			continue
		}
		if !strings.HasSuffix(name, ".json") {
			return nil, errors.New("maat memory: malformed raw self-update evidence name")
		}
		var id string
		switch {
		case strings.HasPrefix(name, "raw-self-update-projection-receipt-"):
			id = strings.TrimSuffix(strings.TrimPrefix(name, "raw-self-update-projection-receipt-"), ".json")
			if name != rawSelfUpdateProjectionReceiptName(id) {
				return nil, errors.New("maat memory: malformed raw self-update projection receipt name")
			}
			projectionIDs[id] = true
		case strings.HasPrefix(name, "raw-self-update-started-"):
			id = strings.TrimSuffix(strings.TrimPrefix(name, "raw-self-update-started-"), ".json")
			if name != rawSelfUpdateEvidenceName("started", id) {
				return nil, errors.New("maat memory: malformed raw self-update attempt name")
			}
			startedIDs[id] = true
		case strings.HasPrefix(name, "raw-self-update-invocation-"):
			id = strings.TrimSuffix(strings.TrimPrefix(name, "raw-self-update-invocation-"), ".json")
			if name != rawSelfUpdateEvidenceName("invocation", id) {
				return nil, errors.New("maat memory: malformed raw self-update invocation name")
			}
		case strings.HasPrefix(name, "raw-self-update-outcome-"):
			id = strings.TrimSuffix(strings.TrimPrefix(name, "raw-self-update-outcome-"), ".json")
			if name != rawSelfUpdateEvidenceName("outcome", id) {
				return nil, errors.New("maat memory: malformed raw self-update outcome name")
			}
		default:
			return nil, errors.New("maat memory: unrecognized raw self-update evidence name")
		}
		if !validUUID(id) {
			return nil, errors.New("maat memory: malformed raw self-update evidence identity")
		}
	}
	for _, name := range names {
		if !strings.HasPrefix(name, "raw-self-update-") || strings.HasPrefix(name, "raw-self-update-started-") || strings.HasPrefix(name, "raw-self-update-projection-receipt-") {
			continue
		}
		id := strings.TrimSuffix(strings.TrimPrefix(strings.TrimPrefix(name, "raw-self-update-invocation-"), "raw-self-update-outcome-"), ".json")
		if !startedIDs[id] {
			return nil, errors.New("maat memory: raw self-update fact has no durable attempt")
		}
	}
	for id := range startedIDs {
		attempt, err := s.readRawSelfUpdateAttemptLocked(id)
		if err != nil {
			return nil, err
		}
		entry := RawSelfUpdateReconciliation{Attempt: attempt}
		if _, err := s.readRawSelfUpdateInvocationLocked(id); err == nil {
			entry.InvocationRecorded = true
		} else if !errors.Is(err, unix.ENOENT) {
			return nil, err
		}
		if outcome, err := s.readRawSelfUpdateOutcomeLocked(id); err == nil {
			if err := validateRawSelfUpdateOutcome(outcome, attempt, entry.InvocationRecorded); err != nil {
				return nil, err
			}
			entry.Outcome = &outcome
			if receipt, err := s.readRawSelfUpdateProjectionReceiptLocked(id); err == nil {
				outcomeDigest, digestErr := digestJSON(outcome)
				if digestErr != nil || receipt.OutcomeSHA256 != outcomeDigest || receipt.JournalKey != outcome.Decision.Evidence ||
					receipt.ReadBackAtUTC.Before(outcome.RecordedAtUTC) || receipt.ReadBackAtUTC.After(binaryUpdateNow().Add(10*time.Second)) {
					return nil, errors.New("maat memory: raw self-update projection receipt does not bind the exact terminal outcome")
				}
				entry.ProjectionReadBack = true
			} else if !errors.Is(err, unix.ENOENT) {
				return nil, err
			}
		} else if !errors.Is(err, unix.ENOENT) {
			return nil, err
		}
		if _, err := s.readRawSelfUpdateProjectionReceiptLocked(id); err == nil && entry.Outcome == nil {
			return nil, errors.New("maat memory: raw self-update projection receipt has no terminal outcome")
		} else if err != nil && !errors.Is(err, unix.ENOENT) {
			return nil, err
		}
		entries = append(entries, entry)
		if len(entries) > 256 {
			return nil, errors.New("maat memory: raw self-update history exceeds the bounded inspection limit")
		}
	}
	for id := range projectionIDs {
		if !startedIDs[id] {
			return nil, errors.New("maat memory: raw self-update projection receipt has no durable attempt")
		}
	}
	return entries, nil
}

// MarkRawSelfUpdateProjectionReadBack persists the exact terminal projection
// receipt after the caller has performed idempotent append and strict readback.
func (s *Store) MarkRawSelfUpdateProjectionReadBack(attempt RawSelfUpdateAttempt, outcome RawSelfUpdateOutcome, readback Decision) error {
	if s == nil || s.evidenceFD < 0 || validateRawSelfUpdateAttempt(attempt, s.storeUUID) != nil ||
		validateRawSelfUpdateOutcome(outcome, attempt, outcome.Determination != "not_invoked") != nil {
		return errors.New("maat memory: invalid raw self-update projection identity")
	}
	if err := s.lock(unix.LOCK_EX); err != nil {
		return err
	}
	defer s.unlock()
	retainedOutcome, err := s.readRawSelfUpdateOutcomeLocked(attempt.ID)
	if err != nil || digestEqual(retainedOutcome, outcome) != nil {
		return errors.New("maat memory: raw self-update projection requires the exact retained terminal outcome")
	}
	expectedBytes, _ := json.Marshal(outcome.Decision)
	readbackBytes, _ := json.Marshal(readback)
	if string(expectedBytes) != string(readbackBytes) {
		return errors.New("maat memory: raw self-update journal readback differs from its retained projection intent")
	}
	outcomeDigest, err := digestJSON(outcome)
	if err != nil {
		return err
	}
	receipt := RawSelfUpdateProjectionReceipt{Schema: MemorySchema, AttemptID: attempt.ID, OutcomeSHA256: outcomeDigest,
		JournalKey: outcome.Decision.Evidence, ReadBackAtUTC: binaryUpdateNow().UTC()}
	if receipt.ReadBackAtUTC.Before(outcome.RecordedAtUTC) || receipt.ReadBackAtUTC.After(binaryUpdateNow().Add(10*time.Second)) {
		return errors.New("maat memory: raw self-update projection readback time is outside terminal outcome")
	}
	data, err := json.Marshal(receipt)
	if err != nil {
		return err
	}
	if err := s.createOrVerify("evidence", s.evidenceFD, rawSelfUpdateProjectionReceiptName(attempt.ID), data); err != nil {
		return err
	}
	retained, err := s.readRawSelfUpdateProjectionReceiptLocked(attempt.ID)
	if err != nil || digestEqual(retained, receipt) != nil {
		return errors.New("maat memory: raw self-update projection readback receipt differs")
	}
	return nil
}

// ensureNoConflictingRawSelfUpdateLocked is shared by update and liveness
// admissions. Callers hold the Store lock, so the check cannot race a raw
// self-update intent or its terminal projection debt.
func (s *Store) ensureNoConflictingRawSelfUpdateLocked() error {
	entries, err := s.rawSelfUpdateReconciliationsLocked(nil)
	if err != nil {
		return fmt.Errorf("maat memory: raw self-update history is unverifiable: %w", err)
	}
	for _, entry := range entries {
		if entry.Outcome == nil {
			return errors.New("maat memory: unresolved raw self-update attempt requires inspection; replay and conflicting actions refused")
		}
		if !entry.ProjectionReadBack {
			return errors.New("maat memory: raw self-update terminal projection debt requires data-only reconciliation")
		}
	}
	return nil
}

func (s *Store) writeRawSelfUpdateRecord(kind, id string, value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return s.createOrVerify("evidence", s.evidenceFD, rawSelfUpdateEvidenceName(kind, id), data)
}

func (s *Store) readRawSelfUpdateAttemptLocked(id string) (RawSelfUpdateAttempt, error) {
	data, err := readExact(s.evidenceFD, rawSelfUpdateEvidenceName("started", id))
	if err != nil {
		return RawSelfUpdateAttempt{}, err
	}
	var attempt RawSelfUpdateAttempt
	if json.Unmarshal(data, &attempt) != nil || validateRawSelfUpdateAttempt(attempt, s.storeUUID) != nil {
		return RawSelfUpdateAttempt{}, errors.New("maat memory: invalid raw self-update attempt record")
	}
	return attempt, nil
}

func (s *Store) readRawSelfUpdateInvocationLocked(id string) (RawSelfUpdateInvocation, error) {
	data, err := readExact(s.evidenceFD, rawSelfUpdateEvidenceName("invocation", id))
	if err != nil {
		return RawSelfUpdateInvocation{}, err
	}
	var invocation RawSelfUpdateInvocation
	if json.Unmarshal(data, &invocation) != nil || invocation.Schema != MemorySchema || invocation.AttemptID != id || !validDigest(invocation.AttemptSHA256) || invocation.AuthorizedAtUTC.IsZero() {
		return RawSelfUpdateInvocation{}, errors.New("maat memory: invalid raw self-update invocation record")
	}
	attempt, err := s.readRawSelfUpdateAttemptLocked(id)
	attemptDigest, digestErr := digestJSON(attempt)
	if err != nil || digestErr != nil || invocation.AttemptSHA256 != attemptDigest || invocation.AuthorizedAtUTC.Before(attempt.StartedAtUTC) || invocation.AuthorizedAtUTC.After(binaryUpdateNow().Add(10*time.Second)) {
		return RawSelfUpdateInvocation{}, errors.New("maat memory: raw self-update invocation does not bind its attempt or bounded time")
	}
	return invocation, nil
}

func (s *Store) readRawSelfUpdateOutcomeLocked(id string) (RawSelfUpdateOutcome, error) {
	data, err := readExact(s.evidenceFD, rawSelfUpdateEvidenceName("outcome", id))
	if err != nil {
		return RawSelfUpdateOutcome{}, err
	}
	var outcome RawSelfUpdateOutcome
	if json.Unmarshal(data, &outcome) != nil || outcome.Schema != MemorySchema || outcome.AttemptID != id || outcome.RecordedAtUTC.IsZero() || !validDigest(outcome.ObservationSHA256) {
		return RawSelfUpdateOutcome{}, errors.New("maat memory: invalid raw self-update outcome record")
	}
	return outcome, nil
}

func (s *Store) readRawSelfUpdateProjectionReceiptLocked(id string) (RawSelfUpdateProjectionReceipt, error) {
	data, err := readExact(s.evidenceFD, rawSelfUpdateProjectionReceiptName(id))
	if err != nil {
		return RawSelfUpdateProjectionReceipt{}, err
	}
	var receipt RawSelfUpdateProjectionReceipt
	if json.Unmarshal(data, &receipt) != nil || receipt.Schema != MemorySchema || receipt.AttemptID != id || !validDigest(receipt.OutcomeSHA256) || receipt.JournalKey == "" || receipt.ReadBackAtUTC.IsZero() {
		return RawSelfUpdateProjectionReceipt{}, errors.New("maat memory: invalid raw self-update projection receipt")
	}
	return receipt, nil
}

func validateRawSelfUpdateAttempt(attempt RawSelfUpdateAttempt, storeUUID string) error {
	if attempt.Schema != MemorySchema || !validUUID(attempt.ID) || attempt.Manifest.StoreUUID != storeUUID || attempt.Manifest.Validate() != nil ||
		!validDigest(attempt.ActionManifestSHA256) || !validDigest(attempt.PreflightReceiptSHA256) || !validDigest(attempt.RegistrySnapshotSHA256) || attempt.StartedAtUTC.IsZero() {
		return errors.New("maat memory: invalid raw self-update attempt identity")
	}
	digest, err := attempt.Manifest.Digest()
	if err != nil || digest != attempt.ActionManifestSHA256 || attempt.PreflightReceipt.ReceiptSHA256 != attempt.PreflightReceiptSHA256 ||
		attempt.PreflightReceipt.ActionSHA256 != attempt.ActionManifestSHA256 || attempt.PreflightReceipt.RegistrySHA256 != attempt.RegistrySnapshotSHA256 ||
		attempt.PreflightReceipt.Action != rawSelfUpdateScope() || attempt.PreflightReceipt.StoreBinding.State != "initialized" || attempt.PreflightReceipt.StoreBinding.StoreUUID != storeUUID {
		return errors.New("maat memory: raw self-update attempt does not bind its retained receipt")
	}
	return nil
}

func validateRawSelfUpdateOutcome(outcome RawSelfUpdateOutcome, attempt RawSelfUpdateAttempt, invocationRecorded bool) error {
	if outcome.Schema != MemorySchema || outcome.AttemptID != attempt.ID || outcome.RecordedAtUTC.IsZero() || !validDigest(outcome.ObservationSHA256) || len(outcome.Detail) > 4096 {
		return errors.New("maat memory: invalid raw self-update outcome identity")
	}
	decisionTime, err := rawSelfUpdateDecisionTime(outcome.RecordedAtUTC)
	if err != nil || outcome.Decision.Time != decisionTime {
		return errors.New("maat memory: raw self-update decision time does not match its canonical retained outcome time")
	}
	switch outcome.Determination {
	case "not_invoked":
		if invocationRecorded || outcome.UpdaterReturned || outcome.PostcheckPassed ||
			(outcome.FactCode != "prelaunch_action_changed" && outcome.FactCode != "prelaunch_schema_changed" && outcome.FactCode != "prelaunch_target_changed" && outcome.FactCode != "invocation_not_recorded" && outcome.FactCode != "reconcile_no_invocation_marker") {
			return errors.New("maat memory: raw self-update not-invoked fact contradicts its invocation boundary")
		}
	case "resolved", "failed":
		if !invocationRecorded || !outcome.UpdaterReturned || outcome.FactCode != "" || (outcome.Determination == "resolved") != outcome.PostcheckPassed {
			return errors.New("maat memory: raw self-update outcome contradicts its invocation or exact postcheck")
		}
	default:
		return errors.New("maat memory: unsupported raw self-update determination")
	}
	if outcome.Decision.Kind != "diagnostic verified repair" || outcome.Decision.Requester != "sirsi self-update" || !validDigest(outcome.DecisionSHA256) {
		return errors.New("maat memory: raw self-update outcome lacks its exact durable terminal projection intent")
	}
	digest, err := digestJSON(outcome.Decision)
	if err != nil || digest != outcome.DecisionSHA256 {
		return errors.New("maat memory: raw self-update terminal projection digest differs")
	}
	expected, err := rawSelfUpdateOutcomeDecision(attempt, outcome)
	if err != nil || digestEqual(expected, outcome.Decision) != nil {
		return errors.New("maat memory: raw self-update terminal projection does not match the retained factual outcome")
	}
	if outcome.RecordedAtUTC.Before(attempt.StartedAtUTC) || outcome.RecordedAtUTC.After(binaryUpdateNow().Add(10*time.Second)) {
		return errors.New("maat memory: raw self-update outcome time is outside its retained attempt")
	}
	return nil
}

func rawSelfUpdateOutcomeDecision(attempt RawSelfUpdateAttempt, outcome RawSelfUpdateOutcome) (Decision, error) {
	determination := "failed"
	after := "raw self-update invocation did not occur; no target was changed by this attempt"
	if outcome.Determination == "resolved" {
		determination = "resolved"
		after = "raw self-update returned and the exact post-replacement target identity check passed"
	} else if outcome.Determination == "failed" {
		after = "raw self-update returned but the exact post-replacement target identity check failed"
	}
	decision, err := BuildDiagnosticRepairDecision("sirsi self-update", DiagnosticRepair{
		Check: "binary-drift", Operation: "sirsi self-update --confirm; preview sha256=" + attempt.Manifest.PreviewBinding,
		Before: attempt.Manifest.PreviewBinding, After: after, Determination: determination,
		Detail: fmt.Sprintf("attempt=%s determination=%s fact=%s observation_sha256=%s detail=%s", attempt.ID, outcome.Determination, outcome.FactCode, outcome.ObservationSHA256, outcome.Detail),
	})
	if err != nil {
		return Decision{}, err
	}
	decision.Time, err = rawSelfUpdateDecisionTime(outcome.RecordedAtUTC)
	if err != nil {
		return Decision{}, err
	}
	// Host is projection provenance, not an action-manifest field. Preserve the
	// original journal host on readback instead of resolving the current host
	// name again; the first write takes the local host from the canonical builder.
	if outcome.Decision.Host != "" {
		decision.Host = outcome.Decision.Host
	}
	if err := validateDecision(decision); err != nil {
		return Decision{}, err
	}
	return decision, nil
}

func rawSelfUpdateDecisionTime(recordedAt time.Time) (string, error) {
	if recordedAt.IsZero() || recordedAt.Location() != time.UTC {
		return "", errors.New("maat memory: raw self-update outcome time is not canonical UTC")
	}
	canonical := recordedAt.UTC().Format(time.RFC3339Nano)
	parsed, err := time.Parse(time.RFC3339Nano, canonical)
	if err != nil || parsed.Location() != time.UTC || !parsed.Equal(recordedAt) {
		return "", errors.New("maat memory: raw self-update outcome time cannot be represented canonically")
	}
	return canonical, nil
}
