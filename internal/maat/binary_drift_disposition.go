package maat

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const binaryDriftInactiveDispositionSchema = "sirsi.maat.binary-drift-inactive-disposition.v1"

// BinaryDriftInactiveDisposition is a fresh, exact-host finding observation
// retained with an explicit owner disposition. It records no repair.
type BinaryDriftInactiveDisposition struct {
	Schema             string    `json:"schema"`
	OriginEvidence     string    `json:"origin_evidence"`
	Host               string    `json:"host"`
	ObservedFromUTC    time.Time `json:"observed_from_utc"`
	ObservedThroughUTC time.Time `json:"observed_through_utc"`
	FindingCheck       string    `json:"finding_check"`
	FindingSeverity    string    `json:"finding_severity"`
	FindingResolution  string    `json:"finding_resolution"`
	ObservationSHA256  string    `json:"observation_sha256"`
}

func (d BinaryDriftInactiveDisposition) validate(decision Decision) error {
	if d.Schema != binaryDriftInactiveDispositionSchema ||
		!validSystemOneEvidenceReference(d.OriginEvidence) || decision.OriginEvidence != d.OriginEvidence ||
		strings.TrimSpace(d.Host) == "" || decision.Host != d.Host || d.ObservedFromUTC.IsZero() || d.ObservedThroughUTC.IsZero() ||
		d.ObservedThroughUTC.Before(d.ObservedFromUTC) || decision.Time != d.ObservedThroughUTC.UTC().Format(time.RFC3339Nano) ||
		d.FindingCheck != "binary-drift" || d.FindingSeverity != "OK" || d.FindingResolution != "information" ||
		!validDigest(d.ObservationSHA256) || decision.Kind != "diagnostic owner acceptance" || decision.ResolutionFor != d.OriginEvidence ||
		decision.Resource != "binary-drift" || decision.Determination != "accepted_not_repaired" ||
		decision.Assessed != "fresh exact binary-drift recheck" ||
		decision.Why != "Owner recorded the exact binary-drift finding as no longer active. No repair was performed or claimed." {
		return errors.New("invalid exact binary-drift inactive disposition")
	}
	if decision.Evidence != d.evidenceReference() {
		return errors.New("binary-drift inactive disposition evidence does not match its observation")
	}
	return nil
}

func (d BinaryDriftInactiveDisposition) evidenceReference() string {
	identity := strings.Join([]string{
		d.Schema, d.OriginEvidence, d.Host, d.FindingCheck, d.FindingSeverity,
		d.FindingResolution, d.ObservationSHA256,
	}, "\x00")
	sum := sha256.Sum256([]byte(identity))
	return "maat-binary-drift-inactive:sha256=" + hex.EncodeToString(sum[:])
}

// BuildBinaryDriftInactiveDisposition constructs the owner disposition only
// after the caller has performed the bounded fresh recheck and obtained the
// user's explicit confirmation.
func BuildBinaryDriftInactiveDisposition(requester string, observation BinaryDriftInactiveDisposition) (Decision, error) {
	requester = strings.TrimSpace(requester)
	observation.Host = strings.TrimSpace(observation.Host)
	if requester == "" || observation.ObservedFromUTC.IsZero() || observation.ObservedThroughUTC.IsZero() {
		return Decision{}, errors.New("binary-drift inactive disposition requires requester and observation time")
	}
	observation.Schema = binaryDriftInactiveDispositionSchema
	observation.FindingCheck = "binary-drift"
	observation.FindingSeverity = "OK"
	observation.FindingResolution = "information"
	decision := Decision{
		Time:                           observation.ObservedThroughUTC.UTC().Format(time.RFC3339Nano),
		Host:                           observation.Host,
		Kind:                           "diagnostic owner acceptance",
		Requester:                      requester,
		Resource:                       "binary-drift",
		Assessed:                       "fresh exact binary-drift recheck",
		Determination:                  "accepted_not_repaired",
		Why:                            "Owner recorded the exact binary-drift finding as no longer active. No repair was performed or claimed.",
		OriginEvidence:                 observation.OriginEvidence,
		ResolutionFor:                  observation.OriginEvidence,
		BinaryDriftInactiveDisposition: &observation,
	}
	decision.Evidence = observation.evidenceReference()
	if err := decision.BinaryDriftInactiveDisposition.validate(decision); err != nil {
		return Decision{}, fmt.Errorf("maat binary-drift disposition: %w", err)
	}
	return decision, nil
}

func validSystemOneEvidenceReference(value string) bool {
	const prefix = "maat-system-one:sha256="
	return strings.HasPrefix(value, prefix) && validDigest(strings.TrimPrefix(value, prefix))
}

// ValidateBinaryDriftInactiveSource confirms the unique source row's exact
// retained verdict, local host binding, floor and singleton canonical finding.
func ValidateBinaryDriftInactiveSource(source Decision, evidence, host string) error {
	if source.Kind != "system one screen" || source.SystemOne == nil || source.Evidence != evidence || !validBinaryDriftInactiveSource(source, evidence, host) {
		return errors.New("binary-drift inactive disposition requires the exact single-finding System One source on this host")
	}
	return nil
}

