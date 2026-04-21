package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"

	"github.com/SirsiMaster/sirsi-pantheon/internal/help"
	"github.com/SirsiMaster/sirsi-pantheon/internal/output"
	"github.com/SirsiMaster/sirsi-pantheon/internal/platform"
	"github.com/SirsiMaster/sirsi-pantheon/internal/workstream"
)

var (
	wsDocs        bool
	wsResume      bool
	wsAutoApprove bool
	wsAI          string
	wsIDE         string
)

var workCmd = &cobra.Command{
	Use:     "work",
	Aliases: []string{"ws"},
	Short:   "Workstream manager — launch AI sessions across projects",
	Long: `Workstream Manager — shape your development context

Manages development workstreams across multiple AI assistants
and IDEs. Supports Claude, Codex, Gemini, Antigravity, VS Code,
Cursor, Windsurf, and Zed.

  sirsi work                   Interactive workstream picker
  sirsi work list              List all workstreams
  sirsi work add <name>        Add a new workstream
  sirsi work launch <n>        Launch workstream with AI/IDE
  sirsi work registry          Show installed AI tools & IDEs`,
	Args: cobra.ArbitraryArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		if wsDocs {
			output.Info("Opening workstream docs...")
			return help.OpenDocs("workstream")
		}
		// If a number is passed directly (e.g., `sirsi work 5` or `sw 5`), launch it
		if len(args) > 0 {
			num, err := strconv.Atoi(args[0])
			if err == nil {
				store, err := workstream.NewStore(workstream.DefaultConfigPath())
				if err != nil {
					return err
				}
				reader := bufio.NewReader(os.Stdin)
				return launchWorkstreamByNum(store, num, wsAutoApprove, reader)
			}
		}
		return runWorkstreamInteractive(false)
	},
}

// ── Interactive Picker ──────────────────────────────────────────────

func runWorkstreamInteractive(autoApprove bool) error {
	store, err := workstream.NewStore(workstream.DefaultConfigPath())
	if err != nil {
		return fmt.Errorf("load workstreams: %w", err)
	}

	active := store.Active()
	if len(active) == 0 {
		output.Banner()
		output.Header("WORKSTREAMS")
		output.Warn("No workstreams configured.")
		output.Dim("  Add one: sirsi work add <name> [directory]")
		return nil
	}

	gold := output.TitleStyle
	dim := output.DimStyle
	cyan := output.SizeStyle

	fmt.Println()
	fmt.Println(gold.Render("𓁟  Sirsi — Workstreams"))
	fmt.Println(dim.Render("─────────────────────────────────────────"))

	for i, ws := range active {
		fmt.Printf("  %s  %s %s\n", cyan.Render(fmt.Sprintf("%d", i+1)), ws.Name, dim.Render("("+ws.Dir+")"))
	}

	fmt.Println(dim.Render("─────────────────────────────────────────"))
	fmt.Printf("  %s  New conversation          ", output.SuccessStyle.Render("n"))
	fmt.Printf("%s  Add workstream\n", output.SuccessStyle.Render("a"))
	fmt.Printf("  %s  Resume any past session    ", output.SuccessStyle.Render("r"))
	fmt.Printf("%s  Quit\n", output.ErrorStyle.Render("q"))
	fmt.Println()
	fmt.Print(dim.Render("  Choice: "))

	reader := bufio.NewReader(os.Stdin)
	choice, _ := reader.ReadString('\n')
	choice = strings.TrimSpace(choice)

	switch {
	case choice == "q" || choice == "Q":
		return nil
	case choice == "n" || choice == "N":
		fmt.Print(gold.Render("  Name ") + dim.Render("[unnamed]") + ": ")
		name, _ := reader.ReadString('\n')
		name = strings.TrimSpace(name)
		launcher := workstream.DefaultAILauncher(platform.Current())
		if launcher == nil {
			output.Error("No AI assistant found. Run: sirsi work registry")
			return nil
		}
		ws := workstream.Workstream{Name: name, Dir: "."}
		return launcher.Launch(ws, workstream.LaunchOptions{SessionName: name})
	case choice == "a" || choice == "A":
		return runWorkstreamAddInteractive(reader, store)
	case choice == "r" || choice == "R":
		launcher := workstream.DefaultAILauncher(platform.Current())
		if launcher == nil {
			output.Error("No AI assistant found.")
			return nil
		}
		ws := workstream.Workstream{Name: "", Dir: "."}
		return launcher.Launch(ws, workstream.LaunchOptions{Resume: true})
	default:
		num, err := strconv.Atoi(choice)
		if err != nil {
			output.Error("Invalid choice: %s", choice)
			return nil
		}
		return launchWorkstreamByNum(store, num, autoApprove, reader)
	}
}

