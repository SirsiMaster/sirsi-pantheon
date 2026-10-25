package maat

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"golang.org/x/sys/unix"
)

const localDispositionSchema = "sirsi.maat.local-prevention-disposition-plan.v1"

type LocalPreventionDispositionPlan struct {
	Schema                 string    `json:"schema"`
	StoreUUID              string    `json:"store_uuid"`
	IncidentKey            string    `json:"incident_key"`
	OccurrenceKey          string    `json:"occurrence_key"`
	OccurrenceBodySHA256   string    `json:"occurrence_body_sha256"`
	Scope                  Scope     `json:"scope"`
	FailureEvidenceSHA256  string    `json:"failure_evidence_sha256"`
	PreflightReceiptSHA256 string    `json:"preflight_receipt_sha256"`
	OwnerReviewEvidence    string    `json:"owner_review_evidence"`
	Effect                 string    `json:"effect"`
	Conclusion             string    `json:"conclusion"`
	CurrentFactReference   string    `json:"current_fact_reference"`
	ReviewedAtUTC          time.Time `json:"reviewed_at_utc"`
}

type LocalPreventionDispositionPreview struct {
	Schema       string                         `json:"schema"`
	Plan         LocalPreventionDispositionPlan `json:"plan"`
	PlanSHA256   string                         `json:"plan_sha256"`
	CanCommit    bool                           `json:"can_commit"`
	Constraints  []string                       `json:"constraints"`
	Confirmation string                         `json:"confirmation"`
}

type localDispositionIntent struct {
	Schema         string                         `json:"schema"`
	PlanSHA256     string                         `json:"plan_sha256"`
	Plan           LocalPreventionDispositionPlan `json:"plan"`
	Host           string                         `json:"host"`
	ConfirmedAtUTC time.Time                      `json:"confirmed_at_utc"`
}

type localDispositionProof struct {
	Schema          string   `json:"schema"`
	PlanSHA256      string   `json:"plan_sha256"`
	IntentSHA256    string   `json:"intent_sha256"`
	OwnerReview     Decision `json:"owner_review"`
	OwnerAcceptance Decision `json:"owner_acceptance"`
}

type LocalPreventionDispositionReceipt struct {
	Schema                  string    `json:"schema"`
	PlanSHA256              string    `json:"plan_sha256"`
	IncidentKey             string    `json:"incident_key"`
	OccurrenceKey           string    `json:"occurrence_key"`
	OccurrenceBodySHA256    string    `json:"occurrence_body_sha256"`
	Effect                  string    `json:"effect"`
	Conclusion              string    `json:"conclusion"`
	OwnerReviewEvidence     string    `json:"owner_review_evidence"`
	OwnerAcceptanceEvidence string    `json:"owner_acceptance_evidence"`
	EventKey                string    `json:"event_key,omitempty"`
	ProjectionEvidence      string    `json:"projection_evidence"`
	RecordedAtUTC           time.Time `json:"recorded_at_utc"`
	Health                  string    `json:"health"`
	ActionAuthorized        bool      `json:"action_authorized"`
}

const localLiftConfirmation = "Accept this issue without claiming repair and stop only this named local prevention from blocking a new review. The observed issue may remain. No repair runs, and no unknown attempt or other safeguard is cleared."

func localDispositionReview(p LocalPreventionDispositionPlan, origin string) (Decision, error) {
	// The review digest binds the exact occurrence, effect, conclusion, store,
	// origin and captured review time. It cannot alias ordinary acceptance.
	copy := p
	copy.OwnerReviewEvidence = ""
	detail, err := json.Marshal(copy)
	if err != nil {
		return Decision{}, err
	}
	return buildDiagnosticOwnerReview("sirsi maat local prevention", DiagnosticOwnerReview{
		Check: "local-managed-watch-prevention", Message: "Accept this issue without claiming repair; effect " + p.Effect,
		Detail: string(detail), OriginEvidence: origin,
	})
}

func (p LocalPreventionDispositionPlan) validate() error {
	if p.Schema != localDispositionSchema || !validUUID(p.StoreUUID) || !validDigest(p.IncidentKey) || !validDigest(p.OccurrenceKey) || !validDigest(p.OccurrenceBodySHA256) || p.Scope != localWatchScope() || !validDigest(p.FailureEvidenceSHA256) || !validDigest(p.PreflightReceiptSHA256) || p.ReviewedAtUTC.IsZero() || p.CurrentFactReference != "unavailable:historical-read-does-not-observe-current-health" || strings.TrimSpace(p.Conclusion) != p.Conclusion || p.Conclusion == "" || len(p.Conclusion) > 4096 || strings.ContainsRune(p.Conclusion, 0) || (p.Effect != "accept-only" && p.Effect != "retire-local-prevention") {
		return errors.New("maat memory: invalid exact local disposition plan")
	}
	if !strings.HasPrefix(p.OwnerReviewEvidence, "diagnostic-finding:sha256=") || !validDigest(strings.TrimPrefix(p.OwnerReviewEvidence, "diagnostic-finding:sha256=")) {
		return errors.New("maat memory: invalid owner review reference")
	}
	return nil
}

