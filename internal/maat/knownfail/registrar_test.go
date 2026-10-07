package knownfail

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// buildFakeRepo makes a tiny git repo at devRoot/repoName with an executable
// script committed and referenced by its ci.yml, returning the commit sha.
func buildFakeRepo(t *testing.T, devRoot, repoName, scriptPath string) string {
	t.Helper()
	dir := filepath.Join(devRoot, repoName)
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(os.MkdirAll(filepath.Join(dir, filepath.Dir(scriptPath)), 0o755))
	must(os.MkdirAll(filepath.Join(dir, ".github", "workflows"), 0o755))
	must(os.WriteFile(filepath.Join(dir, scriptPath), []byte("#!/bin/sh\n"), 0o755))
	must(os.WriteFile(filepath.Join(dir, ".github", "workflows", "ci.yml"), []byte("run: "+scriptPath+"\n"), 0o644))
	run := func(args ...string) {
		t.Helper()
		if err := gitCmd(dir, args...).Run(); err != nil {
			t.Fatalf("git %v: %v", args, err)
		}
	}
	run("init", "-q")
	run("config", "user.email", "test@example.com")
	run("config", "user.name", "test")
	run("add", "-A")
	run("commit", "-q", "-m", "init")
	out, err := gitCmd(dir, "rev-parse", "HEAD").Output()
	if err != nil {
		t.Fatal(err)
	}
	return string(out[:40])
}

func TestGuardKindForDetectsCrossRepoRef(t *testing.T) {
	cases := map[string]string{
		"SirsiMaster/sirsi-mercury@6255871abcdef1234567890abcdef1234567890:packaging/test-release-stamps.sh": "cross-repo-script",
		"scripts/x.sh": "script",
		"TestX":        "test",
	}
	for ref, want := range cases {
		if got := GuardKindFor(ref); got != want {
			t.Errorf("GuardKindFor(%q) = %q, want %q", ref, got, want)
		}
	}
}

// A cross-repo guard is only real if the NAMED repo's own local checkout has the
// commit, the blob there is executable, and that repo's CI (at that commit) runs
// it — proven both directions, including against the current working tree
// lying about what the commit actually contains.
func TestCrossRepoGuardExistsProvesAgainstTheOtherReposCommit(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git unavailable")
	}
	devRoot := t.TempDir()
	sha := buildFakeRepo(t, devRoot, "sirsi-mercury", "packaging/test-release-stamps.sh")

	ref := "SirsiMaster/sirsi-mercury@" + sha + ":packaging/test-release-stamps.sh"
	if err := crossRepoGuardExistsIn(devRoot, ref); err != nil {
		t.Fatalf("a real committed+executable+CI-run guard must pass: %v", err)
	}

	if err := crossRepoGuardExistsIn(devRoot, "SirsiMaster/sirsi-mercury@"+sha+":packaging/missing.sh"); err == nil {
		t.Fatal("a path absent at that commit must be refused")
	}
	if err := crossRepoGuardExistsIn(devRoot, "SirsiMaster/no-such-repo@"+sha+":x.sh"); err == nil {
		t.Fatal("a repo with no local checkout must be refused")
	}
	if err := crossRepoGuardExistsIn(devRoot, "SirsiMaster/sirsi-mercury@deadbeefdeadbeefdeadbeefdeadbeefdeadbeef:packaging/test-release-stamps.sh"); err == nil {
		t.Fatal("a commit absent from the local checkout must be refused")
	}
	if err := crossRepoGuardExistsIn(devRoot, "not-a-valid-ref"); err == nil {
		t.Fatal("a malformed ref must be refused")
	}

	// A non-executable blob at the SAME commit must be refused, even though the
	// file exists and is readable.
	nonExecDevRoot := t.TempDir()
	dir := filepath.Join(nonExecDevRoot, "sirsi-mercury")
	if err := os.MkdirAll(filepath.Join(dir, "packaging"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, ".github", "workflows"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "packaging", "test-release-stamps.sh"), []byte("#!/bin/sh\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".github", "workflows", "ci.yml"), []byte("run: packaging/test-release-stamps.sh\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"init", "-q"}, {"config", "user.email", "t@example.com"}, {"config", "user.name", "t"}, {"add", "-A"}, {"commit", "-q", "-m", "init"}} {
		if err := gitCmd(dir, args...).Run(); err != nil {
			t.Fatal(err)
		}
	}
	out, err := gitCmd(dir, "rev-parse", "HEAD").Output()
	if err != nil {
		t.Fatal(err)
	}
	nonExecSHA := string(out[:40])
	if err := crossRepoGuardExistsIn(nonExecDevRoot, "SirsiMaster/sirsi-mercury@"+nonExecSHA+":packaging/test-release-stamps.sh"); err == nil {
		t.Fatal("a non-executable blob must be refused even if CI references the path")
	}
}
