package main

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/SirsiMaster/sirsi-pantheon/internal/updater"
)

var (
	updateInstallCLI      bool
	updateInstallApp      bool
	updateRollbackVersion string
	updateConfirmRollback bool
	newUpdateClient       = updater.NewClient
)

var updateCmd = &cobra.Command{
	Use:   "update",
	Short: "Check for and install the latest signed release",
	Long: `Check GitHub Releases for a newer Sirsi Pantheon and install it.

  sirsi update          Check only — report the newest release + any advisories.
  sirsi update --cli    Open the exact signed Pantheon package in macOS Installer (CLI compatibility alias).
  sirsi update --app    Open the exact signed Pantheon package in macOS Installer.
  sirsi update --rollback-version 0.24.136 --confirm-rollback
                         Restore one explicitly selected, older signed app release.

The package handoff is not an installed-success claim. Ma'at retains an exact
update intent before opening Installer and reads the installed app, helper,
signatures, and required paths back before closing that attempt. An interrupted
or ambiguous handoff is reconciled by readback only; Installer is never replayed
automatically.`,
	RunE: runUpdate,
}

func runUpdate(_ *cobra.Command, _ []string) error {
	if updateRollbackVersion != "" {
		if updateInstallCLI || updateInstallApp {
			return errors.New("select either normal app update or exact rollback version, not both")
		}
		if !updateConfirmRollback {
			return errors.New("exact rollback requires --confirm-rollback; no package was staged")
		}
	} else if updateConfirmRollback {
		return errors.New("--confirm-rollback requires an exact --rollback-version")
	}
	if updateInstallCLI || updateInstallApp || updateRollbackVersion != "" {
		if handled, err := reconcilePendingMaatAppUpdate(); handled || err != nil {
			return err
		}
	}
	// An explicit `sirsi update` tolerates a slower call than the 3s background
	// version-check, whose tight timeout trips on the full releases list. Use a
	// generous client for both the check and the release fetch.
	c := newUpdateClient()
	c.HTTPClient = &http.Client{Timeout: 20 * time.Second}
	if updateRollbackVersion != "" {
		rel, err := c.ReleaseForVersion(updateRollbackVersion)
		if err != nil {
			return fmt.Errorf("fetch exact selected rollback release: %w", err)
		}
		return installAppRollbackRelease(rel)
	}

	res := c.Check(version)
	if res.Error != nil {
		if errors.Is(res.Error, updater.ErrNoCompleteCommercialRelease) {
			fmt.Println("  𓁢 No complete commercial Pantheon update is published yet; your installed version remains active.")
			fmt.Println("     Recovery: recheck later. A usable update requires the matching signed Pantheon DMG and PKG pair.")
			return nil
		}
		return fmt.Errorf("update check failed: %w", res.Error)
	}
	if notice := updater.FormatAdvisories(res.Advisories); notice != "" {
		fmt.Print(notice)
	}

	// Check-only (default): report and exit.
	if !updateInstallCLI && !updateInstallApp {
		if res.UpdateAvailable {
			fmt.Print(updater.FormatUpdateNotice(res))
			fmt.Println("     Install:  sirsi update --app   (unified CLI + menubar app, notarized)")
		} else {
			fmt.Printf("  𓁢 sirsi %s is current (latest release: %s)\n", res.CurrentVersion, res.LatestVersion)
		}
		return nil
	}

	rel, err := c.NewestRelease()
	if err != nil {
		return fmt.Errorf("fetch release: %w", err)
	}
	if updateInstallCLI {
		fmt.Println("  ↳ Commercial Pantheon updates ship one app payload; --cli is using the unified app installer.")
	}
	if updateInstallCLI || updateInstallApp {
		return installAppRelease(rel)
	}
	return nil
}

// installAppRelease is the existing whole-app update owner. The platform
// adapter retains Ma'at preflight and an exact install intent before handing
// the verified commercial package to macOS Installer. A returned handoff is
// never treated as installed success; a later invocation must read the exact
// installed identity back before closing the retained attempt.
func installAppRelease(rel *updater.Release) error {
	if rel == nil {
		return fmt.Errorf("missing Pantheon release; installed product preserved")
	}
	// Check this final fetched record, not only the earlier availability read:
	// the release list can change between requests. Both --app and --cli use
	// this route, so neither can reopen a known-broken or older installer.
	if err := updater.ValidateUpdateVersion(version, rel.TagName); err != nil {
		return err
	}
	return installAppReleaseWithMaat(rel)
}

// installAppRollbackRelease accepts only the exact release selected by the
// operator. Installed-version direction and compatibility are rechecked from
// the observed app inside the Ma'at owner before package staging or handoff.
func installAppRollbackRelease(rel *updater.Release) error {
	if rel == nil {
		return errors.New("missing selected rollback release; installed product preserved")
	}
	if rel.TagName != "v"+strings.TrimPrefix(updateRollbackVersion, "v") {
		return errors.New("selected rollback release tag changed; installed product preserved")
	}
	return installAppRollbackWithMaat(rel)
}

func init() {
	updateCmd.Flags().BoolVar(&updateInstallCLI, "cli", false, "open the verified unified Pantheon package in macOS Installer (compatibility alias)")
	updateCmd.Flags().BoolVar(&updateInstallApp, "app", false, "open the verified Pantheon package in macOS Installer")
	updateCmd.Flags().StringVar(&updateRollbackVersion, "rollback-version", "", "exact older commercial Pantheon release to restore")
	updateCmd.Flags().BoolVar(&updateConfirmRollback, "confirm-rollback", false, "confirm the exact selected whole-app rollback")
	rootCmd.AddCommand(updateCmd)
}
