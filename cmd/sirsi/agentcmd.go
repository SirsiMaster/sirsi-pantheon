package main

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/agentguard"
	"github.com/SirsiMaster/sirsi-pantheon/internal/output"
	"github.com/SirsiMaster/sirsi-pantheon/internal/router"
	"github.com/spf13/cobra"
)

var agentCmd = &cobra.Command{
	Use:   "agent",
	Short: "Register and govern AI agents with the local router",
}

var (
	agentRegisterID        string
	agentRegisterCLI       string
	agentRegisterCWD       string
	agentRegisterURL       string
	agentRegisterAPIKey    string
	agentRegisterMechanism string
	agentRegisterMCPServer string
	agentCommand           []string
	agentForce             bool
	agentTimeout           time.Duration
	agentMaxOutputBytes    int
	agentMaxOutputLines    int
)

var agentRegisterCmd = &cobra.Command{
	Use:   "register <type>",
	Short: "Register an agent wake profile",
	Long: `Register a new Idea Router agent wake profile.

Examples:
  sirsi agent register gemini --api-key env:GEMINI_API_KEY
  sirsi agent register qwen --cli qwen-cli --cwd ~/Development/myproject
  sirsi agent register webhook --url https://example.test/pantheon-hook`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		repoRoot, err := router.FindRepoRoot()
		if err != nil {
			return fmt.Errorf("no idea-router found: %w", err)
		}
		// Root-ful invocation: persist the clone root for app-context callers (B2).
		router.RememberRepoRoot(repoRoot)
		routerRoot := filepath.Join(repoRoot, ".agents", "idea-router")
		agentType := args[0]
		cwd := agentRegisterCWD
		if cwd == "" {
			cwd = repoRoot
		}
		id := agentRegisterID
		if id == "" {
			id = agentType + "-pantheon"
		}

		cfg := router.AgentConfig{
			ID:         id,
			Type:       agentType,
			Cwd:        cwd,
			Workstream: "pantheon",
		}
		mechanism := agentRegisterMechanism
		if mechanism == "" {
			switch {
			case agentRegisterURL != "":
				mechanism = router.WakeAPICall
			case agentRegisterMCPServer != "":
				mechanism = router.WakeMCPNotification
			default:
				mechanism = router.WakeCLISpawn
			}
		}
		cfg.Wake.Mechanism = mechanism

		switch mechanism {
		case router.WakeCLISpawn:
			cli := agentRegisterCLI
			if cli == "" {
				cli = agentType
			}
			cfg.Command = []string{cli}
			cfg.Wake.HealthCheck = []string{cli, "--version"}
		case router.WakeAPICall:
			if agentRegisterURL == "" {
				return fmt.Errorf("--url is required for api-call registration")
			}
			cfg.Wake.Endpoint = agentRegisterURL
			cfg.Wake.Auth = agentRegisterAPIKey
		case router.WakeMCPNotification:
			server := agentRegisterMCPServer
			if server == "" {
				server = "sirsi"
			}
			cfg.Wake.MCPServer = server
		default:
			return fmt.Errorf("unsupported wake mechanism %q", mechanism)
		}

		if err := router.RegisterAgent(routerRoot, cfg); err != nil {
			return err
		}
		if !quietMode {
			fmt.Printf("Registered %s with %s wake", id, mechanism)
			if cfg.Wake.Endpoint != "" {
				fmt.Printf(" at %s", cfg.Wake.Endpoint)
			}
			if len(cfg.Command) > 0 {
				fmt.Printf(" via %s", strings.Join(cfg.Command, " "))
			}
			fmt.Println()
		}
		return nil
	},
}

var agentListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all registered agents and their wake mechanisms",
	RunE: func(cmd *cobra.Command, args []string) error {
		repoRoot, err := router.FindRepoRoot()
		if err != nil {
			return fmt.Errorf("no idea-router found: %w", err)
		}
		routerRoot := filepath.Join(repoRoot, ".agents", "idea-router")
		reg, err := router.LoadRegistry(routerRoot)
		if err != nil {
			return err
		}

		fmt.Printf("  Registered agents: %d\n\n", len(reg.Agents))
		for id, cfg := range reg.Agents {
			wake := cfg.WakeMechanism()
			if wake == "" {
				wake = "cli-spawn"
			}
			fmt.Printf("  %-28s type:%-8s wake:%-16s\n", id, cfg.Type, wake)
		}
		return nil
	},
}

