package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"runtime"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/SirsiMaster/sirsi-pantheon/internal/output"
	"github.com/SirsiMaster/sirsi-pantheon/internal/setup"
)

var setupCmd = &cobra.Command{
	Use:   "setup",
	Short: "Guided first-run wizard — dependencies, permissions, agent wake",
	Long: `Walk through everything Pantheon needs to run, in order:

  1. Dependencies   — check (and optionally install) the tools Pantheon uses
  2. Full Disk Access — grant macOS the access scans require, without prompts
  3. Agent wake     — register installed AI CLIs so the router can reach them

Run interactively in a terminal and the wizard prompts before each action.

  sirsi setup            Guided wizard (prompts before installing / granting)
  sirsi setup --install  Non-interactive: install missing tools, open the FDA pane
  sirsi setup --json     Machine-readable status, no prompts, no actions`,
	RunE: runSetup,
}

var (
	setupInstall bool
	setupJSON    bool
)

func init() {
	setupCmd.Flags().BoolVar(&setupInstall, "install", false, "Non-interactive: install missing dependencies and open the FDA pane")
	setupCmd.Flags().BoolVar(&setupJSON, "json", false, "Output status as JSON (no prompts, no actions)")
}

// setupInteractive reports whether stdin is a real terminal we can prompt on.
// A pipe, a file, or /dev/null must all return false so the wizard never opens
// System Settings or blocks on a prompt in an unattended context.
func setupInteractive() bool {
	return term.IsTerminal(int(os.Stdin.Fd()))
}

// promptYesNo asks a yes/no question, defaulting to def on an empty answer.
func promptYesNo(reader *bufio.Reader, question string, def bool) bool {
	suffix := " [Y/n] "
	if !def {
		suffix = " [y/N] "
	}
	fmt.Print("  " + question + suffix)
	line, _ := reader.ReadString('\n')
	switch strings.ToLower(strings.TrimSpace(line)) {
	case "":
		return def
	case "y", "yes":
		return true
	default:
		return false
	}
}

type depStatusJSON struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Installed   bool   `json:"installed"`
	Version     string `json:"version,omitempty"`
	Required    bool   `json:"required"`
	InstallCmd  string `json:"install_cmd"`
}

type setupJSONReport struct {
	Platform        string          `json:"platform"`
	Dependencies    []depStatusJSON `json:"dependencies"`
	MissingRequired int             `json:"missing_required"`
	MissingOptional int             `json:"missing_optional"`
	FullDiskAccess  bool            `json:"full_disk_access"`
	BinaryPath      string          `json:"binary_path"`
	AgentsAdded     []string        `json:"agents_added"`
	Ready           bool            `json:"ready"`
}