// Preview retains only an immutable data proposal. It never accepts an issue,
// retires an incident, changes health or launches any installer. All scope and
// authority fields are derived from initialized same-store historical proof.
func (s *Store) PreviewLocalPreventionDisposition(key, headKey, bodySHA, caseEvidence, effect, conclusion string) (LocalPreventionDispositionPreview, error) {
	if err := s.lock(unix.LOCK_EX); err != nil {
		return LocalPreventionDispositionPreview{}, err
	}
	defer s.unlock()
	view, err := s.readLivenessFailureLocked(key)
	if err != nil {
		return LocalPreventionDispositionPreview{}, err
	}
	if view.Head.Status != IncidentActive || view.Head.Key != headKey || view.Head.BodySHA256 != bodySHA {
		return LocalPreventionDispositionPreview{}, errors.New("maat memory: reviewed occurrence changed; inspect the current exact head")
	}
	j, err := NewDefaultDecisionJournal()
	if err != nil {
		return LocalPreventionDispositionPreview{}, err
	}
	row, found, err := j.FindByEvidence("failure memory preflight", caseEvidence)
	if err != nil || !found || row.FailureMemoryReceipt == nil || row.FailureMemoryReceipt.StoreBinding != s.receiptBinding() {
		return LocalPreventionDispositionPreview{}, errors.New("maat memory: captured local Casebook receipt is missing or foreign")
	}
	reference := row.FailureMemoryReceipt
	r, err := s.readPreflightReceiptObjectLocked(reference.ReceiptSHA256)
	if err != nil {
		return LocalPreventionDispositionPreview{}, err
	}
	expectedEvidence, err := r.EvidenceReference()
	if err != nil || caseEvidence != expectedEvidence || r.Action != localWatchScope() {
		return LocalPreventionDispositionPreview{}, errors.New("maat memory: Casebook action identity differs from the retained receipt")
	}
	contains := false
	for _, incidentKey := range r.IncidentKeys {
		if incidentKey == key {
			contains = true
		}
	}
	current, currentErr := s.preflightBoundLocked(r.Action, r.ActionManifestSHA256, resolveMemoryGuard)
	if !contains || !validDigest(current.RegistrySnapshotSHA256) || current.RegistrySnapshotSHA256 != r.RegistrySnapshotSHA256 {
		return LocalPreventionDispositionPreview{}, errors.New("maat memory: captured prevention receipt is stale; retain a fresh exact preflight")
	}
	_ = currentErr // An exact known action debt may still be reviewed, never waived.
	p := LocalPreventionDispositionPlan{Schema: localDispositionSchema, StoreUUID: s.storeUUID, IncidentKey: key, OccurrenceKey: headKey, OccurrenceBodySHA256: bodySHA, Scope: localWatchScope(), FailureEvidenceSHA256: view.Head.EvidenceSHA256, PreflightReceiptSHA256: reference.ReceiptSHA256, Effect: effect, Conclusion: strings.TrimSpace(conclusion), CurrentFactReference: view.CurrentFactReference, ReviewedAtUTC: time.Now().UTC()}
	review, err := localDispositionReview(p, caseEvidence)
	if err != nil {
		return LocalPreventionDispositionPreview{}, err
	}
	p.OwnerReviewEvidence = review.Evidence
	if err := p.validate(); err != nil {
		return LocalPreventionDispositionPreview{}, err
	}
	data, err := json.Marshal(p)
	if err != nil {
		return LocalPreventionDispositionPreview{}, err
	}
	digest, err := s.putLocalEvidenceLocked(data)
	if err != nil {
		return LocalPreventionDispositionPreview{}, err
	}
	constraints := s.localDispositionConstraintsLocked("")
	canCommit := len(constraints) == 0 || effect == "accept-only"
	confirmation := "Accept this conclusion only. This local recurrence prevention remains active; no repair runs and no health claim changes."
	if effect == "retire-local-prevention" {
		confirmation = localLiftConfirmation
	}
	return LocalPreventionDispositionPreview{Schema: "sirsi.maat.local-prevention-disposition-preview.v1", Plan: p, PlanSHA256: digest, CanCommit: canCommit, Constraints: constraints, Confirmation: confirmation}, nil
}

