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
	if report.Verdict == VerdictBlock && !opts.Force {
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
