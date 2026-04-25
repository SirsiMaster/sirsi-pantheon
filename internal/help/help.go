// Package help provides rich terminal-formatted guides and web doc
// launchers for each Pantheon deity.
package help

import (
	"fmt"
	"os/exec"
	"runtime"
	"strings"

	"github.com/SirsiMaster/sirsi-pantheon/internal/output"
)

// deityGuide holds the structured content for a single deity's guide.
type deityGuide struct {
	Name     string
	Glyph    string
	Tagline  string
	Steps    []step
	Examples []string
	Platform string
}

type step struct {
	Title string
	Body  string
}

// guides returns the full registry of deity guides.
func guides() map[string]deityGuide {
	return map[string]deityGuide{
		"thoth": {
			Name: "Thoth", Glyph: "\U000130DF", Tagline: "Persistent Knowledge & Brain Manager",
			Steps: []step{
				{"Initialize a project", "Run `sirsi thoth init` in any Git repo to scaffold\nthe .thoth/ knowledge system (memory.yaml, journal.md, artifacts/)."},
				{"Sync memory", "Run `sirsi thoth sync` to auto-detect project metadata,\nparse recent git history, and update .thoth/memory.yaml."},
				{"Compact before context loss", "Run `sirsi thoth compact -s \"summary\"` to persist\nsession decisions into the journal before context compression."},
				{"Manage neural weights", "Run `sirsi thoth brain` to check status,\n`--update` to fetch latest, `--remove` to clean up."},
			},
			Examples: []string{
				"sirsi thoth init --yes --name myproject",
				"sirsi thoth sync --since \"48 hours ago\"",
				"sirsi thoth compact -s \"Switched to interface providers\"",
			},
			Platform: "All platforms. Neural weights require ~50 MB disk.",
		},
		"maat": {
			Name: "Ma'at", Glyph: "\U00013184", Tagline: "QA/QC Governance & Policy Enforcement",
			Steps: []step{
				{"Run a governance audit", "Run `sirsi maat audit` to execute a full coverage\nassessment with per-package streaming progress."},
				{"Enforce policies", "Run `sirsi maat scales` to detect infrastructure\npolicy drifts and optionally fix them with `--fix`."},
				{"Autonomous healing", "Run `sirsi maat heal` to let Isis auto-remediate\nfailures detected by the audit cycle."},
				{"Measure vital signs", "Run `sirsi maat pulse` for a real-time metrics\nsnapshot written to .pantheon/metrics.json."},
			},
			Examples: []string{
				"sirsi maat audit --skip-test",
				"sirsi maat scales --fix",
				"sirsi maat pulse --json",
			},
			Platform: "All platforms. Tests require Go toolchain in PATH.",
		},
		"anubis": {
			Name: "Anubis", Glyph: "\U00013062", Tagline: "Infrastructure Hygiene, Dedup & Digital Cleanliness",
			Steps: []step{
				{"Scan for waste", "Run `sirsi scan` (or `sirsi anubis weigh`) to\ndetect infrastructure waste across your workstation."},
				{"Reclaim storage", "Run `sirsi anubis judge` to move detected artifacts\nto Trash. Always runs in `--dry-run` mode by default."},
				{"Hunt ghost apps", "Run `sirsi ghosts` (or `sirsi anubis ka`) to\nfind remnants of uninstalled applications."},
				{"Find duplicates", "Run `sirsi dedup` (or `sirsi anubis mirror`) to\nidentify duplicate files across directories."},
				{"Monitor resources", "Run `sirsi guard` (or `sirsi anubis guard`) to\nwatch RAM pressure and system resource usage."},
			},
			Examples: []string{
				"sirsi scan --all",
				"sirsi ghosts --sudo",
				"sirsi dedup ~/Downloads ~/Documents",
			},
			Platform: "macOS (primary), Linux. Some rules are macOS-specific.",
		},
		"seshat": {
			Name: "Seshat", Glyph: "\U00013046", Tagline: "Universal Knowledge Grafting Engine",
			Steps: []step{
				{"Ingest knowledge", "Run `sirsi seshat ingest` to pull knowledge items\nfrom Chrome, Gemini, Claude, Apple Notes, and Google Workspace."},
				{"Export to targets", "Run `sirsi seshat export <target>` to push\ningested knowledge to NotebookLM, Apple Notes, or Thoth."},
				{"List knowledge items", "Run `sirsi seshat list` to see all ingested\nknowledge items with their sources."},
				{"Manage Chrome profiles", "Run `sirsi seshat profiles chrome` to list\navailable profiles, `sirsi seshat open chrome --profile X` to launch."},
			},
			Examples: []string{
				"sirsi seshat ingest --source chrome-history --all-profiles",
				"sirsi seshat export notebooklm --profile SirsiMaster",
				"sirsi seshat adapters",
			},
			Platform: "macOS + Linux. Chrome history requires Chrome installed.",
		},
		"seba": {
			Name: "Seba", Glyph: "\U000131BD", Tagline: "Infrastructure Mapping, Hardware Profiling & Fleet Discovery",
			Steps: []step{
				{"Hardware summary", "Run `sirsi seba hardware` for a dashboard of\nCPU, GPU, RAM, Neural Engine, and accelerator status."},
				{"Deep system profile", "Run `sirsi seba profile` to generate a\nhigh-fidelity architecture profile saved as JSON."},
				{"ANE tokenization", "Run `sirsi seba compute --tokenize \"text\"` to run\nML tokenization via the Apple Neural Engine or CPU fallback."},
				{"Map architecture", "Run `sirsi seba scan` to build a graph of your\nworkstation's architecture and dependencies."},
				{"Generate diagrams", "Run `sirsi seba diagram --type all --html` to\ncreate Mermaid diagrams rendered as self-contained HTML."},
				{"Build project registry", "Run `sirsi seba book` to generate the\nPantheon Book with all projects in your registry."},
				{"Fleet discovery", "Run `sirsi seba fleet` to discover network hosts.\nUse `--containers` for Docker-only audits."},
			},
			Examples: []string{
				"sirsi seba hardware",
				"sirsi seba profile",
				"sirsi seba compute --tokenize \"Hello world\"",
				"sirsi seba diagram --type hierarchy",
				"sirsi seba diagram --type all --html",
				"sirsi seba fleet --containers",
			},
			Platform: "All platforms. ANE on Apple Silicon. NVIDIA/AMD on Linux. Fleet requires network.",
		},
		"ra": {
			Name: "Ra", Glyph: "\u2600\uFE0F", Tagline: "Supreme Overseer & Cross-Repo Orchestrator",
			Steps: []step{
				{"What Ra does", "Ra orchestrates all Pantheon deities across every Sirsi\nrepository. He dispatches parallel Claude agents to run health checks,\ntests, lints, and arbitrary tasks fleet-wide."},
				{"Quick health check", "Run `sirsi ra health` to verify build status,\ngit cleanliness, and recent commits across all repos."},
				{"Parallel testing", "Run `sirsi ra test` to execute each repo's test\nsuite in parallel via dedicated Claude agents."},
				{"Targeted work", "Run `sirsi ra task sirsi \"fix X\"` to dispatch a\nfocused task to a single repo with full tool access."},
				{"Fleet-wide broadcast", "Run `sirsi ra broadcast \"check deps\"` to run\nthe same prompt across every repo simultaneously."},
				{"Nightly CI", "Run `sirsi ra nightly` for a comprehensive three-phase\ncheck: health, lint, and test across the fleet."},
			},
			Examples: []string{
				"sirsi ra health",
				"sirsi ra test",
				"sirsi ra task sirsi \"fix the seba test failures\"",
				"sirsi ra broadcast \"check for security vulnerabilities in dependencies\"",
				"sirsi ra nightly",
				"sirsi ra status",
			},
			Platform: "All platforms. Requires python3 and claude-code-sdk (pip3 install claude-code-sdk).",
		},
		"isis": {
			Name: "Isis", Glyph: "\U00013050", Tagline: "Health, Remediation & Network Security",
			Steps: []step{
				{"System health diagnostic", "Run `sirsi doctor` to execute a one-shot\nhealth check covering RAM, disk, processes, panics, and Jetsam events."},
				{"Network security audit", "Run `sirsi isis network` to audit DNS config,\nWiFi security, TLS, CA certificates, VPN, and firewall state."},
				{"Auto-fix network issues", "Run `sirsi isis network --fix` to safely apply\nencrypted DNS and firewall fixes with automatic rollback on failure."},
				{"Rollback network changes", "Run `sirsi isis network --rollback` to restore\nDNS to the state before the last --fix."},
				{"Autonomous healing", "Run `sirsi isis heal` or `sirsi maat heal` to\nautomatically remediate governance failures."},
				{"Resource monitoring", "Run `sirsi guard` to watch RAM pressure\nand system resource usage in real-time."},
			},
			Examples: []string{
				"sirsi doctor",
				"sirsi doctor --json",
				"sirsi isis network",
				"sirsi isis network --fix",
				"sirsi isis network --rollback",
				"sirsi isis heal --fix --full",
				"sirsi guard",
			},
			Platform: "macOS + Linux. Network checks are macOS-specific. Some features require admin.",
		},
		"net": {
			Name: "Net", Glyph: "\U00013070", Tagline: "Scope Weaver & Task Definition",
			Steps: []step{
				{"Check alignment", "Run `sirsi net status` (or `sirsi neith status`) to\nverify cross-module alignment and detect drift."},
				{"Align modules", "Run `sirsi net align` to run consistency checks\nacross all modules and flag mismatches."},
				{"Scope weaving", "Net defines the task scopes that Ra dispatches.\nScopes are YAML configs in configs/scopes/ that describe work to be done."},
			},
			Examples: []string{
				"sirsi net status",
				"sirsi net align",
			},
			Platform: "All platforms.",
		},
		"osiris": {
			Name: "Osiris", Glyph: "\U00013079", Tagline: "Snapshot Keeper & Checkpoint Guardian",
			Steps: []step{
				{"Assess checkpoint risk", "Run `sirsi osiris assess` to evaluate uncommitted\nwork in the current Git repo. Shows file counts, diff stats,\nand a 5-level risk score with time-based escalation."},
				{"Quick status check", "Run `sirsi osiris status` for a one-line summary\nsuitable for menu bars, scripts, or CI pipelines."},
				{"Monitor a specific repo", "Run `sirsi osiris assess /path/to/repo` to check\na repo outside the current directory."},
				{"JSON output", "Run `sirsi osiris assess --json` to get structured\noutput for programmatic consumption."},
			},
			Examples: []string{
				"sirsi osiris assess",
				"sirsi osiris assess --json",
				"sirsi osiris status",
				"sirsi osiris assess ~/Development/sirsi-pantheon",
			},
			Platform: "All platforms. Requires Git.",
		},
	}
}

