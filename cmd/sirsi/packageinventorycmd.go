package main

import (
	"encoding/json"
	"fmt"

	"github.com/SirsiMaster/sirsi-pantheon/internal/packageinventorycmd"
	"github.com/spf13/cobra"
)

var (
	packageInventoryApp                   string
	packageInventoryVersion               string
	packageInventoryBuild                 string
	packageInventoryInfoPlist             string
	packageInventoryPkgInfo               string
	packageInventoryLaunchAgent           string
	packageInventoryBrandLogo             string
	packageInventoryApolloCapability      string
	packageInventoryApolloProfile         string
	packageInventoryExpectedInfoSHA256    string
	packageInventoryExpectedProfileSHA256 string
	packageInventoryFrozenDigestOutput    string
	packageInventoryRequireCodeSignature  bool
	packageInventoryRequireFrozenDigests  bool
)

var packageInventoryCmd = &cobra.Command{
	Use:   "package-inventory",
	Short: "Verify a Pantheon.app payload without executing it",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		report, digests, err := packageinventorycmd.VerifyWithFrozenDigests(packageinventorycmd.Inputs{
			App:                   packageInventoryApp,
			Version:               packageInventoryVersion,
			Build:                 packageInventoryBuild,
			InfoPlist:             packageInventoryInfoPlist,
			PkgInfo:               packageInventoryPkgInfo,
			LaunchAgent:           packageInventoryLaunchAgent,
			BrandLogo:             packageInventoryBrandLogo,
			ApolloCapability:      packageInventoryApolloCapability,
			ApolloProfile:         packageInventoryApolloProfile,
			ExpectedInfoSHA256:    packageInventoryExpectedInfoSHA256,
			ExpectedProfileSHA256: packageInventoryExpectedProfileSHA256,
			RequireCodeSignature:  packageInventoryRequireCodeSignature,
			RequireFrozenDigests:  packageInventoryRequireFrozenDigests,
		})
		if err != nil {
			return err
		}
		if packageInventoryFrozenDigestOutput != "" {
			if err := packageinventorycmd.WriteFrozenDigests(packageInventoryFrozenDigestOutput, digests); err != nil {
				return err
			}
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
	packageInventoryCmd.Flags().StringVar(&packageInventoryBrandLogo, "brand-logo", "", "canonical Sirsi application-mark PNG path")
	packageInventoryCmd.Flags().StringVar(&packageInventoryApolloCapability, "apollo-capability", "", "required independent expected Apollo native capability: enabled, disabled, or legacy-development")
	packageInventoryCmd.Flags().StringVar(&packageInventoryApolloProfile, "apollo-profile", "", "exact protected Apollo native profile input")
	packageInventoryCmd.Flags().StringVar(&packageInventoryExpectedInfoSHA256, "expected-info-sha256", "", "frozen pre-sign Info.plist digest; profile expectation flags must be supplied with it")
	packageInventoryCmd.Flags().StringVar(&packageInventoryExpectedProfileSHA256, "expected-apollo-profile-sha256", "", "frozen pre-sign Apollo profile digest or absent")
	packageInventoryCmd.Flags().StringVar(&packageInventoryFrozenDigestOutput, "write-frozen-digests", "", "write exact validated Info.plist/profile digest pair to a new mode-0600 file")
	packageInventoryCmd.Flags().BoolVar(&packageInventoryRequireCodeSignature, "require-code-signature", false, "require the _CodeSignature payload")
	packageInventoryCmd.Flags().BoolVar(&packageInventoryRequireFrozenDigests, "require-frozen-digests", false, "require the pre-sign digest pair for release verification")
	rootCmd.AddCommand(packageInventoryCmd)
}
