package knownfail

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// buildFakeRepo makes a tiny git repo at devRoot/repoName with an executable
// script committed, referenced by its ci.yml, and an origin remote matching
// owner/repoName — returning the commit sha.
func buildFakeRepo(t *testing.T, devRoot, owner, repoName, scriptPath string) string {
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
	must(os.WriteFile(filepath.Join(dir, ".github", "workflows", "ci.yml"), []byte("jobs:\n  build:\n    steps:\n      - run: "+scriptPath+"\n"), 0o644))
	run := func(args ...string) {
		t.Helper()
		if err := gitCmd(dir, args...).Run(); err != nil {
			t.Fatalf("git %v: %v", args, err)
		}
	}
	run("init", "-q")
	run("config", "user.email", "test@example.com")
	run("config", "user.name", "test")
	run("remote", "add", "origin", "https://github.com/"+owner+"/"+repoName+".git")
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
	// A ".." component anywhere must never classify as cross-repo-script: it
	// must not even reach the directory-traversal check, because the grammar
	// forbids "." in owner/repo entirely.
	for _, ref := range []string{"../etc@6255871abcdef1234567890abcdef1234567890:x", "owner/..@6255871abcdef1234567890abcdef1234567890:x"} {
		if GuardKindFor(ref) == "cross-repo-script" {
			t.Errorf("GuardKindFor(%q) must not classify a dot-segment as cross-repo-script", ref)
		}
	}
}

// A cross-repo guard is only real if the NAMED repo's own local checkout has the
// commit, the blob there is executable, that checkout's origin remote actually
// IS the declared owner/repo, and that repo's CI (at that commit) runs it as a
// real command token — proven both directions.
func TestCrossRepoGuardExistsProvesAgainstTheOtherReposCommit(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git unavailable")
	}
	devRoot := t.TempDir()
	sha := buildFakeRepo(t, devRoot, "SirsiMaster", "sirsi-mercury", "packaging/test-release-stamps.sh")

	ref := "SirsiMaster/sirsi-mercury@" + sha + ":packaging/test-release-stamps.sh"
	if err := crossRepoGuardExistsIn(devRoot, ref); err != nil {
		t.Fatalf("a real committed+executable+CI-run guard must pass: %v", err)
	}

	cases := map[string]string{
		"SirsiMaster/sirsi-mercury@" + sha + ":packaging/missing.sh":                                          "a path absent at that commit must be refused",
		"SirsiMaster/no-such-repo@" + sha + ":x.sh":                                                           "a repo with no local checkout must be refused",
		"SirsiMaster/sirsi-mercury@deadbeefdeadbeefdeadbeefdeadbeefdeadbeef:packaging/test-release-stamps.sh": "a commit absent from the local checkout must be refused",
		"not-a-valid-ref": "a malformed ref must be refused",
		"UnrelatedOwner/sirsi-mercury@" + sha + ":packaging/test-release-stamps.sh": "a checkout whose origin remote names a DIFFERENT owner must be refused (same directory, wrong identity)",
	}
	for ref, msg := range cases {
		if err := crossRepoGuardExistsIn(devRoot, ref); err == nil {
			t.Fatal(msg)
		}
	}
}

// A same-named directory whose origin remote points at an entirely different
// repo (a fork, or an unrelated project sharing the name) must never be
// accepted just because the directory basename matches.
func TestCrossRepoGuardRefusesWrongOriginRemote(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git unavailable")
	}
	devRoot := t.TempDir()
	sha := buildFakeRepo(t, devRoot, "SomeoneElse", "sirsi-mercury", "packaging/test-release-stamps.sh")
	ref := "SirsiMaster/sirsi-mercury@" + sha + ":packaging/test-release-stamps.sh"
	if err := crossRepoGuardExistsIn(devRoot, ref); err == nil {
		t.Fatal("a checkout whose origin remote is a different owner must be refused even though the directory name matches")
	}
}