func (s *Store) localDispositionConstraintsLocked(excludePlan string) []string {
	constraints := []string{}
	if attempt, err := s.pendingLivenessWatchAttemptRowsLocked(); err == nil {
		constraints = append(constraints, "unresolved local attempt/intake/projection: "+attempt.ID)
	} else if !errors.Is(err, ErrNoIncompleteLivenessWatchAttempt) {
		constraints = append(constraints, "unverifiable local action history: "+err.Error())
	}
	if pending, err := s.pendingLocalDispositionsLocked(excludePlan); err != nil {
		constraints = append(constraints, "unverifiable disposition history: "+err.Error())
	} else {
		for _, id := range pending {
			constraints = append(constraints, "pending exact data-only disposition reconciliation: "+id)
		}
	}
	return constraints
}

func (s *Store) localDispositionRecords(intent localDispositionIntent) (localDispositionProof, *LocalPreventionEvent, Decision, LocalPreventionDispositionReceipt, error) {
	p := intent.Plan
	originating, err := s.readPreflightReceiptObjectLocked(p.PreflightReceiptSHA256)
	if err != nil {
		return localDispositionProof{}, nil, Decision{}, LocalPreventionDispositionReceipt{}, err
	}
	origin, err := originating.EvidenceReference()
	if err != nil || originating.Action != p.Scope {
		return localDispositionProof{}, nil, Decision{}, LocalPreventionDispositionReceipt{}, errors.New("maat memory: originating preflight identity mismatch")
	}
	j, err := NewDefaultDecisionJournal()
	if err != nil {
		return localDispositionProof{}, nil, Decision{}, LocalPreventionDispositionReceipt{}, err
	}
	row, found, err := j.FindByEvidence("failure memory preflight", origin)
	if err != nil || !found || row.FailureMemoryReceipt == nil || row.FailureMemoryReceipt.StoreBinding != s.receiptBinding() {
		return localDispositionProof{}, nil, Decision{}, LocalPreventionDispositionReceipt{}, errors.New("maat memory: originating Casebook custody is unavailable")
	}
	review, err := localDispositionReview(p, origin)
	if err != nil || review.Evidence != p.OwnerReviewEvidence {
		return localDispositionProof{}, nil, Decision{}, LocalPreventionDispositionReceipt{}, errors.New("maat memory: exact review reference does not bind the plan")
	}
	review.Time, review.Host = intent.ConfirmedAtUTC.Format(time.RFC3339Nano), intent.Host
	accept := diagnosticOwnerAcceptance("sirsi maat local prevention", DiagnosticOwnerAcceptance{Evidence: review.Evidence, Conclusion: p.Conclusion}, review)
	accept.Time, accept.Host = review.Time, review.Host
	intentDigest, err := digestJSON(intent)
	if err != nil {
		return localDispositionProof{}, nil, Decision{}, LocalPreventionDispositionReceipt{}, err
	}
	proof := localDispositionProof{Schema: "sirsi.maat.local-prevention-disposition-proof.v1", PlanSHA256: intent.PlanSHA256, IntentSHA256: intentDigest, OwnerReview: review, OwnerAcceptance: accept}
	proofBytes, _ := json.Marshal(proof)
	proofDigest, err := digestCompactBytes(proofBytes)
	if err != nil {
		return proof, nil, Decision{}, LocalPreventionDispositionReceipt{}, err
	}
	var event *LocalPreventionEvent
	if p.Effect == "retire-local-prevention" {
		e := LocalPreventionEvent{Schema: localPreventionEventSchema, IncidentKey: p.IncidentKey, Scope: p.Scope, PredecessorEventKey: p.OccurrenceKey, PredecessorBodySHA256: p.OccurrenceBodySHA256, Kind: "owner-disposition", Status: IncidentRetired, Reason: "owner-accepted-not-repaired", EvidenceSHA256: proofDigest, OwnerReviewEvidence: p.OwnerReviewEvidence, RecordedAtUTC: intent.ConfirmedAtUTC}
		e.EventKey, err = digestJSON(e.identity())
		if err != nil {
			return proof, nil, Decision{}, LocalPreventionDispositionReceipt{}, err
		}
		event = &e
	}
	why := "Accepted — not repaired. The local prevention remains active; historical unhealthy and unknown current health remain unchanged."
	if event != nil {
		why = "Accepted — not repaired. This local prevention was lifted; historical unhealthy and unknown current health remain unchanged. A new action still needs fresh review and separate consent."
	}
	projection := Decision{Time: review.Time, Host: review.Host, Kind: "local prevention owner disposition", Requester: "sirsi maat local prevention", Resource: "pantheon", Affected: "local-macos", Assessed: "install-managed-liveness-watch", Determination: "accepted_not_repaired", Why: why, Evidence: "maat-local-prevention-disposition:" + intent.PlanSHA256, OriginEvidence: origin, ResolutionFor: accept.Evidence}
	receipt := LocalPreventionDispositionReceipt{Schema: "sirsi.maat.local-prevention-disposition-receipt.v1", PlanSHA256: intent.PlanSHA256, IncidentKey: p.IncidentKey, OccurrenceKey: p.OccurrenceKey, OccurrenceBodySHA256: p.OccurrenceBodySHA256, Effect: p.Effect, Conclusion: p.Conclusion, OwnerReviewEvidence: review.Evidence, OwnerAcceptanceEvidence: accept.Evidence, ProjectionEvidence: projection.Evidence, RecordedAtUTC: intent.ConfirmedAtUTC, Health: "historical-known-unhealthy; current-health-unobserved"}
	if event != nil {
		receipt.EventKey = event.EventKey
	}
	return proof, event, projection, receipt, nil
}

