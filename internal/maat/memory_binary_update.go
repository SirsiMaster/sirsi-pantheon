package maat

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

	internalversion "github.com/SirsiMaster/sirsi-pantheon/internal/version"
	"golang.org/x/sys/unix"
)

const BinaryUpdateManifestSchema = "sirsi.maat.binary-update-action.v2"
const BinaryUpdateProjectionSchema = "sirsi.maat.binary-update-journal-projection.v1"
const BinaryUpdateInvocationProtocol = "store-recorded-before-updater-v1"

var binaryUpdateNow = time.Now

func binaryUpdateBoundDetail(value string) string {
	value = strings.ToValidUTF8(value, "�")
	if len(value) > 4096 {
		value = value[:4096]
	}
	return value
}

// BinaryUpdateTarget binds one already-reviewed Homebrew-managed target.
type BinaryUpdateTarget struct {
	Path     string `json:"path"`
	Method   string `json:"method"`
	Version  string `json:"version"`
	SHA256   string `json:"sha256"`
	OwnerUID uint32 `json:"owner_uid"`
	Device   uint64 `json:"device"`
	Inode    uint64 `json:"inode"`
	Mode     uint32 `json:"mode"`
}

// BinaryUpdateManifest is the closed action identity for the existing
// binary-drift updater. It carries no executable command text from evidence.
type BinaryUpdateManifest struct {
	Schema                   string               `json:"schema"`
	CaseEvidence             string               `json:"case_evidence"`
	PreviewBinding           string               `json:"preview_binding"`
	Updater                  string               `json:"updater"`
	UpdaterPath              string               `json:"updater_path"`
	UpdaterSHA256            string               `json:"updater_sha256"`
	UpdaterOwnerUID          uint32               `json:"updater_owner_uid"`
	UpdaterDevice            uint64               `json:"updater_device"`
	UpdaterInode             uint64               `json:"updater_inode"`
	UpdaterMode              uint32               `json:"updater_mode"`
	SelfPath                 string               `json:"self_path"`
	SelfVersion              string               `json:"self_version"`
	SelfSHA256               string               `json:"self_sha256"`
	RunningInfo              internalversion.Info `json:"running_info"`
	SelfOwnerUID             uint32               `json:"self_owner_uid"`
	SelfDevice               uint64               `json:"self_device"`
	SelfInode                uint64               `json:"self_inode"`
	SelfMode                 uint32               `json:"self_mode"`
	Targets                  []BinaryUpdateTarget `json:"targets"`
	AuthorizedUID            uint32               `json:"authorized_uid"`
	ObservedSchemaCompatible bool                 `json:"observed_schema_compatible"`
	ObservedSchemaVersion    int                  `json:"observed_schema_version"`
	StoreUUID                string               `json:"store_uuid"`
	ObservedAtUTC            time.Time            `json:"observed_at_utc"`
}

func (m BinaryUpdateManifest) Validate() error {
	if m.Schema != BinaryUpdateManifestSchema || m.AuthorizedUID != uint32(os.Geteuid()) ||
		!validDigest(m.PreviewBinding) || !strings.HasPrefix(m.CaseEvidence, "maat-system-one:sha256=") ||
		!validDigest(strings.TrimPrefix(m.CaseEvidence, "maat-system-one:sha256=")) ||
		m.Updater != "brew upgrade sirsi-pantheon" || !filepathIsCanonicalAbsolute(m.UpdaterPath) || !validDigest(m.UpdaterSHA256) || m.UpdaterInode == 0 ||
		m.UpdaterMode&uint32(unix.S_IFMT) != uint32(unix.S_IFREG) || !validOwnerMode(m.AuthorizedUID, m.UpdaterOwnerUID, m.UpdaterMode) ||
		!filepathIsCanonicalAbsolute(m.SelfPath) || strings.TrimSpace(m.SelfVersion) == "" || m.SelfVersion == "dev" || !validDigest(m.SelfSHA256) ||
		m.RunningInfo.Binary != "sirsi" || m.RunningInfo.Path != m.SelfPath || m.RunningInfo.Version != m.SelfVersion || m.RunningInfo.RouterSchemaMax <= 0 ||
		m.SelfInode == 0 || m.SelfMode&uint32(unix.S_IFMT) != uint32(unix.S_IFREG) || !validOwnerMode(m.AuthorizedUID, m.SelfOwnerUID, m.SelfMode) ||
		!m.ObservedSchemaCompatible || m.ObservedSchemaVersion < 0 || !validUUID(m.StoreUUID) || m.ObservedAtUTC.IsZero() ||
		len(m.Targets) == 0 || len(m.Targets) > 32 {
		return errors.New("maat memory: invalid binary-update action manifest")
	}
	last := ""
	for _, target := range m.Targets {
		if !filepathIsCanonicalAbsolute(target.Path) || target.Path <= last || target.Method != "homebrew" || strings.TrimSpace(target.Version) == "" ||
			!validDigest(target.SHA256) || target.Inode == 0 || target.Mode&uint32(unix.S_IFMT) != uint32(unix.S_IFREG) ||
			!validOwnerMode(m.AuthorizedUID, target.OwnerUID, target.Mode) {
			return errors.New("maat memory: invalid or unordered binary-update target")
		}
		last = target.Path
	}
	return nil
}

// ValidateFresh is admission-only. Validate and Digest stay structural so
// persisted attempts remain valid historical facts after the review window.
func (m BinaryUpdateManifest) ValidateFresh() error {
	if err := m.Validate(); err != nil {
		return err
	}
	now := binaryUpdateNow()
	if now.Sub(m.ObservedAtUTC) > 2*time.Minute || m.ObservedAtUTC.Sub(now) > 10*time.Second {
		return errors.New("maat memory: binary-update manifest review is outside its admission window")
	}
	return nil
}

func validOwnerMode(actor, owner, mode uint32) bool {
	return (owner == actor || owner == 0) && mode&0o111 != 0 && mode&0o022 == 0 && mode&uint32(unix.S_ISUID|unix.S_ISGID) == 0
}

func (m BinaryUpdateManifest) Digest() (string, error) {
	if err := m.Validate(); err != nil {
		return "", err
	}
	b, err := json.Marshal(m)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:]), nil
}

