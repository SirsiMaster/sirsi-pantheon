package knownfail

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// Register adds an OPEN problem to the catalog file at path (the repo's
// catalog.json): a signature, the cause, and nothing claimed as fixed. The change
// goes through a PR like any other, so a registered problem is on origin.
func Register(path string, e Entry) error {
	e.Status = "open"
	e.Fix, e.Guard = Fix{}, Guard{}
	return mutate(path, func(c *Catalog) error {
		for _, x := range c.Entries {
			if x.ID == e.ID {
				return fmt.Errorf("known failure %q is already registered", e.ID)
			}
		}
		c.Entries = append(c.Entries, e)
		return nil
	})
}

// Resolve records the fix for a registered problem. It refuses unless the named
// regression guard is a test that really exists in repoDir, which is what keeps a
// "fixed" claim from being a note: the fix is on record with the version it shipped
// in and a test that fails if it regresses.
func Resolve(path, repoDir, id string, fix Fix, guardTest string) error {
	if strings.TrimSpace(guardTest) == "" || strings.TrimSpace(fix.FixedIn) == "" || strings.TrimSpace(fix.Text) == "" {
		return fmt.Errorf("resolving %q needs --fixed-in, --text and --guard (a regression test name)", id)
	}
	kind := GuardKindFor(guardTest)
	if err := guardExists(repoDir, kind, guardTest); err != nil {
		return fmt.Errorf("guard %q: %w: write the regression guard first", guardTest, err)
	}
	return mutate(path, func(c *Catalog) error {
		for i := range c.Entries {
			if c.Entries[i].ID == id {
				c.Entries[i].Status = "resolved"
				if fix.Kind == "" {
					fix.Kind = "guide"
				}
				c.Entries[i].Fix = fix
				c.Entries[i].Guard = Guard{Kind: kind, Ref: guardTest}
				return nil
			}
		}
		return fmt.Errorf("known failure %q is not registered", id)
	})
}

var testNameRe = regexp.MustCompile(`^Test[A-Za-z0-9_]+$`)

// crossRepoGuardRef matches a guard that names a commit in another Sirsi repo:
// "owner/repo@commit:path". The commit (not a branch) makes the guard an
// immutable fact instead of a moving target (Ra decision 20261007-110326,
// answering mercury's known-failures-resolve question 20261007-042548). Owner
// and repo exclude "." so neither component can ever be ".." — a directory
// traversal segment — eliminating that escape by construction rather than by
// a path-containment check after the fact.
var crossRepoGuardRef = regexp.MustCompile(`^([A-Za-z0-9_-]+)/([A-Za-z0-9_-]+)@([0-9a-fA-F]{7,40}):(.+)$`)

// githubRemoteRe pulls owner/repo out of a GitHub remote URL, either form
// ("git@github.com:Owner/Repo.git" or "https://github.com/Owner/Repo").
var githubRemoteRe = regexp.MustCompile(`(?i)github\.com[:/]([^/]+)/([^/]+?)(\.git)?/?$`)

// GuardKindFor says which kind of guard a reference names: "owner/repo@commit:path"
// lives in another Sirsi repo's local checkout; any other path is a script this
// repo's CI runs; anything else is a Go test name.
func GuardKindFor(ref string) string {
	if crossRepoGuardRef.MatchString(ref) {
		return "cross-repo-script"
	}
	if strings.Contains(ref, "/") {
		return "script"
	}
	return "test"
}

// guardExists proves a guard is real. A test must be a Go test function in the repo. A
// script must be an executable file in the repo that the CI workflow actually runs, so a
// "fixed" claim cannot rest on a script nobody executes. A cross-repo-script is the same
// proof read from another Sirsi repo's own local checkout at a pinned commit.
func guardExists(repoDir, kind, ref string) error {
	if kind == "cross-repo-script" {
		return crossRepoGuardExists(ref)
	}
	if kind == "script" {
		clean := filepath.Clean(ref)
		if filepath.IsAbs(clean) || strings.HasPrefix(clean, "..") {
			return fmt.Errorf("a script guard must be a path inside the repo")
		}
		info, err := os.Stat(filepath.Join(repoDir, clean))
		if err != nil || info.IsDir() || info.Mode()&0o111 == 0 {
			return fmt.Errorf("%s is not an executable file in the repo", clean)
		}
		ci, err := os.ReadFile(filepath.Join(repoDir, ".github", "workflows", "ci.yml"))
		if err != nil || !strings.Contains(string(ci), clean) {
			return fmt.Errorf("%s is not run by .github/workflows/ci.yml", clean)
		}
		return nil
	}
	if !testExists(repoDir, ref) {
		return fmt.Errorf("not a test in the repo")
	}
	return nil
}

