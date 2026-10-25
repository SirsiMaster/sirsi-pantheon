package main

// livenesswatchcmd.go — `sirsi liveness-watch`: the OS-level (launchd) liveness
// safety net (A32 durability item, routed by claude-home). `install` writes the
// LaunchAgent; its --local run retains bounded Ma'at metadata without routing
// or inference. The legacy no-flag runner remains available separately.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/SirsiMaster/sirsi-pantheon/internal/liveness"
	"github.com/SirsiMaster/sirsi-pantheon/internal/maat"
	"github.com/SirsiMaster/sirsi-pantheon/internal/output"
	"github.com/SirsiMaster/sirsi-pantheon/internal/router"
	"github.com/SirsiMaster/sirsi-pantheon/internal/setup"
)

var livenessWatchCmd = &cobra.Command{
	Use:   "liveness-watch",
	Short: "OS-level (launchd) reboot-proof watch: gemma broker, menubar, memory-death",
	Long: `Install a bounded per-user local metadata schedule with no developer
checkout, router enrollment, network or model-generation prerequisite. The
installed job uses run --local and records Ma'at's existing local Casebook.
Local recording is not Ra delivery or proof of inference readiness. The legacy
run invocation remains available for existing configured router consumers.`,
}

type localWatchDependencies struct {
	journal func() (maat.DecisionJournal, error)
	home    func() (string, error)
	engine  func() (string, error)
	host    func() (string, error)
	probes  func(string, string) []liveness.LocalProbe
}

func newLivenessRunCmd(deps localWatchDependencies) *cobra.Command {
	local := false
	command := &cobra.Command{
		Use:   "run",
		Short: "Run the liveness checks once (invoked by the LaunchAgent each tick)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if local {
				home, err := deps.home()
				if err != nil {
					return err
				}
				engine, err := deps.engine()
				if err != nil {
					return err
				}
				host, err := deps.host()
				if err != nil {
					return err
				}
				journal, err := deps.journal()
				if err != nil {
					return err
				}
				result, err := liveness.RunLocal(cmd.Context(), journal, host, deps.probes(engine, home))
				if writeErr := json.NewEncoder(cmd.OutOrStdout()).Encode(result); writeErr != nil {
					return writeErr
				}
				return err
			}
			root, err := workRoot()
			if err != nil {
				return err
			}
			return liveness.Run(root, os.Stdout)
		},
	}
	command.Flags().BoolVar(&local, "local", false, "Record bounded local metadata in Ma'at; no generation or Ra delivery")
	return command
}

var livenessRunCmd = newLivenessRunCmd(localWatchDependencies{
	journal: func() (maat.DecisionJournal, error) { return maat.NewDefaultDecisionJournal() },
	home:    os.UserHomeDir, engine: liveness.CurrentLocalEngine, host: os.Hostname,
	probes: func(engine, home string) []liveness.LocalProbe { return liveness.NativeLocalProbes(engine, home, nil) },
})

type livenessInstallDependencies struct {
	home    func() (string, error)
	engine  func() (string, error)
	verify  func(string) error
	install func(context.Context, string, string) (string, error)
}

func newLivenessInstallCmd(deps livenessInstallDependencies) *cobra.Command {
	return &cobra.Command{
		Use:   "install",
		Short: "Install the launchd LaunchAgent (runs now + every 15 min, at every login)",
		RunE: func(cmd *cobra.Command, args []string) error {
			msg, err := installLivenessWatchWith(deps)
			if err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), "  "+msg)
			return nil
		},
	}
}

var livenessInstallCmd = newLivenessInstallCmd(livenessInstallDependencies{
	home: os.UserHomeDir, engine: setup.CurrentPackagedMaatEngine,
	verify: setup.VerifyPackagedMaatEngine,
	install: func(ctx context.Context, engine, home string) (string, error) {
		return liveness.InstallLocal(ctx, engine, home, nil)
	},
})

func installLivenessWatchWith(deps livenessInstallDependencies) (string, error) {
	if deps.home == nil || deps.engine == nil || deps.verify == nil || deps.install == nil {
		return "", errors.New("canonical local liveness-watch installation dependencies are unavailable")
	}
	home, err := deps.home()
	if err != nil {
		return "", err
	}
	engine, err := deps.engine()
	if err != nil {
		return "", err
	}
	if err := deps.verify(engine); err != nil {
		return "", fmt.Errorf("verify canonical packaged Sirsi engine: %w", err)
	}
	return deps.install(context.Background(), engine, home)
}