func binaryUpdateScope() Scope {
	return Scope{Component: "pantheon", Profile: "local-macos", Operation: "update-homebrew-binary-drift"}
}

func (s *Store) PreflightBinaryUpdate(m BinaryUpdateManifest) (PreflightReceipt, error) {
	if s == nil || s.storeUUID == "" || m.StoreUUID != s.storeUUID {
		return PreflightReceipt{}, errors.New("maat memory: binary-update action is not bound to this initialized store")
	}
	if err := m.ValidateFresh(); err != nil {
		return PreflightReceipt{}, err
	}
	digest, err := m.Digest()
	if err != nil {
		return PreflightReceipt{}, err
	}
	return s.preflightBound(binaryUpdateScope(), digest, resolveMemoryGuard)
}

type BinaryUpdateAttempt struct {
	Schema                 string               `json:"schema"`
	ID                     string               `json:"id"`
	Manifest               BinaryUpdateManifest `json:"manifest"`
	ActionManifestSHA256   string               `json:"action_manifest_sha256"`
	PreflightReceiptSHA256 string               `json:"preflight_receipt_sha256"`
	RegistrySnapshotSHA256 string               `json:"registry_snapshot_sha256"`
	LaunchClaimEvidence    string               `json:"launch_claim_evidence"`
	InvocationProtocol     string               `json:"invocation_protocol,omitempty"`
	StartedAtUTC           time.Time            `json:"started_at_utc"`
}

type BinaryUpdateOutcome struct {
	Schema            string    `json:"schema"`
	AttemptID         string    `json:"attempt_id"`
	RecordedAtUTC     time.Time `json:"recorded_at_utc"`
	Determination     string    `json:"determination"`
	UpdaterReturned   bool      `json:"updater_returned"`
	PostcheckPassed   bool      `json:"postcheck_passed"`
	ObservationSHA256 string    `json:"observation_sha256"`
	FactCode          string    `json:"fact_code,omitempty"`
	Detail            string    `json:"detail"`
}

type BinaryUpdateInvocation struct {
	Schema          string    `json:"schema"`
	AttemptID       string    `json:"attempt_id"`
	AttemptSHA256   string    `json:"attempt_sha256"`
	AuthorizedAtUTC time.Time `json:"authorized_at_utc"`
}

type BinaryUpdateProjectionDebt struct {
	Schema         string              `json:"schema"`
	Attempt        BinaryUpdateAttempt `json:"attempt"`
	AttemptSHA256  string              `json:"attempt_sha256"`
	Outcome        BinaryUpdateOutcome `json:"outcome"`
	OutcomeSHA256  string              `json:"outcome_sha256"`
	Decision       Decision            `json:"decision"`
	DecisionSHA256 string              `json:"decision_sha256"`
	CreatedAtUTC   time.Time           `json:"created_at_utc"`
}

type BinaryUpdateProjectionReceipt struct {
	Schema        string    `json:"schema"`
	AttemptID     string    `json:"attempt_id"`
	DebtSHA256    string    `json:"debt_sha256"`
	JournalKey    string    `json:"journal_key"`
	ReadBackAtUTC time.Time `json:"read_back_at_utc"`
}

type BinaryUpdateReconciliation struct {
	Attempt            BinaryUpdateAttempt         `json:"attempt"`
	Outcome            *BinaryUpdateOutcome        `json:"outcome,omitempty"`
	Projection         *BinaryUpdateProjectionDebt `json:"projection,omitempty"`
	InvocationRecorded bool                        `json:"invocation_recorded"`
	ProjectionReadBack bool                        `json:"projection_read_back"`
}

func binaryUpdateEvidenceName(kind string, data []byte) string {
	sum := sha256.Sum256(data)
	return "binary-update-" + kind + "-" + hex.EncodeToString(sum[:]) + ".json"
}

func (s *Store) writeBinaryUpdateEvidence(kind string, value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return s.createOrVerify("evidence", s.evidenceFD, binaryUpdateEvidenceName(kind, data), data)
}

// readBinaryUpdateEvidenceLocked finds one create-only, digest-named action
// record by its typed operation id. Ambiguous or corrupt leaves fail closed.
func (s *Store) readBinaryUpdateEvidenceLocked(kind, operationID string) ([]byte, error) {
	names, err := readDirectoryNames(s.evidenceFD)
	if err != nil {
		return nil, err
	}
	prefix := "binary-update-" + kind + "-"
	var match []byte
	for _, name := range names {
		if kind == "projection" && strings.HasPrefix(name, "binary-update-projection-receipt-") {
			continue
		}
		if !strings.HasPrefix(name, prefix) || !strings.HasSuffix(name, ".json") {
			continue
		}
		digest := strings.TrimSuffix(strings.TrimPrefix(name, prefix), ".json")
		if !validDigest(digest) {
			return nil, errors.New("maat memory: malformed binary-update evidence name")
		}
		data, err := readExact(s.evidenceFD, name)
		if err != nil {
			return nil, err
		}
		sum := sha256.Sum256(data)
		if hex.EncodeToString(sum[:]) != digest {
			return nil, errors.New("maat memory: binary-update evidence digest mismatch")
		}
		var record map[string]json.RawMessage
		if err := json.Unmarshal(data, &record); err != nil {
			return nil, errors.New("maat memory: malformed binary-update evidence")
		}
		var id string
		switch kind {
		case "started":
			_ = json.Unmarshal(record["id"], &id)
		case "outcome", "projection-receipt", "invocation":
			_ = json.Unmarshal(record["attempt_id"], &id)
		case "projection":
			var attempt struct {
				ID string `json:"id"`
			}
			_ = json.Unmarshal(record["attempt"], &attempt)
			id = attempt.ID
		default:
			return nil, errors.New("maat memory: unsupported binary-update evidence kind")
		}
		if id != operationID {
			continue
		}
		if match != nil {
			return nil, errors.New("maat memory: duplicate binary-update evidence identity")
		}
		match = data
	}
	if match == nil {
		return nil, unix.ENOENT
	}
	return match, nil
}

