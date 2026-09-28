package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/SirsiMaster/sirsi-pantheon/internal/guard"
	"github.com/SirsiMaster/sirsi-pantheon/internal/maat"
	"github.com/SirsiMaster/sirsi-pantheon/internal/output"
)

var (
	maatTriageConfirm       bool
	maatTriageReadDiagnosis = guard.Doctor
	maatTriageHostname      = os.Hostname
)

// maatTriageCmd is Ma'at's local System One observation loop. It intentionally
// has no caller-supplied evidence packet: it observes one diagnostic snapshot,
// hashes the exact normalized report, derives a deterministic screen, and only
// writes the Casebook after a separate confirmation. It does not repair,
// release, install, or authorize any other operation.
var maatTriageCmd = &cobra.Command{
	Use:   "triage",
	Short: "Observe this Mac and prepare a Ma'at System One health screen",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		started := time.Now()
		report, err := maatTriageReadDiagnosis()
		if err != nil {
			return fmt.Errorf("Ma'at System One: observe this Mac: %w", err)
		}
		host, err := maatTriageHostname()
		if err != nil {
			return fmt.Errorf("Ma'at System One: resolve local host identity: %w", err)
		}
		screen, snapshotHash, err := maatHostSystemOneScreen(report, host, time.Since(started).Milliseconds())
		if err != nil {
			return err
		}
		verdict, err := maat.Screen(screen)
		if err != nil {
			return fmt.Errorf("Ma'at System One: validate local health screen: %w", err)
		}

		if !maatTriageConfirm {
			if JsonOutput || maatJSON {
				return emitJSON(verdict)
			}
			return renderMaatTriage(verdict, snapshotHash, "preview", "Ma'at observed this Mac but did not write the Casebook. Review the deterministic screen, then confirm recording.", "")
		}

		journal, err := newMaatDecisionJournal()
		if err != nil {
			return fmt.Errorf("Ma'at System One: open local Casebook: %w", err)
		}
		decision, err := maat.RecordSystemOne(journal, "sirsi maat triage", verdict)
		if err != nil {
			return fmt.Errorf("Ma'at System One: retain local health screen: %w", err)
		}
		if JsonOutput || maatJSON {
			return emitJSON(struct {
				maat.MaatVerdict
				SnapshotEvidence string `json:"snapshot_evidence"`
				DecisionEvidence string `json:"decision_evidence"`
			}{MaatVerdict: verdict, SnapshotEvidence: snapshotHash, DecisionEvidence: decision.Evidence})
		}
		return renderMaatTriage(verdict, snapshotHash, string(verdict.Gate), systemOneSummary(verdict), decision.Evidence)
	},
}

func renderMaatTriage(verdict maat.MaatVerdict, snapshotHash, status, summary, decisionEvidence string) error {
	result := &output.CommandResult{
		Command:    "sirsi maat triage",
		BriefTitle: "Ma'at System One health screen",
		Status:     status,
		Summary:    summary,
		Evidence: []output.Evidence{
			{Label: "Host", Value: verdict.Subject.Repo},
			{Label: "Diagnostic snapshot", Value: snapshotHash},
			{Label: "Deterministic gate", Value: string(verdict.Gate)},
		},
	}
	if decisionEvidence != "" {
		result.Evidence = append(result.Evidence, output.Evidence{Label: "Ma'at Casebook", Value: decisionEvidence})
		result.NextActions = []output.NextAction{{
			Label: "Open Ma'at Casebook", Command: "sirsi maat casebook",
			Description: "Continue from the exact retained screen. Ma'at will show the next bounded repair, review, or owner-acceptance route.",
		}}
	} else {
		result.NextActions = []output.NextAction{{
			Label: "Record this screen", Command: "sirsi maat triage --confirm",
			Description: "Write this exact local observation to Ma'at Casebook after you review its deterministic gate.",
		}}
	}
	result.Render()
	return nil
}

func maatHostSystemOneScreen(report *guard.DoctorReport, host string, latencyMS int64) (maat.SystemOneScreen, string, error) {
	if report == nil {
		return maat.SystemOneScreen{}, "", fmt.Errorf("Ma'at System One: diagnostic returned no report")
	}
	host = strings.TrimSpace(host)
	if host == "" {
		return maat.SystemOneScreen{}, "", fmt.Errorf("Ma'at System One: local host identity is empty")
	}
	raw, err := json.Marshal(report)
	if err != nil {
		return maat.SystemOneScreen{}, "", fmt.Errorf("Ma'at System One: hash local diagnostic: %w", err)
	}
	sum := sha256.Sum256(raw)
	snapshotHash := "diagnostic:sha256=" + hex.EncodeToString(sum[:])
	findings := make([]maat.ScreenFinding, 0)
	for _, finding := range report.Findings {
		if finding.Severity < guard.SeverityWarn {
			continue
		}
		severity := "major"
		if finding.Severity >= guard.SeverityCritical {
			severity = "block"
		}
		fixHint := "Level 1: re-run this exact observation after conditions change. Level 2: record a Ma'at owner review. Level 3: accept an evidence-bound owner conclusion in Casebook."
		if finding.Fix != "" {
			fixHint = "Level 1: " + finding.Fix + ". Level 2: record a Ma'at owner review if the bounded repair cannot resolve it. Level 3: accept an evidence-bound owner conclusion in Casebook."
		}
		repairID := ""
		// This maps one exact doctor finding to one exact Ma'at-owned recovery.
		// It does not make doctor.Fix executable and it does not expose a generic
		// repair field to imported System One evidence.
		if finding.Check == maatRepairLaunchdDisabledCheck && guard.ResolutionFor(finding) == guard.ResolutionRepair {
			repairID = maat.SystemOneRepairLaunchdDisabled
		}
		findings = append(findings, maat.ScreenFinding{
			ID: "host-health-" + stableMaatFindingID(finding.Check), Severity: severity, Category: "host-health",
			Claim: finding.Message, Evidence: snapshotHash + ":" + stableMaatFindingID(finding.Check), Confidence: 1,
			FixHint: fixHint, RepairID: repairID,
		})
	}
	if latencyMS < 0 {
		latencyMS = 0
	}
	return maat.SystemOneScreen{
		Subject:       maat.VerdictSubject{Kind: "host", Repo: host, Ref: "diagnostic", HeadSHA: hex.EncodeToString(sum[:])},
		FeatherWeight: report.Score, Confidence: 1, Findings: findings,
		Floor: maat.FloorResult{Passed: true, Checks: []maat.FloorCheck{{Name: "diagnostic observation", Passed: true, Detail: "one complete local doctor report was hashed before screening"}}},
		Model: maat.ModelStamp{Provider: "maat-local:deterministic", Version: "v1", Local: true, LatencyMS: int(latencyMS)},
	}, snapshotHash, nil
}

func stableMaatFindingID(check string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(check)))
	return hex.EncodeToString(sum[:6])
}

func init() {
	maatTriageCmd.Flags().BoolVar(&maatTriageConfirm, "confirm", false, "Record the exact local health screen in Ma'at Casebook")
	maatTriageCmd.Flags().BoolVar(&maatJSON, "json", false, "Output the typed System One screen as JSON")
	maatCmd.AddCommand(maatTriageCmd)
}
