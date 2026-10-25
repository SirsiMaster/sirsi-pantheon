package main

import (
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/SirsiMaster/sirsi-pantheon/internal/guard"
	"github.com/SirsiMaster/sirsi-pantheon/internal/maat"
	"github.com/SirsiMaster/sirsi-pantheon/internal/output"
)

var (
	maatTriageConfirm       bool
	maatTriageReadDiagnosis = pantheonDoctor
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
		if maatTriageInspectRecords || maatTriageReconcileRecord != "" {
			return inspectOrReconcileMaatDiagnostic()
		}
		started := time.Now()
		report, err := maatTriageReadDiagnosis()
		if err != nil {
			return fmt.Errorf("Ma'at System One: local diagnostic observation is unavailable")
		}
		host, err := maatTriageHostname()
		if err != nil {
			return fmt.Errorf("Ma'at System One: local host provenance is unavailable")
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
			status := maat.InspectDefaultStore()
			provenance := "fresh"
			observation, obsErr := maatDiagnosticObservation(report, host)
			if obsErr != nil || observation.ValidateFresh() != nil {
				provenance = "unknown"
			}
			if JsonOutput || maatJSON {
				return emitJSON(struct {
					maat.MaatVerdict
					MemoryState     maat.DefaultStoreState `json:"memory_state"`
					Provenance      string                 `json:"diagnostic_provenance"`
					MemoryPreflight string                 `json:"memory_preflight"`
					Recorded        bool                   `json:"recorded"`
				}{verdict, status.State, provenance, "not_performed", false})
			}
			return renderMaatTriage(verdict, snapshotHash, "preview", fmt.Sprintf("Local observation only; provenance %s, memory store %s. No Casebook record or memory admission was performed. A confirmed record requires a fresh scoped preflight.", provenance, status.State), "")
		}

		return recordMaatDiagnostic(report, host, verdict, snapshotHash)
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
			Description: "Observe again and request one memory-preflight-bound local record; no prior preview is reused as authority.",
		}}
	}
	result.Render()
	return nil
}

func maatHostSystemOneScreen(report *guard.DoctorReport, host string, _ int64) (maat.SystemOneScreen, string, error) {
	observation, err := maatDiagnosticObservation(report, host)
	if err != nil {
		return maat.SystemOneScreen{}, "", err
	}
	verdict, err := observation.Verdict()
	if err != nil {
		return maat.SystemOneScreen{}, "", err
	}
	// Only bounded status codes enter the screen; producer prose stays local.
	return maat.SystemOneScreen{Subject: verdict.Subject, FeatherWeight: verdict.FeatherWeight, Confidence: verdict.Confidence,
		Findings: verdict.Findings, Floor: verdict.Floor, Model: verdict.Model}, "diagnostic:sha256=" + observation.SourceSHA256, nil
}

func init() {
	maatTriageCmd.Flags().BoolVar(&maatTriageConfirm, "confirm", false, "Record the exact local health screen in Ma'at Casebook")
	maatTriageCmd.Flags().BoolVar(&maatJSON, "json", false, "Output the typed System One screen as JSON")
	maatTriageCmd.Flags().BoolVar(&maatTriageInspectRecords, "inspect-records", false, "read pending diagnostic record identities without re-observation or resend")
	maatTriageCmd.Flags().StringVar(&maatTriageReconcileRecord, "reconcile-record", "", "confirm data-only readback of this exact pending diagnostic record; never append")
	maatCmd.AddCommand(maatTriageCmd)
}
