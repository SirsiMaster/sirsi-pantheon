package maat

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"sort"
	"strings"

	"golang.org/x/sys/unix"
)

const DiagnosticRecordSchema = "sirsi.maat.diagnostic-record.v1"
const diagnosticIntentSuffix = ".diagnostic-record.json"
const diagnosticResultSuffix = ".diagnostic-result.json"

var ErrDiagnosticDeliveryUnknown = errors.New("maat diagnostic: record delivery is unknown; inspect/reconcile only, never resend")
var ErrDiagnosticNotSubmitted = errors.New("maat diagnostic: journal was not called; fresh observation and confirmation required")
var ErrDiagnosticNotRecorded = errors.New("maat diagnostic: request durably fenced and exact journal has no matching record; never replay this observation")

type DiagnosticRecordManifest struct {
	Schema      string                `json:"schema"`
	UID         uint32                `json:"uid"`
	StoreUUID   string                `json:"store_uuid"`
	Observation DiagnosticObservation `json:"observation"`
}

func (m DiagnosticRecordManifest) Validate() error {
	if m.Schema != DiagnosticRecordSchema || m.UID != uint32(os.Geteuid()) || !validUUID(m.StoreUUID) {
		return errors.New("maat diagnostic: actor/store manifest differs")
	}
	return m.Observation.Validate()
}
func (m DiagnosticRecordManifest) Digest() (string, error) {
	if err := m.Validate(); err != nil {
		return "", err
	}
	return diagnosticSHA(m)
}
func diagnosticScope() Scope {
	return Scope{Component: "pantheon", Profile: "local-macos", Operation: "record-system-one-diagnostic"}
}

type DiagnosticRecordIntent struct {
	Schema   string                    `json:"schema"`
	ID       string                    `json:"id"`
	Manifest DiagnosticRecordManifest  `json:"manifest"`
	Receipt  PreflightReceiptReference `json:"receipt"`
}
type DiagnosticRecordResult struct {
	Schema           string `json:"schema"`
	ID               string `json:"id"`
	IntentSHA256     string `json:"intent_sha256"`
	DecisionEvidence string `json:"decision_evidence"`
	VerdictSHA256    string `json:"verdict_sha256"`
	State            string `json:"state"`
}

// Use the journal's existing atomic append-once + complete strict lookup.
// A Recent window, nil transport result or append exit alone is not readback.
type DiagnosticRecordJournal interface {
	DecisionJournal
	AppendOnce(Decision) (bool, error)
	FindByEvidence(kind, evidence string) (Decision, bool, error)
}
type diagnosticOnceJournal struct {
	DiagnosticRecordJournal
	ref PreflightReceiptReference
	id  string
}

func (j diagnosticOnceJournal) Append(d Decision) error {
	d.OriginEvidence = diagnosticRecordOrigin(j.id, j.ref.ReceiptSHA256)
	_, err := j.AppendOnce(d)
	return err
}

func diagnosticRecordOrigin(id, receipt string) string {
	return "maat-diagnostic-record:" + id + ":preflight-sha256=" + receipt
}

func (s *Store) PreflightDiagnosticRecord(m DiagnosticRecordManifest) (PreflightReceipt, error) {
	if s == nil {
		return PreflightReceipt{}, errors.New("maat diagnostic: missing store")
	}
	if err := m.Validate(); err != nil {
		return PreflightReceipt{}, err
	}
	if m.StoreUUID != s.storeUUID || s.storeUUID == "" {
		return PreflightReceipt{}, errors.New("maat diagnostic: initialized default custody required")
	}
	if err := m.Observation.ValidateFresh(); err != nil {
		return PreflightReceipt{}, err
	}
	digest, err := m.Digest()
	if err != nil {
		return PreflightReceipt{}, err
	}
	return s.preflightBound(diagnosticScope(), digest, resolveMemoryGuard)
}

