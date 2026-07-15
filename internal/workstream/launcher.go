package workstream

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"

	"github.com/SirsiMaster/sirsi-pantheon/internal/platform"
)

// ── Binary resolution ───────────────────────────────────────────────
//
// The menubar runs under launchd, which hands processes a MINIMAL PATH
// (/usr/bin:/bin:/usr/sbin:/sbin) that omits ~/.local/bin — exactly where the
// `claude` and `codex` CLIs install. A bare exec.LookPath therefore reports
// those agents as "not installed" while a GUI app like VS Code (detected via
// its /Applications bundle) shows installed — the diabolical inversion where
// Pantheon can't see the very agents building it. resolveBinary fixes this by
// searching an AUGMENTED candidate path: the inherited PATH, the user's
// login-shell PATH, and the common user-bin dirs a truncated PATH drops.

var (
	augmentedPathOnce sync.Once
	augmentedPathDirs []string
	// forceCandidateDirs is a test seam: when non-nil, resolveBinary searches
	// exactly these dirs (after the inherited PATH), bypassing the cache.
	forceCandidateDirs []string
)

func augmentedPath() []string {
	if forceCandidateDirs != nil {
		return forceCandidateDirs
	}
	augmentedPathOnce.Do(func() {
		seen := map[string]bool{}
		var dirs []string
		add := func(d string) {
			if d != "" && !seen[d] {
				seen[d] = true
				dirs = append(dirs, d)
			}
		}
		// 1) the PATH this process inherited
		for _, d := range filepath.SplitList(os.Getenv("PATH")) {
			add(d)
		}
		// 2) the user's LOGIN-shell PATH — captures shell-profile additions a
		//    launchd/GUI process never sees. Best-effort.
		if sh := os.Getenv("SHELL"); sh != "" {
			if out, err := exec.Command(sh, "-lc", `printf %s "$PATH"`).Output(); err == nil {
				for _, d := range filepath.SplitList(strings.TrimSpace(string(out))) {
					add(d)
				}
			}
		}
		// 3) common user-bin dirs a truncated launchd PATH omits
		if home, err := os.UserHomeDir(); err == nil {
			add(filepath.Join(home, ".local", "bin"))
			add(filepath.Join(home, ".cargo", "bin"))
			add(filepath.Join(home, "bin"))
		}
		add("/opt/homebrew/bin")
		add("/usr/local/bin")
		add("/usr/bin")
		add("/bin")
		augmentedPathDirs = dirs
	})
	return augmentedPathDirs
}

// resolveBinary finds an executable by name across the augmented path. It is the
// PATH-independent replacement for exec.LookPath used by every Installed()/Launch()
// check so detection is correct even under a launchd-truncated PATH.
func resolveBinary(name string) (string, bool) {
	if p, err := exec.LookPath(name); err == nil {
		return p, true
	}
	for _, dir := range augmentedPath() {
		cand := filepath.Join(dir, name)
		if isExecutableFile(cand) {
			return cand, true
		}
	}
	return "", false
}

func isExecutableFile(path string) bool {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return false
	}
	return info.Mode()&0o111 != 0
}

// appBundleInstalled reports whether a GUI .app bundle exists in the standard
// macOS application locations — a secondary signal for IDE launchers whose CLI
// shim may not be on any PATH.
func appBundleInstalled(appName string) bool {
	bases := []string{"/Applications"}
	if home, err := os.UserHomeDir(); err == nil {
		bases = append(bases, filepath.Join(home, "Applications"))
	}
	for _, base := range bases {
		if info, err := os.Stat(filepath.Join(base, appName)); err == nil && info.IsDir() {
			return true
		}
	}
	return false
}

// Launcher knows how to start an AI assistant or IDE for a workstream.
type Launcher interface {
	ID() string
	Name() string
	Kind() string // "ai" or "ide"
	Installed(p platform.Platform) bool
	InstallCmd() string            // shell command to install the tool
	Integration() *IntegrationInfo // how to install Sirsi inside this tool (nil if unsupported)
	Launch(ws Workstream, opts LaunchOptions) error
}

// IntegrationInfo describes how Sirsi integrates into a tool.
type IntegrationInfo struct {
	Note string          // human description shown to user
	Type IntegrationType // how integration works
	Path string          // config file path (for MCP JSON type)
	Key  string          // JSON key style: "mcpServers" or "servers"
}

