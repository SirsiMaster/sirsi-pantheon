//go:build unix

package selfupdate

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestProbeBinaryContextAcceptsOneCompleteVersionIdentity(t *testing.T) {
	path := filepath.Join(t.TempDir(), "version")
	if err := os.WriteFile(path, []byte("#!/bin/sh\nprintf '{\"binary\":\"sirsi\",\"version\":\"1.2.3\"}\\n'\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	info, err := probeBinaryContext(context.Background(), path)
	if err != nil || info.Binary != "sirsi" || info.Version != "1.2.3" {
		t.Fatalf("complete version identity = %+v, %v", info, err)
	}
}

func TestProbeBinaryContextRejectsIncompleteOrTrailingJSON(t *testing.T) {
	for name, payload := range map[string]string{
		"incomplete": "{}",
		"trailing":   "{\"binary\":\"sirsi\",\"version\":\"1.2.3\"} {}",
	} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "version")
			body := "#!/bin/sh\nprintf " + strconv.Quote(payload) + "\n"
			if err := os.WriteFile(path, []byte(body), 0o700); err != nil {
				t.Fatal(err)
			}
			if _, err := probeBinaryContext(context.Background(), path); err == nil {
				t.Fatalf("probe accepted %s version response", name)
			}
		})
	}
}

func TestProbeBinaryContextRejectsOversizedVersionEnvelope(t *testing.T) {
	path := filepath.Join(t.TempDir(), "oversized")
	// printf is a shell builtin, so the 64 KiB + 1-byte fixture does not
	// spend the production 200 ms probe budget starting an external producer.
	if err := os.WriteFile(path, []byte("#!/bin/sh\nprintf '%65537s' ''\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := probeBinaryContext(context.Background(), path); err == nil || !strings.Contains(err.Error(), "exceeds its bounded JSON envelope") {
		t.Fatalf("oversized version probe error = %v", err)
	}
}

func TestProbeBinaryContextBoundsChildRetainingStdout(t *testing.T) {
	root := t.TempDir()
	pidFile := filepath.Join(root, "child.pid")
	path := filepath.Join(root, "retaining-child")
	body := "#!/bin/sh\n/bin/sleep 0.5 &\necho $! > " + strconv.Quote(pidFile) + "\nprintf '{\"binary\":\"sirsi\",\"version\":\"1.2.3\"}'\nexit 0\n"
	if err := os.WriteFile(path, []byte(body), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { waitForProbeFixtureChildExit(t, pidFile) })
	started := time.Now()
	if _, err := probeBinaryContext(context.Background(), path); err == nil {
		t.Fatal("probe accepted output whose inherited pipe did not reach EOF")
	}
	if elapsed := time.Since(started); elapsed > 2*time.Second {
		t.Fatalf("inherited stdout delayed probe for %s", elapsed)
	}
}

func TestProbeBinaryUsesLongerBudgetOnlyForMenubar(t *testing.T) {
	t.Run("menubar survives cold-start delay beyond CLI budget", func(t *testing.T) {
		path := delayedVersionFixture(t, "sirsi-menubar")
		started := time.Now()
		info, err := probeBinary(path)
		if err != nil || info.Binary != "sirsi-menubar" || info.Version != "1.2.3" {
			t.Fatalf("delayed native version identity = %+v, %v", info, err)
		}
		if elapsed := time.Since(started); elapsed <= probeTimeout {
			t.Fatalf("native fixture did not exercise the longer budget: %s", elapsed)
		}
	})

	for _, binary := range []string{"sirsi", "custom-sirsi-menubar"} {
		t.Run(binary+" keeps the shorter budget", func(t *testing.T) {
			path := delayedVersionFixture(t, binary)
			started := time.Now()
			if _, err := probeBinary(path); err == nil {
				t.Fatalf("%s probe accepted a response delayed beyond its 200 ms budget", binary)
			}
			if elapsed := time.Since(started); elapsed >= menubarProbeTimeout {
				t.Fatalf("%s probe exceeded the native-only budget: %s", binary, elapsed)
			}
		})
	}
}

func TestProbeBinaryContextParentDeadlineStillBoundsMenubar(t *testing.T) {
	path := delayedVersionFixture(t, "sirsi-menubar")
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Millisecond)
	defer cancel()
	started := time.Now()
	if _, err := probeBinaryContext(ctx, path); err == nil || ctx.Err() == nil {
		t.Fatalf("short parent deadline did not refuse delayed native probe: err=%v context=%v", err, ctx.Err())
	}
	if elapsed := time.Since(started); elapsed >= menubarProbeTimeout {
		t.Fatalf("native probe outlived its parent deadline and bounded pipe cleanup: %s", elapsed)
	}
}

func delayedVersionFixture(t *testing.T, binary string) string {
	t.Helper()
	root := t.TempDir()
	path := filepath.Join(root, binary)
	pidFile := filepath.Join(root, "sleep.pid")
	body := "#!/bin/sh\n/bin/sleep 0.35 &\nchild=$!\nprintf '%s\\n' \"$child\" > " + strconv.Quote(pidFile) +
		"\nwait \"$child\"\nprintf '{\"binary\":\"" + binary + "\",\"version\":\"1.2.3\"}\\n'\n"
	if err := os.WriteFile(path, []byte(body), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { waitForProbeFixtureChildExit(t, pidFile) })
	return path
}

func waitForProbeFixtureChildExit(t *testing.T, pidFile string) {
	t.Helper()
	data, err := os.ReadFile(pidFile)
	if err != nil {
		t.Errorf("read probe fixture child PID for natural-exit check: %v", err)
		return
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil || pid <= 1 {
		t.Errorf("invalid probe fixture child PID %q", strings.TrimSpace(string(data)))
		return
	}
	deadline := time.Now().Add(2 * time.Second)
	for {
		err := syscall.Kill(pid, 0)
		if err == syscall.ESRCH {
			return
		}
		if err != nil {
			t.Errorf("check natural exit of probe fixture child %d: %v", pid, err)
			return
		}
		if !time.Now().Before(deadline) {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Errorf("probe fixture child %d did not exit naturally within two seconds", pid)
}

func TestProbeBinaryContextHonorsCancellation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "stalled")
	if err := os.WriteFile(path, []byte("#!/bin/sh\nexec /bin/sleep 10\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	if _, err := probeBinaryContext(ctx, path); err == nil || ctx.Err() == nil {
		t.Fatalf("canceled probe = %v, context=%v", err, ctx.Err())
	}
}
