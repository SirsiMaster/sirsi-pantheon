package trustboundary

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

// Gate runs the repo's own verifiers with their exit status as the verdict,
// refuses any newly admitted exemption,
// scans the pushed range for secrets, and lints the pushed files against A–H.
//
// Everything in range mode is read from the exact HEAD commit (git blobs),
// never from the working tree: the tree an agent is looking at is not the
// tree it is pushing.
type Gate struct {
	Root    string // repo root
	Base    string // range base (exclusive); empty with Head empty = working-tree survey
	Head    string // range head (inclusive)
	All     bool   // lint the whole tree instead of the changed files
	Lint    bool   // run only the trust-boundary lint (skip traceability/exemptions/secrets)
	Out     io.Writer
	Context context.Context
	// Private per-instance fixture seams; no CLI, config or environment override.
	tools          *gateTools
	commandTimeout time.Duration
	commandLimit   int
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
	Scope    string    `json:"scope"`
	Steps    []Step    `json:"steps"`
	Findings []Finding `json:"findings"`
	Skipped  []string  `json:"skipped,omitempty"`
	OK       bool      `json:"ok"`
	Complete bool      `json:"complete"`
}

const (
	traceabilityScript = "scripts/verify-commit-traceability.sh"
	exemptionsFile     = "scripts/traceability-historical-exemptions.txt"
	gitleaksConfigFile = ".gitleaks.toml"
	gitleaksIgnoreFile = ".gitleaksignore"
)

var shaLine = regexp.MustCompile(`(?m)^[0-9a-f]{40}\b`)

// errAbsent marks a path that does not exist at a revision (as opposed to a
// read failure, which is never silently treated as absence).
var errAbsent = errors.New("absent at revision")

// Run executes every step and never short-circuits: a push blocked by one
// failure should show the agent all of them.
func (g Gate) Run() Result {
	r := Result{Scope: g.scope()}
	if (g.Base == "") != (g.Head == "") {
		r.Steps = []Step{{Name: "range", Status: "fail", Detail: "--base and --head must be supplied together; no survey fallback"}}
		return r
	}
	parent := g.Context
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithTimeout(parent, 5*time.Minute)
	defer cancel()
	g.Context = ctx
	if g.Out == nil {
		g.Out = io.Discard
	}
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
	r = summarize(r)
	return r
}

func (g Gate) scope() string {
	if g.Lint {
		if g.Base != "" && g.Head != "" {
			return "lint-only-range"
		}
		return "lint-only"
	}
	if g.Base != "" && g.Head != "" {
		return "range"
	}
	return "working-tree-survey"
}

// summarize distinguishes “no executed check failed” from a complete pass.
// In particular, optional or out-of-scope steps remain explicit in JSON.
func summarize(r Result) Result {
	r.OK = true
	r.Complete = len(r.Steps) > 0
	r.Skipped = nil
	for _, s := range r.Steps {
		switch s.Status {
		case "fail":
			r.OK = false
			r.Complete = false
		case "skip":
			r.Complete = false
			name := s.Name
			if s.Ref != "" {
				name = s.Ref + ":" + name
			}
			r.Skipped = append(r.Skipped, name)
		}
	}
	// A skipped required verifier is not a passing enforcement result. Keep OK
	// as "no executed check failed" for surveys, but callers that enforce a
	// complete gate must also require Complete.
	return r
}

// RunRanges gates every pushed ref and aggregates: one failing ref fails the
// push, and every ref's steps are shown.
func RunRanges(g Gate, ranges []Range) Result {
	all := Result{Scope: "pre-push", OK: true, Complete: true, Steps: []Step{}, Findings: []Finding{}}
	if len(ranges) == 0 {
		return all
	}
	for _, rg := range ranges {
		gg := g
		gg.Base, gg.Head = rg.Base, rg.Head
		res := gg.Run()
		for i := range res.Steps {
			res.Steps[i].Ref = rg.Ref
		}
		all.Steps = append(all.Steps, res.Steps...)
		all.Findings = append(all.Findings, res.Findings...)
	}
	return summarize(all)
}

