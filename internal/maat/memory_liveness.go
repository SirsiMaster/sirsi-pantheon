package maat

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"golang.org/x/sys/unix"
)

const LivenessWatchManifestSchema = "sirsi.maat.liveness-watch-action.v1"
const LivenessWatchProjectionSchema = "sirsi.maat.liveness-watch-journal-projection.v1"

// LivenessWatchManifest is a closed, typed identity for the single managed
// watch repair. Scope remains the EDA recurrence key; this manifest binds the
// present executable, owner, target, and fresh finding to that same preflight.
type LivenessWatchManifest struct {
	Schema                string    `json:"schema"`
	UID                   uint32    `json:"uid"`
	Domain                string    `json:"domain"`
	ExecutablePath        string    `json:"executable_path"`
	ExecutableOwnerUID    uint32    `json:"executable_owner_uid,omitempty"`
	ExecutableOwnerBound  bool      `json:"executable_owner_bound,omitempty"`
	ExecutableDevice      uint64    `json:"executable_device"`
	ExecutableInode       uint64    `json:"executable_inode"`
	ExecutableMode        uint32    `json:"executable_mode"`
	ExecutableSize        int64     `json:"executable_size"`
	ExecutableSHA256      string    `json:"executable_sha256"`
	WorkDirectory         string    `json:"work_directory"`
	ManagedLabel          string    `json:"managed_label"`
	TargetPlistPath       string    `json:"target_plist_path"`
	ExpectedPlistSHA256   string    `json:"expected_plist_sha256"`
	FindingCheck          string    `json:"finding_check"`
	FindingSeverity       int       `json:"finding_severity"`
	FindingEvidenceSHA256 string    `json:"finding_evidence_sha256"`
	FindingObservedAtUTC  time.Time `json:"finding_observed_at_utc"`
	StoreUUID             string    `json:"store_uuid"`
}

func (m LivenessWatchManifest) Validate() error {
	executableOwnerUID := m.UID // v1 manifests written before owner binding were actor-owned.
	if m.ExecutableOwnerBound {
		executableOwnerUID = m.ExecutableOwnerUID
	}
	if m.Schema != LivenessWatchManifestSchema || m.UID != uint32(os.Geteuid()) || m.Domain != fmt.Sprintf("gui/%d", m.UID) ||
		!filepathIsCanonicalAbsolute(m.ExecutablePath) || m.ExecutableInode == 0 ||
		m.ExecutableMode&uint32(unix.S_IFMT) != uint32(unix.S_IFREG) ||
		!LivenessExecutableIdentityAllowed(m.UID, executableOwnerUID, m.ExecutableMode) ||
		m.ExecutableSize <= 0 || !validDigest(m.ExecutableSHA256) ||
		!filepathIsCanonicalAbsolute(m.WorkDirectory) || m.ManagedLabel != "ai.sirsi.liveness-watch" ||
		!filepathIsCanonicalAbsolute(m.TargetPlistPath) || !validDigest(m.ExpectedPlistSHA256) ||
		m.FindingCheck != "liveness-watch" || m.FindingSeverity < 2 || !validDigest(m.FindingEvidenceSHA256) ||
		m.FindingObservedAtUTC.IsZero() || !validUUID(m.StoreUUID) {
		return errors.New("maat memory: invalid liveness-watch action manifest")
	}
	return nil
}

// LivenessExecutableIdentityAllowed accepts an actor-owned executable or the
// root-owned executable used by installed app bundles. Group/other writable
// and set-ID files are never accepted; root-owned files need not be writable by
// the unprivileged actor to be launchable from that actor's GUI session.
func LivenessExecutableIdentityAllowed(actorUID, ownerUID, mode uint32) bool {
	return (ownerUID == actorUID || ownerUID == 0) && mode&0o111 != 0 &&
		mode&0o022 == 0 && mode&uint32(unix.S_ISUID|unix.S_ISGID) == 0
}

func (m LivenessWatchManifest) ValidateFresh() error {
	if err := m.Validate(); err != nil {
		return err
	}
	if time.Since(m.FindingObservedAtUTC) > 2*time.Minute || time.Until(m.FindingObservedAtUTC) > 10*time.Second {
		return errors.New("maat memory: liveness-watch finding is not fresh")
	}
	return nil
}

