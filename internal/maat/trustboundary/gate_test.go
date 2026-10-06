package trustboundary

import (
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
		git("commit", "-q", "-m", msg)
		return git("rev-parse", "HEAD")
	}
	return dir, commit, write
}

const sha40 = "0123456789abcdef0123456789abcdef01234567"

func TestGateExitStatusIsTheVerdict(t *testing.T) {
	dir, commit, write := repo(t)
	// A verifier that prints a cheerful line and exits 1 — exactly the shape
	// `… | tail && push` used to let through.
	write(traceabilityScript, "#!/usr/bin/env bash\necho 'commit traceability verification passed'\nexit 1\n")
	write(exemptionsFile, "# header\n")
	base := commit("chore: base")
	write("ok.go", "package ok\n")
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

func TestGateRefusesExemptionGrowth(t *testing.T) {
	dir, commit, write := repo(t)
	write(exemptionsFile, "# header\n")
	base := commit("chore: base")
	write(exemptionsFile, "# header\n"+sha40+" fix(x): I admit myself\n")
	head := commit("chore: self-exemption")

	res := Gate{Root: dir, Base: "HEAD~1", Head: "HEAD"}.Run() // symbolic revs resolve to the exact SHAs
	s := step(res, "exemption-growth")
	_, _ = base, head
	if s.Status != "fail" || !strings.Contains(s.Detail, "grew from 0 to 1") {
		t.Fatalf("exemption-growth = %+v, want fail with growth count", s)
	}
	if res.OK {
		t.Fatal("gate passed with a new exemption in range")
	}
}

func TestGateLintsOnlyChangedFiles(t *testing.T) {
	dir, commit, write := repo(t)
	write("old.go", "package x\n\nimport \"strconv\"\n\nfunc f(s string) []int { n, _ := strconv.Atoi(s); return make([]int, n) }\n")
	base := commit("chore: pre-existing class A")
	write("new.go", "package x\n\nfunc g() int { return 1 }\n")
	head := commit("feat: clean")

	res := Gate{Root: dir, Base: base, Head: head}.Run()
	if got := step(res, "trust-boundary-lint").Status; got != "pass" {
		t.Fatalf("lint on range = %s (%v); pre-existing findings must not block an unrelated push", got, res.Findings)
	}
	res = Gate{Root: dir, Base: base, Head: head, All: true}.Run()
	if len(res.Findings) != 1 || res.Findings[0].Rule != "A" {
		t.Fatalf("--all should surface the pre-existing class A finding, got %v", res.Findings)
	}
}

func TestRangeFromPrePush(t *testing.T) {
	dir, commit, write := repo(t)
	write("a.txt", "a\n")
	head := commit("chore: one")
	const zero = "0000000000000000000000000000000000000000"
	base, got, ok := RangeFromPrePush(dir, strings.NewReader("refs/heads/x "+head+" refs/heads/x "+sha40+"\n"))
	if !ok || base != sha40 || got != head {
		t.Fatalf("existing branch: got %q %q %v", base, got, ok)
	}
	if _, _, ok := RangeFromPrePush(dir, strings.NewReader("refs/tags/v1 "+head+" refs/tags/v1 "+zero+"\nrefs/heads/x "+zero+" refs/heads/x "+head+"\n")); ok {
		t.Fatal("tag push + deletion must carry no content")
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

func step(r Result, name string) Step {
	for _, s := range r.Steps {
		if s.Name == name {
			return s
		}
	}
	return Step{}
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