// BeginBinaryUpdateAttempt serializes memory admission and the existing
// operation-wide journal claim. The claim remains the one launch authority;
// the create-only Store attempt is committed first, then the callback claims
// the exact screen while the Store lock is held. Lock order is Store exclusive
// flock, then journal mutation lock. The callback must not reenter this Store.
// The updater cannot be reached until a separate typed invocation fact is
// written and read back after final action revalidation.
func (s *Store) BeginBinaryUpdateAttempt(m BinaryUpdateManifest, receipt PreflightReceipt, claim func() (string, error)) (BinaryUpdateAttempt, error) {
	if s == nil || s.attemptsFD < 0 || s.evidenceFD < 0 || claim == nil || receipt.Decision != PreflightPass {
		return BinaryUpdateAttempt{}, errors.New("maat memory: binary-update attempt requires a passing preflight and atomic launch claim")
	}
	if err := m.ValidateFresh(); err != nil {
		return BinaryUpdateAttempt{}, err
	}
	digest, err := m.Digest()
	if err != nil || m.StoreUUID != s.storeUUID || receipt.Action != binaryUpdateScope() || receipt.ActionManifestSHA256 != digest {
		return BinaryUpdateAttempt{}, errors.New("maat memory: preflight does not bind this binary-update action")
	}
	receiptDigest, err := receipt.Digest()
	if err != nil {
		return BinaryUpdateAttempt{}, err
	}
	if err := s.lock(unix.LOCK_EX); err != nil {
		return BinaryUpdateAttempt{}, err
	}
	defer s.unlock()
	if err := m.ValidateFresh(); err != nil {
		return BinaryUpdateAttempt{}, err
	}
	if receipt.RegistrySnapshotVersion != 2 {
		return BinaryUpdateAttempt{}, errors.New("maat memory: historical receipt cannot admit binary update")
	}
	retained, err := s.readPreflightReceiptLocked(receiptDigest)
	if err != nil {
		return BinaryUpdateAttempt{}, fmt.Errorf("maat memory: exact retained preflight required: %w", err)
	}
	if digestEqual(retained, receipt) != nil {
		return BinaryUpdateAttempt{}, errors.New("maat memory: retained binary-update receipt differs")
	}
	current, evalErr := s.preflightBoundLocked(binaryUpdateScope(), digest, resolveMemoryGuard)
	if evalErr != nil || current.Decision != PreflightPass || current.RegistrySnapshotSHA256 != receipt.RegistrySnapshotSHA256 {
		return BinaryUpdateAttempt{}, errors.New("maat memory: binary-update preflight or registry changed before admission")
	}
	if _, err := s.pendingBinaryUpdateAttemptLocked(); err == nil {
		return BinaryUpdateAttempt{}, errors.New("maat memory: incomplete binary-update attempt requires inspection; replay refused")
	} else if !errors.Is(err, ErrNoIncompleteBinaryUpdateAttempt) {
		return BinaryUpdateAttempt{}, err
	}
	if debts, err := s.pendingBinaryUpdateProjectionDebtsLocked(); err != nil || len(debts) != 0 {
		return BinaryUpdateAttempt{}, errors.New("maat memory: binary-update projection debt requires reconciliation before consent")
	}
	reconciliations, err := s.binaryUpdateReconciliationsLocked("")
	if err != nil {
		return BinaryUpdateAttempt{}, err
	}
	claimDigest := sha256.Sum256([]byte(m.CaseEvidence))
	for _, history := range reconciliations {
		if history.Outcome != nil && !history.ProjectionReadBack {
			return BinaryUpdateAttempt{}, errors.New("maat memory: known binary-update terminal fact requires data-only projection reconciliation")
		}
	}
	id, err := newStoreUUID()
	if err != nil {
		return BinaryUpdateAttempt{}, err
	}
	a := BinaryUpdateAttempt{Schema: MemorySchema, ID: id, Manifest: m, ActionManifestSHA256: digest, PreflightReceiptSHA256: receiptDigest, RegistrySnapshotSHA256: receipt.RegistrySnapshotSHA256, LaunchClaimEvidence: "diagnostic-repair-attempt:sha256=" + hex.EncodeToString(claimDigest[:]), InvocationProtocol: BinaryUpdateInvocationProtocol, StartedAtUTC: binaryUpdateNow().UTC()}
	b, err := json.Marshal(a)
	if err != nil {
		return BinaryUpdateAttempt{}, err
	}
	if err := s.createOrVerify("evidence", s.evidenceFD, binaryUpdateEvidenceName("started", b), b); err != nil {
		return BinaryUpdateAttempt{}, fmt.Errorf("maat memory: binary-update attempt persistence failed before journal claim; updater not run: %w", err)
	}
	claimEvidence, claimErr := claim()
	if claimErr != nil {
		if _, recordErr := s.recordBinaryUpdateNotInvokedLocked(a, "launch_claim_refused", binaryUpdateBoundDetail(claimErr.Error())); recordErr != nil {
			return a, fmt.Errorf("maat memory: launch claim failed and known no-invocation fact could not be retained; leave unresolved: claim=%v fact=%w", claimErr, recordErr)
		}
		return a, fmt.Errorf("maat memory: existing launch claim refused; updater was not invoked: %w", claimErr)
	}
	if claimEvidence != a.LaunchClaimEvidence {
		if _, recordErr := s.recordBinaryUpdateNotInvokedLocked(a, "launch_claim_linkage_refused", "journal claim returned a key different from the exact source-case claim"); recordErr != nil {
			return a, fmt.Errorf("maat memory: mismatched launch claim and known no-invocation fact could not be retained; leave unresolved: %w", recordErr)
		}
		return a, errors.New("maat memory: launch claim is not bound to this exact System One case; updater was not invoked")
	}
	return a, nil
}

var ErrNoIncompleteBinaryUpdateAttempt = errors.New("maat memory: no incomplete binary-update attempt")