func (g Gate) git(args ...string) (string, error) {
	out, _, err := g.gitFull(args...)
	return strings.TrimSpace(out), err // scalar revisions/config only, never path lists
}

func (g Gate) gitFull(args ...string) (stdout, stderr string, err error) {
	o, e, err := g.command(g.commandTools().git, append([]string{"-c", "core.fsmonitor=false", "-C", g.Root}, args...)...)
	return string(o), strings.TrimSpace(string(e)), err // NUL streams stay byte-exact
}

type gateTools struct{ git, bash, gitleaks string }

func (g Gate) commandTools() gateTools {
	if g.tools != nil {
		return *g.tools
	}
	t := gateTools{git: "/usr/bin/git", bash: "/bin/bash"}
	// Never discover an executable through the caller's PATH.
	for _, path := range []string{"/opt/homebrew/bin/gitleaks", "/usr/local/bin/gitleaks", "/usr/bin/gitleaks"} {
		if info, err := os.Stat(path); err == nil && info.Mode().IsRegular() && info.Mode().Perm()&0111 != 0 {
			t.gitleaks = path
			break
		}
	}
	return t
}

// Closed allowlist: no BASH_ENV/ENV/exported functions, TRACEABILITY_* policy,
// credential/proxy/provider/SIRSI variables, or caller system/global Git
// configuration survives. Git still reads the target repository's local config.
func gateCommandEnv(home string) []string {
	return []string{"PATH=/usr/bin:/bin:/usr/sbin:/sbin", "LANG=C", "LC_ALL=C", "TZ=UTC",
		"HOME=" + home, "XDG_CONFIG_HOME=" + home, "TMPDIR=" + home, "TMP=" + home, "TEMP=" + home,
		"GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_SYSTEM=" + os.DevNull, "GIT_CONFIG_GLOBAL=" + os.DevNull,
		"GIT_TERMINAL_PROMPT=0", "GIT_OPTIONAL_LOCKS=0", "GIT_PAGER=cat", "PAGER=cat"}
}

var errGateOutputLimit = errors.New("gate command output exceeded its explicit byte ceiling; result unknown")

type gateCapture struct {
	mu       sync.Mutex
	data     []byte
	limit    int
	overflow bool
	cancel   context.CancelFunc
}

func (b *gateCapture) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	remaining := b.limit - len(b.data)
	if len(p) > remaining {
		b.data = append(b.data, p[:remaining]...)
		b.overflow = true
		b.cancel()
		return remaining, errGateOutputLimit
	}
	b.data = append(b.data, p...)
	return len(p), nil
}

func (b *gateCapture) snapshot() ([]byte, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]byte(nil), b.data...), b.overflow
}

// Fixed 60s direct-child deadline, 8MiB stdout / 64KiB stderr ceilings and
// 2s pipe-wait bound. Overflow/timeout never becomes truncated success.
// This is cooperative command supervision, NOT a descendant-group/OS sandbox.
func (g Gate) command(path string, args ...string) ([]byte, []byte, error) {
	parent := g.Context
	if parent == nil {
		parent = context.Background()
	}
	timeout, limit := 60*time.Second, 8<<20
	if g.commandTimeout > 0 {
		timeout = g.commandTimeout
	}
	if g.commandLimit > 0 {
		limit = g.commandLimit
	}
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()
	home, err := os.MkdirTemp("", "tb-command-")
	if err != nil {
		return nil, nil, err
	}
	defer os.RemoveAll(home) // only this fresh private command directory
	cmd := exec.CommandContext(ctx, path, args...)
	cmd.Dir, cmd.Env, cmd.WaitDelay = g.Root, gateCommandEnv(home), 2*time.Second
	out := &gateCapture{limit: limit, cancel: cancel}
	stderr := &gateCapture{limit: 64 << 10, cancel: cancel}
	cmd.Stdout, cmd.Stderr = out, stderr
	err = cmd.Run()
	o, overOut := out.snapshot()
	e, overErr := stderr.snapshot()
	if overOut || overErr {
		return o, e, errGateOutputLimit
	}
	if ctx.Err() != nil {
		return o, e, ctx.Err()
	}
	return o, e, err
}

