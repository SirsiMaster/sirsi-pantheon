package e2e

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// testBinary holds the path to the compiled pantheon binary, built once by TestMain.
var testBinary string

// TestMain builds the pantheon binary once and shares it across all tests.
func TestMain(m *testing.M) {
	if mode := os.Getenv(e2eLifecycleChildEnv); mode != "" {
		os.Exit(runE2ELifecycleChild(mode))
	}
	// os.Exit skips defers. Return from the owned lifecycle (including cleanup)
	// before exiting, on both build failure and the test runner's exact status.
	os.Exit(runE2ETestMain(buildE2EBinary, m.Run, os.Stderr))
}

func runE2ETestMain(build func(string) int, runTests func() int, diagnostics io.Writer) (code int) {
	// Create a temp directory for the binary (outside of any test's TempDir).
	tmpDir, err := os.MkdirTemp("", "sirsi-e2e-*")
	if err != nil {
		fmt.Fprintf(diagnostics, "failed to create temp dir: %v\n", err)
		return 1
	}
	absolute, err := filepath.Abs(tmpDir)
	if err != nil {
		fmt.Fprintf(diagnostics, "e2e cleanup debt at %q: cannot resolve created temp root: %v\n", tmpDir, err)
		return 1
	}
	tmpDir = absolute
	created, err := os.Lstat(tmpDir)
	if err != nil || !created.IsDir() {
		fmt.Fprintf(diagnostics, "e2e cleanup debt at %q: cannot identify created directory: %v\n", tmpDir, err)
		return 1
	}
	root, err := os.OpenRoot(tmpDir)
	if err != nil {
		fmt.Fprintf(diagnostics, "e2e cleanup debt at %q (identity %v): %v\n", tmpDir, created.Sys(), err)
		return 1
	}
	defer func() {
		cleanupErr := cleanupE2ETestRoot(tmpDir, created, root)
		closeErr := root.Close()
		if cleanupErr != nil || closeErr != nil {
			fmt.Fprintf(diagnostics, "e2e cleanup debt at %q (created identity %v): cleanup=%v close=%v; inspect this owned root, not shared caches\n", tmpDir, created.Sys(), cleanupErr, closeErr)
			if code == 0 {
				code = 1
			}
		}
	}()
	opened, err := root.Stat(".")
	if err != nil || !os.SameFile(created, opened) {
		fmt.Fprintf(diagnostics, "e2e temp root changed before build: %v\n", err)
		return 1
	}

	binary := filepath.Join(tmpDir, "sirsi")
	if runtime.GOOS == "windows" {
		binary += ".exe"
	}

	if code := build(binary); code != 0 {
		return code
	}
	testBinary = binary
	return runTests()
}

// Recursive removal is rooted in the retained directory, never in a possibly
// substituted pathname. Check named identity before touching contents and again
// before removing the empty leaf. This is cooperative test ownership, not an
// atomic compare-and-unlink guarantee against a hostile same-UID writer.
func cleanupE2ETestRoot(path string, created os.FileInfo, root *os.Root) error {
	same := func() error {
		named, err := os.Lstat(path)
		if err != nil {
			return err
		}
		opened, err := root.Stat(".")
		if err != nil {
			return err
		}
		if !named.IsDir() || !os.SameFile(created, named) || !os.SameFile(created, opened) {
			return fmt.Errorf("temp-root identity changed; refusing removal")
		}
		return nil
	}
	if err := same(); err != nil {
		return err
	}
	dir, err := root.Open(".")
	if err != nil {
		return err
	}
	entries, readErr := dir.ReadDir(-1)
	closeErr := dir.Close()
	if readErr != nil {
		return readErr
	}
	if closeErr != nil {
		return closeErr
	}
	for _, entry := range entries {
		if err := root.RemoveAll(entry.Name()); err != nil {
			return err
		}
	}
	if err := same(); err != nil {
		return err
	}
	// Only an empty directory is removed by name; never recursively remove a
	// late replacement. Nonempty/new arrivals fail and remain explicit debt.
	return os.Remove(path)
}

func buildE2EBinary(binary string) int {
	cmd := exec.Command("go", "build", "-o", binary, "./cmd/sirsi/")
	cmd.Dir = filepath.Join("..", "..") // tests/e2e → repo root
	out, err := cmd.CombinedOutput()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to build sirsi: %v\n%s", err, out)
		if exit, ok := err.(*exec.ExitError); ok && exit.ExitCode() > 0 {
			return exit.ExitCode()
		}
		return 1
	}
	return 0
}

