package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/SirsiMaster/sirsi-pantheon/internal/dashboard"
	"github.com/SirsiMaster/sirsi-pantheon/internal/output"
)

var (
	dashboardPreflightPort            = dashboard.DashboardPort
	dashboardPreflightExpectedCommit  string
	dashboardPreflightExpectedVersion string
)

var dashboardPreflightCmd = &cobra.Command{
	Use:   "preflight",
	Short: "Verify a running dashboard build without starting or mutating it",
	Args:  cobra.NoArgs,
	RunE:  runDashboardPreflight,
}

func init() {
	dashboardPreflightCmd.Flags().IntVar(&dashboardPreflightPort, "port", dashboard.DashboardPort, "Dashboard port to inspect")
	dashboardPreflightCmd.Flags().StringVar(&dashboardPreflightExpectedCommit, "expect-commit", "", "Require this running build commit")
	dashboardPreflightCmd.Flags().StringVar(&dashboardPreflightExpectedVersion, "expect-version", "", "Require this running build version")
	dashboardCmd.AddCommand(dashboardPreflightCmd)
}

func runDashboardPreflight(cmd *cobra.Command, args []string) error {
	endpoint := fmt.Sprintf("http://127.0.0.1:%d/api/identity", dashboardPreflightPort)
	identity, err := fetchDashboardIdentity(&http.Client{Timeout: 2 * time.Second}, endpoint)
	if err != nil {
		return fmt.Errorf("dashboard preflight failed: %w", err)
	}
	if dashboardPreflightExpectedCommit != "" && identity.Commit != dashboardPreflightExpectedCommit {
		return fmt.Errorf("dashboard preflight commit mismatch: running %q, expected %q", identity.Commit, dashboardPreflightExpectedCommit)
	}
	if dashboardPreflightExpectedVersion != "" && identity.Version != dashboardPreflightExpectedVersion {
		return fmt.Errorf("dashboard preflight version mismatch: running %q, expected %q", identity.Version, dashboardPreflightExpectedVersion)
	}

	if JsonOutput {
		return json.NewEncoder(os.Stdout).Encode(identity)
	}
	output.Success("Dashboard identity verified")
	output.Info("Schema: %s", dashboard.IdentitySchema)
	output.Info("Version: %s", identity.Version)
	output.Info("Commit: %s", identity.Commit)
	output.Info("Path: %s", identity.Path)
	return nil
}

func fetchDashboardIdentity(client *http.Client, endpoint string) (dashboard.IdentityResponse, error) {
	var identity dashboard.IdentityResponse
	resp, err := client.Get(endpoint)
	if err != nil {
		return identity, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return identity, fmt.Errorf("identity endpoint returned HTTP %d; the listener may be an older dashboard", resp.StatusCode)
	}
	decoder := json.NewDecoder(resp.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&identity); err != nil {
		return identity, fmt.Errorf("invalid identity response: %w", err)
	}
	if identity.Schema != dashboard.IdentitySchema {
		return identity, fmt.Errorf("identity schema mismatch: got %q, want %q", identity.Schema, dashboard.IdentitySchema)
	}
	if strings.TrimSpace(identity.Commit) == "" || strings.TrimSpace(identity.Version) == "" {
		return identity, fmt.Errorf("identity response omitted commit or version")
	}
	return identity, nil
}
