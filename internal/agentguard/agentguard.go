// Package agentguard composes Pantheon's resource and context-safety tools
// into a small preflight layer for AI agent work.
package agentguard

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/guard"
	"github.com/SirsiMaster/sirsi-pantheon/internal/platform"
	"github.com/SirsiMaster/sirsi-pantheon/internal/rtk"
	"github.com/SirsiMaster/sirsi-pantheon/internal/stele"
	"github.com/SirsiMaster/sirsi-pantheon/internal/yield"
)

type Verdict string

const (
	VerdictAllow Verdict = "allow"
	VerdictWarn  Verdict = "warn"
	VerdictBlock Verdict = "block"
)

type Finding struct {
	Severity string `json:"severity"`
	Check    string `json:"check"`
	Message  string `json:"message"`
}

type PreflightOptions struct {
	Command      []string
	Platform     platform.Platform
	LoadProvider yield.LoadProvider
	IgnoreChecks []string
	// HealthProvider yields the system-health report the preflight gates on.
	// Injectable (Rule A16) because guard.DoctorWith reads some system state
	// directly rather than through Platform (thread counts, Spotlight/mds CPU,
	// etc.), so a "healthy" mock Platform alone can't make the preflight
	// deterministic — on a loaded box a live Critical finding would BLOCK an
	// otherwise-safe command and flake the tests. Defaults to guard.DoctorWith.
	HealthProvider func(platform.Platform) (*guard.DoctorReport, error)
}

type Report struct {
	Verdict  Verdict   `json:"verdict"`
	Command  []string  `json:"command,omitempty"`
	Findings []Finding `json:"findings"`
}

type RunOptions struct {
	Command        []string
	Platform       platform.Platform
	LoadProvider   yield.LoadProvider
	IgnoreChecks   []string
	HealthProvider func(platform.Platform) (*guard.DoctorReport, error)
	Timeout        time.Duration
	MaxOutputBytes int
	MaxOutputLines int
	Force          bool
}

type RunResult struct {
	Report        *Report `json:"report"`
	ExitCode      int     `json:"exitCode"`
	Duration      string  `json:"duration"`
	Output        string  `json:"output"`
	OriginalBytes int     `json:"originalBytes"`
	FilteredBytes int     `json:"filteredBytes"`
	Truncated     bool    `json:"truncated"`
}

func Preflight(opts PreflightOptions) *Report {
	report := &Report{Verdict: VerdictAllow, Command: append([]string(nil), opts.Command...)}
	p := opts.Platform
	if p == nil {
		p = platform.Current()
	}

	if opts.LoadProvider != nil {
		if load, err := yield.CheckWith(opts.LoadProvider); err == nil {
			switch load.Verdict {
			case yield.VerdictYield:
				report.add(VerdictWarn, "system-load", fmt.Sprintf("system load is high: %.0f%% of CPU capacity", load.LoadRatio*100))
			case yield.VerdictCaution:
				report.add(VerdictWarn, "system-load", fmt.Sprintf("system load is elevated: %.0f%% of CPU capacity", load.LoadRatio*100))
			}
		}
	} else if yield.ShouldYield() {
		report.add(VerdictWarn, "system-load", "system load is high; heavy agent work should wait or run with tighter budgets")
	}

	ignore := ignoreSet(opts.IgnoreChecks)
	doctorFn := opts.HealthProvider
	if doctorFn == nil {
		doctorFn = guard.DoctorWith
	}
	if doctor, err := doctorFn(p); err == nil {
		for _, f := range doctor.Findings {
			if ignore[f.Check] {
				continue
			}
			switch f.Severity {
			case guard.SeverityCritical:
				report.add(VerdictBlock, f.Check, f.Message)
			case guard.SeverityWarn:
				report.add(VerdictWarn, f.Check, f.Message)
			}
		}
	}

	for _, finding := range AnalyzeCommand(opts.Command) {
		report.addSeverity(finding)
	}

	if len(report.Findings) == 0 {
		report.Findings = append(report.Findings, Finding{
			Severity: string(VerdictAllow),
			Check:    "agent-preflight",
			Message:  "no resource or command-safety blockers detected",
		})
	}
	return report
}

