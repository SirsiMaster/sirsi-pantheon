package trustboundary

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// Gate runs the repo's own verifiers with their exit status as the verdict
// (rule H: never `… | tail && push`), refuses any newly admitted exemption,
// scans the pushed range for secrets, and lints the pushed files against A–H.
//
// Everything in range mode is read from the exact HEAD commit (git blobs),
// never from the working tree: the tree an agent is looking at is not the
// tree it is pushing.
type Gate struct {
	Root string // repo root
	Base string // range base (exclusive); empty with Head empty = working-tree survey
	Head string // range head (inclusive)
	All  bool   // lint the whole tree instead of the changed files
	Lint bool   // run only the trust-boundary lint (skip traceability/exemptions/secrets)
	Out  io.Writer
}

// Range is one pushed ref's base..head.
type Range struct {
	Ref  string `json:"ref"`
	Base string `json:"base"`
	Head string `json:"head"`
}

// Step is one gate step's outcome. Status is pass, fail or skip.
type Step struct {
	Ref    string `json:"ref,omitempty"`
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

// errAbsent marks a path that does not exist at a revision (as opposed to a
// read failure, which is never silently treated as absence).
var errAbsent = errors.New("absent at revision")

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
			sha, err := g.git("rev-parse", "--verify", *rev+"^{commit}")
			if err != nil {
				r.Steps = append(r.Steps, Step{Name: "range", Status: "fail", Detail: "cannot resolve revision " + *rev})
				return r
			}
			*rev = sha
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

// RunRanges gates every pushed ref and aggregates: one failing ref fails the
// push, and every ref's steps are shown.
func RunRanges(g Gate, ranges []Range) Result {
	all := Result{OK: true}
	for _, rg := range ranges {
		gg := g
		gg.Base, gg.Head = rg.Base, rg.Head
		res := gg.Run()
		for i := range res.Steps {
			if len(ranges) > 1 {
				res.Steps[i].Ref = rg.Ref
			}
		}
		all.Steps = append(all.Steps, res.Steps...)
		all.Findings = append(all.Findings, res.Findings...)
		all.OK = all.OK && res.OK
	}
	return all
}

func (g Gate) git(args ...string) (string, error) {
	out, _, err := g.gitFull(args...)
	return out, err
}

func (g Gate) gitFull(args ...string) (stdout, stderr string, err error) {
	cmd := exec.Command("git", append([]string{"-C", g.Root}, args...)...)
	cmd.Env = scrubGitEnv(os.Environ())
	var o, e bytes.Buffer
	cmd.Stdout, cmd.Stderr = &o, &e
	err = cmd.Run()
	return strings.TrimSpace(o.String()), strings.TrimSpace(e.String()), err
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

var absentMsg = regexp.MustCompile(`does not exist in|Not a valid object name|invalid object name|exists on disk, but not in`)

// blob returns the content of path at rev. errAbsent when the path is not in
// that commit's tree; any other failure is a read error and is returned as such.
func (g Gate) blob(rev, path string) ([]byte, error) {
	cmd := exec.Command("git", "-C", g.Root, "show", rev+":"+path)
	cmd.Env = scrubGitEnv(os.Environ())
	var o, e bytes.Buffer
	cmd.Stdout, cmd.Stderr = &o, &e
	if err := cmd.Run(); err != nil {
		if absentMsg.MatchString(e.String()) {
			return nil, errAbsent
		}
		return nil, fmt.Errorf("git show %s:%s: %v: %s", short(rev), path, err, strings.TrimSpace(e.String()))
	}
	return o.Bytes(), nil
}

func (g Gate) traceability(ranged bool) Step {
	s := Step{Name: "traceability"}
	if !ranged {
		s.Status, s.Detail = "skip", "no commit range (pass --base/--head or --pre-push)"
		return s
	}
	// The verifier that runs is the one at the BASE (what the remote already
	// trusts), beside the base's exemption list. A push that edits, replaces or
	// removes its own verifier cannot attest itself: any difference between
	// base and head in the script fails the gate and asks for a separate review.
	baseScript, baseErr := g.blob(g.Base, traceabilityScript)
	headScript, headErr := g.blob(g.Head, traceabilityScript)
	for _, err := range []error{baseErr, headErr} {
		if err != nil && !errors.Is(err, errAbsent) {
			s.Status, s.Detail = "fail", err.Error()
			return s
		}
	}
	switch {
	case errors.Is(baseErr, errAbsent) && errors.Is(headErr, errAbsent):
		s.Status, s.Detail = "skip", "no "+traceabilityScript+" at "+short(g.Base)+" or "+short(g.Head)
		return s
	case errors.Is(baseErr, errAbsent) || errors.Is(headErr, errAbsent) || !bytes.Equal(baseScript, headScript):
		s.Status, s.Detail = "fail", traceabilityScript+" verifier changed in range "+short(g.Base)+".."+short(g.Head)+" — separate review required; a push may not attest itself (rule H)"
		return s
	}
	tmp, err := os.MkdirTemp("", "tb-gate-")
	if err != nil {
		s.Status, s.Detail = "fail", err.Error()
		return s
	}
	defer os.RemoveAll(tmp)
	scriptPath := filepath.Join(tmp, filepath.Base(traceabilityScript))
	if err := os.WriteFile(scriptPath, baseScript, 0o700); err != nil {
		s.Status, s.Detail = "fail", err.Error()
		return s
	}
	if ex, err := g.blob(g.Base, exemptionsFile); err == nil {
		if err := os.WriteFile(filepath.Join(tmp, filepath.Base(exemptionsFile)), ex, 0o600); err != nil {
			s.Status, s.Detail = "fail", err.Error()
			return s
		}
	} else if !errors.Is(err, errAbsent) {
		s.Status, s.Detail = "fail", err.Error()
		return s
	}
	cmd := exec.Command("bash", scriptPath, "--pull-request", g.Base, g.Head)
	cmd.Dir = g.Root
	cmd.Env = scrubGitEnv(os.Environ())
	var buf bytes.Buffer
	cmd.Stdout, cmd.Stderr = &buf, &buf
	err = cmd.Run() // the script's own exit status is the verdict — nothing between it and us
	s.Detail = strings.TrimSpace(buf.String())
	if err != nil {
		s.Status = "fail"
		return s
	}
	s.Status = "pass"
	return s
}

// shaSet returns the hash-bound entries of the exemption file at rev.
// present=false means the file is not in that commit; a read error is an error.
func (g Gate) shaSet(rev string) (set map[string]bool, present bool, err error) {
	b, err := g.blob(rev, exemptionsFile)
	if errors.Is(err, errAbsent) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	set = map[string]bool{}
	for _, sha := range shaLine.FindAllString(string(b), -1) {
		set[sha] = true
	}
	return set, true, nil
}

// exemptions enforces Law 8: the head's exemption set must be a subset of the
// base's. Any hash admitted in the range fails — including an equal-count
// replacement, which a count comparison would wave through.
func (g Gate) exemptions(ranged bool) Step {
	s := Step{Name: "exemption-growth"}
	if !ranged {
		s.Status, s.Detail = "skip", "no commit range"
		return s
	}
	headSet, headPresent, err := g.shaSet(g.Head)
	if err != nil {
		s.Status, s.Detail = "fail", err.Error()
		return s
	}
	if !headPresent {
		s.Status, s.Detail = "skip", "no "+exemptionsFile+" at "+short(g.Head)
		return s
	}
	baseSet, _, err := g.shaSet(g.Base) // absent at base ⇒ every head entry is an addition
	if err != nil {
		s.Status, s.Detail = "fail", err.Error()
		return s
	}
	var added []string
	for sha := range headSet {
		if !baseSet[sha] {
			added = append(added, sha)
		}
	}
	sort.Strings(added)
	if len(added) > 0 {
		s.Status = "fail"
		s.Detail = fmt.Sprintf("%s admits %d hash(es) not present at %s — a commit may not admit itself (rule H); fix the trailers instead: %s",
			exemptionsFile, len(added), short(g.Base), strings.Join(shorts(added), ", "))
		return s
	}
	s.Status, s.Detail = "pass", fmt.Sprintf("%d exemptions, none added", len(headSet))
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
	case !ranged:
		findings, err = LintTree(g.Root)
		s.Detail = "working tree"
	default:
		var paths []string
		if g.All {
			out, gerr := g.git("ls-tree", "-r", "-z", "--name-only", g.Head)
			if gerr != nil {
				s.Status, s.Detail = "fail", "git ls-tree failed: "+gerr.Error()
				return s, nil
			}
			paths = splitNUL(out)
			s.Detail = "whole tree at " + short(g.Head)
		} else {
			// Every path that exists at head and differs from base: --no-renames
			// shows a rename-with-edit as an added file (so it is linted) and
			// -z hands back exact bytes, never a quoted name the lint would miss.
			out, gerr := g.git("diff", "--name-only", "-z", "--no-renames", "--diff-filter=d", g.Base, g.Head)
			if gerr != nil {
				s.Status, s.Detail = "fail", "git diff failed: "+gerr.Error()
				return s, nil
			}
			paths = splitNUL(out)
			s.Detail = fmt.Sprintf("%d changed files at %s", len(paths), short(g.Head))
		}
		head := g.Head
		findings, err = LintBlobs(func(p string) ([]byte, error) { return g.blob(head, p) }, paths)
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

func splitNUL(s string) []string {
	var out []string
	for _, p := range strings.Split(s, "\x00") {
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func short(sha string) string {
	if len(sha) > 8 {
		return sha[:8]
	}
	return sha
}

func shorts(shas []string) []string {
	out := make([]string, len(shas))
	for i, s := range shas {
		out[i] = short(s)
	}
	return out
}

// RangesFromPrePush reads git's pre-push stdin (`<local-ref> <local-sha>
// <remote-ref> <remote-sha>` per line) and returns a Range for EVERY branch
// ref that carries content. Tag refs and deletions carry none. A new branch
// (remote sha all zeros) is based at its merge-base with origin's default
// branch; when no such base can be resolved the push is refused (fail closed)
// rather than admitted as "nothing to check".
func RangesFromPrePush(root string, stdin io.Reader) ([]Range, error) {
	const zero = "0000000000000000000000000000000000000000"
	g := Gate{Root: root}
	var ranges []Range
	var errs []string
	sc := bufio.NewScanner(stdin)
	for sc.Scan() {
		f := strings.Fields(sc.Text())
		if len(f) != 4 || strings.HasPrefix(f[0], "refs/tags/") || f[1] == zero {
			continue
		}
		rg := Range{Ref: f[0], Head: f[1], Base: f[3]}
		if rg.Base == zero {
			rg.Base = ""
			for _, ref := range []string{"origin/main", "origin/master", "origin/HEAD"} {
				if mb, err := g.git("merge-base", rg.Head, ref); err == nil && mb != "" {
					rg.Base = mb
					break
				}
			}
			if rg.Base == "" {
				errs = append(errs, rg.Ref+": new branch with no resolvable base (no origin/main, origin/master or common ancestor) — fetch origin or pass --base explicitly")
				continue
			}
		}
		ranges = append(ranges, rg)
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	if len(errs) > 0 {
		return ranges, errors.New(strings.Join(errs, "; "))
	}
	return ranges, nil
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
