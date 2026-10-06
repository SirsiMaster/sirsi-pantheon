package trustboundary

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

// Gate runs the repo's own verifiers with their exit status as the verdict
// (rule H: never `… | tail && push`), refuses exemption growth, scans the
// pushed range for secrets, and lints the changed files against A–H.
type Gate struct {
	Root string // repo root
	Base string // range base (exclusive); empty = whole tree lint, no range steps
	Head string // range head (inclusive)
	All  bool   // lint the whole tree instead of the changed files
	Lint bool   // run only the trust-boundary lint (skip traceability/secrets)
	Out  io.Writer
}

// Step is one gate step's outcome. Status is pass, fail or skip.
type Step struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Detail string `json:"detail,omitempty"`
}

// Result is the whole gate's outcome.
type Result struct {
	Steps    []Step    `json:"steps"`
	Findings []Finding `json:"findings"`
	OK       bool      `json:"ok"`
}

const (
	traceabilityScript = "scripts/verify-commit-traceability.sh"
	exemptionsFile     = "scripts/traceability-historical-exemptions.txt"
)

var shaLine = regexp.MustCompile(`(?m)^[0-9a-f]{40}\b`)

// Run executes every step and never short-circuits: a push blocked by one
// failure should show the agent all of them.
func (g Gate) Run() Result {
	if g.Out == nil {
		g.Out = io.Discard
	}
	var r Result
	// Verifiers want exact 40-hex SHAs (the traceability script refuses "HEAD");
	// accept any revision here and resolve it once.
	for _, rev := range []*string{&g.Base, &g.Head} {
		if *rev != "" {
			if sha, err := g.git("rev-parse", "--verify", *rev+"^{commit}"); err == nil {
				*rev = sha
			}
		}
	}
	ranged := g.Base != "" && g.Head != ""
	if !g.Lint {
		r.Steps = append(r.Steps, g.traceability(ranged), g.exemptions(ranged), g.secrets(ranged))
	}
	lintStep, findings := g.lint(ranged)
	r.Steps = append(r.Steps, lintStep)
	r.Findings = findings
	r.OK = true
	for _, s := range r.Steps {
		if s.Status == "fail" {
			r.OK = false
		}
	}
	return r
}

func (g Gate) git(args ...string) (string, error) {
	cmd := exec.Command("git", append([]string{"-C", g.Root}, args...)...)
	cmd.Env = scrubGitEnv(os.Environ())
	out, err := cmd.Output()
	return strings.TrimSpace(string(out)), err
}

// scrubGitEnv drops GIT_* so a gate spawned by `git push` does not inherit the
// hook's GIT_DIR/GIT_INDEX_FILE and operate on the wrong tree.
func scrubGitEnv(env []string) []string {
	var out []string
	for _, kv := range env {
		if !strings.HasPrefix(kv, "GIT_") {
			out = append(out, kv)
		}
	}
	return out
}

func (g Gate) traceability(ranged bool) Step {
	s := Step{Name: "traceability"}
	script := filepath.Join(g.Root, traceabilityScript)
	if _, err := os.Stat(script); err != nil {
		s.Status, s.Detail = "skip", "no "+traceabilityScript+" in this repo"
		return s
	}
	if !ranged {
		s.Status, s.Detail = "skip", "no commit range (pass --base/--head or --pre-push)"
		return s
	}
	cmd := exec.Command("bash", script, "--pull-request", g.Base, g.Head)
	cmd.Dir = g.Root
	cmd.Env = scrubGitEnv(os.Environ())
	var buf bytes.Buffer
	cmd.Stdout, cmd.Stderr = &buf, &buf
	err := cmd.Run() // the script's own exit status is the verdict — nothing between it and us
	s.Detail = strings.TrimSpace(buf.String())
	if err != nil {
		s.Status = "fail"
		return s
	}
	s.Status = "pass"
	return s
}

func (g Gate) exemptions(ranged bool) Step {
	s := Step{Name: "exemption-growth"}
	if _, err := os.Stat(filepath.Join(g.Root, exemptionsFile)); err != nil {
		s.Status, s.Detail = "skip", "no "+exemptionsFile
		return s
	}
	if !ranged {
		s.Status, s.Detail = "skip", "no commit range"
		return s
	}
	count := func(rev string) int {
		out, err := g.git("show", rev+":"+exemptionsFile)
		if err != nil {
			return 0 // absent at this revision
		}
		return len(shaLine.FindAllString(out, -1))
	}
	before, after := count(g.Base), count(g.Head)
	if after > before {
		s.Status = "fail"
		s.Detail = fmt.Sprintf("%s grew from %d to %d hash-bound exemptions in %s..%s — a commit may not admit itself (rule H); fix the trailers instead", exemptionsFile, before, after, short(g.Base), short(g.Head))
		return s
	}
	s.Status, s.Detail = "pass", fmt.Sprintf("%d exemptions, unchanged", after)
	return s
}

