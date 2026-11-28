package trustboundary

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// repo builds a throwaway git repo with a verifier whose exit status is
// controlled by the fixture, so the gate's verdict can be pinned to it.
func repo(t *testing.T) (dir string, commit func(msg string) string, write func(rel, body string)) {
	t.Helper()
	dir = privateFixtureDir(t)
	emptyHooks, emptyTemplate := privateFixtureDir(t), privateFixtureDir(t)
	git := func(args ...string) string {
		t.Helper()
		// Only this newly created private fixture: never a live repo's hooks.
		fixed := []string{"-c", "core.hooksPath=" + emptyHooks, "-c", "commit.gpgsign=false",
			"-c", "tag.gpgsign=false", "-c", "user.name=t", "-c", "user.email=t@t", "-C", dir}
		g := Gate{tools: fixtureGateTools(dir), Root: dir}
		out, stderr, err := g.command("/usr/bin/git", append(fixed, args...)...)
		if err != nil {
			t.Fatalf("private fixture git %v: %v\n%s\n%s", args, err, out, stderr)
		}
		return strings.TrimSpace(string(out))
	}
	git("init", "-q", "-b", "main", "--template="+emptyTemplate)
	if err := os.WriteFile(filepath.Join(dir, ".git", "fixture-gitleaks"), []byte("#!/bin/bash\nprintf 'private-stub\\n' >> \"$0.called\"\nexit 0\n"), 0700); err != nil {
		t.Fatal(err)
	}
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

func privateFixtureDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	physical, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}
	return physical
}

func fixtureGateTools(dir string) *gateTools {
	return &gateTools{git: "/usr/bin/git", bash: "/bin/bash", gitleaks: filepath.Join(dir, ".git", "fixture-gitleaks")}
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

	res := Gate{tools: fixtureGateTools(dir), Root: dir, Base: base, Head: head}.Run()
	if res.OK {
		t.Fatal("gate passed although the verifier exited 1")
	}
	if got := step(res, "traceability").Status; got != "fail" {
		t.Errorf("traceability = %s, want fail", got)
	}
	if got := step(res, "exemption-growth").Status; got != "pass" {
		t.Errorf("exemption-growth = %s, want pass", got)
	}

	// A passing verifier already trusted at the base passes a clean range.
	write(traceabilityScript, "#!/usr/bin/env bash\nexit 0\n")
	base2 := commit("chore: verifier fixed (reviewed separately)")
	write("ok2.go", clean)
	head2 := commit("chore: head2")
	res = Gate{tools: fixtureGateTools(dir), Root: dir, Base: base2, Head: head2}.Run()
	if got := step(res, "traceability").Status; got != "pass" {
		t.Errorf("traceability = %s, want pass: %s", got, step(res, "traceability").Detail)
	}
}

// TestVerifierChangedInRangeFails pins sweep #1: a push cannot attest itself
// by editing, replacing or removing its own verifier. The verifier that runs is
// the BASE's; any difference in range fails with a separate-review message.
func TestVerifierChangedInRangeFails(t *testing.T) {
	dir, commit, write := repo(t)
	write(traceabilityScript, "#!/usr/bin/env bash\nexit 1\n")
	base := commit("chore: strict verifier at base")
	write(traceabilityScript, "#!/usr/bin/env bash\nexit 0\n") // head neuters it
	head := commit("chore: neuter verifier")
	s := step(Gate{tools: fixtureGateTools(dir), Root: dir, Base: base, Head: head}.Run(), "traceability")
	if s.Status != "fail" || !strings.Contains(s.Detail, "verifier changed in range") {
		t.Fatalf("neutered verifier: %+v, want fail 'verifier changed in range'", s)
	}
	// Removed at head: same refusal.
	if err := os.Remove(filepath.Join(dir, traceabilityScript)); err != nil {
		t.Fatal(err)
	}
	head2 := commit("chore: remove verifier")
	if s := step(Gate{tools: fixtureGateTools(dir), Root: dir, Base: base, Head: head2}.Run(), "traceability"); s.Status != "fail" || !strings.Contains(s.Detail, "verifier changed in range") {
		t.Fatalf("removed verifier: %+v, want fail", s)
	}
	// Absent at both: a reported skip, pinned to the revisions.
	d2, c2, w2 := repo(t)
	b2 := c2("chore: no verifier")
	w2("a.go", clean)
	h2 := c2("chore: file")
	if s := step(Gate{tools: fixtureGateTools(d2), Root: d2, Base: b2, Head: h2}.Run(), "traceability"); s.Status != "skip" || !strings.Contains(s.Detail, short(b2)) {
		t.Fatalf("absent at both: %+v, want skip naming the revisions", s)
	}
}