func (s *Store) pendingBinaryUpdateAttemptLocked() (BinaryUpdateAttempt, error) {
	if _, err := s.verifyWriteJournal(); err != nil {
		return BinaryUpdateAttempt{}, err
	}
	names, err := readDirectoryNames(s.evidenceFD)
	if err != nil {
		return BinaryUpdateAttempt{}, err
	}
	starts, ends, invocations := map[string]BinaryUpdateAttempt{}, map[string]BinaryUpdateOutcome{}, map[string]BinaryUpdateInvocation{}
	for _, name := range names {
		switch {
		case strings.HasPrefix(name, "binary-update-started-") && strings.HasSuffix(name, ".json"):
			digest := strings.TrimSuffix(strings.TrimPrefix(name, "binary-update-started-"), ".json")
			if !validDigest(digest) {
				return BinaryUpdateAttempt{}, errors.New("maat memory: malformed binary-update attempt name")
			}
			b, err := readExact(s.evidenceFD, name)
			if err != nil {
				return BinaryUpdateAttempt{}, err
			}
			sum := sha256.Sum256(b)
			if hex.EncodeToString(sum[:]) != digest {
				return BinaryUpdateAttempt{}, errors.New("maat memory: binary-update attempt digest mismatch")
			}
			var a BinaryUpdateAttempt
			if json.Unmarshal(b, &a) != nil || validateBinaryUpdateAttempt(a, s.storeUUID) != nil {
				return BinaryUpdateAttempt{}, errors.New("maat memory: invalid binary-update attempt")
			}
			if _, duplicate := starts[a.ID]; duplicate {
				return BinaryUpdateAttempt{}, errors.New("maat memory: multiple launch records exist for one binary-update attempt")
			}
			starts[a.ID] = a
		case strings.HasPrefix(name, "binary-update-outcome-") && strings.HasSuffix(name, ".json"):
			digest := strings.TrimSuffix(strings.TrimPrefix(name, "binary-update-outcome-"), ".json")
			if !validDigest(digest) {
				return BinaryUpdateAttempt{}, errors.New("maat memory: malformed binary-update outcome name")
			}
			b, err := readExact(s.evidenceFD, name)
			if err != nil {
				return BinaryUpdateAttempt{}, err
			}
			sum := sha256.Sum256(b)
			if hex.EncodeToString(sum[:]) != digest {
				return BinaryUpdateAttempt{}, errors.New("maat memory: binary-update outcome digest mismatch")
			}
			var o BinaryUpdateOutcome
			if json.Unmarshal(b, &o) != nil || validateBinaryUpdateOutcome(o, o.AttemptID) != nil {
				return BinaryUpdateAttempt{}, errors.New("maat memory: invalid binary-update outcome")
			}
			if _, duplicate := ends[o.AttemptID]; duplicate {
				return BinaryUpdateAttempt{}, errors.New("maat memory: multiple terminal outcomes exist for one binary-update attempt")
			}
			ends[o.AttemptID] = o
		case strings.HasPrefix(name, "binary-update-invocation-") && strings.HasSuffix(name, ".json"):
			digest := strings.TrimSuffix(strings.TrimPrefix(name, "binary-update-invocation-"), ".json")
			if !validDigest(digest) {
				return BinaryUpdateAttempt{}, errors.New("maat memory: malformed binary-update invocation name")
			}
			b, err := readExact(s.evidenceFD, name)
			if err != nil {
				return BinaryUpdateAttempt{}, err
			}
			sum := sha256.Sum256(b)
			if hex.EncodeToString(sum[:]) != digest {
				return BinaryUpdateAttempt{}, errors.New("maat memory: binary-update invocation digest mismatch")
			}
			var invocation BinaryUpdateInvocation
			if json.Unmarshal(b, &invocation) != nil || invocation.Schema != MemorySchema || !validUUID(invocation.AttemptID) || !validDigest(invocation.AttemptSHA256) || invocation.AuthorizedAtUTC.IsZero() {
				return BinaryUpdateAttempt{}, errors.New("maat memory: invalid binary-update invocation fact")
			}
			if _, duplicate := invocations[invocation.AttemptID]; duplicate {
				return BinaryUpdateAttempt{}, errors.New("maat memory: multiple invocation facts exist for one binary-update attempt")
			}
			invocations[invocation.AttemptID] = invocation
		case strings.HasPrefix(name, "binary-update-projection-") || strings.HasPrefix(name, "binary-update-projection-receipt-"):
		case strings.HasPrefix(name, "binary-update-"):
			return BinaryUpdateAttempt{}, errors.New("maat memory: malformed binary-update evidence name")
		default:
			// Other governed evidence objects share this existing namespace.
		}
	}
	var pending *BinaryUpdateAttempt
	for id, a := range starts {
		if _, ok := ends[id]; ok {
			continue
		}
		if pending != nil {
			return BinaryUpdateAttempt{}, errors.New("maat memory: multiple incomplete binary-update attempts require review")
		}
		copy := a
		pending = &copy
	}
	for id := range ends {
		if _, ok := starts[id]; !ok {
			return BinaryUpdateAttempt{}, errors.New("maat memory: binary-update outcome has no attempt")
		}
	}
	for id, invocation := range invocations {
		attempt, ok := starts[id]
		if !ok {
			return BinaryUpdateAttempt{}, errors.New("maat memory: binary-update invocation has no attempt")
		}
		attemptDigest, err := digestJSON(attempt)
		if err != nil || invocation.AttemptSHA256 != attemptDigest {
			return BinaryUpdateAttempt{}, errors.New("maat memory: binary-update invocation does not bind its attempt")
		}
		if attempt.InvocationProtocol != BinaryUpdateInvocationProtocol {
			return BinaryUpdateAttempt{}, errors.New("maat memory: legacy attempt contains an unrecognized invocation marker")
		}
		if outcome, ok := ends[id]; ok && outcome.Determination == "not_invoked" {
			return BinaryUpdateAttempt{}, errors.New("maat memory: not-invoked outcome contradicts a persisted invocation fact")
		}
	}
	if pending == nil {
		return BinaryUpdateAttempt{}, ErrNoIncompleteBinaryUpdateAttempt
	}
	return *pending, nil
}

func (s *Store) PendingBinaryUpdateAttempt() (BinaryUpdateAttempt, error) {
	if err := s.lock(unix.LOCK_SH); err != nil {
		return BinaryUpdateAttempt{}, err
	}
	defer s.unlock()
	return s.pendingBinaryUpdateAttemptLocked()
}

