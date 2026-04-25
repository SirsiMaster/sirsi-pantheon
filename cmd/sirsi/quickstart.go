package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/jackal"
	"github.com/SirsiMaster/sirsi-pantheon/internal/jackal/rules"
	"github.com/SirsiMaster/sirsi-pantheon/internal/output"
	"github.com/spf13/cobra"
)

var quickstartCmd = &cobra.Command{
	Use:   "quickstart",
	Short: "Guided first scan — find waste and see what Pantheon can do",
	Long:  "Runs a scan, shows your top findings with plain-English recommendations, and tells you what to do next.",
	RunE:  runQuickstart,
}

func runQuickstart(cmd *cobra.Command, args []string) error {
	gold := output.TitleStyle
	dim := output.DimStyle
	green := output.SuccessStyle

	fmt.Println()
	fmt.Println(gold.Render("  Sirsi Pantheon — Quick Start"))
	fmt.Println(dim.Render("  ─────────────────────────────"))
	fmt.Println()
	fmt.Println(dim.Render("  Scanning your machine for infrastructure waste..."))
	fmt.Println()

	engine := jackal.DefaultEngine()
	engine.RegisterAll(rules.AllRules()...)
	start := time.Now()
	ctx := context.Background()
	res, err := engine.Scan(ctx, jackal.ScanOptions{})
	elapsed := time.Since(start)
	if err != nil {
		return fmt.Errorf("scan failed: %w", err)
	}
	jackal.EnrichAdvisory(res)
	_ = jackal.Persist(res, elapsed)

	if len(res.Findings) == 0 {
		fmt.Println(green.Render("  Your machine is clean. No waste found."))
		fmt.Println()
		fmt.Println(dim.Render("  Other things to try:"))
		fmt.Printf("    %s  %s\n", gold.Render("sirsi doctor"), dim.Render("System health diagnostic"))
		fmt.Printf("    %s  %s\n", gold.Render("sirsi ghosts"), dim.Render("Find remnants of uninstalled apps"))
		fmt.Printf("    %s  %s\n", gold.Render("sirsi network"), dim.Render("Network security audit"))
		fmt.Println()
		return nil
	}

	// Show summary
	fmt.Printf("  Found %s of reclaimable waste across %d items (%s)\n\n",
		gold.Render(jackal.FormatSize(res.TotalSize)),
		len(res.Findings),
		dim.Render(elapsed.Round(time.Millisecond).String()))

	// Show top findings (max 7)
	shown := 0
	for _, f := range res.Findings {
		if shown >= 7 {
			remaining := len(res.Findings) - shown
			if remaining > 0 {
				fmt.Printf("  %s\n", dim.Render(fmt.Sprintf("  ... and %d more items", remaining)))
			}
			break
		}

		severity := "safe"
		switch f.Severity {
		case jackal.SeverityCaution:
			severity = "caution"
		case jackal.SeverityWarning:
			severity = "warning"
		}

		fmt.Printf("    %-8s  %8s  %s\n",
			severity,
			jackal.FormatSize(f.SizeBytes),
			f.Description)

		if f.Advisory != "" {
			advisory := f.Advisory
			if len(advisory) > 70 {
				advisory = advisory[:67] + "..."
			}
			fmt.Printf("    %s\n", dim.Render("         "+advisory))
		}
		shown++
	}

	// Next steps
	fmt.Println()
	fmt.Println(dim.Render("  ─────────────────────────────"))
	fmt.Println()

	safeCount := 0
	for _, f := range res.Findings {
		if f.Severity == jackal.SeveritySafe && f.CanFix {
			safeCount++
		}
	}

	if safeCount > 0 {
		fmt.Printf("  %d items are safe to clean. Next steps:\n\n", safeCount)
		fmt.Printf("    %s  %s\n", gold.Render("sirsi judge --dry-run"), dim.Render("Preview what would be cleaned"))
		fmt.Printf("    %s  %s\n", gold.Render("sirsi judge --confirm"), dim.Render("Clean safely (moves to Trash)"))
	} else {
		fmt.Println("  All findings require review before cleaning.")
		fmt.Printf("\n    %s  %s\n", gold.Render("sirsi scan --json"), dim.Render("Get detailed findings as JSON"))
	}

	fmt.Println()
	fmt.Println(dim.Render("  More commands:"))
	fmt.Printf("    %s  %s\n", gold.Render("sirsi doctor"), dim.Render("System health diagnostic"))
	fmt.Printf("    %s  %s\n", gold.Render("sirsi ghosts"), dim.Render("Find remnants of uninstalled apps"))
	fmt.Printf("    %s  %s\n", gold.Render("sirsi network"), dim.Render("Network security audit"))
	fmt.Println()

	// Category breakdown
	if len(res.ByCategory) > 1 {
		fmt.Println(dim.Render("  Waste by category:"))
		for cat, summary := range res.ByCategory {
			bar := strings.Repeat("█", min(int(summary.TotalSize/(1<<28)), 30)) // 256MB per block
			if bar == "" {
				bar = "▏"
			}
			fmt.Printf("    %-14s %8s  %s\n",
				cat, jackal.FormatSize(summary.TotalSize), dim.Render(bar))
		}
		fmt.Println()
	}

	return nil
}
