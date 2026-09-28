package main

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/SirsiMaster/sirsi-pantheon/internal/maat"
	"github.com/SirsiMaster/sirsi-pantheon/internal/output"
)

var (
	maatPreflightRoot    string
	maatPreflightConfirm bool
)

// maatPreflightCmd owns non-executing Ma'at observations. Each child must
// construct typed local evidence without launching the assessed workflow; a
// separate --confirm is required before the shared decision journal remembers
// it and Casebook projects it everywhere else.
var maatPreflightCmd = &cobra.Command{
	Use:   "preflight",
	Short: "Construct non-executing Ma'at evidence from local sources",
}

var maatPreflightReleaseCmd = &cobra.Command{
	Use:   "release",
	Short: "Preflight Pantheon's commercial release source contract",
	Long: `Observe and hash the local release-artifact sources without executing a build,
package, signing, notarization, network, or release command.

The result is a delivery-bound System One verdict: a green deterministic floor
still requires the separate credentialed release proof. Use --confirm only
after reviewing the evidence to retain the same result in Ma'at's Casebook.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		preflight, err := maat.PreflightReleaseContract(maatPreflightRoot)
		if err != nil {
			return err
		}
		decisionEvidence := ""
		if maatPreflightConfirm {
			journal, journalErr := newMaatDecisionJournal()
			if journalErr != nil {
				return fmt.Errorf("open Ma'at decision journal: %w", journalErr)
			}
			decision, recordErr := maat.RecordSystemOne(journal, "sirsi maat preflight release", preflight.Verdict)
			if recordErr != nil {
				return recordErr
			}
			decisionEvidence = decision.Evidence
		}
		if JsonOutput || maatJSON {
			return emitJSON(struct {
				maat.ReleaseContractPreflight
				DecisionEvidence string `json:"decision_evidence,omitempty"`
			}{ReleaseContractPreflight: preflight, DecisionEvidence: decisionEvidence})
		}

		result := &output.CommandResult{
			Command: "sirsi maat preflight release", BriefTitle: "Ma'at release-contract preflight",
			Status: string(preflight.Verdict.Gate), Summary: systemOneSummary(preflight.Verdict),
			Evidence: []output.Evidence{
				{Label: "Source fingerprint", Value: preflight.Fingerprint},
				{Label: "Deterministic floor", Value: floorLabel(preflight.Verdict.Floor.Passed)},
				{Label: "Observed files", Value: fmt.Sprintf("%d", len(preflight.Files))},
			},
		}
		if decisionEvidence == "" {
			result.NextActions = append(result.NextActions, output.NextAction{
				Label: "Record in Ma'at Casebook", Command: "sirsi maat preflight release --root " + shellQuote(maatPreflightRoot) + " --confirm",
				Description: "After inspecting the typed source evidence, explicitly retain this preflight in the shared Casebook.",
			})
		} else {
			result.Evidence = append(result.Evidence, output.Evidence{Label: "Casebook evidence", Value: decisionEvidence})
			result.NextActions = append(result.NextActions, output.NextAction{
				Label: "Review Ma'at Casebook", Command: "sirsi maat casebook --kind governance --status open",
				Description: "Inspect this evidence with its guided recovery route; it does not authorize a release.",
			})
		}
		for _, finding := range preflight.Verdict.Findings {
			result.AddEvidence("Finding: "+finding.ID, finding.Claim)
		}
		result.Render()
		return nil
	},
}

func shellQuote(value string) string {
	if value == "" {
		return "."
	}
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}

func init() {
	maatPreflightReleaseCmd.Flags().StringVar(&maatPreflightRoot, "root", ".", "Pantheon checkout root to inspect")
	maatPreflightReleaseCmd.Flags().BoolVar(&maatPreflightConfirm, "confirm", false, "confirm recording this typed preflight in Ma'at's Casebook")
	maatPreflightReleaseCmd.Flags().BoolVar(&maatJSON, "json", false, "JSON output")
	maatPreflightCmd.AddCommand(maatPreflightReleaseCmd)
	maatCmd.AddCommand(maatPreflightCmd)
}
