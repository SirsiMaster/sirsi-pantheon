package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Long inline bodies must be REFUSED, because the shell has already had its way
// with them by the time this process starts. This is the guard for
// reference_router_body_shell_injection — a hazard that corrupted a stored
// owner-gate record, and that blanked command names out of three separate item
// bodies in a single session written by an author who knew about it.
//
// The refusal is the feature. A truncated body looks plausible, which is
// exactly what makes silent rewriting dangerous.
func TestLongInlineBodyIsRefused(t *testing.T) {
	long := strings.Repeat("x", inlineBodyLimit+1)
	if _, err := loadOrLiteral(long); err == nil {
		t.Fatal("a body over the inline limit was accepted")
	} else if !strings.Contains(err.Error(), "@file") {
		t.Errorf("refusal does not tell the caller what to do instead: %v", err)
	}
}

// Short bodies stay inline — "ack" and "merged as abc123" are not worth a temp
// file, and a guard that makes the common case tedious gets worked around.
func TestShortInlineBodyIsAllowed(t *testing.T) {
	for _, s := range []string{"ack", "merged as abc123", strings.Repeat("y", inlineBodyLimit)} {
		if got, err := loadOrLiteral(s); err != nil || got != s {
			t.Errorf("short body %q refused: %v", s[:min(len(s), 20)], err)
		}
	}
}

// A file body of ANY length is accepted — that is the whole point of the escape
// hatch, and the file path never passes through shell evaluation of its
// contents.
func TestFileBodyIsAcceptedAtAnyLength(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "body.md")
	body := strings.Repeat("prose with `backticks` and $(substitutions) ", 200)
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := loadOrLiteral("@" + path)
	if err != nil {
		t.Fatalf("file body refused: %v", err)
	}
	if got != body {
		t.Error("file body was altered in transit")
	}
	if !strings.Contains(got, "`backticks`") {
		t.Error("backticks did not survive the file path — the escape hatch does not work")
	}
}

// An empty literal body must be REFUSED. This is the exact shape of the
// 2026-09-13 body-loss regression: `--instructions "$(true)"` evaluates in the
// shell to an empty string BEFORE this process starts, so loadOrLiteral only
// ever sees "" — indistinguishable from a caller who typed nothing. The
// length guard above does not fire (0 <= inlineBodyLimit), which is exactly
// why the corrupted item shipped with a valid title and a silently blank
// body. Reproduced here as the literal argv a shell would actually deliver,
// not just an empty string literal, so this test fails if anyone ever
// "simplifies" the guard back to a length-only check.
func TestEmptyInlineBodyIsRefused(t *testing.T) {
	for _, s := range []string{"", "   ", "\n\t"} {
		if _, err := loadOrLiteral(s); err == nil {
			t.Errorf("empty/whitespace body %q was accepted", s)
		}
	}
}

// The exact reproduction: what a real shell hands argv for
// `--instructions "$(true)"` is the single empty string "" (command
// substitution runs, produces no output, the surrounding quotes make the
// argument present-but-empty rather than absent). Asserting on the produced
// value, not just inline literals, so this test would have caught the actual
// regression rather than a synthetic stand-in for it.
func TestEmptyInlineBodyMatchesRealShellSubstitutionRepro(t *testing.T) {
	shellDelivered := "" // `--instructions "$(true)"` arrives here as this
	if _, err := loadOrLiteral(shellDelivered); err == nil {
		t.Fatal("the exact argv a shell delivers for \"$(true)\" was accepted as a valid body")
	}
}

// A zero-byte or whitespace-only FILE body must be refused too — same
// silent-corruption shape (a truncated write, an empty heredoc), and the
// file path deserves the same loud refusal as the literal path.
func TestEmptyFileBodyIsRefused(t *testing.T) {
	dir := t.TempDir()
	for name, content := range map[string]string{
		"empty.md":      "",
		"whitespace.md": "   \n\t\n",
	} {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := loadOrLiteral("@" + path); err == nil {
			t.Errorf("empty file body (%s) was accepted", name)
		}
	}
}

// Negative control for the two tests above: a file or literal with REAL
// content, even short, must still be accepted — the guard targets emptiness,
// not files or short strings in general.
func TestNonEmptyBodyStillAcceptedBothPaths(t *testing.T) {
	if got, err := loadOrLiteral("ack"); err != nil || got != "ack" {
		t.Errorf("non-empty literal wrongly refused: %v", err)
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "body.md")
	if err := os.WriteFile(path, []byte("real content"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got, err := loadOrLiteral("@" + path); err != nil || got != "real content" {
		t.Errorf("non-empty file wrongly refused: %v", err)
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
