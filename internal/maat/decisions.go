package maat

// The decision journal is a local, append-only read model for Ma'at.  It
// deliberately records what the scheduler assessed; it does not make policy
// decisions for another component, and it does not claim that a local record
// is a cross-host attestation.

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Decision is the stable, surface-neutral explanation of one Ma'at outcome.
// It is intentionally a fact record: consumers render it, but never infer an
// authorization absent from its determination and evidence reference.
type Decision struct {
	Time          string `json:"time"`
	Host          string `json:"host"`
	Kind          string `json:"kind"`
	Requester     string `json:"requester"`
	Resource      string `json:"resource,omitempty"`
	Assessed      string `json:"assessed"`
	Affected      string `json:"affected,omitempty"`
	Determination string `json:"determination"`
	Why           string `json:"why"`
	Evidence      string `json:"evidence,omitempty"`
	// OriginEvidence binds an owner review to the existing case that requested
	// it. ResolutionFor binds an explicit owner acceptance to that review. Both
	// are factual links, not claims that a subsystem was repaired.
	OriginEvidence string `json:"origin_evidence,omitempty"`
	ResolutionFor  string `json:"resolution_for,omitempty"`
	// SystemOne is Ma'at's strict local screen. It is advisory evidence only;
	// the recorded screen does not become mutation or release authority.
	SystemOne *MaatVerdict `json:"system_one,omitempty"`
	// SystemOneCalibration binds one recorded local auto-pass to a distinct
	// independent outcome. It is audit evidence only, never a local grant.
	SystemOneCalibration *CalibrationRecord `json:"system_one_calibration,omitempty"`
}

// DecisionJournal persists the local decision projection. Implementations may
// be supplied by tests or by a different replicated transport later.
type DecisionJournal interface {
	Append(Decision) error
	Recent(limit int) ([]Decision, error)
}

// ProjectFailureMemoryPreflight records a completed failure-memory screen in
// Ma'at's existing decision journal.  This is intentionally one-way: the
// journal is an inspectable projection for Casebook and surfaces, never an
// input that can alter a failure-memory decision or authorize remediation.
func ProjectFailureMemoryPreflight(j DecisionJournal, receipt PreflightReceipt) error {
	if j == nil {
		return fmt.Errorf("maat failure memory projection: nil decision journal")
	}
	if err := receipt.validate(); err != nil {
		return fmt.Errorf("maat failure memory projection: invalid preflight receipt")
	}
	evidence, err := receipt.EvidenceReference()
	if err != nil {
		return fmt.Errorf("maat failure memory projection: bind receipt evidence: %w", err)
	}
	why := fmt.Sprintf("%d measured records; %d recovery actions", len(receipt.MeasuredChecks), len(receipt.RecoveryActions))
	if receipt.RecoveryReference != "" {
		why += "; recovery reference: " + receipt.RecoveryReference
	}
	return j.Append(Decision{
		Time:          receipt.EvaluatedAtUTC.UTC().Format(time.RFC3339Nano),
		Kind:          "failure memory preflight",
		Requester:     "maat",
		Resource:      receipt.Action.Component,
		Assessed:      receipt.Action.Operation,
		Affected:      receipt.Action.Profile,
		Determination: string(receipt.Decision),
		Why:           why,
		Evidence:      evidence,
	})
}

// RecordReport projects a completed Ma'at report into the same append-only
// decision journal used by reservations and cede outcomes. It makes local
// quality, canon, pipeline, and future Stack Lab assessments inspectable by
// Casebook without giving Casebook a second assessment or policy authority.
//
// Every assessment is recorded independently, followed by a report summary.
// A caller must treat an append error as an incomplete audit: rendering an
// unrecorded report as though it were available to local intelligence would
// create a false clean state.
func RecordReport(j DecisionJournal, requester string, report *Report) error {
	if j == nil {
		return fmt.Errorf("maat decision journal: nil")
	}
	if report == nil {
		return fmt.Errorf("maat decision journal: nil report")
	}
	requester = strings.TrimSpace(requester)
	if requester == "" {
		return fmt.Errorf("maat decision journal: requester is required")
	}
	evidence := "maat-report:" + report.AssessedAt.UTC().Format(time.RFC3339Nano)
	for _, assessment := range report.Assessments {
		why := assessment.Message
		if assessment.Remediation != "" {
			why += "; remediation: " + assessment.Remediation
		}
		if err := j.Append(Decision{
			Kind:          "assessment " + string(assessment.Domain),
			Requester:     requester,
			Resource:      string(assessment.Domain),
			Assessed:      assessment.Standard,
			Determination: assessment.Verdict.String(),
			Why:           why,
			Evidence:      evidence,
		}); err != nil {
			return fmt.Errorf("record assessment %q: %w", assessment.Subject, err)
		}
	}
	return j.Append(Decision{
		Kind:          "assessment report",
		Requester:     requester,
		Resource:      "maat",
		Assessed:      "overall quality report",
		Determination: report.OverallVerdict.String(),
		Why:           fmt.Sprintf("weight %d/100; %d passed, %d warnings, %d failures", report.OverallWeight, report.Passes, report.Warnings, report.Failures),
		Evidence:      evidence,
	})
}

