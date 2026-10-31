package maat

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
)

const lifecycleHistoryBytes = 64 << 20
const lifecycleHistoryRows = 100000

type LifecycleObservation struct {
	Schema        string             `json:"schema"`
	ReceiptID     string             `json:"receipt_id"`
	ReceiptSHA    string             `json:"receipt_sha256"`
	Repo          string             `json:"repo"`
	HeadSHA       string             `json:"head_sha"`
	ProfileID     string             `json:"profile_id"`
	CustodyIssuer string             `json:"custody_issuer"`
	Acceptance    string             `json:"acceptance"`
	Recorded      bool               `json:"recorded"`
	Duplicate     bool               `json:"duplicate"`
	State         QualificationState `json:"state"`
}

type LifecycleAdoptionObservation struct {
	Schema      string                   `json:"schema"`
	Adoption    LifecycleAdoptionSummary `json:"adoption"`
	AdoptionSHA string                   `json:"adoption_sha256"`
	Recorded    bool                     `json:"recorded"`
	Duplicate   bool                     `json:"duplicate"`
	Provenance  string                   `json:"provenance"`
}

type LifecycleAdoptionSummary struct {
	ReviewID          string                `json:"review_id"`
	ReviewSHA         string                `json:"review_sha256"`
	OriginID          string                `json:"origin_receipt_id"`
	OriginSHA         string                `json:"origin_receipt_sha256"`
	Repo              string                `json:"repo"`
	HeadSHA           string                `json:"head_sha"`
	Profile           SourceProfileIdentity `json:"profile"`
	ScreenSHA         string                `json:"screen_sha256"`
	CandidateGate     GateDecision          `json:"candidate_gate"`
	FinalGate         GateDecision          `json:"final_gate"`
	Producer          string                `json:"producer"`
	ReviewerClaim     string                `json:"reviewer_claim"`
	OriginIssuerClaim string                `json:"origin_issuer_claim"`
}

func lifecycleAdoptionSummary(a LifecycleOwnerAdoption) LifecycleAdoptionSummary {
	return LifecycleAdoptionSummary{ReviewID: a.ReviewID, ReviewSHA: a.ReviewSHA, OriginID: a.OriginID,
		OriginSHA: a.OriginSHA, Repo: a.Repo, HeadSHA: a.HeadSHA, Profile: a.Profile, ScreenSHA: a.ScreenSHA,
		CandidateGate: a.CandidateGate, FinalGate: a.FinalGate, Producer: a.Producer, ReviewerClaim: a.ReviewerClaim, OriginIssuerClaim: a.OriginIssuerClaim}
}