// TestVerifierRunsFromBaseNotWorkingTree: the working tree holds a passing
// verifier, base and head hold a failing one — the committed base's verdict wins.
func TestVerifierRunsFromBaseNotWorkingTree(t *testing.T) {
	dir, commit, write := repo(t)
	write(traceabilityScript, "#!/usr/bin/env bash\nexit 1\n")
	base := commit("chore: failing verifier")
	write("a.go", clean)
	head := commit("chore: file")
	write(traceabilityScript, "#!/usr/bin/env bash\nexit 0\n") // uncommitted: not what is being pushed
	if got := step(Gate{tools: fixtureGateTools(dir), Root: dir, Base: base, Head: head}.Run(), "traceability").Status; got != "fail" {
		t.Fatalf("traceability = %s, want fail from the committed blob", got)
	}
}

func TestExemptionSetAdditionRefusedEvenAtEqualCount(t *testing.T) {
	dir, commit, write := repo(t)
	write(exemptionsFile, "# header\n"+sha40+" fix(a): audited\n")
	base := commit("chore: base")
	// Equal-count REPLACEMENT: one hash swapped for another. A count check passes this.
	write(exemptionsFile, "# header\n"+sha40b+" fix(b): I admit myself\n")
	head := commit("chore: swap")
	res := Gate{tools: fixtureGateTools(dir), Root: dir, Base: "HEAD~1", Head: "HEAD"}.Run() // symbolic revs resolve to the exact SHAs
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
	if s := step(Gate{tools: fixtureGateTools(dir), Root: dir, Base: base, Head: head}.Run(), "exemption-growth"); s.Status != "pass" {
		t.Fatalf("shrink = %+v, want pass", s)
	}
	// File absent at base, present at head: every entry is an addition.
	dir2, commit2, write2 := repo(t)
	b2 := commit2("chore: empty")
	write2(exemptionsFile, sha40+" new\n")
	h2 := commit2("chore: introduce list")
	if s := step(Gate{tools: fixtureGateTools(dir2), Root: dir2, Base: b2, Head: h2}.Run(), "exemption-growth"); s.Status != "fail" {
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
	lintStep, findings := (Gate{tools: fixtureGateTools(dir), Root: dir, Base: base, Head: head}).lint(true)
	if lintStep.Status != "pass" {
		t.Fatalf("clean head failed because of an uncommitted edit: %v / %v", lintStep, findings)
	}

	write("new.go", classA)
	head2 := commit("feat: class A committed")
	write("new.go", clean) // fixed in the tree only
	lintStep, findings = (Gate{tools: fixtureGateTools(dir), Root: dir, Base: head, Head: head2}).lint(true)
	if lintStep.Status != "fail" || len(findings) != 1 || findings[0].Rule != "A" {
		t.Fatalf("head carrying class A passed because the working tree was fixed: step=%+v findings=%+v", lintStep, findings)
	}
}

// TestRenamedAndEditedUnsafeFileIsLinted pins sweep #2: a rename-with-edit is
// not a "rename" to the gate (--no-renames) — the new path is linted.
func TestRenamedAndEditedUnsafeFileIsLinted(t *testing.T) {
	dir, commit, write := repo(t)
	write("old.go", clean)
	base := commit("chore: base")
	if err := os.Remove(filepath.Join(dir, "old.go")); err != nil {
		t.Fatal(err)
	}
	write("renamed.go", strings.Replace(classA, "package x", "package x\n// moved", 1))
	head := commit("refactor: rename + edit")
	lintStep, findings := (Gate{tools: fixtureGateTools(dir), Root: dir, Base: base, Head: head}).lint(true)
	if lintStep.Status != "fail" || len(findings) != 1 || findings[0].File != "renamed.go" || findings[0].Rule != "A" {
		t.Fatalf("renamed+edited class A must be flagged, got step=%+v findings=%+v", lintStep, findings)
	}
}

// TestNonASCIIPathIsLinted pins sweep #3: paths come back NUL-separated and
// exact, so a name with a space and non-ASCII is read, not skipped as quoted.
func TestNonASCIIPathIsLinted(t *testing.T) {
	dir, commit, write := repo(t)
	base := commit("chore: base")
	write("héllo wörld.go", classA)
	head := commit("feat: odd path")
	lintStep, findings := (Gate{tools: fixtureGateTools(dir), Root: dir, Base: base, Head: head}).lint(true)
	if lintStep.Status != "fail" || len(findings) != 1 || findings[0].File != "héllo wörld.go" {
		t.Fatalf("non-ASCII path must be linted, got step=%+v findings=%+v", lintStep, findings)
	}
	lintStep, findings = (Gate{tools: fixtureGateTools(dir), Root: dir, Base: base, Head: head, All: true}).lint(true)
	if lintStep.Status != "fail" || len(findings) != 1 {
		t.Fatalf("--all at head must lint the odd path too, got step=%+v findings=%+v", lintStep, findings)
	}
	if _, err := LintBlobs(func(string) ([]byte, error) { return nil, errAbsent }, []string{`"h\303\251llo.go"`}); err == nil || !strings.Contains(err.Error(), "quoted path") {
		t.Fatalf("a quoted path must be an error, got %v", err)
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
	g := Gate{tools: fixtureGateTools(dir), Root: dir}
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
	lintStep, findings := (Gate{tools: fixtureGateTools(dir), Root: dir, Base: base, Head: head}).lint(true)
	if lintStep.Status != "pass" {
		t.Fatalf("range lint must not block an unrelated push on pre-existing findings: %v / %v", lintStep, findings)
	}
	lintStep, findings = (Gate{tools: fixtureGateTools(dir), Root: dir, Base: base, Head: head, All: true}).lint(true)
	if lintStep.Status != "fail" || len(findings) != 1 || findings[0].Rule != "A" {
		t.Fatalf("--all at head should surface the pre-existing class A finding, got %v / %v", lintStep, findings)
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
			res := RunRanges(Gate{tools: fixtureGateTools(dir), Root: dir}, ranges)
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
		"refs/tags/v1 "+cleanHead+" refs/tags/v1 "+zero+"\n(delete) "+zero+" refs/heads/x "+cleanHead+"\n"))
	if err != nil || len(ranges) != 0 {
		t.Fatalf("tag + deletion: ranges=%v err=%v", ranges, err)
	}
}

func TestRangesFromPrePushTagSourceToBranchDestinationIsGated(t *testing.T) {
	dir, base, cleanHead, _ := multiRefRepo(t)
	input := "refs/tags/release " + cleanHead + " refs/heads/production " + base + "\n"
	ranges, err := RangesFromPrePush(dir, strings.NewReader(input))
	if err != nil || len(ranges) != 1 {
		t.Fatalf("tag-to-branch update must be gated: ranges=%v err=%v", ranges, err)
	}
	if ranges[0].Ref != "refs/heads/production" || ranges[0].Base != base || ranges[0].Head != cleanHead {
		t.Fatalf("tag-to-branch range = %+v; want remote branch destination and exact range", ranges[0])
	}
}

func TestRangesFromPrePushRejectsMalformedOnlyAndMixedRecords(t *testing.T) {
	dir, base, cleanHead, _ := multiRefRepo(t)
	malformed := "refs/heads/broken " + cleanHead + " refs/heads/broken\n"
	if ranges, err := RangesFromPrePush(dir, strings.NewReader(malformed)); err == nil || len(ranges) != 0 {
		t.Fatalf("malformed-only input must fail closed: ranges=%v err=%v", ranges, err)
	}
	valid := "refs/heads/valid " + cleanHead + " refs/heads/valid " + base + "\n"
	ranges, err := RangesFromPrePush(dir, strings.NewReader(valid+malformed))
	if err == nil || len(ranges) != 1 || ranges[0].Head != cleanHead || !strings.Contains(err.Error(), "line 2") {
		t.Fatalf("valid + malformed must return the valid range and an error: ranges=%v err=%v", ranges, err)
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
	g := Gate{tools: fixtureGateTools(dir), Root: dir}
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
	privateHome := privateFixtureDir(t)
	t.Setenv("HOME", privateHome)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(privateHome, "xdg"))
	write(".githooks/pre-push", "#!/bin/sh\nexit 0\n")
	if err := os.Chmod(filepath.Join(dir, ".githooks", "pre-push"), 0755); err != nil {
		t.Fatal(err)
	}
	commit("chore: hook")
	if err := ArmHooks(dir); err != nil {
		t.Fatal(err)
	}
	if err := ArmHooks(dir); err != nil { // idempotent
		t.Fatal(err)
	}
	out, _ := Gate{tools: fixtureGateTools(dir), Root: dir}.git("config", "--get", "core.hooksPath")
	if out != ".githooks" {
		t.Fatalf("core.hooksPath = %q", out)
	}
}

func TestArmHooksRefusesNonExecutableHookWithoutChangingConfig(t *testing.T) {
	for _, tc := range []struct {
		name string
		mode os.FileMode
		body string
		link bool
	}{
		{name: "not-executable", mode: 0644, body: "#!/bin/sh\nexit 0\n"},
		{name: "directory", mode: 0755},
		{name: "symlink-to-executable", mode: 0755, body: "#!/bin/sh\nexit 0\n", link: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir, commit, write := repo(t)
			privateHome := privateFixtureDir(t)
			t.Setenv("HOME", privateHome)
			t.Setenv("XDG_CONFIG_HOME", filepath.Join(privateHome, "xdg"))
			hook := filepath.Join(dir, ".githooks", "pre-push")
			if tc.name == "directory" {
				if err := os.MkdirAll(hook, tc.mode); err != nil {
					t.Fatal(err)
				}
			} else if tc.link {
				if err := os.MkdirAll(filepath.Dir(hook), 0755); err != nil {
					t.Fatal(err)
				}
				target := filepath.Join(dir, "real-pre-push")
				if err := os.WriteFile(target, []byte(tc.body), tc.mode); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(target, hook); err != nil {
					t.Fatal(err)
				}
			} else {
				write(".githooks/pre-push", tc.body)
				if err := os.Chmod(hook, tc.mode); err != nil {
					t.Fatal(err)
				}
			}
			commit("chore: invalid hook")
			if err := ArmHooks(dir); err == nil {
				t.Fatal("non-executable or non-regular hook must be refused")
			}
			if got, err := (Gate{tools: fixtureGateTools(dir), Root: dir}).git("config", "--local", "--get", "core.hooksPath"); err == nil {
				t.Fatalf("hook path was configured despite refusal: %q", got)
			}
		})
	}
}

