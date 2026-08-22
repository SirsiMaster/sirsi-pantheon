package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/SirsiMaster/sirsi-pantheon/internal/apprecovery"
)

var recoveryCmd = &cobra.Command{
	Use:   "recovery",
	Short: "Register and inspect Pantheon application recovery targets",
}

var recoveryListCmd = &cobra.Command{
	Use:   "list",
	Short: "List governed recovery capabilities without exposing private paths",
	RunE: func(cmd *cobra.Command, args []string) error {
		manager, err := apprecovery.LoadDefaultManager()
		if err != nil {
			return err
		}
		if manager == nil {
			fmt.Println("No governed recovery targets are registered.")
			return nil
		}
		encoder := json.NewEncoder(cmd.OutOrStdout())
		encoder.SetIndent("", "  ")
		return encoder.Encode(manager.Capabilities())
	},
}

var (
	recoveryID             string
	recoveryKind           string
	recoveryBundleID       string
	recoveryExecutable     string
	recoveryLaunchdTarget  string
	recoveryStatePaths     []string
	recoveryFreshPaths     []string
	recoveryReadinessURL   string
	recoveryReadyTimeout   time.Duration
	recoveryStartArguments []string
	recoveryAutoResume     bool
)

var recoveryAddCmd = &cobra.Command{
	Use:   "add",
	Short: "Atomically register one restore/fresh restart target",
	RunE: func(cmd *cobra.Command, args []string) error {
		target := apprecovery.Target{
			ID: recoveryID, Kind: apprecovery.Kind(recoveryKind), BundleID: recoveryBundleID,
			ExecutablePath: recoveryExecutable, LaunchdTarget: recoveryLaunchdTarget,
			StatePaths: recoveryStatePaths, FreshStatePaths: recoveryFreshPaths,
			ReadinessURL: recoveryReadinessURL, ReadyTimeout: recoveryReadyTimeout,
			StartArguments: recoveryStartArguments, AutoResume: recoveryAutoResume,
		}
		if err := apprecovery.RegisterDefaultTarget(target); err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Registered recovery target %s. Restart Pantheon to load the updated registry.\n", recoveryID)
		return nil
	},
}

var recoveryRemoveCmd = &cobra.Command{
	Use:   "remove TARGET_ID",
	Short: "Atomically remove one recovery target",
	Args: func(cmd *cobra.Command, args []string) error {
		if len(args) != 1 {
			return errors.New("exactly one target id is required")
		}
		return nil
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := apprecovery.RemoveDefaultTarget(args[0]); err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Removed recovery target %s. Restart Pantheon to unload it.\n", args[0])
		return nil
	},
}

func init() {
	recoveryAddCmd.Flags().StringVar(&recoveryID, "id", "", "stable recovery target id")
	recoveryAddCmd.Flags().StringVar(&recoveryKind, "kind", "", "app_saved_state, launchd_service, or checkpointed_process")
	recoveryAddCmd.Flags().StringVar(&recoveryBundleID, "bundle-id", "", "macOS application bundle id")
	recoveryAddCmd.Flags().StringVar(&recoveryExecutable, "executable", "", "absolute executable path")
	recoveryAddCmd.Flags().StringVar(&recoveryLaunchdTarget, "launchd-target", "", "exact launchd domain/label")
	recoveryAddCmd.Flags().StringSliceVar(&recoveryStatePaths, "state", nil, "absolute durable state file (repeatable)")
	recoveryAddCmd.Flags().StringSliceVar(&recoveryFreshPaths, "fresh-state", nil, "absolute transient file cleared by fresh restart (repeatable)")
	recoveryAddCmd.Flags().StringVar(&recoveryReadinessURL, "readiness-url", "", "optional loopback HTTP readiness URL")
	recoveryAddCmd.Flags().DurationVar(&recoveryReadyTimeout, "ready-timeout", 30*time.Second, "replacement readiness timeout")
	recoveryAddCmd.Flags().StringSliceVar(&recoveryStartArguments, "start-arg", nil, "checkpoint-aware process argument (repeatable)")
	recoveryAddCmd.Flags().BoolVar(&recoveryAutoResume, "auto-resume", false, "resume an already-started interrupted operation when Pantheon starts")
	_ = recoveryAddCmd.MarkFlagRequired("id")
	_ = recoveryAddCmd.MarkFlagRequired("kind")
	_ = recoveryAddCmd.MarkFlagRequired("executable")
	recoveryCmd.AddCommand(recoveryListCmd, recoveryAddCmd, recoveryRemoveCmd)
	rootCmd.AddCommand(recoveryCmd)
}