func runWorkstreamAddInteractive(reader *bufio.Reader, store *workstream.Store) error {
	gold := output.TitleStyle
	dim := output.DimStyle
	p := platform.Current()

	fmt.Print(gold.Render("  Workstream name: "))
	name, _ := reader.ReadString('\n')
	name = strings.TrimSpace(name)
	if name == "" {
		fmt.Println("Canceled.")
		return nil
	}

	cwd, _ := os.Getwd()
	fmt.Print(gold.Render("  Working directory ") + dim.Render("["+cwd+"]") + ": ")
	dir, _ := reader.ReadString('\n')
	dir = strings.TrimSpace(dir)
	if dir == "" {
		dir = cwd
	}
	dir = workstream.CompressDir(dir)

	// AI tool picker
	fmt.Println()
	fmt.Println(dim.Render("  AI Assistants:"))
	aiLaunchers := installedByKind("ai", p)
	for i, l := range aiLaunchers {
		def := ""
		if i == 0 {
			def = " (default)"
		}
		fmt.Printf("    %s  %s%s\n", gold.Render(fmt.Sprintf("%d", i+1)), l.Name(), dim.Render(def))
	}
	if len(aiLaunchers) == 0 {
		fmt.Println(dim.Render("    (none installed)"))
	}
	fmt.Print(gold.Render("  AI ") + dim.Render("[1]") + ": ")
	aiChoice, _ := reader.ReadString('\n')
	aiChoice = strings.TrimSpace(aiChoice)
	aiID := ""
	if len(aiLaunchers) > 0 {
		aiIdx := 0
		if aiChoice != "" {
			if n, err := strconv.Atoi(aiChoice); err == nil && n >= 1 && n <= len(aiLaunchers) {
				aiIdx = n - 1
			}
		}
		aiID = aiLaunchers[aiIdx].ID()
	}

	// IDE picker
	fmt.Println()
	fmt.Println(dim.Render("  IDEs:"))
	ideLaunchers := installedByKind("ide", p)
	for i, l := range ideLaunchers {
		fmt.Printf("    %s  %s\n", gold.Render(fmt.Sprintf("%d", i+1)), l.Name())
	}
	if len(ideLaunchers) == 0 {
		fmt.Println(dim.Render("    (none installed)"))
	}
	fmt.Print(gold.Render("  IDE ") + dim.Render("[skip]") + ": ")
	ideChoice, _ := reader.ReadString('\n')
	ideChoice = strings.TrimSpace(ideChoice)
	ideID := ""
	if ideChoice != "" && len(ideLaunchers) > 0 {
		if n, err := strconv.Atoi(ideChoice); err == nil && n >= 1 && n <= len(ideLaunchers) {
			ideID = ideLaunchers[n-1].ID()
		}
	}

	if err := store.Add(name, dir); err != nil {
		return err
	}

	// Set AI/IDE on the newly added workstream
	for i, ws := range store.All() {
		if ws.Name == name {
			if aiID != "" {
				_ = store.SetAI(i, aiID)
			}
			if ideID != "" {
				_ = store.SetIDE(i, ideID)
			}
			break
		}
	}

	output.Success("Added workstream: %s → %s", name, dir)
	if aiID != "" {
		output.Dim("  AI: %s", aiID)
	}
	if ideID != "" {
		output.Dim("  IDE: %s", ideID)
	}
	return nil
}