func validateBinaryUpdateAttempt(a BinaryUpdateAttempt, storeUUID string) error {
	if a.Schema != MemorySchema || !validUUID(a.ID) || a.Manifest.StoreUUID != storeUUID || a.Manifest.Validate() != nil || a.StartedAtUTC.IsZero() || !validDigest(a.ActionManifestSHA256) || !validDigest(a.PreflightReceiptSHA256) || !validDigest(a.RegistrySnapshotSHA256) || a.LaunchClaimEvidence == "" || (a.InvocationProtocol != "" && a.InvocationProtocol != BinaryUpdateInvocationProtocol) {
		return errors.New("maat memory: invalid binary-update attempt identity")
	}
	d, e := a.Manifest.Digest()
	if e != nil || d != a.ActionManifestSHA256 {
		return errors.New("maat memory: binary-update manifest digest mismatch")
	}
	claimDigest := sha256.Sum256([]byte(a.Manifest.CaseEvidence))
	if a.LaunchClaimEvidence != "diagnostic-repair-attempt:sha256="+hex.EncodeToString(claimDigest[:]) {
		return errors.New("maat memory: binary-update attempt is not linked to the source-case launch claim")
	}
	return nil
}

func validBinaryUpdateDetermination(v string) bool {
	return v == "resolved" || v == "failed" || v == "not_invoked"
}

func binaryUpdateProjectionDetermination(o BinaryUpdateOutcome) string {
	if o.Determination == "not_invoked" {
		return "failed"
	}
	return o.Determination
}

func validateBinaryUpdateOutcome(o BinaryUpdateOutcome, attemptID string) error {
	if o.Schema != MemorySchema || o.AttemptID != attemptID || o.RecordedAtUTC.IsZero() || !validBinaryUpdateDetermination(o.Determination) || !validDigest(o.ObservationSHA256) || len(o.Detail) > 4096 {
		return errors.New("maat memory: invalid binary-update outcome")
	}
	if o.Determination == "not_invoked" {
		if o.UpdaterReturned || o.PostcheckPassed || (o.FactCode != "launch_claim_refused" && o.FactCode != "launch_claim_linkage_refused" && o.FactCode != "prelaunch_revalidation_failed" && o.FactCode != "invocation_record_failed" && o.FactCode != "claim_not_invoked_before_barrier") {
			return errors.New("maat memory: invalid known-not-invoked fact")
		}
	} else if !o.UpdaterReturned || o.FactCode != "" {
		return errors.New("maat memory: terminal updater outcome contradicts its invocation fact")
	}
	if o.Determination != "not_invoked" && (o.Determination == "resolved") != o.PostcheckPassed {
		return errors.New("maat memory: binary-update outcome contradicts the exact postcheck")
	}
	return nil
}

func (s *Store) RecordBinaryUpdateInvocation(a BinaryUpdateAttempt) error {
	if s == nil || s.evidenceFD < 0 || validateBinaryUpdateAttempt(a, s.storeUUID) != nil {
		return errors.New("maat memory: invalid binary-update invocation identity")
	}
	if err := a.Manifest.ValidateFresh(); err != nil {
		return err
	}
	if err := s.lock(unix.LOCK_EX); err != nil {
		return err
	}
	defer s.unlock()
	if err := a.Manifest.ValidateFresh(); err != nil {
		return err
	}
	pending, err := s.pendingBinaryUpdateAttemptLocked()
	if err != nil || pending.ID != a.ID || digestEqual(pending, a) != nil {
		return errors.New("maat memory: updater invocation requires the unique incomplete attempt")
	}
	retained, err := s.readPreflightReceiptLocked(a.PreflightReceiptSHA256)
	if err != nil || retained.Action != binaryUpdateScope() || retained.ActionManifestSHA256 != a.ActionManifestSHA256 || retained.RegistrySnapshotSHA256 != a.RegistrySnapshotSHA256 {
		return errors.New("maat memory: invocation barrier requires the exact retained full-manifest receipt")
	}
	current, evalErr := s.preflightBoundLocked(binaryUpdateScope(), a.ActionManifestSHA256, resolveMemoryGuard)
	if evalErr != nil || current.Decision != PreflightPass || current.RegistrySnapshotSHA256 != a.RegistrySnapshotSHA256 {
		return errors.New("maat memory: binary-update guard registry or action changed before updater invocation")
	}
	if _, err := s.readBinaryUpdateEvidenceLocked("outcome", a.ID); err == nil {
		return errors.New("maat memory: terminal binary-update fact already exists")
	} else if !errors.Is(err, unix.ENOENT) {
		return err
	}
	if _, err := s.readBinaryUpdateEvidenceLocked("invocation", a.ID); err == nil {
		return errors.New("maat memory: updater invocation was already authorized; do not invoke it again")
	} else if !errors.Is(err, unix.ENOENT) {
		return err
	}
	attemptDigest, err := digestJSON(a)
	if err != nil {
		return err
	}
	invocation := BinaryUpdateInvocation{Schema: MemorySchema, AttemptID: a.ID, AttemptSHA256: attemptDigest, AuthorizedAtUTC: binaryUpdateNow().UTC()}
	if err := s.writeBinaryUpdateEvidence("invocation", invocation); err != nil {
		return err
	}
	readback, err := s.readBinaryUpdateEvidenceLocked("invocation", a.ID)
	if err != nil {
		return err
	}
	var got BinaryUpdateInvocation
	if json.Unmarshal(readback, &got) != nil || digestEqual(got, invocation) != nil {
		return errors.New("maat memory: updater invocation fact readback differs; execution must remain unresolved")
	}
	return nil
}

func (s *Store) RecordBinaryUpdateNotInvoked(a BinaryUpdateAttempt, factCode, detail string) (BinaryUpdateOutcome, error) {
	if s == nil || s.evidenceFD < 0 || validateBinaryUpdateAttempt(a, s.storeUUID) != nil || len(detail) > 4096 {
		return BinaryUpdateOutcome{}, errors.New("maat memory: invalid known-not-invoked input")
	}
	if err := s.lock(unix.LOCK_EX); err != nil {
		return BinaryUpdateOutcome{}, err
	}
	defer s.unlock()
	return s.recordBinaryUpdateNotInvokedLocked(a, factCode, detail)
}