// The second string is the exact independently captured confirmation binding,
// not a general --approved flag. Data-only reconciliation below can consume
// only a previously durable consent intent; it cannot mint one.
func (s *Store) CommitLocalPreventionDisposition(expectedPlanSHA, confirmedPlanSHA string) (LocalPreventionDispositionReceipt, error) {
	if !validDigest(expectedPlanSHA) || confirmedPlanSHA != expectedPlanSHA {
		return LocalPreventionDispositionReceipt{}, errors.New("maat memory: explicit confirmation must bind the exact reviewed plan")
	}
	if err := s.lock(unix.LOCK_EX); err != nil {
		return LocalPreventionDispositionReceipt{}, err
	}
	defer s.unlock()
	if _, err := s.verifyWriteJournal(); err != nil {
		return LocalPreventionDispositionReceipt{}, err
	}
	if existing, err := s.readDispositionIntentLocked(expectedPlanSHA); err == nil {
		return s.resumeLocalDispositionLocked(existing)
	} else if !errors.Is(err, unix.ENOENT) {
		return LocalPreventionDispositionReceipt{}, err
	}
	p, err := s.readDispositionPlanLocked(expectedPlanSHA)
	if err != nil {
		return LocalPreventionDispositionReceipt{}, err
	}
	if time.Since(p.ReviewedAtUTC) > 2*time.Minute || time.Until(p.ReviewedAtUTC) > 10*time.Second {
		return LocalPreventionDispositionReceipt{}, errors.New("maat memory: stale disposition preview requires a new review")
	}
	view, err := s.readLivenessFailureLocked(p.IncidentKey)
	if err != nil {
		return LocalPreventionDispositionReceipt{}, err
	}
	if view.Head.Status != IncidentActive || view.Head.Key != p.OccurrenceKey || view.Head.BodySHA256 != p.OccurrenceBodySHA256 || view.Head.EvidenceSHA256 != p.FailureEvidenceSHA256 {
		return LocalPreventionDispositionReceipt{}, errors.New("maat memory: exact active head changed before consent")
	}
	if p.Effect == "retire-local-prevention" {
		if debt := s.localDispositionConstraintsLocked(""); len(debt) > 0 {
			return LocalPreventionDispositionReceipt{}, fmt.Errorf("maat memory: prevention lift refused; inspect/re-observe or reconcile: %s", strings.Join(debt, "; "))
		}
	}
	host, err := os.Hostname()
	if err != nil || strings.TrimSpace(host) == "" {
		return LocalPreventionDispositionReceipt{}, errors.New("maat memory: local journal host unavailable")
	}
	confirmedAt := time.Now().UTC()
	if confirmedAt.Before(p.ReviewedAtUTC) || confirmedAt.Before(view.Head.RecordedAtUTC) || confirmedAt.Sub(p.ReviewedAtUTC) > 2*time.Minute {
		return LocalPreventionDispositionReceipt{}, errors.New("maat memory: local clock precedes the reviewed occurrence; fresh review is required")
	}
	intent := localDispositionIntent{Schema: "sirsi.maat.local-prevention-disposition-intent.v1", PlanSHA256: expectedPlanSHA, Plan: p, Host: host, ConfirmedAtUTC: confirmedAt}
	if err := s.writeLocalAttemptLocked(expectedPlanSHA+".disposition-v1.json", intent); err != nil {
		return LocalPreventionDispositionReceipt{}, err
	}
	return s.resumeLocalDispositionLocked(intent)
}