func TestArmHooksRefusesConflictingLocalPath(t *testing.T) {
	dir, commit, write := repo(t)
	write(".githooks/pre-push", "#!/bin/sh\nexit 0\n")
	commit("chore: hook")
	g := Gate{tools: fixtureGateTools(dir), Root: dir}
	if _, err := g.git("config", "--local", "core.hooksPath", "/custom/hooks"); err != nil {
		t.Fatal(err)
	}
	if err := ArmHooks(dir); err == nil || !strings.Contains(err.Error(), "/custom/hooks") {
		t.Fatalf("conflicting local hook path must be refused, got %v", err)
	}
	got, err := g.git("config", "--local", "--get", "core.hooksPath")
	if err != nil || got != "/custom/hooks" {
		t.Fatalf("local hook path changed after refusal: got=%q err=%v", got, err)
	}
}

func TestArmHooksRefusesConflictingGlobalPath(t *testing.T) {
	dir, commit, write := repo(t)
	write(".githooks/pre-push", "#!/bin/sh\nexit 0\n")
	commit("chore: hook")
	privateHome := privateFixtureDir(t)
	if err := os.WriteFile(filepath.Join(privateHome, ".gitconfig"), []byte("[core]\n\thooksPath = /custom/global-hooks\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", privateHome)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(privateHome, "xdg"))
	if err := ArmHooks(dir); err == nil || !strings.Contains(err.Error(), "/custom/global-hooks") {
		t.Fatalf("conflicting global hook path must be refused, got %v", err)
	}
	if got, err := (Gate{tools: fixtureGateTools(dir), Root: dir}).git("config", "--local", "--get", "core.hooksPath"); err == nil {
		t.Fatalf("local hook path should remain unset after global conflict, got %q", got)
	}
}