// recordBinaryUpdateNotInvokedLocked is called only while the current process
// still knows it has not crossed the invocation boundary. A missing invocation
// leaf alone is never converted into this fact during history reconciliation.
func (s *Store) recordBinaryUpdateNotInvokedLocked(a BinaryUpdateAttempt, factCode, detail string) (BinaryUpdateOutcome, error) {
	pending, err := s.pendingBinaryUpdateAttemptLocked()
	if err != nil || pending.ID != a.ID || digestEqual(pending, a) != nil {
		return BinaryUpdateOutcome{}, errors.New("maat memory: known-not-invoked fact requires the unique incomplete attempt")
	}
	if _, err := s.readBinaryUpdateEvidenceLocked("invocation", a.ID); err == nil {
		return BinaryUpdateOutcome{}, errors.New("maat memory: invocation fact exists; leave updater completion unresolved")
	} else if !errors.Is(err, unix.ENOENT) {
		return BinaryUpdateOutcome{}, err
	}
	observation := sha256.Sum256([]byte("binary-update-not-invoked\x00" + factCode + "\x00" + detail))
	o := BinaryUpdateOutcome{Schema: MemorySchema, AttemptID: a.ID, RecordedAtUTC: binaryUpdateNow().UTC(), Determination: "not_invoked", ObservationSHA256: hex.EncodeToString(observation[:]), FactCode: factCode, Detail: detail}
	if err := validateBinaryUpdateOutcome(o, a.ID); err != nil {
		return BinaryUpdateOutcome{}, err
	}
	b, err := json.Marshal(o)
	if err != nil {
		return BinaryUpdateOutcome{}, err
	}
	if err := s.createOrVerify("evidence", s.evidenceFD, binaryUpdateEvidenceName("outcome", b), b); err != nil {
		return BinaryUpdateOutcome{}, err
	}
	return o, nil
}

func (s *Store) RecordBinaryUpdateOutcome(a BinaryUpdateAttempt, updaterReturned, postcheckPassed bool, observationDigest, detail string) (BinaryUpdateOutcome, error) {
	if s == nil || s.attemptsFD < 0 || s.evidenceFD < 0 || validateBinaryUpdateAttempt(a, s.storeUUID) != nil || len(detail) > 4096 || !validDigest(observationDigest) {
		return BinaryUpdateOutcome{}, errors.New("maat memory: invalid binary-update outcome input")
	}
	if !updaterReturned {
		return BinaryUpdateOutcome{}, errors.New("maat memory: updater completion is unknown; leave the durable attempt unresolved")
	}
	if err := s.lock(unix.LOCK_EX); err != nil {
		return BinaryUpdateOutcome{}, err
	}
	defer s.unlock()
	pending, err := s.pendingBinaryUpdateAttemptLocked()
	if err != nil || pending.ID != a.ID || digestEqual(pending, a) != nil {
		return BinaryUpdateOutcome{}, errors.New("maat memory: binary-update is not the unique incomplete attempt")
	}
	invocationBytes, err := s.readBinaryUpdateEvidenceLocked("invocation", a.ID)
	if err != nil {
		return BinaryUpdateOutcome{}, errors.New("maat memory: updater invocation fact is required before a returned outcome")
	}
	var invocation BinaryUpdateInvocation
	attemptDigest, digestErr := digestJSON(a)
	if digestErr != nil || json.Unmarshal(invocationBytes, &invocation) != nil || invocation.Schema != MemorySchema || invocation.AttemptID != a.ID || invocation.AttemptSHA256 != attemptDigest || invocation.AuthorizedAtUTC.IsZero() {
		return BinaryUpdateOutcome{}, errors.New("maat memory: updater invocation fact differs from the retained attempt")
	}
	determination := "failed"
	if postcheckPassed {
		determination = "resolved"
	}
	o := BinaryUpdateOutcome{Schema: MemorySchema, AttemptID: a.ID, RecordedAtUTC: binaryUpdateNow().UTC(), Determination: determination, UpdaterReturned: updaterReturned, PostcheckPassed: postcheckPassed, ObservationSHA256: observationDigest, Detail: detail}
	b, err := json.Marshal(o)
	if err != nil {
		return BinaryUpdateOutcome{}, err
	}
	if err := s.createOrVerify("evidence", s.evidenceFD, binaryUpdateEvidenceName("outcome", b), b); err != nil {
		return BinaryUpdateOutcome{}, err
	}
	return o, nil
}

func (s *Store) BeginBinaryUpdateProjection(a BinaryUpdateAttempt, o BinaryUpdateOutcome, d Decision) (BinaryUpdateProjectionDebt, error) {
	if s == nil || s.evidenceFD < 0 || validateBinaryUpdateAttempt(a, s.storeUUID) != nil || validateBinaryUpdateOutcome(o, a.ID) != nil || d.Kind != "diagnostic verified repair" || d.Resource != "binary-drift" || d.OriginEvidence != a.Manifest.CaseEvidence || d.Determination != binaryUpdateProjectionDetermination(o) {
		return BinaryUpdateProjectionDebt{}, errors.New("maat memory: invalid binary-update projection identity")
	}
	if err := s.lock(unix.LOCK_EX); err != nil {
		return BinaryUpdateProjectionDebt{}, err
	}
	defer s.unlock()
	if _, err := s.verifyWriteJournal(); err != nil {
		return BinaryUpdateProjectionDebt{}, err
	}
	storedAttempt, err := s.readBinaryUpdateEvidenceLocked("started", a.ID)
	if err != nil {
		return BinaryUpdateProjectionDebt{}, errors.New("maat memory: durable binary-update attempt is required before projection")
	}
	var priorAttempt BinaryUpdateAttempt
	if json.Unmarshal(storedAttempt, &priorAttempt) != nil || digestEqual(priorAttempt, a) != nil {
		return BinaryUpdateProjectionDebt{}, errors.New("maat memory: supplied binary-update attempt differs from retained launch record")
	}
	storedOutcome, err := s.readBinaryUpdateEvidenceLocked("outcome", a.ID)
	if err != nil {
		return BinaryUpdateProjectionDebt{}, errors.New("maat memory: durable binary-update outcome is required before projection")
	}
	var priorOutcome BinaryUpdateOutcome
	if json.Unmarshal(storedOutcome, &priorOutcome) != nil || digestEqual(priorOutcome, o) != nil {
		return BinaryUpdateProjectionDebt{}, errors.New("maat memory: supplied binary-update outcome differs from retained terminal result")
	}
	debt := BinaryUpdateProjectionDebt{Schema: BinaryUpdateProjectionSchema, Attempt: a, Outcome: o, Decision: d, CreatedAtUTC: o.RecordedAtUTC}
	debt.AttemptSHA256, _ = digestJSON(a)
	debt.OutcomeSHA256, _ = digestJSON(o)
	debt.DecisionSHA256, _ = digestJSON(d)
	b, err := json.Marshal(debt)
	if err != nil {
		return BinaryUpdateProjectionDebt{}, err
	}
	if err := s.createOrVerify("evidence", s.evidenceFD, binaryUpdateEvidenceName("projection", b), b); err != nil {
		return BinaryUpdateProjectionDebt{}, err
	}
	return debt, nil
}

