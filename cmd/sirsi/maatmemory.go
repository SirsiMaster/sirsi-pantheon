package main

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/SirsiMaster/sirsi-pantheon/internal/maat"
	"github.com/SirsiMaster/sirsi-pantheon/internal/output"
)

var (
	maatMemoryStore     string
	maatMemoryComponent string
	maatMemoryProfile   string
	maatMemoryOperation string
	maatMemoryConfirm   bool
)

// maatFailureMemoryCmd exposes the local, evidence-only half of failure
// memory. It never invokes a recovery instruction: callers inspect the
// receipt and explicitly choose an existing authorized operation.
var maatFailureMemoryCmd = &cobra.Command{
	Use:   "failure-memory",
	Short: "Inspect Ma'at's evidence-bound operational failure memory",
}

var maatFailureMemoryPreflightCmd = &cobra.Command{
	Use:   "preflight",
	Short: "Preflight one exact operation against retained failure memory",
	Long: `Read a local Ma'at failure-memory store and evaluate one exact component,
profile, and operation. This command does not run any recovery instruction.

Use --confirm only after inspecting the receipt to project that same factual
result into Ma'at's existing Casebook decision journal.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		store, err := maat.OpenStore(strings.TrimSpace(maatMemoryStore))
		if err != nil {
			return fmt.Errorf("open Ma'at failure-memory store: %w", err)
		}
		defer store.Close()
		receipt, err := store.Preflight(maat.Scope{Component: maatMemoryComponent, Profile: maatMemoryProfile, Operation: maatMemoryOperation})
		if err != nil {
			return err
		}
		decisionEvidence := ""
		if maatMemoryConfirm {
			journal, err := newMaatDecisionJournal()
			if err != nil {
				return fmt.Errorf("open Ma'at decision journal: %w", err)
			}
			if err := maat.ProjectFailureMemoryPreflight(journal, receipt); err != nil {
				return err
			}
			decisionEvidence, err = receipt.EvidenceReference()
			if err != nil {
				return fmt.Errorf("bind Ma'at failure-memory receipt evidence: %w", err)
			}
		}
		if JsonOutput || maatJSON {
			return emitJSON(struct {
				maat.PreflightReceipt
				DecisionEvidence string `json:"decision_evidence,omitempty"`
			}{PreflightReceipt: receipt, DecisionEvidence: decisionEvidence})
		}
		result := &output.CommandResult{
			Command:    "sirsi maat failure-memory preflight",
			BriefTitle: "Ma'at failure-memory preflight",
			Status:     string(receipt.Decision),
			Summary:    failureMemorySummary(receipt),
			Evidence: []output.Evidence{
				{Label: "Action manifest", Value: receipt.ActionManifestSHA256},
				{Label: "Registry snapshot", Value: receipt.RegistrySnapshotSHA256},
				{Label: "Measured records", Value: fmt.Sprintf("%d", len(receipt.MeasuredChecks))},
			},
		}
		for _, action := range receipt.RecoveryActions {
			result.AddEvidence("Recovery: "+action.Label, action.Instruction)
		}
		if decisionEvidence == "" {
			result.NextActions = append(result.NextActions, output.NextAction{
				Label:       "Record this preflight in Ma'at Casebook",
				Command:     failureMemoryCommand(true),
				Description: "After reviewing the exact receipt and recovery guidance, explicitly retain it in the shared Casebook.",
			})
		} else {
			result.Evidence = append(result.Evidence, output.Evidence{Label: "Casebook evidence", Value: decisionEvidence})
			result.NextActions = append(result.NextActions, output.NextAction{
				Label:       "Review Ma'at Casebook",
				Command:     "sirsi maat casebook --kind 'failure memory preflight'",
				Description: "Continue from the recorded evidence; Ma'at does not execute recovery instructions for you.",
			})
		}
		result.Render()
		return nil
	},
}

func failureMemorySummary(receipt maat.PreflightReceipt) string {
	switch receipt.Decision {
	case maat.PreflightPass:
		return "No active retained failure blocks this exact operation. This is local evidence, not execution authority."
	case maat.PreflightReject:
		return fmt.Sprintf("%d active retained failure record(s) match this exact operation. Review the supplied recovery guidance before proceeding.", len(receipt.IncidentKeys))
	default:
		return "The failure-memory registry could not be verified. Do not treat this operation as clear; repair or resolve the retained evidence first."
	}
}

func failureMemoryCommand(confirm bool) string {
	parts := []string{
		"sirsi maat failure-memory preflight",
		"--store " + shellQuote(maatMemoryStore),
		"--component " + shellQuote(maatMemoryComponent),
		"--profile " + shellQuote(maatMemoryProfile),
		"--operation " + shellQuote(maatMemoryOperation),
	}
	if confirm {
		parts = append(parts, "--confirm")
	}
	return strings.Join(parts, " ")
}

func init() {
	maatFailureMemoryPreflightCmd.Flags().StringVar(&maatMemoryStore, "store", "", "absolute existing failure-memory store root")
	maatFailureMemoryPreflightCmd.Flags().StringVar(&maatMemoryComponent, "component", "", "exact component identity")
	maatFailureMemoryPreflightCmd.Flags().StringVar(&maatMemoryProfile, "profile", "", "exact profile identity")
	maatFailureMemoryPreflightCmd.Flags().StringVar(&maatMemoryOperation, "operation", "", "exact operation identity")
	maatFailureMemoryPreflightCmd.Flags().BoolVar(&maatMemoryConfirm, "confirm", false, "confirm recording this typed preflight in Ma'at Casebook")
	maatFailureMemoryPreflightCmd.Flags().BoolVar(&maatJSON, "json", false, "JSON output")
	_ = maatFailureMemoryPreflightCmd.MarkFlagRequired("store")
	_ = maatFailureMemoryPreflightCmd.MarkFlagRequired("component")
	_ = maatFailureMemoryPreflightCmd.MarkFlagRequired("profile")
	_ = maatFailureMemoryPreflightCmd.MarkFlagRequired("operation")
	maatFailureMemoryCmd.AddCommand(maatFailureMemoryPreflightCmd)
	maatCmd.AddCommand(maatFailureMemoryCmd)
}
