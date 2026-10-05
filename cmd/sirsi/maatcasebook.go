package main

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/SirsiMaster/sirsi-pantheon/internal/maat"
	"github.com/SirsiMaster/sirsi-pantheon/internal/maat/casebook"
)

var (
	maatCasebookKind          string
	maatCasebookStatus        string
	maatCasebookLimit         int
	maatCasebookRepairConfirm bool
)

// tolerantCasebookJournal is deliberately narrower than DecisionJournal.
// Casebook is a read-only operator projection, so it may keep healthy
// decision history visible while reporting a malformed legacy row. Mutation
// and authorization paths retain the strict DecisionJournal.Recent contract.
type tolerantCasebookJournal interface {
	maat.DecisionJournal
	RecentTolerant(limit int) ([]maat.Decision, maat.JournalIntegrity, error)
}

type maatCasebookProjection struct {
	casebook.View
	JournalIntegrity maat.JournalIntegrity `json:"journal_integrity"`
}

type repairableCasebookJournal interface {
	maat.DecisionJournal
	RepairInvalidRecords() (maat.JournalRepairReceipt, error)
}

// maatCasebookCmd is a local System One view over Ma'at's recorded decisions.
// It deliberately does not write, re-score, or authorize anything: Ma'at's
// journal stays the policy authority and the casebook makes it inspectable.
var maatCasebookCmd = &cobra.Command{
	Use:   "casebook [text]",
	Short: "Search and prioritize Ma'at's local System One casebook",
	Long: `Search Ma'at's recorded decisions as classified, evidence-linked cases.

The casebook is local to this Pantheon node. It projects the recorded decision
journal without changing Ma'at's determination or creating a second policy
engine. Use --kind for a source kind or case category, and --status open or
resolved to narrow the view.`,
	Args: cobra.ArbitraryArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		status := casebook.Status(strings.ToLower(strings.TrimSpace(maatCasebookStatus)))
		if status != "" && status != casebook.StatusOpen && status != casebook.StatusResolved {
			return fmt.Errorf("invalid --status %q (want open or resolved)", maatCasebookStatus)
		}
		journal, err := newMaatDecisionJournal()
		if err != nil {
			return err
		}
		var rows []maat.Decision
		var integrity maat.JournalIntegrity
		if tolerant, ok := journal.(tolerantCasebookJournal); ok {
			rows, integrity, err = tolerant.RecentTolerant(maatCasebookLimit)
		} else {
			rows, err = journal.Recent(maatCasebookLimit)
		}
		if err != nil {
			return err
		}
		view := casebook.Search(rows, casebook.Query{
			Text: strings.Join(args, " "), Kind: maatCasebookKind, Status: status, Limit: maatCasebookLimit,
		})
		if maatJSON {
			return emitJSON(maatCasebookProjection{View: view, JournalIntegrity: integrity})
		}
		if integrity.InvalidCount > 0 {
			fmt.Printf("⚠ Ma'at casebook retained %d invalid journal record(s); valid cases remain visible. Open Pantheon to review the guided preservation repair.\n", integrity.InvalidCount)
		}
		if len(view.Cases) == 0 {
			fmt.Println("𓆄 no Ma'at cases match this query")
			return nil
		}
		fmt.Printf("𓆄 Ma'at casebook — %d cases · %d open · %d urgent · %d high\n", view.Summary.Total, view.Summary.Open, view.Summary.Urgent, view.Summary.High)
		for _, c := range view.Cases {
			fmt.Printf("  %-7s %-8s %-11s %-14s %s\n", strings.ToUpper(string(c.Priority)), c.Status, c.Category, c.Resource, c.Why)
			fmt.Printf("          %s · %s", c.Kind, c.Determination)
			if c.Evidence != "" {
				fmt.Printf(" · evidence: %s", c.Evidence)
			}
			fmt.Println()
			if c.NextAction != nil {
				fmt.Printf("          next step: %s · %s\n", c.NextAction.Title, c.NextAction.Detail)
				for _, step := range c.NextAction.Steps {
					fmt.Printf("          recovery level %d: %s · %s\n", step.Level, step.Title, step.Detail)
				}
			}
			if c.SystemOne != nil {
				fmt.Printf("          screen model: %s %s · %s · %dms\n", c.SystemOne.Model.Provider, c.SystemOne.Model.Version, localModelLabel(c.SystemOne.Model.Local), c.SystemOne.Model.LatencyMS)
				fmt.Printf("          deterministic floor: %s\n", floorLabel(c.SystemOne.Floor.Passed))
				for _, check := range c.SystemOne.Floor.Checks {
					fmt.Printf("          floor check [%s]: %s", check.Name, floorLabel(check.Passed))
					if check.Detail != "" {
						fmt.Printf(" · %s", check.Detail)
					}
					fmt.Println()
				}
				for _, finding := range c.SystemOne.Findings {
					fmt.Printf("          finding [%s · %s]: %s\n", finding.Severity, finding.Category, finding.Claim)
					if finding.FixHint != "" {
						// A fix hint is retained producer evidence, not a command
						// dispatcher. Present the same recovery step every surface
						// sees, but never execute it merely by rendering Casebook.
						fmt.Printf("          prescribed next step: %s\n", finding.FixHint)
					}
				}
			}
			if c.Resolution != "" {
				fmt.Printf("          owner acceptance: %s · system repair: not claimed\n", c.Resolution)
			}
		}
		return nil
	},
}

