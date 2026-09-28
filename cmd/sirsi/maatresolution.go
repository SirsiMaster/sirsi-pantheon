package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/SirsiMaster/sirsi-pantheon/internal/maat"
	"github.com/SirsiMaster/sirsi-pantheon/internal/output"
)

var (
	maatResolutionCheck          string
	maatResolutionMessage        string
	maatResolutionDetail         string
	maatResolutionOriginEvidence string
	maatResolutionConfirm        bool
	maatAcceptanceEvidence       string
	maatAcceptanceNote           string
	maatAcceptanceConfirm        bool
)

// maatRecordResolutionCmd is the non-automating third level of Ma'at's guided
// resolution ladder. It preserves an evidence-bound owner decision when no
// safe automatic action exists; recording the decision never repairs, starts,
// stops, signs, or otherwise mutates the diagnosed subsystem.
var maatRecordResolutionCmd = &cobra.Command{
	Use:   "record-resolution",
	Short: "Record an evidence-bound Ma'at owner-resolution decision",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		if !maatResolutionConfirm {
			return fmt.Errorf("recording an owner-resolution decision changes the Ma'at journal; rerun with --confirm after reviewing the finding")
		}
		journal, err := newMaatDecisionJournal()
		if err != nil {
			return fmt.Errorf("open Ma'at decision journal: %w", err)
		}
		decision, err := maat.RecordDiagnosticOwnerReview(journal, "sirsi maat record-resolution", maat.DiagnosticOwnerReview{
			Check: maatResolutionCheck, Message: maatResolutionMessage, Detail: maatResolutionDetail, OriginEvidence: maatResolutionOriginEvidence,
		})
		if err != nil {
			return err
		}
		result := &output.CommandResult{
			Command:    "sirsi maat record-resolution",
			BriefTitle: "Ma'at Owner Resolution",
			Status:     "ok",
			Summary:    "Ma'at recorded the owner-resolution decision. No system change was made.",
			Evidence: []output.Evidence{
				{Label: "Finding", Value: decision.Resource},
				{Label: "Evidence", Value: decision.Evidence},
				{Label: "Conclusion", Value: "owner review required"},
			},
			NextActions: []output.NextAction{{
				Label: "Review Ma'at casebook", Command: "sirsi maat casebook", Description: "Inspect the durable evidence and record the owner decision when the required action is known.",
			}},
		}
		result.Render()
		return nil
	},
}

var maatAcceptResolutionCmd = &cobra.Command{
	Use:   "accept-resolution",
	Short: "Record an explicit owner acceptance for a Ma'at resolution case",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		if !maatAcceptanceConfirm {
			return fmt.Errorf("accepting an owner-resolution case changes the Ma'at journal; rerun with --confirm after reviewing the case evidence")
		}
		journal, err := newMaatDecisionJournal()
		if err != nil {
			return fmt.Errorf("open Ma'at decision journal: %w", err)
		}
		decision, err := maat.AcceptDiagnosticOwnerReview(journal, "sirsi maat accept-resolution", maat.DiagnosticOwnerAcceptance{
			Evidence: maatAcceptanceEvidence, Conclusion: maatAcceptanceNote,
		})
		if err != nil {
			return err
		}
		result := &output.CommandResult{
			Command: "sirsi maat accept-resolution", BriefTitle: "Ma'at Owner Acceptance", Status: "ok",
			Summary:     "Ma'at recorded the owner acceptance. The diagnostic system state was not changed or claimed repaired.",
			Evidence:    []output.Evidence{{Label: "Accepted case", Value: decision.ResolutionFor}, {Label: "Acceptance evidence", Value: decision.Evidence}},
			NextActions: []output.NextAction{{Label: "Review Ma'at casebook", Command: "sirsi maat casebook", Description: "Confirm the resolved case and retain the linked owner acceptance evidence."}},
		}
		result.Render()
		return nil
	},
}

func init() {
	maatRecordResolutionCmd.Flags().StringVar(&maatResolutionCheck, "check", "", "diagnostic finding name")
	maatRecordResolutionCmd.Flags().StringVar(&maatResolutionMessage, "message", "", "diagnostic finding summary")
	maatRecordResolutionCmd.Flags().StringVar(&maatResolutionDetail, "detail", "", "optional observed detail")
	maatRecordResolutionCmd.Flags().StringVar(&maatResolutionOriginEvidence, "origin-evidence", "", "optional exact evidence reference of the existing Ma'at case")
	maatRecordResolutionCmd.Flags().BoolVar(&maatResolutionConfirm, "confirm", false, "confirm recording the Ma'at owner-resolution decision")
	maatAcceptResolutionCmd.Flags().StringVar(&maatAcceptanceEvidence, "evidence", "", "exact evidence reference of the owner-resolution case")
	maatAcceptResolutionCmd.Flags().StringVar(&maatAcceptanceNote, "note", "", "owner acceptance conclusion")
	maatAcceptResolutionCmd.Flags().BoolVar(&maatAcceptanceConfirm, "confirm", false, "confirm recording the Ma'at owner acceptance")
	maatCmd.AddCommand(maatRecordResolutionCmd)
	maatCmd.AddCommand(maatAcceptResolutionCmd)
}
