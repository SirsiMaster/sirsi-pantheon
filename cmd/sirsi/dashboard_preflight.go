package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/SirsiMaster/sirsi-pantheon/internal/dashboard"
	"github.com/SirsiMaster/sirsi-pantheon/internal/output"
	"github.com/SirsiMaster/sirsi-pantheon/internal/routerboard"
	buildversion "github.com/SirsiMaster/sirsi-pantheon/internal/version"
)

const dashboardIdentityBodyLimit = 16 << 10

var (
	dashboardPreflightPort            = dashboard.DashboardPort
	dashboardPreflightExpectedCommit  string
	dashboardPreflightExpectedVersion string
	dashboardPreflightExpectSelf      bool
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
	dashboardPreflightCmd.Flags().BoolVar(&dashboardPreflightExpectSelf, "expect-self", false, "Require the dashboard to be served by this exact sirsi build and executable")
	dashboardCmd.AddCommand(dashboardPreflightCmd)
}

func runDashboardPreflight(cmd *cobra.Command, args []string) error {
	if dashboardPreflightExpectSelf && (dashboardPreflightExpectedCommit != "" || dashboardPreflightExpectedVersion != "") {
		return fmt.Errorf("--expect-self cannot be combined with --expect-commit or --expect-version")
	}
	endpoint := fmt.Sprintf("http://127.0.0.1:%d/api/identity", dashboardPreflightPort)
	identity, err := fetchDashboardIdentity(&http.Client{Timeout: 2 * time.Second}, endpoint)
	if err != nil {
		return fmt.Errorf("dashboard preflight failed: %w", err)
	}
	if dashboardPreflightExpectSelf {
		if err := validateDashboardIdentityMatchesSelf(buildversion.Current("sirsi"), identity); err != nil {
			return fmt.Errorf("dashboard preflight self-identity mismatch: %w", err)
		}
	}
	if dashboardPreflightExpectedCommit != "" && identity.Commit != dashboardPreflightExpectedCommit {
		return fmt.Errorf("dashboard preflight commit mismatch: running %q, expected %q", identity.Commit, dashboardPreflightExpectedCommit)
	}
	if dashboardPreflightExpectedVersion != "" && identity.Version != dashboardPreflightExpectedVersion {
		return fmt.Errorf("dashboard preflight version mismatch: running %q, expected %q", identity.Version, dashboardPreflightExpectedVersion)
	}

	hasExpectedIdentity := dashboardPreflightExpectSelf || dashboardPreflightExpectedCommit != "" || dashboardPreflightExpectedVersion != ""
	if JsonOutput {
		return json.NewEncoder(os.Stdout).Encode(identity)
	}
	if hasExpectedIdentity {
		output.Success("Dashboard identity matched expected build")
	} else {
		output.Info("Dashboard identity observed; no expected commit or version was supplied")
	}
	output.Info("Schema: %s", dashboard.IdentitySchema)
	output.Info("Version: %s", identity.Version)
	output.Info("Commit: %s", identity.Commit)
	output.Info("Path: %s", identity.Path)
	return nil
}

func validateDashboardIdentityMatchesSelf(self buildversion.Info, running dashboard.IdentityResponse) error {
	got := running.Info
	if !samePantheonEnginePair(self.Binary, got.Binary) {
		return fmt.Errorf("running binary %q is not this CLI or its Pantheon app sibling", got.Binary)
	}
	for _, field := range []struct {
		name string
		want string
		got  string
	}{
		{name: "version", want: self.Version, got: got.Version},
		{name: "commit", want: self.Commit, got: got.Commit},
		{name: "date", want: self.Date, got: got.Date},
	} {
		if field.got != field.want {
			return fmt.Errorf("running %s %q differs from this CLI %q", field.name, field.got, field.want)
		}
	}
	if got.Dirty != self.Dirty {
		return fmt.Errorf("running dirty=%t differs from this CLI dirty=%t", got.Dirty, self.Dirty)
	}
	if strings.TrimSpace(self.Path) == "" || strings.TrimSpace(got.Path) == "" {
		return fmt.Errorf("CLI or dashboard executable path is missing")
	}
	if filepath.Base(self.Path) != self.Binary || filepath.Base(got.Path) != got.Binary {
		return fmt.Errorf("reported executable names do not match their identity: CLI=%q dashboard=%q", self.Path, got.Path)
	}
	if self.Binary == got.Binary {
		selfFile, err := os.Stat(self.Path)
		if err != nil {
			return fmt.Errorf("stat this CLI executable: %w", err)
		}
		runningFile, err := os.Stat(got.Path)
		if err != nil {
			return fmt.Errorf("stat dashboard executable: %w", err)
		}
		if os.SameFile(selfFile, runningFile) {
			return nil
		}
		return fmt.Errorf("dashboard executable %q is not this CLI executable %q", got.Path, self.Path)
	}
	if !isCanonicalPantheonAppExecutable(self.Path, self.Binary) || !isCanonicalPantheonAppExecutable(got.Path, got.Binary) {
		return fmt.Errorf("dashboard executable %q is not this CLI executable %q or its sibling in the same Pantheon app", got.Path, self.Path)
	}
	selfPath, err := filepath.EvalSymlinks(self.Path)
	if err != nil {
		return fmt.Errorf("resolve this CLI executable: %w", err)
	}
	runningPath, err := filepath.EvalSymlinks(got.Path)
	if err != nil {
		return fmt.Errorf("resolve dashboard executable: %w", err)
	}
	if filepath.Dir(selfPath) != filepath.Dir(runningPath) {
		return fmt.Errorf("dashboard executable %q is not in the same Pantheon app as this CLI %q", got.Path, self.Path)
	}
	return nil
}

func samePantheonEnginePair(left, right string) bool {
	return left == right && (left == "sirsi" || left == "sirsi-menubar") ||
		(left == "sirsi" && right == "sirsi-menubar") ||
		(left == "sirsi-menubar" && right == "sirsi")
}

func isCanonicalPantheonAppExecutable(path, binary string) bool {
	if filepath.Base(path) != binary || (binary != "sirsi" && binary != "sirsi-menubar") {
		return false
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil || filepath.Base(resolved) != binary {
		return false
	}
	info, err := os.Stat(resolved)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0o111 == 0 {
		return false
	}
	macOSDir := filepath.Dir(resolved)
	contentsDir := filepath.Dir(macOSDir)
	appDir := filepath.Dir(contentsDir)
	return filepath.Base(macOSDir) == "MacOS" && filepath.Base(contentsDir) == "Contents" && strings.HasSuffix(appDir, ".app")
}

func fetchDashboardIdentity(client *http.Client, endpoint string) (dashboard.IdentityResponse, error) {
	var identity dashboard.IdentityResponse
	if client == nil {
		return identity, fmt.Errorf("dashboard identity client is required")
	}
	readOnlyClient := *client
	readOnlyClient.CheckRedirect = func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}
	resp, err := readOnlyClient.Get(endpoint)
	if err != nil {
		return identity, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return identity, fmt.Errorf("identity endpoint returned HTTP %d; the listener may be an older dashboard", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, dashboardIdentityBodyLimit+1))
	if err != nil {
		return identity, fmt.Errorf("read dashboard identity response: %w", err)
	}
	if len(body) > dashboardIdentityBodyLimit {
		return identity, fmt.Errorf("dashboard identity response exceeds %d-byte limit", dashboardIdentityBodyLimit)
	}
	if err := routerboard.ValidateJSONNoDuplicateKeys(body); err != nil {
		return identity, fmt.Errorf("invalid dashboard identity JSON: %v", err)
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
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