// Lock order: existing memory lock -> existing journal mutation/read lock.
// The caller's provenance verifier is read-only; no new lock or store is added.
// Intent commits before RecordSystemOne; no error path automatically appends.
func (s *Store) RecordDiagnostic(m DiagnosticRecordManifest, h StoredPreflightReceipt, j DiagnosticRecordJournal, verifyProvenance func() error) (DiagnosticRecordResult, error) {
	if j == nil || verifyProvenance == nil {
		return DiagnosticRecordResult{}, errors.New("maat diagnostic: exact journal and provenance verification required")
	}
	// Freeze the typed status slice before invoking any caller-owned callback.
	manifestBytes, err := json.Marshal(m)
	if err != nil {
		return DiagnosticRecordResult{}, err
	}
	var snapshot DiagnosticRecordManifest
	if err := json.Unmarshal(manifestBytes, &snapshot); err != nil {
		return DiagnosticRecordResult{}, err
	}
	m = snapshot
	id, err := m.Digest()
	if err != nil {
		return DiagnosticRecordResult{}, err
	}
	if err := s.lock(unix.LOCK_EX); err != nil {
		return DiagnosticRecordResult{}, err
	}
	defer s.unlock()
	return s.recordDiagnosticLocked(m, h, j, verifyProvenance, id)
}

func (s *Store) recordDiagnosticLocked(m DiagnosticRecordManifest, h StoredPreflightReceipt, j DiagnosticRecordJournal, verifyProvenance func() error, id string) (DiagnosticRecordResult, error) {
	if m.StoreUUID != s.storeUUID || s.storeUUID == "" {
		return DiagnosticRecordResult{}, errors.New("maat diagnostic: store binding changed")
	}
	if _, err := s.verifyWriteJournal(); err != nil {
		return DiagnosticRecordResult{}, err
	}
	if _, err := s.pendingLivenessWatchAttemptLocked(); err != nil && !errors.Is(err, ErrNoIncompleteLivenessWatchAttempt) {
		return DiagnosticRecordResult{}, err
	}
	intents, err := s.diagnosticIntentsLocked()
	if err != nil {
		return DiagnosticRecordResult{}, err
	}
	for key := range intents {
		result, err := s.readDiagnosticResultLocked(key)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return DiagnosticRecordResult{}, err
		}
		if key == id {
			if err == nil {
				if result.State == "not_recorded" {
					return result, ErrDiagnosticNotRecorded
				}
				if result.State == "not_submitted" {
					return result, ErrDiagnosticNotSubmitted
				}
				return s.reconcileDiagnosticRecordLocked(intents[key], j)
			}
			return DiagnosticRecordResult{}, ErrDiagnosticDeliveryUnknown
		}
		if errors.Is(err, os.ErrNotExist) {
			return DiagnosticRecordResult{}, ErrDiagnosticDeliveryUnknown
		}
	}
	if err := m.Observation.ValidateFresh(); err != nil {
		return DiagnosticRecordResult{}, err
	}
	if err := verifyProvenance(); err != nil {
		return DiagnosticRecordResult{}, errors.New("maat diagnostic: selected provenance changed")
	}
	r, err := s.readPreflightReceiptLocked(h.Reference().ReceiptSHA256)
	if err != nil {
		return DiagnosticRecordResult{}, err
	}
	ref := h.Reference()
	rd, err := r.Digest()
	if err != nil || rd != ref.ReceiptSHA256 || ref.validate() != nil || ref.Action != diagnosticScope() || ref.ActionSHA256 != id || ref.RegistrySHA256 != r.RegistrySnapshotSHA256 || ref.StoreBinding != s.receiptBinding() || r.Decision != PreflightPass || r.Action != diagnosticScope() || r.ActionManifestSHA256 != id || r.RegistrySnapshotVersion != 2 {
		return DiagnosticRecordResult{}, errors.New("maat diagnostic: exact retained scoped receipt required")
	}
	current, err := s.preflightBoundLocked(diagnosticScope(), id, resolveMemoryGuard)
	if err != nil || current.Decision != PreflightPass || current.RegistrySnapshotSHA256 != r.RegistrySnapshotSHA256 {
		return DiagnosticRecordResult{}, errors.New("maat diagnostic: registry changed or scoped record refused")
	}
	intent := DiagnosticRecordIntent{DiagnosticRecordSchema, id, m, ref}
	raw, err := json.Marshal(intent)
	if err != nil {
		return DiagnosticRecordResult{}, err
	}
	if err := s.createOrVerify("attempts", s.attemptsFD, id+diagnosticIntentSuffix, raw); err != nil {
		return DiagnosticRecordResult{}, err
	}
	// Recheck the same captured local provenance immediately before the effect.
	// Known pre-append refusal records not_submitted; uncertain journal delivery
	// remains debt. Neither state permits replay of this captured observation.
	if err := m.Observation.ValidateFresh(); err != nil {
		return s.diagnosticNotSubmittedLocked(intent)
	}
	if err := verifyProvenance(); err != nil {
		return s.diagnosticNotSubmittedLocked(intent)
	}
	verdict, err := m.Observation.Verdict()
	if err != nil {
		return DiagnosticRecordResult{}, err
	}
	if _, err := RecordSystemOne(diagnosticOnceJournal{j, ref, id}, "sirsi maat triage", verdict); err != nil {
		return DiagnosticRecordResult{}, ErrDiagnosticDeliveryUnknown
	}
	return s.reconcileDiagnosticRecordLocked(intent, j)
}

