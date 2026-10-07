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

// TestCiRunsExactPathRejectsShellTextAndPathSuffixes covers the four
// non-proof forms codex-pantheon found still accepted at 2c387915: a
// statement separator or comment marker hidden inside quotes/comments/
// heredoc data, and a different path merely sharing a suffix with the
// pinned one.
func TestCiRunsExactPathRejectsShellTextAndPathSuffixes(t *testing.T) {
	path := "scripts/guard.sh"
	reject := []string{
		wrapStepBlock("echo 'x; " + path + "'"),
		wrapStepBlock("echo nothing # x; " + path),
		wrapStepBlock("cat <<'EOF'\n" + path + "\nEOF"),
		wrapStepBlock("bash /tmp/other/" + path),
	}
	for _, yaml := range reject {
		if ciRunsExactPath([]byte(yaml), path) {
			t.Errorf("must reject non-invocation: %q", yaml)
		}
	}
	accept := []string{
		wrapStepBlock("echo start ; " + path),
		wrapStepBlock("echo 'quoted arg' && " + path),
	}
	for _, yaml := range accept {
		if !ciRunsExactPath([]byte(yaml), path) {
			t.Errorf("must accept real invocation: %q", yaml)
		}
	}
}

// TestSplitShellStatementsHandlesMultilineQuotesAndEscapes covers the
// automated security review's follow-up on e9f8570e: quote state must
// survive a newline (a single/double-quoted string legitimately spans
// lines), a backslash must escape the next character (including a line
// continuation) rather than being read specially, and `#` must start a
// comment right after a statement separator, not only after whitespace.
func TestSplitShellStatementsHandlesMultilineQuotesAndEscapes(t *testing.T) {
	path := "scripts/guard.sh"

	hasPathAsWord0 := func(stmts [][]string) bool {
		for _, words := range stmts {
			if len(words) > 0 && words[0] == path {
				return true
			}
		}
		return false
	}

	// A single-quoted string spanning a newline must stay one quoted
	// token: the embedded newline is data, not a statement separator,
	// so the path on the second physical line is still inside the quote.
	stmts := splitShellStatements("echo 'line one\n" + path + "'")
	if hasPathAsWord0(stmts) {
		t.Errorf("multiline single-quote leaked the path into command position: stmts=%q", stmts)
	}

	// A backslash-escaped separator is literal text, not a split point.
	stmts = splitShellStatements(`echo foo\;` + path)
	if hasPathAsWord0(stmts) {
		t.Errorf("backslash-escaped ';' must not split: stmts=%q", stmts)
	}

	// '#' right after a statement separator (no intervening space) still
	// starts a comment.
	stmts = splitShellStatements("true;# " + path)
	if hasPathAsWord0(stmts) {
		t.Errorf("comment immediately after ';' must not count: stmts=%q", stmts)
	}

	// A backslash-newline line continuation joins two lines without
	// introducing a statement split or quoting the path.
	stmts = splitShellStatements("echo start && \\\n" + path)
	if !hasPathAsWord0(stmts) {
		t.Errorf("line continuation must still let the following real invocation match: stmts=%q", stmts)
	}
}

// TestCiRunsExactPathRejectsWordIdentityAndHeredocDelimiterFalsePositives
// covers codex-pantheon's successor review on fa2d6059 (item
// 20261007-124810): strings.Fields re-splitting an already-built statement
// string on whitespace discards the quote/escape awareness that built it
// (so a quoted or escaped embedded space could still smuggle a second
// "word" in), and the heredoc delimiter regexp silently truncated at the
// first non-identifier character while treating `<<` (exact-match
// termination) the same as `<<-` (leading-tab-stripped termination).
func TestCiRunsExactPathRejectsWordIdentityAndHeredocDelimiterFalsePositives(t *testing.T) {
	path := "scripts/guard.sh"
	reject := []string{
		// The "command" is one word containing an escaped space, so it is
		// not literally scripts/guard.sh as a shell would invoke it.
		wrapStepBlock(`scripts/guard.sh\ --fake`),
		wrapStepBlock(`'scripts/guard.sh --fake'`),
		// Plain `<<EOF` requires an EXACT body-line match (no stripping);
		// the indented " EOF" line must NOT end the heredoc early, so the
		// real path line stays heredoc data.
		wrapStepBlock("cat <<EOF\n EOF\n" + path + "\nEOF"),
		// A quoted delimiter containing a non-identifier character must be
		// matched in FULL, not truncated to its leading identifier prefix.
		wrapStepBlock("cat <<'END-TEXT'\nEND\n" + path + "\nEND-TEXT"),
		// Heredoc bodies (plain and dash-form) are data, never commands,
		// even though the path is the only "statement" visible per line.
		wrapStepBlock("cat <<EOF\n" + path + "\nEOF"),
		wrapStepBlock("cat <<-EOF\n\t" + path + "\n\tEOF"),
	}
	for _, yaml := range reject {
		if ciRunsExactPath([]byte(yaml), path) {
			t.Errorf("must reject non-invocation: %q", yaml)
		}
	}
}

