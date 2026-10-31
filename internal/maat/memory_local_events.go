package maat

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"golang.org/x/sys/unix"
)

const localPreventionEventSchema = "sirsi.maat.local-prevention-event.v1"

// Field order is the contract. EventKey hashes this same ordered object with
// EventKey omitted; it never replaces the stable signature/incident key.
type LocalPreventionEvent struct {
	Schema                string         `json:"schema"`
	EventKey              string         `json:"event_key"`
	IncidentKey           string         `json:"incident_key"`
	Scope                 Scope          `json:"scope"`
	PredecessorEventKey   string         `json:"predecessor_event_key"`
	PredecessorBodySHA256 string         `json:"predecessor_body_sha256"`
	Kind                  string         `json:"kind"`
	Status                IncidentStatus `json:"status"`
	Reason                string         `json:"reason"`
	EvidenceSHA256        string         `json:"evidence_sha256"`
	OwnerReviewEvidence   string         `json:"owner_review_evidence"`
	RecordedAtUTC         time.Time      `json:"recorded_at_utc"`
}

type localEventIdentity struct {
	Schema                string         `json:"schema"`
	IncidentKey           string         `json:"incident_key"`
	Scope                 Scope          `json:"scope"`
	PredecessorEventKey   string         `json:"predecessor_event_key"`
	PredecessorBodySHA256 string         `json:"predecessor_body_sha256"`
	Kind                  string         `json:"kind"`
	Status                IncidentStatus `json:"status"`
	Reason                string         `json:"reason"`
	EvidenceSHA256        string         `json:"evidence_sha256"`
	OwnerReviewEvidence   string         `json:"owner_review_evidence"`
	RecordedAtUTC         time.Time      `json:"recorded_at_utc"`
}

func (e LocalPreventionEvent) identity() localEventIdentity {
	return localEventIdentity{e.Schema, e.IncidentKey, e.Scope, e.PredecessorEventKey, e.PredecessorBodySHA256, e.Kind, e.Status, e.Reason, e.EvidenceSHA256, e.OwnerReviewEvidence, e.RecordedAtUTC}
}
func (e LocalPreventionEvent) validate() error {
	digest, err := digestJSON(e.identity())
	if err != nil || digest != e.EventKey || e.Schema != localPreventionEventSchema || !validDigest(e.IncidentKey) || e.Scope != localWatchScope() || !validDigest(e.PredecessorEventKey) || !validDigest(e.PredecessorBodySHA256) || !validDigest(e.EvidenceSHA256) || e.RecordedAtUTC.IsZero() {
		return errors.New("maat memory: invalid local event identity")
	}
	if e.Kind == "owner-disposition" && e.Status == IncidentRetired && e.Reason == "owner-accepted-not-repaired" && strings.HasPrefix(e.OwnerReviewEvidence, "diagnostic-finding:sha256=") && validDigest(strings.TrimPrefix(e.OwnerReviewEvidence, "diagnostic-finding:sha256=")) {
		return nil
	}
	if e.Kind == "verified-recurrence" && e.Status == IncidentActive && e.Reason == "verified-local-failure" && e.OwnerReviewEvidence == "" {
		return nil
	}
	return errors.New("maat memory: unsupported local event status or effect")
}

type LocalPreventionHead struct {
	Key            string         `json:"key"`
	BodySHA256     string         `json:"body_sha256"`
	Status         IncidentStatus `json:"status"`
	EvidenceSHA256 string         `json:"evidence_sha256"`
	RecordedAtUTC  time.Time      `json:"recorded_at_utc"`
}

type LivenessFailureView struct {
	Schema                 string                              `json:"schema"`
	StoreUUID              string                              `json:"store_uuid"`
	Incident               Incident                            `json:"incident"`
	Head                   LocalPreventionHead                 `json:"head"`
	Events                 []LocalPreventionEvent              `json:"events"`
	Completion             LivenessCompletion                  `json:"completion"`
	Outcome                LivenessWatchOutcome                `json:"historical_outcome"`
	PreflightReceiptSHA256 string                              `json:"preflight_receipt_sha256"`
	OriginatingPreflight   PreflightReceipt                    `json:"originating_preflight"`
	CurrentFactReference   string                              `json:"current_fact_reference"`
	Prevention             string                              `json:"prevention"`
	Health                 string                              `json:"health"`
	Dispositions           []LocalPreventionDispositionReceipt `json:"dispositions"`
	ActionAuthorized       bool                                `json:"action_authorized"`
}