func AnalyzeCommand(command []string) []Finding {
	if len(command) == 0 {
		return nil
	}

	var findings []Finding
	if IsDirectDisplayPowerCommand(command) {
		findings = append(findings, Finding{
			Severity: string(VerdictBlock),
			Check:    "desktop-custody",
			Message:  "direct display-power commands are denied; use the typed sirsi desktop-sleep workflow for governed sleep",
		})
	}
	name := filepath.Base(command[0])
	lowerName := strings.ToLower(name)
	joined := strings.ToLower(strings.Join(command, " "))
	home, _ := os.UserHomeDir()
	devRoot := filepath.Join(home, "Development")

	for _, arg := range command[1:] {
		clean := normalizePathArg(arg, home)
		switch clean {
		case home, devRoot:
			findings = append(findings, Finding{
				Severity: string(VerdictBlock),
				Check:    "scope",
				Message:  fmt.Sprintf("refusing unbounded agent scan over %s; narrow to a repo or file list", clean),
			})
		}
		if strings.Contains(clean, filepath.Join(".codex", "sessions")) && strings.HasSuffix(clean, ".jsonl") {
			severity := VerdictWarn
			if lowerName == "cat" || lowerName == "python" || lowerName == "python3" || lowerName == "rg" || lowerName == "grep" {
				severity = VerdictBlock
			}
			findings = append(findings, Finding{
				Severity: string(severity),
				Check:    "session-log",
				Message:  "Codex JSONL transcripts can contain huge single-line payloads; use bounded ranges or router/Thoth summaries",
			})
		}
	}

	if (lowerName == "python" || lowerName == "python3") && (strings.Contains(joined, "development") || strings.Contains(joined, ".codex/sessions")) {
		findings = append(findings, Finding{
			Severity: string(VerdictBlock),
			Check:    "python-unbounded-analysis",
			Message:  "repo-wide or transcript-wide Python analysis must be replaced with bounded Go/Pantheon primitives or explicit budgets",
		})
	}

	if lowerName == "rg" && !hasAny(command, "--max-count", "-m", "--files") && (strings.Contains(joined, "development") || strings.Contains(joined, ".codex/sessions")) {
		findings = append(findings, Finding{
			Severity: string(VerdictWarn),
			Check:    "output-budget",
			Message:  "recursive search over large roots must use an output budget or a narrower path",
		})
	}

	return findings
}

// IsDirectDisplayPowerCommand recognizes the narrow set of macOS display
// disruption routes that must never run through a generic agent shell. It is
// intentionally a deny list for managed command surfaces, not a claimed shell
// sandbox: arbitrary unmanaged processes remain outside agentguard's scope.
//
// It recognizes the direct invocation forms used by managed tools (absolute
// paths, sudo, and sh/zsh -c). It deliberately does not claim to parse an
// arbitrary shell program; unmanaged shell composition stays outside this
// narrow command-boundary control. A read-only `pmset -g` is not matched.
func IsDirectDisplayPowerCommand(command []string) bool {
	return isDirectDisplayPowerInvocation(command, 0)
}

// IsDirectDisplayPowerShell applies the same narrow check to the command text
// received from Codex/Claude shell tools. It keeps quoted text supplied to
// harmless commands harmless: `printf 'pmset displaysleepnow'` is not an
// invocation and is therefore permitted as a safe activation canary.
func IsDirectDisplayPowerShell(command string) bool {
	return isDirectDisplayPowerShell(command, 0)
}

// Substitutions and nested shell wrappers share this depth bound. Exhaustion
// means unknown command safety, so deny rather than silently omitting a child.
const maxDisplayPowerParseDepth = 32

func isDirectDisplayPowerShell(command string, depth int) bool {
	invocations, complete := shellInvocations(command, depth)
	if !complete {
		return true
	}
	for _, invocation := range invocations {
		if isDirectDisplayPowerInvocation(invocation, depth+1) {
			return true
		}
	}
	return false
}