func (s *Store) PersistBinaryUpdateProjectionOutcome(debt BinaryUpdateProjectionDebt) error {
	if err := s.lock(unix.LOCK_EX); err != nil {
		return err
	}
	defer s.unlock()
	if _, err := s.verifyWriteJournal(); err != nil {
		return err
	}
	stored, err := s.readBinaryUpdateProjectionLocked(debt.Attempt.ID)
	if err != nil || digestEqual(stored, debt) != nil {
		return errors.New("maat memory: binary-update projection debt differs")
	}
	b, err := json.Marshal(debt.Outcome)
	if err != nil {
		return err
	}
	return s.createOrVerify("evidence", s.evidenceFD, binaryUpdateEvidenceName("outcome", b), b)
}

func (s *Store) PendingBinaryUpdateProjectionDebts() ([]BinaryUpdateProjectionDebt, error) {
	if err := s.lock(unix.LOCK_SH); err != nil {
		return nil, err
	}
	defer s.unlock()
	return s.pendingBinaryUpdateProjectionDebtsLocked()
}

// BinaryUpdateReconciliations returns exact immutable records for read-only
// recovery. It never derives a no-invocation fact from a missing leaf.
func (s *Store) BinaryUpdateReconciliations(caseEvidence string) ([]BinaryUpdateReconciliation, error) {
	if caseEvidence != "" && (!strings.HasPrefix(caseEvidence, "maat-system-one:sha256=") || !validDigest(strings.TrimPrefix(caseEvidence, "maat-system-one:sha256="))) {
		return nil, errors.New("maat memory: invalid source case for binary-update reconciliation")
	}
	if err := s.lock(unix.LOCK_SH); err != nil {
		return nil, err
	}
	defer s.unlock()
	if _, err := s.verifyWriteJournal(); err != nil {
		return nil, err
	}
	if _, err := s.pendingBinaryUpdateAttemptLocked(); err != nil && !errors.Is(err, ErrNoIncompleteBinaryUpdateAttempt) {
		return nil, err
	}
	names, err := readDirectoryNames(s.evidenceFD)
	if err != nil {
		return nil, err
	}
	history := []BinaryUpdateReconciliation{}
	for _, name := range names {
		if !strings.HasPrefix(name, "binary-update-started-") || !strings.HasSuffix(name, ".json") {
			continue
		}
		digest := strings.TrimSuffix(strings.TrimPrefix(name, "binary-update-started-"), ".json")
		if !validDigest(digest) {
			return nil, errors.New("maat memory: malformed binary-update attempt name")
		}
		data, err := readExact(s.evidenceFD, name)
		if err != nil {
			return nil, err
		}
		bodyDigest := sha256.Sum256(data)
		if hex.EncodeToString(bodyDigest[:]) != digest {
			return nil, errors.New("maat memory: binary-update attempt digest mismatch")
		}
		var attempt BinaryUpdateAttempt
		if json.Unmarshal(data, &attempt) != nil || validateBinaryUpdateAttempt(attempt, s.storeUUID) != nil {
			return nil, errors.New("maat memory: invalid binary-update attempt")
		}
		if caseEvidence != "" && attempt.Manifest.CaseEvidence != caseEvidence {
			continue
		}
		entry := BinaryUpdateReconciliation{Attempt: attempt}
		if _, err := s.readBinaryUpdateEvidenceLocked("invocation", attempt.ID); err == nil {
			entry.InvocationRecorded = true
		} else if !errors.Is(err, unix.ENOENT) {
			return nil, err
		}
		if raw, err := s.readBinaryUpdateEvidenceLocked("outcome", attempt.ID); err == nil {
			var outcome BinaryUpdateOutcome
			if json.Unmarshal(raw, &outcome) != nil || validateBinaryUpdateOutcome(outcome, attempt.ID) != nil {
				return nil, errors.New("maat memory: invalid binary-update outcome")
			}
			copy := outcome
			entry.Outcome = &copy
		} else if !errors.Is(err, unix.ENOENT) {
			return nil, err
		}
		if debt, err := s.readBinaryUpdateProjectionLocked(attempt.ID); err == nil {
			copy := debt
			entry.Projection = &copy
		} else if !errors.Is(err, unix.ENOENT) {
			return nil, err
		}
		if raw, err := s.readBinaryUpdateEvidenceLocked("projection-receipt", attempt.ID); err == nil {
			if entry.Projection == nil {
				return nil, errors.New("maat memory: projection readback receipt has no retained projection intent")
			}
			var receipt BinaryUpdateProjectionReceipt
			debtDigest, digestErr := digestJSON(*entry.Projection)
			if json.Unmarshal(raw, &receipt) != nil || digestErr != nil || receipt.Schema != BinaryUpdateProjectionSchema || receipt.AttemptID != attempt.ID || receipt.DebtSHA256 != debtDigest || receipt.JournalKey != entry.Projection.Decision.Evidence || receipt.ReadBackAtUTC.IsZero() {
				return nil, errors.New("maat memory: binary-update projection readback receipt differs")
			}
			entry.ProjectionReadBack = true
		} else if !errors.Is(err, unix.ENOENT) {
			return nil, err
		}
		if entry.Outcome != nil && entry.Projection != nil && digestEqual(entry.Outcome, entry.Projection.Outcome) != nil {
			return nil, errors.New("maat memory: terminal outcome and projection disagree")
		}
		history = append(history, entry)
	}
	sort.Slice(history, func(i, j int) bool {
		if history[i].Attempt.StartedAtUTC.Equal(history[j].Attempt.StartedAtUTC) {
			return history[i].Attempt.ID < history[j].Attempt.ID
		}
		return history[i].Attempt.StartedAtUTC.Before(history[j].Attempt.StartedAtUTC)
	})
	return history, nil
}