// AllDeities returns the sorted list of deity names with guides.
func AllDeities() []string {
	return []string{
		"anubis", "isis", "maat",
		"net", "osiris", "ra", "seba",
		"seshat", "thoth",
	}
}

// ShowGuide renders a rich terminal guide for the given deity.
func ShowGuide(deity string) error {
	deity = strings.ToLower(strings.TrimSpace(deity))
	g, ok := guides()[deity]
	if !ok {
		return fmt.Errorf("unknown deity %q — run `sirsi help --list` to see available guides", deity)
	}

	// Title
	title := fmt.Sprintf("%s %s -- %s", g.Glyph, g.Name, g.Tagline)
	fmt.Println()
	fmt.Println(output.TitleStyle.Render(title))
	fmt.Println(output.DimStyle.Render(strings.Repeat("─", 60)))
	fmt.Println()

	// Steps
	for i, s := range g.Steps {
		header := fmt.Sprintf("  %d. %s", i+1, s.Title)
		fmt.Println(output.HeaderStyle.Render(header))
		for _, line := range strings.Split(s.Body, "\n") {
			fmt.Println(output.BodyStyle.Render("     " + line))
		}
		fmt.Println()
	}

	// Examples
	if len(g.Examples) > 0 {
		fmt.Println(output.HeaderStyle.Render("  Examples"))
		fmt.Println()
		codeStyle := output.BoxStyle
		var codeLines []string
		for _, ex := range g.Examples {
			codeLines = append(codeLines, "  $ "+ex)
		}
		fmt.Println(codeStyle.Render(strings.Join(codeLines, "\n")))
		fmt.Println()
	}

	// Platform notes
	if g.Platform != "" {
		fmt.Println(output.DimStyle.Render("  Platform: " + g.Platform))
		fmt.Println()
	}

	// Footer
	url := docsURL(deity)
	fmt.Println(output.DimStyle.Render(fmt.Sprintf("  Web docs: %s", url)))
	fmt.Println(output.DimStyle.Render("  Open in browser: sirsi help " + deity + " --docs"))
	fmt.Println()

	return nil
}