var absentMsg = regexp.MustCompile(`does not exist in|Not a valid object name|invalid object name|exists on disk, but not in`)

// blob returns the content of path at rev. errAbsent when the path is not in
// that commit's tree; any other failure is a read error and is returned as such.
func (g Gate) blob(rev, path string) ([]byte, error) {
	o, e, err := g.command(g.commandTools().git, "-c", "core.fsmonitor=false", "-C", g.Root, "show", rev+":"+path)
	if err != nil {
		var exited *exec.ExitError
		if errors.As(err, &exited) && exited.ExitCode() == 128 && absentMsg.Match(e) {
			return nil, errAbsent
		}
		return nil, fmt.Errorf("git show %s:%s: %w: %s", short(rev), path, err, strings.TrimSpace(string(e)))
	}
	return o, nil
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
		s.Status, s.Detail = "fail", traceabilityScript+" verifier changed in range "+short(g.Base)+".."+short(g.Head)+" — separate review required; a push may not attest itself"
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
	out, stderr, err := g.command(g.commandTools().bash, "--noprofile", "--norc", scriptPath, "--pull-request", g.Base, g.Head)
	s.Detail = strings.TrimSpace(string(out) + "\n" + string(stderr))
	if err != nil {
		s.Status = "fail"
		s.Detail += "\nverifier execution failed or uncertain: " + err.Error()
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

// exemptions checks that the head's exemption set is a subset of the base's.
// Any hash admitted in the range fails — including an equal-count
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
		s.Detail = fmt.Sprintf("%s admits %d hash(es) not present at %s — a commit may not admit itself; fix the trailers instead: %s",
			exemptionsFile, len(added), short(g.Base), strings.Join(shorts(added), ", "))
		return s
	}
	s.Status, s.Detail = "pass", fmt.Sprintf("%d exemptions, none added", len(headSet))
	return s
}

