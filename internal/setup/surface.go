package setup

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// Surface is a user-facing way to drive Pantheon. The underlying capabilities
// are identical across every surface — one engine, swappable faces. A surface
// is only a front-end; switching surface never changes what Pantheon can do.
type Surface string

const (
	// SurfaceCLI is direct terminal commands (`sirsi scan`, ...). Always present
	// once the binary is installed.
	SurfaceCLI Surface = "cli"
	// SurfaceTUI is the full-screen terminal app.
	SurfaceTUI Surface = "tui"
	// SurfaceIDE is invisible programmatic access for AI IDEs via the MCP server.
	SurfaceIDE Surface = "ide"
	// SurfaceMenubar is the macOS menu bar app. Installed by default on macOS,
	// not part of the opt-in selection.
	SurfaceMenubar Surface = "menubar"
	// SurfaceGUI is the macOS native app (later-phase polish surface).
	SurfaceGUI Surface = "gui"
	// SurfaceSupervisor is the resident Horus agent-router LaunchAgent. Like the
	// menubar it is installed by default on macOS and is not part of the opt-in
	// set; it is not a user-facing surface but reuses InstallResult for a uniform
	// install report.
	SurfaceSupervisor Surface = "supervisor"
	// SurfaceMaatGate is the 𓆄 Ma'at pre-push gate (git core.hooksPath →
	// .githooks). Not a user-facing surface; reuses InstallResult for a uniform
	// install report. Armed only inside a Pantheon source clone (contributors).
	SurfaceMaatGate Surface = "maat-gate"
)

// Selectable returns the surfaces a user opts into at install time
// (multi-select). The menubar is installed by default on macOS and is not
// listed here; the GUI is a later-phase surface.
func Selectable() []Surface {
	return []Surface{SurfaceCLI, SurfaceTUI, SurfaceIDE}
}

// Title is the human label for a surface.
func (s Surface) Title() string {
	switch s {
	case SurfaceCLI:
		return "CLI"
	case SurfaceTUI:
		return "TUI"
	case SurfaceIDE:
		return "IDE (MCP)"
	case SurfaceMenubar:
		return "Menubar"
	case SurfaceGUI:
		return "GUI"
	case SurfaceSupervisor:
		return "Agent routing"
	case SurfaceMaatGate:
		return "Ma'at gate"
	}
	return string(s)
}

// Detail describes what selecting the surface installs/enables.
func (s Surface) Detail() string {
	switch s {
	case SurfaceCLI:
		return "Direct terminal commands — the foundation, always installed"
	case SurfaceTUI:
		return "Full-screen terminal dashboard (launch with `sirsi surface use tui`)"
	case SurfaceIDE:
		return "Invisible access for AI IDEs (Claude, Cursor) via the MCP server"
	case SurfaceMenubar:
		return "macOS menu bar app — installed by default"
	case SurfaceGUI:
		return "macOS native app"
	case SurfaceSupervisor:
		return "Resident Horus agent-router supervisor — installed by default"
	case SurfaceMaatGate:
		return "𓆄 Ma'at pre-push gate — gofmt + vet + golangci-lint + tests before push"
	}
	return ""
}

// InstallResult reports the outcome of enabling one surface.
type InstallResult struct {
	Surface Surface
	Status  Status
	Message string
}

// ── Menubar ────────────────────────────────────────────────────────────────

// menubarPlistLabel is the LaunchAgent label for the menu bar app.
const menubarPlistLabel = "ai.sirsi.pantheon"