// ValidateBinaryDriftInactiveCase performs one strict journal pass for the
// exact source, host, and prior-action fence before a fresh observation.
func ValidateBinaryDriftInactiveCase(journal *FileDecisionJournal, evidence, host string) error {
	if journal == nil || strings.TrimSpace(evidence) == "" {
		return errors.New("binary-drift inactive disposition requires exact source evidence")
	}
	var source Decision
	sourceCount := 0
	conflict := false
	if _, err := journal.scanStrict(func(row Decision) bool {
		if row.Evidence == evidence {
			source, sourceCount = row, sourceCount+1
		}
		if row.OriginEvidence != evidence && row.ResolutionFor != evidence {
			return false
		}
		switch row.Kind {
		case "diagnostic repair attempt", "diagnostic verified repair", "diagnostic owner review":
			conflict = true
		case "diagnostic owner acceptance":
			if row.BinaryDriftInactiveDisposition == nil || row.OriginEvidence != evidence || row.ResolutionFor != evidence {
				conflict = true
			}
		}
		return false
	}); err != nil {
		return fmt.Errorf("binary-drift inactive disposition: inspect strict journal history: %w", err)
	}
	if sourceCount != 1 {
		return errors.New("binary-drift inactive disposition requires one unique System One source case")
	}
	if err := ValidateBinaryDriftInactiveSource(source, evidence, host); err != nil {
		return err
	}
	if conflict {
		return errors.New("binary-drift source already has an owner or updater decision")
	}
	return nil
}

func validBinaryDriftInactiveSource(source Decision, evidence, host string) bool {
	verdict := source.SystemOne
	if verdict == nil || ValidateMaatVerdict(*verdict) != nil || !DiagnosticHostMatches(verdict.Subject, host) || !verdict.Floor.Passed || len(verdict.Findings) != 1 || !IsCanonicalHostDiagnosticFinding(*verdict, verdict.Findings[0], "binary-drift") || (verdict.Findings[0].Severity != "major" && verdict.Findings[0].Severity != "block") {
		return false
	}
	raw, err := json.Marshal(verdict)
	if err != nil {
		return false
	}
	digest := sha256.Sum256(raw)
	return evidence == "maat-system-one:sha256="+hex.EncodeToString(digest[:])
}

// AppendBinaryDriftInactiveDispositionOnce durably records one exact owner
// disposition under the journal mutation lock. Replaying the same bound
// observation returns its original timestamp and evidence; conflicting
// dispositions or updater/owner actions for the case are refused.
func (j *FileDecisionJournal) AppendBinaryDriftInactiveDispositionOnce(decision Decision) (Decision, bool, error) {
	if j == nil || strings.TrimSpace(j.Path) == "" {
		return Decision{}, false, errors.New("maat decision journal: empty path")
	}
	if decision.BinaryDriftInactiveDisposition == nil {
		return Decision{}, false, errors.New("maat decision journal: missing typed binary-drift disposition")
	}
	if err := normalizeDecision(&decision); err != nil {
		return Decision{}, false, err
	}
	if err := decision.BinaryDriftInactiveDisposition.validate(decision); err != nil {
		return Decision{}, false, err
	}
	if err := ensureJournalParent(j.Path); err != nil {
		return Decision{}, false, err
	}
	var result Decision
	appended := false
	err := withJournalMutationLock(j.Path, func() error {
		var sameEvidence *Decision
		conflictingRelated := false
		hasSameCaseDisposition := false
		sameCaseDispositionCount := 0
		var origin Decision
		originCount := 0
		if _, err := j.scanStrict(func(existing Decision) bool {
			if existing.Evidence == decision.OriginEvidence {
				origin = existing
				originCount++
			}
			if existing.Kind == decision.Kind && existing.OriginEvidence == decision.OriginEvidence && existing.BinaryDriftInactiveDisposition != nil {
				hasSameCaseDisposition = true
				sameCaseDispositionCount++
				copy := existing
				sameEvidence = &copy
				return false
			}
			if existing.OriginEvidence == decision.OriginEvidence && (existing.Kind == "diagnostic repair attempt" || existing.Kind == "diagnostic verified repair" || existing.Kind == "diagnostic owner review" || existing.Kind == "diagnostic owner acceptance" && existing.BinaryDriftInactiveDisposition == nil) {
				conflictingRelated = true
			}
			return false
		}); err != nil {
			return err
		}
		if originCount != 1 || origin.Kind != "system one screen" || origin.SystemOne == nil || !validBinaryDriftInactiveSource(origin, decision.OriginEvidence, decision.Host) {
			return errors.New("binary-drift inactive disposition requires one unchanged exact-host source case")
		}
		if conflictingRelated {
			return errors.New("binary-drift case already has an updater or owner decision; refresh the exact case")
		}
		if hasSameCaseDisposition {
			if sameCaseDispositionCount != 1 || sameEvidence == nil || sameEvidence.Evidence != decision.Evidence || !sameInactiveObservationIdentity(*sameEvidence.BinaryDriftInactiveDisposition, *decision.BinaryDriftInactiveDisposition) {
				return errors.New("binary-drift case already has a different inactive disposition")
			}
			result = *sameEvidence
			return nil
		}
		if err := j.appendLocked(decision); err != nil {
			return err
		}
		appended = true
		found, err := decisionEvidenceReadback(j.Path, decision)
		if err != nil {
			return err
		}
		if !found {
			return errors.New("maat decision journal: binary-drift disposition readback is missing")
		}
		result = decision
		return nil
	})
	if err != nil {
		return Decision{}, false, fmt.Errorf("maat decision journal: append inactive disposition: %w", err)
	}
	return result, appended, nil
}

func sameInactiveObservationIdentity(left, right BinaryDriftInactiveDisposition) bool {
	left.ObservedFromUTC, right.ObservedFromUTC = time.Time{}, time.Time{}
	left.ObservedThroughUTC, right.ObservedThroughUTC = time.Time{}, time.Time{}
	return left == right
}

func ensureJournalParent(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("maat decision journal: create parent: %w", err)
	}
	return nil
}