func pickAITool(reader *bufio.Reader, launchers []workstream.Launcher, gold, dim lipgloss.Style) workstream.Launcher {
	fmt.Println()
	for i, l := range launchers {
		fmt.Printf("    %s  %s\n", gold.Render(fmt.Sprintf("%d", i+1)), l.Name())
	}
	fmt.Print(gold.Render("  AI: "))
	choice, _ := reader.ReadString('\n')
	choice = strings.TrimSpace(choice)
	if n, err := strconv.Atoi(choice); err == nil && n >= 1 && n <= len(launchers) {
		return launchers[n-1]
	}
	return nil
}

func pickIDETool(reader *bufio.Reader, launchers []workstream.Launcher, gold, dim lipgloss.Style) workstream.Launcher {
	if len(launchers) == 1 {
		return launchers[0]
	}
	fmt.Println()
	for i, l := range launchers {
		fmt.Printf("    %s  %s\n", gold.Render(fmt.Sprintf("%d", i+1)), l.Name())
	}
	fmt.Print(gold.Render("  IDE: "))
	choice, _ := reader.ReadString('\n')
	choice = strings.TrimSpace(choice)
	if n, err := strconv.Atoi(choice); err == nil && n >= 1 && n <= len(launchers) {
		return launchers[n-1]
	}
	return nil
}

// installedByKind returns all installed launchers of the given kind ("ai" or "ide").
func installedByKind(kind string, p platform.Platform) []workstream.Launcher {
	var out []workstream.Launcher
	for _, l := range workstream.AllLaunchers() {
		if l.Kind() == kind && l.Installed(p) {
			out = append(out, l)
		}
	}
	return out
}

func launchWorkstreamByNum(store *workstream.Store, num int, autoApprove bool, reader *bufio.Reader) error {
	ws, idx, err := store.GetActive(num)
	if err != nil {
		output.Error("%v", err)
		return nil
	}

	gold := output.TitleStyle
	dim := output.DimStyle
	accent := output.WarningStyle
	cyan := output.SizeStyle

	// Determine which AI tool to use
	aiID := ws.AI
	if wsAI != "" {
		aiID = wsAI
	}
	if aiID == "" {
		aiID = "claude" // default
	}
	launcher := workstream.FindLauncher(aiID)
	if launcher == nil {
		output.Error("Unknown AI tool: %s", aiID)
		return nil
	}

	// Quick launch if auto-approve
	if autoApprove {
		fmt.Println()
		fmt.Printf("%s  %s\n", gold.Render("→ "+ws.Name), accent.Render("⚡ auto-approve"))
		_ = store.TouchLastUsed(idx)
		return launcher.Launch(ws, workstream.LaunchOptions{AutoApprove: true})
	}

	// Interactive sub-menu
	fmt.Println()
	fmt.Println(gold.Render("→ " + ws.Name))
	fmt.Printf("  %s %s\n", dim.Render("ai:"), launcher.Name())
	if ws.IDE != "" {
		if ideLauncher := workstream.FindLauncher(ws.IDE); ideLauncher != nil {
			fmt.Printf("  %s %s\n", dim.Render("ide:"), ideLauncher.Name())
		}
	}
	fmt.Println()
	fmt.Printf("  %s  Resume last session\n", cyan.Render("r"))
	fmt.Printf("  %s  Start fresh\n", output.SuccessStyle.Render("f"))
	fmt.Printf("  %s  Resume (auto-approve)\n", accent.Render("R"))
	fmt.Printf("  %s  Fresh  (auto-approve)\n", accent.Render("F"))

	// Show installed AI alternatives
	p := platform.Current()
	altAI := installedByKind("ai", p)
	if len(altAI) > 1 {
		fmt.Printf("  %s  Switch AI tool\n", gold.Render("a"))
	}
	// Show IDE launch option
	altIDE := installedByKind("ide", p)
	if len(altIDE) > 0 {
		fmt.Printf("  %s  Open in IDE\n", gold.Render("i"))
	}

	fmt.Println()
	fmt.Print(dim.Render("  Choice: "))

	action, _ := reader.ReadString('\n')
	action = strings.TrimSpace(action)

	opts := workstream.LaunchOptions{}
	switch action {
	case "r":
		opts.Resume = true
	case "R":
		opts.Resume = true
		opts.AutoApprove = true
	case "f", "":
		// fresh, no special opts
	case "F":
		opts.AutoApprove = true
	case "a":
		// Switch AI tool then launch fresh
		launcher = pickAITool(reader, altAI, gold, dim)
		if launcher == nil {
			fmt.Println("Canceled.")
			return nil
		}
	case "i":
		// Open in IDE
		ideLauncher := pickIDETool(reader, altIDE, gold, dim)
		if ideLauncher == nil {
			fmt.Println("Canceled.")
			return nil
		}
		_ = store.TouchLastUsed(idx)
		return ideLauncher.Launch(ws, workstream.LaunchOptions{})
	default:
		fmt.Println("Canceled.")
		return nil
	}

	_ = store.TouchLastUsed(idx)
	return launcher.Launch(ws, opts)
}

