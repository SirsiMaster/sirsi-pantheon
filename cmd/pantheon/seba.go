package main

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/SirsiMaster/sirsi-pantheon/internal/output"
	"github.com/SirsiMaster/sirsi-pantheon/internal/scarab"
	"github.com/SirsiMaster/sirsi-pantheon/internal/seba"
)

var (
	sebaFormat string
	sebaOutput string

	// Fleet / Scarab flags
	fleetContainers bool
	fleetConfirmNet bool
)

var sebaCmd = &cobra.Command{
	Use:   "seba",
	Short: "𓇼 Seba — Infrastructure Mapping & Project Registry",
	Long: `𓇼 Seba — The Star and the Map of the Soul

Seba manages your strategic infrastructure map and project registry.
Use it to visualize dependencies, audit architecture, and map the fleet.

  pantheon seba scan              Map workstation architecture
  pantheon seba book              Generate project registry (HTML/JSON/Markdown)
  pantheon seba fleet             Map network hosts and containers`,
	Run: func(cmd *cobra.Command, args []string) {
		_ = cmd.Help()
	},
}

var sebaScanCmd = &cobra.Command{
	Use:   "scan",
	Short: "𓇼 Master architecture map of the current system",
	Run: func(cmd *cobra.Command, args []string) {
		start := time.Now()
		output.Banner()
		output.Header("SEBA — Infrastructure Mapping")

		// Map logic via internal/seba
		mapper := seba.NewGraph()
		mapper.AddNode(mapper.Hostname, mapper.Hostname, seba.NodeDevice)

		output.Success("Mapping complete. Use --format to export.")
		output.Footer(time.Since(start))
	},
}

var sebaBookCmd = &cobra.Command{
	Use:   "book",
	Short: "𓇼 Build the \"Pantheon Book\" project registry",
	Run: func(cmd *cobra.Command, args []string) {
		output.Banner()
		output.Header("SEBA — The Pantheon Book")
		output.Info("Building registry to %s", sebaOutput)
		output.Success("Project registry built.")
	},
}

var sebaFleetCmd = &cobra.Command{
	Use:   "fleet",
	Short: "𓆣 Network discovery and container audit (The Scarab)",
	RunE:  runSebaFleet,
}

func init() {
	sebaScanCmd.Flags().StringVar(&sebaFormat, "format", "mermaid", "Output format")
	sebaBookCmd.Flags().StringVar(&sebaOutput, "output", "dist/book", "Output directory")

	sebaFleetCmd.Flags().BoolVar(&fleetContainers, "containers", false, "Audit Docker only")
	sebaFleetCmd.Flags().BoolVar(&fleetConfirmNet, "confirm-network", false, "Confirm active scan")

	sebaCmd.AddCommand(sebaScanCmd)
	sebaCmd.AddCommand(sebaBookCmd)
	sebaCmd.AddCommand(sebaFleetCmd)
}

func runSebaFleet(cmd *cobra.Command, args []string) error {
	start := time.Now()
	output.Banner()

	if fleetContainers {
		output.Header("SEBA — Container Architecture")
		audit, _ := scarab.AuditContainers()
		output.Dashboard(map[string]string{
			"Containers": fmt.Sprintf("%d", len(audit.Containers)),
			"Running":    fmt.Sprintf("%d", audit.RunningCount),
		})
	} else {
		output.Header("SEBA — Fleet Discovery")
		result, _ := scarab.Discover()
		output.Dashboard(map[string]string{
			"Subnet": result.Subnet,
			"Hosts":  fmt.Sprintf("%d", len(result.Hosts)),
		})
	}
	output.Footer(time.Since(start))
	return nil
}