// menubarPlistContent renders the LaunchAgent for a known absolute binary path.
//
// It execs the resolved binary directly rather than relying on a login-shell
// `command -v sirsi-menubar` lookup with an /opt/homebrew fallback: that lookup
// fails (exit 127) whenever the binary lives somewhere not on the launchd login
// PATH (e.g. ~/.local/bin) and the hardcoded Homebrew path is absent — the exact
// reason the menubar silently never started. A login shell still wraps the exec
// so the menubar inherits a full PATH for the `sirsi` subprocesses it spawns.
func menubarPlistContent(binPath string) string {
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key>
	<string>ai.sirsi.pantheon</string>
	<key>ProgramArguments</key>
	<array>
		<string>/bin/zsh</string>
		<string>-l</string>
		<string>-c</string>
		<string>exec %q</string>
	</array>
	<key>RunAtLoad</key>
	<true/>
	<key>KeepAlive</key>
	<true/>
	<key>StandardOutPath</key>
	<string>/tmp/sirsi-menubar.log</string>
	<key>StandardErrorPath</key>
	<string>/tmp/sirsi-menubar.err</string>
	<key>ProcessType</key>
	<string>Interactive</string>
</dict>
</plist>
`, binPath)
}

// MenubarBinaryPath returns the resolved path to the sirsi-menubar binary, or
// empty if it is not installed. It looks on PATH and next to the running sirsi
// binary (the release archive ships them side by side).
func MenubarBinaryPath() string {
	if p, err := exec.LookPath("sirsi-menubar"); err == nil {
		return p
	}
	if sibling := filepath.Join(filepath.Dir(BinaryPath()), "sirsi-menubar"); fileExists(sibling) {
		return sibling
	}
	return ""
}

// GUIBinaryPath returns the resolved path to the sirsi-gui binary (the macOS
// GUI surface), or empty if it is not installed. Looks on PATH and next to the
// running sirsi binary.
func GUIBinaryPath() string {
	if p, err := exec.LookPath("sirsi-gui"); err == nil {
		return p
	}
	if sibling := filepath.Join(filepath.Dir(BinaryPath()), "sirsi-gui"); fileExists(sibling) {
		return sibling
	}
	return ""
}

// menubarPlistPath is where the LaunchAgent is written.
func menubarPlistPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "Library", "LaunchAgents", menubarPlistLabel+".plist")
}

// MenubarInstalled reports whether the LaunchAgent is in place.
func MenubarInstalled() bool {
	return runtime.GOOS == "darwin" && fileExists(menubarPlistPath())
}

// InstallMenubar writes the LaunchAgent and loads it so the menu bar app starts
// now and at every login. macOS only.
func InstallMenubar() InstallResult {
	res := InstallResult{Surface: SurfaceMenubar}
	if runtime.GOOS != "darwin" {
		res.Status, res.Message = StatusSkipped, "menubar is macOS only"
		return res
	}
	bin := MenubarBinaryPath()
	if bin == "" {
		res.Status = StatusFailed
		res.Message = "sirsi-menubar binary not found on PATH or beside sirsi"
		return res
	}
	path := menubarPlistPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		res.Status, res.Message = StatusFailed, err.Error()
		return res
	}
	if err := os.WriteFile(path, []byte(menubarPlistContent(bin)), 0o644); err != nil {
		res.Status, res.Message = StatusFailed, err.Error()
		return res
	}
	// Reload: unload first so a re-install picks up changes; ignore unload error
	// (it fails when not yet loaded, which is fine).
	_ = exec.Command("launchctl", "unload", path).Run()
	if err := exec.Command("launchctl", "load", path).Run(); err != nil {
		res.Status = StatusFailed
		res.Message = "wrote LaunchAgent but launchctl load failed: " + err.Error()
		return res
	}
	res.Status, res.Message = StatusOK, "menu bar app installed and started"
	return res
}

// ── IDE (MCP) ──────────────────────────────────────────────────────────────

// MCPConfigSnippet is the canonical MCP server entry IDEs need.
func MCPConfigSnippet() string {
	return fmt.Sprintf(`{ "mcpServers": { "sirsi": { "command": %q, "args": ["mcp"] } } }`, BinaryPath())
}

// claudeCLIAvailable reports whether the Claude Code CLI is installed (the most
// reliable way to register an MCP server, since it owns its own config).
func claudeCLIAvailable() bool {
	_, err := exec.LookPath("claude")
	return err == nil
}

// IDERegistered reports whether the sirsi MCP server is registered with the
// Claude Code CLI. Best-effort: false if the CLI is absent.
func IDERegistered() bool {
	if !claudeCLIAvailable() {
		return false
	}
	out, err := exec.Command("claude", "mcp", "list").Output()
	if err != nil {
		return false
	}
	return strings.Contains(string(out), "sirsi")
}

// RegisterIDE registers the sirsi MCP server so AI IDEs can call Pantheon's
// tools. It uses the Claude Code CLI when present; otherwise it returns a
// result instructing the caller to add MCPConfigSnippet manually.
func RegisterIDE() InstallResult {
	res := InstallResult{Surface: SurfaceIDE}
	if !claudeCLIAvailable() {
		res.Status = StatusMissing
		res.Message = "no `claude` CLI found — add to your IDE: " + MCPConfigSnippet()
		return res
	}
	if IDERegistered() {
		res.Status, res.Message = StatusOK, "MCP server already registered with Claude"
		return res
	}
	// `claude mcp add --scope user <name> <command> [args...]`
	cmd := exec.Command("claude", "mcp", "add", "--scope", "user", "sirsi", BinaryPath(), "mcp")
	if out, err := cmd.CombinedOutput(); err != nil {
		res.Status = StatusFailed
		res.Message = fmt.Sprintf("claude mcp add failed: %v — add manually: %s", strings.TrimSpace(string(out)), MCPConfigSnippet())
		return res
	}
	res.Status, res.Message = StatusOK, "registered sirsi MCP server with Claude (user scope)"
	return res
}

// ── Orchestration ──────────────────────────────────────────────────────────

// Install enables a single selected surface and returns its result. CLI is a
// no-op (the binary being run is the CLI). On macOS the menubar is installed
// separately and unconditionally by the caller.
func Install(s Surface) InstallResult {
	switch s {
	case SurfaceCLI:
		return InstallResult{Surface: SurfaceCLI, Status: StatusOK, Message: "CLI ready (this binary)"}
	case SurfaceTUI:
		// The full-screen TUI is a real runnable app (`sirsi tui`).
		return InstallResult{Surface: SurfaceTUI, Status: StatusOK, Message: "TUI ready (sirsi tui)"}
	case SurfaceIDE:
		return RegisterIDE()
	case SurfaceMenubar:
		return InstallMenubar()
	}
	return InstallResult{Surface: s, Status: StatusSkipped, Message: "unknown surface"}
}

// ── Switching (the swappable face) ─────────────────────────────────────────

// surfacePrefPath is where the user's active-surface preference is recorded.
func surfacePrefPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "sirsi", "surface")
}

// ActiveSurface returns the user's preferred surface, defaulting to CLI.
func ActiveSurface() Surface {
	b, err := os.ReadFile(surfacePrefPath())
	if err != nil {
		return SurfaceCLI
	}
	s := Surface(strings.TrimSpace(string(b)))
	switch s {
	case SurfaceCLI, SurfaceTUI, SurfaceIDE, SurfaceMenubar, SurfaceGUI:
		return s
	}
	return SurfaceCLI
}

// SaveActiveSurface records the user's preferred surface.
func SaveActiveSurface(s Surface) error {
	path := surfacePrefPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(string(s)+"\n"), 0o644)
}

// LaunchSurface brings the given surface to the foreground. The underlying
// capabilities are identical across surfaces; this only changes the face. It
// records the choice as the new preference. CLI is a no-op (the caller is
// already the CLI). Returns a human message describing what happened.
func LaunchSurface(s Surface) (string, error) {
	if err := SaveActiveSurface(s); err != nil {
		return "", err
	}
	switch s {
	case SurfaceCLI:
		return "CLI is the active surface — run `sirsi <command>` directly.", nil
	case SurfaceTUI:
		return "TUI is the active surface — run `sirsi tui` or `sirsi surface use tui`.", nil
	case SurfaceIDE:
		if IDERegistered() {
			return "IDE surface active — Pantheon is reachable from your AI IDE via MCP.", nil
		}
		r := RegisterIDE()
		return r.Message, nil
	case SurfaceGUI:
		if runtime.GOOS != "darwin" {
			return "", fmt.Errorf("the GUI surface is macOS only")
		}
		if bin := GUIBinaryPath(); bin != "" {
			if err := exec.Command(bin).Start(); err != nil {
				return "", fmt.Errorf("launch sirsi-gui: %w", err)
			}
			return "GUI surface launched — the Pantheon window is opening.", nil
		}
		// No GUI binary present; fall back to the menu bar app.
		_ = exec.Command("open", "-a", "Pantheon").Start()
		return "sirsi-gui not installed — opened the menu bar app instead.", nil
	case SurfaceMenubar:
		if runtime.GOOS != "darwin" {
			return "", fmt.Errorf("the menubar surface is macOS only")
		}
		// Ensure it is installed, then it is already running via the LaunchAgent.
		if !MenubarInstalled() {
			if r := InstallMenubar(); r.Status != StatusOK {
				return "", fmt.Errorf("%s", r.Message)
			}
		}
		_ = exec.Command("open", "-a", "Pantheon").Start()
		return "Menubar surface active — look for the Pantheon glyph in your menu bar.", nil
	}
	return "", fmt.Errorf("unknown surface %q", s)
}

// fileExists is a small helper.
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
