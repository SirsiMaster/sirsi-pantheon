package maat

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"golang.org/x/sys/unix"
)

const localWatchProducer = "pantheon.local-managed-watch/v1"
const localCompletionSchema = "sirsi.maat.liveness-completion.v1"
const localIntakeSchema = "sirsi.maat.liveness-failure-intake.v1"
const maxLocalRecordBytes = 16 * 1024

func localWatchScope() Scope {
	return Scope{Component: "pantheon", Profile: "local-macos", Operation: "install-managed-liveness-watch"}
}

type livenessInvocation struct {
	Schema        string    `json:"schema"`
	StoreUUID     string    `json:"store_uuid"`
	AttemptID     string    `json:"attempt_id"`
	AttemptSHA256 string    `json:"attempt_sha256"`
	StartedAtUTC  time.Time `json:"started_at_utc"`
	Mode          string    `json:"mode"`
	Host          string    `json:"host"`
}

// Completion witnesses have no raw-JSON writer. RunLivenessWatchDirectAttempt
// creates them around the existing bounded installer/observer callbacks.
type LivenessCompletion struct {
	Schema                 string    `json:"schema"`
	StoreUUID              string    `json:"store_uuid"`
	AttemptID              string    `json:"attempt_id"`
	AttemptSHA256          string    `json:"attempt_sha256"`
	ActionManifestSHA256   string    `json:"action_manifest_sha256"`
	PreflightReceiptSHA256 string    `json:"preflight_receipt_sha256"`
	Producer               string    `json:"producer"`
	Mode                   string    `json:"mode"`
	Invocation             string    `json:"invocation"`
	Observation            string    `json:"observation"`
	Scope                  Scope     `json:"scope"`
	OwnerUID               uint32    `json:"owner_uid"`
	Host                   string    `json:"host"`
	ManagedLabel           string    `json:"managed_label"`
	TargetPlistPath        string    `json:"target_plist_path"`
	ExpectedPlistSHA256    string    `json:"expected_plist_sha256"`
	ObservationSHA256      string    `json:"observation_sha256"`
	OutcomeSHA256          string    `json:"outcome_sha256"`
	RecordedAtUTC          time.Time `json:"recorded_at_utc"`
}

type LivenessDirectResult struct {
	Completion LivenessCompletion
	Outcome    LivenessWatchOutcome
	Message    string
}

type FailureIntakeReceipt struct {
	Schema           string    `json:"schema"`
	StoreUUID        string    `json:"store_uuid"`
	AttemptID        string    `json:"attempt_id"`
	CompletionSHA256 string    `json:"completion_sha256"`
	OutcomeSHA256    string    `json:"outcome_sha256"`
	EvidenceSHA256   string    `json:"evidence_sha256"`
	Disposition      string    `json:"disposition"`
	IncidentKey      string    `json:"incident_key,omitempty"`
	OccurrenceKey    string    `json:"occurrence_key,omitempty"`
	RecordedAtUTC    time.Time `json:"recorded_at_utc"`
}

type localFailureProof struct {
	Schema                 string    `json:"schema"`
	StoreUUID              string    `json:"store_uuid"`
	AttemptID              string    `json:"attempt_id"`
	AttemptSHA256          string    `json:"attempt_sha256"`
	CompletionSHA256       string    `json:"completion_sha256"`
	OutcomeSHA256          string    `json:"outcome_sha256"`
	PreflightReceiptSHA256 string    `json:"preflight_receipt_sha256"`
	Producer               string    `json:"producer"`
	Observation            string    `json:"observation"`
	RecordedAtUTC          time.Time `json:"recorded_at_utc"`
}

// The durable invocation marker is claimed under the same store lock as
// admission. Re-entering an attempt never invokes its installer again, even
// after a crash with no completion witness. No generic command is accepted.
func (s *Store) RunLivenessWatchDirectAttempt(attempt LivenessWatchAttempt,
	install func(LivenessWatchManifest) (string, error), observe func(LivenessWatchManifest) (LivenessWatchObservation, error),
) (LivenessDirectResult, error) {
	if install == nil || observe == nil {
		return LivenessDirectResult{}, errors.New("maat memory: closed installer and observer are required")
	}
	if err := s.lock(unix.LOCK_EX); err != nil {
		return LivenessDirectResult{}, err
	}
	if _, err := s.verifyWriteJournal(); err != nil {
		s.unlock()
		return LivenessDirectResult{}, err
	}
	if err := s.verifyStartedAttemptLocked(attempt); err != nil {
		s.unlock()
		return LivenessDirectResult{}, err
	}
	if attempt.CompletionProtocolVersion != 1 {
		s.unlock()
		return LivenessDirectResult{}, errors.New("maat memory: legacy attempt cannot acquire direct provenance")
	}
	if completed, err := s.readLivenessCompletionLocked(attempt.ID); err == nil {
		s.unlock()
		return completed, nil
	}
	if _, err := readExact(s.attemptsFD, attempt.ID+".invocation-v1.json"); err == nil {
		s.unlock()
		return LivenessDirectResult{}, errors.New("maat memory: invocation already claimed; inspect retained completion or unknown action, never replay")
	} else if !errors.Is(err, unix.ENOENT) {
		s.unlock()
		return LivenessDirectResult{}, err
	}
	digest, err := digestJSON(attempt)
	if err != nil {
		s.unlock()
		return LivenessDirectResult{}, err
	}
	host, hostErr := os.Hostname()
	if hostErr != nil || strings.TrimSpace(host) == "" || len(host) > 256 {
		s.unlock()
		return LivenessDirectResult{}, errors.New("maat memory: local host metadata unavailable")
	}
	claim := livenessInvocation{Schema: "sirsi.maat.liveness-invocation.v1", StoreUUID: s.storeUUID, AttemptID: attempt.ID, AttemptSHA256: digest, StartedAtUTC: time.Now().UTC(), Mode: "direct-install", Host: host}
	if err := s.writeLocalAttemptLocked(attempt.ID+".invocation-v1.json", claim); err != nil {
		s.unlock()
		return LivenessDirectResult{}, err
	}
	s.unlock()
	message, installErr := install(attempt.Manifest)
	observation, observeErr := observe(attempt.Manifest)
	invocation := "returned-success"
	if installErr != nil {
		invocation = "returned-error"
	}
	return s.persistLivenessCompletion(attempt, claim, invocation, message, installErr, observation, observeErr)
}

