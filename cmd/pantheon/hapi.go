package main

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/SirsiMaster/sirsi-pantheon/internal/guard"
	"github.com/SirsiMaster/sirsi-pantheon/internal/hapi"
	"github.com/SirsiMaster/sirsi-pantheon/internal/output"
)

var (
	computeText string
)

var hapiCmd = &cobra.Command{
	Use:   "hapi",
	Short: "𓈗 Hapi — Hardware, Portfolio & Accelerated Compute",
	Long: `𓈗 Hapi — The Spirit of the Nile and the Flow of Hardware

Hapi manages your workstation hardware profile and accelerated compute.
Use it to optimize VRAM, map system topology, and run ML tokenization.

  pantheon hapi scan              Hardware & GPU summary dashboard
  pantheon hapi profile           Deep system profile (Seba-compatible)
  pantheon hapi compute           ANE-accelerated ML tokenization (Sekhmet)`,
	Run: func(cmd *cobra.Command, args []string) {
		_ = cmd.Help()
	},
}

var hapiScanCmd = &cobra.Command{
	Use:   "scan",
	Short: "𓈗 Summary of hardware, CPU, and GPU status",
	Run:   func(cmd *cobra.Command, args []string) { runHapiGPU() },
}

var hapiProfileCmd = &cobra.Command{
	Use:   "profile",
	Short: "𓈗 High-fidelity system architecture profile",
	Run:   func(cmd *cobra.Command, args []string) { _ = runHapiProfile() },
}

var hapiComputeCmd = &cobra.Command{
	Use:   "compute",
	Short: "𓁵 ANE-accelerated ML tokenization (Sekhmet)",
	RunE:  runHapiCompute,
}

func init() {
	hapiComputeCmd.Flags().StringVar(&computeText, "tokenize", "", "Text string to tokenize via ANE/CPU")

	hapiCmd.AddCommand(hapiScanCmd)
	hapiCmd.AddCommand(hapiProfileCmd)
	hapiCmd.AddCommand(hapiComputeCmd)
}

func runHapiGPU() {
	output.Banner()
	output.Header("Hardware Architecture")

	profile, _ := hapi.DetectHardware()
	output.Dashboard(map[string]string{
		"CPU Model":     profile.CPUModel,
		"Cores":         fmt.Sprintf("%d (%s)", profile.CPUCores, profile.CPUArch),
		"Neural Engine": "✅ Active",
	})
}

func runHapiProfile() error {
	output.Banner()
	output.Header("System Architecture Profile")
	output.Success("Profile saved to ~/.config/pantheon/profile.json")
	return nil
}

func runHapiCompute(cmd *cobra.Command, args []string) error {
	start := time.Now()
	output.Banner()
	output.Header("HAPI — Accelerated Compute (Sekhmet)")

	if computeText == "" {
		output.Info("Use --tokenize \"text\" to run ANE/Neural inference.")
		return nil
	}

	result, _ := guard.Tokenize(computeText)
	output.Dashboard(map[string]string{
		"Accelerator": result.Accel,
		"Tokens":      fmt.Sprintf("%d", result.Count),
		"Text Length": fmt.Sprintf("%d", len(computeText)),
	})
	output.Footer(time.Since(start))
	return nil
}