// ── List Command ────────────────────────────────────────────────────

var wsListCmd = &cobra.Command{
	Use:     "list",
	Short:   "List all workstreams",
	Aliases: []string{"ls"},
	RunE: func(cmd *cobra.Command, args []string) error {
		start := time.Now()
		store, err := workstream.NewStore(workstream.DefaultConfigPath())
		if err != nil {
			return err
		}

		all := store.All()
		if JsonOutput {
			data, _ := json.MarshalIndent(all, "", "  ")
			fmt.Println(string(data))
			return nil
		}

		output.Banner()
		output.Header("WORKSTREAMS")

		if len(all) == 0 {
			output.Dim("  No workstreams configured.")
			output.Dim("  Add one: sirsi work add <name> [directory]")
			return nil
		}

		headers := []string{"#", "Name", "Directory", "AI", "Status"}
		var rows [][]string
		activeNum := 0
		for _, ws := range all {
			num := ""
			if ws.Status == workstream.StatusActive {
				activeNum++
				num = fmt.Sprintf("%d", activeNum)
			}
			ai := ws.AI
			if ai == "" {
				ai = "claude"
			}
			rows = append(rows, []string{num, ws.Name, ws.Dir, ai, string(ws.Status)})
		}
		output.Table(headers, rows)

		output.Dashboard(map[string]string{
			"Active": fmt.Sprintf("%d", activeNum),
			"Total":  fmt.Sprintf("%d", len(all)),
		})
		output.Footer(time.Since(start))
		return nil
	},
}

// ── Add Command ─────────────────────────────────────────────────────

var wsAddCmd = &cobra.Command{
	Use:   "add <name> [directory]",
	Short: "Add a new workstream",
	Args:  cobra.RangeArgs(1, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		store, err := workstream.NewStore(workstream.DefaultConfigPath())
		if err != nil {
			return err
		}
		name := args[0]
		var dir string
		if len(args) > 1 {
			dir = args[1]
		} else {
			dir, _ = os.Getwd()
		}
		dir = workstream.CompressDir(dir)

		if err := store.Add(name, dir); err != nil {
			return err
		}

		// Set AI/IDE if flags provided
		for i, ws := range store.All() {
			if ws.Name == name {
				if wsAI != "" {
					_ = store.SetAI(i, wsAI)
				}
				if wsIDE != "" {
					_ = store.SetIDE(i, wsIDE)
				}
				break
			}
		}

		output.Success("Added workstream: %s → %s", name, dir)
		return nil
	},
}

// ── Rename Command ──────────────────────────────────────────────────

var wsRenameCmd = &cobra.Command{
	Use:   "rename <number> <new-name>",
	Short: "Rename a workstream",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		store, err := workstream.NewStore(workstream.DefaultConfigPath())
		if err != nil {
			return err
		}
		num, err := strconv.Atoi(args[0])
		if err != nil {
			return fmt.Errorf("invalid number: %s", args[0])
		}
		_, idx, err := store.GetActive(num)
		if err != nil {
			return err
		}
		oldName := store.All()[idx].Name
		if err := store.Rename(idx, args[1]); err != nil {
			return err
		}
		output.Success("Renamed: %s → %s", oldName, args[1])
		return nil
	},
}

