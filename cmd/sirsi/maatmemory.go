package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/SirsiMaster/sirsi-pantheon/internal/maat"
	"github.com/SirsiMaster/sirsi-pantheon/internal/output"
)

var (
	maatMemoryStore      string
	maatMemoryComponent  string
	maatMemoryProfile    string
	maatMemoryOperation  string
	maatMemoryConfirm    bool
	maatMemoryReceiptSHA string
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
		store, err := openMaatMemoryExisting(maatMemoryStore)
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
			stored, err := store.RetainPreflightReceipt(receipt)
			if err != nil {
				return err
			}
			if err := maat.ProjectFailureMemoryPreflight(journal, stored); err != nil {
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
		"--component " + shellQuote(maatMemoryComponent),
		"--profile " + shellQuote(maatMemoryProfile),
		"--operation " + shellQuote(maatMemoryOperation),
	}
	if strings.TrimSpace(maatMemoryStore) != "" {
		parts = append(parts, "--store "+shellQuote(maatMemoryStore))
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
	_ = maatFailureMemoryPreflightCmd.MarkFlagRequired("component")
	_ = maatFailureMemoryPreflightCmd.MarkFlagRequired("profile")
	_ = maatFailureMemoryPreflightCmd.MarkFlagRequired("operation")
	maatFailureMemoryReceiptCmd.Flags().StringVar(&maatMemoryReceiptSHA, "sha256", "", "exact retained preflight receipt SHA256")
	maatFailureMemoryReceiptCmd.Flags().StringVar(&maatMemoryReceiptStore, "store", "", "optional absolute existing private legacy store; never initializes")
	maatFailureMemoryReceiptCmd.Flags().BoolVar(&maatJSON, "json", false, "JSON output")
	maatFailureMemoryReceiptCmd.Flags().BoolVar(&maatMemoryReceiptRecover, "recover", false, "complete only this exact already-retained interrupted receipt; no action or projection")
	maatFailureMemoryReceiptCmd.Flags().BoolVar(&maatMemoryReceiptConfirm, "confirm", false, "confirm exact historical receipt write recovery")
	_ = maatFailureMemoryReceiptCmd.MarkFlagRequired("sha256")
	maatFailureMemoryCmd.AddCommand(maatFailureMemoryPreflightCmd, maatFailureMemoryReceiptCmd)
	maatCmd.AddCommand(maatFailureMemoryCmd)
}

// Reads always open existing custody. Explicit legacy roots never acquire the
// default store's initialized authority through this inspection command.
func openMaatMemoryExisting(root string) (*maat.Store, error) {
	if strings.TrimSpace(root) == "" {
		return maat.OpenDefaultStore()
	}
	return maat.OpenExistingStore(strings.TrimSpace(root))
}

var maatMemoryReceiptStore string
var maatMemoryReceiptRecover bool
var maatMemoryReceiptConfirm bool
var maatFailureMemoryReceiptCmd = &cobra.Command{
	Use: "receipt", Short: "Read an exact retained historical preflight; grants no action authority", Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		if maatMemoryReceiptRecover != maatMemoryReceiptConfirm {
			return errors.New("exact receipt recovery requires --recover and --confirm together; no bytes were changed")
		}
		store, err := openMaatMemoryExisting(maatMemoryReceiptStore)
		if err != nil {
			return fmt.Errorf("open existing receipt custody: %w", err)
		}
		defer store.Close()
		var receipt maat.PreflightReceiptRead
		if maatMemoryReceiptRecover {
			receipt, err = store.RecoverPreflightReceipt(maatMemoryReceiptSHA)
		} else {
			receipt, err = store.ReadPreflightReceipt(maatMemoryReceiptSHA)
		}
		if err != nil {
			if (JsonOutput || maatJSON) && errors.Is(err, maat.ErrHistoricalReceiptUnavailable) {
				if outputErr := emitJSON(struct {
					Schema        string `json:"schema"`
					ReceiptSHA256 string `json:"receipt_sha256"`
					ErrorCode     string `json:"error_code"`
				}{maat.PreflightReceiptReadSchema, maatMemoryReceiptSHA, "historical-receipt-unavailable"}); outputErr != nil {
					return outputErr
				}
			}
			return err
		}
		if JsonOutput || maatJSON {
			return emitJSON(receipt)
		}
		data, err := json.MarshalIndent(receipt, "", "  ")
		if err != nil {
			return err
		}
		cmd.Println(string(data))
		return nil
	},
}
