package main

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/SirsiMaster/sirsi-pantheon/internal/osiris"
	"github.com/SirsiMaster/sirsi-pantheon/internal/output"
)

var osirisCmd = &cobra.Command{
	Use:   "osiris",
	Short: "𓁹 Osiris — Snapshot Keeper & Checkpoint Guardian",
	Long: `𓁹 Osiris — Snapshot Keeper & Checkpoint Guardian

Detects uncommitted work, measures session drift, and warns before data loss.
5-level risk assessment with time-based escalation.

  sirsi osiris assess        Show current checkpoint status
  sirsi osiris status        One-line risk summary`,
	Run: func(cmd *cobra.Command, args []string) {
		_ = cmd.Help()
	},
}

var osirisAssessCmd = &cobra.Command{
	Use:   "assess [path]",
	Short: "Full checkpoint assessment of uncommitted work",
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

  sirsi osiris assess             Assess current directory
  sirsi osiris assess /path/to    Assess a specific repo
  sirsi osiris assess --json      Output as JSON`,
	RunE: runOsirisAssess,
}

var osirisStatusCmd = &cobra.Command{
	Use:   "status [path]",
	Short: "One-line risk summary for menu bar or scripting",
	RunE:  runOsirisStatus,
}

func init() {
	osirisCmd.AddCommand(osirisAssessCmd)
	osirisCmd.AddCommand(osirisStatusCmd)
}

func runOsirisAssess(cmd *cobra.Command, args []string) error {
	start := time.Now()

	repoDir := "."
	if len(args) > 0 {
		repoDir = args[0]
	}

	cp, err := osiris.Assess(repoDir)
	if err != nil {
		return fmt.Errorf("osiris assess: %w", err)
	}

	if JsonOutput {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(cp)
	}

	output.Banner()
	output.Header("OSIRIS — Checkpoint Assessment")

	// Main report
	fmt.Print(cp.FormatReport())
	fmt.Println()

	// Dashboard summary
	dashboard := map[string]string{
		"Risk Level": fmt.Sprintf("%s %s", cp.StatusIcon(), cp.Risk),
		"Branch":     cp.Branch,
		"Changes":    fmt.Sprintf("%d files", cp.TotalChanges),
	}
	if cp.LinesAdded > 0 || cp.LinesDeleted > 0 {
		dashboard["Diff"] = fmt.Sprintf("+%d / -%d lines", cp.LinesAdded, cp.LinesDeleted)
	}
	if !cp.LastCommitTime.IsZero() {
		dashboard["Last Commit"] = cp.LastCommitHash
	}
	output.Dashboard(dashboard)

	if cp.ShouldWarn() {
		output.Warn("%s", cp.Warning)
	}

	output.Footer(time.Since(start))
	return nil
}

func runOsirisStatus(cmd *cobra.Command, args []string) error {
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

	fmt.Println(cp.Summary())
	return nil
}