// crossRepoGuardExists proves an "owner/repo@commit:path" guard the same way
// guardExists proves a local one, but reads the OTHER repo's own local checkout
// (convention: ~/Development/<repo>) at the pinned commit: the commit exists
// there, the blob at that commit is executable, and that repo's CI workflow at
// that commit runs it. It never trusts the current working tree of that repo,
// so a later uncommitted edit there can't forge the guard retroactively.
func crossRepoGuardExists(ref string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	return crossRepoGuardExistsIn(filepath.Join(home, "Development"), ref)
}

func crossRepoGuardExistsIn(devRoot, ref string) error {
	m := crossRepoGuardRef.FindStringSubmatch(ref)
	if m == nil {
		return fmt.Errorf("must be owner/repo@commit:path")
	}
	owner, repo, commit, path := m[1], m[2], m[3], m[4]
	dir := filepath.Join(devRoot, repo)
	// Belt-and-suspenders: crossRepoGuardRef already forbids "." in repo, so
	// this can't actually escape devRoot, but a future loosened regex must not
	// silently regain a traversal — fail the containment check explicitly too.
	absDevRoot, err := filepath.Abs(devRoot)
	if err != nil {
		return err
	}
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return err
	}
	if absDir != filepath.Join(absDevRoot, repo) || !strings.HasPrefix(absDir+string(filepath.Separator), absDevRoot+string(filepath.Separator)) {
		return fmt.Errorf("repo %q escapes the local checkout root", repo)
	}
	if _, err := os.Stat(dir); err != nil {
		return fmt.Errorf("no local checkout of %s/%s at %s", owner, repo, dir)
	}
	// The directory name alone is not proof of identity: verify the checkout's
	// own origin remote actually points at the declared owner/repo, so a
	// same-named checkout of a DIFFERENT repo (or a fork) can't be substituted.
	remote, err := gitCmd(dir, "remote", "get-url", "origin").Output()
	if err != nil {
		return fmt.Errorf("could not read origin remote of %s: %w", dir, err)
	}
	rm := githubRemoteRe.FindStringSubmatch(strings.TrimSpace(string(remote)))
	if rm == nil {
		return fmt.Errorf("origin remote %q of %s is not a recognizable GitHub URL", strings.TrimSpace(string(remote)), dir)
	}
	if !strings.EqualFold(rm[1], owner) || !strings.EqualFold(rm[2], repo) {
		return fmt.Errorf("checkout at %s is %s/%s, not %s/%s", dir, rm[1], rm[2], owner, repo)
	}
	if err := gitCmd(dir, "cat-file", "-e", commit+"^{commit}").Run(); err != nil {
		return fmt.Errorf("commit %s not found in local checkout %s", commit, dir)
	}
	out, err := gitCmd(dir, "ls-tree", commit, "--", path).Output()
	if err != nil || strings.TrimSpace(string(out)) == "" {
		return fmt.Errorf("%s not found at commit %s in %s", path, commit, dir)
	}
	if fields := strings.Fields(string(out)); len(fields) == 0 || fields[0] != "100755" {
		return fmt.Errorf("%s at commit %s is not mode 100755", path, commit)
	}
	ci, err := gitCmd(dir, "show", commit+":.github/workflows/ci.yml").Output()
	if err != nil || !ciRunsExactPath(ci, path) {
		return fmt.Errorf("%s is not run by %s/%s's .github/workflows/ci.yml at commit %s", path, owner, repo, commit)
	}
	return nil
}

// ciWorkflow is the slice of GitHub Actions workflow shape ciRunsExactPath
// needs: every job's steps, each step's run script. Any other field (name,
// uses, env, with, comments) is not a shell command and is never consulted —
// a path merely named in a step's "name:" or passed as an argument to echo/cat
// is not an invocation.
type ciWorkflow struct {
	Jobs map[string]struct {
		Steps []struct {
			Run string `yaml:"run"`
		} `yaml:"steps"`
	} `yaml:"jobs"`
}

