package main

import (
	"encoding/json"
	"fmt"

	"github.com/SirsiMaster/sirsi-pantheon/internal/packageinventorycmd"
	"github.com/spf13/cobra"
)

var (
	packageInventoryApp                  string
	packageInventoryVersion              string
	packageInventoryBuild                string
	packageInventoryInfoPlist            string
	packageInventoryPkgInfo              string
	packageInventoryLaunchAgent          string
	packageInventoryRequireCodeSignature bool
)

var packageInventoryCmd = &cobra.Command{
	Use:   "package-inventory",
	Short: "Verify a Pantheon.app payload without executing it",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		report, err := packageinventorycmd.Verify(packageinventorycmd.Inputs{
			App:                  packageInventoryApp,
			Version:              packageInventoryVersion,
			Build:                packageInventoryBuild,
			InfoPlist:            packageInventoryInfoPlist,
			PkgInfo:              packageInventoryPkgInfo,
			LaunchAgent:          packageInventoryLaunchAgent,
			RequireCodeSignature: packageInventoryRequireCodeSignature,
		})
		if err != nil {
			return err
		}
		if JsonOutput {
			encoder := json.NewEncoder(cmd.OutOrStdout())
			encoder.SetIndent("", "  ")
			return encoder.Encode(report)
		}
		_, err = fmt.Fprintf(cmd.OutOrStdout(), "package inventory accepted=true entries=%d engines=%d executables=%d python_free=%t\n", len(report.Entries), report.EngineCount, report.ExecutableCount, report.PythonFree)
		return err
	},
}

func init() {
	packageInventoryCmd.Flags().StringVar(&packageInventoryApp, "app", "", "canonical absolute Pantheon.app path (no symlinked ancestors)")
	packageInventoryCmd.Flags().StringVar(&packageInventoryVersion, "version", "", "expected CFBundleShortVersionString")
	packageInventoryCmd.Flags().StringVar(&packageInventoryBuild, "build", "", "expected CFBundleVersion")
	packageInventoryCmd.Flags().StringVar(&packageInventoryInfoPlist, "info-plist", "", "canonical absolute Info.plist path (no symlinked ancestors)")
	packageInventoryCmd.Flags().StringVar(&packageInventoryPkgInfo, "pkg-info", "", "canonical absolute PkgInfo path (no symlinked ancestors)")
	packageInventoryCmd.Flags().StringVar(&packageInventoryLaunchAgent, "launch-agent", "", "canonical absolute LaunchAgent path (no symlinked ancestors)")
	packageInventoryCmd.Flags().BoolVar(&packageInventoryRequireCodeSignature, "require-code-signature", false, "require the _CodeSignature payload")
	rootCmd.AddCommand(packageInventoryCmd)
}
