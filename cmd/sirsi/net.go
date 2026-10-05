package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/SirsiMaster/sirsi-pantheon/internal/output"
	"github.com/SirsiMaster/sirsi-pantheon/internal/suggest"
)

var netCmd = &cobra.Command{
	Use:     "net",
	Aliases: []string{"neith"},
	Short:   "𓁯 Net — Build-log alignment and repository checks",
	Long: `𓁯 Net — Scope Weaver & Plan Alignment

Net reports build-log alignment only when a recorded session plan exists, and
runs repository quality checks against the current Go module.

  sirsi net status    Report whether alignment can be measured
  sirsi net align     Run go vet, build, and gofmt checks`,
	Run: func(cmd *cobra.Command, args []string) {
		_ = cmd.Help()
	},
}

var netStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Check plan alignment against build logs",
	RunE:  runNetStatus,
}

var netAlignCmd = &cobra.Command{
	Use:   "align",
	Short: "Run Go vet, build, and formatting checks",
	RunE:  runNetAlign,
}

type netCheckCommandRunner func(context.Context, string, string, ...string) ([]byte, error)

type netCheckResult struct {
	name   string
	output []byte
	err    error
}

func runNetRepositoryChecks(ctx context.Context, root string, run netCheckCommandRunner) ([]netCheckResult, error) {
	if ctx == nil {
		return nil, fmt.Errorf("net align: context is required")
	}
	if strings.TrimSpace(root) == "" || run == nil {
		return nil, fmt.Errorf("net align: repository root and command runner are required")
	}
	commands := []struct {
		name   string
		binary string
		args   []string
	}{
		{name: "go vet", binary: "go", args: []string{"vet", "./..."}},
		{name: "go build", binary: "go", args: []string{"build", "./..."}},
		{name: "gofmt", binary: "gofmt", args: []string{"-l", "./internal/", "./cmd/"}},
	}
	results := make([]netCheckResult, 0, len(commands))
	for _, command := range commands {
		if err := ctx.Err(); err != nil {
			return results, fmt.Errorf("net align cancelled before %s: %w", command.name, err)
		}
		out, err := run(ctx, root, command.binary, command.args...)
		results = append(results, netCheckResult{name: command.name, output: out, err: err})
		if ctxErr := ctx.Err(); ctxErr != nil {
			return results, fmt.Errorf("net align cancelled during %s: %w", command.name, ctxErr)
		}
	}
	return results, nil
}

func (r netCheckResult) passed() bool {
	if r.err != nil {
		return false
	}
	return r.name != "gofmt" || len(bytes.TrimSpace(r.output)) == 0
}

func init() {
	netCmd.AddCommand(netStatusCmd)
	netCmd.AddCommand(netAlignCmd)
}

// findGoRepoRoot walks up from the working directory to the nearest
// directory containing go.mod. Repo-scoped verbs (net status/align, maat
// audit) MUST anchor here: the menubar and other surfaces shell these verbs
// from $HOME, and running `go vet`/log lookups against the user's home
// directory produced fabricated failures on an owner-facing surface
// (2026-07-05 popover: "0.0% DRIFTING", "go vet failed" — against $HOME).
func findGoRepoRoot() (string, bool) {
	dir, err := os.Getwd()
	if err != nil {
		return "", false
	}
	for {
		if _, statErr := os.Stat(filepath.Join(dir, "go.mod")); statErr == nil {
			return dir, true
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", false
		}
		dir = parent
	}
}

// findGitRepoRoot walks up for a .git entry (dir OR file — worktrees use a
// .git file). Lets callers tell "not a git repo at all" apart from "a git repo
// that just isn't a Go module" so the message can be accurate (a JS/web repo
// like assiduous IS a code repository; maat simply can't weigh Go coverage there).
func findGitRepoRoot() (string, bool) {
	dir, err := os.Getwd()
	if err != nil {
		return "", false
	}
	for {
		if _, statErr := os.Stat(filepath.Join(dir, ".git")); statErr == nil {
			return dir, true
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", false
		}
		dir = parent
	}
}

func inspectBuildLog(path string) (sections int, modified time.Time, err error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, time.Time{}, err
	}
	info, statErr := f.Stat()
	if statErr != nil {
		_ = f.Close()
		return 0, time.Time{}, statErr
	}
	if !info.Mode().IsRegular() {
		_ = f.Close()
		return 0, time.Time{}, fmt.Errorf("build log is not a regular file")
	}
	data, readErr := io.ReadAll(f)
	closeErr := f.Close()
	if readErr != nil {
		return 0, time.Time{}, readErr
	}
	if closeErr != nil {
		return 0, time.Time{}, closeErr
	}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "## ") {
			sections++
		}
	}
	return sections, info.ModTime(), nil
}

