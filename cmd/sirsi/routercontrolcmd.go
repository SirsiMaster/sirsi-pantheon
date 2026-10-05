package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/SirsiMaster/sirsi-pantheon/internal/router"
	"github.com/SirsiMaster/sirsi-pantheon/internal/routerboard"
	"github.com/spf13/cobra"
)

var routerControlEndpoint string

var routerControlClientOnly bool

var routerControlLocalAuthority bool

var routerControlActionRequestFile string

var routerControlCmd = &cobra.Command{
	Use:   "control",
	Short: "Inspect the canonical worker control plane as one JSON envelope",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		endpoint := firstNonEmptyControlEndpoint(routerControlEndpoint, os.Getenv("SIRSI_CONTROL_ENDPOINT"))
		useLocal, err := useLocalControlAuthority(endpoint, routerControlClientOnly || controlClientOnlyEnv(os.Getenv("SIRSI_CONTROL_CLIENT_ONLY")), routerControlLocalAuthority)
		if err != nil {
			return err
		}
		if !useLocal {
			body, err := fetchRemoteControl(cmd.Context(), endpoint, os.Getenv("SIRSI_CONTROL_TOKEN"))
			if err != nil {
				return err
			}
			return printControlJSON(cmd.OutOrStdout(), body)
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
		return printControlJSON(cmd.OutOrStdout(), body)
	},
}

func init() {
	routerControlCmd.Flags().StringVar(&routerControlEndpoint, "endpoint", "", "Authenticated M5 control endpoint (or SIRSI_CONTROL_ENDPOINT)")
	routerControlCmd.Flags().BoolVar(&routerControlClientOnly, "client-only", false, "Require the authenticated M5 control plane; never use local router state")
	routerControlCmd.Flags().BoolVar(&routerControlLocalAuthority, "local-authority", false, "Explicitly inspect local canonical router state (use only on its authority host)")
	routerCmd.AddCommand(routerControlCmd)
	routerControlActionCmd.Flags().StringVar(&routerControlEndpoint, "endpoint", "", "Authenticated M5 control endpoint (or SIRSI_CONTROL_ENDPOINT)")
	routerControlActionCmd.Flags().StringVar(&routerControlActionRequestFile, "request-file", "-", "JSON action request file, or - for stdin")
	routerCmd.AddCommand(routerControlActionCmd)
}

func useLocalControlAuthority(endpoint string, clientOnly, localAuthority bool) (bool, error) {
	if endpoint != "" {
		if localAuthority {
			return false, fmt.Errorf("--local-authority cannot be combined with a remote control endpoint")
		}
		return false, nil
	}
	if clientOnly {
		return false, fmt.Errorf("M1 control client requires an authenticated M5 endpoint via --endpoint or SIRSI_CONTROL_ENDPOINT")
	}
	if !localAuthority {
		return false, fmt.Errorf("control authority is not configured; use --endpoint for M5 or --local-authority only on the canonical router host")
	}
	return true, nil
}

func controlClientOnlyEnv(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
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
		return sendAndPrintRemoteControlAction(cmd.Context(), endpoint, os.Getenv("SIRSI_CONTROL_TOKEN"), body, cmd.OutOrStdout())
	},
}

func sendAndPrintRemoteControlAction(ctx context.Context, endpoint, token string, requestBody []byte, output io.Writer) error {
	response, err := sendRemoteControlAction(ctx, endpoint, token, requestBody)
	if err == nil {
		return printControlJSON(output, response)
	}
	var rejection *routerboard.ControlActionFailureError
	if !errors.As(err, &rejection) || rejection == nil {
		return err
	}
	failureBody, marshalErr := json.Marshal(rejection.Failure)
	if marshalErr != nil {
		return fmt.Errorf("encode canonical worker action failure receipt: %w", marshalErr)
	}
	if printErr := printControlJSON(output, failureBody); printErr != nil {
		return fmt.Errorf("write canonical worker action failure receipt: %w", printErr)
	}
	return fmt.Errorf("canonical worker action rejected: %w", err)
}