var livenessUninstallCmd = &cobra.Command{
	Use:   "uninstall",
	Short: "Remove the launchd LaunchAgent",
	RunE: func(cmd *cobra.Command, args []string) error {
		msg, err := liveness.Uninstall()
		if err != nil {
			return err
		}
		fmt.Println("  " + msg)
		return nil
	},
}

var livenessStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Verify the exact local metadata LaunchAgent configuration and loaded state",
	RunE: func(cmd *cobra.Command, args []string) error {
		status, err := inspectLivenessWatchStatus(cmd.Context(), os.UserHomeDir, liveness.CurrentLocalEngine, liveness.InspectLocalWatch)
		fmt.Println("  " + status)
		return err
	},
}

func inspectLivenessWatchStatus(
	ctx context.Context,
	home func() (string, error),
	engine func() (string, error),
	inspect func(context.Context, string, string, liveness.LocalCommand) (liveness.LocalObservation, error),
) (string, error) {
	homePath, err := home()
	if err != nil {
		return "local watch status unavailable", fmt.Errorf("resolve local HOME: %w", err)
	}
	enginePath, err := engine()
	if err != nil {
		return "local watch status unavailable", fmt.Errorf("resolve canonical local engine: %w", err)
	}
	observation, err := inspect(ctx, enginePath, homePath, nil)
	if err != nil {
		return "local watch status unverified", fmt.Errorf("inspect local watch: %w", err)
	}
	if observation.Check != "local-watch" || observation.State != "observed" || observation.Concern || len(observation.ConfigurationSHA256) != 64 {
		if observation.Detail == "" {
			observation.Detail = "exact local schedule is not verified"
		}
		return "local watch not verified: " + observation.Detail, errors.New("local watch is not verified")
	}
	return "verified exact local schedule loaded: " + observation.Detail, nil
}

var livenessRestoreDisabledConfirm bool

// livenessRestoreDisabledCmd is the bounded operator repair for a disabled
// launchd override. It deliberately delegates to router's managed-plist
// recovery rather than accepting a caller-supplied label or launchctl command.
var livenessRestoreDisabledCmd = &cobra.Command{
	Use:   "restore-disabled",
	Short: "Re-enable and bootstrap eligible managed LaunchAgents",
	Long: `Clears disabled launchd overrides only for managed Sirsi/runner labels
that still have an exact LaunchAgent plist, then bootstraps only labels absent
from launchd. Quarantined services remain untouched. Requires --confirm.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		if !livenessRestoreDisabledConfirm {
			return fmt.Errorf("restore-disabled changes launchd state; rerun with --confirm after reviewing the managed labels")
		}
		recovery, err := router.RestoreDisabledManagedLaunchAgents()
		result := &output.CommandResult{
			Command:    "sirsi liveness-watch restore-disabled",
			BriefTitle: "LaunchAgent recovery",
			Status:     "ok",
			Summary:    "Managed LaunchAgent recovery completed. Re-run Diagnose to verify the disabled-override finding.",
			Evidence: []output.Evidence{
				{Label: "Overrides enabled", Value: joinedLabels(recovery.Enabled)},
				{Label: "Services bootstrapped", Value: joinedLabels(recovery.Bootstrapped)},
			},
			NextActions: []output.NextAction{{Label: "Verify health", Command: "sirsi diagnose --json", Description: "Confirm no managed labels remain disabled in the launchd override database."}},
		}
		if err != nil {
			result.Status = "error"
			result.Summary = "LaunchAgent recovery completed only in part; inspect the exact error before retrying."
			result.Errors = []string{err.Error()}
			result.Render()
			return err
		}
		result.Render()
		return nil
	},
}

func joinedLabels(labels []string) string {
	if len(labels) == 0 {
		return "none"
	}
	return strings.Join(labels, ", ")
}

func init() {
	livenessRestoreDisabledCmd.Flags().BoolVar(&livenessRestoreDisabledConfirm, "confirm", false, "confirm managed LaunchAgent recovery")
	livenessWatchCmd.AddCommand(livenessRunCmd, livenessInstallCmd, livenessUninstallCmd, livenessStatusCmd, livenessRestoreDisabledCmd)
	rootCmd.AddCommand(livenessWatchCmd)
}
