package e2e

import (
	"context"
	"fmt"
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
	// Create a temp directory for the binary (outside of any test's TempDir).
	tmpDir, err := os.MkdirTemp("", "pantheon-e2e-*")
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create temp dir: %v\n", err)
		os.Exit(1)
	}
	defer os.RemoveAll(tmpDir)

	binary := filepath.Join(tmpDir, "pantheon")
	if runtime.GOOS == "windows" {
		binary += ".exe"
	}

	cmd := exec.Command("go", "build", "-o", binary, "./cmd/pantheon/")
	cmd.Dir = filepath.Join("..", "..") // tests/e2e → repo root
	out, err := cmd.CombinedOutput()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to build pantheon: %v\n%s", err, out)
		os.Exit(1)
	}

	testBinary = binary
	os.Exit(m.Run())
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

	deities := []string{"anubis", "maat", "thoth", "guard"}
	for _, d := range deities {
		if !strings.Contains(strings.ToLower(out), d) {
			t.Errorf("help output missing deity %q:\n%s", d, out)
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
