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
			Message:  "direct display-power commands are denied; use Pantheon's custody-gated desktop actuator",
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
	if len(command) == 0 {
		return false
	}
	name := filepath.Base(normalizeCommandToken(command[0]))
	if name == "sh" || name == "bash" || name == "zsh" {
		for i, arg := range command[:len(command)-1] {
			if strings.Contains(normalizeCommandToken(arg), "c") && strings.HasPrefix(normalizeCommandToken(arg), "-") {
				return IsDirectDisplayPowerShell(command[i+1])
			}
		}
	}
	return isDirectDisplayPowerInvocation(command)
}

// IsDirectDisplayPowerShell applies the same narrow check to the command text
// received from Codex/Claude shell tools. It keeps quoted text supplied to
// harmless commands harmless: `printf 'pmset displaysleepnow'` is not an
// invocation and is therefore permitted as a safe activation canary.
func IsDirectDisplayPowerShell(command string) bool {
	for _, invocation := range shellInvocations(command) {
		if isDirectDisplayPowerInvocation(invocation) {
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
func shellInvocations(command string) [][]string {
	var (
		invocations [][]string
		words       []string
		word        strings.Builder
		quote       byte
		escaped     bool
	)
	finishWord := func() {
		if word.Len() != 0 {
			words = append(words, word.String())
			word.Reset()
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
			if quote == '"' && ch == '$' && i+1 < len(command) && command[i+1] == '(' {
				if end := shellClosingParen(command, i+2); end >= 0 {
					invocations = append(invocations, shellInvocations(command[i+2:end])...)
					i = end
					continue
				}
			}
			word.WriteByte(ch)
			continue
		}
		switch ch {
		case '\'', '"':
			quote = ch
		case '$':
			if i+1 < len(command) && command[i+1] == '(' {
				if end := shellClosingParen(command, i+2); end >= 0 {
					invocations = append(invocations, shellInvocations(command[i+2:end])...)
					i = end
					continue
				}
			}
			word.WriteByte(ch)
		case '`':
			if end := strings.IndexByte(command[i+1:], '`'); end >= 0 {
				end += i + 1
				invocations = append(invocations, shellInvocations(command[i+1:end])...)
				i = end
				continue
			}
			word.WriteByte(ch)
		case ' ', '\t', '\r':
			finishWord()
		case ';', '\n', '|', '&':
			finishInvocation()
			if i+1 < len(command) && command[i+1] == ch && (ch == '|' || ch == '&') {
				i++
			}
		default:
			word.WriteByte(ch)
		}
	}
	finishInvocation()
	return invocations
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

func isDirectDisplayPowerInvocation(command []string) bool {
	for len(command) > 0 {
		name := filepath.Base(normalizeCommandToken(command[0]))
		if name == "sh" || name == "bash" || name == "zsh" {
			for i, arg := range command[:len(command)-1] {
				normalized := normalizeCommandToken(arg)
				if strings.HasPrefix(normalized, "-") && strings.Contains(normalized, "c") {
					return IsDirectDisplayPowerShell(strings.Join(command[i+1:], " "))
				}
			}
			return false
		}
		if name == "sudo" || name == "doas" || name == "exec" {
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