// ── Retire Command ──────────────────────────────────────────────────

var wsRetireCmd = &cobra.Command{
	Use:   "retire <number>",
	Short: "Archive a workstream",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		store, err := workstream.NewStore(workstream.DefaultConfigPath())
		if err != nil {
			return err
		}
		num, err := strconv.Atoi(args[0])
		if err != nil {
			return fmt.Errorf("invalid number: %s", args[0])
		}
		ws, idx, err := store.GetActive(num)
		if err != nil {
			return err
		}
		if err := store.Retire(idx); err != nil {
			return err
		}
		output.Success("Retired: %s", ws.Name)
		return nil
	},
}

// ── Activate Command ────────────────────────────────────────────────

var wsActivateCmd = &cobra.Command{
	Use:   "activate <index>",
	Short: "Re-activate a retired workstream (use index from list)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		store, err := workstream.NewStore(workstream.DefaultConfigPath())
		if err != nil {
			return err
		}
		idx, err := strconv.Atoi(args[0])
		if err != nil {
			return fmt.Errorf("invalid index: %s", args[0])
		}
		all := store.All()
		if idx < 0 || idx >= len(all) {
			return fmt.Errorf("invalid index: %d (have %d workstreams)", idx, len(all))
		}
		if err := store.Activate(idx); err != nil {
			return err
		}
		output.Success("Re-activated: %s", all[idx].Name)
		return nil
	},
}

// ── Delete Command ──────────────────────────────────────────────────

var wsDeleteCmd = &cobra.Command{
	Use:   "delete <index>",
	Short: "Permanently remove a workstream (use index from list)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		store, err := workstream.NewStore(workstream.DefaultConfigPath())
		if err != nil {
			return err
		}
		idx, err := strconv.Atoi(args[0])
		if err != nil {
			return fmt.Errorf("invalid index: %s", args[0])
		}
		all := store.All()
		if idx < 0 || idx >= len(all) {
			return fmt.Errorf("invalid index: %d", idx)
		}

		reader := bufio.NewReader(os.Stdin)
		fmt.Printf("  Delete '%s' permanently? (y/N): ", all[idx].Name)
		confirm, _ := reader.ReadString('\n')
		confirm = strings.TrimSpace(confirm)
		if confirm != "y" && confirm != "Y" {
			fmt.Println("Canceled.")
			return nil
		}

		name := all[idx].Name
		if err := store.Delete(idx); err != nil {
			return err
		}
		output.Success("Deleted: %s", name)
		return nil
	},
}

// ── Launch Command ──────────────────────────────────────────────────

var wsLaunchCmd = &cobra.Command{
	Use:   "launch <number>",
	Short: "Launch a workstream with an AI assistant",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		store, err := workstream.NewStore(workstream.DefaultConfigPath())
		if err != nil {
			return err
		}
		num, err := strconv.Atoi(args[0])
		if err != nil {
			return fmt.Errorf("invalid number: %s", args[0])
		}
		ws, idx, err := store.GetActive(num)
		if err != nil {
			return err
		}

		aiID := wsAI
		if aiID == "" {
			aiID = ws.AI
		}
		if aiID == "" {
			aiID = "claude"
		}
		launcher := workstream.FindLauncher(aiID)
		if launcher == nil {
			return fmt.Errorf("unknown AI tool: %s", aiID)
		}
		if !launcher.Installed(platform.Current()) {
			output.Error("%s is not installed.", launcher.Name())
			output.Dim("  Install: %s", launcher.InstallCmd())
			return nil
		}

		_ = store.TouchLastUsed(idx)
		return launcher.Launch(ws, workstream.LaunchOptions{
			Resume:      wsResume,
			AutoApprove: wsAutoApprove,
		})
	},
}

// ── Registry Command ────────────────────────────────────────────────

