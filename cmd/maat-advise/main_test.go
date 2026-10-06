package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func advise(t *testing.T, input string, args ...string) (string, int, string) {
	t.Helper()
	summary := filepath.Join(t.TempDir(), "summary.md")
	var out bytes.Buffer
	rc := run(args, strings.NewReader(input), &out, summary)
	b, _ := os.ReadFile(summary)
	return out.String(), rc, string(b)
}

// A known failure prints its cause and fix at once, and never changes the verdict (rc 0).
func TestKnownFailureIsAnsweredWithCauseAndFix(t *testing.T) {
	out, rc, summary := advise(t, "ERROR codex: writable root /Users/x/.sirsi/relay contains symlink component /Users/x/.sirsi", "--step", "tests")
	if rc != 0 {
		t.Fatalf("advising must not change the verdict, rc=%d", rc)
	}
	for _, want := range []string{"KNOWN failure in tests", "codex-symlinked-writable-root", "cause:", "fix:", "fixed in"} {
		if !strings.Contains(out, want) {
			t.Errorf("output lacks %q:\n%s", want, out)
		}
	}
	if !strings.Contains(summary, "codex-symlinked-writable-root") {
		t.Errorf("step summary lacks the entry: %q", summary)
	}
}

// An unknown failure says how to record it; it does not invent an answer.
func TestUnknownFailureSaysHowToRecordIt(t *testing.T) {
	out, rc, _ := advise(t, "some brand new compile error: undefined: Frobnicate", "--step", "build")
	if rc != 0 || !strings.Contains(out, "no known failure matches") || !strings.Contains(out, "known-failures register") {
		t.Fatalf("unknown failure output wrong (rc=%d):\n%s", rc, out)
	}
	if strings.Contains(out, "KNOWN failure") {
		t.Fatalf("invented a match for unknown text:\n%s", out)
	}
}

// GitHub mode emits annotations, and quotes catalog text only, never the (untrusted) log.
func TestGithubModeAnnotatesAndNeverEchoesLogContent(t *testing.T) {
	out, _, _ := advise(t, "@everyone <script>x</script> writable root /a contains symlink component /b", "--github")
	if !strings.Contains(out, "::error title=Ma'at known failure: codex-symlinked-writable-root::") {
		t.Fatalf("no error annotation:\n%s", out)
	}
	if strings.Contains(out, "@everyone") || strings.Contains(out, "<script>") {
		t.Fatalf("log content leaked into the advice:\n%s", out)
	}
}

func TestBadUsageIsExit2(t *testing.T) {
	if _, rc, _ := advise(t, "x", "--nope"); rc != 2 {
		t.Fatalf("unknown flag rc=%d, want 2", rc)
	}
}