func (s *Store) persistLivenessCompletion(attempt LivenessWatchAttempt, claim livenessInvocation, invocation, message string, installErr error, observation LivenessWatchObservation, observeErr error) (LivenessDirectResult, error) {
	state := classifyLocalPostcondition(attempt.Manifest, observation, observeErr)
	errorText := ""
	if installErr != nil {
		errorText = "installer returned an error"
	}
	if invocation == "not-invoked" {
		errorText = "installer was not invoked"
	}
	if state == "unverifiable" {
		errorText += "; exact post-observation unavailable"
	}
	observation = canonicalCompletionObservation(attempt.Manifest, state)
	outcome, err := NewLivenessWatchOutcome(attempt, observation, errorText)
	if err != nil {
		return LivenessDirectResult{}, err
	}
	observationDigest, err := digestJSON(outcome.Observation)
	if err != nil {
		return LivenessDirectResult{}, err
	}
	outcomeDigest, err := digestJSON(outcome)
	if err != nil {
		return LivenessDirectResult{}, err
	}
	witness := LivenessCompletion{Schema: localCompletionSchema, StoreUUID: s.storeUUID, AttemptID: attempt.ID, AttemptSHA256: claim.AttemptSHA256,
		ActionManifestSHA256: attempt.ActionManifestSHA256, PreflightReceiptSHA256: attempt.PreflightReceiptSHA256,
		Producer: localWatchProducer, Mode: claim.Mode, Invocation: invocation, Observation: state, Scope: localWatchScope(),
		OwnerUID: attempt.Manifest.UID, Host: claim.Host, ManagedLabel: attempt.Manifest.ManagedLabel, TargetPlistPath: attempt.Manifest.TargetPlistPath,
		ExpectedPlistSHA256: attempt.Manifest.ExpectedPlistSHA256, ObservationSHA256: observationDigest, OutcomeSHA256: outcomeDigest, RecordedAtUTC: outcome.RecordedAtUTC}
	if err := s.lock(unix.LOCK_EX); err != nil {
		return LivenessDirectResult{}, err
	}
	defer s.unlock()
	if err := s.writeLocalAttemptLocked(attempt.ID+".completion-v1.json", witness); err != nil {
		return LivenessDirectResult{}, fmt.Errorf("maat memory: invocation returned but completion custody is incomplete; never replay: %w", err)
	}
	if err := s.writeLocalAttemptLocked(attempt.ID+".outcome.json", outcome); err != nil {
		return LivenessDirectResult{}, err
	}
	return LivenessDirectResult{Completion: witness, Outcome: outcome, Message: message}, nil
}

func classifyLocalPostcondition(m LivenessWatchManifest, o LivenessWatchObservation, err error) string {
	if err != nil || o.UID != m.UID || o.Domain != m.Domain || o.Label != m.ManagedLabel || o.PlistPath != m.TargetPlistPath {
		return "unverifiable"
	}
	switch o.Postcondition {
	case "exact":
		if o.LaunchdLoaded && o.PlistSHA256 == m.ExpectedPlistSHA256 {
			return "exact"
		}
	case "known-absent":
		if !o.LaunchdLoaded && o.PlistSHA256 == "" {
			return "known-absent"
		}
	case "known-unloaded":
		if !o.LaunchdLoaded && o.PlistSHA256 == m.ExpectedPlistSHA256 {
			return "known-unloaded"
		}
	}
	return "unverifiable"
}

func (s *Store) verifyStartedAttemptLocked(attempt LivenessWatchAttempt) error {
	if validateLivenessAttempt(attempt, attempt.ID, s.storeUUID) != nil || !validUUID(s.storeUUID) {
		return errors.New("maat memory: attempt does not bind initialized local custody")
	}
	data, err := readExact(s.attemptsFD, attempt.ID+".started.json")
	if err != nil {
		return err
	}
	var stored LivenessWatchAttempt
	if err := decodeLocalRecord(data, &stored); err != nil || digestEqual(stored, attempt) != nil {
		return errors.New("maat memory: started attempt changed")
	}
	return s.verifyCommittedBytes("attempts", attempt.ID+".started.json", data)
}