// ListGuides prints all available deity guides.
func ListGuides() {
	fmt.Println()
	fmt.Println(output.TitleStyle.Render("  Available Pantheon Guides"))
	fmt.Println(output.DimStyle.Render("  " + strings.Repeat("─", 40)))
	fmt.Println()

	all := guides()
	for _, name := range AllDeities() {
		g := all[name]
		line := fmt.Sprintf("  %-12s %s %s", name, g.Glyph, g.Tagline)
		fmt.Println(output.BodyStyle.Render(line))
	}

	fmt.Println()
	fmt.Println(output.DimStyle.Render("  Usage: sirsi help <deity>"))
	fmt.Println(output.DimStyle.Render("         sirsi help <deity> --docs   (open web docs)"))
	fmt.Println()
}

// docsURL returns the web documentation URL for a deity.
func docsURL(deity string) string {
	return fmt.Sprintf("https://sirsi.ai/pantheon/%s.html", strings.ToLower(deity))
}

// OpenDocs opens the web documentation for a deity in the default browser.
func OpenDocs(deity string) error {
	deity = strings.ToLower(strings.TrimSpace(deity))

	// Validate deity name
	valid := false
	for _, d := range AllDeities() {
		if d == deity {
			valid = true
			break
		}
	}
	if !valid {
		return fmt.Errorf("unknown deity %q — run `sirsi help --list` to see available guides", deity)
	}

	url := docsURL(deity)
	return openBrowser(url)
}

// openBrowser opens a URL in the system default browser.
func openBrowser(url string) error {
	var cmd *exec.Cmd

	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "linux":
		cmd = exec.Command("xdg-open", url)
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", url)
	default:
		return fmt.Errorf("unsupported platform %q — open manually: %s", runtime.GOOS, url)
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to open browser: %w", err)
	}
	return nil
}