// shellWrappers are the only supported command-grammar prefixes that make the
// FOLLOWING token the thing actually executed (`bash scripts/guard.sh`,
// `sh scripts/guard.sh`, `source scripts/guard.sh`, `. scripts/guard.sh`).
// Anything else in command position — echo, cat, printf, or the path itself —
// is read literally: the path must BE the command, not its argument.
var shellWrappers = map[string]bool{"bash": true, "sh": true, "source": true, ".": true}

// ciRunsExactPath reports whether a CI workflow actually EXECUTES path as a
// shell command, fail-closed over a small supported grammar: the first WORD
// (quote- and escape-resolved, so a literal space inside quotes or after a
// backslash can never masquerade as a word boundary) of a statement (split
// on &&, ||, ;, | — never inside quotes, a comment, or a heredoc body) in a
// step's run: script, or the second word when the first is a known
// interpreter wrapper, matched by EXACT repo-relative identity (never a path
// suffix, so a guard script at a different path can't be substituted). A
// path that is merely mentioned — as a step's name, a comment, a quoted or
// escaped argument, or heredoc data — is inert and does not count; only real
// YAML parsing (not line/token scanning) can tell a run: step from a name:
// field in the first place.
func ciRunsExactPath(ciYAML []byte, path string) bool {
	var wf ciWorkflow
	if err := yaml.Unmarshal(ciYAML, &wf); err != nil {
		return false
	}
	for _, job := range wf.Jobs {
		for _, step := range job.Steps {
			for _, words := range splitShellStatements(step.Run) {
				if len(words) == 0 {
					continue
				}
				cmd := strings.TrimPrefix(words[0], "./")
				if cmd == path {
					return true
				}
				if shellWrappers[cmd] && len(words) > 1 {
					arg := strings.TrimPrefix(words[1], "./")
					if arg == path {
						return true
					}
				}
			}
		}
	}
	return false
}

// blockOpenWords are the shell keywords that start a compound construct
// (if/while/until/for/case/select/function) whose body is NOT command
// position at the top level this grammar reasons about — the body only runs
// conditionally, repeatedly, or as a function call elsewhere, never simply
// because it is text in the script. Appearing as a statement's first word,
// each opens one level of blockDepth.
var blockOpenWords = map[string]bool{
	"if": true, "while": true, "until": true, "for": true,
	"case": true, "select": true, "function": true,
}

// blockCloseWords are the matching closers (fi/done/esac) for blockOpenWords.
// A bare "}" closes a brace group or `name() { ... }` function body instead
// (checked separately, since "}" is a word by itself rather than a keyword).
var blockCloseWords = map[string]bool{"fi": true, "done": true, "esac": true}

