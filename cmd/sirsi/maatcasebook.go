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
	maatCasebookScope         string
	maatCasebookEvidence      string
	maatCasebookCursor        string
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
	JournalIntegrity maat.JournalIntegrity  `json:"journal_integrity"`
	History          casebook.HistoryWindow `json:"history"`
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
		if maatCasebookScope != maat.HistoryScopeAll && maatCasebookScope != maat.HistoryScopeSystemOne {
			return fmt.Errorf("invalid --scope %q (want all or system-one)", maatCasebookScope)
		}
		journal, err := newMaatDecisionJournal()
		if err != nil {
			return err
		}
		var view casebook.View
		var integrity maat.JournalIntegrity
		var history casebook.HistoryWindow
		if fileJournal, ok := journal.(*maat.FileDecisionJournal); ok {
			result, searchErr := casebook.SearchHistory(fileJournal, casebook.HistoryQuery{
				Query: casebook.Query{
					Text: strings.Join(args, " "), Kind: maatCasebookKind, Status: status, Limit: maatCasebookLimit,
				},
				Scope: maatCasebookScope, Evidence: maatCasebookEvidence, Cursor: maatCasebookCursor, Context: cmd.Context(),
			})
			if searchErr != nil {
				return searchErr
			}
			view, integrity, history = result.View, result.Integrity, result.Window
		} else {
			// Alternate test/transport implementations retain their existing
			// read behavior until they implement the bounded snapshot contract.
			var rows []maat.Decision
			if tolerant, ok := journal.(tolerantCasebookJournal); ok {
				rows, integrity, err = tolerant.RecentTolerant(maatCasebookLimit)
			} else {
				rows, err = journal.Recent(maatCasebookLimit)
			}
			if err != nil {
				return err
			}
			if maatCasebookCursor != "" {
				return fmt.Errorf("this journal backend does not support snapshot continuation; no history claim was made")
			}
			filteredRows := make([]maat.Decision, 0, len(rows))
			for _, row := range rows {
				if maatCasebookScope == maat.HistoryScopeSystemOne && row.SystemOne == nil && row.SystemOneCalibration == nil {
					continue
				}
				if maatCasebookEvidence != "" && row.Evidence != maatCasebookEvidence {
					continue
				}
				filteredRows = append(filteredRows, row)
			}
			view = casebook.Search(filteredRows, casebook.Query{
				Text: strings.Join(args, " "), Kind: maatCasebookKind, Status: status, Limit: maatCasebookLimit,
			})
			history = casebook.HistoryWindow{Scope: maatCasebookScope, Returned: len(view.Cases), PageSize: maatCasebookLimit, Coverage: "unavailable"}
		}
		if maatJSON {
			return emitJSON(maatCasebookProjection{View: view, JournalIntegrity: integrity, History: history})
		}
		if integrity.InvalidCount > 0 {
			fmt.Printf("⚠ Ma'at casebook retained %d invalid journal record(s); valid cases remain visible. Open Pantheon to review the guided preservation repair.\n", integrity.InvalidCount)
		}
		if len(view.Cases) == 0 {
			if history.Coverage == maat.HistoryCoveragePartial {
				fmt.Printf("𓆄 Ma'at history scan is incomplete (%d records, %d bytes); continue with --cursor %s\n", history.ScannedRecords, history.ScannedBytes, history.NextCursor)
			} else if history.Coverage != maat.HistoryCoverageComplete {
				fmt.Println("𓆄 this journal backend cannot establish complete history; no absence conclusion was made")
			} else if history.CountScope == "after-cursor" || history.CountScope == "after-cursor-scanned-so-far" {
				fmt.Println("𓆄 no matching cases remain after this history cursor; the original query total is not inferred from this page")
			} else if history.HasMore {
				fmt.Printf("𓆄 no matching cases on this page; older history is available with --cursor %s\n", history.NextCursor)
			} else if integrity.InvalidCount > 0 {
				fmt.Printf("𓆄 no valid cases matched, but %d invalid journal record(s) prevent a complete absence conclusion\n", integrity.InvalidCount)
			} else {
				fmt.Println("𓆄 no Ma'at cases match this complete query")
			}
			return nil
		}
		fmt.Printf("𓆄 Ma'at casebook — %d cases · %d open · %d urgent · %d high\n", view.Summary.Total, view.Summary.Open, view.Summary.Urgent, view.Summary.High)
		if history.Coverage == maat.HistoryCoveragePartial {
			fmt.Printf("  history scan incomplete; continue with --cursor %s\n", history.NextCursor)
		} else if history.Coverage != maat.HistoryCoverageComplete {
			fmt.Println("  history completeness unavailable from this journal backend")
		} else if history.HasMore {
			fmt.Printf("  older history is available with --cursor %s\n", history.NextCursor)
		}
		if history.Coverage == maat.HistoryCoverageComplete && !history.TotalComplete {
			if history.CountScope == "after-cursor" || history.CountScope == "after-cursor-scanned-so-far" {
				fmt.Printf("  %d matching cases occur after this cursor; this is not a whole-query total\n", history.CandidateMatches)
			} else {
				fmt.Printf("  %d matching cases on the initial query; invalid history prevents complete totals\n", history.CandidateMatches)
			}
		}
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
			if c.VerifiedRepairEvidence != "" {
				fmt.Printf("          verified binary repair receipt: %s · screen status: %s\n", c.VerifiedRepairEvidence, c.Status)
			} else if c.RepairOutcomeEvidence != "" {
				fmt.Printf("          unverified repair outcome: %s\n", c.RepairOutcomeEvidence)
			}
			if c.PendingRepairAttemptEvidence != "" {
				fmt.Printf("          unresolved updater attempt: %s · inspect before any new consent\n", c.PendingRepairAttemptEvidence)
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
	maatCasebookCmd.Flags().StringVar(&maatCasebookScope, "scope", maat.HistoryScopeAll, "history scope: all or system-one")
	maatCasebookCmd.Flags().StringVar(&maatCasebookEvidence, "evidence", "", "exact evidence identity to inspect")
	maatCasebookCmd.Flags().StringVar(&maatCasebookCursor, "cursor", "", "continue the same bounded history snapshot")
	maatCasebookCmd.Flags().BoolVar(&maatJSON, "json", false, "JSON output")
	maatCasebookRepairCmd.Flags().BoolVar(&maatCasebookRepairConfirm, "confirm", false, "confirm preservation and active Casebook rebuild")
	maatCasebookCmd.AddCommand(maatCasebookRepairCmd)
	maatCmd.AddCommand(maatCasebookCmd)
}
