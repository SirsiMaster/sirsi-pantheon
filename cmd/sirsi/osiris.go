package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/SirsiMaster/sirsi-pantheon/internal/osiris"
	"github.com/SirsiMaster/sirsi-pantheon/internal/output"
	"github.com/SirsiMaster/sirsi-pantheon/internal/suggest"
)

var osirisCmd = &cobra.Command{
	Use:   "osiris",
	Short: "𓁹 Osiris — Snapshot Keeper & Checkpoint Guardian",
	Long: `𓁹 Osiris — Snapshot Keeper & Checkpoint Guardian

Detects uncommitted work, measures session drift, and warns before data loss.
5-level risk assessment with time-based escalation.

  sirsi osiris risk           Risk assessment of uncommitted work
  sirsi osiris status        One-line risk summary`,
	Run: func(cmd *cobra.Command, args []string) {
		_ = cmd.Help()
	},
}

var osirisAssessCmd = &cobra.Command{
	Use:     "risk [path]",
	Aliases: []string{"assess"},
	Short:   "Risk assessment of uncommitted work",
	Long: `𓁹 Osiris Assess — Checkpoint Assessment

Evaluates the current Git repository state:
  • Uncommitted, staged, modified, untracked, and deleted files
  • Lines added/deleted
  • Time since last commit
  • 5-level risk scoring (none → low → moderate → high → critical)

Risk escalates automatically:
  • 30+ uncommitted files → Critical
  • 2+ hours since last commit → Critical
  • 16-30 files → High

  sirsi risk                      Assess current directory
  sirsi osiris risk /path/to      Assess a specific repo
  sirsi risk --json               Output as JSON`,
	RunE: runOsirisAssess,
}

var osirisStatusCmd = &cobra.Command{
	Use:   "status [path]",
	Short: "One-line risk summary for menu bar or scripting",
	RunE:  runOsirisStatus,
}

var osirisCheckpointCmd = &cobra.Command{
	Use:   "checkpoint [path]",
	Short: "Commit everything as a checkpoint — the lever behind the risk finding",
	Long: `𓁹 Osiris Checkpoint — secure uncommitted work NOW (Rule A18)

Stages all changes and commits them as a checkpoint in the current (or given)
repository. Local commit only — nothing is pushed, and a checkpoint is fully
reversible (git reset). A clean tree is a successful no-op.`,
	RunE: runOsirisCheckpoint,
}

func runOsirisCheckpoint(cmd *cobra.Command, args []string) error {
	start := time.Now()
	dir := "."
	if len(args) > 0 {
		dir = args[0]
	}
	res := &output.CommandResult{Command: "sirsi osiris checkpoint", BriefTitle: "Checkpoint"}
	cp, err := osiris.CommitCheckpoint(dir)
	if err != nil {
		res.Summary = "Couldn't checkpoint here: " + err.Error()
		res.Status = "error"
		res.Errors = append(res.Errors, err.Error())
	} else if !cp.Committed {
		res.Summary = "Working tree clean — everything is already checkpointed."
		res.Status = "ok"
	} else {
		res.Summary = fmt.Sprintf("Checkpointed %d file(s) as commit %s. Local only — nothing was pushed; undo with git reset.", cp.FilesCommitted, cp.Hash)
		res.Status = "ok"
		res.AddEvidence("Commit", cp.Hash)
		res.AddEvidence("Files secured", fmt.Sprintf("%d", cp.FilesCommitted))
	}
	res.Duration = time.Since(start)
	res.Render()
	return nil
}

func init() {
	osirisCmd.AddCommand(osirisAssessCmd)
	osirisCmd.AddCommand(osirisStatusCmd)
	osirisCmd.AddCommand(osirisCheckpointCmd)
}

func runOsirisAssess(cmd *cobra.Command, args []string) error {
	start := time.Now()

	repoDir := "."
	if len(args) > 0 {
		repoDir = args[0]
	}

	cp, err := osiris.Assess(repoDir)
	if err != nil {
		// The common case run outside a project: give an actionable line, not a
		// raw `exit status 128`. Risk assessment reads git state, so it needs a repo.
		if strings.Contains(err.Error(), "not a git repository") {
			return fmt.Errorf("risk assessment needs a git repository — run `sirsi risk` inside a project, or pass a repo path: `sirsi risk <path>`")
		}
		return fmt.Errorf("osiris assess: %w", err)
	}

	if JsonOutput {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(cp)
	}

	output.Banner()
	output.Header("Checkpoint Assessment")

	// Main report
	fmt.Print(cp.FormatReport())
	fmt.Println()

	elapsed := time.Since(start)

	cr := &output.CommandResult{
		Command:  "sirsi risk",
		Summary:  fmt.Sprintf("Risk: %s %s — %d uncommitted files on %s", cp.StatusIcon(), cp.Risk, cp.TotalChanges, cp.Branch),
		Duration: elapsed,
	}
	cr.AddEvidence("Risk level", fmt.Sprintf("%s %s", cp.StatusIcon(), cp.Risk))
	cr.AddEvidence("Branch", cp.Branch)
	cr.AddEvidence("Uncommitted changes", fmt.Sprintf("%d files", cp.TotalChanges))
	if cp.LinesAdded > 0 || cp.LinesDeleted > 0 {
		cr.AddEvidence("Diff", fmt.Sprintf("+%d / -%d lines", cp.LinesAdded, cp.LinesDeleted))
	}
	if !cp.LastCommitTime.IsZero() {
		cr.AddEvidence("Last commit", cp.LastCommitHash)
	}
	if cp.ShouldWarn() {
		cr.AddWarning("%s", cp.Warning)
	}
	cr.AddNextAction("git commit", "Commit your changes to reduce risk")
	cr.AddNextAction("sirsi scan", "Scan for infrastructure waste")
	cr.AddNextAction("sirsi diagnose", "Check system health")
	cr.Render()
	return nil
}

func runOsirisStatus(cmd *cobra.Command, args []string) error {
	start := time.Now()

	repoDir := "."
	if len(args) > 0 {
		repoDir = args[0]
	}

	cp, err := osiris.Assess(repoDir)
	if err != nil {
		return fmt.Errorf("osiris status: %w", err)
	}

	if JsonOutput {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(map[string]string{
			"icon":    cp.StatusIcon(),
			"risk":    string(cp.Risk),
			"summary": cp.Summary(),
		})
	}

	output.Banner()
	output.Header("Recovery Status")

	dashboard := map[string]string{
		"Uncommitted": fmt.Sprintf("%d files", cp.TotalChanges),
		"Risk Level":  fmt.Sprintf("%s %s", cp.StatusIcon(), cp.Risk),
	}
	if !cp.LastCommitTime.IsZero() {
		dashboard["Last Commit"] = fmt.Sprintf("%s ago", cp.TimeSinceCommit.Round(time.Second))
	}
	output.Dashboard(dashboard)

	if cp.ShouldWarn() {
		output.Warn("%s", cp.Warning)
	}

	output.Footer(time.Since(start))
	actions := suggest.After(suggest.Context{Deity: "osiris", Subcommand: "status"})
	var steps [][]string
	for _, a := range actions {
		steps = append(steps, []string{a.Command, a.Description})
	}
	output.NextSteps(steps)
	return nil
}