// splitShellStatements breaks a shell script into top-level statements, each
// already split into fully quote- and escape-RESOLVED words (no leftover
// quote characters, no literal-whitespace-inside-a-word masquerading as a
// word boundary) — so a caller never re-splits on whitespace itself and
// cannot repeat the quoting mistake this function exists to avoid.
//
// It is a single pass over the WHOLE script, not a per-line scan, because a
// single or double quote legitimately spans a newline (the embedded newline
// is part of the string, not a statement separator) — tracking quote state
// per line would let such a line's second half fall out of the quote and be
// read as a fresh, unquoted statement. Heredoc bodies are skipped entirely
// (they are data, never commands), with the real shell rule for where a
// heredoc body ends: a plain `<<DELIM` requires the body line to equal DELIM
// byte-for-byte (no stripping), while `<<-DELIM` strips only leading TAB
// characters (never spaces) from each body line before comparing. A trailing
// backslash escapes the following character, including a line continuation,
// so neither is ever treated as quoting, a comment, or a separator; `&&`,
// `||`, `;`, `|`, and an unquoted newline split statements; `#` starts a
// comment only at a word boundary (start of script, after whitespace, or
// right after a split).
//
// Compound constructs (if/while/until/for/case/select, a `name() { ... }`
// function, or a bare `{ ... }` brace group) are grammar this scanner does
// not model any deeper than recognizing their open/close keywords — rather
// than try to decide whether an `if false` branch or a never-called function
// body would really run, every statement while blockDepth > 0 is poisoned
// (fail closed, same as an unmodeled redirect or subshell): a statement
// merely sitting inside a conditional, loop, or function body is never
// command position for this guard's purposes, even when the enclosing
// construct would in fact execute it.
//
// `&&` and `||` are conditional-list operators, not plain separators: the
// right-hand statement only runs if the left-hand one's exit status matches
// (succeeded for &&, failed for ||). This scanner cannot evaluate an exit
// status, so every statement to the right of a `&&`/`||` — and transitively
// every statement after THAT in the same chain, since each one's execution
// still depends on the unevaluated statement before it — is poisoned the
// same way a compound-construct body is. `;`, a bare newline, `&`
// (background), and `|` (pipe) are true separators: each side still runs
// unconditionally, so they never poison. A parenthesized subshell is poison
// for the same depth-tracked reason as a block: parenDepth counts unclosed
// `(` the way blockDepth counts unclosed if/while/etc, so a statement two
// lines inside an open paren is still poisoned even though it sits between
// two ordinary newlines.
//
// The &&/|| poison is held in condPending rather than applied to poisoned
// directly, because real shell grammar lets a newline or a comment-only line
// sit between the operator and the statement it actually governs (`false &&
// \n` then `echo c` on the next line is still conditional on `false`,
// exactly like `false && echo c` on one line). An empty statement — no words
// at all, produced by a bare separator run or a comment-only line — must
// never consume or clear that pending poison; only a statement that actually
// collected words can, whether it ends up poisoned or not.
func splitShellStatements(script string) [][]string {
	var stmts [][]string
	var words []string
	var quotedWord []bool // parallel to words: true if quoting/escaping touched this word
	var word strings.Builder
	var wordStarted bool   // a word exists even if empty, e.g. a bare "" argument
	var curWordQuoted bool // true once the word under construction has seen a quote or escape
	var inSingle, inDouble bool
	var pending []heredocSpec // heredocs opened on the current line, in order
	var poisoned bool         // this statement contains ungrammared text; never let it match
	var blockDepth int        // >0 means every statement is inside an unmodeled compound construct
	var parenDepth int        // >0 means every statement is inside an unclosed subshell
	var condPending bool      // a preceding &&/|| still needs a real (non-empty) statement to poison
	atBoundary := true
	runes := []rune(script)
	n := len(runes)
	addRune := func(r rune) {
		word.WriteRune(r)
		wordStarted = true
		atBoundary = false
	}
	markQuoted := func() { curWordQuoted = true }
	endWord := func() {
		if wordStarted {
			words = append(words, word.String())
			quotedWord = append(quotedWord, curWordQuoted)
			word.Reset()
			wordStarted = false
			curWordQuoted = false
		}
	}
	// isBareReservedWord reports whether words[i] can be trusted as a real
	// shell reserved word or brace rather than a quoted/escaped string that
	// merely contains the same text (`echo "{"`, `echo done`-as-a-literal-
	// looking token still a plain argument, not this check's concern since
	// position gates that separately) — quoting is the one thing that can
	// make text LOOK like a keyword without the shell ever treating it as
	// one, so an escaped or quoted occurrence must never move blockDepth.
	isBareReservedWord := func(i int) bool {
		return i >= 0 && i < len(quotedWord) && !quotedWord[i]
	}
	endStmt := func() {
		endWord()
		if blockDepth > 0 || parenDepth > 0 || condPending {
			poisoned = true
		}
		if len(words) > 0 && !poisoned {
			stmts = append(stmts, words)
		}
		if len(words) > 0 {
			// Only a statement that actually collected words resolves a
			// pending &&/|| poison — an empty statement (consecutive
			// separators, or a comment-only line) must leave it pending
			// for whichever real statement comes next.
			condPending = false
			// A closer is only trusted in command position (the statement's
			// first word) — a real `fi`/`done`/`esac`/`}` never appears
			// anywhere else, so this can't be fooled by one showing up as a
			// later argument.
			first := words[0]
			if isBareReservedWord(0) && (blockCloseWords[first] || first == "}") {
				if blockDepth > 0 {
					blockDepth--
				}
			}
			// An opener, unlike a closer, is NOT confined to words[0]: `!`
			// (negation), `time`, or chaining a control keyword straight
			// after `then`/`else`/`elif`/`do` without a `;` (`then if`,
			// `! if`) all put a real nested opener at a later word position
			// in the SAME split statement. Scanning every word (instead of
			// only the first or the last) means a negated or chained opener
			// is never missed; the cost is poisoning an occasional statement
			// that merely contains a bare reserved word as a plain argument
			// (e.g. `echo if`) — safe, since over-poisoning can only reject
			// a real match, never accept a false one.
			for i, w := range words {
				if !isBareReservedWord(i) {
					continue
				}
				if blockOpenWords[w] || w == "{" {
					blockDepth++
				}
			}
		}
		words = nil
		quotedWord = nil
		poisoned = false
		atBoundary = true
	}
	// consumeHeredocBodies skips the body of every queued heredoc, in the
	// order their openers appeared, starting at line start index k.
	consumeHeredocBodies := func(k int) int {
		for _, h := range pending {
			for k < n {
				bodyEnd := k
				for bodyEnd < n && runes[bodyEnd] != '\n' {
					bodyEnd++
				}
				line := string(runes[k:bodyEnd])
				if h.dash {
					line = strings.TrimLeft(line, "\t")
				}
				if line == h.delim {
					k = bodyEnd
					break
				}
				if bodyEnd >= n {
					k = n
					break
				}
				k = bodyEnd + 1
			}
			if k < n {
				k++ // step past the delimiter line's newline for the next body
			}
		}
		pending = nil
		return k
	}
	for i := 0; i < n; i++ {
		c := runes[i]
		switch {
		case c == '\\' && !inSingle && (!inDouble || isDQEscapable(peek(runes, i+1))) && i+1 < n:
			i++
			if runes[i] != '\n' {
				addRune(runes[i])
				markQuoted()
			}
		case c == '\\' && inDouble:
			// Inside double quotes a backslash keeps its special meaning
			// only before $, `, ", \, or newline (handled above); before
			// any other character it is LITERAL and both runes survive.
			addRune(c)
			markQuoted()
		case c == '\'' && !inDouble:
			inSingle = !inSingle
			wordStarted = true
			atBoundary = false
			markQuoted()
		case c == '"' && !inSingle:
			inDouble = !inDouble
			wordStarted = true
			atBoundary = false
			markQuoted()
		case inSingle || inDouble:
			addRune(c)
			markQuoted()
		case c == '<' && i+1 < n && runes[i+1] == '<':
			dashForm := i+2 < n && runes[i+2] == '-'
			rest := i + 2
			if dashForm {
				rest++
			}
			for rest < n && (runes[rest] == ' ' || runes[rest] == '\t') {
				rest++
			}
			delim, consumed, ok := parseHeredocDelim(runes[rest:])
			if !ok {
				// Not a recognizable heredoc opener (e.g. a bare `<<` with
				// no delimiter word) — read it as a literal character
				// rather than risk silently skipping real statements.
				addRune(c)
				continue
			}
			pending = append(pending, heredocSpec{delim: delim, dash: dashForm})
			i = rest + consumed - 1
			atBoundary = false
		case c == '#' && atBoundary:
			for i < n && runes[i] != '\n' {
				i++
			}
			i--
		case c == ' ' || c == '\t':
			endWord()
			atBoundary = true
		case c == ';':
			endStmt()
		case c == '\n':
			endStmt()
			if len(pending) > 0 {
				i = consumeHeredocBodies(i+1) - 1
			}
		case c == '|':
			isOr := i+1 < n && runes[i+1] == '|'
			if isOr {
				i++
			}
			endStmt()
			if isOr {
				// `||` only runs the right-hand statement if the left one
				// failed — an exit status this scanner cannot evaluate —
				// so poison it (and transitively the rest of the chain)
				// the same way an unmodeled block body is poisoned. A bare
				// `|` (pipe) runs both sides unconditionally and must not
				// set this. condPending (not poisoned directly) survives
				// any blank/comment-only statement before the real one.
				condPending = true
			}
		case c == '&' && i+1 < n && runes[i+1] == '&':
			i++
			endStmt()
			// `&&` only runs the right-hand statement if the left one
			// succeeded — poison it for the same reason as `||` above.
			condPending = true
		case c == '&' && i > 0 && (runes[i-1] == '<' || runes[i-1] == '>'):
			// Part of a compound redirect operator (>&, <&), not the
			// background operator — the metachar case below already
			// poisoned this statement when it saw the '<'/'>'; this is
			// still the SAME statement (the redirect target follows), so
			// it must not end it the way a true lone '&' does.
			atBoundary = true
		case c == '&':
			// A lone '&' is the background operator: a statement
			// terminator exactly like ';', not a literal word character.
			endStmt()
		case c == '(':
			// Unquoted `(` opens a subshell this narrow grammar does not
			// model. Depth-track it like blockDepth so every statement
			// until the matching `)` is poisoned — not just the statement
			// that happens to contain the `(` itself, which would let a
			// later line inside the subshell slip through as if it were
			// plain top-level command position.
			endWord()
			poisoned = true
			atBoundary = true
			parenDepth++
		case c == ')':
			endWord()
			poisoned = true
			atBoundary = true
			if parenDepth > 0 {
				parenDepth--
			}
		case c == '<' || c == '>':
			// Unquoted <, > are redirect metacharacters this narrow
			// grammar does not model (a redirect target like
			// `>scripts/guard.sh` is never command position). Poison the
			// whole statement: it still scans cleanly (words end here,
			// same as any boundary) but can never produce a match when it
			// is finally flushed.
			endWord()
			poisoned = true
			atBoundary = true
		default:
			addRune(c)
		}
	}
	endStmt()
	return stmts
}

