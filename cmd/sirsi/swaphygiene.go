package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/SirsiMaster/sirsi-pantheon/internal/setup"
	"github.com/SirsiMaster/sirsi-pantheon/internal/swaphygiene"
)

const swapHygieneLabel = "ai.sirsi.swap-hygiene"

var (
	swapHygieneStatus  bool
	swapHygieneInstall bool
	swapHygieneJSON    bool
)

var swapHygieneCmd = &cobra.Command{
	Use:   "swap-hygiene",
	Short: "Measure host swap and memory pressure and record a receipt (never reclaims, kills or restarts)",
	Long: `Owner priority: recurring swap and memory hygiene. Samples vm.swapusage, the vm_stat
swap-in/out counters and memory_pressure, compares with the previous sample and records
a receipt under ~/.sirsi/swap-hygiene/.

A nonzero swap ALLOCATION is not paging: the verdict comes from swap-in/out movement
between samples. A restart is only ever PROPOSED (paging under memory pressure), never
triggered. Swap files are never touched.

  sirsi swap-hygiene              sample now and record
  sirsi swap-hygiene --status     show the last receipt
  sirsi swap-hygiene --install    run it every 30 minutes through launchd (no resident process)`,
	RunE: func(cmd *cobra.Command, args []string) error {
		home, err := os.UserHomeDir()
		if err != nil {
			return err
		}
		if swapHygieneInstall {
			return installSwapHygiene(home)
		}
		var r *swaphygiene.Receipt
		if swapHygieneStatus {
			if r, err = swaphygiene.Last(home); err != nil {
				return fmt.Errorf("no receipt yet — run `sirsi swap-hygiene` first: %w", err)
			}
		} else {
			rec, rerr := swaphygiene.Record(home, nil, time.Now())
			if rerr != nil {
				return rerr
			}
			r = &rec
		}
		if swapHygieneJSON {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(r)
		}
		fmt.Printf("swap %s — used %.1f of %.1f MiB, free memory %d%%, paging delta %d pages\n",
			r.Verdict, r.SwapUsedMiB, r.SwapTotalMiB, r.FreePct, r.DeltaSwapPages)
		fmt.Printf("  correctness-only diagnosis eligible: %v · release-timing qualification eligible: %v\n", r.CorrectnessOnlyOK, r.ReleaseTimingOK)
		if r.RestartProposed {
			fmt.Println("  ⚠ restart PROPOSED for the owner and active workload owners — nothing was restarted.")
		}
		if r.Note != "" {
			fmt.Println("  " + r.Note)
		}
		return nil
	},
}

func installSwapHygiene(home string) error {
	bin := setup.BinaryPath()
	if bin == "" || bin == "sirsi" {
		return fmt.Errorf("sirsi binary path could not be resolved")
	}
	plist := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key>
	<string>%s</string>
	<key>AssociatedBundleIdentifiers</key>
	<array>
		<string>ai.sirsi.pantheon</string>
	</array>
	<key>ProgramArguments</key>
	<array>
		<string>%s</string>
		<string>swap-hygiene</string>
	</array>
	<key>StartInterval</key>
	<integer>1800</integer>
	<key>RunAtLoad</key>
	<true/>
	<key>ProcessType</key>
	<string>Background</string>
	<key>StandardOutPath</key>
	<string>%s/.sirsi/swap-hygiene/launchd.log</string>
	<key>StandardErrorPath</key>
	<string>%s/.sirsi/swap-hygiene/launchd.log</string>
</dict>
</plist>
`, swapHygieneLabel, bin, home, home)
	if err := os.MkdirAll(swaphygiene.Dir(home), 0o755); err != nil {
		return err
	}
	path := filepath.Join(home, "Library", "LaunchAgents", swapHygieneLabel+".plist")
	if err := os.WriteFile(path, []byte(plist), 0o644); err != nil {
		return err
	}
	uid := fmt.Sprintf("gui/%d", os.Getuid())
	_ = exec.Command("launchctl", "bootout", uid+"/"+swapHygieneLabel).Run() // idempotent reinstall
	if out, err := exec.Command("launchctl", "bootstrap", uid, path).CombinedOutput(); err != nil {
		return fmt.Errorf("launchctl bootstrap: %w (%s)", err, out)
	}
	fmt.Printf("installed %s: samples every 30 minutes through launchd (no resident process). Receipts: %s\n", swapHygieneLabel, swaphygiene.Dir(home))
	return nil
}

func init() {
	swapHygieneCmd.Flags().BoolVar(&swapHygieneStatus, "status", false, "Show the last receipt without sampling")
	swapHygieneCmd.Flags().BoolVar(&swapHygieneInstall, "install", false, "Install the 30-minute launchd job")
	swapHygieneCmd.Flags().BoolVar(&swapHygieneJSON, "json", false, "Emit JSON")
	rootCmd.AddCommand(swapHygieneCmd)
}
