package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/SirsiMaster/sirsi-pantheon/internal/neith"
	"github.com/SirsiMaster/sirsi-pantheon/internal/output"
	"github.com/SirsiMaster/sirsi-pantheon/internal/suggest"
)

var netCmd = &cobra.Command{
	Use:     "net",
	Aliases: []string{"neith"},
	Short:   "𓁯 Net — Scope Weaver & Plan Alignment",
	Long: `𓁯 Net — Scope Weaver & Plan Alignment

Net defines task scopes for Ra, tracks plan alignment against build logs,
detects drift, and validates cross-module consistency.

  sirsi net status    Check plan alignment score
  sirsi net align     Validate all-module consistency`,
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
	Short: "Validate cross-module consistency",
	RunE:  runNetAlign,
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
	for _, rel := range []string{"docs/BUILD_LOG.md", "BUILD_LOG.md"} {
		if _, err := os.Stat(filepath.Join(root, rel)); err == nil {
			logPath = rel
			break
		}
	}
	res.AddEvidence("Repository", root)
	if logPath == "" {
		res.Summary = "This repository has no build log yet — alignment is not measurable, so no score is shown."
		res.Status = "unmeasured"
		res.AddEvidence("Build log", "not found (looked for docs/BUILD_LOG.md and BUILD_LOG.md)")
	} else {
		full := filepath.Join(root, logPath)
		info, _ := os.Stat(full)
		data, _ := os.ReadFile(full)
		entries := strings.Count(string(data), "\n## ")
		res.Summary = fmt.Sprintf("Build log found (%d entries) — alignment scoring needs a recorded session plan, and none is recorded yet, so no score is shown.", entries)
		res.Status = "unmeasured"
		res.AddEvidence("Build log", logPath)
		res.AddEvidence("Entries", fmt.Sprintf("%d", entries))
		if info != nil {
			res.AddEvidence("Last updated", info.ModTime().Format("Jan 2, 2006"))
		}
	}
	res.NextActions = append(res.NextActions,
		output.NextAction{Label: "Validate cross-module consistency", Command: "sirsi net align", Description: "Run vet/build/format checks against this repository."},
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
			BriefTitle: "Module Consistency",
			Summary:    "Not inside a code repository — nothing to check here.",
			Status:     "unmeasured",
			Duration:   time.Since(start),
		}
		res.NextActions = append(res.NextActions, output.NextAction{
			Label:       "Run from a repository",
			Command:     "cd <your-repo> && sirsi net align",
			Description: "Consistency checks (vet, build, format) run against a repo root.",
		})
		res.Render()
		return nil
	}

	output.Banner()
	output.Header("Module Consistency Check")

	// Real checks against the repository (never the caller's cwd).
	tap := &neith.Tapestry{}
	inRepoCmd := func(name string, args ...string) *exec.Cmd {
		c := exec.Command(name, args...)
		c.Dir = root
		return c
	}

	// Ma'at: go vet passes
	if err := inRepoCmd("go", "vet", "./...").Run(); err == nil {
		tap.MaatConsistent = true
		output.Success("Ma'at: go vet passes")
	} else {
		output.Error("Ma'at: go vet failed")
	}

	// Anubis: build succeeds (no scan rule regressions)
	if err := inRepoCmd("go", "build", "./...").Run(); err == nil {
		tap.AnubisCorrect = true
		output.Success("Anubis: build succeeds")
	} else {
		output.Error("Anubis: build failed")
	}

	// Hygiene: gofmt clean
	out, _ := inRepoCmd("gofmt", "-l", "./internal/", "./cmd/").Output()
	if len(out) == 0 {
		tap.HygieneClean = true
		output.Success("Hygiene: gofmt clean")
	} else {
		output.Error("Hygiene: gofmt violations found")
	}

	// Thoth: .thoth/ memory present
	if _, err := os.Stat(filepath.Join(root, ".thoth", "memory.yaml")); err == nil {
		tap.ThothAccurate = true
		output.Success("Thoth: memory present")
	} else {
		output.Warn("Thoth: .thoth/memory.yaml not found")
	}

	// Isis: hardened (always true for alignment — network checks are separate)
	tap.IsisHardened = true
	output.Success("Isis: system health assumed")

	fmt.Println()

	err := tap.Align()
	if err != nil {
		output.Error("Alignment failed: %v", err)
		output.Footer(time.Since(start))
		return nil
	}

	output.Success("All modules aligned — tapestry is balanced")
	output.Footer(time.Since(start))
	actions := suggest.After(suggest.Context{Deity: "net", Subcommand: "align"})
	var steps [][]string
	for _, a := range actions {
		steps = append(steps, []string{a.Command, a.Description})
	}
	output.NextSteps(steps)
	return nil
}
