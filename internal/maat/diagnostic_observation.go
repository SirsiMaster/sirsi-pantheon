package maat

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

const DiagnosticObservationSchema = "sirsi.maat.local-diagnostic-observation.v1"

// No producer prose, paths, process arguments, prompts or credentials enter
// this projection. Unknown check names become a digest, never retained text.
type DiagnosticStatus struct {
	Check    string `json:"check"`
	Severity int    `json:"severity"`
	RepairID string `json:"repair_id,omitempty"`
}
type DiagnosticObservation struct {
	Schema        string             `json:"schema"`
	SourceSHA256  string             `json:"source_sha256"`
	HostSHA256    string             `json:"host_sha256"`
	ObservedAtUTC time.Time          `json:"observed_at_utc"`
	Score         int                `json:"score"`
	Statuses      []DiagnosticStatus `json:"statuses"`
}

var diagnosticChecks = map[string]bool{
	"RAM Pressure": true, "Swap Usage": true, "Disk Space": true, "Top Memory Consumers": true,
	"Spotlight Storm": true, "App Hangs (7d)": true, "App Crashes (7d)": true, "Sirsi Processes": true,
	"launchd Disabled Override": true, "Agent Pause Ledger": true, "liveness-watch": true, "binary-drift": true,
}

func DiagnosticCheckCode(check string) string {
	if diagnosticChecks[check] {
		return check
	}
	sum := sha256.Sum256([]byte(check))
	return "unclassified-" + hex.EncodeToString(sum[:])
}

func (o DiagnosticObservation) Validate() error {
	if o.Schema != DiagnosticObservationSchema || !validDigest(o.SourceSHA256) || !validDigest(o.HostSHA256) || o.Score < 0 || o.Score > 100 || len(o.Statuses) > 128 {
		return errors.New("maat diagnostic: invalid bounded observation provenance")
	}
	seen := map[string]bool{}
	for _, s := range o.Statuses {
		unknown := strings.HasPrefix(s.Check, "unclassified-") && validDigest(strings.TrimPrefix(s.Check, "unclassified-"))
		if (!diagnosticChecks[s.Check] && !unknown) || s.Severity < 0 || s.Severity > 3 || seen[s.Check] {
			return errors.New("maat diagnostic: closed unique status codes required")
		}
		seen[s.Check] = true
		if s.RepairID != "" && (s.Severity < 2 || !((s.Check == "launchd Disabled Override" && s.RepairID == SystemOneRepairLaunchdDisabled) || (s.Check == "liveness-watch" && s.RepairID == SystemOneRepairLivenessWatch) || (s.Check == "binary-drift" && s.RepairID == SystemOneRepairBinaryDrift))) {
			return errors.New("maat diagnostic: repair identity is not a closed local route")
		}
	}
	return nil
}

func (o DiagnosticObservation) ValidateFresh() error {
	if err := o.Validate(); err != nil {
		return err
	}
	age := time.Since(o.ObservedAtUTC)
	if len(o.Statuses) == 0 || o.ObservedAtUTC.IsZero() || age > 2*time.Minute || age < -10*time.Second {
		return errors.New("maat diagnostic: observation freshness is unknown or expired")
	}
	return nil
}

// Verdict is a deterministic data projection. Freshness is checked separately
// at the record boundary; historical reconstruction never renews observation.
func (o DiagnosticObservation) Verdict() (MaatVerdict, error) {
	if err := o.Validate(); err != nil {
		return MaatVerdict{}, err
	}
	findings := []ScreenFinding{}
	for _, s := range o.Statuses {
		if s.Severity < 2 {
			continue
		}
		severity, word := "major", "warning"
		if s.Severity == 3 {
			severity, word = "block", "critical"
		}
		label := s.Check
		if strings.HasPrefix(label, "unclassified-") {
			label = "unclassified local check"
		}
		findings = append(findings, ScreenFinding{ID: CanonicalHostFindingID(s.Check), Severity: severity, Category: "host-health",
			Claim: fmt.Sprintf("%s reports a %s status.", label, word), Evidence: CanonicalHostFindingEvidence(o.SourceSHA256, s.Check), Confidence: 1,
			FixHint: "Level 1: re-observe this local check. Level 2: review the typed Ma'at action or record an owner review. Level 3: retain an evidence-bound owner conclusion; recovery text is data, not a command.", RepairID: s.RepairID})
	}
	return Screen(SystemOneScreen{Subject: VerdictSubject{Kind: "host", Repo: "local-host-" + o.HostSHA256, Ref: "diagnostic", HeadSHA: o.SourceSHA256},
		FeatherWeight: o.Score, Confidence: 1, Findings: findings,
		Floor: FloorResult{Passed: !o.ObservedAtUTC.IsZero(), Checks: []FloorCheck{{Name: "diagnostic provenance", Passed: !o.ObservedAtUTC.IsZero(), Detail: "Selected status codes and source digest only; producer prose was excluded."}}},
		Model: ModelStamp{Provider: "maat-local:deterministic", Version: "v1", Local: true}})
}

func diagnosticSHA(v any) (string, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:]), nil
}