// IntegrationType is the method of integration.
type IntegrationType int

const (
	IntegrateMCPJSON IntegrationType = iota // write MCP config to a JSON file
	IntegrateCLI                            // run a CLI command (e.g. claude mcp add)
)

// Install runs the launcher's install command with live output.
func Install(l Launcher) error {
	cmd := exec.Command("sh", "-c", l.InstallCmd())
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// Integrate installs Sirsi as a plugin/extension inside the given tool.
func Integrate(l Launcher) error {
	info := l.Integration()
	if info == nil {
		return nil
	}
	switch info.Type {
	case IntegrateCLI:
		cmd := exec.Command("sh", "-c", info.Path) // Path holds the CLI command for this type
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		return cmd.Run()
	case IntegrateMCPJSON:
		return writeMCPConfig(info.Path, info.Key)
	}
	return nil
}

// writeMCPConfig writes or merges the Sirsi MCP server entry into a JSON config file.
func writeMCPConfig(path string, key string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create dir %s: %w", dir, err)
	}

	// Read existing config or start fresh
	existing := make(map[string]interface{})
	if data, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(data, &existing)
	}

	// Ensure the servers key exists
	servers, ok := existing[key].(map[string]interface{})
	if !ok {
		servers = make(map[string]interface{})
	}

	// Add sirsi entry
	servers["sirsi"] = map[string]interface{}{
		"command": sirsiCommand(),
		"args":    []string{"mcp"},
	}
	existing[key] = servers

	data, err := json.MarshalIndent(existing, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	return os.WriteFile(path, data, 0644)
}

// LaunchOptions configures a workstream launch.
type LaunchOptions struct {
	Resume      bool
	AutoApprove bool
	SessionName string
	Dir         string // expanded path
}

// SetTerminalTitle sets the terminal window and tab title via ANSI escapes.
func SetTerminalTitle(title string) {
	fmt.Fprintf(os.Stdout, "\033]0;%s\007", title)
	fmt.Fprintf(os.Stdout, "\033]1;%s\007", title)
}

// ── Claude Launcher ─────────────────────────────────────────────────

// ClaudeLauncher launches Claude Code CLI sessions.
type ClaudeLauncher struct{}

func (c ClaudeLauncher) ID() string   { return "claude" }
func (c ClaudeLauncher) Name() string { return "Claude Code" }
func (c ClaudeLauncher) Kind() string { return "ai" }

func (c ClaudeLauncher) Installed(p platform.Platform) bool {
	_, ok := resolveBinary("claude")
	return ok
}

func (c ClaudeLauncher) InstallCmd() string {
	return "npm install -g @anthropic-ai/claude-code"
}

func (c ClaudeLauncher) Integration() *IntegrationInfo {
	return &IntegrationInfo{
		Note: "Add Sirsi as an MCP server inside Claude Code",
		Type: IntegrateCLI,
		Path: fmt.Sprintf("claude mcp add --scope user sirsi %s mcp", shellQuote(sirsiCommand())),
	}
}

func (c ClaudeLauncher) Launch(ws Workstream, opts LaunchOptions) error {
	binary, ok := resolveBinary("claude")
	if !ok {
		return fmt.Errorf("claude not found in PATH or common install dirs")
	}

	dir := opts.Dir
	if dir == "" {
		dir = ExpandDir(ws.Dir)
	}

	name := opts.SessionName
	if name == "" {
		name = ws.Name
	}

	args := []string{"claude"}
	if opts.Resume {
		args = append(args, "--resume", name)
	} else {
		args = append(args, "--name", name)
	}
	if opts.AutoApprove {
		args = append(args, "--dangerously-skip-permissions")
	}

	SetTerminalTitle("𓁟 " + ws.Name)

	if err := os.Chdir(dir); err != nil {
		return fmt.Errorf("chdir %s: %w", dir, err)
	}

	return syscall.Exec(binary, args, os.Environ())
}

// ── IDE Launchers ───────────────────────────────────────────────────

// VSCodeLauncher launches Visual Studio Code.
type VSCodeLauncher struct{}

func (v VSCodeLauncher) ID() string   { return "vscode" }
func (v VSCodeLauncher) Name() string { return "VS Code" }
func (v VSCodeLauncher) Kind() string { return "ide" }