func (s *Store) readLivenessCompletionLocked(id string) (LivenessDirectResult, error) {
	if !validUUID(id) {
		return LivenessDirectResult{}, errors.New("maat memory: invalid completion attempt id")
	}
	var attempt LivenessWatchAttempt
	var witness LivenessCompletion
	var outcome LivenessWatchOutcome
	var claim livenessInvocation
	if err := s.readLocalAttemptLocked(id+".started.json", &attempt); err != nil {
		return LivenessDirectResult{}, err
	}
	if err := s.verifyStartedAttemptLocked(attempt); err != nil {
		return LivenessDirectResult{}, err
	}
	if err := s.readLocalAttemptLocked(id+".invocation-v1.json", &claim); err != nil {
		return LivenessDirectResult{}, err
	}
	if err := s.readLocalAttemptLocked(id+".completion-v1.json", &witness); err != nil {
		return LivenessDirectResult{}, err
	}
	if err := s.readLocalAttemptLocked(id+".outcome.json", &outcome); err != nil {
		return LivenessDirectResult{}, err
	}
	aDigest, _ := digestJSON(attempt)
	oDigest, _ := digestJSON(outcome)
	observedDigest, _ := digestJSON(outcome.Observation)
	if attempt.CompletionProtocolVersion != 1 || witness.Schema != localCompletionSchema || witness.StoreUUID != s.storeUUID || witness.AttemptID != id || witness.AttemptSHA256 != aDigest ||
		witness.ActionManifestSHA256 != attempt.ActionManifestSHA256 || witness.PreflightReceiptSHA256 != attempt.PreflightReceiptSHA256 || witness.Producer != localWatchProducer ||
		(witness.Mode != "direct-install" && witness.Mode != "reobserve-only") || (witness.Invocation != "returned-success" && witness.Invocation != "returned-error" && witness.Invocation != "not-invoked") || witness.Scope != localWatchScope() || witness.OwnerUID != attempt.Manifest.UID ||
		witness.ManagedLabel != attempt.Manifest.ManagedLabel || witness.TargetPlistPath != attempt.Manifest.TargetPlistPath || witness.ExpectedPlistSHA256 != attempt.Manifest.ExpectedPlistSHA256 ||
		witness.OutcomeSHA256 != oDigest || witness.ObservationSHA256 != observedDigest || witness.RecordedAtUTC != outcome.RecordedAtUTC || witness.RecordedAtUTC.Before(claim.StartedAtUTC) ||
		claim.Schema != "sirsi.maat.liveness-invocation.v1" || claim.StoreUUID != s.storeUUID || claim.AttemptID != id || claim.AttemptSHA256 != aDigest || claim.StartedAtUTC.Before(attempt.StartedAtUTC) ||
		validateLivenessOutcome(outcome, attempt) != nil {
		return LivenessDirectResult{}, errors.New("maat memory: completion provenance or identity mismatch")
	}
	if err := s.validateCompletionBindingsLocked(witness, attempt, claim); err != nil {
		return LivenessDirectResult{}, err
	}
	expected, err := completionOutcome(witness, attempt, claim)
	if err != nil || digestEqual(expected, outcome) != nil {
		return LivenessDirectResult{}, errors.New("maat memory: completion outcome is not its closed canonical observation")
	}
	if witness.Observation != "unverifiable" && witness.Observation != classifyLocalPostcondition(attempt.Manifest, outcome.Observation, nil) {
		return LivenessDirectResult{}, errors.New("maat memory: completion contradicts retained observation")
	}
	if _, err := s.readPreflightReceiptObjectLocked(witness.PreflightReceiptSHA256); err != nil {
		return LivenessDirectResult{}, err
	}
	return LivenessDirectResult{Completion: witness, Outcome: outcome}, nil
}

// Read-only historical completion is useful for data-only reconciliation. It
// can never invoke the callback path or manufacture a missing witness.
func (s *Store) ReadLivenessWatchCompletion(id string) (LivenessDirectResult, error) {
	if err := s.lock(unix.LOCK_SH); err != nil {
		return LivenessDirectResult{}, err
	}
	defer s.unlock()
	if _, err := s.verifyWriteJournal(); err != nil {
		return LivenessDirectResult{}, err
	}
	return s.readLivenessCompletionLocked(id)
}

func eligibleLocalFailure(c LivenessCompletion) bool {
	return c.Mode == "direct-install" && c.Invocation == "returned-success" && (c.Observation == "known-absent" || c.Observation == "known-unloaded")
}

func localFailureSignature(observation string) FailureSignature {
	code := "watch-absent-after-success"
	if observation == "known-unloaded" {
		code = "watch-unloaded-after-success"
	}
	return FailureSignature{Namespace: "pantheon", Version: "local-managed-watch-failure/v1", FailureClass: "managed-watch-postcondition", Operation: "install-managed-liveness-watch", Component: "pantheon", Invariant: "managed-watch-present-and-loaded-after-installer-success", ErrorCode: code}
}

func localFailureActions() []RecoveryAction {
	return []RecoveryAction{{ID: "inspect-managed-watch-failure", Label: "Inspect retained direct failure", Instruction: "Inspect the exact attempt, complete preflight and authoritative postcondition; unknown action debt cannot be waived."},
		{ID: "accept-local-prevention", Label: "Review this local prevention", Instruction: "Explicit owner acceptance may lift only this occurrence's local recurrence prevention without claiming repair. A later repair needs fresh diagnosis, preflight and separate consent."}}
}

