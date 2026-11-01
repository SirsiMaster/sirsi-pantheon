package maat

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
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
	decision, err := buildDiagnosticOwnerReview(requester, review)
	if err != nil {
		return Decision{}, err
	}
	if err := normalizeDecision(&decision); err != nil {
		return Decision{}, fmt.Errorf("maat diagnostic resolution: normalize owner review: %w", err)
	}
	if err := j.Append(decision); err != nil {
		return Decision{}, fmt.Errorf("maat diagnostic resolution: append owner review: %w", err)
	}
	return decision, nil
}

func buildDiagnosticOwnerReview(requester string, review DiagnosticOwnerReview) (Decision, error) {
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

	if fileJournal, ok := j.(*FileDecisionJournal); ok {
		var accepted Decision
		err := withJournalMutationLock(fileJournal.Path, func() error {
			original, found, alreadyAccepted, err := strictOwnerReviewExact(fileJournal.Path, acceptance.Evidence)
			if err != nil {
				return err
			}
			if alreadyAccepted {
				return fmt.Errorf("maat diagnostic resolution: owner review is already accepted")
			}
			if !found {
				return fmt.Errorf("maat diagnostic resolution: no matching diagnostic owner review")
			}
			accepted = diagnosticOwnerAcceptance(requester, acceptance, original)
			if err := normalizeDecision(&accepted); err != nil {
				return err
			}
			return fileJournal.appendLocked(accepted)
		})
		if err != nil {
			return Decision{}, fmt.Errorf("maat diagnostic resolution: accept owner review: %w", err)
		}
		return accepted, nil
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

	decision := diagnosticOwnerAcceptance(requester, acceptance, *original)
	if err := j.Append(decision); err != nil {
		return Decision{}, fmt.Errorf("maat diagnostic resolution: append owner acceptance: %w", err)
	}
	return decision, nil
}

func diagnosticOwnerAcceptance(requester string, acceptance DiagnosticOwnerAcceptance, original Decision) Decision {
	sum := sha256.Sum256([]byte(acceptance.Evidence + "\x00" + acceptance.Conclusion))
	return Decision{
		Kind:          "diagnostic owner acceptance",
		Requester:     requester,
		Resource:      original.Resource,
		Assessed:      "owner-reviewed diagnostic finding",
		Determination: "accepted",
		Why:           acceptance.Conclusion,
		Evidence:      "diagnostic-acceptance:sha256=" + hex.EncodeToString(sum[:]),
		ResolutionFor: acceptance.Evidence,
	}
}

// strictOwnerReviewExact is a bounded streaming authority read used only by
// the confirmed owner-acceptance mutation. It does not inherit Recent's age
// window and does not consume tolerant Casebook continuation state.
func strictOwnerReviewExact(path, evidence string) (Decision, bool, bool, error) {
	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return Decision{}, false, false, nil
		}
		return Decision{}, false, false, fmt.Errorf("open strict journal: %w", err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() < 0 || info.Size() > maxAppendOnceJournalBytes {
		return Decision{}, false, false, fmt.Errorf("strict journal is not a bounded regular file")
	}
	scanner := bufio.NewScanner(io.LimitReader(file, maxAppendOnceJournalBytes+1))
	scanner.Buffer(make([]byte, 4096), maxHistoryRecordBytes)
	var original Decision
	found, accepted := false, false
	for lineNumber := 1; scanner.Scan(); lineNumber++ {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var row Decision
		if err := json.Unmarshal([]byte(line), &row); err != nil {
			return Decision{}, false, false, fmt.Errorf("strict journal has malformed record at line %d", lineNumber)
		}
		if err := validateDecision(row); err != nil {
			return Decision{}, false, false, fmt.Errorf("strict journal has invalid record at line %d", lineNumber)
		}
		if row.Kind == "diagnostic owner review" && row.Evidence == evidence {
			if found {
				return Decision{}, false, false, fmt.Errorf("strict journal has duplicate owner-review evidence")
			}
			original, found = row, true
		}
		if row.Kind == "diagnostic owner acceptance" && row.ResolutionFor == evidence {
			accepted = true
		}
	}
	if err := scanner.Err(); err != nil {
		return Decision{}, false, false, fmt.Errorf("strict journal exceeds its bound or is unreadable")
	}
	return original, found, accepted, nil
}

// Same exact-review authority as AcceptDiagnosticOwnerReview, with the already
// captured bytes/time needed for data-only recovery. Lock order is Store then
// journal; this callback never calls Store or performs a service operation.
func retainExactLocalOwnerAcceptance(j *FileDecisionJournal, review, acceptance Decision) error {
	return withJournalMutationLock(j.Path, func() error {
		original, found, accepted, err := strictOwnerReviewExact(j.Path, review.Evidence)
		if err != nil || !found || digestEqual(original, review) != nil {
			return fmt.Errorf("maat memory: exact occurrence-bound owner review is unavailable")
		}
		if accepted {
			return strictLocalOwnerConclusionReadback(j, review, acceptance)
		}
		expected := diagnosticOwnerAcceptance(acceptance.Requester, DiagnosticOwnerAcceptance{Evidence: review.Evidence, Conclusion: acceptance.Why}, original)
		expected.Time, expected.Host = acceptance.Time, acceptance.Host
		if digestEqual(expected, acceptance) != nil || validateDecision(acceptance) != nil {
			return fmt.Errorf("maat memory: retained owner acceptance is not this review's exact conclusion")
		}
		if err := j.appendLocked(acceptance); err != nil {
			return err
		}
		return strictLocalOwnerConclusionReadback(j, review, acceptance)
	})
}
func strictLocalOwnerConclusionReadback(j *FileDecisionJournal, review, acceptance Decision) error {
	for _, decision := range []Decision{review, acceptance} {
		if ok, err := decisionEvidenceReadback(j.Path, decision); err != nil || !ok {
			return fmt.Errorf("maat memory: exact local owner conclusion bytes are unavailable")
		}
	}
	count := 0
	conflict := false
	_, err := j.scanStrict(func(d Decision) bool {
		if d.Kind == "diagnostic owner acceptance" && d.ResolutionFor == review.Evidence {
			count++
			if digestEqual(d, acceptance) != nil {
				conflict = true
			}
		}
		return false
	})
	if err != nil || count != 1 || conflict {
		return fmt.Errorf("maat memory: owner review has duplicate or conflicting conclusions")
	}
	return nil
}