var wsRegistryCmd = &cobra.Command{
	Use:   "registry",
	Short: "Show installed AI assistants and IDEs",
	RunE: func(cmd *cobra.Command, args []string) error {
		start := time.Now()
		p := platform.Current()

		output.Banner()
		output.Header("WORKSTREAMS — Tool Registry")

		headers := []string{"Tool", "Type", "Status", "Install"}
		var rows [][]string
		for _, l := range workstream.AllLaunchers() {
			status := "✗ not found"
			if l.Installed(p) {
				status = "✓ installed"
			}
			rows = append(rows, []string{l.Name(), l.Kind(), status, l.InstallCmd()})
		}
		output.Table(headers, rows)
		output.Footer(time.Since(start))
		return nil
	},
}

// ── Inventory Command ───────────────────────────────────────────────

var wsInventoryCmd = &cobra.Command{
	Use:     "inventory",
	Short:   "Scan system for installed AI tools, IDEs, and git repos",
	Aliases: []string{"inv"},
	RunE: func(cmd *cobra.Command, args []string) error {
		start := time.Now()
		p := platform.Current()

		inv := workstream.ScanInventory(p)
		if err := workstream.SaveInventory(inv); err != nil {
			return err
		}

		if JsonOutput {
			data, _ := json.MarshalIndent(inv, "", "  ")
			fmt.Println(string(data))
			return nil
		}

		gold := output.TitleStyle
		dim := output.DimStyle
		green := output.SuccessStyle
		red := output.ErrorStyle

		output.Banner()
		output.Header("SYSTEM INVENTORY")

		// System info
		fmt.Printf("  System:  %s/%s  Shell: %s\n", inv.OS, inv.Arch, inv.Shell)
		fmt.Println()

		// Tools table
		headers := []string{"Tool", "Type", "Status"}
		var rows [][]string
		for _, t := range inv.Tools {
			status := "not installed"
			if t.Installed {
				status = "installed"
			}
			rows = append(rows, []string{t.Name, t.Kind, status})
		}
		output.Table(headers, rows)

		// Summary
		ai := inv.InstalledAI()
		ides := inv.InstalledIDEs()

		aiLabel := red.Render("0")
		if len(ai) > 0 {
			aiLabel = green.Render(fmt.Sprintf("%d", len(ai)))
		}
		ideLabel := red.Render("0")
		if len(ides) > 0 {
			ideLabel = green.Render(fmt.Sprintf("%d", len(ides)))
		}

		output.Dashboard(map[string]string{
			"AI Tools":  fmt.Sprintf("%s installed", aiLabel),
			"IDEs":      fmt.Sprintf("%s installed", ideLabel),
			"Git Repos": fmt.Sprintf("%d found", len(inv.GitRepos)),
		})

		// Show repos
		if len(inv.GitRepos) > 0 {
			fmt.Println()
			fmt.Println(gold.Render("  Git Repositories"))
			limit := len(inv.GitRepos)
			if limit > 20 {
				limit = 20
			}
			for _, r := range inv.GitRepos[:limit] {
				fmt.Printf("    %s  %s\n", dim.Render(r.Name), dim.Render(r.Path))
			}
			if len(inv.GitRepos) > 20 {
				fmt.Printf("    %s\n", dim.Render(fmt.Sprintf("... and %d more", len(inv.GitRepos)-20)))
			}
		}

		_ = gold
		output.Footer(time.Since(start))
		return nil
	},
}

// ── Setup Command ───────────────────────────────────────────────────

var wsSetupCmd = &cobra.Command{
	Use:   "setup",
	Short: "Install AI assistants, IDEs, and configure Sirsi integration",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runSetupFlow()
	},
}

func runSetupFlow() error {
	gold := output.TitleStyle
	dim := output.DimStyle
	reader := bufio.NewReader(os.Stdin)
	p := platform.Current()

	fmt.Println()
	installToolGroup("AI Assistants", "ai", p, reader, gold, dim)
	fmt.Println()
	installToolGroup("IDEs", "ide", p, reader, gold, dim)

	// Offer to add a workstream if none exist
	store, _ := workstream.NewStore(workstream.DefaultConfigPath())
	if store != nil && len(store.Active()) == 0 {
		fmt.Println()
		fmt.Println(gold.Render("  Add your first workstream"))
		fmt.Println(dim.Render("  ─────────────────────────────"))
		_ = runWorkstreamAddInteractive(reader, store)
	}

	// Refresh inventory after setup
	inv := workstream.ScanInventory(p)
	_ = workstream.SaveInventory(inv)

	fmt.Println()
	output.Success("Setup complete. Run 'sirsi' or 'sw' to get started.")
	return nil
}