func (s *Store) pendingBinaryUpdateProjectionDebtsLocked() ([]BinaryUpdateProjectionDebt, error) {
	if _, err := s.verifyWriteJournal(); err != nil {
		return nil, err
	}
	names, err := readDirectoryNames(s.evidenceFD)
	if err != nil {
		return nil, err
	}
	debts := []BinaryUpdateProjectionDebt{}
	for _, name := range names {
		if !strings.HasPrefix(name, "binary-update-projection-") || strings.HasPrefix(name, "binary-update-projection-receipt-") || !strings.HasSuffix(name, ".json") {
			continue
		}
		digest := strings.TrimSuffix(strings.TrimPrefix(name, "binary-update-projection-"), ".json")
		if !validDigest(digest) {
			return nil, errors.New("maat memory: malformed binary-update projection name")
		}
		data, err := readExact(s.evidenceFD, name)
		if err != nil {
			return nil, err
		}
		sum := sha256.Sum256(data)
		if hex.EncodeToString(sum[:]) != digest {
			return nil, errors.New("maat memory: binary-update projection digest mismatch")
		}
		var header struct {
			Attempt BinaryUpdateAttempt `json:"attempt"`
		}
		if json.Unmarshal(data, &header) != nil || !validUUID(header.Attempt.ID) {
			return nil, errors.New("maat memory: malformed binary-update projection")
		}
		id := header.Attempt.ID
		d, err := s.readBinaryUpdateProjectionLocked(id)
		if err != nil {
			return nil, err
		}
		if _, err := s.readBinaryUpdateEvidenceLocked("projection-receipt", id); errors.Is(err, unix.ENOENT) {
			debts = append(debts, d)
		} else if err != nil {
			return nil, err
		}
	}
	sort.Slice(debts, func(i, j int) bool { return debts[i].Attempt.ID < debts[j].Attempt.ID })
	return debts, nil
}

func (s *Store) readBinaryUpdateProjectionLocked(id string) (BinaryUpdateProjectionDebt, error) {
	if !validUUID(id) {
		return BinaryUpdateProjectionDebt{}, errors.New("maat memory: invalid binary projection id")
	}
	b, err := s.readBinaryUpdateEvidenceLocked("projection", id)
	if err != nil {
		return BinaryUpdateProjectionDebt{}, err
	}
	var d BinaryUpdateProjectionDebt
	if json.Unmarshal(b, &d) != nil || d.Schema != BinaryUpdateProjectionSchema || d.Attempt.ID != id || validateBinaryUpdateAttempt(d.Attempt, s.storeUUID) != nil || validateBinaryUpdateOutcome(d.Outcome, id) != nil || validateDecision(d.Decision) != nil || d.Decision.Kind != "diagnostic verified repair" || d.Decision.Resource != "binary-drift" || d.Decision.OriginEvidence != d.Attempt.Manifest.CaseEvidence || d.Decision.Determination != binaryUpdateProjectionDetermination(d.Outcome) || d.CreatedAtUTC.IsZero() {
		return BinaryUpdateProjectionDebt{}, errors.New("maat memory: invalid binary-update projection debt")
	}
	if da, ea := digestJSON(d.Attempt); ea != nil || da != d.AttemptSHA256 {
		return BinaryUpdateProjectionDebt{}, errors.New("maat memory: binary-update attempt digest mismatch")
	}
	if do, eo := digestJSON(d.Outcome); eo != nil || do != d.OutcomeSHA256 {
		return BinaryUpdateProjectionDebt{}, errors.New("maat memory: binary-update outcome digest mismatch")
	}
	if dd, ed := digestJSON(d.Decision); ed != nil || dd != d.DecisionSHA256 {
		return BinaryUpdateProjectionDebt{}, errors.New("maat memory: binary-update decision digest mismatch")
	}
	return d, nil
}

func (s *Store) MarkBinaryUpdateProjectionReadBack(d BinaryUpdateProjectionDebt) error {
	if err := s.lock(unix.LOCK_EX); err != nil {
		return err
	}
	defer s.unlock()
	if _, err := s.verifyWriteJournal(); err != nil {
		return err
	}
	stored, err := s.readBinaryUpdateProjectionLocked(d.Attempt.ID)
	if err != nil || digestEqual(stored, d) != nil {
		return errors.New("maat memory: binary-update projection changed")
	}
	debtDigest, err := digestJSON(d)
	if err != nil {
		return err
	}
	outcomeBytes, err := s.readBinaryUpdateEvidenceLocked("outcome", d.Attempt.ID)
	if err != nil {
		return errors.New("maat memory: binary-update outcome is not durable")
	}
	var outcome BinaryUpdateOutcome
	if json.Unmarshal(outcomeBytes, &outcome) != nil || digestEqual(outcome, d.Outcome) != nil {
		return errors.New("maat memory: binary-update outcome readback differs")
	}
	var got BinaryUpdateProjectionReceipt
	if readback, readErr := s.readBinaryUpdateEvidenceLocked("projection-receipt", d.Attempt.ID); readErr == nil {
		if json.Unmarshal(readback, &got) != nil || got.Schema != BinaryUpdateProjectionSchema || got.AttemptID != d.Attempt.ID || got.DebtSHA256 != debtDigest || got.JournalKey != d.Decision.Evidence || got.ReadBackAtUTC.IsZero() {
			return errors.New("maat memory: binary-update projection receipt readback differs")
		}
		return nil
	} else if !errors.Is(readErr, unix.ENOENT) {
		return readErr
	}
	r := BinaryUpdateProjectionReceipt{Schema: BinaryUpdateProjectionSchema, AttemptID: d.Attempt.ID, DebtSHA256: debtDigest, JournalKey: d.Decision.Evidence, ReadBackAtUTC: binaryUpdateNow().UTC()}
	if err := s.writeBinaryUpdateEvidence("projection-receipt", r); err != nil {
		return err
	}
	readback, err := s.readBinaryUpdateEvidenceLocked("projection-receipt", d.Attempt.ID)
	if err != nil {
		return err
	}
	if json.Unmarshal(readback, &got) != nil || got.Schema != r.Schema || got.AttemptID != r.AttemptID || got.DebtSHA256 != r.DebtSHA256 || got.JournalKey != r.JournalKey || got.ReadBackAtUTC.IsZero() {
		return errors.New("maat memory: binary-update projection receipt readback differs")
	}
	return nil
}