// Data-only recovery. It never invokes Append/AppendOnce, re-observes the host,
// initializes memory, alters an incident or executes recovery prose.
func (s *Store) ReconcileDiagnosticRecord(id string, j DiagnosticRecordJournal) (DiagnosticRecordResult, error) {
	if !validDigest(id) || j == nil {
		return DiagnosticRecordResult{}, errors.New("maat diagnostic: exact record identity required")
	}
	if err := s.lock(unix.LOCK_EX); err != nil {
		return DiagnosticRecordResult{}, err
	}
	defer s.unlock()
	intent, err := s.readDiagnosticIntentLocked(id)
	if err != nil {
		return DiagnosticRecordResult{}, err
	}
	return s.reconcileDiagnosticRecordLocked(intent, j)
}

func (s *Store) reconcileDiagnosticRecordLocked(intent DiagnosticRecordIntent, j DiagnosticRecordJournal) (DiagnosticRecordResult, error) {
	if result, err := s.readDiagnosticResultLocked(intent.ID); err == nil && (result.State == "not_submitted" || result.State == "not_recorded") {
		data, e := json.Marshal(result)
		if e != nil {
			return DiagnosticRecordResult{}, e
		}
		if e = s.createOrVerify("attempts", s.attemptsFD, intent.ID+diagnosticResultSuffix, data); e != nil {
			return DiagnosticRecordResult{}, e
		}
		return result, nil
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return DiagnosticRecordResult{}, err
	}
	verdict, err := intent.Manifest.Observation.Verdict()
	if err != nil {
		return DiagnosticRecordResult{}, err
	}
	vd, err := diagnosticSHA(verdict)
	if err != nil {
		return DiagnosticRecordResult{}, err
	}
	evidence := "maat-system-one:sha256=" + vd
	decision, found, err := j.FindByEvidence("system one screen", evidence)
	if err != nil || !found || decision.SystemOne == nil {
		return DiagnosticRecordResult{}, ErrDiagnosticDeliveryUnknown
	}
	observed, err := diagnosticSHA(*decision.SystemOne)
	if err != nil || observed != vd || decision.Kind != "system one screen" || decision.Requester != "sirsi maat triage" || decision.Resource != verdict.Subject.Repo || decision.Assessed != verdict.Subject.Kind+" "+verdict.Subject.Ref || decision.Affected != verdict.Subject.HeadSHA || decision.Determination != string(verdict.Gate) || decision.Why != systemOneReason(verdict) || decision.Evidence != evidence || decision.OriginEvidence != diagnosticRecordOrigin(intent.ID, intent.Receipt.ReceiptSHA256) || decision.ResolutionFor != "" || decision.FailureMemoryReceipt != nil {
		return DiagnosticRecordResult{}, ErrDiagnosticDeliveryUnknown
	}
	intentSHA, err := diagnosticSHA(intent)
	if err != nil {
		return DiagnosticRecordResult{}, err
	}
	result := DiagnosticRecordResult{DiagnosticRecordSchema, intent.ID, intentSHA, evidence, vd, "recorded_verified"}
	raw, err := json.Marshal(result)
	if err != nil {
		return DiagnosticRecordResult{}, err
	}
	// Deterministic data-only readback receipt can resume only its own exact WAL
	// tuple; any other prepared object or corrupt committed target still refuses.
	if err := s.createOrVerify("attempts", s.attemptsFD, intent.ID+diagnosticResultSuffix, raw); err != nil {
		return DiagnosticRecordResult{}, err
	}
	return s.readDiagnosticResultLocked(intent.ID)
}

