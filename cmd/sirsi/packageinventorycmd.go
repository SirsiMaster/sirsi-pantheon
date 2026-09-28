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
		_, err = fmt.Fprintf(cmd.OutOrStdout(), "package inventory accepted=true entries=%d engines=%d python_free=%t\n", len(report.Entries), report.EngineCount, report.PythonFree)
		return err
	},
}

func init() {
	packageInventoryCmd.Flags().StringVar(&packageInventoryApp, "app", "", "Pantheon.app path")
	packageInventoryCmd.Flags().StringVar(&packageInventoryVersion, "version", "", "expected CFBundleShortVersionString")
	packageInventoryCmd.Flags().StringVar(&packageInventoryBuild, "build", "", "expected CFBundleVersion")
	packageInventoryCmd.Flags().StringVar(&packageInventoryInfoPlist, "info-plist", "", "canonical Info.plist path")
	packageInventoryCmd.Flags().StringVar(&packageInventoryPkgInfo, "pkg-info", "", "canonical PkgInfo path")
	packageInventoryCmd.Flags().StringVar(&packageInventoryLaunchAgent, "launch-agent", "", "canonical LaunchAgent path")
	packageInventoryCmd.Flags().BoolVar(&packageInventoryRequireCodeSignature, "require-code-signature", false, "require the _CodeSignature payload")
	rootCmd.AddCommand(packageInventoryCmd)
}