// shellInvocations is deliberately a small, bounded shell lexer rather than a
// shell evaluator. It identifies executable words separated by the shell
// operators that can start another command, while preserving quoted literals
// as arguments. It also recurses into command substitutions, because those do
// execute commands. The guard does not claim to be a general shell sandbox;
// malformed or exotic shell syntax remains subject to the shell itself.
func shellInvocations(command string, depth int) ([][]string, bool) {
	if depth >= maxDisplayPowerParseDepth {
		return nil, false
	}
	var (
		invocations [][]string
		words       []string
		word        strings.Builder
		wordStarted bool
		quote       byte
		escaped     bool
	)
	finishWord := func() {
		if wordStarted {
			words = append(words, word.String())
			word.Reset()
			wordStarted = false
		}
	}
	finishInvocation := func() {
		finishWord()
		if len(words) != 0 {
			invocations = append(invocations, words)
			words = nil
		}
	}
	for i := 0; i < len(command); i++ {
		ch := command[i]
		if escaped {
			word.WriteByte(ch)
			wordStarted = true
			escaped = false
			continue
		}
		if ch == '\\' && quote != '\'' {
			escaped = true
			continue
		}
		if quote != 0 {
			if ch == quote {
				quote = 0
				continue
			}
			if quote == '"' && ch == '`' {
				if end := shellClosingBacktick(command, i+1); end >= 0 {
					child, complete := shellInvocations(command[i+1:end], depth+1)
					if !complete {
						return nil, false
					}
					invocations = append(invocations, child...)
					i = end
					wordStarted = true
					continue
				}
			}
			if quote == '"' && ch == '$' && i+1 < len(command) && command[i+1] == '(' {
				if end := shellClosingParen(command, i+2); end >= 0 {
					child, complete := shellInvocations(command[i+2:end], depth+1)
					if !complete {
						return nil, false
					}
					invocations = append(invocations, child...)
					i = end
					wordStarted = true
					continue
				}
			}
			word.WriteByte(ch)
			wordStarted = true
			continue
		}
		switch ch {
		case '\'', '"':
			quote = ch
			wordStarted = true
		case '$':
			if i+1 < len(command) && command[i+1] == '(' {
				if end := shellClosingParen(command, i+2); end >= 0 {
					child, complete := shellInvocations(command[i+2:end], depth+1)
					if !complete {
						return nil, false
					}
					invocations = append(invocations, child...)
					i = end
					wordStarted = true
					continue
				}
			}
			word.WriteByte(ch)
			wordStarted = true
		case '`':
			if end := shellClosingBacktick(command, i+1); end >= 0 {
				child, complete := shellInvocations(command[i+1:end], depth+1)
				if !complete {
					return nil, false
				}
				invocations = append(invocations, child...)
				i = end
				wordStarted = true
				continue
			}
			word.WriteByte(ch)
			wordStarted = true
		case ' ', '\t', '\r':
			finishWord()
		case ';', '\n', '|', '&':
			finishInvocation()
			if i+1 < len(command) && command[i+1] == ch && (ch == '|' || ch == '&') {
				i++
			}
		default:
			word.WriteByte(ch)
			wordStarted = true
		}
	}
	finishInvocation()
	return invocations, true
}

// Escaped backticks are literal data, including inside double quotes.
func shellClosingBacktick(command string, start int) int {
	escaped := false
	for i := start; i < len(command); i++ {
		if escaped {
			escaped = false
			continue
		}
		if command[i] == '\\' {
			escaped = true
			continue
		}
		if command[i] == '`' {
			return i
		}
	}
	return -1
}