// maatCasebookRepairCmd is the only mutation path for a legacy Casebook
// projection. It never accepts a caller-selected path or record list: the
// active local journal is preserved as a create-only backup, rebuilt from
// records that pass the existing strict contract, and strictly re-read before
// the native application can report recovery.
var maatCasebookRepairCmd = &cobra.Command{
	Use:   "repair",
	Short: "Preserve invalid local decision records and rebuild the active Casebook",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		if !maatCasebookRepairConfirm {
			return fmt.Errorf("Ma'at Casebook repair preserves the original journal and replaces the active projection; rerun with --confirm after reviewing the integrity summary")
		}
		journal, err := newMaatDecisionJournal()
		if err != nil {
			return err
		}
		repairable, ok := journal.(repairableCasebookJournal)
		if !ok {
			return fmt.Errorf("Ma'at Casebook repair is unavailable for this journal backend; no journal data changed")
		}
		receipt, err := repairable.RepairInvalidRecords()
		if err != nil {
			return err
		}
		result := struct {
			Status  string                    `json:"status"`
			Summary string                    `json:"summary"`
			Repair  maat.JournalRepairReceipt `json:"repair"`
		}{
			Status: "ok", Summary: "Ma'at preserved the original decision journal and verified the recovered active Casebook.", Repair: receipt,
		}
		if maatJSON {
			return emitJSON(result)
		}
		fmt.Printf("Ma'at Casebook recovered: preserved original at %s · removed %d invalid record(s) · retained %d verified record(s)\n", receipt.BackupPath, receipt.RemovedCount, receipt.RetainedCount)
		return nil
	},
}

func floorLabel(passed bool) string {
	if passed {
		return "passed"
	}
	return "failed"
}

func localModelLabel(local bool) string {
	if local {
		return "local"
	}
	return "external"
}

func init() {
	maatCasebookCmd.Flags().StringVar(&maatCasebookKind, "kind", "", "source decision kind or case category")
	maatCasebookCmd.Flags().StringVar(&maatCasebookStatus, "status", "", "case status: open or resolved")
	maatCasebookCmd.Flags().IntVar(&maatCasebookLimit, "limit", 50, "maximum cases to inspect")
	maatCasebookCmd.Flags().BoolVar(&maatJSON, "json", false, "JSON output")
	maatCasebookRepairCmd.Flags().BoolVar(&maatCasebookRepairConfirm, "confirm", false, "confirm preservation and active Casebook rebuild")
	maatCasebookCmd.AddCommand(maatCasebookRepairCmd)
	maatCmd.AddCommand(maatCasebookCmd)
}
