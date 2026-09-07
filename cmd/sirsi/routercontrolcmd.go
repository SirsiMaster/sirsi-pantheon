package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/SirsiMaster/sirsi-pantheon/internal/router"
	"github.com/SirsiMaster/sirsi-pantheon/internal/routerboard"
	"github.com/spf13/cobra"
)

var routerControlEndpoint string

var routerControlActionRequestFile string

var routerControlCmd = &cobra.Command{
	Use:   "control",
	Short: "Inspect the canonical worker control plane as one JSON envelope",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		if endpoint := firstNonEmptyControlEndpoint(routerControlEndpoint, os.Getenv("SIRSI_CONTROL_ENDPOINT")); endpoint != "" {
			body, err := fetchRemoteControl(cmd.Context(), endpoint, os.Getenv("SIRSI_CONTROL_TOKEN"))
			if err != nil {
				return err
			}
			return printControlJSON(body)
		}
		repoRoot, err := router.FindRepoRoot()
		if err != nil {
			return fmt.Errorf("locate repo root: %w", err)
		}
		bin, err := os.Executable()
		if err != nil || bin == "" {
			return fmt.Errorf("resolve canonical sirsi executable: %w", err)
		}
		agentsJSON := filepath.Join(repoRoot, ".agents", "idea-router", "agents.json")
		board := routerboard.New(bin, agentsJSON, routerboard.ControlSchema)
		board.Poll(context.Background())
		body, version, err := board.SnapshotControl()
		if err != nil {
			return fmt.Errorf("build control snapshot: %w", err)
		}
		if version == 0 || len(body) == 0 {
			return fmt.Errorf("control snapshot unavailable: canonical router poll did not complete")
		}
		return printControlJSON(body)
	},
}

func init() {
	routerControlCmd.Flags().StringVar(&routerControlEndpoint, "endpoint", "", "Authenticated M5 control endpoint (or SIRSI_CONTROL_ENDPOINT)")
	routerCmd.AddCommand(routerControlCmd)
	routerControlActionCmd.Flags().StringVar(&routerControlEndpoint, "endpoint", "", "Authenticated M5 control endpoint (or SIRSI_CONTROL_ENDPOINT)")
	routerControlActionCmd.Flags().StringVar(&routerControlActionRequestFile, "request-file", "-", "JSON action request file, or - for stdin")
	routerCmd.AddCommand(routerControlActionCmd)
}

var routerControlActionCmd = &cobra.Command{
	Use:   "control-action",
	Short: "Send one closed authenticated worker-control action to M5",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		endpoint := firstNonEmptyControlEndpoint(routerControlEndpoint, os.Getenv("SIRSI_CONTROL_ENDPOINT"))
		if endpoint == "" {
			return fmt.Errorf("control endpoint is required via --endpoint or SIRSI_CONTROL_ENDPOINT")
		}
		body, err := readControlActionRequest(routerControlActionRequestFile)
		if err != nil {
			return err
		}
		response, err := sendRemoteControlAction(cmd.Context(), endpoint, os.Getenv("SIRSI_CONTROL_TOKEN"), body)
		if err != nil {
			return err
		}
		return printControlJSON(response)
	},
}
