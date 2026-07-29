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

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