func (s *Store) RememberLivenessFailure(id string) (FailureIntakeReceipt, error) {
	if err := s.lock(unix.LOCK_EX); err != nil {
		return FailureIntakeReceipt{}, err
	}
	defer s.unlock()
	result, err := s.readLivenessCompletionLocked(id)
	if err != nil {
		return FailureIntakeReceipt{}, err
	}
	c := result.Completion
	completionDigest, _ := digestJSON(c)
	if existing, err := s.readFailureIntakeLocked(id); err == nil {
		if _, err := s.verifyWriteJournal(); err != nil {
			return FailureIntakeReceipt{}, err
		}
		return existing, nil
	} else if !errors.Is(err, unix.ENOENT) {
		data, readErr := readExact(s.attemptsFD, id+".intake-v1.json")
		if readErr != nil || s.verifyCommittedBytes("attempts", id+".intake-v1.json", data) == nil {
			return FailureIntakeReceipt{}, err
		}
	}
	proof := localFailureProof{Schema: "sirsi.maat.local-managed-watch-failure-proof.v1", StoreUUID: s.storeUUID, AttemptID: id, AttemptSHA256: c.AttemptSHA256, CompletionSHA256: completionDigest, OutcomeSHA256: c.OutcomeSHA256, PreflightReceiptSHA256: c.PreflightReceiptSHA256, Producer: localWatchProducer, Observation: c.Observation, RecordedAtUTC: c.RecordedAtUTC}
	proofBytes, err := json.Marshal(proof)
	if err != nil {
		return FailureIntakeReceipt{}, err
	}
	proofDigest, err := s.putLocalEvidenceLocked(proofBytes)
	if err != nil {
		return FailureIntakeReceipt{}, err
	}
	receipt := FailureIntakeReceipt{Schema: localIntakeSchema, StoreUUID: s.storeUUID, AttemptID: id, CompletionSHA256: completionDigest, OutcomeSHA256: c.OutcomeSHA256, EvidenceSHA256: proofDigest, Disposition: "not-eligible", RecordedAtUTC: c.RecordedAtUTC}
	if c.Invocation == "returned-error" || (c.Observation == "unverifiable" && c.Invocation != "not-invoked") {
		receipt.Disposition = "unknown"
	}
	if eligibleLocalFailure(c) {
		incident, err := NewIncident(localFailureSignature(c.Observation), proofDigest, localWatchScope(), ActiveIncidentGuardID, ActiveIncidentGuardVersion, localFailureActions())
		if err != nil {
			return FailureIntakeReceipt{}, err
		}
		head, err := s.linkLocalFailureLocked(incident, c.RecordedAtUTC)
		if err != nil {
			return FailureIntakeReceipt{}, err
		}
		receipt.Disposition, receipt.IncidentKey, receipt.OccurrenceKey = "active-linked", incident.Key, head.Key
	}
	data, err := json.Marshal(receipt)
	if err != nil {
		return FailureIntakeReceipt{}, err
	}
	if err := s.localPhaseLocked("attempts", s.attemptsFD, id+".intake-v1.json", data); err != nil {
		return FailureIntakeReceipt{}, err
	}
	if _, err := s.verifyWriteJournal(); err != nil {
		return FailureIntakeReceipt{}, err
	}
	return receipt, nil
}

func (s *Store) readFailureIntakeLocked(id string) (FailureIntakeReceipt, error) {
	var r FailureIntakeReceipt
	if err := s.readLocalAttemptLocked(id+".intake-v1.json", &r); err != nil {
		return r, err
	}
	result, err := s.readLivenessCompletionLocked(id)
	if err != nil {
		return r, err
	}
	c := result.Completion
	digest, _ := digestJSON(c)
	if r.Schema != localIntakeSchema || r.StoreUUID != s.storeUUID || r.AttemptID != id || r.CompletionSHA256 != digest || r.OutcomeSHA256 != c.OutcomeSHA256 || r.RecordedAtUTC != c.RecordedAtUTC || !validDigest(r.EvidenceSHA256) {
		return r, errors.New("maat memory: intake identity mismatch")
	}
	if err := s.verifyLocalFailureProofLocked(r.EvidenceSHA256); err != nil {
		return r, err
	}
	if eligibleLocalFailure(c) {
		key, _ := localFailureSignature(c.Observation).Digest()
		if r.Disposition != "active-linked" || r.IncidentKey != key || !validDigest(r.OccurrenceKey) {
			return r, errors.New("maat memory: eligible failure lacks its exact intake link")
		}
	} else if r.Disposition != expectedIneligibleDisposition(c) || r.IncidentKey != "" || r.OccurrenceKey != "" {
		return r, errors.New("maat memory: ineligible failure claimed active status")
	}
	if r.Disposition == "active-linked" {
		introduction, err := readExact(s.incidentFD, r.IncidentKey+".json")
		if err != nil {
			return r, err
		}
		if err := s.verifyCommittedBytes("incidents", r.IncidentKey+".json", introduction); err != nil {
			return r, err
		}
		initialKey, err := digestCompactBytes(introduction)
		if err != nil {
			return r, err
		}
		if r.OccurrenceKey != initialKey {
			leaf := r.OccurrenceKey + ".json"
			data, err := readExact(s.transitionFD, leaf)
			if err != nil {
				return r, err
			}
			if err := s.verifyCommittedBytes("transitions", leaf, data); err != nil {
				return r, err
			}
			var event LocalPreventionEvent
			if err := decodeLocalRecord(data, &event); err != nil || event.validate() != nil || event.IncidentKey != r.IncidentKey || event.Kind != "verified-recurrence" || event.Status != IncidentActive || event.RecordedAtUTC.After(c.RecordedAtUTC) {
				return r, errors.New("maat memory: intake does not link an exact active occurrence")
			}
		}
	}
	return r, nil
}

