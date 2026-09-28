package maat

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
)

// DiagnosticOwnerReview is the evidence a native surface supplies when there
// is no safe automatic mutation. It records an actionable owner-decision path
// in the shared Ma'at journal rather than presenting an operator with a dead
// end or pretending that an informational command repaired the condition.
type DiagnosticOwnerReview struct {
	Check   string
	Message string
	Detail  string
	// OriginEvidence is required when the review originates from a Casebook
	// case, preserving the exact decision that led an operator here.
	OriginEvidence string
}

// DiagnosticOwnerAcceptance is the explicit third-level completion outcome
// for one owner-review record. It accepts a documented conclusion only; it
// never represents an observed repair or an implicit authorization.
type DiagnosticOwnerAcceptance struct {
	Evidence   string
	Conclusion string
}

// RecordDiagnosticOwnerReview appends an evidence-bound, local owner-review
// decision. The record does not authorize mutation and is intentionally
// surface-neutral so the CLI, Mac app, MCP, TUI, and dashboard can project the
// same third-level resolution outcome.
func RecordDiagnosticOwnerReview(j DecisionJournal, requester string, review DiagnosticOwnerReview) (Decision, error) {
	if j == nil {
		return Decision{}, fmt.Errorf("maat diagnostic resolution: nil journal")
	}
	requester = strings.TrimSpace(requester)
	review.Check = strings.TrimSpace(review.Check)
	review.Message = strings.TrimSpace(review.Message)
	review.Detail = strings.TrimSpace(review.Detail)
	review.OriginEvidence = strings.TrimSpace(review.OriginEvidence)
	if requester == "" || review.Check == "" || review.Message == "" {
		return Decision{}, fmt.Errorf("maat diagnostic resolution: requester, check, and message are required")
	}
	if len(review.Check) > 256 || len(review.Message) > 4096 || len(review.Detail) > 16*1024 || len(review.OriginEvidence) > 512 {
		return Decision{}, fmt.Errorf("maat diagnostic resolution: review fields exceed bounded record size")
	}

	sum := sha256.Sum256([]byte(review.Check + "\x00" + review.Message + "\x00" + review.Detail))
	why := review.Message
	if review.Detail != "" {
		why += "; observed detail: " + review.Detail
	}
	decision := Decision{
		Kind:           "diagnostic owner review",
		Requester:      requester,
		Resource:       review.Check,
		Assessed:       "diagnostic finding",
		Determination:  "owner_review_required",
		Why:            why,
		Evidence:       "diagnostic-finding:sha256=" + hex.EncodeToString(sum[:]),
		OriginEvidence: review.OriginEvidence,
	}
	if err := j.Append(decision); err != nil {
		return Decision{}, fmt.Errorf("maat diagnostic resolution: append owner review: %w", err)
	}
	return decision, nil
}

// AcceptDiagnosticOwnerReview records one explicit owner conclusion for one
// existing owner-review evidence record. Duplicate and unknown references are
// rejected so an open case cannot silently become resolved by a UI replay.
func AcceptDiagnosticOwnerReview(j DecisionJournal, requester string, acceptance DiagnosticOwnerAcceptance) (Decision, error) {
	if j == nil {
		return Decision{}, fmt.Errorf("maat diagnostic resolution: nil journal")
	}
	requester = strings.TrimSpace(requester)
	acceptance.Evidence = strings.TrimSpace(acceptance.Evidence)
	acceptance.Conclusion = strings.TrimSpace(acceptance.Conclusion)
	if requester == "" || acceptance.Evidence == "" || acceptance.Conclusion == "" {
		return Decision{}, fmt.Errorf("maat diagnostic resolution: requester, evidence, and conclusion are required")
	}
	if len(acceptance.Evidence) > 512 || len(acceptance.Conclusion) > 4096 {
		return Decision{}, fmt.Errorf("maat diagnostic resolution: acceptance fields exceed bounded record size")
	}

	rows, err := j.Recent(1000)
	if err != nil {
		return Decision{}, fmt.Errorf("maat diagnostic resolution: read owner review: %w", err)
	}
	var original *Decision
	for index := range rows {
		row := rows[index]
		if row.ResolutionFor == acceptance.Evidence {
			return Decision{}, fmt.Errorf("maat diagnostic resolution: owner review is already accepted")
		}
		if row.Kind == "diagnostic owner review" && row.Evidence == acceptance.Evidence {
			original = &row
		}
	}
	if original == nil {
		return Decision{}, fmt.Errorf("maat diagnostic resolution: no matching diagnostic owner review")
	}

	sum := sha256.Sum256([]byte(acceptance.Evidence + "\x00" + acceptance.Conclusion))
	decision := Decision{
		Kind:          "diagnostic owner acceptance",
		Requester:     requester,
		Resource:      original.Resource,
		Assessed:      "owner-reviewed diagnostic finding",
		Determination: "accepted",
		Why:           acceptance.Conclusion,
		Evidence:      "diagnostic-acceptance:sha256=" + hex.EncodeToString(sum[:]),
		ResolutionFor: acceptance.Evidence,
	}
	if err := j.Append(decision); err != nil {
		return Decision{}, fmt.Errorf("maat diagnostic resolution: append owner acceptance: %w", err)
	}
	return decision, nil
}
