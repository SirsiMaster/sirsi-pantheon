package main

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/SirsiMaster/sirsi-anubis/internal/maat"
	"github.com/SirsiMaster/sirsi-anubis/internal/output"
)

var (
	maatPipeline bool
	maatCoverage bool
	maatCanon    bool
	maatCommits  int
)

var maatCmd = &cobra.Command{
	Use:   "maat",
	Short: "🪶 Ma'at — QA/QC governance assessment",
	Long: `🪶 Ma'at — The Feather of Truth

Assess development quality across pipeline, coverage, and canon linkage.
Ma'at weighs your project against the standard and reports verdicts.

  anubis maat              Full assessment (pipeline + coverage + canon)
  anubis maat --pipeline   CI pipeline status only
  anubis maat --coverage   Test coverage audit only
  anubis maat --canon      Commit canon linkage only

Ma'at uses the Feather weight system (0-100):
  100 = perfect (light as a feather)
  0   = critical failure (heavier than the heart)

See ADR-004 for the architecture decision.`,
	Run: runMaat,
}

func init() {
	maatCmd.Flags().BoolVar(&maatPipeline, "pipeline", false, "Assess CI pipeline only")
	maatCmd.Flags().BoolVar(&maatCoverage, "coverage", false, "Assess test coverage only")
	maatCmd.Flags().BoolVar(&maatCanon, "canon", false, "Assess canon linkage only")
	maatCmd.Flags().IntVar(&maatCommits, "commits", 10, "Number of recent commits to check for canon linkage")
}

func runMaat(cmd *cobra.Command, args []string) {
	start := time.Now()

	if !quietMode {
		output.Header("🪶 Ma'at — QA/QC Governance Assessment")
		fmt.Println()
	}

	// Determine which assessors to run.
	runAll := !maatPipeline && !maatCoverage && !maatCanon

	var assessors []maat.Assessor

	if runAll || maatPipeline {
		assessors = append(assessors, &maat.PipelineAssessor{
			RunCount: 5,
		})
	}

	if runAll || maatCoverage {
		assessors = append(assessors, &maat.CoverageAssessor{
			Thresholds: maat.DefaultThresholds(),
		})
	}

	if runAll || maatCanon {
		assessors = append(assessors, &maat.CanonAssessor{
			CommitCount: maatCommits,
		})
	}

	// Run assessments.
	report, err := maat.Weigh(assessors...)
	if err != nil {
		output.Error("Assessment failed: %v", err)
		os.Exit(1)
	}

	// JSON output.
	if jsonOutput {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(report)
		return
	}

	// Terminal output.
	currentDomain := maat.Domain("")
	for _, a := range report.Assessments {
		if a.Domain != currentDomain {
			currentDomain = a.Domain
			fmt.Println()
			output.Header(fmt.Sprintf("  %s — %s", domainIcon(a.Domain), a.Domain))
			fmt.Println()
		}
		fmt.Printf("  %s\n", a.Format())
	}

	fmt.Println()
	output.Header("  📊 Summary")
	fmt.Println()
	fmt.Printf("  Assessments: %d total — %d passed, %d warnings, %d failures\n",
		len(report.Assessments), report.Passes, report.Warnings, report.Failures)
	fmt.Printf("  Feather Weight: %d/100\n", report.OverallWeight)
	fmt.Printf("  Overall Verdict: %s %s\n",
		report.OverallVerdict.Icon(), report.OverallVerdict)

	elapsed := time.Since(start)
	fmt.Println()
	output.Dim("  Weighed in %s", elapsed.Round(time.Millisecond))

	if report.OverallVerdict == maat.VerdictFail {
		fmt.Println()
		output.Dim("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		output.Dim("🪶 The heart is heavier than the feather.")
		output.Dim("   Fix the failures above before shipping.")
		output.Dim("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		os.Exit(1)
	}
}

// domainIcon returns the icon for a quality domain.
func domainIcon(d maat.Domain) string {
	switch d {
	case maat.DomainPipeline:
		return "🔄"
	case maat.DomainCoverage:
		return "📐"
	case maat.DomainCanon:
		return "📜"
	default:
		return "🪶"
	}
}
