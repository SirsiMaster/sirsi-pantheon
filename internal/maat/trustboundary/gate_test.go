package trustboundary

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// repo builds a throwaway git repo with a verifier whose exit status is
// controlled by the fixture, so the gate's verdict can be pinned to it.
func repo(t *testing.T) (dir string, commit func(msg string) string, write func(rel, body string)) {
	t.Helper()
	dir = t.TempDir()
	git := func(args ...string) string {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		cmd.Env = append(scrubGitEnv(os.Environ()), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	git("init", "-q", "-b", "main")
	write = func(rel, body string) {
		t.Helper()
		p := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	commit = func(msg string) string {
		t.Helper()
		git("add", "-A")
		git("commit", "-q", "--allow-empty", "-m", msg)
		return git("rev-parse", "HEAD")
	}
	return dir, commit, write
}

const (
	sha40  = "0123456789abcdef0123456789abcdef01234567"
	sha40b = "89abcdef0123456789abcdef0123456789abcdef"
	zero   = "0000000000000000000000000000000000000000"
	classA = "package x\n\nimport \"strconv\"\n\nfunc f(s string) []int { n, _ := strconv.Atoi(s); return make([]int, n) }\n"
	clean  = "package x\n\nfunc g() int { return 1 }\n"
)

func TestGateExitStatusIsTheVerdict(t *testing.T) {
	dir, commit, write := repo(t)
	// A verifier that prints a cheerful line and exits 1 — exactly the shape
	// `… | tail && push` used to let through.
	write(traceabilityScript, "#!/usr/bin/env bash\necho 'commit traceability verification passed'\nexit 1\n")
	write(exemptionsFile, "# header\n")
	base := commit("chore: base")
	write("ok.go", clean)
	head := commit("chore: head")

	res := Gate{Root: dir, Base: base, Head: head}.Run()
	if res.OK {
		t.Fatal("gate passed although the verifier exited 1")
	}
	if got := step(res, "traceability").Status; got != "fail" {
		t.Errorf("traceability = %s, want fail", got)
	}
	if got := step(res, "exemption-growth").Status; got != "pass" {
		t.Errorf("exemption-growth = %s, want pass", got)
	}

	write(traceabilityScript, "#!/usr/bin/env bash\nexit 0\n")
	head2 := commit("chore: verifier fixed")
	res = Gate{Root: dir, Base: base, Head: head2}.Run()
	if got := step(res, "traceability").Status; got != "pass" {
		t.Errorf("traceability = %s, want pass: %s", got, step(res, "traceability").Detail)
	}
}

// TestVerifierRunsFromPushedHeadNotWorkingTree: the working tree holds a passing
// verifier, the pushed head holds a failing one — the head's verdict wins.
func TestVerifierRunsFromPushedHeadNotWorkingTree(t *testing.T) {
	dir, commit, write := repo(t)
	base := commit("chore: base")
	write(traceabilityScript, "#!/usr/bin/env bash\nexit 1\n")
	head := commit("chore: failing verifier at head")
	write(traceabilityScript, "#!/usr/bin/env bash\nexit 0\n") // uncommitted: not what is being pushed
	res := Gate{Root: dir, Base: base, Head: head}.Run()
	if got := step(res, "traceability").Status; got != "fail" {
		t.Fatalf("traceability = %s, want fail from the HEAD blob", got)
	}
	// And absence is decided at the head, not by os.Stat of the tree.
	res = Gate{Root: dir, Base: base, Head: base}.Run()
	if got := step(res, "traceability"); got.Status != "skip" || !strings.Contains(got.Detail, "at "+short(base)) {
		t.Fatalf("traceability at a head without the script = %+v, want skip pinned to the head", got)
	}
}

func TestExemptionSetAdditionRefusedEvenAtEqualCount(t *testing.T) {
	dir, commit, write := repo(t)
	write(exemptionsFile, "# header\n"+sha40+" fix(a): audited\n")
	base := commit("chore: base")
	// Equal-count REPLACEMENT: one hash swapped for another. A count check passes this.
	write(exemptionsFile, "# header\n"+sha40b+" fix(b): I admit myself\n")
	head := commit("chore: swap")
	res := Gate{Root: dir, Base: "HEAD~1", Head: "HEAD"}.Run() // symbolic revs resolve to the exact SHAs
	s := step(res, "exemption-growth")
	if s.Status != "fail" || !strings.Contains(s.Detail, short(sha40b)) {
		t.Fatalf("exemption-growth = %+v, want fail naming the added hash", s)
	}
	if res.OK {
		t.Fatal("gate passed with a newly admitted exemption in range")
	}
	_, _ = base, head
}

func TestExemptionShrinkPassesAndNewFileFails(t *testing.T) {
	dir, commit, write := repo(t)
	write(exemptionsFile, "# header\n"+sha40+" a\n"+sha40b+" b\n")
	base := commit("chore: base")
	write(exemptionsFile, "# header\n"+sha40+" a\n")
	head := commit("chore: shrink")
	if s := step(Gate{Root: dir, Base: base, Head: head}.Run(), "exemption-growth"); s.Status != "pass" {
		t.Fatalf("shrink = %+v, want pass", s)
	}
	// File absent at base, present at head: every entry is an addition.
	dir2, commit2, write2 := repo(t)
	b2 := commit2("chore: empty")
	write2(exemptionsFile, sha40+" new\n")
	h2 := commit2("chore: introduce list")
	if s := step(Gate{Root: dir2, Base: b2, Head: h2}.Run(), "exemption-growth"); s.Status != "fail" {
		t.Fatalf("new list = %+v, want fail", s)
	}
}

// TestLintReadsPushedHeadNotWorkingTree pins P1 #1: the range lint reads the
// blobs at HEAD. An uncommitted class-A edit must not fail a clean push, and
// an uncommitted fix must not pass a push whose head still carries the defect.
func TestLintReadsPushedHeadNotWorkingTree(t *testing.T) {
	dir, commit, write := repo(t)
	base := commit("chore: base")
	write("new.go", clean)
	head := commit("feat: clean")
	write("new.go", classA) // dirty tree, not pushed
	res := Gate{Root: dir, Base: base, Head: head, Lint: true}.Run()
	if !res.OK {
		t.Fatalf("clean head failed because of an uncommitted edit: %v", res.Findings)
	}

	write("new.go", classA)
	head2 := commit("feat: class A committed")
	write("new.go", clean) // fixed in the tree only
	res = Gate{Root: dir, Base: head, Head: head2, Lint: true}.Run()
	if res.OK || len(res.Findings) != 1 || res.Findings[0].Rule != "A" {
		t.Fatalf("head carrying class A passed because the working tree was fixed: %+v", res)
	}
}

func TestLintBlobsMissingOrUnreadableIsAnError(t *testing.T) {
	_, err := LintBlobs(func(string) ([]byte, error) { return nil, errAbsent }, []string{"a.go"})
	if err == nil || !errors.Is(err, errAbsent) {
		t.Fatalf("absent blob must surface as an error, got %v", err)
	}
	_, err = LintBlobs(func(string) ([]byte, error) { return nil, errors.New("disk") }, []string{"a.ts"})
	if err == nil {
		t.Fatal("read failure must surface as an error")
	}
}

func TestBlobDistinguishesAbsentFromReadFailure(t *testing.T) {
	dir, commit, write := repo(t)
	write("a.txt", "a\n")
	head := commit("chore: one")
	g := Gate{Root: dir}
	if _, err := g.blob(head, "missing.txt"); !errors.Is(err, errAbsent) {
		t.Fatalf("missing path: want errAbsent, got %v", err)
	}
	if _, err := g.blob("not-a-rev", "a.txt"); err == nil {
		t.Fatal("unreadable revision must be an error")
	}
	if b, err := g.blob(head, "a.txt"); err != nil || string(b) != "a\n" {
		t.Fatalf("present blob: %q %v", b, err)
	}
}

func TestLintAllAtHeadSurveysPreExisting(t *testing.T) {
	dir, commit, write := repo(t)
	write("old.go", classA)
	base := commit("chore: pre-existing class A")
	write("new.go", clean)
	head := commit("feat: clean")
	res := Gate{Root: dir, Base: base, Head: head, Lint: true}.Run()
	if !res.OK {
		t.Fatalf("range lint must not block an unrelated push on pre-existing findings: %v", res.Findings)
	}
	res = Gate{Root: dir, Base: base, Head: head, Lint: true, All: true}.Run()
	if len(res.Findings) != 1 || res.Findings[0].Rule != "A" {
		t.Fatalf("--all at head should surface the pre-existing class A finding, got %v", res.Findings)
	}
}

// ── multi-ref pushes (P1 #2) ────────────────────────────────────────────────

func multiRefRepo(t *testing.T) (dir, base, cleanHead, badHead string) {
	t.Helper()
	d, commit, write := repo(t)
	write(traceabilityScript, "#!/usr/bin/env bash\n# fails for a head carrying bad.go\ngit -C \"$PWD\" show \"$3:bad.go\" >/dev/null 2>&1 && exit 1\nexit 0\n")
	base = commit("chore: base")
	write("clean.go", clean)
	cleanHead = commit("feat: clean")
	write("bad.go", clean)
	badHead = commit("feat: bad")
	return d, base, cleanHead, badHead
}

func TestRangesFromPrePushEvaluatesEveryRef(t *testing.T) {
	dir, base, cleanHead, badHead := multiRefRepo(t)
	for name, lines := range map[string]string{
		"clean-first-bad-second": "refs/heads/a " + cleanHead + " refs/heads/a " + base + "\nrefs/heads/b " + badHead + " refs/heads/b " + base + "\n",
		"bad-first-clean-second": "refs/heads/b " + badHead + " refs/heads/b " + base + "\nrefs/heads/a " + cleanHead + " refs/heads/a " + base + "\n",
	} {
		t.Run(name, func(t *testing.T) {
			ranges, err := RangesFromPrePush(dir, strings.NewReader(lines))
			if err != nil || len(ranges) != 2 {
				t.Fatalf("ranges=%v err=%v", ranges, err)
			}
			res := RunRanges(Gate{Root: dir}, ranges)
			if res.OK {
				t.Fatal("a failing second/first ref must fail the push")
			}
			var failed []string
			for _, s := range res.Steps {
				if s.Name == "traceability" && s.Status == "fail" {
					failed = append(failed, s.Ref)
				}
			}
			if len(failed) != 1 || failed[0] != "refs/heads/b" {
				t.Fatalf("failing refs = %v, want exactly refs/heads/b", failed)
			}
		})
	}
}

func TestRangesFromPrePushTagAndDeleteOnlyCarryNothing(t *testing.T) {
	dir, _, cleanHead, _ := multiRefRepo(t)
	ranges, err := RangesFromPrePush(dir, strings.NewReader(
		"refs/tags/v1 "+cleanHead+" refs/tags/v1 "+zero+"\nrefs/heads/x "+zero+" refs/heads/x "+cleanHead+"\n"))
	if err != nil || len(ranges) != 0 {
		t.Fatalf("tag + deletion: ranges=%v err=%v", ranges, err)
	}
}

func TestRangesFromPrePushNewBranchWithoutBaseFailsClosed(t *testing.T) {
	dir, _, cleanHead, _ := multiRefRepo(t) // no origin/* refs exist
	ranges, err := RangesFromPrePush(dir, strings.NewReader("refs/heads/new "+cleanHead+" refs/heads/new "+zero+"\n"))
	if err == nil || !strings.Contains(err.Error(), "refs/heads/new") {
		t.Fatalf("new branch with no resolvable base must be refused, got ranges=%v err=%v", ranges, err)
	}
	// Mixed: one resolvable ref plus one unresolvable — still an error, resolvable one still returned.
	_, base, _, badHead := multiRefRepo(t)
	_ = base
	ranges, err = RangesFromPrePush(dir, strings.NewReader(
		"refs/heads/ok "+cleanHead+" refs/heads/ok "+cleanHead+"\nrefs/heads/new "+badHead+" refs/heads/new "+zero+"\n"))
	if err == nil || len(ranges) != 1 {
		t.Fatalf("mixed refs: want 1 range + error, got %v / %v", ranges, err)
	}
}

func TestRangesFromPrePushNewBranchUsesMergeBase(t *testing.T) {
	dir, base, cleanHead, _ := multiRefRepo(t)
	g := Gate{Root: dir}
	if _, err := g.git("update-ref", "refs/remotes/origin/main", base); err != nil {
		t.Fatal(err)
	}
	ranges, err := RangesFromPrePush(dir, strings.NewReader("refs/heads/new "+cleanHead+" refs/heads/new "+zero+"\n"))
	if err != nil || len(ranges) != 1 || ranges[0].Base != base || ranges[0].Head != cleanHead {
		t.Fatalf("new branch: ranges=%v err=%v, want base=%s", ranges, err, short(base))
	}
}

func TestArmHooksCoversTheRepo(t *testing.T) {
	dir, commit, write := repo(t)
	write(".githooks/pre-push", "#!/bin/sh\nexit 0\n")
	commit("chore: hook")
	if err := ArmHooks(dir); err != nil {
		t.Fatal(err)
	}
	if err := ArmHooks(dir); err != nil { // idempotent
		t.Fatal(err)
	}
	out, _ := Gate{Root: dir}.git("config", "--get", "core.hooksPath")
	if out != ".githooks" {
		t.Fatalf("core.hooksPath = %q", out)
	}
}

func TestRepoRootIsWorktreeAware(t *testing.T) {
	dir, commit, write := repo(t)
	write("sub/a.txt", "a\n")
	commit("chore: one")
	got, err := RepoRoot(filepath.Join(dir, "sub"))
	if err != nil {
		t.Fatal(err)
	}
	want, _ := filepath.EvalSymlinks(dir)
	if g, _ := filepath.EvalSymlinks(got); g != want {
		t.Fatalf("RepoRoot = %q, want %q", got, dir)
	}
}

func step(r Result, name string) Step {
	for _, s := range r.Steps {
		if s.Name == name {
			return s
		}
	}
	return Step{}
}
