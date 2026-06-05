package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"runtime"
	"strings"

	"github.com/spf13/cobra"

	"github.com/SirsiMaster/sirsi-pantheon/internal/output"
	"github.com/SirsiMaster/sirsi-pantheon/internal/setup"
)

var setupCmd = &cobra.Command{
	Use:   "setup",
	Short: "Guided first-run wizard — surfaces, dependencies, permissions",
	Long: `Set up everything Pantheon needs, in order:

  1. Surfaces     — choose which faces to install: CLI / TUI / IDE
                    (the menubar is installed by default on macOS)
  2. Dependencies — check (and optionally install) the tools Pantheon uses
  3. Permissions  — grant Full Disk Access once, so scans never hit a prompt
  4. Agent wake   — register installed AI CLIs so the router can reach them

Every surface is a face over the same engine — install several and switch
between them anytime with 'sirsi surface use <cli|tui|gui|ide>'.

Run interactively in a terminal and the wizard prompts before each action.

  sirsi setup                       Guided wizard (multi-select, prompts)
  sirsi setup --surfaces cli,ide    Non-interactive: install these surfaces
  sirsi setup --install             Non-interactive: install everything + open FDA pane
  sirsi setup --json                Machine-readable status, no prompts, no actions`,
	RunE: runSetup,
}

var (
	setupInstall  bool
	setupJSON     bool
	setupSurfaces string
)

func init() {
	setupCmd.Flags().BoolVar(&setupInstall, "install", false, "Non-interactive: install all surfaces + dependencies and open the FDA pane")
	setupCmd.Flags().BoolVar(&setupJSON, "json", false, "Output status as JSON (no prompts, no actions)")
	setupCmd.Flags().StringVar(&setupSurfaces, "surfaces", "", "Comma-separated surfaces to install (cli,tui,ide); skips the interactive picker")
}

// chooseSurfaces resolves which surfaces to install. Precedence:
//
//	--surfaces flag → that set; --install with no flag → all; interactive TTY →
//	prompt; otherwise → all selectable (safe non-interactive default).
func chooseSurfaces(reader *bufio.Reader, interactive bool) []setup.Surface {
	if setupSurfaces != "" {
		return parseSurfaces(setupSurfaces)
	}
	all := setup.Selectable()
	if !interactive {
		return all
	}

	fmt.Println("  Which surface(s) do you want? The menubar is always installed on macOS.")
	fmt.Println()
	for i, s := range all {
		fmt.Printf("    %d. %-9s %s\n", i+1, s.Title(), s.Detail())
	}
	fmt.Println()
	fmt.Print("  Enter numbers (e.g. 1,3), 'all', or press Enter for all: ")
	line, _ := reader.ReadString('\n')
	line = strings.ToLower(strings.TrimSpace(line))
	if line == "" || line == "all" {
		return all
	}

	var picked []setup.Surface
	seen := map[setup.Surface]bool{}
	for _, tok := range strings.FieldsFunc(line, func(r rune) bool { return r == ',' || r == ' ' }) {
		switch tok {
		case "cli", "1":
			addSurface(&picked, seen, setup.SurfaceCLI)
		case "tui", "2":
			addSurface(&picked, seen, setup.SurfaceTUI)
		case "ide", "3":
			addSurface(&picked, seen, setup.SurfaceIDE)
		}
	}
	if len(picked) == 0 {
		// Unparseable input: default to CLI so the user is never left with nothing.
		return []setup.Surface{setup.SurfaceCLI}
	}
	return picked
}

func parseSurfaces(csv string) []setup.Surface {
	var picked []setup.Surface
	seen := map[setup.Surface]bool{}
	for _, tok := range strings.Split(csv, ",") {
		switch strings.ToLower(strings.TrimSpace(tok)) {
		case "cli":
			addSurface(&picked, seen, setup.SurfaceCLI)
		case "tui":
			addSurface(&picked, seen, setup.SurfaceTUI)
		case "ide":
			addSurface(&picked, seen, setup.SurfaceIDE)
		}
	}
	if len(picked) == 0 {
		return []setup.Surface{setup.SurfaceCLI}
	}
	return picked
}

func addSurface(picked *[]setup.Surface, seen map[setup.Surface]bool, s setup.Surface) {
	if !seen[s] {
		seen[s] = true
		*picked = append(*picked, s)
	}
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

// printInstallResult renders one surface install outcome.
func printInstallResult(r setup.InstallResult) {
	switch r.Status {
	case setup.StatusOK:
		output.Success("  %s — %s", r.Surface.Title(), r.Message)
	case setup.StatusSkipped:
		fmt.Printf("  • %s — %s\n", r.Surface.Title(), r.Message)
	default:
		output.Warn("  %s — %s", r.Surface.Title(), r.Message)
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

	interactive := isTerminal(os.Stdin.Fd()) && !setupInstall
	reader := bufio.NewReader(os.Stdin)

	output.Banner()
	fmt.Println("Pantheon Setup")
	fmt.Println()

	// ── Step 1 / 4 — Surfaces ───────────────────────────────────────────────
	fmt.Println("  Step 1 / 4 — Surfaces")
	fmt.Println()

	surfaces := chooseSurfaces(reader, interactive)
	labels := make([]string, len(surfaces))
	for i, s := range surfaces {
		labels[i] = s.Title()
	}
	fmt.Printf("  Installing: %s", strings.Join(labels, ", "))
	if runtime.GOOS == "darwin" {
		fmt.Print(" + Menubar (default)")
	}
	fmt.Println()
	fmt.Println()

	for _, s := range surfaces {
		r := setup.Install(s)
		printInstallResult(r)
	}
	// Menubar is installed by default on macOS, regardless of selection.
	if runtime.GOOS == "darwin" {
		printInstallResult(setup.InstallMenubar())
	}

	// ── Step 2 / 4 — Dependencies ──────────────────────────────────────────
	fmt.Println()
	fmt.Println("  Step 2 / 4 — Dependencies")
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

	// ── Step 3 / 4 — Full Disk Access ──────────────────────────────────────
	fmt.Println()
	fmt.Println("  Step 3 / 4 — Full Disk Access")
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

	// ── Step 4 / 4 — Agent wake registration ───────────────────────────────
	fmt.Println()
	fmt.Println("  Step 4 / 4 — Agent wake registration")
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