func localManagedWatchIncident(i Incident) bool {
	if i.Scope != localWatchScope() || i.GuardID != ActiveIncidentGuardID || i.GuardVersion != ActiveIncidentGuardVersion {
		return false
	}
	return i.Signature == localFailureSignature("known-absent") || i.Signature == localFailureSignature("known-unloaded")
}

func (s *Store) localEventsLocked() ([]LocalPreventionEvent, error) {
	names, err := readDirectoryNames(s.transitionFD)
	if err != nil {
		return nil, err
	}
	events := []LocalPreventionEvent{}
	for _, name := range names {
		data, err := readExact(s.transitionFD, name)
		if err != nil {
			return nil, err
		}
		if err := s.verifyCommittedBytes("transitions", name, data); err != nil {
			return nil, err
		}
		var envelope struct {
			Schema string `json:"schema"`
		}
		if err := json.Unmarshal(data, &envelope); err != nil {
			return nil, err
		}
		if envelope.Schema == MemorySchema {
			continue
		} // Existing legacy decoder owns these records.
		var event LocalPreventionEvent
		if err := decodeLocalRecord(data, &event); err != nil || event.validate() != nil || name != event.EventKey+".json" {
			return nil, errors.New("maat memory: invalid or unsupported local transition")
		}
		events = append(events, event)
	}
	return events, nil
}

func (s *Store) localFamilyLocked(i Incident, events []LocalPreventionEvent) (LocalPreventionHead, []LocalPreventionEvent, error) {
	if !localManagedWatchIncident(i) || i.Status != IncidentActive || i.validate() != nil {
		return LocalPreventionHead{}, nil, errors.New("maat memory: unsupported local incident family")
	}
	data, err := readExact(s.incidentFD, i.Key+".json")
	if err != nil {
		return LocalPreventionHead{}, nil, err
	}
	if err := s.verifyCommittedBytes("incidents", i.Key+".json", data); err != nil {
		return LocalPreventionHead{}, nil, err
	}
	bodyDigest, err := digestCompactBytes(data)
	if err != nil {
		return LocalPreventionHead{}, nil, err
	}
	if err := s.verifyEligibleLocalProofLocked(i.EvidenceSHA256, i.Key); err != nil {
		return LocalPreventionHead{}, nil, err
	}
	proofBytes, err := readExact(s.evidenceFD, i.EvidenceSHA256)
	if err != nil {
		return LocalPreventionHead{}, nil, err
	}
	var initialProof localFailureProof
	if err := decodeLocalRecord(proofBytes, &initialProof); err != nil {
		return LocalPreventionHead{}, nil, err
	}
	head := LocalPreventionHead{Key: bodyDigest, BodySHA256: bodyDigest, Status: IncidentActive, EvidenceSHA256: i.EvidenceSHA256, RecordedAtUTC: initialProof.RecordedAtUTC}
	children := map[string]LocalPreventionEvent{}
	count := 0
	for _, event := range events {
		if event.IncidentKey == i.Key {
			if _, exists := children[event.PredecessorEventKey]; exists {
				return head, nil, errors.New("maat memory: forked local prevention chain")
			}
			children[event.PredecessorEventKey] = event
			count++
		}
	}
	chain := []LocalPreventionEvent{}
	visited := map[string]bool{head.Key: true}
	for {
		event, exists := children[head.Key]
		if !exists {
			break
		}
		if event.PredecessorBodySHA256 != head.BodySHA256 || event.Scope != i.Scope || visited[event.EventKey] || event.RecordedAtUTC.Before(head.RecordedAtUTC) {
			return head, nil, errors.New("maat memory: local event predecessor/body mismatch or cycle")
		}
		if event.Kind == "owner-disposition" {
			if head.Status != IncidentActive {
				return head, nil, errors.New("maat memory: local disposition requires active occurrence")
			}
			if err := s.verifyLocalOwnerDispositionProofLocked(event); err != nil {
				return head, nil, err
			}
		} else {
			if head.Status != IncidentRetired {
				return head, nil, errors.New("maat memory: recurrence must follow exact retired occurrence")
			}
			if err := s.verifyEligibleLocalProofLocked(event.EvidenceSHA256, i.Key); err != nil {
				return head, nil, err
			}
		}
		encoded, err := json.Marshal(event)
		if err != nil {
			return head, nil, err
		}
		bodyDigest, err := digestCompactBytes(encoded)
		if err != nil {
			return head, nil, err
		}
		head = LocalPreventionHead{Key: event.EventKey, BodySHA256: bodyDigest, Status: event.Status, EvidenceSHA256: event.EvidenceSHA256, RecordedAtUTC: event.RecordedAtUTC}
		visited[event.EventKey] = true
		chain = append(chain, event)
	}
	if len(chain) != count {
		return head, nil, errors.New("maat memory: local prevention chain has an orphan or foreign predecessor")
	}
	return head, chain, nil
}