// runNetStatus reports plan-alignment state HONESTLY (Rule A14: no number
// that cannot be independently verified). The previous implementation scored
// a hardcoded three-item demo plan against whatever BUILD_LOG.md happened to
// be in the cwd, promised "1.0" in a warning, and rendered "0.0% DRIFTING"
// in the same breath. There is no recorded session plan to align against
// yet, so NO score is emitted — the build log's presence and freshness are
// reported instead, and the output says exactly what would make alignment
// measurable. Emits the CommandResult contract every surface renders.
func runNetStatus(cmd *cobra.Command, args []string) error {
	start := time.Now()
	res := &output.CommandResult{Command: "sirsi net status", BriefTitle: "Plan Alignment"}

	root, inRepo := findGoRepoRoot()
	if !inRepo {
		res.Summary = "Not inside a code repository — plan alignment has nothing to weigh here."
		res.Status = "unmeasured"
		res.NextActions = append(res.NextActions, output.NextAction{
			Label:       "Run from a repository",
			Command:     "cd <your-repo> && sirsi net status",
			Description: "Plan alignment reads a repo's build log; run it from a repo root.",
		})
		res.Duration = time.Since(start)
		res.Render()
		return nil
	}

	logPath := ""
	var lookupErr error
	for _, rel := range []string{"docs/BUILD_LOG.md", "BUILD_LOG.md"} {
		if _, err := os.Stat(filepath.Join(root, rel)); err == nil {
			logPath = rel
			break
		} else if !os.IsNotExist(err) {
			logPath, lookupErr = rel, err
			break
		}
	}
	res.AddEvidence("Repository", root)
	if lookupErr != nil {
		res.Summary = "Build log status is unavailable because its path could not be inspected; alignment remains unmeasured."
		res.Status = "unavailable"
		res.AddEvidence("Build log", logPath)
		res.AddEvidence("Path error", lookupErr.Error())
	} else if logPath == "" {
		res.Summary = "This repository has no build log yet — alignment is not measurable, so no score is shown."
		res.Status = "unmeasured"
		res.AddEvidence("Build log", "not found (looked for docs/BUILD_LOG.md and BUILD_LOG.md)")
	} else {
		full := filepath.Join(root, logPath)
		sections, modified, readErr := inspectBuildLog(full)
		res.AddEvidence("Build log", logPath)
		if readErr != nil {
			res.Summary = "Build log status is unavailable because the file could not be read; alignment remains unmeasured."
			res.Status = "unavailable"
			res.AddEvidence("Read error", readErr.Error())
		} else {
			res.Summary = fmt.Sprintf("Build log read (%d sections) — alignment scoring needs a recorded session plan, and none is recorded yet, so no score is shown.", sections)
			res.Status = "unmeasured"
			res.AddEvidence("Sections", fmt.Sprintf("%d", sections))
			res.AddEvidence("Last updated", modified.Format("Jan 2, 2006"))
		}
	}
	res.NextActions = append(res.NextActions,
		output.NextAction{Label: "Run repository checks", Command: "sirsi net align", Description: "Run go vet, go build, and gofmt checks against this repository."},
		output.NextAction{Label: "Run governance quality check", Command: "sirsi maat audit", Description: "Ma'at weighs coverage, canon, and pipeline health."},
	)
	res.Duration = time.Since(start)
	res.Render()
	return nil
}

func runNetAlign(cmd *cobra.Command, args []string) error {
	start := time.Now()

	// Anchor to the repo root — these are repo checks, and surfaces shell
	// this verb from $HOME (see findGoRepoRoot).
	root, inRepo := findGoRepoRoot()
	if !inRepo {
		res := &output.CommandResult{
			Command:    "sirsi net align",
			BriefTitle: "Repository Checks",
			Summary:    "Not inside a code repository — nothing to check here.",
			Status:     "unmeasured",
			Duration:   time.Since(start),
		}
		res.NextActions = append(res.NextActions, output.NextAction{
			Label:       "Run from a repository",
			Command:     "cd <your-repo> && sirsi net align",
			Description: "go vet, go build, and gofmt checks run against a Go repository root.",
		})
		res.Render()
		return nil
	}

	output.Banner()
	output.Header("Repository Checks")

	// Run only the checks this command actually measures. Network/security
	// health and other module state are intentionally not inferred from these
	// repository checks.
	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}
	runCommand := func(ctx context.Context, root, name string, args ...string) ([]byte, error) {
		c := exec.CommandContext(ctx, name, args...)
		c.Dir = root
		return c.CombinedOutput()
	}
	checks, checksErr := runNetRepositoryChecks(ctx, root, runCommand)
	failed := 0
	for _, check := range checks {
		if check.passed() {
			switch check.name {
			case "go vet":
				output.Success("Ma'at: go vet passes")
			case "go build":
				output.Success("Go package build passes")
			case "gofmt":
				output.Success("gofmt check passes")
			}
			continue
		}
		failed++
		if check.err != nil {
			output.Error("%s failed: %v", check.name, check.err)
		}
		if detail := strings.TrimSpace(string(check.output)); detail != "" {
			output.Error("%s", detail)
		}
		if check.name == "gofmt" && check.err == nil {
			output.Error("gofmt check found files requiring formatting")
		}
	}

	output.Info("Network/security health is not measured by net align.")

	fmt.Println()
	if checksErr != nil {
		output.Error("Repository checks stopped: %v", checksErr)
		output.Footer(time.Since(start))
		return checksErr
	}
	if failed != 0 {
		output.Error("Repository consistency checks failed (%d check(s)).", failed)
		output.Footer(time.Since(start))
		return fmt.Errorf("net align: %d repository consistency check(s) failed", failed)
	}

	output.Success("Repository consistency checks passed (go vet, go build, gofmt).")
	output.Footer(time.Since(start))
	actions := suggest.After(suggest.Context{Deity: "net", Subcommand: "align"})
	var steps [][]string
	for _, a := range actions {
		steps = append(steps, []string{a.Command, a.Description})
	}
	output.NextSteps(steps)
	return nil
}