func (s *Store) ReconcileLocalPreventionDisposition(planSHA string) (LocalPreventionDispositionReceipt, error) {
	if !validDigest(planSHA) {
		return LocalPreventionDispositionReceipt{}, errors.New("maat memory: exact existing consent plan required")
	}
	if err := s.lock(unix.LOCK_EX); err != nil {
		return LocalPreventionDispositionReceipt{}, err
	}
	defer s.unlock()
	intent, err := s.resumeDispositionIntentLocked(planSHA)
	if err != nil {
		return LocalPreventionDispositionReceipt{}, err
	}
	return s.resumeLocalDispositionLocked(intent)
}

func (s *Store) readDispositionPlanLocked(digest string) (LocalPreventionDispositionPlan, error) {
	var p LocalPreventionDispositionPlan
	if !validDigest(digest) {
		return p, errors.New("maat memory: invalid disposition plan digest")
	}
	data, err := readExact(s.evidenceFD, digest)
	if err != nil {
		return p, err
	}
	actual, err := digestCompactBytes(data)
	if err != nil || actual != digest {
		return p, errors.New("maat memory: disposition plan digest mismatch")
	}
	if err := s.verifyCommittedBytes("evidence", digest, data); err != nil {
		return p, err
	}
	if err := decodeLocalRecord(data, &p); err != nil || p.validate() != nil || p.StoreUUID != s.storeUUID {
		return p, errors.New("maat memory: disposition plan is malformed or foreign")
	}
	return p, nil
}

func (s *Store) readDispositionIntentLocked(digest string) (localDispositionIntent, error) {
	var i localDispositionIntent
	if !validDigest(digest) {
		return i, errors.New("maat memory: invalid disposition intent id")
	}
	if err := s.readLocalAttemptLocked(digest+".disposition-v1.json", &i); err != nil {
		return i, err
	}
	p, err := s.readDispositionPlanLocked(digest)
	if err != nil || i.Schema != "sirsi.maat.local-prevention-disposition-intent.v1" || i.PlanSHA256 != digest || digestEqual(i.Plan, p) != nil || i.ConfirmedAtUTC.Before(p.ReviewedAtUTC) || i.ConfirmedAtUTC.Sub(p.ReviewedAtUTC) > 2*time.Minute || i.Host == "" || len(i.Host) > 256 {
		return i, errors.New("maat memory: local consent intent does not match its exact retained plan")
	}
	return i, nil
}

// Committed phases are immutable readback, not re-appended with a fresh time.
// An interrupted next phase may resume only its own exact WAL intent bytes.
func (s *Store) localPhaseLocked(namespace string, fd int, leaf string, data []byte) error {
	if actual, err := readExact(fd, leaf); err == nil && bytes.Equal(actual, data) && s.verifyCommittedBytes(namespace, leaf, data) == nil {
		return nil
	}
	return s.createOrVerify(namespace, fd, leaf, data)
}