func (s *Store) verifyLocalFailureProofLocked(digest string) error {
	if err := s.verifyEvidence(digest); err != nil {
		return err
	}
	data, err := readExact(s.evidenceFD, digest)
	if err != nil {
		return err
	}
	var proof localFailureProof
	if err := decodeLocalRecord(data, &proof); err != nil {
		return err
	}
	result, err := s.readLivenessCompletionLocked(proof.AttemptID)
	if err != nil {
		return err
	}
	c := result.Completion
	completionDigest, _ := digestJSON(c)
	if proof.Schema != "sirsi.maat.local-managed-watch-failure-proof.v1" || proof.StoreUUID != s.storeUUID || proof.Producer != localWatchProducer || proof.AttemptSHA256 != c.AttemptSHA256 || proof.CompletionSHA256 != completionDigest || proof.OutcomeSHA256 != c.OutcomeSHA256 || proof.PreflightReceiptSHA256 != c.PreflightReceiptSHA256 || proof.Observation != c.Observation || proof.RecordedAtUTC != c.RecordedAtUTC {
		return errors.New("maat memory: local proof does not match retained producer completion")
	}
	return nil
}

func (s *Store) putLocalEvidenceLocked(data []byte) (string, error) {
	if len(data) == 0 || len(data) > maxLocalRecordBytes {
		return "", errors.New("maat memory: local evidence exceeds closed bound")
	}
	digest, err := digestCompactBytes(data)
	if err != nil {
		return "", err
	}
	if err := s.localPhaseLocked("evidence", s.evidenceFD, digest, data); err != nil {
		return "", err
	}
	return digest, nil
}

func (s *Store) writeLocalAttemptLocked(leaf string, value any) error {
	data, err := json.Marshal(value)
	if err != nil || len(data) > maxLocalRecordBytes {
		return errors.New("maat memory: invalid bounded local record")
	}
	return s.createOrVerify("attempts", s.attemptsFD, leaf, data)
}
func (s *Store) readLocalAttemptLocked(leaf string, value any) error {
	data, err := readExact(s.attemptsFD, leaf)
	if err != nil {
		return err
	}
	if err := s.verifyCommittedBytes("attempts", leaf, data); err != nil {
		return err
	}
	return decodeLocalRecord(data, value)
}
func decodeLocalRecord(data []byte, value any) error {
	if len(data) == 0 || len(data) > maxLocalRecordBytes {
		return errors.New("maat memory: local record size refused")
	}
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if err := d.Decode(value); err != nil {
		return err
	}
	var extra any
	if err := d.Decode(&extra); err != io.EOF {
		return errors.New("maat memory: trailing local record")
	}
	canonical, err := json.Marshal(value)
	if err != nil || !bytes.Equal(canonical, data) {
		return errors.New("maat memory: noncanonical local record")
	}
	return nil
}

func localAttemptLeaf(name string) bool {
	for _, suffix := range []string{".invocation-v1.json", ".completion-v1.json", ".intake-v1.json"} {
		if strings.HasSuffix(name, suffix) {
			return validUUID(strings.TrimSuffix(name, suffix))
		}
	}
	return false
}