// Neither a dot-segment repo name nor a nested path can escape devRoot: the
// grammar rejects "." in owner/repo outright, so this never reaches the
// filesystem lookup with an escaped path.
func TestCrossRepoGuardRefusesDotSegmentAndNestedEscape(t *testing.T) {
	devRoot := t.TempDir()
	for _, ref := range []string{
		"SirsiMaster/..@6255871abcdef1234567890abcdef1234567890:x",
		"../etc/SirsiMaster@6255871abcdef1234567890abcdef1234567890:x",
		"SirsiMaster/sub/dir@6255871abcdef1234567890abcdef1234567890:x",
	} {
		if err := crossRepoGuardExistsIn(devRoot, ref); err == nil {
			t.Fatalf("ref %q must be refused as malformed, not resolved to a path", ref)
		}
	}
}

// A non-executable blob at the SAME commit must be refused, even though the
// file exists, is readable, and CI references its path.
func TestCrossRepoGuardRefusesNonExecutableBlob(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git unavailable")
	}
	devRoot := t.TempDir()
	dir := filepath.Join(devRoot, "sirsi-mercury")
	must := func(err error) {
		if err != nil {
			t.Fatal(err)
		}
	}
	must(os.MkdirAll(filepath.Join(dir, "packaging"), 0o755))
	must(os.MkdirAll(filepath.Join(dir, ".github", "workflows"), 0o755))
	must(os.WriteFile(filepath.Join(dir, "packaging", "test-release-stamps.sh"), []byte("#!/bin/sh\n"), 0o644))
	must(os.WriteFile(filepath.Join(dir, ".github", "workflows", "ci.yml"), []byte("run: packaging/test-release-stamps.sh\n"), 0o644))
	for _, args := range [][]string{
		{"init", "-q"}, {"config", "user.email", "t@example.com"}, {"config", "user.name", "t"},
		{"remote", "add", "origin", "https://github.com/SirsiMaster/sirsi-mercury.git"},
		{"add", "-A"}, {"commit", "-q", "-m", "init"},
	} {
		must(gitCmd(dir, args...).Run())
	}
	out, err := gitCmd(dir, "rev-parse", "HEAD").Output()
	if err != nil {
		t.Fatal(err)
	}
	sha := string(out[:40])
	if err := crossRepoGuardExistsIn(devRoot, "SirsiMaster/sirsi-mercury@"+sha+":packaging/test-release-stamps.sh"); err == nil {
		t.Fatal("a non-executable blob must be refused even if CI references the path")
	}
}

// wrapStep puts a run: script inside a minimal real jobs/steps document, the
// only shape ciRunsExactPath reads.
func wrapStep(run string) string {
	return "jobs:\n  build:\n    steps:\n      - run: " + run + "\n"
}

// wrapStepBlock uses a literal block scalar so a multi-line script (or one
// containing a "#") survives verbatim into the run field.
func wrapStepBlock(run string) string {
	indented := strings.ReplaceAll(run, "\n", "\n          ")
	return "jobs:\n  build:\n    steps:\n      - run: |\n          " + indented + "\n"
}

func TestCiRunsExactPathRejectsCommentsAndSuffixMatches(t *testing.T) {
	path := "scripts/guard.sh"
	accept := []string{
		wrapStep("scripts/guard.sh"),
		wrapStep("bash scripts/guard.sh --flag"),
		wrapStep("./scripts/guard.sh"),
		wrapStepBlock("scripts/guard.sh"),
		wrapStepBlock("set -e\nscripts/guard.sh"),
		wrapStepBlock("echo start && scripts/guard.sh"),
	}
	for _, yaml := range accept {
		if !ciRunsExactPath([]byte(yaml), path) {
			t.Errorf("must accept real invocation: %q", yaml)
		}
	}
	reject := []string{
		"# " + path + "\n",                 // a comment, no jobs at all
		wrapStep("echo nothing # " + path), // YAML comment stripped before the shell ever sees it
		wrapStep(path + ".disabled"),       // suffix match, not the path itself
		wrapStep("other-" + path),          // a different, longer path
		wrapStep("echo " + path),           // path is an ARGUMENT to echo, not the command run
		wrapStep("cat " + path),            // path is an ARGUMENT to cat, not the command run
		"jobs:\n  build:\n    steps:\n      - name: " + path + "\n        run: echo nothing\n", // named for the path but never invoked
	}
	for _, yaml := range reject {
		if ciRunsExactPath([]byte(yaml), path) {
			t.Errorf("must reject non-invocation: %q", yaml)
		}
	}
}