func (s *Store) resumeLocalDispositionLocked(intent localDispositionIntent) (LocalPreventionDispositionReceipt, error) {
	p := intent.Plan
	proof, event, projection, receipt, err := s.localDispositionRecords(intent)
	if err != nil {
		return receipt, err
	}
	proofBytes, err := json.Marshal(proof)
	if err != nil {
		return receipt, err
	}
	proofDigest, err := digestCompactBytes(proofBytes)
	if err != nil {
		return receipt, err
	}
	if err := s.localPhaseLocked("evidence", s.evidenceFD, proofDigest, proofBytes); err != nil {
		return receipt, err
	}
	// Only the next exact historical event or completion receipt may be the
	// unresolved write. No unrelated prepared write is ignored for a conclusion.
	receiptBytes, err := json.Marshal(receipt)
	if err != nil {
		return receipt, err
	}
	nextLeaf := intent.PlanSHA256 + ".disposition-complete-v1.json"
	nextNamespace := "attempts"
	nextBytes := receiptBytes
	var eventBytes []byte
	if event != nil {
		eventBytes, err = json.Marshal(event)
		if err != nil {
			return receipt, err
		}
		if s.verifyCommittedBytes("transitions", event.EventKey+".json", eventBytes) != nil {
			nextLeaf, nextNamespace, nextBytes = event.EventKey+".json", "transitions", eventBytes
		}
	}
	allowed, err := newWriteIntent(nextNamespace, nextLeaf, nextBytes)
	if err != nil {
		return receipt, err
	}
	if _, err := s.verifyWriteJournalAllow(allowed.Key); err != nil {
		return receipt, err
	}
	eventCommitted := event != nil && s.verifyCommittedBytes("transitions", event.EventKey+".json", eventBytes) == nil
	if p.Effect == "retire-local-prevention" && !eventCommitted {
		if debts := s.localDispositionConstraintsLocked(intent.PlanSHA256); len(debts) > 0 {
			return receipt, fmt.Errorf("maat memory: disposition still blocked by %s", strings.Join(debts, "; "))
		}
	}
	j, err := NewDefaultDecisionJournal()
	if err != nil {
		return receipt, err
	}
	// Existing exact owner review/acceptance semantics, then strict readback.
	// Store lock precedes any journal mutation lock; no journal callback enters
	// Store. Neither conclusion alone nor Casebook acceptance mutates memory.
	if err := j.AppendIdempotent(proof.OwnerReview); err != nil {
		return receipt, err
	}
	if err := retainExactLocalOwnerAcceptance(j, proof.OwnerReview, proof.OwnerAcceptance); err != nil {
		return receipt, err
	}
	// Re-read the exact chain with only this known uncommitted event omitted.
	// Every other event remains subject to committed custody and chain checks.
	data, err := readExact(s.incidentFD, p.IncidentKey+".json")
	if err != nil {
		return receipt, err
	}
	var incident Incident
	if err := json.Unmarshal(data, &incident); err != nil {
		return receipt, err
	}
	events, err := s.localEventsForDispositionLocked(event)
	if err != nil {
		return receipt, err
	}
	head, chain, err := s.localFamilyLocked(incident, events)
	if err != nil {
		return receipt, err
	}
	if eventCommitted {
		found := false
		for _, historical := range chain {
			if digestEqual(historical, *event) == nil {
				found = true
				break
			}
		}
		if !found {
			return receipt, errors.New("maat memory: retained historical disposition is absent from the verified chain")
		}
	} else if head.Key != p.OccurrenceKey || head.BodySHA256 != p.OccurrenceBodySHA256 || head.Status != IncidentActive {
		return receipt, errors.New("maat memory: consent no longer names the exact active head")
	}
	if event != nil && !eventCommitted {
		if err := s.localPhaseLocked("transitions", s.transitionFD, event.EventKey+".json", eventBytes); err != nil {
			return receipt, err
		}
	}
	if _, err := s.verifyWriteJournal(); err != nil {
		// A completion receipt may be the sole remaining known pending write.
		completionIntent, intentErr := newWriteIntent("attempts", intent.PlanSHA256+".disposition-complete-v1.json", receiptBytes)
		if intentErr != nil {
			return receipt, intentErr
		}
		if _, checkErr := s.verifyWriteJournalAllow(completionIntent.Key); checkErr != nil {
			return receipt, checkErr
		}
	}
	if err := j.AppendIdempotent(projection); err != nil {
		return receipt, err
	}
	if err := s.localPhaseLocked("attempts", s.attemptsFD, intent.PlanSHA256+".disposition-complete-v1.json", receiptBytes); err != nil {
		return receipt, err
	}
	if _, err := s.verifyWriteJournal(); err != nil {
		return receipt, err
	}
	return receipt, nil
}

func (s *Store) localEventsForDispositionLocked(expected *LocalPreventionEvent) ([]LocalPreventionEvent, error) {
	if expected == nil {
		return s.localEventsLocked()
	}
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
		if name == expected.EventKey+".json" && s.verifyCommittedBytes("transitions", name, data) != nil {
			canonical, _ := json.Marshal(expected)
			if !bytes.Equal(data, canonical) {
				return nil, errors.New("maat memory: pending event bytes changed")
			}
			continue
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
		}
		var event LocalPreventionEvent
		if err := decodeLocalRecord(data, &event); err != nil || event.validate() != nil {
			return nil, errors.New("maat memory: invalid local event")
		}
		events = append(events, event)
	}
	return events, nil
}

func (s *Store) verifyLocalOwnerDispositionProofLocked(e LocalPreventionEvent) error {
	if err := s.verifyEvidence(e.EvidenceSHA256); err != nil {
		return err
	}
	data, err := readExact(s.evidenceFD, e.EvidenceSHA256)
	if err != nil {
		return err
	}
	var proof localDispositionProof
	if err := decodeLocalRecord(data, &proof); err != nil {
		return err
	}
	intent, err := s.readDispositionIntentLocked(proof.PlanSHA256)
	if err != nil {
		return err
	}
	expected, event, _, _, err := s.localDispositionRecords(intent)
	if err != nil || event == nil || digestEqual(expected, proof) != nil || digestEqual(*event, e) != nil {
		return errors.New("maat memory: local event does not bind its exact explicit owner consent")
	}
	j, err := NewDefaultDecisionJournal()
	if err != nil {
		return err
	}
	return strictLocalOwnerConclusionReadback(j, proof.OwnerReview, proof.OwnerAcceptance)
}