// installToolGroup displays launchers of a given kind, lets the user pick
// one or more to install (comma-separated), and offers Sirsi integration.
func installToolGroup(title, kind string, p platform.Platform, reader *bufio.Reader, gold, dim lipgloss.Style) {
	green := output.SuccessStyle

	fmt.Println(gold.Render("  " + title))
	fmt.Println(dim.Render("  ─────────────────────────────"))

	var launchers []workstream.Launcher
	for _, l := range workstream.AllLaunchers() {
		if l.Kind() == kind {
			launchers = append(launchers, l)
		}
	}

	for i, l := range launchers {
		status := dim.Render("not installed")
		if l.Installed(p) {
			status = green.Render("✓ installed")
		}
		fmt.Printf("    %s  %-16s %s\n", gold.Render(fmt.Sprintf("%d", i+1)), l.Name(), status)
	}

	fmt.Println()
	fmt.Print(dim.Render("  Install (e.g. 1,3 or enter to skip): "))
	input, _ := reader.ReadString('\n')
	input = strings.TrimSpace(input)
	if input == "" {
		return
	}

	for _, part := range strings.Split(input, ",") {
		part = strings.TrimSpace(part)
		n, err := strconv.Atoi(part)
		if err != nil || n < 1 || n > len(launchers) {
			continue
		}
		l := launchers[n-1]
		if l.Installed(p) {
			output.Dim("  %s already installed.", l.Name())
			continue
		}
		fmt.Println()
		fmt.Printf("  Installing %s...\n", l.Name())
		fmt.Printf("  %s\n", dim.Render(l.InstallCmd()))
		fmt.Println()
		if err := workstream.Install(l); err != nil {
			output.Error("Install failed: %v", err)
			continue
		}
		output.Success("%s installed.", l.Name())
		offerSirsiIntegration(l, reader, gold, dim)
	}
}

func offerSirsiIntegration(l workstream.Launcher, reader *bufio.Reader, gold, dim lipgloss.Style) {
	info := l.Integration()
	if info == nil {
		return
	}
	fmt.Println()
	fmt.Printf("  %s\n", info.Note)
	fmt.Printf("  Install Sirsi inside %s? (Y/n): ", l.Name())
	answer, _ := reader.ReadString('\n')
	answer = strings.TrimSpace(strings.ToLower(answer))
	if answer == "n" || answer == "no" {
		return
	}
	if err := workstream.Integrate(l); err != nil {
		output.Error("Integration failed: %v", err)
	} else {
		output.Success("Sirsi integrated into %s.", l.Name())
	}
}

// ── init ────────────────────────────────────────────────────────────

func init() {
	workCmd.Flags().BoolVar(&wsDocs, "docs", false, "Open workstream documentation")

	wsLaunchCmd.Flags().BoolVar(&wsResume, "resume", false, "Resume last session")
	wsLaunchCmd.Flags().BoolVar(&wsAutoApprove, "auto-approve", false, "Skip permission prompts")
	wsLaunchCmd.Flags().StringVar(&wsAI, "ai", "", "AI tool to use (claude, codex, gemini, antigravity)")

	wsAddCmd.Flags().StringVar(&wsAI, "ai", "", "Default AI tool for this workstream")
	wsAddCmd.Flags().StringVar(&wsIDE, "ide", "", "Default IDE for this workstream")

	workCmd.AddCommand(wsListCmd)
	workCmd.AddCommand(wsAddCmd)
	workCmd.AddCommand(wsRenameCmd)
	workCmd.AddCommand(wsRetireCmd)
	workCmd.AddCommand(wsActivateCmd)
	workCmd.AddCommand(wsDeleteCmd)
	workCmd.AddCommand(wsLaunchCmd)
	workCmd.AddCommand(wsRegistryCmd)
	workCmd.AddCommand(wsSetupCmd)
	workCmd.AddCommand(wsInventoryCmd)
}