func (g Gate) secrets(ranged bool) Step {
	s := Step{Name: "secrets"}
	tool := g.commandTools().gitleaks
	if tool == "" {
		s.Status, s.Detail = "skip", "gitleaks not installed (brew install gitleaks) — CI's secrets scan remains authoritative; this check was not performed"
		return s
	}
	if !ranged {
		s.Status, s.Detail = "skip", "no commit range"
		return s
	}
	// Never let Gitleaks discover policy or ignores from the mutable checkout.
	// Policy changes must be reviewed separately; the scan runs with the exact
	// base-commit policy and ignore entries only after verifying they are
	// unchanged through HEAD.
	baseConfig, err := g.blob(g.Base, gitleaksConfigFile)
	if err != nil {
		if errors.Is(err, errAbsent) {
			s.Status, s.Detail = "fail", "base commit has no committed .gitleaks.toml; refusing mutable/default policy discovery"
		} else {
			s.Status, s.Detail = "fail", err.Error()
		}
		return s
	}
	headConfig, err := g.blob(g.Head, gitleaksConfigFile)
	if err != nil || !bytes.Equal(baseConfig, headConfig) {
		s.Status, s.Detail = "fail", "committed .gitleaks.toml changed or cannot be read in range; separate policy review required"
		return s
	}
	baseIgnore, ignoreErr := g.blob(g.Base, gitleaksIgnoreFile)
	if ignoreErr != nil && !errors.Is(ignoreErr, errAbsent) {
		s.Status, s.Detail = "fail", ignoreErr.Error()
		return s
	}
	headIgnore, headIgnoreErr := g.blob(g.Head, gitleaksIgnoreFile)
	if headIgnoreErr != nil && !errors.Is(headIgnoreErr, errAbsent) {
		s.Status, s.Detail = "fail", headIgnoreErr.Error()
		return s
	}
	if errors.Is(ignoreErr, errAbsent) != errors.Is(headIgnoreErr, errAbsent) || !bytes.Equal(baseIgnore, headIgnore) {
		s.Status, s.Detail = "fail", "committed .gitleaksignore changed or cannot be read in range; separate policy review required"
		return s
	}
	policyDir, err := os.MkdirTemp("", "tb-gitleaks-policy-")
	if err != nil {
		s.Status, s.Detail = "fail", "create private policy directory: "+err.Error()
		return s
	}
	defer os.RemoveAll(policyDir)
	configPath := filepath.Join(policyDir, "gitleaks.toml")
	ignorePath := filepath.Join(policyDir, ".gitleaksignore")
	if err := os.WriteFile(configPath, baseConfig, 0o600); err != nil {
		s.Status, s.Detail = "fail", "write committed Gitleaks policy: "+err.Error()
		return s
	}
	if errors.Is(ignoreErr, errAbsent) {
		baseIgnore = nil
	}
	if err := os.WriteFile(ignorePath, baseIgnore, 0o600); err != nil {
		s.Status, s.Detail = "fail", "write committed Gitleaks ignore policy: "+err.Error()
		return s
	}
	out, stderr, err := g.command(tool, "detect", "--log-opts="+g.Base+".."+g.Head, "--config", configPath,
		"--gitleaks-ignore-path", ignorePath, "--no-banner", "--redact", "--exit-code", "1")
	if err != nil {
		s.Status, s.Detail = "fail", strings.TrimSpace(string(out)+"\n"+string(stderr))+"\nscanner execution failed or uncertain: "+err.Error()
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
			out, _, gerr := g.gitFull("ls-tree", "-r", "-z", "--name-only", g.Head)
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
			out, _, gerr := g.gitFull("diff", "--name-only", "-z", "--no-renames", "--diff-filter=d", g.Base, g.Head)
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
// (remote sha all zeros) requires an explicit base: the hook input does not
// identify a trustworthy default branch for the destination remote.
func RangesFromPrePush(root string, stdin io.Reader, newBranchBase string) ([]Range, error) {
	g := Gate{Root: root}
	var ranges []Range
	var errs []string
	line := 0
	records := 0
	sc := bufio.NewScanner(stdin)
	for sc.Scan() {
		line++
		if strings.TrimSpace(sc.Text()) == "" {
			continue
		}
		records++
		f := strings.Fields(sc.Text())
		if len(f) != 4 {
			errs = append(errs, fmt.Sprintf("line %d: malformed pre-push record: expected 4 fields, got %d", line, len(f)))
			continue
		}
		if !validObjectID(f[1]) || !validObjectID(f[3]) {
			errs = append(errs, fmt.Sprintf("line %d: malformed pre-push record: object IDs must be full 40- or 64-character lowercase hex", line))
			continue
		}
		if f[0] == "" || f[2] == "" || !strings.HasPrefix(f[0], "refs/") || !strings.HasPrefix(f[2], "refs/") {
			errs = append(errs, fmt.Sprintf("line %d: malformed pre-push record: both refs must be fully qualified", line))
			continue
		}
		if strings.HasPrefix(f[0], "refs/tags/") || isZeroObjectID(f[1]) {
			continue
		}
		rg := Range{Ref: f[0], Head: f[1], Base: f[3]}
		if isZeroObjectID(rg.Base) {
			if strings.TrimSpace(newBranchBase) == "" {
				errs = append(errs, rg.Ref+": new branch requires --new-branch-base; destination default cannot be inferred from pre-push input")
				continue
			}
			resolved, err := g.git("rev-parse", "--verify", newBranchBase+"^{commit}")
			if err != nil || !validObjectID(resolved) {
				errs = append(errs, rg.Ref+": explicit --new-branch-base does not resolve to a commit")
				continue
			}
			if resolved == rg.Head {
				errs = append(errs, rg.Ref+": explicit --new-branch-base equals the pushed head; refusing an empty scan")
				continue
			}
			if _, err := g.git("merge-base", "--is-ancestor", resolved, rg.Head); err != nil {
				errs = append(errs, rg.Ref+": explicit --new-branch-base is not an ancestor of the pushed head")
				continue
			}
			rg.Base = resolved
		}
		ranges = append(ranges, rg)
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	if records == 0 {
		errs = append(errs, "pre-push input contained no ref records")
	}
	if len(errs) > 0 {
		return ranges, errors.New(strings.Join(errs, "; "))
	}
	return ranges, nil
}

var objectIDPattern = regexp.MustCompile(`^(?:[0-9a-f]{40}|[0-9a-f]{64})$`)

func validObjectID(s string) bool { return objectIDPattern.MatchString(s) }

func isZeroObjectID(s string) bool {
	if !validObjectID(s) {
		return false
	}
	for _, c := range s {
		if c != '0' {
			return false
		}
	}
	return true
}

// ArmHooks points core.hooksPath at .githooks for the repo at root only when no
// conflicting hook path is configured. It refuses local, worktree, global, or
// system hook paths it does not own; it never overwrites an existing choice.
func ArmHooks(root string) error {
	if _, err := os.Stat(filepath.Join(root, ".githooks", "pre-push")); err != nil {
		return fmt.Errorf("%s ships no .githooks/pre-push", root)
	}
	paths, err := configuredHookPaths(root)
	if err != nil {
		return err
	}
	for _, path := range paths {
		if path != ".githooks" {
			return fmt.Errorf("refusing to replace existing core.hooksPath %q; resolve hook ownership explicitly", path)
		}
	}
	if len(paths) > 0 {
		return nil
	}
	g := Gate{Root: root}
	if _, err := g.git("config", "core.hooksPath", ".githooks"); err != nil {
		return fmt.Errorf("git config core.hooksPath: %w", err)
	}
	return nil
}

func configuredHookPaths(root string) ([]string, error) {
	var paths []string
	env, err := hookConfigEnv(os.Environ())
	if err != nil {
		return nil, err
	}
	scopes := []string{"--local", "--global", "--system"}
	if enabled, err := configBool(root, "extensions.worktreeConfig", env); err != nil {
		return nil, err
	} else if enabled {
		scopes = []string{"--worktree", "--local", "--global", "--system"}
	}
	for _, scope := range scopes {
		cmd := exec.Command("/usr/bin/git", "-C", root, "config", scope, "--null", "--get-all", "core.hooksPath")
		cmd.Env = env
		out, err := cmd.Output()
		if err != nil {
			var exitErr *exec.ExitError
			if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
				continue
			}
			return nil, fmt.Errorf("read %s core.hooksPath: %w", strings.TrimPrefix(scope, "--"), err)
		}
		values := strings.Split(string(out), "\x00")
		for i, value := range values {
			if i == len(values)-1 && value == "" {
				continue // final NUL terminator, not a configured empty value
			}
			paths = append(paths, value)
		}
	}
	return paths, nil
}

func configBool(root, key string, env []string) (bool, error) {
	cmd := exec.Command("/usr/bin/git", "-C", root, "config", "--local", "--bool", "--get", key)
	cmd.Env = env
	out, err := cmd.Output()
	if err == nil {
		return strings.TrimSpace(string(out)) == "true", nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
		return false, nil
	}
	return false, fmt.Errorf("read local %s: %w", key, err)
}

func hookConfigEnv(env []string) ([]string, error) {
	clean := make([]string, 0, len(env))
	for _, entry := range env {
		key, _, _ := strings.Cut(entry, "=")
		if strings.HasPrefix(key, "GIT_CONFIG_") {
			return nil, fmt.Errorf("refusing hook configuration while %s may override effective Git config", key)
		}
		switch key {
		case "GIT_DIR", "GIT_WORK_TREE", "GIT_INDEX_FILE", "GIT_COMMON_DIR", "GIT_PREFIX":
			continue
		}
		clean = append(clean, entry)
	}
	return clean, nil
}

// RepoRoot is the top level of the git checkout containing dir (worktree-aware:
// `git rev-parse --show-toplevel`, not a walk-up for a `.git` directory).
func RepoRoot(dir string) (string, error) {
	g := Gate{Root: dir}
	out, _, err := g.command(g.commandTools().git, "-c", "core.fsmonitor=false", "-C", dir, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", err
	}
	return strings.TrimSuffix(string(out), "\n"), nil
}