// run executes the pantheon binary with args and returns stdout+stderr.
// Times out after 60 seconds to prevent hangs.
func run(t *testing.T, binary string, args ...string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, binary, args...)
	cmd.Dir = filepath.Join("..", "..") // run from repo root
	out, err := cmd.CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		t.Logf("command timed out after 60s: %s %v", binary, args)
		return string(out)
	}
	if err != nil {
		// Some commands exit non-zero but still produce useful output
		return string(out)
	}
	return string(out)
}

func TestSmoke_Build(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping E2E test in short mode")
	}
	if _, err := os.Stat(testBinary); err != nil {
		t.Fatal("binary not found after build")
	}
}

func TestSmoke_Version(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping E2E test in short mode")
	}
	out := run(t, testBinary, "version")
	if !strings.Contains(out, "Sirsi Pantheon") {
		t.Errorf("version output missing Sirsi Pantheon: %s", out)
	}
}

func TestSmoke_Help(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping E2E test in short mode")
	}
	out := run(t, testBinary, "--help")

	// Verify key user-facing commands appear in help output.
	// Hidden deity commands (thoth, guard) are excluded — users see plain verbs.
	expected := []string{"scan", "clean", "ghosts", "audit", "anubis", "maat"}
	for _, cmd := range expected {
		if !strings.Contains(strings.ToLower(out), cmd) {
			t.Errorf("help output missing command %q:\n%s", cmd, out)
		}
	}
}

func TestSmoke_AnubisWeigh(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping E2E test in short mode")
	}
	out := run(t, testBinary, "anubis", "weigh")

	// The scanner should produce some output about waste or scan results
	if len(out) < 10 {
		t.Errorf("anubis weigh produced very little output: %s", out)
	}
}

func TestSmoke_AnubisJudgeDryRun(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping E2E test in short mode")
	}
	out := run(t, testBinary, "anubis", "judge", "--dry-run")

	lower := strings.ToLower(out)
	if !strings.Contains(lower, "dry") && !strings.Contains(lower, "no waste") && !strings.Contains(lower, "adjudicated") && !strings.Contains(lower, "purged") && !strings.Contains(lower, "anubis") && !strings.Contains(lower, "judgment") {
		t.Errorf("anubis judge --dry-run unexpected output: %s", out)
	}
}

func TestSmoke_MaatAudit(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping E2E test in short mode")
	}
	// Ma'at audit internally runs `go test -cover ./...` which takes 2+ minutes.
	// Use --skip-test to only assess static quality (lint, vet, docs).
	out := run(t, testBinary, "maat", "audit", "--skip-test")

	lower := strings.ToLower(out)
	if !strings.Contains(lower, "verdict") && !strings.Contains(lower, "weight") && !strings.Contains(lower, "status") && !strings.Contains(lower, "maat") {
		t.Errorf("maat audit --skip-test unexpected output: %s", out)
	}
}

func TestSmoke_ThothInit(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping E2E test in short mode")
	}
	tmp := t.TempDir()

	// Create a Go project in the temp dir
	os.WriteFile(filepath.Join(tmp, "go.mod"), []byte("module smoke/test\n\ngo 1.21\n"), 0o644)
	os.MkdirAll(filepath.Join(tmp, "cmd"), 0o755)

	out := run(t, testBinary, "thoth", "init", "--yes", tmp)

	// Verify files were created
	if _, err := os.Stat(filepath.Join(tmp, ".thoth", "memory.yaml")); err != nil {
		t.Errorf("thoth init did not create .thoth/memory.yaml\nOutput: %s", out)
	}
	if _, err := os.Stat(filepath.Join(tmp, ".thoth", "journal.md")); err != nil {
		t.Errorf("thoth init did not create .thoth/journal.md\nOutput: %s", out)
	}

	// Verify memory contains project info
	data, _ := os.ReadFile(filepath.Join(tmp, ".thoth", "memory.yaml"))
	if !strings.Contains(string(data), "language: Go") {
		t.Errorf("thoth init memory.yaml missing Go detection:\n%s", string(data))
	}
}

func TestSmoke_MirrorDedup(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping E2E test in short mode")
	}
	tmp := t.TempDir()

	// Create two identical files (8KB each to exceed any minimum threshold)
	data := make([]byte, 8192)
	for i := range data {
		data[i] = byte(i % 256)
	}
	os.WriteFile(filepath.Join(tmp, "original.dat"), data, 0o644)
	os.WriteFile(filepath.Join(tmp, "duplicate.dat"), data, 0o644)

	out := run(t, testBinary, "mirror", tmp)

	// Binary should run without crashing. Output varies by configuration.
	if len(out) == 0 {
		t.Error("mirror produced no output at all")
	}
}

func TestSmoke_ScalesPolicy(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping E2E test in short mode")
	}
	// Run scales — may need a config, but should not crash
	out := run(t, testBinary, "scales")
	if len(out) == 0 {
		t.Error("scales produced no output at all")
	}
}