func (s *Store) verifyEligibleLocalProofLocked(digest, incidentKey string) error {
	if err := s.verifyLocalFailureProofLocked(digest); err != nil {
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
	key, _ := localFailureSignature(result.Completion.Observation).Digest()
	if !eligibleLocalFailure(result.Completion) || key != incidentKey {
		return errors.New("maat memory: proof is not the exact verified local failure family")
	}
	return nil
}

func (s *Store) linkLocalFailureLocked(i Incident, at time.Time) (LocalPreventionHead, error) {
	expected, err := json.Marshal(i)
	if err != nil {
		return LocalPreventionHead{}, err
	}
	existing, readErr := readExact(s.incidentFD, i.Key+".json")
	if errors.Is(readErr, unix.ENOENT) || readErr == nil && bytes.Equal(existing, expected) {
		if err := s.localPhaseLocked("incidents", s.incidentFD, i.Key+".json", expected); err != nil {
			return LocalPreventionHead{}, err
		}
	} else if readErr != nil {
		return LocalPreventionHead{}, readErr
	}
	data, err := readExact(s.incidentFD, i.Key+".json")
	if err != nil {
		return LocalPreventionHead{}, err
	}
	var original Incident
	if err := json.Unmarshal(data, &original); err != nil {
		return LocalPreventionHead{}, err
	}
	events, pending, err := s.localEventsForIntakeLocked(i.Key, i.EvidenceSHA256, at)
	if err != nil {
		return LocalPreventionHead{}, err
	}
	head, _, err := s.localFamilyLocked(original, events)
	if err != nil {
		return head, err
	}
	if head.Status == IncidentActive {
		if pending != nil {
			return head, errors.New("maat memory: pending recurrence does not follow the current retired head")
		}
		return head, nil
	}
	if !at.After(head.RecordedAtUTC) {
		return head, errors.New("maat memory: a recurrence must be independently observed after the prior disposition")
	}
	event := LocalPreventionEvent{Schema: localPreventionEventSchema, IncidentKey: i.Key, Scope: i.Scope, PredecessorEventKey: head.Key, PredecessorBodySHA256: head.BodySHA256, Kind: "verified-recurrence", Status: IncidentActive, Reason: "verified-local-failure", EvidenceSHA256: i.EvidenceSHA256, RecordedAtUTC: at}
	event.EventKey, err = digestJSON(event.identity())
	if err != nil {
		return head, err
	}
	if pending != nil && digestEqual(*pending, event) != nil {
		return head, errors.New("maat memory: interrupted recurrence tuple differs from its retained failure")
	}
	if err := s.appendLocalEventLocked(event); err != nil {
		return head, err
	}
	encoded, _ := json.Marshal(event)
	digest, err := digestCompactBytes(encoded)
	if err != nil {
		return head, err
	}
	return LocalPreventionHead{Key: event.EventKey, BodySHA256: digest, Status: IncidentActive, EvidenceSHA256: i.EvidenceSHA256, RecordedAtUTC: at}, nil
}

func (s *Store) appendLocalEventLocked(e LocalPreventionEvent) error {
	if err := e.validate(); err != nil {
		return err
	}
	data, err := json.Marshal(e)
	if err != nil || len(data) > maxLocalRecordBytes {
		return errors.New("maat memory: local event exceeds bound")
	}
	return s.createOrVerify("transitions", s.transitionFD, e.EventKey+".json", data)
}

func (s *Store) ReadLivenessFailure(key string) (LivenessFailureView, error) {
	if err := s.lock(unix.LOCK_SH); err != nil {
		return LivenessFailureView{}, err
	}
	defer s.unlock()
	return s.readLivenessFailureLocked(key)
}
func (s *Store) readLivenessFailureLocked(key string) (LivenessFailureView, error) {
	if !validDigest(key) || !validUUID(s.storeUUID) {
		return LivenessFailureView{}, errors.New("maat memory: initialized local incident identity required")
	}
	if _, err := s.verifyWriteJournal(); err != nil {
		return LivenessFailureView{}, err
	}
	data, err := readExact(s.incidentFD, key+".json")
	if err != nil {
		return LivenessFailureView{}, err
	}
	var i Incident
	if err := json.Unmarshal(data, &i); err != nil || i.Key != key {
		return LivenessFailureView{}, errors.New("maat memory: local incident key mismatch")
	}
	events, err := s.localEventsLocked()
	if err != nil {
		return LivenessFailureView{}, err
	}
	head, chain, err := s.localFamilyLocked(i, events)
	if err != nil {
		return LivenessFailureView{}, err
	}
	// A retired head references disposition proof; its prior active head's
	// failure proof remains the historical observation, never healthy.
	proofDigest := i.EvidenceSHA256
	for _, event := range chain {
		if event.Kind == "verified-recurrence" {
			proofDigest = event.EvidenceSHA256
		}
	}
	proofBytes, err := readExact(s.evidenceFD, proofDigest)
	if err != nil {
		return LivenessFailureView{}, err
	}
	var proof localFailureProof
	if err := decodeLocalRecord(proofBytes, &proof); err != nil {
		return LivenessFailureView{}, err
	}
	result, err := s.readLivenessCompletionLocked(proof.AttemptID)
	if err != nil {
		return LivenessFailureView{}, err
	}
	originatingPreflight, err := s.readPreflightReceiptObjectLocked(proof.PreflightReceiptSHA256)
	if err != nil {
		return LivenessFailureView{}, err
	}
	history, err := s.localDispositionHistoryLocked(key)
	if err != nil {
		return LivenessFailureView{}, err
	}
	prevention := "active"
	if head.Status == IncidentRetired {
		prevention = "accepted-not-repaired"
	}
	return LivenessFailureView{Schema: "sirsi.maat.local-managed-watch-failure-read.v1", StoreUUID: s.storeUUID, Incident: i, Head: head, Events: chain, Completion: result.Completion, Outcome: result.Outcome, PreflightReceiptSHA256: proof.PreflightReceiptSHA256, OriginatingPreflight: originatingPreflight, CurrentFactReference: "unavailable:historical-read-does-not-observe-current-health", Dispositions: history, Prevention: prevention, Health: "historical-known-unhealthy; current-health-unobserved"}, nil
}

// Only a retained direct failure can supply this exact pending recurrence
// tuple. All other pending writes remain unverifiable; original introductions
// and committed chain nodes are never overwritten to finish an intake.
func (s *Store) localEventsForIntakeLocked(key, proof string, at time.Time) ([]LocalPreventionEvent, *LocalPreventionEvent, error) {
	names, err := readDirectoryNames(s.transitionFD)
	if err != nil {
		return nil, nil, err
	}
	events := []LocalPreventionEvent{}
	var pending *LocalPreventionEvent
	for _, name := range names {
		data, err := readExact(s.transitionFD, name)
		if err != nil {
			return nil, nil, err
		}
		var schema struct {
			Schema string `json:"schema"`
		}
		if err := json.Unmarshal(data, &schema); err != nil {
			return nil, nil, err
		}
		if schema.Schema == MemorySchema {
			if err := s.verifyCommittedBytes("transitions", name, data); err != nil {
				return nil, nil, err
			}
			continue
		}
		var event LocalPreventionEvent
		if err := decodeLocalRecord(data, &event); err != nil || event.validate() != nil || name != event.EventKey+".json" {
			return nil, nil, errors.New("maat memory: malformed intake chain event")
		}
		if s.verifyCommittedBytes("transitions", name, data) == nil {
			events = append(events, event)
			continue
		}
		if pending != nil || event.IncidentKey != key || event.Kind != "verified-recurrence" || event.EvidenceSHA256 != proof || !event.RecordedAtUTC.Equal(at) {
			return nil, nil, errors.New("maat memory: unrelated pending transition cannot be reconciled")
		}
		intent, err := newWriteIntent("transitions", name, data)
		if err != nil {
			return nil, nil, err
		}
		expected, _ := json.Marshal(intent)
		prepared, err := readExact(s.writesFD, intent.Key+".prepared.json")
		if err != nil || !bytes.Equal(prepared, expected) {
			return nil, nil, errors.New("maat memory: recurrence has no exact prepared tuple")
		}
		if _, err := s.verifyWriteJournalAllow(intent.Key); err != nil {
			return nil, nil, err
		}
		copy := event
		pending = &copy
	}
	return events, pending, nil
}

func (s *Store) ReadLocalPreventionDisposition(key string) (LivenessFailureView, error) {
	return s.ReadLivenessFailure(key)
}