// TestCiRunsExactPathRejectsEmptyWordsEscapesAndQueuedHeredocs covers
// codex-pantheon's successor review on 833eb725 (item 20261007-132511):
// (1) an explicit empty quoted argument/command was silently dropped
// because endWord only kept words with nonzero builder length, which let
// the NEXT real word slide into command position; (2) inside double quotes
// a backslash was stripped before ANY character, when POSIX only grants it
// that meaning before $, `, ", \, or a line-continuation newline — before
// any other character the backslash is literal; (3) a line opening more
// than one heredoc (`cmd <<A <<B`) lost the second queued delimiter and let
// its body fall through as a real statement.
func TestCiRunsExactPathRejectsEmptyWordsEscapesAndQueuedHeredocs(t *testing.T) {
	path := "scripts/guard.sh"
	reject := []string{
		// bash exits 127 trying to run "" as $0; scripts/guard.sh is never
		// reached as the executed command.
		wrapStepBlock(`bash "" ` + path),
		wrapStepBlock(`"" ` + path),
		// Backslash before a non-escapable character inside double quotes
		// is literal, so this names a DIFFERENT path (with a backslash in
		// it), not scripts/guard.sh.
		wrapStepBlock(`"scripts/\guard.sh"`),
		// Two heredocs opened on one line must both be tracked in order;
		// the real shell prints the path as heredoc-B data, never runs it.
		wrapStepBlock("cat <<A <<B\nfirst\nA\n" + path + "\nB"),
	}
	for _, yaml := range reject {
		if ciRunsExactPath([]byte(yaml), path) {
			t.Errorf("must reject non-invocation: %q", yaml)
		}
	}
	accept := []string{
		// A backslash before an escapable character (") inside double
		// quotes is consumed, so it does NOT end the string early — the
		// real invocation after && must still be found.
		wrapStepBlock(`echo "say \"hi\"" && ` + path),
		// bash with a REAL first argument still finds the path as $2.
		wrapStepBlock("bash " + path),
	}
	for _, yaml := range accept {
		if !ciRunsExactPath([]byte(yaml), path) {
			t.Errorf("must accept real invocation: %q", yaml)
		}
	}
}

// TestSplitShellStatementsTracksWordBoundaryThroughNonWhitespaceRunes covers
// automated security review on 6bc5bc96: addRune stopped clearing
// atBoundary, so it stayed true (its zero-value) past any non-whitespace
// character that didn't happen to toggle a quote. A `#` immediately after
// such a character (e.g. `x#`) was then wrongly treated as starting a
// comment — a REAL shell only treats `#` as a comment opener at the start
// of a word (preceded by whitespace or the start of script), never stuck to
// the end of the previous token. Skipping the "comment" also skipped the
// quote character it happened to contain WITHOUT opening that quote, so a
// single-quoted guard-path-as-data block after it was never actually
// quoted: the path then parsed as a normal, unquoted top-level statement
// and matched — a verification bypass, not just an over-rejection.
func TestSplitShellStatementsTracksWordBoundaryThroughNonWhitespaceRunes(t *testing.T) {
	path := "scripts/guard.sh"
	reject := []string{
		// The guard path is supposed to be heredoc-shaped data inside a
		// single-quoted block; if the preceding '#' is wrongly treated as
		// a mid-word comment, the quote char is skipped unopened and the
		// path below parses as a real top-level statement.
		wrapStepBlock("echo x#'\n" + path + "\n'"),
		wrapStepBlock("echo ''#'\n" + path + "\n'"),
	}
	for _, yaml := range reject {
		if ciRunsExactPath([]byte(yaml), path) {
			t.Errorf("must reject non-invocation (comment-swallowed-quote bypass): %q", yaml)
		}
	}
	// A '#' immediately after a real word boundary must still start a
	// comment — the fix must not disable comment detection altogether.
	accept := []string{
		wrapStepBlock("echo x #" + path + "\n" + path),
	}
	for _, yaml := range accept {
		if !ciRunsExactPath([]byte(yaml), path) {
			t.Errorf("must accept real invocation after a genuine comment line: %q", yaml)
		}
	}
}
