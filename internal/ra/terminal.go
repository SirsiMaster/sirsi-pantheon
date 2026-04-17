package ra

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// ProtectGlyph is the Eye of Horus — a sentinel stamped into the custom title
// of any Terminal.app window that must survive KillAll. Ra checks for this
// glyph before closing windows; if present, the window is untouchable.
const ProtectGlyph = "𓂀"

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
	BudgetUSD  float64 // max API spend per agent (--max-budget-usd) — API users only
	Sprints    int     // number of sprint turns (1 = single shot, N = loop with --continue)
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

	// Build the claude command with full autonomy flags.
	// --dangerously-skip-permissions: bypass all permission prompts (scope is pre-approved by Ra)
	// --allowedTools: whitelist the tools the agent needs
	// --print: non-interactive mode (skips workspace trust dialog, reads prompt from stdin)
	// --output-format stream-json --verbose: stream JSON events in real-time
	// --name: session name for /resume identification
	//
	// Sprint loop: claude --print runs one conversation turn, then exits.
	// To work through an entire phase, we loop N sprints:
	//   Sprint 1: claude --print < prompt     (initial scope from Neith)
	//   Sprint 2+: claude --continue --print  (resume same session, keep context)
	// Each sprint picks up where the last left off. The agent has full
	// conversation history and continues working through the canon's plan.
	claudeBase := fmt.Sprintf(
		"--dangerously-skip-permissions --allowedTools \"Bash Edit Write Read Glob Grep Agent\" --name \"Ra: %s\" --output-format stream-json --verbose",
		cfg.Name,
	)

	sprints := cfg.Sprints
	if sprints < 1 {
		sprints = 1
	}

	// Python stream filter — reads stream-json from claude, writes:
	//   1. Human-readable text to stdout (terminal) and log file
	//   2. Hash-chained Stele entries to stele.jsonl (ADR-014)
	//
	// The Stele is the single source of truth. Every deity and agent writes to it.
	// The Command Center and other deities read from it via mmap'd offset tracking.
	stelePath := filepath.Join(filepath.Dir(cfg.LogFile), "..", "stele.jsonl")
	streamFilter := fmt.Sprintf(
		`python3 -u -c "
import sys, json, time, hashlib
SCOPE = %s
DEITY = 'agent:' + SCOPE
STELE = %s
log = open(%s, 'a', buffering=1)

# Read last hash from Stele for chain continuity
prev_hash = '0' * 64
seq = 0
try:
    with open(STELE, 'r') as f:
        for line in f:
            line = line.strip()
            if line:
                try:
                    e = json.loads(line)
                    prev_hash = e.get('hash', prev_hash)
                    seq = e.get('seq', seq) + 1
                except:
                    pass
except FileNotFoundError:
    pass

def compute_hash(entry):
    e = dict(entry)
    e['hash'] = ''
    return hashlib.sha256(json.dumps(e, sort_keys=True).encode()).hexdigest()

def inscribe(etype, data=None):
    global prev_hash, seq
    entry = {
        'seq': seq, 'prev': prev_hash, 'deity': DEITY,
        'type': etype, 'scope': SCOPE,
        'data': data or {}, 'ts': time.strftime('%%Y-%%m-%%dT%%H:%%M:%%S'),
        'hash': ''
    }
    entry['hash'] = compute_hash(entry)
    with open(STELE, 'a') as f:
        f.write(json.dumps(entry) + '\n')
    prev_hash = entry['hash']
    seq += 1

def out(s):
    print(s, flush=True)
    log.write(s + '\n')

inscribe('sprint_start', {'sprint': '1', 'sprints': str(%d)})
for line in sys.stdin:
    line = line.strip()
    if not line:
        continue
    try:
        ev = json.loads(line)
    except:
        continue
    t = ev.get('type','')
    if t == 'assistant':
        msg = ev.get('message',{})
        for c in msg.get('content',[]):
            if c.get('type') == 'text':
                txt = c['text']
                out(txt)
                inscribe('text', {'text': txt[:200]})
            elif c.get('type') == 'tool_use':
                name = c.get('name','?')
                out('[tool: ' + name + ']')
                inscribe('tool_use', {'tool': name})
    elif t == 'result':
        out('--- sprint complete (exit: ' + ev.get('stop_reason','?') + ') ---')
        inscribe('sprint_end', {'sprint': '1', 'sprints': str(%d)})
log.close()
"`, escapeShell(cfg.Name), escapeShell(stelePath), escapeShell(cfg.LogFile), sprints, sprints)

	// Build the sprint loop shell command.
	//
	// The governance loop between sprints:
	//   1. Agent works (claude --print)
	//   2. Thoth compact — compresses session memory, frees context budget
	//   3. Ma'at check — verifies build and tests pass
	//   4. Seshat scribe — logs sprint results to .thoth/
	//   5. Feed governance report as next sprint's prompt via --continue
	//
	// This keeps the context window healthy and ensures quality gates
	// between each sprint. If Ma'at fails (build broken), the next sprint
	// gets told to fix it before continuing.

	// Governance script runs between sprints. Outputs a one-line report
	// that becomes the next sprint's prompt.
	governanceScript := fmt.Sprintf(`
SPRINT_NUM=$1
TOTAL_SPRINTS=$2
SCOPE=%s
STELE=%s

# Inscribe a hash-chained entry to the Stele (ADR-014).
# Uses python for SHA-256 since bash doesn't have native hashing on all systems.
inscribe() {
  python3 -c "
import json, hashlib, sys
stele_path = sys.argv[1]
deity = sys.argv[2]
etype = sys.argv[3]
scope = sys.argv[4]
data_json = sys.argv[5]

prev_hash = '0' * 64
seq = 0
try:
    with open(stele_path, 'r') as f:
        for line in f:
            line = line.strip()
            if line:
                try:
                    e = json.loads(line)
                    prev_hash = e.get('hash', prev_hash)
                    seq = e.get('seq', seq) + 1
                except:
                    pass
except FileNotFoundError:
    pass

import time
entry = {
    'seq': seq, 'prev': prev_hash, 'deity': deity,
    'type': etype, 'scope': scope,
    'data': json.loads(data_json),
    'ts': time.strftime('%%Y-%%m-%%dT%%H:%%M:%%S'),
    'hash': ''
}
e2 = dict(entry); e2['hash'] = ''
entry['hash'] = hashlib.sha256(json.dumps(e2, sort_keys=True).encode()).hexdigest()
with open(stele_path, 'a') as f:
    f.write(json.dumps(entry) + '\n')
" "$STELE" "$1" "$2" "$SCOPE" "$3"
}

# Thoth compact
if command -v pantheon >/dev/null 2>&1; then
  pantheon thoth compact . 2>/dev/null
  THOTH="compacted"
elif [ -f "$HOME/go/bin/pantheon" ]; then
  "$HOME/go/bin/pantheon" thoth compact . 2>/dev/null
  THOTH="compacted"
else
  THOTH="skipped"
fi

# Ma'at check
if [ -f "go.mod" ]; then
  if go build ./... 2>/dev/null && go test ./... 2>/dev/null; then
    MAAT="PASS"
    MAAT_MSG="PASS (build clean, tests pass)"
  else
    MAAT="FAIL"
    MAAT_MSG="FAIL (build or tests broken — fix before continuing)"
  fi
elif [ -f "package.json" ]; then
  if npm run build 2>/dev/null; then
    MAAT="PASS"
    MAAT_MSG="PASS (build clean)"
  else
    MAAT="FAIL"
    MAAT_MSG="FAIL (build broken — fix before continuing)"
  fi
else
  MAAT="skipped"
  MAAT_MSG="skipped (no build system detected)"
fi

# Inscribe governance results to the Stele
inscribe "maat" "governance" "{\"maat\":\"$MAAT\",\"thoth\":\"$THOTH\",\"sprint\":\"$SPRINT_NUM\",\"sprints\":\"$TOTAL_SPRINTS\"}"
inscribe "agent:$SCOPE" "sprint_start" "{\"sprint\":\"$SPRINT_NUM\",\"sprints\":\"$TOTAL_SPRINTS\"}"

echo "Sprint ${SPRINT_NUM}/${TOTAL_SPRINTS}. Thoth: ${THOTH}. Ma'at: ${MAAT_MSG}. Continue working through the canon — pick up the next incomplete phase. Do not repeat work from previous sprints."
`, cfg.Name, stelePath)

	// Write governance script to a temp file
	govScriptPath := cfg.LogFile + ".gov.sh"

	// Sprint 1: initial scope from Neith
	shellCmd := fmt.Sprintf(
		`echo $$ > %s && cd %s && > %s && cat > %s << 'GOVEOF'
%sGOVEOF
chmod +x %s && claude %s --print < %s 2>/dev/null | %s`,
		escapeShell(cfg.PIDFile),
		escapeShell(cfg.WorkDir),
		escapeShell(cfg.LogFile),
		escapeShell(govScriptPath),
		governanceScript,
		escapeShell(govScriptPath),
		claudeBase,
		escapeShell(cfg.PromptFile),
		streamFilter,
	)

	// Sprints 2+: governance loop → continue
	for i := 2; i <= sprints; i++ {
		shellCmd += fmt.Sprintf(
			` && echo '--- governance: sprint %d/%d ---' >> %s && GOV_MSG=$(sh %s %d %d) && echo "$GOV_MSG" >> %s && claude %s --continue --print "$GOV_MSG" 2>/dev/null | %s`,
			i, sprints,
			escapeShell(cfg.LogFile),
			escapeShell(govScriptPath), i, sprints,
			escapeShell(cfg.LogFile),
			claudeBase,
			streamFilter,
		)
	}

	// Cleanup and capture exit code
	shellCmd += fmt.Sprintf("; rm -f %s; echo $? > %s",
		escapeShell(govScriptPath),
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
// Sets the window to auto-close when the shell exits so KillAll doesn't
// leave orphaned windows that the user has to close manually.
func buildTerminalScript(shellCmd, title string) string {
	return fmt.Sprintf(`tell application "Terminal"
	activate
	do script "%s; exit"
	delay 0.5
	tell front window
		set custom title to "%s"
		set current settings to settings set "Basic"
	end tell
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

	// Close Terminal.app windows left by killed Ra agents.
	// Killing PIDs stops the agent but the shell window stays open.
	// We send `exit` to each window via AppleScript, then close it.
	//
	// Protection: any window whose custom title contains the ProtectGlyph (𓂀)
	// is immune. Ra stamps this on the Claude Code session and Command Center
	// before spawning agents. Everything else matching Ra patterns gets closed.
	closeScript := fmt.Sprintf(`tell application "Terminal"
		set windowCount to count of windows
		repeat with i from windowCount to 1 by -1
			try
				set w to window i
				set cTitle to custom title of w
				-- Protected: skip any window bearing the Eye of Horus
				if cTitle contains "%s" then
					-- inoculated, do not touch
				else
					-- Check both custom title and window name for Ra agent patterns
					set wName to name of w
					if cTitle contains "Ra:" or cTitle contains "Ra " or wName contains "Ra:" or wName contains "Ra " then
						do script "exit" in w
						delay 0.3
						close w saving no
					end if
				end if
			end try
		end repeat
	end tell`, ProtectGlyph)
	_ = exec.Command("osascript", "-e", closeScript).Run()

	if len(errs) > 0 {
		return fmt.Errorf("ra kill-all: %d errors: %s", len(errs), strings.Join(errs, "; "))
	}
	return nil
}

// ProtectFrontWindow stamps the frontmost Terminal.app window with the
// ProtectGlyph, making it immune to KillAll. Call this before spawning
// agent windows so the user's Claude Code session is inoculated.
func ProtectFrontWindow() {
	script := fmt.Sprintf(`tell application "Terminal"
		try
			set custom title of front window to "%s " & (custom title of front window)
		on error
			-- Front window has no custom title yet; set one.
			set custom title of front window to "%s Claude Code"
		end try
	end tell`, ProtectGlyph, ProtectGlyph)
	_ = exec.Command("osascript", "-e", script).Run()
}

// isWatchRunning checks if a Ra Command Center process is already running.
func isWatchRunning() bool {
	out, err := exec.Command("pgrep", "-f", "sirsi ra watch").Output()
	return err == nil && len(strings.TrimSpace(string(out))) > 0
}

// SpawnWatchWindow opens a new terminal window running `sirsi ra watch`.
// Kills any existing Command Center window first to prevent duplicates.
func SpawnWatchWindow(useITerm2 bool) {
	// Kill existing Command Center window if running
	killScript := `tell application "Terminal"
		repeat with w in every window
			try
				if name of w contains "Ra Command Center" then
					close w saving no
				end if
			end try
		end repeat
	end tell`
	_ = exec.Command("osascript", "-e", killScript).Run()

	// Find the sirsi binary
	sirsiBin := "sirsi"
	if p, err := exec.LookPath("sirsi"); err == nil {
		sirsiBin = p
	} else {
		home, _ := os.UserHomeDir()
		goPath := filepath.Join(home, "go", "bin", "sirsi")
		if _, err := os.Stat(goPath); err == nil {
			sirsiBin = goPath
		}
	}

	shellCmd := fmt.Sprintf("%s ra watch", escapeShell(sirsiBin))
	title := ProtectGlyph + " 𓇶 Ra Command Center"

	var script string
	if useITerm2 {
		script = buildITerm2Script(shellCmd, title)
	} else {
		script = buildTerminalScript(shellCmd, title)
	}

	cmd := exec.Command("osascript", "-e", script)
	_ = cmd.Run()
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
