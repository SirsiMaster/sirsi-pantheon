package ra

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// SpawnConfig describes how to spawn a terminal window for a Ra scope.
type SpawnConfig struct {
	Name       string // scope name
	Title      string // window title (e.g. "𓇶 Ra: Assiduous")
	WorkDir    string // repo path
	PromptFile string // path to Neith's generated prompt
	LogFile    string // stdout/stderr capture
	ExitFile   string // exit code file
	PIDFile    string // process ID file
	UseITerm2  bool
}

// SpawnResult holds references to the spawned window's tracking files.
type SpawnResult struct {
	Name    string
	PIDFile string
	LogFile string
}

// SpawnWindow opens a new macOS terminal window and runs claude --print
// with the given prompt file, capturing output to the log file.
func SpawnWindow(cfg SpawnConfig) (*SpawnResult, error) {
	// Create parent directories for all tracking files.
	for _, f := range []string{cfg.LogFile, cfg.ExitFile, cfg.PIDFile} {
		dir := filepath.Dir(f)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, fmt.Errorf("ra spawn: create dir %s: %w", dir, err)
		}
	}

	// Build the shell command that records PID, runs claude, and captures exit code.
	shellCmd := fmt.Sprintf(
		"echo $$ > %s && cd %s && claude --print < %s 2>&1 | tee %s; echo $? > %s",
		escapeShell(cfg.PIDFile),
		escapeShell(cfg.WorkDir),
		escapeShell(cfg.PromptFile),
		escapeShell(cfg.LogFile),
		escapeShell(cfg.ExitFile),
	)

	var script string
	if cfg.UseITerm2 {
		script = buildITerm2Script(shellCmd, cfg.Title)
	} else {
		script = buildTerminalScript(shellCmd, cfg.Title)
	}

	cmd := exec.Command("osascript", "-e", script)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("ra spawn: osascript failed: %w", err)
	}

	return &SpawnResult{
		Name:    cfg.Name,
		PIDFile: cfg.PIDFile,
		LogFile: cfg.LogFile,
	}, nil
}

// buildTerminalScript generates AppleScript for macOS Terminal.app.
func buildTerminalScript(shellCmd, title string) string {
	return fmt.Sprintf(`tell application "Terminal"
	activate
	do script "%s"
	set custom title of front window to "%s"
end tell`, escapeAppleScript(shellCmd), escapeAppleScript(title))
}

// buildITerm2Script generates AppleScript for iTerm2.
func buildITerm2Script(shellCmd, title string) string {
	return fmt.Sprintf(`tell application "iTerm"
	activate
	set newWindow to (create window with default profile)
	tell current session of newWindow
		write text "%s"
		set name to "%s"
	end tell
end tell`, escapeAppleScript(shellCmd), escapeAppleScript(title))
}

// KillWindow reads the PID from pidFile and sends SIGTERM to that process.
func KillWindow(pidFile string) error {
	data, err := os.ReadFile(pidFile)
	if err != nil {
		return fmt.Errorf("ra kill: read pid file %s: %w", pidFile, err)
	}

	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return fmt.Errorf("ra kill: parse pid from %s: %w", pidFile, err)
	}

	if err := killProcess(pid); err != nil {
		return fmt.Errorf("ra kill: terminate pid %d: %w", pid, err)
	}

	return nil
}

// KillAll reads all PID files from ~/.config/ra/pids/ and kills each process.
func KillAll(raDir string) error {
	pidsDir := filepath.Join(raDir, "pids")
	entries, err := os.ReadDir(pidsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // nothing to kill
		}
		return fmt.Errorf("ra kill-all: read pids dir: %w", err)
	}

	var errs []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		pidFile := filepath.Join(pidsDir, entry.Name())
		if err := KillWindow(pidFile); err != nil {
			errs = append(errs, err.Error())
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("ra kill-all: %d errors: %s", len(errs), strings.Join(errs, "; "))
	}
	return nil
}

// escapeShell wraps a path in single quotes for safe shell embedding.
func escapeShell(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'"
}

// escapeAppleScript escapes backslashes and double quotes for AppleScript strings.
func escapeAppleScript(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "\"", "\\\"")
	return s
}