func (m LivenessWatchManifest) Digest() (string, error) {
	if err := m.Validate(); err != nil {
		return "", err
	}
	data, err := json.Marshal(m)
	if err != nil {
		return "", fmt.Errorf("maat memory: encode liveness-watch manifest: %w", err)
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

// PreflightLivenessWatch retains exact-scope EDA matching while hashing the
// entire fixed action identity, rather than Scope alone.
func (s *Store) PreflightLivenessWatch(manifest LivenessWatchManifest) (PreflightReceipt, error) {
	if s == nil || s.storeUUID == "" || manifest.StoreUUID != s.storeUUID {
		return PreflightReceipt{}, errors.New("maat memory: action is not bound to this initialized store")
	}
	if err := manifest.ValidateFresh(); err != nil {
		return PreflightReceipt{}, err
	}
	digest, err := manifest.Digest()
	if err != nil {
		return PreflightReceipt{}, err
	}
	scope := Scope{Component: "pantheon", Profile: "local-macos", Operation: "install-managed-liveness-watch"}
	return s.preflightBound(scope, digest, resolveMemoryGuard)
}

type LivenessWatchAttempt struct {
	Schema                 string                `json:"schema"`
	ID                     string                `json:"id"`
	Manifest               LivenessWatchManifest `json:"manifest"`
	ActionManifestSHA256   string                `json:"action_manifest_sha256"`
	PreflightReceiptSHA256 string                `json:"preflight_receipt_sha256"`
	RegistrySnapshotSHA256 string                `json:"registry_snapshot_sha256"`
	StartedAtUTC           time.Time             `json:"started_at_utc"`
}

type LivenessWatchObservation struct {
	UID           uint32 `json:"uid"`
	Domain        string `json:"domain"`
	Label         string `json:"label"`
	PlistPath     string `json:"plist_path"`
	PlistSHA256   string `json:"plist_sha256"`
	LaunchdLoaded bool   `json:"launchd_loaded"`
}

type LivenessWatchOutcome struct {
	Schema         string                   `json:"schema"`
	AttemptID      string                   `json:"attempt_id"`
	RecordedAtUTC  time.Time                `json:"recorded_at_utc"`
	Determination  string                   `json:"determination"`
	InstallerError string                   `json:"installer_error,omitempty"`
	Observation    LivenessWatchObservation `json:"observation"`
}

// LivenessWatchProjectionDebt retains the exact terminal result and Casebook
// decision before the append. A missing receipt means the projection still
// requires readback; it never authorizes rerunning the installer.
type LivenessWatchProjectionDebt struct {
	Schema         string               `json:"schema"`
	Attempt        LivenessWatchAttempt `json:"attempt"`
	AttemptSHA256  string               `json:"attempt_sha256"`
	Outcome        LivenessWatchOutcome `json:"outcome"`
	OutcomeSHA256  string               `json:"outcome_sha256"`
	Decision       Decision             `json:"decision"`
	DecisionSHA256 string               `json:"decision_sha256"`
	CreatedAtUTC   time.Time            `json:"created_at_utc"`
}

type LivenessWatchProjectionReceipt struct {
	Schema        string    `json:"schema"`
	AttemptID     string    `json:"attempt_id"`
	DebtSHA256    string    `json:"debt_sha256"`
	JournalKey    string    `json:"journal_key"`
	ReadBackAtUTC time.Time `json:"read_back_at_utc"`
}

func (s *Store) BeginLivenessWatchAttempt(manifest LivenessWatchManifest, receipt PreflightReceipt) (LivenessWatchAttempt, error) {
	if s == nil || s.attemptsFD < 0 || receipt.Decision != PreflightPass {
		return LivenessWatchAttempt{}, errors.New("maat memory: liveness-watch attempt requires a passing initialized-store preflight")
	}
	digest, err := manifest.Digest()
	if err != nil || manifest.StoreUUID != s.storeUUID || receipt.ActionManifestSHA256 != digest {
		return LivenessWatchAttempt{}, errors.New("maat memory: preflight does not bind this liveness-watch action")
	}
	receiptDigest, err := receipt.Digest()
	if err != nil {
		return LivenessWatchAttempt{}, err
	}
	if err := s.lock(unix.LOCK_EX); err != nil {
		return LivenessWatchAttempt{}, err
	}
	defer s.unlock()
	currentReceipt, err := s.preflightBoundLocked(
		Scope{Component: "pantheon", Profile: "local-macos", Operation: "install-managed-liveness-watch"}, digest, resolveMemoryGuard,
	)
	if err != nil || currentReceipt.Decision != PreflightPass || currentReceipt.Action != receipt.Action ||
		currentReceipt.ActionManifestSHA256 != receipt.ActionManifestSHA256 || currentReceipt.RegistrySnapshotSHA256 != receipt.RegistrySnapshotSHA256 {
		return LivenessWatchAttempt{}, errors.New("maat memory: reviewed preflight is stale, rejected, or unverifiable at attempt admission; fresh review and confirmation are required")
	}
	if _, err := s.pendingLivenessWatchAttemptLocked(); err == nil {
		return LivenessWatchAttempt{}, errors.New("maat memory: an incomplete liveness-watch attempt must be inspected and re-observed before a new consent")
	} else if !errors.Is(err, ErrNoIncompleteLivenessWatchAttempt) {
		return LivenessWatchAttempt{}, err
	}
	if debts, err := s.pendingLivenessWatchProjectionsLocked(); err != nil {
		return LivenessWatchAttempt{}, err
	} else if len(debts) != 0 {
		return LivenessWatchAttempt{}, errors.New("maat memory: an unprojected liveness-watch outcome must be reconciled before new consent")
	}
	id, err := newStoreUUID()
	if err != nil {
		return LivenessWatchAttempt{}, err
	}
	attempt := LivenessWatchAttempt{
		Schema: MemorySchema, ID: id, Manifest: manifest, ActionManifestSHA256: digest,
		PreflightReceiptSHA256: receiptDigest, RegistrySnapshotSHA256: receipt.RegistrySnapshotSHA256,
		StartedAtUTC: time.Now().UTC(),
	}
	data, err := json.Marshal(attempt)
	if err != nil {
		return LivenessWatchAttempt{}, err
	}
	if err := s.createOrVerify("attempts", s.attemptsFD, id+".started.json", data); err != nil {
		return LivenessWatchAttempt{}, fmt.Errorf("maat memory: persist durable liveness-watch attempt start: %w", err)
	}
	return attempt, nil
}

var ErrNoIncompleteLivenessWatchAttempt = errors.New("maat memory: no incomplete liveness-watch attempt")

func (s *Store) PendingLivenessWatchAttempt() (LivenessWatchAttempt, error) {
	if err := s.lock(unix.LOCK_SH); err != nil {
		return LivenessWatchAttempt{}, err
	}
	defer s.unlock()
	return s.pendingLivenessWatchAttemptLocked()
}

func (s *Store) pendingLivenessWatchAttemptLocked() (LivenessWatchAttempt, error) {
	if _, err := s.verifyWriteJournal(); err != nil {
		return LivenessWatchAttempt{}, fmt.Errorf("maat memory: attempt history is unverifiable: %w", err)
	}
	names, err := readDirectoryNames(s.attemptsFD)
	if err != nil {
		return LivenessWatchAttempt{}, err
	}
	started := map[string]LivenessWatchAttempt{}
	outcomes := map[string]LivenessWatchOutcome{}
	for _, name := range names {
		switch {
		case strings.HasSuffix(name, ".started.json"):
			id := strings.TrimSuffix(name, ".started.json")
			if !validUUID(id) {
				return LivenessWatchAttempt{}, errors.New("maat memory: malformed liveness-watch attempt namespace")
			}
			data, err := readExact(s.attemptsFD, name)
			if err != nil {
				return LivenessWatchAttempt{}, err
			}
			var attempt LivenessWatchAttempt
			if err := json.Unmarshal(data, &attempt); err != nil || validateLivenessAttempt(attempt, id, s.storeUUID) != nil {
				return LivenessWatchAttempt{}, errors.New("maat memory: invalid liveness-watch attempt record")
			}
			started[id] = attempt
		case strings.HasSuffix(name, ".outcome.json"):
			id := strings.TrimSuffix(name, ".outcome.json")
			if !validUUID(id) {
				return LivenessWatchAttempt{}, errors.New("maat memory: malformed liveness-watch outcome namespace")
			}
			data, err := readExact(s.attemptsFD, name)
			if err != nil {
				return LivenessWatchAttempt{}, err
			}
			var outcome LivenessWatchOutcome
			if err := json.Unmarshal(data, &outcome); err != nil || outcome.Schema != MemorySchema || outcome.AttemptID != id || outcome.RecordedAtUTC.IsZero() || (outcome.Determination != "resolved" && outcome.Determination != "incomplete") {
				return LivenessWatchAttempt{}, errors.New("maat memory: invalid liveness-watch outcome record")
			}
			outcomes[id] = outcome
		case strings.HasSuffix(name, ".projection.json"), strings.HasSuffix(name, ".projection-receipt.json"):
			// Projection records are validated by the projection reader below.
		default:
			return LivenessWatchAttempt{}, errors.New("maat memory: unrecognized liveness-watch attempt record")
		}
	}
	var pending *LivenessWatchAttempt
	for id, attempt := range started {
		if outcome, ok := outcomes[id]; ok {
			if err := validateLivenessOutcome(outcome, attempt); err != nil {
				return LivenessWatchAttempt{}, err
			}
			continue
		}
		if _, ok := started[id]; !ok {
			return LivenessWatchAttempt{}, errors.New("maat memory: outcome has no matching attempt")
		}
		if pending != nil {
			return LivenessWatchAttempt{}, errors.New("maat memory: multiple incomplete liveness-watch attempts require review")
		}
		pending = &attempt
	}
	for id := range outcomes {
		if _, ok := started[id]; !ok {
			return LivenessWatchAttempt{}, errors.New("maat memory: outcome has no matching attempt")
		}
	}
	if pending == nil {
		return LivenessWatchAttempt{}, ErrNoIncompleteLivenessWatchAttempt
	}
	return *pending, nil
}

func NewLivenessWatchOutcome(attempt LivenessWatchAttempt, observation LivenessWatchObservation, installerError string) (LivenessWatchOutcome, error) {
	if len(installerError) > 4096 {
		installerError = installerError[:4096]
	}
	determination := "incomplete"
	if installerError == "" && observation.UID == attempt.Manifest.UID && observation.Domain == attempt.Manifest.Domain &&
		observation.Label == attempt.Manifest.ManagedLabel && observation.PlistPath == attempt.Manifest.TargetPlistPath &&
		observation.PlistSHA256 == attempt.Manifest.ExpectedPlistSHA256 && observation.LaunchdLoaded {
		determination = "resolved"
	}
	outcome := LivenessWatchOutcome{Schema: MemorySchema, AttemptID: attempt.ID, RecordedAtUTC: time.Now().UTC(), Determination: determination, InstallerError: installerError, Observation: observation}
	if err := validateLivenessOutcome(outcome, attempt); err != nil {
		return LivenessWatchOutcome{}, err
	}
	return outcome, nil
}

func (s *Store) BeginLivenessWatchProjection(attempt LivenessWatchAttempt, outcome LivenessWatchOutcome, decision Decision) (LivenessWatchProjectionDebt, error) {
	if s == nil || s.attemptsFD < 0 || validateLivenessAttempt(attempt, attempt.ID, s.storeUUID) != nil || validateLivenessOutcome(outcome, attempt) != nil {
		return LivenessWatchProjectionDebt{}, errors.New("maat memory: invalid liveness-watch projection identity")
	}
	if err := validateDecision(decision); err != nil || decision.Kind != "diagnostic verified repair" || decision.Resource != "liveness-watch" ||
		!strings.HasPrefix(decision.Evidence, "maat-liveness-watch-attempt:"+attempt.ID+":") ||
		!validLivenessProjectionDetermination(decision.Determination, outcome.Determination) {
		return LivenessWatchProjectionDebt{}, errors.New("maat memory: projection decision does not bind the exact attempt outcome")
	}
	if err := s.lock(unix.LOCK_EX); err != nil {
		return LivenessWatchProjectionDebt{}, err
	}
	defer s.unlock()
	pending, err := s.pendingLivenessWatchAttemptLocked()
	if err != nil || pending.ID != attempt.ID {
		return LivenessWatchProjectionDebt{}, errors.New("maat memory: projection debt requires the exact incomplete attempt")
	}
	if debts, err := s.pendingLivenessWatchProjectionsLocked(); err != nil || len(debts) != 0 {
		return LivenessWatchProjectionDebt{}, errors.New("maat memory: another liveness-watch projection requires reconciliation")
	}
	// Reuse the durable outcome timestamp so a retry after an uncertain create
	// reproduces the exact same projection intent bytes.
	debt := LivenessWatchProjectionDebt{Schema: LivenessWatchProjectionSchema, Attempt: attempt, Outcome: outcome, Decision: decision, CreatedAtUTC: outcome.RecordedAtUTC}
	var errAttempt, errOutcome, errDecision error
	debt.AttemptSHA256, errAttempt = digestJSON(attempt)
	debt.OutcomeSHA256, errOutcome = digestJSON(outcome)
	debt.DecisionSHA256, errDecision = digestJSON(decision)
	if errAttempt != nil || errOutcome != nil || errDecision != nil {
		return LivenessWatchProjectionDebt{}, errors.New("maat memory: encode liveness-watch projection debt")
	}
	data, err := json.Marshal(debt)
	if err != nil {
		return LivenessWatchProjectionDebt{}, err
	}
	if err := s.createOrVerify("attempts", s.attemptsFD, attempt.ID+".projection.json", data); err != nil {
		return LivenessWatchProjectionDebt{}, fmt.Errorf("maat memory: retain liveness-watch projection debt: %w", err)
	}
	return debt, nil
}

func (s *Store) PersistLivenessWatchProjectionOutcome(debt LivenessWatchProjectionDebt) error {
	if s == nil || s.attemptsFD < 0 {
		return errors.New("maat memory: store is closed")
	}
	if err := s.lock(unix.LOCK_EX); err != nil {
		return err
	}
	defer s.unlock()
	stored, err := s.readLivenessWatchProjectionLocked(debt.Attempt.ID)
	if err != nil || !sameProjectionDebt(stored, debt) {
		return errors.New("maat memory: liveness-watch projection debt is missing or changed")
	}
	data, err := json.Marshal(debt.Outcome)
	if err != nil {
		return err
	}
	return s.createOrVerify("attempts", s.attemptsFD, debt.Attempt.ID+".outcome.json", data)
}

func (s *Store) PendingLivenessWatchProjectionDebts() ([]LivenessWatchProjectionDebt, error) {
	if err := s.lock(unix.LOCK_SH); err != nil {
		return nil, err
	}
	defer s.unlock()
	return s.pendingLivenessWatchProjectionsLocked()
}

func (s *Store) pendingLivenessWatchProjectionsLocked() ([]LivenessWatchProjectionDebt, error) {
	if _, err := s.verifyWriteJournal(); err != nil {
		return nil, fmt.Errorf("maat memory: projection history is unverifiable: %w", err)
	}
	names, err := readDirectoryNames(s.attemptsFD)
	if err != nil {
		return nil, err
	}
	debts := make(map[string]LivenessWatchProjectionDebt)
	receipts := make(map[string]LivenessWatchProjectionReceipt)
	for _, name := range names {
		switch {
		case strings.HasSuffix(name, ".projection.json"):
			id := strings.TrimSuffix(name, ".projection.json")
			if !validUUID(id) {
				return nil, errors.New("maat memory: malformed projection namespace")
			}
			data, err := readExact(s.attemptsFD, name)
			if err != nil {
				return nil, err
			}
			var debt LivenessWatchProjectionDebt
			if err := json.Unmarshal(data, &debt); err != nil || debt.Attempt.ID != id || validateLivenessProjectionDebt(debt, s.storeUUID) != nil {
				return nil, errors.New("maat memory: invalid liveness-watch projection debt")
			}
			attemptBytes, err := readExact(s.attemptsFD, id+".started.json")
			if err != nil {
				return nil, errors.New("maat memory: projection debt has no durable attempt")
			}
			var attempt LivenessWatchAttempt
			if err := json.Unmarshal(attemptBytes, &attempt); err != nil || digestEqual(attempt, debt.Attempt) != nil {
				return nil, errors.New("maat memory: projection debt attempt readback differs")
			}
			if outcomeBytes, outcomeErr := readExact(s.attemptsFD, id+".outcome.json"); outcomeErr == nil {
				var outcome LivenessWatchOutcome
				if json.Unmarshal(outcomeBytes, &outcome) != nil || digestEqual(outcome, debt.Outcome) != nil {
					return nil, errors.New("maat memory: projection debt outcome readback differs")
				}
			} else if !errors.Is(outcomeErr, unix.ENOENT) {
				return nil, outcomeErr
			}
			debts[id] = debt
		case strings.HasSuffix(name, ".projection-receipt.json"):
			id := strings.TrimSuffix(name, ".projection-receipt.json")
			if !validUUID(id) {
				return nil, errors.New("maat memory: malformed projection receipt namespace")
			}
			data, err := readExact(s.attemptsFD, name)
			if err != nil {
				return nil, err
			}
			var receipt LivenessWatchProjectionReceipt
			if err := json.Unmarshal(data, &receipt); err != nil || receipt.Schema != LivenessWatchProjectionSchema || receipt.AttemptID != id || !validDigest(receipt.DebtSHA256) || receipt.JournalKey == "" || receipt.ReadBackAtUTC.IsZero() {
				return nil, errors.New("maat memory: invalid liveness-watch projection receipt")
			}
			receipts[id] = receipt
		}
	}
	for id, receipt := range receipts {
		debt, ok := debts[id]
		if !ok {
			return nil, errors.New("maat memory: projection receipt has no matching debt")
		}
		outcomeBytes, err := readExact(s.attemptsFD, id+".outcome.json")
		if err != nil {
			return nil, errors.New("maat memory: completed projection receipt has no durable outcome")
		}
		var outcome LivenessWatchOutcome
		if json.Unmarshal(outcomeBytes, &outcome) != nil || digestEqual(outcome, debt.Outcome) != nil {
			return nil, errors.New("maat memory: completed projection outcome differs from its debt")
		}
		digest, err := digestJSON(debt)
		if err != nil || digest != receipt.DebtSHA256 || receipt.JournalKey != debt.Decision.Evidence {
			return nil, errors.New("maat memory: projection receipt does not bind the exact debt")
		}
		delete(debts, id)
	}
	result := make([]LivenessWatchProjectionDebt, 0, len(debts))
	for _, debt := range debts {
		result = append(result, debt)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Attempt.ID < result[j].Attempt.ID })
	return result, nil
}

func (s *Store) MarkLivenessWatchProjectionReadBack(debt LivenessWatchProjectionDebt) error {
	if s == nil || s.attemptsFD < 0 {
		return errors.New("maat memory: store is closed")
	}
	if err := s.lock(unix.LOCK_EX); err != nil {
		return err
	}
	defer s.unlock()
	stored, err := s.readLivenessWatchProjectionLocked(debt.Attempt.ID)
	if err != nil || !sameProjectionDebt(stored, debt) {
		return errors.New("maat memory: liveness-watch projection debt is missing or changed")
	}
	outcomeBytes, err := readExact(s.attemptsFD, debt.Attempt.ID+".outcome.json")
	if err != nil {
		return errors.New("maat memory: projection outcome is not durably recorded")
	}
	var outcome LivenessWatchOutcome
	if json.Unmarshal(outcomeBytes, &outcome) != nil || digestEqual(outcome, debt.Outcome) != nil {
		return errors.New("maat memory: projection outcome readback differs")
	}
	receipt := LivenessWatchProjectionReceipt{Schema: LivenessWatchProjectionSchema, AttemptID: debt.Attempt.ID, JournalKey: debt.Decision.Evidence, ReadBackAtUTC: time.Now().UTC()}
	receipt.DebtSHA256, err = digestJSON(debt)
	if err != nil {
		return err
	}
	name := debt.Attempt.ID + ".projection-receipt.json"
	if existing, err := readExact(s.attemptsFD, name); err == nil {
		var prior LivenessWatchProjectionReceipt
		if json.Unmarshal(existing, &prior) != nil || prior.Schema != receipt.Schema || prior.AttemptID != receipt.AttemptID || prior.DebtSHA256 != receipt.DebtSHA256 || prior.JournalKey != receipt.JournalKey || prior.ReadBackAtUTC.IsZero() {
			return errors.New("maat memory: existing projection receipt differs")
		}
		existing, err = json.Marshal(prior)
		if err != nil {
			return err
		}
		return s.createOrVerify("attempts", s.attemptsFD, name, existing)
	} else if !errors.Is(err, unix.ENOENT) {
		return err
	}
	data, err := json.Marshal(receipt)
	if err != nil {
		return err
	}
	return s.createOrVerify("attempts", s.attemptsFD, name, data)
}

func (s *Store) readLivenessWatchProjectionLocked(id string) (LivenessWatchProjectionDebt, error) {
	if !validUUID(id) {
		return LivenessWatchProjectionDebt{}, errors.New("maat memory: invalid projection attempt ID")
	}
	data, err := readExact(s.attemptsFD, id+".projection.json")
	if err != nil {
		return LivenessWatchProjectionDebt{}, err
	}
	var debt LivenessWatchProjectionDebt
	if err := json.Unmarshal(data, &debt); err != nil || validateLivenessProjectionDebt(debt, s.storeUUID) != nil {
		return LivenessWatchProjectionDebt{}, errors.New("maat memory: invalid liveness-watch projection debt")
	}
	attemptBytes, err := readExact(s.attemptsFD, id+".started.json")
	if err != nil {
		return LivenessWatchProjectionDebt{}, errors.New("maat memory: projection debt has no durable attempt")
	}
	var attempt LivenessWatchAttempt
	if json.Unmarshal(attemptBytes, &attempt) != nil || digestEqual(attempt, debt.Attempt) != nil {
		return LivenessWatchProjectionDebt{}, errors.New("maat memory: projection debt attempt readback differs")
	}
	return debt, nil
}

func validateLivenessProjectionDebt(debt LivenessWatchProjectionDebt, storeUUID string) error {
	if debt.Schema != LivenessWatchProjectionSchema || debt.Attempt.Manifest.StoreUUID != storeUUID ||
		validateLivenessAttempt(debt.Attempt, debt.Attempt.ID, storeUUID) != nil || validateLivenessOutcome(debt.Outcome, debt.Attempt) != nil ||
		validateDecision(debt.Decision) != nil || debt.Decision.Kind != "diagnostic verified repair" || debt.Decision.Resource != "liveness-watch" ||
		!strings.HasPrefix(debt.Decision.Evidence, "maat-liveness-watch-attempt:"+debt.Attempt.ID+":") ||
		!validLivenessProjectionDetermination(debt.Decision.Determination, debt.Outcome.Determination) || debt.CreatedAtUTC.IsZero() {
		return errors.New("maat memory: invalid liveness-watch projection debt")
	}
	attemptDigest, errA := digestJSON(debt.Attempt)
	outcomeDigest, errO := digestJSON(debt.Outcome)
	decisionDigest, errD := digestJSON(debt.Decision)
	if errA != nil || errO != nil || errD != nil || attemptDigest != debt.AttemptSHA256 || outcomeDigest != debt.OutcomeSHA256 || decisionDigest != debt.DecisionSHA256 {
		return errors.New("maat memory: liveness-watch projection debt digest mismatch")
	}
	return nil
}

func validLivenessProjectionDetermination(decision, outcome string) bool {
	return (decision == "resolved" && outcome == "resolved") ||
		(decision == "failed" && (outcome == "resolved" || outcome == "incomplete"))
}

func sameProjectionDebt(a, b LivenessWatchProjectionDebt) bool {
	aBytes, errA := json.Marshal(a)
	bBytes, errB := json.Marshal(b)
	return errA == nil && errB == nil && string(aBytes) == string(bBytes)
}

func digestJSON(value any) (string, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

func digestEqual(a, b any) error {
	aDigest, errA := digestJSON(a)
	bDigest, errB := digestJSON(b)
	if errA != nil || errB != nil || aDigest != bDigest {
		return errors.New("digest mismatch")
	}
	return nil
}

func validateLivenessAttempt(attempt LivenessWatchAttempt, id, storeUUID string) error {
	digest, err := attempt.Manifest.Digest()
	if err != nil || attempt.Schema != MemorySchema || attempt.ID != id || attempt.Manifest.StoreUUID != storeUUID ||
		!validDigest(digest) || digest != attempt.ActionManifestSHA256 || !validDigest(attempt.PreflightReceiptSHA256) ||
		!validDigest(attempt.RegistrySnapshotSHA256) || attempt.StartedAtUTC.IsZero() {
		return errors.New("maat memory: invalid liveness-watch attempt identity")
	}
	return nil
}

func validateLivenessOutcome(outcome LivenessWatchOutcome, attempt LivenessWatchAttempt) error {
	if outcome.Schema != MemorySchema || outcome.AttemptID != attempt.ID || outcome.RecordedAtUTC.IsZero() ||
		(outcome.Determination != "resolved" && outcome.Determination != "incomplete") {
		return errors.New("maat memory: invalid liveness-watch outcome identity")
	}
	exact := outcome.Observation.UID == attempt.Manifest.UID && outcome.Observation.Domain == attempt.Manifest.Domain &&
		outcome.Observation.Label == attempt.Manifest.ManagedLabel && outcome.Observation.PlistPath == attempt.Manifest.TargetPlistPath &&
		outcome.Observation.PlistSHA256 == attempt.Manifest.ExpectedPlistSHA256 && outcome.Observation.LaunchdLoaded
	if (outcome.Determination == "resolved") != (exact && outcome.InstallerError == "") {
		return errors.New("maat memory: liveness-watch outcome does not match exact observation")
	}
	return nil
}

// RecordLivenessWatchOutcome never accepts a success claim without the exact
// expected per-user target and loaded managed label observed after installation.
func (s *Store) RecordLivenessWatchOutcome(attempt LivenessWatchAttempt, observation LivenessWatchObservation, installerErr string) (LivenessWatchOutcome, error) {
	if s == nil || s.attemptsFD < 0 || !validUUID(attempt.ID) || validateLivenessAttempt(attempt, attempt.ID, s.storeUUID) != nil {
		return LivenessWatchOutcome{}, errors.New("maat memory: invalid liveness-watch outcome reference")
	}
	if err := s.lock(unix.LOCK_EX); err != nil {
		return LivenessWatchOutcome{}, err
	}
	defer s.unlock()
	pending, err := s.pendingLivenessWatchAttemptLocked()
	if err != nil || pending != attempt {
		return LivenessWatchOutcome{}, errors.New("maat memory: liveness-watch attempt is not the unique incomplete action")
	}
	determination := "incomplete"
	if installerErr == "" && observation.UID == pending.Manifest.UID && observation.Domain == pending.Manifest.Domain &&
		observation.Label == pending.Manifest.ManagedLabel && observation.PlistPath == pending.Manifest.TargetPlistPath &&
		observation.PlistSHA256 == pending.Manifest.ExpectedPlistSHA256 && observation.LaunchdLoaded {
		determination = "resolved"
	}
	if len(installerErr) > 4096 {
		installerErr = installerErr[:4096]
	}
	outcome := LivenessWatchOutcome{Schema: MemorySchema, AttemptID: attempt.ID, RecordedAtUTC: time.Now().UTC(), Determination: determination, InstallerError: installerErr, Observation: observation}
	data, err := json.Marshal(outcome)
	if err != nil {
		return LivenessWatchOutcome{}, err
	}
	if err := s.createOrVerify("attempts", s.attemptsFD, attempt.ID+".outcome.json", data); err != nil {
		return LivenessWatchOutcome{}, fmt.Errorf("maat memory: durable liveness-watch outcome unavailable; operation remains incomplete: %w", err)
	}
	return outcome, nil
}

func filepathIsCanonicalAbsolute(value string) bool {
	return filepath.IsAbs(value) && filepath.Clean(value) == value && !strings.ContainsRune(value, 0)
}