func shellClosingParen(command string, start int) int {
	depth := 1
	var quote byte
	escaped := false
	for i := start; i < len(command); i++ {
		ch := command[i]
		if escaped {
			escaped = false
			continue
		}
		if ch == '\\' && quote != '\'' {
			escaped = true
			continue
		}
		if quote != 0 {
			if ch == quote {
				quote = 0
			}
			continue
		}
		if ch == '\'' || ch == '"' {
			quote = ch
			continue
		}
		switch ch {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

func isDirectDisplayPowerInvocation(command []string, depth int) bool {
	if depth >= maxDisplayPowerParseDepth {
		return true
	}
	for len(command) > 0 {
		name := filepath.Base(normalizeCommandToken(command[0]))
		if name == "sh" || name == "bash" || name == "zsh" {
			if script, ok := shellCommandString(name, command); ok {
				// Only this operand is shell code. Later operands bind $0,
				// $1, etc.; joining them invents executable command text.
				return isDirectDisplayPowerShell(script, depth+1)
			}
			return false
		}
		// Shell assignment words and ordinary execution prefixes preserve the
		// command's identity. This does not evaluate variable expansion.
		if isShellAssignment(command[0]) {
			command = command[1:]
			continue
		}
		if name == "env" {
			command = command[1:]
			for len(command) > 0 {
				arg := command[0]
				if arg == "--" {
					command = command[1:]
					break
				}
				if isShellAssignment(arg) {
					command = command[1:]
					continue
				}
				if !strings.HasPrefix(arg, "-") {
					break
				}
				command = command[1:]
				if (arg == "-u" || arg == "--unset" || arg == "-C" || arg == "--chdir") && len(command) > 0 {
					command = command[1:]
				}
			}
			continue
		}
		if name == "command" {
			command = command[1:]
			for len(command) > 0 && strings.HasPrefix(command[0], "-") {
				arg := command[0]
				// -v/-V query command resolution and do not execute it.
				if strings.ContainsAny(arg, "vV") {
					return false
				}
				command = command[1:]
				if arg == "--" {
					break
				}
			}
			continue
		}
		if name == "exec" {
			command = command[1:]
			for len(command) > 0 && strings.HasPrefix(command[0], "-") {
				option := command[0]
				command = command[1:]
				if option == "--" {
					break
				}
				// exec -a supplies argv[0], not a command to inspect.
				// Unlike sudo/doas, exec's -a consumes the following name.
				if option == "-a" && len(command) > 0 {
					command = command[1:]
				}
			}
			continue
		}
		if name == "sudo" || name == "doas" {
			command = command[1:]
			for len(command) > 0 && strings.HasPrefix(normalizeCommandToken(command[0]), "-") {
				option := normalizeCommandToken(command[0])
				command = command[1:]
				if (option == "-u" || option == "-g" || option == "-h" || option == "-p" || option == "-r" || option == "-t") && len(command) > 0 {
					command = command[1:]
				}
			}
			continue
		}
		switch name {
		case "pmset":
			for _, arg := range command[1:] {
				switch normalizeCommandToken(arg) {
				case "displaysleepnow", "sleepnow", "lock", "displaysleep":
					return true
				}
			}
		case "cgsession":
			for _, arg := range command[1:] {
				if normalizeCommandToken(arg) == "-suspend" {
					return true
				}
			}
		}
		return false
	}
	return false
}

// shellCommandString selects the command operand using the supported shell's
// option arities. It is an argv selector, not a shell parser.
func shellCommandString(shell string, command []string) (string, bool) {
	if len(command) < 2 {
		return "", false
	}
	args := command[1:]
	for len(args) > 0 {
		arg := args[0]
		args = args[1:]
		if arg == "--" || arg == "-" {
			return "", false
		}
		if strings.HasPrefix(arg, "--") {
			switch {
			case shell == "zsh" && arg == "--emulate":
				if len(args) == 0 {
					return "", false
				}
				// Zsh requires a separate mode argument before its other
				// invocation options. The selector keeps using Zsh's argv
				// option arities after the mode has been consumed.
				args = args[1:]
			case arg == "--rcfile", arg == "--init-file":
				if shell != "bash" || len(args) == 0 {
					return "", false
				}
				args = args[1:]
			default:
				// Long options are not short-option bundles, even if their name
				// contains the letter c (for example, --norc).
			}
			continue
		}
		if strings.HasPrefix(arg, "-") || (strings.HasPrefix(arg, "+") && len(arg) > 1) {
			prefix := arg[0]
			options := []rune(arg[1:])
			for i, option := range options {
				if option == 'c' && prefix == '-' {
					if len(args) == 0 {
						return "", false
					}
					return args[0], true
				}
				if option == 'o' {
					if shell == "bash" {
						args = consumeOptionalShellOptionName(options, i, args)
					} else {
						args = consumeRequiredShellOptionName(options, i, args)
					}
					break
				}
				if option == 'O' {
					switch shell {
					case "bash":
						args = consumeOptionalShellOptionName(options, i, args)
					case "zsh":
						// Zsh -O/+O are single-letter flags and take no name.
						continue
					default:
						// Uppercase O is outside the selected sh option contract.
						continue
					}
					break
				}
			}
			continue
		}
		// The first non-option is a script operand; later argv entries are
		// script arguments, not shell options or command text.
		return "", false
	}
	return "", false
}

func consumeRequiredShellOptionName(options []rune, index int, args []string) []string {
	if index+1 < len(options) || len(args) == 0 {
		return args
	}
	return args[1:]
}

func consumeOptionalShellOptionName(options []rune, index int, args []string) []string {
	if index+1 < len(options) || len(args) == 0 || isShellOption(args[0]) {
		return args
	}
	return args[1:]
}

func isShellOption(arg string) bool {
	return arg == "--" || arg == "-" || strings.HasPrefix(arg, "-") || strings.HasPrefix(arg, "+")
}

func isShellAssignment(word string) bool {
	name, _, ok := strings.Cut(word, "=")
	if !ok || name == "" {
		return false
	}
	for i, ch := range name {
		if ch != '_' && (ch < 'a' || ch > 'z') && (ch < 'A' || ch > 'Z') && (i == 0 || ch < '0' || ch > '9') {
			return false
		}
	}
	return true
}

func normalizeCommandToken(value string) string {
	return strings.ToLower(strings.Trim(value, " \t\n\r'\";()[]{}"))
}

func SafeRun(ctx context.Context, opts RunOptions) (*RunResult, error) {
	if len(opts.Command) == 0 {
		return nil, errors.New("command is required")
	}
	if opts.Timeout <= 0 {
		opts.Timeout = 2 * time.Minute
	}
	if opts.MaxOutputBytes <= 0 {
		opts.MaxOutputBytes = 512 * 1024
	}
	if opts.MaxOutputLines <= 0 {
		opts.MaxOutputLines = 400
	}

	report := Preflight(PreflightOptions{
		Command:        opts.Command,
		Platform:       opts.Platform,
		LoadProvider:   opts.LoadProvider,
		IgnoreChecks:   opts.IgnoreChecks,
		HealthProvider: opts.HealthProvider,
	})
	if report.Verdict == VerdictBlock && (!opts.Force || hasNonBypassableBlock(report.Findings)) {
		return &RunResult{Report: report, ExitCode: 126}, fmt.Errorf("agent safety preflight blocked command")
	}

	runCtx, cancel := context.WithTimeout(ctx, opts.Timeout)
	defer cancel()

	start := time.Now()
	lim := &limitedBuffer{limit: opts.MaxOutputBytes}
	cmd := exec.CommandContext(runCtx, opts.Command[0], opts.Command[1:]...)
	cmd.Stdout = lim
	cmd.Stderr = lim
	err := cmd.Run()
	duration := time.Since(start)

	exitCode := 0
	if err != nil {
		exitCode = 1
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			exitCode = exitErr.ExitCode()
		}
		if runCtx.Err() == context.DeadlineExceeded {
			exitCode = 124
		}
	}

	cfg := rtk.DefaultConfig()
	cfg.MaxBytes = opts.MaxOutputBytes
	cfg.MaxLines = opts.MaxOutputLines
	filtered := rtk.New(cfg).Apply(lim.String())

	// This is the path RTK actually runs on — every guarded command an agent
	// executes. It was computing the saving and dropping it, so the surface
	// built to report savings had nothing to report. Same field names as the
	// MCP filter_output handler so one aggregate reads both.
	stele.Inscribe("rtk", stele.TypeRTKFilter, "", map[string]string{
		"original_bytes": strconv.Itoa(lim.written),
		"filtered_bytes": strconv.Itoa(filtered.FilteredBytes),
		"ratio":          fmt.Sprintf("%.2f", filtered.Ratio),
		"dupes":          strconv.Itoa(filtered.DupsCollapsed),
	})

	return &RunResult{
		Report:        report,
		ExitCode:      exitCode,
		Duration:      duration.Round(time.Millisecond).String(),
		Output:        filtered.Output,
		OriginalBytes: lim.written,
		FilteredBytes: filtered.FilteredBytes,
		Truncated:     lim.truncated || filtered.Truncated,
	}, err
}

func hasNonBypassableBlock(findings []Finding) bool {
	for _, finding := range findings {
		if finding.Severity == string(VerdictBlock) && finding.Check == "desktop-custody" {
			return true
		}
	}
	return false
}

func (r *Report) add(verdict Verdict, check, message string) {
	r.addSeverity(Finding{Severity: string(verdict), Check: check, Message: message})
}

func (r *Report) addSeverity(f Finding) {
	r.Findings = append(r.Findings, f)
	switch Verdict(f.Severity) {
	case VerdictBlock:
		r.Verdict = VerdictBlock
	case VerdictWarn:
		if r.Verdict != VerdictBlock {
			r.Verdict = VerdictWarn
		}
	}
}

func normalizePathArg(arg, home string) string {
	arg = strings.Trim(arg, `"'`)
	arg = strings.TrimPrefix(arg, "file://")
	if arg == "~" {
		return home
	}
	if strings.HasPrefix(arg, "~/") {
		arg = filepath.Join(home, strings.TrimPrefix(arg, "~/"))
	}
	return filepath.Clean(arg)
}

func hasAny(values []string, needles ...string) bool {
	for _, v := range values {
		for _, n := range needles {
			if v == n {
				return true
			}
		}
	}
	return false
}

func ignoreSet(values []string) map[string]bool {
	out := make(map[string]bool, len(values))
	for _, v := range values {
		out[v] = true
	}
	return out
}

type limitedBuffer struct {
	buf       bytes.Buffer
	limit     int
	written   int
	truncated bool
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	b.written += len(p)
	remaining := b.limit - b.buf.Len()
	if remaining <= 0 {
		b.truncated = true
		return len(p), nil
	}
	if len(p) > remaining {
		b.buf.Write(p[:remaining])
		b.truncated = true
		return len(p), nil
	}
	_, _ = b.buf.Write(p)
	return len(p), nil
}

func (b *limitedBuffer) String() string {
	if b == nil {
		return ""
	}
	var out strings.Builder
	_, _ = io.Copy(&out, bytes.NewReader(b.buf.Bytes()))
	if b.truncated {
		out.WriteString("\n[output truncated by sirsi agent safe-run]\n")
	}
	return out.String()
}