func (s *Store) diagnosticNotSubmittedLocked(intent DiagnosticRecordIntent) (DiagnosticRecordResult, error) {
	ih, err := diagnosticSHA(intent)
	if err != nil {
		return DiagnosticRecordResult{}, err
	}
	result := DiagnosticRecordResult{Schema: DiagnosticRecordSchema, ID: intent.ID, IntentSHA256: ih, State: "not_submitted"}
	data, err := json.Marshal(result)
	if err != nil {
		return result, err
	}
	if err := s.createOrVerify("attempts", s.attemptsFD, intent.ID+diagnosticResultSuffix, data); err != nil {
		return result, errors.Join(ErrDiagnosticNotSubmitted, err)
	}
	return result, ErrDiagnosticNotSubmitted
}

func decodeDiagnosticCanonical(raw []byte, v any) error {
	if err := json.Unmarshal(raw, v); err != nil {
		return err
	}
	b, err := json.Marshal(v)
	if err != nil || !bytes.Equal(b, raw) {
		return errors.New("maat diagnostic: noncanonical record custody")
	}
	return nil
}
func (s *Store) readDiagnosticIntentLocked(id string) (DiagnosticRecordIntent, error) {
	var intent DiagnosticRecordIntent
	raw, err := readExact(s.attemptsFD, id+diagnosticIntentSuffix)
	if err != nil {
		return intent, err
	}
	if decodeDiagnosticCanonical(raw, &intent) != nil || intent.Schema != DiagnosticRecordSchema || intent.ID != id || intent.Manifest.StoreUUID != s.storeUUID || intent.Receipt.validate() != nil {
		return intent, errors.New("maat diagnostic: invalid record intent")
	}
	digest, err := intent.Manifest.Digest()
	if err != nil || digest != id || intent.Receipt.Action != diagnosticScope() || intent.Receipt.ActionSHA256 != id || intent.Receipt.StoreBinding != s.receiptBinding() {
		return intent, errors.New("maat diagnostic: intent scope/store differs")
	}
	receiptBytes, err := readExact(s.evidenceFD, intent.Receipt.ReceiptSHA256)
	if err != nil {
		return intent, err
	}
	receipt, err := decodePreflightReceipt(receiptBytes)
	if err != nil {
		return intent, err
	}
	rd, err := receipt.Digest()
	if err != nil || rd != intent.Receipt.ReceiptSHA256 || receipt.RegistrySnapshotVersion != 2 || receipt.Action != diagnosticScope() || receipt.ActionManifestSHA256 != id || receipt.RegistrySnapshotSHA256 != intent.Receipt.RegistrySHA256 || receipt.Decision != PreflightPass {
		return intent, errors.New("maat diagnostic: intent retained receipt differs")
	}
	return intent, nil
}
func (s *Store) readDiagnosticResultLocked(id string) (DiagnosticRecordResult, error) {
	var result DiagnosticRecordResult
	raw, err := readExact(s.attemptsFD, id+diagnosticResultSuffix)
	if err != nil {
		return result, err
	}
	intent, err := s.readDiagnosticIntentLocked(id)
	if err != nil {
		return result, err
	}
	ih, err := diagnosticSHA(intent)
	if err != nil {
		return result, err
	}
	verdict, err := intent.Manifest.Observation.Verdict()
	if err != nil {
		return result, err
	}
	vd, err := diagnosticSHA(verdict)
	if err != nil {
		return result, err
	}
	if decodeDiagnosticCanonical(raw, &result) != nil || result.Schema != DiagnosticRecordSchema || result.ID != id || result.IntentSHA256 != ih {
		return result, errors.New("maat diagnostic: invalid exact readback result")
	}
	if (result.State == "not_submitted" || result.State == "not_recorded") && result.VerdictSHA256 == "" && result.DecisionEvidence == "" {
		if result.State == "not_recorded" {
			if err := s.verifyDiagnosticRequestFenceLocked(id); err != nil {
				return result, err
			}
		}
		return result, nil
	}
	if result.State != "recorded_verified" || result.VerdictSHA256 != vd || result.DecisionEvidence != "maat-system-one:sha256="+vd {
		return result, errors.New("maat diagnostic: invalid recorded postcondition")
	}
	return result, nil
}