func TestResultSeparatesSkippedChecksFromCompletePass(t *testing.T) {
	dir, _, _ := repo(t)
	result := (Gate{tools: fixtureGateTools(dir), Root: dir}).Run()
	if !result.OK || result.Complete || result.Scope != "working-tree-survey" {
		t.Fatalf("working-tree survey summary = %+v; want no executed failure but incomplete scope", result)
	}
	if len(result.Skipped) == 0 {
		t.Fatalf("expected machine-readable skipped steps, got %+v", result)
	}
	data, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{`"scope":"working-tree-survey"`, `"complete":false`, `"skipped":`} {
		if !strings.Contains(string(data), field) {
			t.Errorf("JSON result lacks %s: %s", field, data)
		}
	}
}

func TestLintOnlyExplicitRangeResultIsRefusedAndIncomplete(t *testing.T) {
	result := (Gate{Base: sha40, Head: sha40b, Lint: true}).Run()
	if result.OK || result.Complete || result.Scope != "lint-only" {
		t.Fatalf("explicit-range lint-only result = %+v; want refusal, not a complete pass", result)
	}
	if got := step(result, "mode"); got.Status != "fail" {
		t.Fatalf("mode status = %q; want fail", got.Status)
	}
}

func TestNoRangeLintOnlyResultRemainsCompatibilityScope(t *testing.T) {
	dir, _, _ := repo(t)
	result := (Gate{tools: fixtureGateTools(dir), Root: dir, Lint: true, All: true}).Run()
	if result.Scope != "lint-only" || !result.OK || !result.Complete || len(result.Skipped) != 0 {
		t.Fatalf("no-range lint-only result = %+v; want its own completed informational scope", result)
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

// A caller-controlled startup file must never substitute an exit0 for the
// BASE verifier. Policy/credential variables are checked without printing them.
func TestVerifierIgnoresStartupAndPolicyEnvironment(t *testing.T) {
	for _, shouldFail := range []bool{false, true} {
		t.Run(fmt.Sprintf("verifier_fails_%v", shouldFail), func(t *testing.T) {
			dir, commit, write := repo(t)
			poison := filepath.Join(privateFixtureDir(t), "startup.sh")
			marker := poison + ".called"
			if err := os.WriteFile(poison, []byte("printf poisoned > '"+strings.ReplaceAll(marker, "'", "'\\''")+"'\nexit 0\n"), 0700); err != nil {
				t.Fatal(err)
			}
			t.Setenv("BASH_ENV", poison)
			t.Setenv("ENV", poison)
			t.Setenv("TRACEABILITY_ENFORCEMENT_BOUNDARY", "caller_must_not_select_policy")
			t.Setenv("AWS_SECRET_ACCESS_KEY", "inert-fixture-not-a-credential")
			exit := "0"
			if shouldFail {
				exit = "47"
			}
			write(traceabilityScript, "#!/bin/bash\nif [ -n \"${BASH_ENV+x}${ENV+x}${TRACEABILITY_ENFORCEMENT_BOUNDARY+x}${AWS_SECRET_ACCESS_KEY+x}\" ]; then exit 48; fi\nexit "+exit+"\n")
			base := commit("fixture: trusted verifier")
			write("ok.go", clean)
			head := commit("fixture: clean head")
			result := Gate{tools: fixtureGateTools(dir), Root: dir, Base: base, Head: head}.Run()
			want := "pass"
			if shouldFail {
				want = "fail"
			}
			if got := step(result, "traceability"); got.Status != want || (shouldFail && !strings.Contains(got.Detail, "exit status 47")) {
				t.Fatalf("BASE verifier status=%s; expected %s with original exit preserved", got.Status, want)
			}
			if result.OK == shouldFail {
				t.Fatal("whole gate lost the verifier's exit verdict")
			}
			if len(result.Steps) != 4 {
				t.Fatal("a verifier failure short-circuited another step")
			}
			if _, err := os.Stat(marker); !os.IsNotExist(err) {
				t.Fatal("caller startup file executed or marker observation failed")
			}
		})
	}
}

func TestNULPathsAreExactWithCleanShadow(t *testing.T) {
	for _, prefix := range []string{" ", "\t", "\n"} {
		for _, all := range []bool{false, true} {
			t.Run(fmt.Sprintf("prefix_%q_all_%v", prefix, all), func(t *testing.T) {
				dir, commit, write := repo(t)
				write("unsafe.go", clean) // formerly selected by TrimSpace instead
				base := commit("fixture: clean shadow")
				name := prefix + "unsafe.go"
				write(name, classA)
				head := commit("fixture: actual unsafe filename")
				lintStep, findings := (Gate{tools: fixtureGateTools(dir), Root: dir, Base: base, Head: head, All: all}).lint(true)
				if lintStep.Status != "fail" || len(findings) != 1 || findings[0].File != name || findings[0].Rule != "A" {
					t.Fatal("lint did not bind the exact first NUL-delimited filename")
				}
			})
		}
	}
}

func TestPartialRangeRefusesBeforeAnyObservation(t *testing.T) {
	dir, commit, write := repo(t)
	write("clean.go", clean)
	head := commit("fixture: valid endpoint and clean survey")
	for _, request := range []Gate{{Base: head}, {Head: head}} {
		request.Root = dir
		request.tools = fixtureGateTools(dir)
		result := request.Run()
		if result.OK || len(result.Steps) != 1 || result.Steps[0].Name != "range" || result.Steps[0].Status != "fail" || !strings.Contains(result.Steps[0].Detail, "together") {
			t.Fatal("partial endpoint silently became a survey or external observation")
		}
	}
}

func TestBoundedCommandPreservesExitAndExactOutput(t *testing.T) {
	g := Gate{Root: privateFixtureDir(t), commandLimit: 4}
	out, _, err := g.command("/bin/bash", "--noprofile", "--norc", "-c", "printf 1234")
	if err != nil || string(out) != "1234" {
		t.Fatal("exact byte ceiling was incorrectly truncated/refused")
	}
	_, _, err = g.command("/bin/bash", "--noprofile", "--norc", "-c", "exit 7")
	var exited *exec.ExitError
	if !errors.As(err, &exited) || exited.ExitCode() != 7 {
		t.Fatal("direct verifier exit status was lost")
	}
}

func TestBoundedCommandOverflowIsNeverSuccess(t *testing.T) {
	g := Gate{Root: privateFixtureDir(t), commandLimit: 4}
	_, _, err := g.command("/bin/bash", "--noprofile", "--norc", "-c", "printf 12345")
	if !errors.Is(err, errGateOutputLimit) {
		t.Fatal("stdout overflow did not explicitly refuse")
	}
	_, _, err = g.command("/bin/bash", "--noprofile", "--norc", "-c", "printf '%65537s' x >&2")
	if !errors.Is(err, errGateOutputLimit) {
		t.Fatal("stderr overflow did not explicitly refuse")
	}
}

func TestBoundedCommandDeadlineIsNeverSuccess(t *testing.T) {
	g := Gate{Root: privateFixtureDir(t), commandTimeout: 20 * time.Millisecond}
	// exec retains the direct child's PID; no background or descendant fixture.
	_, _, err := g.command("/bin/bash", "--noprofile", "--norc", "-c", "exec /bin/sleep 5")
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("deadline did not return an explicit failure")
	}
}

func TestGateCommandEnvironmentIsClosed(t *testing.T) {
	for _, name := range []string{"BASH_ENV", "ENV", "TRACEABILITY_ENFORCEMENT_BOUNDARY", "AWS_SECRET_ACCESS_KEY", "GIT_CONFIG_GLOBAL", "SIRSI_ROUTER_TOKEN", "ALL_PROXY"} {
		t.Setenv(name, "inert-parent-canary")
	}
	env := map[string]string{}
	for _, pair := range gateCommandEnv("/private/fixture-home") {
		key, value, ok := strings.Cut(pair, "=")
		if !ok || env[key] != "" {
			t.Fatal("malformed or duplicated child environment key")
		}
		env[key] = value
	}
	for _, forbidden := range []string{"BASH_ENV", "ENV", "TRACEABILITY_ENFORCEMENT_BOUNDARY", "AWS_SECRET_ACCESS_KEY", "SIRSI_ROUTER_TOKEN", "ALL_PROXY"} {
		if _, exists := env[forbidden]; exists {
			t.Fatal("ambient policy/startup/credential key crossed child boundary")
		}
	}
	if env["GIT_CONFIG_GLOBAL"] != os.DevNull || env["GIT_CONFIG_SYSTEM"] != os.DevNull || env["GIT_CONFIG_NOSYSTEM"] != "1" || env["HOME"] != "/private/fixture-home" {
		t.Fatal("child Git configuration/home was not fixed")
	}
}

func TestPrivateGitFixtureIgnoresAmbientHooksSigningAndTools(t *testing.T) {
	hostile := privateFixtureDir(t)
	marker := filepath.Join(hostile, "must-not-run")
	poison := filepath.Join(hostile, "poison")
	code := "#!/bin/bash\nprintf poisoned > '" + strings.ReplaceAll(marker, "'", "'\\''") + "'\nexit 0\n"
	if err := os.WriteFile(poison, []byte(code), 0700); err != nil {
		t.Fatal(err)
	}
	hooks := filepath.Join(hostile, "hooks")
	if err := os.Mkdir(hooks, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(hooks, "pre-commit"), []byte(code), 0700); err != nil {
		t.Fatal(err)
	}
	config := filepath.Join(hostile, ".gitconfig")
	configText := "[core]\n hooksPath = " + hooks + "\n[commit]\n gpgSign = true\n[gpg]\n program = " + poison + "\n"
	if err := os.WriteFile(config, []byte(configText), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(hostile, "git"), []byte(code), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(hostile, "gitleaks"), []byte(code), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", hostile)
	t.Setenv("PATH", hostile)
	t.Setenv("GIT_CONFIG_GLOBAL", config)
	t.Setenv("GIT_TEMPLATE_DIR", hostile)
	t.Setenv("GIT_CONFIG_COUNT", "1")
	t.Setenv("GIT_CONFIG_KEY_0", "core.hooksPath")
	t.Setenv("GIT_CONFIG_VALUE_0", hooks)
	t.Setenv("BASH_ENV", poison)
	t.Setenv("GITLEAKS_CONFIG", poison)
	dir, commit, write := repo(t)
	base := commit("fixture: private base")
	write("ok.go", clean)
	head := commit("fixture: private head")
	result := Gate{tools: fixtureGateTools(dir), Root: dir, Base: base, Head: head}.Run()
	if !result.OK || step(result, "secrets").Status != "pass" {
		t.Fatal("private fixture gate did not complete using its own tool seam")
	}
	if _, err := os.Stat(filepath.Join(dir, ".git", "fixture-gitleaks.called")); err != nil {
		t.Fatal("private gitleaks stub was not the executed scanner")
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatal("ambient hook/signer/startup/PATH tool executed or marker observation failed")
	}
}