func (g Gate) secrets(ranged bool) Step {
	s := Step{Name: "secrets"}
	if _, err := exec.LookPath("gitleaks"); err != nil {
		s.Status, s.Detail = "skip", "gitleaks not installed (brew install gitleaks) — CI's secrets scan still gates; a disarmed local gate is not compliance"
		return s
	}
	if !ranged {
		s.Status, s.Detail = "skip", "no commit range"
		return s
	}
	cmd := exec.Command("gitleaks", "detect", "--log-opts="+g.Base+".."+g.Head, "--no-banner", "--redact", "--exit-code", "1")
	cmd.Dir = g.Root
	var buf bytes.Buffer
	cmd.Stdout, cmd.Stderr = &buf, &buf
	if err := cmd.Run(); err != nil {
		s.Status, s.Detail = "fail", strings.TrimSpace(buf.String())
		return s
	}
	s.Status = "pass"
	return s
}

func (g Gate) lint(ranged bool) (Step, []Finding) {
	s := Step{Name: "trust-boundary-lint"}
	var (
		findings []Finding
		err      error
	)
	switch {
	case g.All || !ranged:
		findings, err = LintTree(g.Root)
		s.Detail = "whole tree"
	default:
		out, gerr := g.git("diff", "--name-only", "--diff-filter=AM", g.Base, g.Head)
		if gerr != nil {
			s.Status, s.Detail = "fail", "git diff failed: "+gerr.Error()
			return s, nil
		}
		var paths []string
		for _, p := range strings.Split(out, "\n") {
			if p = strings.TrimSpace(p); p != "" {
				paths = append(paths, p)
			}
		}
		findings, err = LintFiles(g.Root, paths)
		s.Detail = fmt.Sprintf("%d changed files", len(paths))
	}
	if err != nil {
		s.Status, s.Detail = "fail", err.Error()
		return s, nil
	}
	if len(findings) > 0 {
		s.Status = "fail"
		s.Detail += fmt.Sprintf(", %d finding(s); silence a reviewed false positive with `%s <reason>` on the line or the one above", len(findings), allowMarker)
		return s, findings
	}
	s.Status = "pass"
	return s, nil
}

func short(sha string) string {
	if len(sha) > 8 {
		return sha[:8]
	}
	return sha
}

// RangeFromPrePush reads git's pre-push stdin (`<local-ref> <local-sha>
// <remote-ref> <remote-sha>` per line) and returns the base/head of the first
// branch ref that carries content. A new branch (remote sha all zeros) is
// based at its merge-base with origin's default branch so the gate checks
// only what this push adds. ok is false when the push carries no content
// (tags, deletions).
func RangeFromPrePush(root string, stdin io.Reader) (base, head string, ok bool) {
	const zero = "0000000000000000000000000000000000000000"
	sc := bufio.NewScanner(stdin)
	for sc.Scan() {
		f := strings.Fields(sc.Text())
		if len(f) != 4 || strings.HasPrefix(f[0], "refs/tags/") || f[1] == zero {
			continue
		}
		head = f[1]
		if f[3] != zero {
			return f[3], head, true
		}
		g := Gate{Root: root}
		for _, ref := range []string{"origin/main", "origin/master", "origin/HEAD"} {
			if mb, err := g.git("merge-base", head, ref); err == nil && mb != "" {
				return mb, head, true
			}
		}
		// First push of a repo with no remote default: lint the whole tree.
		return "", head, true
	}
	return "", "", false
}

// ArmHooks points core.hooksPath at .githooks for the repo at root. Worktrees
// share .git/config, so one call covers every checkout of that repository.
func ArmHooks(root string) error {
	if _, err := os.Stat(filepath.Join(root, ".githooks", "pre-push")); err != nil {
		return fmt.Errorf("%s ships no .githooks/pre-push", root)
	}
	g := Gate{Root: root}
	if cur, _ := g.git("config", "--get", "core.hooksPath"); cur == ".githooks" {
		return nil
	}
	if _, err := g.git("config", "core.hooksPath", ".githooks"); err != nil {
		return fmt.Errorf("git config core.hooksPath: %w", err)
	}
	return nil
}

// RepoRoot is the top level of the git checkout containing dir (worktree-aware:
// `git rev-parse --show-toplevel`, not a walk-up for a `.git` directory).
func RepoRoot(dir string) (string, error) {
	cmd := exec.Command("git", "-C", dir, "rev-parse", "--show-toplevel")
	cmd.Env = scrubGitEnv(os.Environ())
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}