var agentPreflightCmd = &cobra.Command{
	Use:          "preflight [command args...]",
	Short:        "Check whether an agent command is safe to run",
	SilenceUsage: true,
	Long: `Runs Pantheon's agent safety governor before risky work.

Examples:
  sirsi agent preflight
  sirsi agent preflight cat .codex/sessions/example.jsonl
  sirsi agent preflight -- rg --files internal/agentguard
  sirsi agent preflight --command rg -- .codex/sessions/example.jsonl`,
	RunE: func(cmd *cobra.Command, args []string) error {
		command := append([]string(nil), agentCommand...)
		command = append(command, args...)
		report := agentguard.Preflight(agentguard.PreflightOptions{Command: command})
		if JsonOutput {
			enc := json.NewEncoder(cmd.OutOrStdout())
			enc.SetIndent("", "  ")
			return enc.Encode(report)
		}
		renderAgentPreflight(report)
		if report.Verdict == agentguard.VerdictBlock {
			return fmt.Errorf("agent preflight blocked command")
		}
		return nil
	},
}

var agentSafeRunCmd = &cobra.Command{
	Use:          "safe-run -- <command> [args...]",
	Short:        "Run an agent command through Pantheon safety and output budgets",
	Args:         cobra.MinimumNArgs(1),
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		result, err := agentguard.SafeRun(context.Background(), agentguard.RunOptions{
			Command:        args,
			Timeout:        agentTimeout,
			MaxOutputBytes: agentMaxOutputBytes,
			MaxOutputLines: agentMaxOutputLines,
			Force:          agentForce,
		})
		if JsonOutput {
			enc := json.NewEncoder(cmd.OutOrStdout())
			enc.SetIndent("", "  ")
			_ = enc.Encode(result)
			return err
		}
		renderAgentPreflight(result.Report)
		if result.Output != "" {
			fmt.Fprint(cmd.OutOrStdout(), result.Output)
			if !strings.HasSuffix(result.Output, "\n") {
				fmt.Fprintln(cmd.OutOrStdout())
			}
		}
		if result.Truncated {
			output.Warn("Output was truncated by Pantheon budget controls.")
		}
		if err != nil {
			return fmt.Errorf("safe-run exit=%d: %w", result.ExitCode, err)
		}
		return nil
	},
}

func init() {
	agentRegisterCmd.Flags().StringVar(&agentRegisterID, "id", "", "Agent id (default: <type>-pantheon)")
	agentRegisterCmd.Flags().StringVar(&agentRegisterCLI, "cli", "", "CLI binary for cli-spawn wake")
	agentRegisterCmd.Flags().StringVar(&agentRegisterCWD, "cwd", "", "Working directory for cli-spawn wake")
	agentRegisterCmd.Flags().StringVar(&agentRegisterURL, "url", "", "HTTP endpoint for api-call wake")
	agentRegisterCmd.Flags().StringVar(&agentRegisterAPIKey, "api-key", "", "API auth token or env:VARIABLE reference")
	agentRegisterCmd.Flags().StringVar(&agentRegisterMechanism, "mechanism", "", "Wake mechanism: cli-spawn, api-call, mcp-notification")
	agentRegisterCmd.Flags().StringVar(&agentRegisterMCPServer, "mcp-server", "", "MCP server name for mcp-notification wake")
	agentPreflightCmd.Flags().StringSliceVar(&agentCommand, "command", nil, "Command and arguments to assess")
	agentSafeRunCmd.Flags().BoolVar(&agentForce, "force", false, "Run even when preflight blocks")
	agentSafeRunCmd.Flags().DurationVar(&agentTimeout, "timeout", 2*time.Minute, "Maximum command runtime")
	agentSafeRunCmd.Flags().IntVar(&agentMaxOutputBytes, "max-output-bytes", 512*1024, "Maximum captured output bytes")
	agentSafeRunCmd.Flags().IntVar(&agentMaxOutputLines, "max-output-lines", 400, "Maximum output lines after filtering")
	agentCmd.AddCommand(agentRegisterCmd, agentListCmd, agentPreflightCmd, agentSafeRunCmd)
}

func renderAgentPreflight(report *agentguard.Report) {
	if report == nil {
		return
	}
	output.Banner()
	output.Header("Agent Safety Preflight")
	fmt.Printf("Verdict: %s\n", strings.ToUpper(string(report.Verdict)))
	if len(report.Command) > 0 {
		fmt.Printf("Command: %s\n", strings.Join(report.Command, " "))
	}
	fmt.Println()
	for _, finding := range report.Findings {
		fmt.Printf("  [%s] %s: %s\n", strings.ToUpper(finding.Severity), finding.Check, finding.Message)
	}
}