// FileDecisionJournal stores newline-delimited JSON beneath the user's Sirsi
// state. It is a projection, not the reservation scheduler's authority.
type FileDecisionJournal struct {
	Path string
}

func DefaultDecisionPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("maat decision path: resolve home: %w", err)
	}
	return filepath.Join(home, ".sirsi", "maat", "decisions.jsonl"), nil
}

func NewDefaultDecisionJournal() (*FileDecisionJournal, error) {
	path, err := DefaultDecisionPath()
	if err != nil {
		return nil, err
	}
	return &FileDecisionJournal{Path: path}, nil
}

func (j *FileDecisionJournal) Append(decision Decision) error {
	if j == nil || strings.TrimSpace(j.Path) == "" {
		return fmt.Errorf("maat decision journal: empty path")
	}
	if err := normalizeDecision(&decision); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(j.Path), 0o700); err != nil {
		return fmt.Errorf("maat decision journal: create parent: %w", err)
	}
	encoded, err := json.Marshal(decision)
	if err != nil {
		return fmt.Errorf("maat decision journal: encode: %w", err)
	}
	f, err := os.OpenFile(j.Path, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0o600)
	if err != nil {
		return fmt.Errorf("maat decision journal: open: %w", err)
	}
	defer f.Close()
	if _, err := f.Write(append(encoded, '\n')); err != nil {
		return fmt.Errorf("maat decision journal: append: %w", err)
	}
	if err := f.Sync(); err != nil {
		return fmt.Errorf("maat decision journal: sync: %w", err)
	}
	return nil
}

func (j *FileDecisionJournal) Recent(limit int) ([]Decision, error) {
	if j == nil || strings.TrimSpace(j.Path) == "" {
		return nil, fmt.Errorf("maat decision journal: empty path")
	}
	if limit <= 0 {
		limit = 50
	}
	f, err := os.Open(j.Path)
	if err != nil {
		if os.IsNotExist(err) {
			return []Decision{}, nil
		}
		return nil, fmt.Errorf("maat decision journal: open: %w", err)
	}
	defer f.Close()

	var decisions []Decision
	scanner := bufio.NewScanner(f)
	// A decision is deliberately bounded; raising Scanner's default is for
	// structured evidence links, not an invitation to treat this as a trace log.
	scanner.Buffer(make([]byte, 4096), 256*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var decision Decision
		if err := json.Unmarshal([]byte(line), &decision); err != nil {
			return nil, fmt.Errorf("maat decision journal: malformed record: %w", err)
		}
		if err := validateDecision(decision); err != nil {
			return nil, err
		}
		decisions = append(decisions, decision)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("maat decision journal: read: %w", err)
	}
	sort.SliceStable(decisions, func(i, k int) bool { return decisions[i].Time > decisions[k].Time })
	if len(decisions) > limit {
		decisions = decisions[:limit]
	}
	return decisions, nil
}

func normalizeDecision(decision *Decision) error {
	if decision.Time == "" {
		decision.Time = time.Now().UTC().Format(time.RFC3339Nano)
	}
	if decision.Host == "" {
		host, err := os.Hostname()
		if err != nil {
			return fmt.Errorf("maat decision journal: resolve host: %w", err)
		}
		decision.Host = host
	}
	return validateDecision(*decision)
}

func validateDecision(decision Decision) error {
	for field, value := range map[string]string{
		"time": decision.Time, "host": decision.Host, "kind": decision.Kind,
		"requester": decision.Requester, "assessed": decision.Assessed,
		"determination": decision.Determination, "why": decision.Why,
	} {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("maat decision journal: %s is required", field)
		}
	}
	if _, err := time.Parse(time.RFC3339Nano, decision.Time); err != nil {
		return fmt.Errorf("maat decision journal: invalid time: %w", err)
	}
	if len(decision.OriginEvidence) > 512 || len(decision.ResolutionFor) > 512 {
		return fmt.Errorf("maat decision journal: resolution evidence exceeds bounded record size")
	}
	if decision.SystemOne != nil {
		if err := ValidateMaatVerdict(*decision.SystemOne); err != nil {
			return fmt.Errorf("maat decision journal: invalid System One verdict: %w", err)
		}
		if decision.Kind != "system one screen" {
			return fmt.Errorf("maat decision journal: System One verdict is valid only for system one screen")
		}
	}
	if decision.SystemOneCalibration != nil {
		if err := validateCalibrationRecord(*decision.SystemOneCalibration); err != nil {
			return fmt.Errorf("maat decision journal: invalid System One calibration: %w", err)
		}
		if decision.Kind != "system one calibration" {
			return fmt.Errorf("maat decision journal: System One calibration is valid only for system one calibration")
		}
	}
	if decision.SystemOne != nil && decision.SystemOneCalibration != nil {
		return fmt.Errorf("maat decision journal: screen and calibration records cannot coexist")
	}
	return nil
}

// ReadAll is a narrow helper for an already-open trusted stream. It exists for
// future descriptor-bound transport adapters; the file journal itself owns the
// on-disk path lifecycle.
func ReadAll(r io.Reader) ([]byte, error) { return io.ReadAll(r) }