func (s *Store) pendingLocalDispositionsLocked(exclude string) ([]string, error) {
	names, err := readDirectoryNames(s.attemptsFD)
	if err != nil {
		return nil, err
	}
	pending := []string{}
	for _, name := range names {
		if strings.HasSuffix(name, ".disposition-v1.json") {
			digest := strings.TrimSuffix(name, ".disposition-v1.json")
			intent, err := s.readDispositionIntentLocked(digest)
			if err != nil {
				return nil, err
			}
			_, _, projection, expected, err := s.localDispositionRecords(intent)
			if err != nil {
				return nil, err
			}
			var actual LocalPreventionDispositionReceipt
			if err := s.readLocalAttemptLocked(digest+".disposition-complete-v1.json", &actual); err != nil {
				if errors.Is(err, unix.ENOENT) {
					if digest != exclude {
						pending = append(pending, digest)
					}
					continue
				}
				return nil, err
			}
			if digestEqual(actual, expected) != nil {
				return nil, errors.New("maat memory: disposition completion changed")
			}
			j, err := NewDefaultDecisionJournal()
			if err != nil {
				return nil, err
			}
			if ok, err := decisionEvidenceReadback(j.Path, projection); err != nil || !ok {
				return nil, errors.New("maat memory: completed disposition projection is unavailable")
			}
		}
	}
	return pending, nil
}

func localDispositionAttemptLeaf(name string) bool {
	for _, suffix := range []string{".disposition-v1.json", ".disposition-complete-v1.json"} {
		if strings.HasSuffix(name, suffix) {
			return validDigest(strings.TrimSuffix(name, suffix))
		}
	}
	return false
}

// Recovery may finish a complete retained exact consent-intent prefix. It
// cannot invent the missing confirmation time/body or promote a bare plan.
func (s *Store) resumeDispositionIntentLocked(digest string) (localDispositionIntent, error) {
	if i, err := s.readDispositionIntentLocked(digest); err == nil {
		return i, nil
	}
	var i localDispositionIntent
	if !validDigest(digest) {
		return i, errors.New("maat memory: invalid consent recovery digest")
	}
	leaf := digest + ".disposition-v1.json"
	data, err := readExact(s.attemptsFD, leaf)
	if err != nil {
		return i, err
	}
	if err := decodeLocalRecord(data, &i); err != nil {
		return i, err
	}
	p, err := s.readDispositionPlanLocked(digest)
	if err != nil || i.Schema != "sirsi.maat.local-prevention-disposition-intent.v1" || i.PlanSHA256 != digest || digestEqual(i.Plan, p) != nil || i.ConfirmedAtUTC.Before(p.ReviewedAtUTC) || i.ConfirmedAtUTC.Sub(p.ReviewedAtUTC) > 2*time.Minute || i.Host == "" || len(i.Host) > 256 {
		return i, errors.New("maat memory: retained consent prefix does not bind the exact reviewed plan")
	}
	intent, err := newWriteIntent("attempts", leaf, data)
	if err != nil {
		return i, err
	}
	expected, _ := json.Marshal(intent)
	prepared, err := readExact(s.writesFD, intent.Key+".prepared.json")
	if err != nil || !bytes.Equal(expected, prepared) {
		return i, errors.New("maat memory: consent recovery requires its exact prepared target")
	}
	if err := s.localPhaseLocked("attempts", s.attemptsFD, leaf, data); err != nil {
		return i, err
	}
	return s.readDispositionIntentLocked(digest)
}

type LocalPreventionRecoveryOffer struct {
	PlanSHA256  string                         `json:"plan_sha256"`
	IncidentKey string                         `json:"incident_key"`
	Effect      string                         `json:"effect"`
	State       string                         `json:"state"`
	Plan        LocalPreventionDispositionPlan `json:"plan"`
}
type LocalPreventionRecoveryInspection struct {
	Schema           string                         `json:"schema"`
	StoreUUID        string                         `json:"store_uuid"`
	IncidentKey      string                         `json:"incident_key"`
	RegistryState    string                         `json:"registry_state"`
	Offers           []LocalPreventionRecoveryOffer `json:"offers"`
	ActionAuthorized bool                           `json:"action_authorized"`
}

