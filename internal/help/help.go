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
				{"Initialize a project", "Run `pantheon thoth init` in any Git repo to scaffold\nthe .thoth/ knowledge system (memory.yaml, journal.md, artifacts/)."},
				{"Sync memory", "Run `pantheon thoth sync` to auto-detect project metadata,\nparse recent git history, and update .thoth/memory.yaml."},
				{"Compact before context loss", "Run `pantheon thoth compact -s \"summary\"` to persist\nsession decisions into the journal before context compression."},
				{"Manage neural weights", "Run `pantheon thoth brain` to check status,\n`--update` to fetch latest, `--remove` to clean up."},
			},
			Examples: []string{
				"pantheon thoth init --yes --name myproject",
				"pantheon thoth sync --since \"48 hours ago\"",
				"pantheon thoth compact -s \"Switched to interface providers\"",
			},
			Platform: "All platforms. Neural weights require ~50 MB disk.",
		},
		"maat": {
			Name: "Ma'at", Glyph: "\U00013184", Tagline: "QA/QC Governance & Policy Enforcement",
			Steps: []step{
				{"Run a governance audit", "Run `pantheon maat audit` to execute a full coverage\nassessment with per-package streaming progress."},
				{"Enforce policies", "Run `pantheon maat scales` to detect infrastructure\npolicy drifts and optionally fix them with `--fix`."},
				{"Autonomous healing", "Run `pantheon maat heal` to let Isis auto-remediate\nfailures detected by the audit cycle."},
				{"Measure vital signs", "Run `pantheon maat pulse` for a real-time metrics\nsnapshot written to .pantheon/metrics.json."},
			},
			Examples: []string{
				"pantheon maat audit --skip-test",
				"pantheon maat scales --fix",
				"pantheon maat pulse --json",
			},
			Platform: "All platforms. Tests require Go toolchain in PATH.",
		},
		"anubis": {
			Name: "Anubis", Glyph: "\U00013062", Tagline: "Infrastructure Hygiene, Dedup & Digital Cleanliness",
			Steps: []step{
				{"Scan for waste", "Run `pantheon scan` (or `pantheon anubis weigh`) to\ndetect infrastructure waste across your workstation."},
				{"Reclaim storage", "Run `pantheon anubis judge` to move detected artifacts\nto Trash. Always runs in `--dry-run` mode by default."},
				{"Hunt ghost apps", "Run `pantheon ghosts` (or `pantheon anubis ka`) to\nfind remnants of uninstalled applications."},
				{"Find duplicates", "Run `pantheon dedup` (or `pantheon anubis mirror`) to\nidentify duplicate files across directories."},
				{"Monitor resources", "Run `pantheon guard` (or `pantheon anubis guard`) to\nwatch RAM pressure and system resource usage."},
			},
			Examples: []string{
				"pantheon scan --all",
				"pantheon ghosts --sudo",
				"pantheon dedup ~/Downloads ~/Documents",
			},
			Platform: "macOS (primary), Linux. Some rules are macOS-specific.",
		},
		"seshat": {
			Name: "Seshat", Glyph: "\U00013046", Tagline: "Universal Knowledge Grafting Engine",
			Steps: []step{
				{"Ingest knowledge", "Run `pantheon seshat ingest` to pull knowledge items\nfrom Chrome, Gemini, Claude, Apple Notes, and Google Workspace."},
				{"Export to targets", "Run `pantheon seshat export <target>` to push\ningested knowledge to NotebookLM, Apple Notes, or Thoth."},
				{"List knowledge items", "Run `pantheon seshat list` to see all ingested\nknowledge items with their sources."},
				{"Manage Chrome profiles", "Run `pantheon seshat profiles chrome` to list\navailable profiles, `pantheon seshat open chrome --profile X` to launch."},
			},
			Examples: []string{
				"pantheon seshat ingest --source chrome-history --all-profiles",
				"pantheon seshat export notebooklm --profile SirsiMaster",
				"pantheon seshat adapters",
			},
			Platform: "macOS + Linux. Chrome history requires Chrome installed.",
		},
		"hapi": {
			Name: "Hapi", Glyph: "\U00013197", Tagline: "Hardware, Portfolio & Accelerated Compute",
			Steps: []step{
				{"Hardware summary", "Run `pantheon hapi scan` for a quick dashboard of\nCPU, GPU, and Neural Engine status."},
				{"Deep system profile", "Run `pantheon hapi profile` to generate a\nhigh-fidelity architecture profile for Seba consumption."},
				{"ANE tokenization", "Run `pantheon hapi compute --tokenize \"text\"` to run\nML tokenization via the Apple Neural Engine or CPU fallback."},
			},
			Examples: []string{
				"pantheon hapi scan",
				"pantheon hapi profile",
				"pantheon hapi compute --tokenize \"Hello world\"",
			},
			Platform: "macOS (Apple Silicon preferred). Neural Engine acceleration on M-series.",
		},
		"seba": {
			Name: "Seba", Glyph: "\U000131BD", Tagline: "Infrastructure Mapping, Fleet Discovery & Project Registry",
			Steps: []step{
				{"Map architecture", "Run `pantheon seba scan` to build a graph of your\nworkstation's architecture and dependencies."},
				{"Generate diagrams", "Run `pantheon seba diagram --type all --html` to\ncreate Mermaid diagrams rendered as self-contained HTML."},
				{"Build project registry", "Run `pantheon seba book` to generate the\nPantheon Book with all projects in your registry."},
				{"Fleet discovery", "Run `pantheon seba fleet` to discover network hosts.\nUse `--containers` for Docker-only audits."},
			},
			Examples: []string{
				"pantheon seba diagram --type hierarchy",
				"pantheon seba diagram --type all --html",
				"pantheon seba fleet --containers",
			},
			Platform: "All platforms. Fleet discovery requires network access.",
		},
		"ka": {
			Name: "Ka", Glyph: "\U00013093", Tagline: "Ghost App Detection & Spirit Hunting",
			Steps: []step{
				{"Detect ghost apps", "Run `pantheon ghosts` to scan for remnants of\nuninstalled applications (Launch Services, plists, caches)."},
				{"Deep scan with sudo", "Run `pantheon ghosts --sudo` to include\nsystem-level directories that require elevated access."},
			},
			Examples: []string{
				"pantheon ghosts",
				"pantheon ghosts --sudo",
			},
			Platform: "macOS primary. Linux support for common package managers.",
		},
		"ra": {
			Name: "Ra", Glyph: "\u2600\uFE0F", Tagline: "Supreme Overseer & Cross-Repo Orchestrator",
			Steps: []step{
				{"What Ra does", "Ra orchestrates all Pantheon deities across every Sirsi\nrepository. He dispatches parallel Claude agents to run health checks,\ntests, lints, and arbitrary tasks fleet-wide."},
				{"Quick health check", "Run `pantheon ra health` to verify build status,\ngit cleanliness, and recent commits across all repos."},
				{"Parallel testing", "Run `pantheon ra test` to execute each repo's test\nsuite in parallel via dedicated Claude agents."},
				{"Targeted work", "Run `pantheon ra task pantheon \"fix X\"` to dispatch a\nfocused task to a single repo with full tool access."},
				{"Fleet-wide broadcast", "Run `pantheon ra broadcast \"check deps\"` to run\nthe same prompt across every repo simultaneously."},
				{"Nightly CI", "Run `pantheon ra nightly` for a comprehensive three-phase\ncheck: health, lint, and test across the fleet."},
			},
			Examples: []string{
				"pantheon ra health",
				"pantheon ra test",
				"pantheon ra task pantheon \"fix the seba test failures\"",
				"pantheon ra broadcast \"check for security vulnerabilities in dependencies\"",
				"pantheon ra nightly",
				"pantheon ra status",
			},
			Platform: "All platforms. Requires python3 and claude-code-sdk (pip3 install claude-code-sdk).",
		},
		"isis": {
			Name: "Isis", Glyph: "\U00013050", Tagline: "Health, Remediation & Network Security",
			Steps: []step{
				{"System health diagnostic", "Run `pantheon doctor` to execute a one-shot\nhealth check covering RAM, disk, processes, panics, and Jetsam events."},
				{"Network security audit", "Run `pantheon isis network` to audit DNS config,\nWiFi security, TLS, CA certificates, VPN, and firewall state."},
				{"Auto-fix network issues", "Run `pantheon isis network --fix` to safely apply\nencrypted DNS and firewall fixes with automatic rollback on failure."},
				{"Rollback network changes", "Run `pantheon isis network --rollback` to restore\nDNS to the state before the last --fix."},
				{"Autonomous healing", "Run `pantheon isis heal` or `pantheon maat heal` to\nautomatically remediate governance failures."},
				{"Resource monitoring", "Run `pantheon guard` to watch RAM pressure\nand system resource usage in real-time."},
			},
			Examples: []string{
				"pantheon doctor",
				"pantheon doctor --json",
				"pantheon isis network",
				"pantheon isis network --fix",
				"pantheon isis network --rollback",
				"pantheon isis heal --fix --full",
				"pantheon guard",
			},
			Platform: "macOS + Linux. Network checks are macOS-specific. Some features require admin.",
		},
		"net": {
			Name: "Net", Glyph: "\U00013070", Tagline: "Scope Weaver & Task Definition",
			Steps: []step{
				{"Check alignment", "Run `pantheon net status` (or `pantheon neith status`) to\nverify cross-module alignment and detect drift."},
				{"Align modules", "Run `pantheon net align` to run consistency checks\nacross all modules and flag mismatches."},
				{"Scope weaving", "Net defines the task scopes that Ra dispatches.\nScopes are YAML configs in configs/scopes/ that describe work to be done."},
			},
			Examples: []string{
				"pantheon net status",
				"pantheon net align",
			},
			Platform: "All platforms.",
		},
		"osiris": {
			Name: "Osiris", Glyph: "\U00013079", Tagline: "State Snapshots & Checkpoints",
			Steps: []step{
				{"State preservation", "Osiris preserves system state snapshots for\nrollback and recovery. Currently in development."},
				{"Integration with Thoth", "Osiris complements Thoth's session memory by\ncapturing point-in-time infrastructure state."},
			},
			Examples: []string{
				"pantheon osiris  (coming soon)",
			},
			Platform: "All platforms.",
		},
	}
}

// AllDeities returns the sorted list of deity names with guides.
func AllDeities() []string {
	return []string{
		"anubis", "hapi", "isis",
		"maat", "net", "osiris", "ra", "seba",
		"seshat", "thoth",
	}
}

// ShowGuide renders a rich terminal guide for the given deity.
func ShowGuide(deity string) error {
	deity = strings.ToLower(strings.TrimSpace(deity))
	g, ok := guides()[deity]
	if !ok {
		return fmt.Errorf("unknown deity %q — run `pantheon help --list` to see available guides", deity)
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
		codeStyle := output.BoxStyle.Copy()
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
	fmt.Println(output.DimStyle.Render("  Open in browser: pantheon help " + deity + " --docs"))
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
	fmt.Println(output.DimStyle.Render("  Usage: pantheon help <deity>"))
	fmt.Println(output.DimStyle.Render("         pantheon help <deity> --docs   (open web docs)"))
	fmt.Println()
}

// docsURL returns the web documentation URL for a deity.
func docsURL(deity string) string {
	return fmt.Sprintf("https://pantheon.sirsi.ai/pantheon/%s.html", strings.ToLower(deity))
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
		return fmt.Errorf("unknown deity %q — run `pantheon help --list` to see available guides", deity)
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
