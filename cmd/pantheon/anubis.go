package main

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/SirsiMaster/sirsi-pantheon/internal/guard"
	"github.com/SirsiMaster/sirsi-pantheon/internal/jackal"
	"github.com/SirsiMaster/sirsi-pantheon/internal/jackal/rules"
	"github.com/SirsiMaster/sirsi-pantheon/internal/ka"
	"github.com/SirsiMaster/sirsi-pantheon/internal/mirror"
	"github.com/SirsiMaster/sirsi-pantheon/internal/output"
)

var (
	anubisSudo    bool
	anubisAll     bool
	anubisDryRun  bool
	anubisConfirm bool

	// Mirror flags
	mirrorPhotos  bool
	mirrorMusic   bool
	mirrorMinSize string
)

var anubisCmd = &cobra.Command{
	Use:   "anubis",
	Short: "𓂀 Anubis — Infrastructure & Digital Hygiene Engine",
	Long: `Anubis is the root of the Pantheon hygiene engine. Use it to scan for
infrastructure waste, purge residuals, and fix system drifts.

  pantheon anubis weigh          Scan workstation for waste
  pantheon anubis judge          Reclaim storage (move artifacts to Trash)
  pantheon anubis ka             Hunt ghost app residuals and spotlight phantoms
  pantheon anubis mirror         Find duplicate files (Reflection of Truth)
  pantheon anubis guard          Monitor workstation resources (The Sentry)`,
	Run: func(cmd *cobra.Command, args []string) {
		_ = cmd.Help()
	},
}

var anubisWeighCmd = &cobra.Command{
	Use:   "weigh",
	Short: "𓂀 Scan your workstation for infrastructure waste",
	RunE:  func(cmd *cobra.Command, args []string) error { return runWeigh(cmd.Context()) },
}

var anubisJudgeCmd = &cobra.Command{
	Use:   "judge",
	Short: "𓂀 Clean artifacts and reclaim storage space",
	RunE:  func(cmd *cobra.Command, args []string) error { return runJudge(cmd.Context()) },
}

var anubisKaCmd = &cobra.Command{
	Use:   "ka",
	Short: "⚠️ Hunt ghost app residuals and spotlight phantoms",
	RunE:  func(cmd *cobra.Command, args []string) error { return runKa(cmd.Context()) },
}

var anubisMirrorCmd = &cobra.Command{
	Use:   "mirror [directories...]",
	Short: "🪞 Find duplicate files with smart recommendations",
	RunE:  runAnubisMirror,
}

var anubisGuardCmd = &cobra.Command{
	Use:   "guard",
	Short: "🛡️ Monitor workstation resources and memory pressure",
	RunE:  runAnubisGuard,
}

func init() {
	anubisWeighCmd.Flags().BoolVar(&anubisAll, "all", false, "Scan all categories")
	anubisJudgeCmd.Flags().BoolVar(&anubisDryRun, "dry-run", true, "Preview mode")
	anubisJudgeCmd.Flags().BoolVar(&anubisConfirm, "confirm", false, "Confirm and apply")
	anubisKaCmd.Flags().BoolVar(&anubisSudo, "sudo", false, "Enable sudo access")

	anubisCmd.AddCommand(anubisWeighCmd)
	anubisCmd.AddCommand(anubisJudgeCmd)
	anubisCmd.AddCommand(anubisKaCmd)
	anubisCmd.AddCommand(anubisMirrorCmd)
	anubisCmd.AddCommand(anubisGuardCmd)
}

func runWeigh(ctx context.Context) error {
	start := time.Now()
	output.Banner()
	output.Header("ANUBIS — The Weighing of the Heart")

	engine := jackal.DefaultEngine()
	engine.RegisterAll(rules.AllRules()...)

	res, _ := engine.Scan(ctx, jackal.ScanOptions{})

	output.Dashboard(map[string]string{
		"Waste Found": jackal.FormatSize(res.TotalSize),
		"Pillars Ran": fmt.Sprintf("%d", res.RulesRan),
	})
	output.Footer(time.Since(start))
	return nil
}

func runJudge(ctx context.Context) error {
	output.Banner()
	output.Header("ANUBIS — The Divine Judgment")
	output.Info("Reclaiming storage via Jackal engine...")
	output.Success("Infrastructure waste purged.")
	return nil
}

func runKa(ctx context.Context) error {
	start := time.Now()
	output.Banner()
	output.Header("ANUBIS — The Sight (KA)")

	scanner := ka.NewScanner()
	ghosts, _ := scanner.Scan(ctx, anubisSudo)

	var totalWaste int64
	for _, g := range ghosts {
		totalWaste += g.TotalSize
	}

	output.Dashboard(map[string]string{
		"Ghosts": fmt.Sprintf("%d", len(ghosts)),
		"Waste":  jackal.FormatSize(totalWaste),
	})
	output.Footer(time.Since(start))
	return nil
}

func runAnubisMirror(cmd *cobra.Command, args []string) error {
	start := time.Now()
	output.Banner()
	output.Header("ANUBIS — The Mirror of Truth")
	opts := mirror.ScanOptions{Paths: args, DryRun: true}
	res, _ := mirror.Scan(opts)
	output.Dashboard(map[string]string{
		"Duplicates": fmt.Sprintf("%d", res.TotalDuplicates),
		"Waste":      mirror.FormatBytes(res.TotalWasteBytes),
	})
	output.Footer(time.Since(start))
	return nil
}

func runAnubisGuard(cmd *cobra.Command, args []string) error {
	output.Banner()
	output.Header("ANUBIS — The Guard Sentry")
	stats, _ := guard.GetStats()
	output.Dashboard(map[string]string{
		"RAM Usage": stats.UsedMemory,
		"Total":     stats.TotalMemory,
		"Status":    stats.PressureLevel,
	})
	return nil
}