func (j *FileDecisionJournal) lifecycleHistory() ([]QualificationEvent, []LifecycleOwnerAdoption, error) {
	raw, err := readLifecycleJournal(j.Path)
	if err != nil {
		return nil, nil, err
	}
	if len(raw) > 0 && raw[len(raw)-1] != '\n' {
		return nil, nil, fmt.Errorf("Ma'at lifecycle history has an unterminated final journal record; no observation was appended. Preserve the existing bytes and repair the record boundary before retrying")
	}
	scanner := bufio.NewScanner(bytes.NewReader(raw))
	scanner.Buffer(make([]byte, 4096), 256*1024)
	var events []QualificationEvent
	var adoptions []LifecycleOwnerAdoption
	rows := 0
	for scanner.Scan() {
		rows++
		if rows > lifecycleHistoryRows {
			return nil, nil, fmt.Errorf("complete lifecycle history exceeds 100000 rows; not a truncated qualified view")
		}
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		var decision Decision
		if err := json.Unmarshal(line, &decision); err != nil {
			return nil, nil, fmt.Errorf("lifecycle history row %d malformed", rows)
		}
		if err := validateDecision(decision); err != nil {
			return nil, nil, fmt.Errorf("lifecycle history row %d: %w", rows, err)
		}
		if decision.SourceQualification != nil {
			var fields map[string]json.RawMessage
			if err := json.Unmarshal(line, &fields); err != nil {
				return nil, nil, err
			}
			rawEvent, ok := fields["source_qualification"]
			if !ok {
				return nil, nil, fmt.Errorf("lifecycle event key must use exact case")
			}
			d := json.NewDecoder(bytes.NewReader(rawEvent))
			if err := sourceJSONValue(d, reflect.TypeOf(QualificationEvent{}), 0); err != nil {
				return nil, nil, err
			}
			if _, err := d.Token(); err != io.EOF {
				return nil, nil, fmt.Errorf("invalid lifecycle event EOF")
			}
			events = append(events, *decision.SourceQualification)
		}
		if decision.LifecycleAdoption != nil {
			var fields map[string]json.RawMessage
			if err := json.Unmarshal(line, &fields); err != nil {
				return nil, nil, err
			}
			rawAdoption, ok := fields["lifecycle_owner_adoption"]
			if !ok {
				return nil, nil, fmt.Errorf("owner-adoption event key must use exact case")
			}
			d := json.NewDecoder(bytes.NewReader(rawAdoption))
			if err := sourceJSONValue(d, reflect.TypeOf(LifecycleOwnerAdoption{}), 0); err != nil {
				return nil, nil, err
			}
			if _, err := d.Token(); err != io.EOF {
				return nil, nil, fmt.Errorf("invalid owner-adoption event EOF")
			}
			for _, previous := range adoptions {
				if previous.ReviewID == decision.LifecycleAdoption.ReviewID && previous != *decision.LifecycleAdoption {
					return nil, nil, fmt.Errorf("lifecycle review has conflicting retained owner adoptions")
				}
			}
			adoptions = append(adoptions, *decision.LifecycleAdoption)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, nil, fmt.Errorf("complete lifecycle history unavailable: %w", err)
	}
	return events, adoptions, nil
}

// Preview does not create a journal or append. Confirm repeats complete history,
// receipt resolution, replay, duplicate/conflict checks and append under ONE
// existing mutation lock. It never calls Append recursively under that lock.
func (j *FileDecisionJournal) ObserveLifecycle(ctx context.Context, id, repo, head, expectedSHA string, confirm bool, resolver LifecycleResolver) (LifecycleObservation, error) {
	var result LifecycleObservation
	if j == nil || !filepath.IsAbs(j.Path) || repo == "" || !lifecycleHead(head) || id == "" || len(id) > 512 || resolver == nil {
		return result, fmt.Errorf("lifecycle journal/receipt/independent expected repository and head required")
	}
	if confirm && !sourceDigest(expectedSHA) {
		return result, fmt.Errorf("confirm requires exact preview receipt SHA256")
	}
	operation := func() error {
		events, adoptions, err := j.lifecycleHistory()
		if err != nil {
			return err
		}
		effectiveResolver := withLifecycleAdoptions(resolver, adoptions)
		v, err := effectiveResolver.ResolveLifecycle(ctx, id)
		if err != nil {
			return err
		}
		if v.ID != id || !sourceDigest(v.SHA) || v.Receipt.HeadSHA != head || v.Receipt.Repo != repo {
			return fmt.Errorf("lifecycle receipt/repository/head mismatch")
		}
		if err := v.Receipt.validate(); err != nil {
			return err
		}
		if expectedSHA != "" && expectedSHA != v.SHA {
			return fmt.Errorf("lifecycle receipt bytes changed; preview again")
		}
		event := QualificationEvent{Schema: LifecycleSchema, ReceiptID: id, ReceiptSHA: v.SHA, Profile: v.Receipt.Profile, HeadSHA: head}
		duplicate := false
		for _, old := range events {
			if old.ReceiptID == id {
				if old != event {
					return fmt.Errorf("lifecycle receipt ID conflicts with retained event")
				}
				duplicate = true
			}
		}
		if !duplicate {
			events = append(events, event)
		}
		state, err := ReplayQualification(ctx, event.Profile, events, effectiveResolver)
		if err != nil {
			return err
		}
		acceptance := "unknown: canonical custody is not authenticated independent acceptance"
		if v.accepted {
			acceptance = "Receipt context established by verifier; profile state remains informational, not gate authorization."
		}
		result = LifecycleObservation{Schema: LifecycleSchema, ReceiptID: id, ReceiptSHA: v.SHA, Repo: v.Receipt.Repo, HeadSHA: head, ProfileID: event.Profile.ID(), CustodyIssuer: v.Issuer, Acceptance: acceptance, Duplicate: duplicate, State: state}
		if !confirm || duplicate {
			return nil
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		decision := Decision{Kind: "source qualification observation", Requester: "sirsi maat lifecycle", Resource: v.Receipt.Repo, Affected: head, Assessed: "source lifecycle receipt custody and replay", Determination: state.Status, Why: state.Reason, Evidence: "maat-source-lifecycle:sha256=" + lifecycleDigest(event), SourceQualification: &event}
		if err := normalizeDecision(&decision); err != nil {
			return err
		}
		if err := j.appendLocked(decision); err != nil {
			return err
		}
		result.Recorded = true
		return nil
	}
	if !confirm {
		err := operation()
		return result, err
	}
	if err := os.MkdirAll(filepath.Dir(j.Path), 0o700); err != nil {
		return result, err
	}
	err := withJournalMutationLock(j.Path, operation)
	return result, err
}

type LifecycleRawResolver interface {
	ResolveLifecycleRaw(context.Context, string) (LifecycleRawRecord, error)
}

func exactRawLifecycle(ctx context.Context, resolver LifecycleResolver, id string) (LifecycleEvidence, LifecycleRawRecord, error) {
	rawResolver, ok := resolver.(LifecycleRawResolver)
	if !ok {
		return LifecycleEvidence{}, LifecycleRawRecord{}, fmt.Errorf("owner adoption requires exact canonical raw custody bytes")
	}
	record, err := rawResolver.ResolveLifecycleRaw(ctx, id)
	if err != nil {
		return LifecycleEvidence{}, LifecycleRawRecord{}, err
	}
	fromRaw, err := lifecycleEvidenceFromRaw(record)
	if err != nil {
		return LifecycleEvidence{}, LifecycleRawRecord{}, err
	}
	resolved, err := resolver.ResolveLifecycle(ctx, id)
	if err != nil {
		return LifecycleEvidence{}, LifecycleRawRecord{}, err
	}
	if resolved.ID != fromRaw.ID || resolved.SHA != fromRaw.SHA || resolved.Issuer != fromRaw.Issuer || !reflect.DeepEqual(resolved.Receipt, fromRaw.Receipt) {
		return LifecycleEvidence{}, LifecycleRawRecord{}, fmt.Errorf("canonical parsed receipt does not match its exact raw Instructions bytes")
	}
	return resolved, record, nil
}

func buildLifecycleOwnerAdoption(ctx context.Context, resolver LifecycleResolver, reviewID, originID, repo, head, expectedReviewSHA, expectedOriginSHA string) (LifecycleOwnerAdoption, error) {
	var adoption LifecycleOwnerAdoption
	review, reviewRecord, err := exactRawLifecycle(ctx, resolver, reviewID)
	if err != nil {
		return adoption, err
	}
	origin, originRecord, err := exactRawLifecycle(ctx, resolver, originID)
	if err != nil {
		return adoption, err
	}
	if expectedReviewSHA != "" && review.SHA != expectedReviewSHA {
		return adoption, fmt.Errorf("independent review bytes changed; preview again")
	}
	if expectedOriginSHA != "" && origin.SHA != expectedOriginSHA {
		return adoption, fmt.Errorf("originating evaluation bytes changed; preview again")
	}
	if review.Receipt.Kind != "outcome" || origin.Receipt.Kind != "evaluation" ||
		review.Receipt.Repo != repo || origin.Receipt.Repo != repo || review.Receipt.HeadSHA != head ||
		review.Receipt.Profile != origin.Receipt.Profile || review.Issuer == "" || review.Issuer == review.Receipt.Profile.Producer {
		return adoption, fmt.Errorf("review and originating evaluation must match exact repository/head/profile and a non-producer reviewer attribution")
	}
	matched := false
	selected := make(map[string]bool, len(origin.Receipt.SelectedIDs))
	for _, sampleID := range origin.Receipt.SelectedIDs {
		selected[sampleID] = true
	}
	for _, sample := range origin.Receipt.Samples {
		if selected[sample.ID] && sample.OutcomeRef == reviewID && sample.Screen.Subject.Repo == repo && sample.Screen.Subject.HeadSHA == head &&
			lifecycleDigest(sample.Screen) == review.Receipt.ScreenSHA && shadowSourceGate(sample.Screen, origin.Receipt.Policy.Threshold) == review.Receipt.CandidateGate {
			matched = true
			break
		}
	}
	if !matched {
		return adoption, fmt.Errorf("independent review is not the exact outcome reference for an originating evaluation sample")
	}
	adoption = LifecycleOwnerAdoption{Schema: "maat.source-lifecycle-owner-adoption.v1", ReviewID: review.ID,
		ReviewSHA: review.SHA, OriginID: origin.ID, OriginSHA: origin.SHA, Repo: repo, HeadSHA: head,
		Profile: review.Receipt.Profile, ScreenSHA: review.Receipt.ScreenSHA, CandidateGate: review.Receipt.CandidateGate,
		FinalGate: review.Receipt.FinalGate, Producer: review.Receipt.Profile.Producer, ReviewerClaim: review.Issuer,
		OriginIssuerClaim: origin.Issuer, ReviewInstructions: base64.StdEncoding.EncodeToString(reviewRecord.RawInstructions),
		OriginInstructions: base64.StdEncoding.EncodeToString(originRecord.RawInstructions)}
	return adoption, adoption.validate()
}

// AdoptLifecycleReview previews or records an explicit local owner adoption of
// exact retained independent-review and originating-evaluation Instructions.
// Confirm binds both receipt hashes and the complete envelope digest.
func (j *FileDecisionJournal) AdoptLifecycleReview(ctx context.Context, reviewID, originID, repo, head, expectedReviewSHA, expectedOriginSHA, expectedAdoptionSHA string, confirm bool, resolver LifecycleResolver) (LifecycleAdoptionObservation, error) {
	var result LifecycleAdoptionObservation
	if j == nil || !filepath.IsAbs(j.Path) || resolver == nil || repo == "" || !lifecycleHead(head) || reviewID == "" || originID == "" {
		return result, fmt.Errorf("exact review, originating receipt, repository and head required")
	}
	if confirm && (!sourceDigest(expectedReviewSHA) || !sourceDigest(expectedOriginSHA) || !sourceDigest(expectedAdoptionSHA)) {
		return result, fmt.Errorf("owner adoption confirm requires previewed review, origin and adoption SHA256 values")
	}
	operation := func() error {
		_, adoptions, err := j.lifecycleHistory()
		if err != nil {
			return err
		}
		adoption, err := buildLifecycleOwnerAdoption(ctx, resolver, reviewID, originID, repo, head, expectedReviewSHA, expectedOriginSHA)
		if err != nil {
			return err
		}
		digest := lifecycleDigest(adoption)
		if expectedAdoptionSHA != "" && expectedAdoptionSHA != digest {
			return fmt.Errorf("owner adoption operands changed; preview again")
		}
		duplicate := false
		for _, old := range adoptions {
			if old.ReviewID == reviewID {
				if old != adoption {
					return fmt.Errorf("independent review already has a conflicting owner adoption")
				}
				duplicate = true
			}
		}
		result = LifecycleAdoptionObservation{Schema: "maat.source-lifecycle-owner-adoption.v1", Adoption: lifecycleAdoptionSummary(adoption),
			AdoptionSHA: digest, Duplicate: duplicate,
			Provenance: "Owner adoption of exact review bytes; canonical From is displayed custody attribution, not authenticated reviewer identity."}
		if !confirm || duplicate {
			return nil
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		decision := Decision{Kind: "source qualification owner adoption", Requester: "sirsi maat lifecycle owner", Resource: repo,
			Affected: head, Assessed: "exact independent review and originating evaluation adoption", Determination: "adopted",
			Why: result.Provenance, Evidence: "maat-source-lifecycle-adoption:sha256=" + digest, LifecycleAdoption: &adoption}
		if err := normalizeDecision(&decision); err != nil {
			return err
		}
		if err := j.appendLocked(decision); err != nil {
			return err
		}
		result.Recorded = true
		return nil
	}
	if !confirm {
		err := operation()
		return result, err
	}
	if err := os.MkdirAll(filepath.Dir(j.Path), 0o700); err != nil {
		return result, err
	}
	err := withJournalMutationLock(j.Path, operation)
	return result, err
}
