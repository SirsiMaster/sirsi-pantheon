package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/SirsiMaster/sirsi-pantheon/internal/maat"
	"github.com/SirsiMaster/sirsi-pantheon/internal/output"
)

var (
	maatScreenInput   string
	maatScreenConfirm bool
)

// maatScreenCmd turns a closed, typed local observation into Ma'at's
// JEV-like System One verdict. It is deliberately provider-neutral: the
// deterministic policy validates the supplied observation, refuses malformed
// schema, and appends the same result that Casebook projects everywhere.
var maatScreenCmd = &cobra.Command{
	Use:   "screen --input <system-one-screen.json>",
	Short: "Record a strict local Ma'at System One screen",
	Long: `Turn a closed local screen input into Ma'at's strict System One verdict.

The deterministic floor always wins. Sensitive boundaries and low-confidence
screens escalate to frontier review; this command does not authorize mutation,
release, router work, signing, installation, or a final owner decision.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		if strings.TrimSpace(maatScreenInput) == "" {
			return fmt.Errorf("--input is required")
		}
		raw, err := os.ReadFile(maatScreenInput)
		if err != nil {
			return fmt.Errorf("read Ma'at System One input: %w", err)
		}
		decoder := json.NewDecoder(bytes.NewReader(raw))
		decoder.DisallowUnknownFields()
		var input maat.SystemOneScreen
		if decodeErr := decoder.Decode(&input); decodeErr != nil {
			return fmt.Errorf("decode Ma'at System One input: %w", decodeErr)
		}
		var trailing any
		if trailingErr := decoder.Decode(&trailing); trailingErr != io.EOF {
			if trailingErr == nil {
				return fmt.Errorf("decode Ma'at System One input: multiple JSON values")
			}
			return fmt.Errorf("decode Ma'at System One input: trailing JSON: %w", trailingErr)
		}
		verdict, err := maat.Screen(input)
		if err != nil {
			return err
		}
		// A valid screen is still a durable journal write. Validate the closed
		// packet first so an operator gets useful feedback, then require the same
		// explicit confirmation used by Ma'at owner review and acceptance before
		// opening the decision writer.
		if !maatScreenConfirm {
			return fmt.Errorf("recording a Ma'at System One screen writes the decision journal; rerun with --confirm after reviewing the deterministic gate")
		}
		journal, err := newMaatDecisionJournal()
		if err != nil {
			return fmt.Errorf("open Ma'at decision journal: %w", err)
		}
		decision, err := maat.RecordSystemOne(journal, "sirsi maat screen", verdict)
		if err != nil {
			return err
		}
		if JsonOutput || maatJSON {
			return emitJSON(verdict)
		}
		result := &output.CommandResult{
			Command:    "sirsi maat screen",
			BriefTitle: "Ma'at System One",
			Status:     string(verdict.Gate),
			Summary:    systemOneSummary(verdict),
			Evidence: []output.Evidence{
				{Label: "Subject", Value: verdict.Subject.Kind + " " + verdict.Subject.Ref},
				{Label: "Head", Value: verdict.Subject.HeadSHA},
				{Label: "Evidence", Value: decision.Evidence},
			},
			NextActions: []output.NextAction{{
				Label: "Review Ma'at casebook", Command: "sirsi maat casebook",
				Description: "Inspect the same evidence-bound System One result in the shared local Casebook.",
			}},
		}
		if verdict.Gate == maat.GateEscalate {
			result.NextActions = append([]output.NextAction{{
				Label: "Request frontier review", Command: "sirsi maat casebook --kind governance --status open",
				Description: "A screen is not a binding decision. Review the retained evidence and record an explicit owner conclusion when appropriate.",
			}}, result.NextActions...)
		}
		result.Render()
		return nil
	},
}

func systemOneSummary(verdict maat.MaatVerdict) string {
	switch verdict.Gate {
	case maat.GatePass:
		return "Local System One found no issues above its configured threshold. This is evidence, not mutation authority."
	case maat.GateChanges:
		return "Local System One found bounded changes to address. The Casebook retains the evidence and a guided decision route."
	case maat.GateBlock:
		return "Local System One blocked this screen on deterministic or high-confidence evidence. It did not change the assessed system."
	case maat.GateEscalate:
		return "Local System One escalated this screen. A sensitive boundary or insufficient confidence requires an explicit frontier review."
	default:
		return "Local System One recorded an unrecognized screen result."
	}
}

func init() {
	maatScreenCmd.Flags().StringVar(&maatScreenInput, "input", "", "closed Ma'at System One screen JSON")
	maatScreenCmd.Flags().BoolVar(&maatScreenConfirm, "confirm", false, "confirm recording the validated Ma'at System One screen")
	maatScreenCmd.Flags().BoolVar(&maatJSON, "json", false, "JSON output")
	maatCmd.AddCommand(maatScreenCmd)
}