func runSetup(_ *cobra.Command, _ []string) error {
	if runtime.GOOS != "darwin" {
		return fmt.Errorf("setup currently supports macOS only (detected %s)", runtime.GOOS)
	}

	if setupJSON {
		return runSetupJSON()
	}

	interactive := setupInteractive() && !setupInstall
	reader := bufio.NewReader(os.Stdin)

	output.Banner()
	fmt.Println("Pantheon Setup")
	fmt.Println()

	// ── Step 1 / 3 — Dependencies ──────────────────────────────────────────
	fmt.Println("  Step 1 / 3 — Dependencies")
	fmt.Println()

	deps := setup.Dependencies()
	type depState struct {
		dep       setup.Dependency
		installed bool
	}
	var states []depState
	var rows [][]string
	missingRequired, missingOptional := 0, 0
	for _, d := range deps {
		ok, ver := d.Installed()
		states = append(states, depState{dep: d, installed: ok})
		icon, status := "✅", "installed"
		if !ok {
			if d.Required {
				icon, status = "❌", "MISSING (required)"
				missingRequired++
			} else {
				icon, status = "⚠️", "not installed"
				missingOptional++
			}
		}
		_ = ver
		rows = append(rows, []string{icon, d.Name, d.Description, status})
	}
	output.Table([]string{"", "Tool", "Purpose", "Status"}, rows)
	fmt.Println()

	missing := missingRequired + missingOptional
	if missing == 0 {
		output.Success("  All dependencies satisfied.")
	} else if interactive {
		for _, st := range states {
			if st.installed {
				continue
			}
			label := "optional"
			if st.dep.Required {
				label = "required"
			}
			q := fmt.Sprintf("Install %s (%s) — %s?", st.dep.Name, label, st.dep.InstallCmd)
			if promptYesNo(reader, q, st.dep.Required) {
				fmt.Printf("  Installing %s...\n", st.dep.Name)
				if err := st.dep.Install(os.Stdout, os.Stderr); err != nil {
					output.Warn("  %s install failed: %v", st.dep.Name, err)
					fmt.Printf("    Manual: %s\n", st.dep.InstallCmd)
				} else {
					output.Success("  %s installed.", st.dep.Name)
				}
			}
		}
	} else if setupInstall {
		for _, st := range states {
			if st.installed {
				continue
			}
			fmt.Printf("  Installing %s... ", st.dep.Name)
			if err := st.dep.Install(os.Stdout, os.Stderr); err != nil {
				fmt.Printf("❌ %v\n", err)
				fmt.Printf("    Manual: %s\n", st.dep.InstallCmd)
			} else {
				fmt.Println("✅")
			}
		}
	} else {
		fmt.Printf("  %d not installed. Run 'sirsi setup --install' to install automatically.\n", missing)
	}

	// ── Step 2 / 3 — Full Disk Access ──────────────────────────────────────
	fmt.Println()
	fmt.Println("  Step 2 / 3 — Full Disk Access")
	fmt.Println()

	fdaOK := setup.FullDiskAccessGranted()
	if fdaOK {
		output.Success("  Full Disk Access: granted.")
	} else {
		output.Warn("  Full Disk Access: not granted.")
		fmt.Println("  Without it, scans hit a permission prompt for every protected folder.")
		fmt.Println()
		fmt.Println("  Add this binary to Full Disk Access:")
		fmt.Printf("    %s\n", setup.BinaryPath())
		if term := os.Getenv("TERM_PROGRAM"); term != "" {
			fmt.Printf("  (and your terminal app: %s)\n", term)
		}
		fmt.Println()

		openPane := setupInstall
		if interactive {
			openPane = promptYesNo(reader, "Open System Settings → Full Disk Access now?", true)
		}
		if openPane {
			if err := setup.OpenFullDiskAccessPane(); err != nil {
				output.Warn("  Could not open System Settings: %v", err)
				fmt.Println("  Open manually: System Settings → Privacy & Security → Full Disk Access")
			} else {
				fmt.Println("  Opened System Settings. Drag the binary above into the list and toggle it on.")
			}
			if interactive {
				fmt.Print("  Press Enter once granted to re-check... ")
				_, _ = reader.ReadString('\n')
				if setup.FullDiskAccessGranted() {
					fdaOK = true
					output.Success("  Full Disk Access: granted.")
				} else {
					output.Warn("  Still not detected — a terminal restart is sometimes required.")
				}
			}
		} else if !interactive {
			fmt.Println("  Run 'sirsi setup --install' or 'sirsi permissions' to open the pane.")
		}
	}

	// ── Step 3 / 3 — Agent wake registration ───────────────────────────────
	fmt.Println()
	fmt.Println("  Step 3 / 3 — Agent wake registration")
	fmt.Println()

	if added, err := setup.RegisterAgentWake(); err != nil {
		output.Warn("  Skipped: %v", err)
	} else if len(added) == 0 {
		output.Success("  Ready: no new AI CLIs to register.")
	} else {
		output.Success("  Registered %d agent wake profile(s):", len(added))
		for _, id := range added {
			fmt.Printf("    • %s\n", id)
		}
	}

	// ── Summary ────────────────────────────────────────────────────────────
	fmt.Println()
	// Re-evaluate required deps after any installs above.
	missingRequired = 0
	for _, d := range setup.Dependencies() {
		if !d.Required {
			continue
		}
		if ok, _ := d.Installed(); !ok {
			missingRequired++
		}
	}
	if missingRequired == 0 && fdaOK {
		output.Success("  Pantheon is ready. Try:  sirsi scan")
		return nil
	}
	output.Warn("  Setup incomplete:")
	if missingRequired > 0 {
		fmt.Printf("    • %d required dependency(ies) still missing\n", missingRequired)
	}
	if !fdaOK {
		fmt.Println("    • Full Disk Access not yet granted")
	}
	fmt.Println("  Re-run 'sirsi setup' after addressing the above.")
	return nil
}

func runSetupJSON() error {
	deps := setup.Dependencies()
	rep := setupJSONReport{
		Platform:       runtime.GOOS,
		FullDiskAccess: setup.FullDiskAccessGranted(),
		BinaryPath:     setup.BinaryPath(),
	}
	for _, d := range deps {
		ok, ver := d.Installed()
		rep.Dependencies = append(rep.Dependencies, depStatusJSON{
			Name:        d.Name,
			Description: d.Description,
			Installed:   ok,
			Version:     ver,
			Required:    d.Required,
			InstallCmd:  d.InstallCmd,
		})
		if !ok {
			if d.Required {
				rep.MissingRequired++
			} else {
				rep.MissingOptional++
			}
		}
	}
	// Agent wake is read-only-ish detection; surface failures as empty.
	if added, err := setup.RegisterAgentWake(); err == nil {
		rep.AgentsAdded = added
	}
	rep.Ready = rep.MissingRequired == 0 && rep.FullDiskAccess

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(rep)
}
