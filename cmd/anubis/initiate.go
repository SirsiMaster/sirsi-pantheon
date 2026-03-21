package main

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"

	"github.com/spf13/cobra"

	"github.com/SirsiMaster/sirsi-anubis/internal/output"
)

var initiateCmd = &cobra.Command{
	Use:   "initiate",
	Short: "🔑 Grant macOS permissions for deep scanning",
	Long: `🔑 Initiate — The Ritual of Access

Grant Anubis the permissions it needs for deep scanning on macOS.
This walks you through enabling Full Disk Access, which is required
for scanning protected directories like ~/Library and /var.

  anubis initiate          Run the permission wizard
  anubis initiate --check  Check current permission status

After granting access, Anubis can scan every corner of your workstation.`,
	Run: runInitiate,
}

var initiateCheck bool

func init() {
	initiateCmd.Flags().BoolVar(&initiateCheck, "check", false, "Check current permission status without changing anything")
}

func runInitiate(cmd *cobra.Command, args []string) {
	if runtime.GOOS != "darwin" {
		output.Info("🔑 Initiate is only needed on macOS")
		output.Info("   On Linux, run anubis with sudo for full access")
		return
	}

	output.Header("🔑 Initiate — The Ritual of Access")
	fmt.Println()

	// Check current permissions
	perms := checkPermissions()

	printPermStatus("Full Disk Access", perms.fullDisk)
	printPermStatus("Library Access", perms.library)
	printPermStatus("Developer Tools", perms.devtools)
	printPermStatus("Terminal Access", perms.terminal)

	if initiateCheck {
		fmt.Println()
		if perms.fullDisk && perms.library {
			output.Info("✅ All required permissions are granted")
		} else {
			output.Warn("⚠️  Some permissions are missing — scanning will be limited")
		}
		return
	}

	if perms.fullDisk && perms.library {
		fmt.Println()
		output.Info("✅ All required permissions are already granted!")
		output.Info("   Anubis has full access to scan your workstation.")
		return
	}

	fmt.Println()
	output.Warn("⚠️  Full Disk Access is required for deep scanning")
	fmt.Println()
	output.Info("  To grant Full Disk Access:")
	output.Info("  1. Open System Settings → Privacy & Security → Full Disk Access")
	output.Info("  2. Click the + button")
	output.Info("  3. Navigate to your terminal app (Terminal.app, iTerm2, etc.)")
	output.Info("  4. Add it and enable the toggle")
	fmt.Println()

	// Offer to open the settings pane
	output.Info("Opening System Settings → Privacy & Security...")
	openPrivacySettings()

	fmt.Println()
	output.Info("After granting access, run: anubis initiate --check")
}

type permissionStatus struct {
	fullDisk bool
	library  bool
	devtools bool
	terminal bool
}

func checkPermissions() permissionStatus {
	perms := permissionStatus{}

	// Test Full Disk Access by trying to read a protected path
	_, err := os.ReadDir("/Library/Application Support/com.apple.TCC")
	perms.fullDisk = err == nil

	// Test Library access
	home, _ := os.UserHomeDir()
	_, err = os.ReadDir(home + "/Library/Application Support")
	perms.library = err == nil

	// Test if Xcode CLI tools are installed
	_, err = exec.Command("xcode-select", "-p").Output()
	perms.devtools = err == nil

	// Check if running from a terminal
	perms.terminal = os.Getenv("TERM") != ""

	return perms
}

func printPermStatus(name string, granted bool) {
	if granted {
		fmt.Printf("    ✅ %-25s  Granted\n", name)
	} else {
		fmt.Printf("    ❌ %-25s  Not granted\n", name)
	}
}

func openPrivacySettings() {
	if runtime.GOOS == "darwin" {
		_ = exec.Command("open", "x-apple.systempreferences:com.apple.preference.security?Privacy_AllFiles").Start()
	}
}
