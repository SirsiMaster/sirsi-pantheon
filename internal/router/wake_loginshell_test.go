package router

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// The whole defect: launchd execs the consumer directly, so no shell startup
// file runs and the credentials those files export never reach the child. This
// gives the test its own HOME with a .zshenv exporting a marker and asserts the
// marker arrives — a signal that CANNOT exist unless the login shell ran.
// Reverting the dispatch site to exec.Command(rc.Argv[0], ...) reddens this.
func TestDispatchConsumerRunsThroughLoginShellSoStartupFilesApply(t *testing.T) {
	if _, err := os.Stat("/bin/zsh"); err != nil {
		t.Skip("no /bin/zsh on this host")
	}
	home := t.TempDir()
	const marker = "SIRSI_LOGIN_SHELL_MARKER_OK"
	if err := os.WriteFile(filepath.Join(home, ".zshenv"),
		[]byte("export SIRSI_LOGIN_SHELL_PROBE="+marker+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SHELL", "/bin/zsh")

	run, err := dispatchConsumer(&ResolvedConsumer{
		// Backticks and $(...) in the argv assert the second half at the same
		// time: they must arrive LITERALLY, never executed by the wrapper.
		Argv: []string{"/bin/sh", "-c", `printf '%s|%s' "$SIRSI_LOGIN_SHELL_PROBE" 'literal ` + "`id`" + ` $(whoami)'`},
		Env:  []string{"HOME=" + home, "PATH=/usr/bin:/bin"},
	})
	if err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	select {
	case <-run.done:
	case <-time.After(30 * time.Second):
		t.Fatal("consumer did not complete")
	}
	got := run.tail.String()
	if !strings.Contains(got, marker) {
		t.Fatalf("startup file did not run — consumer was exec'd directly.\n want %q in: %q", marker, got)
	}
	if !strings.Contains(got, "literal `id` $(whoami)") {
		t.Fatalf("argv was interpolated into the script instead of passed positionally.\n got: %q", got)
	}
}

// Without the wrapper the consumer never sees the shell startup files that
// export its credentials. Assert the wrapper is actually applied.
func TestLoginShellArgvWrapsThroughShell(t *testing.T) {
	t.Setenv("SHELL", "/bin/zsh")
	got := loginShellArgv([]string{"claude", "--print", "hi"})
	want := []string{"/bin/zsh", "-lc", `exec "$@"`, "/bin/zsh", "claude", "--print", "hi"}
	if strings.Join(got, "\x00") != strings.Join(want, "\x00") {
		t.Fatalf("wrapper shape wrong.\n want %q\n got  %q", want, got)
	}
}

// Fail OPEN: an unset or unusable SHELL must dispatch exactly as before rather
// than refusing to spawn — same stance as an unreadable load average.
func TestLoginShellArgvFailsOpenOnUnusableShell(t *testing.T) {
	argv := []string{"claude", "--print", "hi"}
	for name, shell := range map[string]string{
		"unset":     "",
		"missing":   "/nonexistent/shell",
		"directory": "/tmp",
	} {
		t.Run(name, func(t *testing.T) {
			t.Setenv("SHELL", shell)
			if got := loginShellArgv(argv); strings.Join(got, "\x00") != strings.Join(argv, "\x00") {
				t.Fatalf("expected unchanged argv, got %q", got)
			}
		})
	}
	t.Run("empty argv", func(t *testing.T) {
		t.Setenv("SHELL", "/bin/zsh")
		if got := loginShellArgv(nil); len(got) != 0 {
			t.Fatalf("expected empty, got %q", got)
		}
	})
}
