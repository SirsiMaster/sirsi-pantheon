package main

import (
	"fmt"
	"os"

	"github.com/SirsiMaster/sirsi-pantheon/internal/caskrelease"
	"github.com/spf13/cobra"
)

var (
	caskVersion string
	caskSHA256  string
	caskFile    string
)

var caskReleaseCmd = &cobra.Command{
	Use:    "cask-release",
	Short:  "Render or verify canonical Homebrew Cask bytes",
	Hidden: true,
}

var caskReleaseRenderCmd = &cobra.Command{
	Use:   "render",
	Short: "Write the canonical cask for one uploaded DMG tuple",
	RunE: func(cmd *cobra.Command, args []string) error {
		bytes, err := caskrelease.Render(caskrelease.Input{Version: caskVersion, DMGSHA256: caskSHA256})
		if err != nil {
			return err
		}
		_, err = os.Stdout.Write(bytes)
		return err
	},
}

var caskReleaseVerifyCmd = &cobra.Command{
	Use:   "verify",
	Short: "Require cask bytes to match one uploaded DMG tuple exactly",
	RunE: func(cmd *cobra.Command, args []string) error {
		if caskFile == "" {
			return fmt.Errorf("--file is required")
		}
		bytes, err := os.ReadFile(caskFile)
		if err != nil {
			return fmt.Errorf("read cask file: %w", err)
		}
		return caskrelease.Verify(bytes, caskrelease.Input{Version: caskVersion, DMGSHA256: caskSHA256})
	},
}

func init() {
	for _, command := range []*cobra.Command{caskReleaseRenderCmd, caskReleaseVerifyCmd} {
		command.Flags().StringVar(&caskVersion, "version", "", "Release version without a v prefix")
		command.Flags().StringVar(&caskSHA256, "dmg-sha256", "", "Lowercase SHA-256 of the uploaded signed DMG")
	}
	caskReleaseVerifyCmd.Flags().StringVar(&caskFile, "file", "", "Cask file to verify")
	caskReleaseCmd.AddCommand(caskReleaseRenderCmd, caskReleaseVerifyCmd)
}