// Pure inspection advertises retained consent references without attesting
// an uncommitted event or treating a corrupt registry as clear. The separate
// reconciliation command revalidates all exact phases before any completion.
func (s *Store) InspectLocalPreventionRecovery(key string) (LocalPreventionRecoveryInspection, error) {
	if !validDigest(key) || !validUUID(s.storeUUID) {
		return LocalPreventionRecoveryInspection{}, errors.New("maat memory: captured initialized local incident key required")
	}
	if err := s.lock(unix.LOCK_SH); err != nil {
		return LocalPreventionRecoveryInspection{}, err
	}
	defer s.unlock()
	result := LocalPreventionRecoveryInspection{Schema: "sirsi.maat.local-prevention-recovery-inspection.v1", StoreUUID: s.storeUUID, IncidentKey: key, RegistryState: "unverifiable", Offers: []LocalPreventionRecoveryOffer{}}
	if _, err := s.verifyWriteJournal(); err == nil {
		result.RegistryState = "verified"
	}
	names, err := readDirectoryNames(s.attemptsFD)
	if err != nil {
		return result, err
	}
	for _, name := range names {
		if strings.HasSuffix(name, ".disposition-v1.json") {
			digest := strings.TrimSuffix(name, ".disposition-v1.json")
			if !validDigest(digest) {
				return result, errors.New("maat memory: malformed consent reference")
			}
			data, err := readExact(s.attemptsFD, name)
			if err != nil {
				return result, err
			}
			var i localDispositionIntent
			if err := decodeLocalRecord(data, &i); err != nil || i.Schema != "sirsi.maat.local-prevention-disposition-intent.v1" || i.PlanSHA256 != digest || i.Plan.validate() != nil || i.Plan.StoreUUID != s.storeUUID {
				return result, errors.New("maat memory: invalid retained consent reference")
			}
			if i.Plan.IncidentKey != key {
				continue
			}
			plan, err := s.readDispositionPlanLocked(digest)
			if err != nil || digestEqual(i.Plan, plan) != nil {
				return result, errors.New("maat memory: recovery candidate plan bytes differ")
			}
			writeIntent, err := newWriteIntent("attempts", name, data)
			if err != nil {
				return result, err
			}
			prepared, err := readExact(s.writesFD, writeIntent.Key+".prepared.json")
			expectedIntent, _ := json.Marshal(writeIntent)
			if err != nil || !bytes.Equal(prepared, expectedIntent) {
				return result, errors.New("maat memory: recovery candidate has no exact prepared consent tuple")
			}
			if complete, err := readExact(s.attemptsFD, digest+".disposition-complete-v1.json"); err == nil && s.verifyCommittedBytes("attempts", digest+".disposition-complete-v1.json", complete) == nil {
				_, _, projection, expected, err := s.localDispositionRecords(i)
				if err != nil {
					return result, err
				}
				var actual LocalPreventionDispositionReceipt
				if err := decodeLocalRecord(complete, &actual); err != nil || digestEqual(actual, expected) != nil {
					return result, errors.New("maat memory: corrupt disposition completion")
				}
				j, err := NewDefaultDecisionJournal()
				if err != nil {
					return result, err
				}
				if ok, err := decisionEvidenceReadback(j.Path, projection); err != nil || !ok {
					return result, errors.New("maat memory: historical disposition projection cannot be verified")
				}
				continue
			}
			// A candidate is not an attestation of committed consent: the engine
			// completes only its exact verified prepared bytes when explicitly asked.
			result.Offers = append(result.Offers, LocalPreventionRecoveryOffer{PlanSHA256: digest, IncidentKey: key, Effect: i.Plan.Effect, Plan: i.Plan, State: "retained-prefix-requires-exact-engine-revalidation"})
		}
	}
	return result, nil
}

func (s *Store) localDispositionHistoryLocked(key string) ([]LocalPreventionDispositionReceipt, error) {
	names, err := readDirectoryNames(s.attemptsFD)
	if err != nil {
		return nil, err
	}
	history := []LocalPreventionDispositionReceipt{}
	j, err := NewDefaultDecisionJournal()
	if err != nil {
		return nil, err
	}
	for _, name := range names {
		if strings.HasSuffix(name, ".disposition-complete-v1.json") {
			digest := strings.TrimSuffix(name, ".disposition-complete-v1.json")
			intent, err := s.readDispositionIntentLocked(digest)
			if err != nil {
				return nil, err
			}
			if intent.Plan.IncidentKey != key {
				continue
			}
			proof, _, projection, expected, err := s.localDispositionRecords(intent)
			if err != nil {
				return nil, err
			}
			var actual LocalPreventionDispositionReceipt
			if err := s.readLocalAttemptLocked(name, &actual); err != nil || digestEqual(actual, expected) != nil {
				return nil, errors.New("maat memory: completed local owner history changed")
			}
			if err := strictLocalOwnerConclusionReadback(j, proof.OwnerReview, proof.OwnerAcceptance); err != nil {
				return nil, err
			}
			if ok, err := decisionEvidenceReadback(j.Path, projection); err != nil || !ok {
				return nil, errors.New("maat memory: owner disposition projection history unavailable")
			}
			history = append(history, actual)
		}
	}
	return history, nil
}