func isDiagnosticRecordLeaf(name string) bool {
	return isDiagnosticRequestLeaf(name) || strings.HasSuffix(name, diagnosticIntentSuffix) || strings.HasSuffix(name, diagnosticResultSuffix)
}
func (s *Store) validateDiagnosticRecordLeafLocked(name string) error {
	if isDiagnosticRequestLeaf(name) {
		return s.validateDiagnosticRequestLeafLocked(name)
	}
	if strings.HasSuffix(name, diagnosticIntentSuffix) {
		id := strings.TrimSuffix(name, diagnosticIntentSuffix)
		if !validDigest(id) {
			return errors.New("maat diagnostic: invalid intent name")
		}
		_, err := s.readDiagnosticIntentLocked(id)
		return err
	}
	id := strings.TrimSuffix(name, diagnosticResultSuffix)
	if !validDigest(id) {
		return errors.New("maat diagnostic: invalid result name")
	}
	_, err := s.readDiagnosticResultLocked(id)
	return err
}
func (s *Store) diagnosticIntentsLocked() (map[string]DiagnosticRecordIntent, error) {
	names, err := readDirectoryNames(s.attemptsFD)
	if err != nil {
		return nil, err
	}
	intents := map[string]DiagnosticRecordIntent{}
	for _, name := range names {
		if !isDiagnosticRecordLeaf(name) {
			continue
		}
		if err := s.validateDiagnosticRecordLeafLocked(name); err != nil {
			return nil, err
		}
		if strings.HasSuffix(name, diagnosticIntentSuffix) {
			id := strings.TrimSuffix(name, diagnosticIntentSuffix)
			intent, err := s.readDiagnosticIntentLocked(id)
			if err != nil {
				return nil, err
			}
			intents[id] = intent
		}
	}
	return intents, nil
}

// Inspection may admit only one exact diagnostic RESULT write as pending;
// every other WAL target still verifies. It returns identity, never success.
func (s *Store) diagnosticPendingResultLocked() (string, string, error) {
	names, err := readDirectoryNames(s.writesFD)
	if err != nil {
		return "", "", err
	}
	id, key := "", ""
	for _, name := range names {
		if !strings.HasSuffix(name, ".prepared.json") {
			continue
		}
		raw, err := readExact(s.writesFD, name)
		if err != nil {
			return "", "", err
		}
		var intent writeIntent
		if decodeDiagnosticCanonical(raw, &intent) != nil || intent.validate() != nil || name != intent.Key+".prepared.json" {
			return "", "", errors.New("maat diagnostic: invalid prepared metadata")
		}
		if _, err := readExact(s.writesFD, intent.Key+".committed.json"); err == nil {
			continue
		} else if !errors.Is(err, os.ErrNotExist) {
			return "", "", err
		}
		candidate := strings.TrimSuffix(intent.Leaf, diagnosticResultSuffix)
		if key != "" || intent.Namespace != "attempts" || !strings.HasSuffix(intent.Leaf, diagnosticResultSuffix) || !validDigest(candidate) {
			return "", "", errors.New("maat diagnostic: unrelated unresolved write remains")
		}
		if _, err := s.readDiagnosticIntentLocked(candidate); err != nil {
			return "", "", err
		}
		id, key = candidate, intent.Key
	}
	if _, err := s.verifyWriteJournalAllow(key); err != nil {
		return "", "", err
	}
	return id, key, nil
}

// Read-only discoverable intent/debt identities; no caller must hunt raw files.
func (s *Store) PendingDiagnosticRecords() ([]DiagnosticRecordIntent, error) {
	if err := s.lock(unix.LOCK_SH); err != nil {
		return nil, err
	}
	defer s.unlock()
	pendingResult, pendingKey, err := s.diagnosticPendingResultLocked()
	if err != nil {
		return nil, err
	}
	if pendingKey == "" {
		if _, err := s.pendingLivenessWatchAttemptLocked(); err != nil && !errors.Is(err, ErrNoIncompleteLivenessWatchAttempt) {
			return nil, err
		}
	}
	intents, err := s.diagnosticIntentsLocked()
	if err != nil {
		return nil, err
	}
	pending := []DiagnosticRecordIntent{}
	for id, intent := range intents {
		if id == pendingResult {
			pending = append(pending, intent)
			continue
		}
		if _, err := s.readDiagnosticResultLocked(id); errors.Is(err, os.ErrNotExist) {
			pending = append(pending, intent)
		} else if err != nil {
			return nil, err
		}
	}
	sort.Slice(pending, func(i, j int) bool { return pending[i].ID < pending[j].ID })
	return pending, nil
}