func digestCompactBytes(data []byte) (string, error) {
	if len(data) == 0 || len(data) > maxLocalRecordBytes {
		return "", errors.New("maat memory: invalid local evidence bytes")
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}
func canonicalCompletionObservation(m LivenessWatchManifest, state string) LivenessWatchObservation {
	if state == "unverifiable" {
		return LivenessWatchObservation{}
	}
	o := LivenessWatchObservation{UID: m.UID, Domain: m.Domain, Label: m.ManagedLabel, PlistPath: m.TargetPlistPath, Postcondition: state}
	if state == "exact" || state == "known-unloaded" {
		o.PlistSHA256 = m.ExpectedPlistSHA256
	}
	o.LaunchdLoaded = state == "exact"
	return o
}

// Completion normalization means a retained witness can reconstruct ONLY its
// exact already-hashed outcome after a crash between those writes. It cannot
// infer a missing invocation result from a later healthy observation.
func completionOutcome(c LivenessCompletion, a LivenessWatchAttempt, claim livenessInvocation) (LivenessWatchOutcome, error) {
	if c.Observation != "exact" && c.Observation != "known-absent" && c.Observation != "known-unloaded" && c.Observation != "unverifiable" {
		return LivenessWatchOutcome{}, errors.New("maat memory: unsupported completion observation")
	}
	if (c.Mode == "direct-install" && (claim.Mode != "direct-install" || (c.Invocation != "returned-success" && c.Invocation != "returned-error"))) || (c.Mode == "reobserve-only" && (claim.Mode != "reobserve-only" || c.Invocation != "not-invoked")) || (c.Mode != "direct-install" && c.Mode != "reobserve-only") {
		return LivenessWatchOutcome{}, errors.New("maat memory: invocation mode mismatch")
	}
	errorText := ""
	if c.Invocation == "returned-error" {
		errorText = "installer returned an error"
	}
	if c.Invocation == "not-invoked" {
		errorText = "installer was not invoked"
	}
	if c.Observation == "unverifiable" {
		errorText += "; exact post-observation unavailable"
	}
	outcome := LivenessWatchOutcome{Schema: MemorySchema, AttemptID: a.ID, RecordedAtUTC: c.RecordedAtUTC, Determination: "incomplete", InstallerError: errorText, Observation: canonicalCompletionObservation(a.Manifest, c.Observation)}
	if c.Invocation == "returned-success" && c.Observation == "exact" {
		outcome.Determination = "resolved"
	}
	digest, _ := digestJSON(outcome)
	observed, _ := digestJSON(outcome.Observation)
	if digest != c.OutcomeSHA256 || observed != c.ObservationSHA256 || validateLivenessOutcome(outcome, a) != nil {
		return LivenessWatchOutcome{}, errors.New("maat memory: completion cannot reproduce its exact outcome")
	}
	return outcome, nil
}

func (s *Store) ReconcileLivenessWatchCompletion(id string) (LivenessDirectResult, error) {
	if !validUUID(id) {
		return LivenessDirectResult{}, errors.New("maat memory: invalid completion reconciliation id")
	}
	if err := s.lock(unix.LOCK_EX); err != nil {
		return LivenessDirectResult{}, err
	}
	defer s.unlock()
	var a LivenessWatchAttempt
	var c LivenessCompletion
	var claim livenessInvocation
	if err := s.readLocalAttemptLocked(id+".started.json", &a); err != nil {
		return LivenessDirectResult{}, err
	}
	if err := s.readLocalAttemptLocked(id+".invocation-v1.json", &claim); err != nil {
		return LivenessDirectResult{}, err
	}
	witnessBytes, err := readExact(s.attemptsFD, id+".completion-v1.json")
	if err != nil {
		return LivenessDirectResult{}, err
	}
	if err := decodeLocalRecord(witnessBytes, &c); err != nil {
		return LivenessDirectResult{}, err
	}
	// Exact readback below validates the full provenance after the one known
	// outcome write has finished; no installer or observer callback is present.
	if err := s.validateCompletionBindingsLocked(c, a, claim); err != nil {
		return LivenessDirectResult{}, err
	}
	outcome, err := completionOutcome(c, a, claim)
	if err != nil {
		return LivenessDirectResult{}, err
	}
	if err := s.localPhaseLocked("attempts", s.attemptsFD, id+".completion-v1.json", witnessBytes); err != nil {
		return LivenessDirectResult{}, err
	}
	data, err := json.Marshal(outcome)
	if err != nil {
		return LivenessDirectResult{}, err
	}
	if err := s.localPhaseLocked("attempts", s.attemptsFD, id+".outcome.json", data); err != nil {
		return LivenessDirectResult{}, err
	}
	if _, err := s.verifyWriteJournal(); err != nil {
		return LivenessDirectResult{}, err
	}
	return s.readLivenessCompletionLocked(id)
}

func (s *Store) CloseLivenessWatchBeforeInvocation(a LivenessWatchAttempt, observe func(LivenessWatchManifest) (LivenessWatchObservation, error)) (LivenessDirectResult, error) {
	if observe == nil {
		return LivenessDirectResult{}, errors.New("maat memory: bounded observer required")
	}
	if err := s.lock(unix.LOCK_EX); err != nil {
		return LivenessDirectResult{}, err
	}
	if _, err := s.verifyWriteJournal(); err != nil {
		s.unlock()
		return LivenessDirectResult{}, err
	}
	if err := s.verifyStartedAttemptLocked(a); err != nil {
		s.unlock()
		return LivenessDirectResult{}, err
	}
	if _, err := readExact(s.attemptsFD, a.ID+".invocation-v1.json"); err == nil {
		s.unlock()
		return LivenessDirectResult{}, errors.New("maat memory: invocation already claimed; no non-invocation assertion is possible")
	} else if !errors.Is(err, unix.ENOENT) {
		s.unlock()
		return LivenessDirectResult{}, err
	}
	digest, _ := digestJSON(a)
	host, hostErr := os.Hostname()
	if hostErr != nil || strings.TrimSpace(host) == "" || len(host) > 256 {
		s.unlock()
		return LivenessDirectResult{}, errors.New("maat memory: local host metadata unavailable")
	}
	claim := livenessInvocation{Schema: "sirsi.maat.liveness-invocation.v1", StoreUUID: s.storeUUID, AttemptID: a.ID, AttemptSHA256: digest, StartedAtUTC: time.Now().UTC(), Mode: "reobserve-only", Host: host}
	if err := s.writeLocalAttemptLocked(a.ID+".invocation-v1.json", claim); err != nil {
		s.unlock()
		return LivenessDirectResult{}, err
	}
	s.unlock()
	observation, err := observe(a.Manifest)
	return s.persistLivenessCompletion(a, claim, "not-invoked", "", errors.New("installer was not invoked"), observation, err)
}

func (s *Store) validateCompletionBindingsLocked(c LivenessCompletion, a LivenessWatchAttempt, claim livenessInvocation) error {
	aDigest, _ := digestJSON(a)
	if validateLivenessAttempt(a, a.ID, s.storeUUID) != nil || a.CompletionProtocolVersion != 1 || c.Schema != localCompletionSchema || c.StoreUUID != s.storeUUID || c.AttemptID != a.ID || c.AttemptSHA256 != aDigest || c.ActionManifestSHA256 != a.ActionManifestSHA256 || c.PreflightReceiptSHA256 != a.PreflightReceiptSHA256 || c.Producer != localWatchProducer || c.Scope != localWatchScope() || c.OwnerUID != a.Manifest.UID || c.Host != claim.Host || c.Host == "" || len(c.Host) > 256 || c.ManagedLabel != a.Manifest.ManagedLabel || c.TargetPlistPath != a.Manifest.TargetPlistPath || c.ExpectedPlistSHA256 != a.Manifest.ExpectedPlistSHA256 || claim.Schema != "sirsi.maat.liveness-invocation.v1" || claim.StoreUUID != s.storeUUID || claim.AttemptID != a.ID || claim.AttemptSHA256 != aDigest || claim.StartedAtUTC.Before(a.StartedAtUTC) || c.RecordedAtUTC.Before(claim.StartedAtUTC) {
		return errors.New("maat memory: completion does not bind the exact invocation and target")
	}
	bytes, err := readExact(s.evidenceFD, c.PreflightReceiptSHA256)
	if err != nil {
		return err
	}
	digest := sha256.Sum256(bytes)
	if hex.EncodeToString(digest[:]) != c.PreflightReceiptSHA256 {
		return errors.New("maat memory: originating preflight hash mismatch")
	}
	if err := s.verifyCommittedBytes("evidence", c.PreflightReceiptSHA256, bytes); err != nil {
		return err
	}
	r, err := decodePreflightReceipt(bytes)
	if err != nil || r.RegistrySnapshotVersion != 2 || r.Action != localWatchScope() || r.ActionManifestSHA256 != a.ActionManifestSHA256 || r.Decision != PreflightPass {
		return errors.New("maat memory: originating preflight is not the exact admitted action")
	}
	return nil
}

func (s *Store) localAttemptSettledLocked(a LivenessWatchAttempt) (bool, error) {
	result, err := s.readLivenessCompletionLocked(a.ID)
	if errors.Is(err, unix.ENOENT) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	intake, err := s.readFailureIntakeLocked(a.ID)
	if errors.Is(err, unix.ENOENT) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	c := result.Completion
	known := (c.Invocation == "returned-success" && c.Observation == "exact" && intake.Disposition == "not-eligible") ||
		(eligibleLocalFailure(c) && intake.Disposition == "active-linked") ||
		(c.Mode == "reobserve-only" && c.Invocation == "not-invoked" && intake.Disposition == "not-eligible")
	if !known {
		return false, nil
	}
	var debt LivenessWatchProjectionDebt
	var receipt LivenessWatchProjectionReceipt
	if err := s.readLocalAttemptLocked(a.ID+".projection.json", &debt); err != nil {
		if errors.Is(err, unix.ENOENT) {
			return false, nil
		}
		return false, err
	}
	if err := s.readLocalAttemptLocked(a.ID+".projection-receipt.json", &receipt); err != nil {
		if errors.Is(err, unix.ENOENT) {
			return false, nil
		}
		return false, err
	}
	if validateLivenessProjectionDebt(debt, s.storeUUID) != nil || digestEqual(debt.Attempt, a) != nil || digestEqual(debt.Outcome, result.Outcome) != nil {
		return false, errors.New("maat memory: completion projection does not bind this exact attempt/outcome")
	}
	digest, _ := digestJSON(debt)
	if receipt.Schema != LivenessWatchProjectionSchema || receipt.AttemptID != a.ID || receipt.DebtSHA256 != digest || receipt.JournalKey != debt.Decision.Evidence || receipt.ReadBackAtUTC.IsZero() {
		return false, errors.New("maat memory: projection receipt changed")
	}
	j, err := NewDefaultDecisionJournal()
	if err != nil {
		return false, err
	}
	ok, err := decisionEvidenceReadback(j.Path, debt.Decision)
	if err != nil || !ok {
		return false, errors.New("maat memory: exact terminal Casebook readback is unavailable")
	}
	return true, nil
}

func (s *Store) validateLocalAttemptNamespaceLocked(names []string, started map[string]LivenessWatchAttempt, outcomes map[string]LivenessWatchOutcome) error {
	for _, name := range names {
		if localDispositionAttemptLeaf(name) {
			if strings.HasSuffix(name, ".disposition-v1.json") {
				if _, err := s.readDispositionIntentLocked(strings.TrimSuffix(name, ".disposition-v1.json")); err != nil {
					return err
				}
			}
			if strings.HasSuffix(name, ".disposition-complete-v1.json") {
				digest := strings.TrimSuffix(name, ".disposition-complete-v1.json")
				intent, err := s.readDispositionIntentLocked(digest)
				if err != nil {
					return err
				}
				_, _, _, expected, err := s.localDispositionRecords(intent)
				if err != nil {
					return err
				}
				var actual LocalPreventionDispositionReceipt
				if err := s.readLocalAttemptLocked(name, &actual); err != nil || digestEqual(actual, expected) != nil {
					return errors.New("maat memory: invalid local disposition completion")
				}
			}
			continue
		}
		if !localAttemptLeaf(name) {
			continue
		}
		id := strings.SplitN(name, ".", 2)[0]
		a, ok := started[id]
		if !ok || a.CompletionProtocolVersion != 1 {
			return errors.New("maat memory: orphan local completion/intake record")
		}
		switch {
		case strings.HasSuffix(name, ".invocation-v1.json"):
			var claim livenessInvocation
			if err := s.readLocalAttemptLocked(name, &claim); err != nil {
				return err
			}
			digest, _ := digestJSON(a)
			if claim.Schema != "sirsi.maat.liveness-invocation.v1" || claim.StoreUUID != s.storeUUID || claim.AttemptID != id || claim.AttemptSHA256 != digest || claim.StartedAtUTC.Before(a.StartedAtUTC) || (claim.Mode != "direct-install" && claim.Mode != "reobserve-only") {
				return errors.New("maat memory: invalid local invocation claim")
			}
		case strings.HasSuffix(name, ".completion-v1.json"):
			var c LivenessCompletion
			var claim livenessInvocation
			if err := s.readLocalAttemptLocked(name, &c); err != nil {
				return err
			}
			if err := s.readLocalAttemptLocked(id+".invocation-v1.json", &claim); err != nil {
				return err
			}
			if err := s.validateCompletionBindingsLocked(c, a, claim); err != nil {
				return err
			}
			expected, err := completionOutcome(c, a, claim)
			if err != nil {
				return err
			}
			if actual, exists := outcomes[id]; exists && digestEqual(actual, expected) != nil {
				return errors.New("maat memory: local completion/outcome mismatch")
			}
		case strings.HasSuffix(name, ".intake-v1.json"):
			if _, err := s.readFailureIntakeLocked(id); err != nil {
				return err
			}
		}
	}
	return nil
}

// Deterministic repair projection can be recovered from retained producer
// facts without a new observation, timestamp, consent or installer invocation.
func LivenessCompletionDecision(result LivenessDirectResult) (Decision, error) {
	c := result.Completion
	determination := "failed"
	if result.Outcome.Determination == "resolved" && c.Invocation == "returned-success" && c.Observation == "exact" {
		determination = "resolved"
	}
	decision, err := BuildDiagnosticRepairDecision("sirsi maat repair liveness-watch", DiagnosticRepair{Check: "liveness-watch", Operation: "install the exact reviewed managed per-user liveness LaunchAgent from the canonical Sirsi executable", Before: "reviewed local managed-watch action " + c.ActionManifestSHA256, After: "retained exact postcondition " + c.Observation, Determination: determination, Detail: "attempt " + c.AttemptID + "; invocation " + c.Invocation + "; historical outcome " + result.Outcome.Determination})
	if err != nil {
		return Decision{}, err
	}
	decision.Time = c.RecordedAtUTC.Format(time.RFC3339Nano)
	decision.Host = c.Host
	decision.Evidence = "maat-liveness-watch-attempt:" + c.AttemptID + ":" + decision.Evidence
	return decision, nil
}

func expectedIneligibleDisposition(c LivenessCompletion) string {
	if c.Invocation == "returned-error" || (c.Observation == "unverifiable" && c.Invocation != "not-invoked") {
		return "unknown"
	}
	return "not-eligible"
}

// No installer/observer callbacks exist here. Only retained canonical
// completions and their same-key intake phases can be reconciled. Unknown
// invocations remain pending, and unrelated WAL debt still refuses at exit.
func (s *Store) ReconcileLivenessWatchData() error {
	if err := s.lock(unix.LOCK_SH); err != nil {
		return err
	}
	names, err := readDirectoryNames(s.attemptsFD)
	if err != nil {
		s.unlock()
		return err
	}
	ids := []string{}
	for _, name := range names {
		if strings.HasSuffix(name, ".started.json") {
			var a LivenessWatchAttempt
			if err := s.readLocalAttemptLocked(name, &a); err != nil {
				s.unlock()
				return err
			}
			if validateLivenessAttempt(a, a.ID, s.storeUUID) != nil || name != a.ID+".started.json" {
				s.unlock()
				return errors.New("maat memory: invalid retained reconciliation candidate")
			}
			if a.CompletionProtocolVersion == 1 {
				ids = append(ids, a.ID)
			}
		}
	}
	s.unlock()
	for _, id := range ids {
		_, _ = s.ReconcileLivenessWatchCompletion(id)
		_, _ = s.RememberLivenessFailure(id)
	}
	if err := s.lock(unix.LOCK_SH); err != nil {
		return err
	}
	defer s.unlock()
	if _, err := s.verifyWriteJournal(); err != nil {
		return err
	}
	if _, err := s.pendingLivenessWatchAttemptRowsLocked(); err != nil && !errors.Is(err, ErrNoIncompleteLivenessWatchAttempt) {
		return err
	}
	return nil
}