func (v VSCodeLauncher) Installed(p platform.Platform) bool {
	if _, ok := resolveBinary("code"); ok {
		return true
	}
	return appBundleInstalled("Visual Studio Code.app")
}

func (v VSCodeLauncher) InstallCmd() string {
	return "brew install --cask visual-studio-code"
}

func (v VSCodeLauncher) Integration() *IntegrationInfo {
	return &IntegrationInfo{
		Note: "Add Sirsi MCP server to .vscode/mcp.json",
		Type: IntegrateMCPJSON,
		Path: ".vscode/mcp.json",
		Key:  "servers",
	}
}

func (v VSCodeLauncher) Launch(ws Workstream, opts LaunchOptions) error {
	dir := opts.Dir
	if dir == "" {
		dir = ExpandDir(ws.Dir)
	}
	cmd := exec.Command("code", dir)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// CursorLauncher launches the Cursor IDE.
type CursorLauncher struct{}

func (c CursorLauncher) ID() string   { return "cursor" }
func (c CursorLauncher) Name() string { return "Cursor" }
func (c CursorLauncher) Kind() string { return "ide" }

func (c CursorLauncher) Installed(p platform.Platform) bool {
	if _, ok := resolveBinary("cursor"); ok {
		return true
	}
	return appBundleInstalled("Cursor.app")
}

func (c CursorLauncher) InstallCmd() string {
	return "brew install --cask cursor"
}

func (c CursorLauncher) Integration() *IntegrationInfo {
	return &IntegrationInfo{
		Note: "Add Sirsi MCP server to .cursor/mcp.json",
		Type: IntegrateMCPJSON,
		Path: ".cursor/mcp.json",
		Key:  "mcpServers",
	}
}

func (c CursorLauncher) Launch(ws Workstream, opts LaunchOptions) error {
	dir := opts.Dir
	if dir == "" {
		dir = ExpandDir(ws.Dir)
	}
	cmd := exec.Command("cursor", dir)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// WindsurfLauncher launches the Windsurf IDE.
type WindsurfLauncher struct{}

func (w WindsurfLauncher) ID() string   { return "windsurf" }
func (w WindsurfLauncher) Name() string { return "Windsurf" }
func (w WindsurfLauncher) Kind() string { return "ide" }

func (w WindsurfLauncher) Installed(p platform.Platform) bool {
	if _, ok := resolveBinary("windsurf"); ok {
		return true
	}
	return appBundleInstalled("Windsurf.app")
}

func (w WindsurfLauncher) InstallCmd() string {
	return "brew install --cask windsurf"
}

func (w WindsurfLauncher) Integration() *IntegrationInfo {
	return &IntegrationInfo{
		Note: "Add Sirsi MCP server to .windsurf/mcp.json",
		Type: IntegrateMCPJSON,
		Path: ".windsurf/mcp.json",
		Key:  "mcpServers",
	}
}

func (w WindsurfLauncher) Launch(ws Workstream, opts LaunchOptions) error {
	dir := opts.Dir
	if dir == "" {
		dir = ExpandDir(ws.Dir)
	}
	cmd := exec.Command("windsurf", dir)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// ZedLauncher launches the Zed editor.
type ZedLauncher struct{}

func (z ZedLauncher) ID() string   { return "zed" }
func (z ZedLauncher) Name() string { return "Zed" }
func (z ZedLauncher) Kind() string { return "ide" }

func (z ZedLauncher) Installed(p platform.Platform) bool {
	if _, ok := resolveBinary("zed"); ok {
		return true
	}
	return appBundleInstalled("Zed.app")
}

func (z ZedLauncher) InstallCmd() string {
	return "brew install --cask zed"
}

func (z ZedLauncher) Integration() *IntegrationInfo { return nil }

func (z ZedLauncher) Launch(ws Workstream, opts LaunchOptions) error {
	dir := opts.Dir
	if dir == "" {
		dir = ExpandDir(ws.Dir)
	}
	cmd := exec.Command("zed", dir)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// ── AI Launchers (stubs for Phase 2) ────────────────────────────────

// CodexLauncher launches OpenAI Codex CLI.
type CodexLauncher struct{}

func (c CodexLauncher) ID() string   { return "codex" }
func (c CodexLauncher) Name() string { return "Codex" }
func (c CodexLauncher) Kind() string { return "ai" }

func (c CodexLauncher) Installed(p platform.Platform) bool {
	_, ok := resolveBinary("codex")
	return ok
}

func (c CodexLauncher) InstallCmd() string {
	return "npm install -g @openai/codex"
}

func (c CodexLauncher) Integration() *IntegrationInfo { return nil }

func (c CodexLauncher) Launch(ws Workstream, opts LaunchOptions) error {
	binary, ok := resolveBinary("codex")
	if !ok {
		return fmt.Errorf("codex not found in PATH or common install dirs")
	}
	dir := opts.Dir
	if dir == "" {
		dir = ExpandDir(ws.Dir)
	}
	SetTerminalTitle("𓁟 " + ws.Name)
	if err := os.Chdir(dir); err != nil {
		return fmt.Errorf("chdir %s: %w", dir, err)
	}
	args := []string{"codex"}
	if opts.AutoApprove {
		args = append(args, "--dangerously-bypass-approvals-and-sandbox")
	}
	return syscall.Exec(binary, args, os.Environ())
}

// GeminiLauncher launches Google Gemini CLI.
type GeminiLauncher struct{}

func (g GeminiLauncher) ID() string   { return "gemini" }
func (g GeminiLauncher) Name() string { return "Gemini CLI" }
func (g GeminiLauncher) Kind() string { return "ai" }

func (g GeminiLauncher) Installed(p platform.Platform) bool {
	_, ok := resolveBinary("gemini")
	return ok
}

func (g GeminiLauncher) InstallCmd() string {
	return "npm install -g @google/gemini-cli"
}

func (g GeminiLauncher) Integration() *IntegrationInfo { return nil }

func (g GeminiLauncher) Launch(ws Workstream, opts LaunchOptions) error {
	binary, ok := resolveBinary("gemini")
	if !ok {
		return fmt.Errorf("gemini not found in PATH or common install dirs")
	}
	dir := opts.Dir
	if dir == "" {
		dir = ExpandDir(ws.Dir)
	}
	SetTerminalTitle("𓁟 " + ws.Name)
	if err := os.Chdir(dir); err != nil {
		return fmt.Errorf("chdir %s: %w", dir, err)
	}
	return syscall.Exec(binary, []string{"gemini"}, os.Environ())
}

// AntigravityLauncher launches Google Antigravity (IDE with built-in AI).
type AntigravityLauncher struct{}

func (a AntigravityLauncher) ID() string   { return "antigravity" }
func (a AntigravityLauncher) Name() string { return "Antigravity" }
func (a AntigravityLauncher) Kind() string { return "ide" }

func (a AntigravityLauncher) Installed(p platform.Platform) bool {
	if _, ok := resolveBinary("antigravity"); ok {
		return true
	}
	return appBundleInstalled("Antigravity.app")
}

func (a AntigravityLauncher) InstallCmd() string {
	return "See https://antigravity.dev for installation"
}

func (a AntigravityLauncher) Integration() *IntegrationInfo { return nil }

func (a AntigravityLauncher) Launch(ws Workstream, opts LaunchOptions) error {
	dir := opts.Dir
	if dir == "" {
		dir = ExpandDir(ws.Dir)
	}
	cmd := exec.Command("antigravity", dir)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// ── Launcher Registry ───────────────────────────────────────────────

// AllLaunchers returns every known launcher (AI first, then IDE).
func AllLaunchers() []Launcher {
	return []Launcher{
		// AI assistants (CLI tools that take over the terminal)
		ClaudeLauncher{},
		CodexLauncher{},
		GeminiLauncher{},
		// IDEs (GUI applications, some with built-in AI)
		VSCodeLauncher{},
		CursorLauncher{},
		WindsurfLauncher{},
		AntigravityLauncher{},
		ZedLauncher{},
	}
}

// FindLauncher returns the launcher matching the given ID, or nil.
func FindLauncher(id string) Launcher {
	for _, l := range AllLaunchers() {
		if l.ID() == id {
			return l
		}
	}
	return nil
}

// DefaultAILauncher returns the first installed AI launcher.
// Preference order: claude > codex > gemini.
func DefaultAILauncher(p platform.Platform) Launcher {
	for _, l := range AllLaunchers() {
		if l.Kind() == "ai" && l.Installed(p) {
			return l
		}
	}
	return nil
}

func sirsiCommand() string {
	exe, err := os.Executable()
	if err != nil {
		return "sirsi"
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		return resolved
	}
	return exe
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}