// heredocSpec is one `<<DELIM`/`<<-DELIM` opener queued on a line, to be
// consumed in the order it appeared — a line can open more than one heredoc
// (`cmd <<A <<B`), and each one's body must be skipped as data regardless of
// what text it contains.
type heredocSpec struct {
	delim string
	dash  bool
}

// peek returns runes[i] or 0 past the end, so a trailing backslash at
// end-of-script never panics the double-quote escapability check.
func peek(runes []rune, i int) rune {
	if i < len(runes) {
		return runes[i]
	}
	return 0
}

// isDQEscapable reports whether r is one of the handful of characters a
// backslash retains its special (escaping) meaning before INSIDE double
// quotes per POSIX shell quoting: $, `, ", \, or a line-continuation
// newline. Before any other character, a backslash inside double quotes is
// literal and both runes survive untouched.
func isDQEscapable(r rune) bool {
	return r == '$' || r == '`' || r == '"' || r == '\\' || r == '\n'
}

// parseHeredocDelim reads a heredoc delimiter word starting at r[0] (the
// first non-blank rune after `<<`/`<<-`), applying real shell quote removal
// — including the double-quote escape rule above — so `'END-TEXT'` yields
// the exact delimiter END-TEXT, not a regex-truncated prefix, and stops at
// the first unquoted whitespace or newline. Reports the number of runes
// consumed so the caller can resume scanning the rest of the opener line
// (which may queue further heredocs). Reports false for an empty or
// all-blank delimiter.
func parseHeredocDelim(r []rune) (delim string, consumed int, ok bool) {
	var b strings.Builder
	var inSingle, inDouble bool
	i := 0
loop:
	for i < len(r) {
		c := r[i]
		switch {
		case !inSingle && !inDouble && (c == ' ' || c == '\t' || c == '\n'):
			break loop
		case c == '\\' && !inSingle && (!inDouble || isDQEscapable(peek(r, i+1))) && i+1 < len(r):
			b.WriteRune(r[i+1])
			i += 2
		case c == '\\' && inDouble:
			b.WriteRune(c)
			i++
		case c == '\'' && !inDouble:
			inSingle = !inSingle
			i++
		case c == '"' && !inSingle:
			inDouble = !inDouble
			i++
		default:
			b.WriteRune(c)
			i++
		}
	}
	if b.Len() == 0 {
		return "", i, false
	}
	return b.String(), i, true
}

func testExists(repoDir, name string) bool {
	if !testNameRe.MatchString(name) {
		return false
	}
	out, err := gitCmd(repoDir, "grep", "--untracked", "-l", "func "+name+"(", "--", "*_test.go").Output()
	return err == nil && strings.TrimSpace(string(out)) != ""
}

// gitCmd runs git in dir with the GIT_* environment removed: inside a git hook
// GIT_DIR/GIT_INDEX_FILE point at the pushing repository and would silently redirect
// the query.
func gitCmd(dir string, args ...string) *exec.Cmd {
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	for _, kv := range os.Environ() {
		if !strings.HasPrefix(kv, "GIT_") {
			cmd.Env = append(cmd.Env, kv)
		}
	}
	return cmd
}

func mutate(path string, fn func(*Catalog) error) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var c Catalog
	if err = json.Unmarshal(b, &c); err != nil {
		return err
	}
	if err = fn(&c); err != nil {
		return err
	}
	out, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	if _, err = parse(out); err != nil { // never write a catalog the loader would reject
		return err
	}
	return os.WriteFile(path, append(out, '\n'), 0o644)
}
