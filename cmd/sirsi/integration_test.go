package main_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// testBinary holds the path to the compiled sirsi binary, built once in TestMain.
var testBinary string

// repoRoot is the absolute path to the repository root.
var repoRoot string

func TestMain(m *testing.M) {
	// Determine the repo root (two levels up from cmd/sirsi/).
	wd, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "cannot determine working directory: %v\n", err)
		os.Exit(1)
	}
	repoRoot = filepath.Join(wd, "..", "..")

	// Build the binary once into a temp directory.
	tmpDir, err := os.MkdirTemp("", "sirsi-integration-*")
	if err != nil {
		fmt.Fprintf(os.Stderr, "cannot create temp dir: %v\n", err)
		os.Exit(1)
	}
	defer os.RemoveAll(tmpDir)

	testBinary = filepath.Join(tmpDir, "sirsi")

	buildCtx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	build := exec.CommandContext(buildCtx, "go", "build", "-o", testBinary, "./cmd/sirsi/")
	build.Dir = repoRoot
	build.Env = append(os.Environ(), "CGO_ENABLED=1")
	if out, err := build.CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "FATAL: failed to build sirsi binary:\n%s\n%v\n", out, err)
		os.Exit(1)
	}

	os.Exit(m.Run())
}

// sirsiTestEnv returns the parent environment with every GIT_* variable removed
// and PWD pinned to dir, for handing to a sirsi subprocess.
//
// Stripping GIT_* is what makes per-test router roots actually isolated. When
// the suite runs under a git hook — most importantly the Ma'at pre-push gate,
// which executes inside `git push` — git exports GIT_DIR, GIT_INDEX_FILE,
// GIT_PREFIX, GIT_COMMON_DIR, etc. into the environment, and `go test` (and
// every subprocess it spawns) inherits them. The sirsi binary resolves the
// router root via router.FindRepoRoot, which shells `git rev-parse
// --git-common-dir`; git honors GIT_DIR over the process working directory, so
// a binary launched in an isolated t.TempDir() would resolve the REAL repo's
// router root instead of the temp one — writing there and leaving the temp
// state.json untouched. That is the TestRouterAckLegacyPending gate flake:
// green under a bare `go test` (no GIT_* in the env, so FindRepoRoot falls back
// to the cwd walk-up) but red under the pre-push gate. Removing GIT_* forces
// the subprocess to resolve from its own cwd. Pinning PWD additionally keeps any
// os.Getwd()-based resolution consistent with cmd.Dir.
func sirsiTestEnv(dir string, extra ...string) []string {
	base := os.Environ()
	out := make([]string, 0, len(base)+len(extra)+1)
	for _, kv := range base {
		if strings.HasPrefix(kv, "GIT_") {
			continue
		}
		if dir != "" && strings.HasPrefix(kv, "PWD=") {
			continue
		}
		out = append(out, kv)
	}
	if dir != "" {
		out = append(out, "PWD="+dir)
	}
	// Sandbox the router store: without this, every test send lands in the
	// LIVE ~/.sirsi/router.db (six polluted rows found there 2026-07-07, and
	// the idempotency window made TestRouterPullModelRoundtrip dedupe against
	// a PREVIOUS run's row — flaky by the hour bucket). Per-process temp file;
	// an explicit SIRSI_ROUTER_DB in extra still wins (append order).
	out = append(out, "SIRSI_ROUTER_DB="+filepath.Join(os.TempDir(), fmt.Sprintf("sirsi-test-router-%d.db", os.Getpid())))
	return append(out, extra...)
}

// runSirsi executes the test binary with the given args and a timeout.
// It returns stdout, stderr, and any error (including non-zero exit).
func runSirsi(t *testing.T, timeout time.Duration, args ...string) (stdout, stderr string, err error) {
	return runSirsiWithEnv(t, timeout, nil, args...)
}

func runSirsiWithEnv(t *testing.T, timeout time.Duration, env []string, args ...string) (stdout, stderr string, err error) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, testBinary, args...)
	cmd.Dir = repoRoot
	cmd.Env = sirsiTestEnv(repoRoot, env...)
	// Prevent interactive prompts by closing stdin.
	cmd.Stdin = nil

	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf

	err = cmd.Run()
	return outBuf.String(), errBuf.String(), err
}

// isolatedHomeEnv returns env vars pinning HOME and XDG_* to a per-test temp
// directory so scan/ghost rules don't walk the developer's actual home tree
// (which exceeds the 30-60s test budget on machines with large $HOME).
func isolatedHomeEnv(t *testing.T) []string {
	t.Helper()
	homeDir := t.TempDir()
	return []string{
		"HOME=" + homeDir,
		"XDG_CONFIG_HOME=" + filepath.Join(homeDir, ".config"),
		"XDG_CACHE_HOME=" + filepath.Join(homeDir, ".cache"),
	}
}

// runSirsiInDir is like runSirsi but runs the binary in the given working
// directory instead of repoRoot. Used to isolate router state mutations.
func runSirsiInDir(t *testing.T, dir string, timeout time.Duration, args ...string) (stdout, stderr string, err error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, testBinary, args...)
	cmd.Dir = dir
	// Strip GIT_* (e.g. GIT_DIR leaked by the pre-push gate) and pin PWD so the
	// subprocess resolves its router root from dir, not the gate's repo. See
	// sirsiTestEnv for the full rationale (TestRouterAckLegacyPending flake).
	cmd.Env = sirsiTestEnv(dir)
	cmd.Stdin = nil
	var outBuf, errBuf bytes.Buffer
	cmd.Stdout, cmd.Stderr = &outBuf, &errBuf
	err = cmd.Run()
	return outBuf.String(), errBuf.String(), err
}

func writeRouterTestAgents(t *testing.T, repoRoot string, agents ...string) {
	t.Helper()
	root := filepath.Join(repoRoot, ".agents", "idea-router")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	var b strings.Builder
	b.WriteString(`{"agents":{`)
	for i, agent := range agents {
		if i > 0 {
			b.WriteString(",")
		}
		fmt.Fprintf(&b, "%q:{%q:%q,%q:%q,%q:[%q],%q:%q,%q:%q,%q:{%q:%q}}",
			agent, "id", agent, "type", "test", "command", "true", "cwd", "/tmp",
			"workstream", "test", "wake", "mechanism", "none")
	}
	b.WriteString(`}}`)
	if err := os.WriteFile(filepath.Join(root, "agents.json"), []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestRouterPullModelRoundtrip verifies the new pull-model loop: A sends to B,
// B pulls and sees the item, B closes with a result, B's pull is then empty.
// This is the bare-minimum any-to-any flow, independent of legacy state.json.
func TestRouterPullModelRoundtrip(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	if err := os.MkdirAll(filepath.Join(tmp, ".agents", "idea-router"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeRouterTestAgents(t, tmp, "claude-a", "claude-b")

	stdout, stderr, err := runSirsiInDir(t, tmp, 10*time.Second,
		"router", "send",
		"--from", "claude-a", "--to", "claude-b",
		"--title", "test handoff",
		"--instructions", "do the thing, then close with a one-line summary")
	if err != nil {
		t.Fatalf("send failed: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}
	if !strings.Contains(stdout, "Sent claude-a → claude-b") {
		t.Errorf("expected send confirmation, got: %s", stdout)
	}

	stdoutB, _, err := runSirsiInDir(t, tmp, 10*time.Second, "router", "pull", "claude-b")
	if err != nil {
		t.Fatalf("pull B failed: %v", err)
	}
	if !strings.Contains(stdoutB, "1 open items for claude-b") {
		t.Errorf("expected 1 open item for B, got: %s", stdoutB)
	}
	stdoutA, _, err := runSirsiInDir(t, tmp, 10*time.Second, "router", "pull", "claude-a")
	if err != nil {
		t.Fatalf("pull A failed: %v", err)
	}
	if !strings.Contains(stdoutA, "No open items for claude-a") {
		t.Errorf("expected empty pull for A, got: %s", stdoutA)
	}

	var id string
	for _, line := range strings.Split(stdoutB, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "•") {
			id = strings.TrimSpace(strings.TrimPrefix(line, "•"))
			break
		}
	}
	if id == "" {
		t.Fatalf("could not extract item id:\n%s", stdoutB)
	}

	stdoutShow, _, err := runSirsiInDir(t, tmp, 10*time.Second, "router", "show", id)
	if err != nil {
		t.Fatalf("show failed: %v", err)
	}
	if !strings.Contains(stdoutShow, "claude-a") || !strings.Contains(stdoutShow, "status: open") {
		t.Errorf("show missing expected frontmatter:\n%s", stdoutShow)
	}

	stdoutClose, _, err := runSirsiInDir(t, tmp, 10*time.Second,
		"router", "close", id, "--agent", "claude-b", "--result", "did the thing")
	if err != nil {
		t.Fatalf("close failed: %v", err)
	}
	if !strings.Contains(stdoutClose, "Closed") {
		t.Errorf("expected close confirmation, got: %s", stdoutClose)
	}

	stdoutB2, _, err := runSirsiInDir(t, tmp, 10*time.Second, "router", "pull", "claude-b")
	if err != nil {
		t.Fatalf("pull B after close failed: %v", err)
	}
	if !strings.Contains(stdoutB2, "No open items for claude-b") {
		t.Errorf("expected empty pull after close, got: %s", stdoutB2)
	}

	// Double-close is idempotent: the file is already closed and the store
	// mirror heals (phantom-open fix) — store-only items always behaved this
	// way, so file-backed items now match.
	if _, stderrDC, dcErr := runSirsiInDir(t, tmp, 10*time.Second, "router", "close", id, "--agent", "claude-b"); dcErr != nil {
		t.Errorf("expected double-close to succeed idempotently, got: %v (%s)", dcErr, stderrDC)
	}
}

func TestRouterAckLegacyPending(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	root := filepath.Join(tmp, ".agents", "idea-router")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	state := `{
  "pending": {
    "claude-pantheon": ["item-a", "item-b"],
    "codex-pantheon": ["item-c"]
  },
  "pending_for_claude": ["item-a", "item-z"],
  "pending_for_codex": ["item-c"]
}`
	if err := os.WriteFile(filepath.Join(root, "state.json"), []byte(state), 0o644); err != nil {
		t.Fatal(err)
	}

	stdout, stderr, err := runSirsiInDir(t, tmp, 10*time.Second, "router", "ack", "claude-pantheon", "item-a", "missing-item")
	if err != nil {
		t.Fatalf("ack failed: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}
	if !strings.Contains(stdout, "Acked 2 legacy pending item") {
		t.Fatalf("expected ack confirmation, got: %s", stdout)
	}

	data, err := os.ReadFile(filepath.Join(root, "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if jerr := json.Unmarshal(data, &got); jerr != nil {
		t.Fatal(jerr)
	}
	pending := got["pending"].(map[string]any)
	claudePending := pending["claude-pantheon"].([]any)
	if len(claudePending) != 1 || claudePending[0].(string) != "item-b" {
		t.Fatalf("unexpected claude pending: %#v", claudePending)
	}
	mirror := got["pending_for_claude"].([]any)
	if len(mirror) != 1 || mirror[0].(string) != "item-z" {
		t.Fatalf("unexpected claude mirror: %#v", mirror)
	}
	if got["last_claude_read"] == nil {
		t.Fatalf("last_claude_read was not bumped")
	}

	_, stderr, err = runSirsiInDir(t, tmp, 10*time.Second, "router", "ack", "claude-pantheon", "item-a")
	if err != nil {
		t.Fatalf("second ack should be idempotent: %v\nstderr: %s", err, stderr)
	}
}

// --- Table-Driven Deity Command Tests ---

// deityTest defines a single integration test case for a CLI command.
type deityTest struct {
	name           string
	args           []string
	timeout        time.Duration
	wantExit0      bool
	outputContains []string // substrings expected in combined stdout+stderr
	skipShort      bool     // skip when -short flag is set
	skipReason     string   // reason for skip (displayed with t.Skip)
}

func TestVersion(t *testing.T) {
	t.Parallel()

	stdout, _, err := runSirsi(t, 10*time.Second, "version")
	if err != nil {
		t.Fatalf("sirsi version failed: %v", err)
	}

	combined := stdout
	// Version is stamped via ldflags (internal/version), not a frozen literal,
	// so assert the banner renders rather than a specific number (ADR-023).
	if !strings.Contains(combined, "Sirsi Pantheon") {
		t.Errorf("version output missing 'Sirsi Pantheon' banner, got:\n%s", combined)
	}
}

func TestHelp(t *testing.T) {
	t.Parallel()

	stdout, stderr, err := runSirsi(t, 10*time.Second, "--help")
	if err != nil {
		t.Fatalf("sirsi --help failed: %v", err)
	}

	combined := stdout + stderr
	if !strings.Contains(combined, "Pantheon") {
		t.Errorf("help output missing 'Pantheon', got:\n%s", combined)
	}
	if !strings.Contains(combined, "sirsi") {
		t.Errorf("help output missing 'sirsi' command references, got:\n%s", combined)
	}
}

func TestAnubisWeigh(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("skipping scan in short mode (may take several seconds)")
	}

	stdout, stderr, err := runSirsi(t, 60*time.Second, "anubis", "weigh", "--json")
	if err != nil {
		t.Fatalf("sirsi anubis weigh --json failed: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}

	// JSON mode outputs structured data to stdout.
	if len(stdout) == 0 {
		t.Error("expected non-empty JSON output from anubis weigh")
	}
}

func TestAnubisWeighTerminal(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("skipping scan in short mode")
	}

	stdout, stderr, err := runSirsiWithEnv(t, 60*time.Second, isolatedHomeEnv(t), "scan")
	if err != nil {
		t.Fatalf("sirsi scan failed: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}

	combined := stdout + stderr
	// Terminal mode should contain either "Waste Found" in dashboard or "Completed in" in footer.
	if !strings.Contains(combined, "Waste Found") && !strings.Contains(combined, "Completed in") {
		t.Errorf("scan output missing expected patterns, got:\n%s", combined)
	}
}

func TestAnubisKa(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("skipping ghost scan in short mode")
	}

	stdout, stderr, err := runSirsiWithEnv(t, 30*time.Second, isolatedHomeEnv(t), "ghosts")
	if err != nil {
		t.Fatalf("sirsi ghosts failed: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}

	combined := stdout + stderr
	// Ghost scan should produce dashboard output with "Ghosts" count.
	if !strings.Contains(combined, "Ghost apps") && !strings.Contains(combined, "ghost") && !strings.Contains(combined, "Completed in") {
		t.Errorf("ghost scan output missing expected patterns, got:\n%s", combined)
	}
}

func TestDoctor(t *testing.T) {
	t.Parallel()

	stdout, stderr, err := runSirsi(t, 30*time.Second, "doctor", "--json")
	if err != nil {
		t.Fatalf("sirsi doctor --json failed: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}

	if len(stdout) == 0 {
		t.Error("expected non-empty JSON output from doctor")
	}

	// Also test terminal mode for the professional system brief.
	stdout2, stderr2, err := runSirsi(t, 30*time.Second, "doctor")
	if err != nil {
		t.Fatalf("sirsi doctor failed: %v\nstdout: %s\nstderr: %s", err, stdout2, stderr2)
	}

	combined := stdout2 + stderr2
	if !strings.Contains(combined, "Pantheon System Brief") || !strings.Contains(combined, "Recommended action") {
		t.Errorf("doctor output missing system brief contract, got:\n%s", combined)
	}
}

func TestIsisNetwork(t *testing.T) {
	t.Parallel()

	stdout, stderr, err := runSirsi(t, 30*time.Second, "isis", "network", "--json")
	if err != nil {
		t.Fatalf("sirsi isis network --json failed: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}

	if len(stdout) == 0 {
		t.Error("expected non-empty JSON output from isis network")
	}

	// Also test terminal mode for Security Score.
	stdout2, stderr2, err := runSirsi(t, 30*time.Second, "network")
	if err != nil {
		t.Fatalf("sirsi network failed: %v\nstdout: %s\nstderr: %s", err, stdout2, stderr2)
	}

	combined := stdout2 + stderr2
	if !strings.Contains(combined, "Security Score") && !strings.Contains(combined, "Completed in") {
		t.Errorf("network output missing 'Security Score' or 'Completed in', got:\n%s", combined)
	}
}

func TestSebaHardware(t *testing.T) {
	t.Parallel()

	stdout, stderr, err := runSirsi(t, 15*time.Second, "hardware", "--json")
	if err != nil {
		t.Fatalf("sirsi hardware --json failed: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}

	if len(stdout) == 0 {
		t.Error("expected non-empty JSON output from hardware")
	}

	// Terminal mode should show hardware details.
	stdout2, stderr2, err := runSirsi(t, 15*time.Second, "hardware")
	if err != nil {
		t.Fatalf("sirsi hardware failed: %v\nstdout: %s\nstderr: %s", err, stdout2, stderr2)
	}

	combined := stdout2 + stderr2
	if !strings.Contains(combined, "CPU") && !strings.Contains(combined, "SEBA") {
		t.Errorf("hardware output missing expected content, got:\n%s", combined)
	}
}

func TestHorusStats(t *testing.T) {
	t.Parallel()

	stdout, stderr, err := runSirsi(t, 30*time.Second, "horus", "scan", ".")
	if err != nil {
		t.Fatalf("sirsi horus scan failed: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}

	combined := stdout + stderr
	if !strings.Contains(combined, "Files") {
		t.Errorf("horus scan output missing 'Files', got:\n%s", combined)
	}
}

func TestOsirisStatus(t *testing.T) {
	t.Parallel()

	stdout, stderr, err := runSirsi(t, 15*time.Second, "osiris", "status", "--json")
	if err != nil {
		t.Fatalf("sirsi osiris status --json failed: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}

	if len(stdout) == 0 {
		t.Error("expected non-empty JSON output from osiris status")
	}
}

func TestOsirisAssess(t *testing.T) {
	t.Parallel()

	stdout, stderr, err := runSirsi(t, 15*time.Second, "osiris", "assess", "--json")
	if err != nil {
		t.Fatalf("sirsi osiris assess --json failed: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}

	if len(stdout) == 0 {
		t.Error("expected non-empty JSON output from osiris assess")
	}

	// Terminal mode should show risk information.
	stdout2, stderr2, err := runSirsi(t, 15*time.Second, "osiris", "assess")
	if err != nil {
		t.Fatalf("sirsi osiris assess failed: %v\nstdout: %s\nstderr: %s", err, stdout2, stderr2)
	}

	combined := stdout2 + stderr2
	if !strings.Contains(combined, "Risk") && !strings.Contains(combined, "OSIRIS") {
		t.Errorf("osiris assess output missing expected content, got:\n%s", combined)
	}
}

func TestMaatPulse(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("skipping maat pulse in short mode (runs go test internally)")
	}

	stdout, stderr, err := runSirsi(t, 5*time.Minute, "maat", "pulse", "--skip-test", "--json")
	if err != nil {
		t.Fatalf("sirsi maat pulse --skip-test --json failed: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}

	if len(stdout) == 0 {
		t.Error("expected non-empty JSON output from maat pulse")
	}
}

// TestDeityCommands is the master table-driven test that covers all deity
// commands with exit code and output pattern verification.
func TestDeityCommands(t *testing.T) {
	tests := []deityTest{
		{
			name:           "version",
			args:           []string{"version"},
			timeout:        10 * time.Second,
			wantExit0:      true,
			outputContains: []string{"Pantheon"},
		},
		{
			name:           "help",
			args:           []string{"--help"},
			timeout:        10 * time.Second,
			wantExit0:      true,
			outputContains: []string{"Pantheon", "sirsi"},
		},
		{
			name:           "anubis_help",
			args:           []string{"anubis", "--help"},
			timeout:        10 * time.Second,
			wantExit0:      true,
			outputContains: []string{"Anubis"},
		},
		{
			name:           "maat_help",
			args:           []string{"maat", "--help"},
			timeout:        10 * time.Second,
			wantExit0:      true,
			outputContains: []string{"Ma'at"},
		},
		{
			name:           "seba_help",
			args:           []string{"seba", "--help"},
			timeout:        10 * time.Second,
			wantExit0:      true,
			outputContains: []string{"Seba"},
		},
		{
			name:           "osiris_help",
			args:           []string{"osiris", "--help"},
			timeout:        10 * time.Second,
			wantExit0:      true,
			outputContains: []string{"Osiris"},
		},
		{
			name:           "isis_help",
			args:           []string{"isis", "--help"},
			timeout:        10 * time.Second,
			wantExit0:      true,
			outputContains: []string{"Isis"},
		},
		{
			name:           "horus_help",
			args:           []string{"horus", "--help"},
			timeout:        10 * time.Second,
			wantExit0:      true,
			outputContains: []string{"Horus"},
		},
		{
			name:           "doctor_json",
			args:           []string{"doctor", "--json"},
			timeout:        30 * time.Second,
			wantExit0:      true,
			outputContains: []string{"{"},
		},
		{
			name:           "hardware_json",
			args:           []string{"hardware", "--json"},
			timeout:        15 * time.Second,
			wantExit0:      true,
			outputContains: []string{"{"},
		},
		{
			name:           "network_json",
			args:           []string{"network", "--json"},
			timeout:        30 * time.Second,
			wantExit0:      true,
			outputContains: []string{"{"},
		},
		{
			name:           "osiris_status_json",
			args:           []string{"osiris", "status", "--json"},
			timeout:        15 * time.Second,
			wantExit0:      true,
			outputContains: []string{"{"},
		},
		{
			name:           "osiris_assess_json",
			args:           []string{"osiris", "assess", "--json"},
			timeout:        15 * time.Second,
			wantExit0:      true,
			outputContains: []string{"{"},
		},
		{
			name:           "horus_scan",
			args:           []string{"horus", "scan", "."},
			timeout:        30 * time.Second,
			wantExit0:      true,
			outputContains: []string{"Files"},
		},
		{
			name:           "scan_json",
			args:           []string{"scan", "--json"},
			timeout:        60 * time.Second,
			wantExit0:      true,
			outputContains: []string{"{"},
			skipShort:      true,
			skipReason:     "full scan may take several seconds",
		},
		{
			name:           "ghosts",
			args:           []string{"ghosts"},
			timeout:        30 * time.Second,
			wantExit0:      true,
			outputContains: []string{"Ghost apps"},
			skipShort:      true,
			skipReason:     "ghost scan may take several seconds",
		},
		{
			name:           "maat_pulse_skip_test",
			args:           []string{"maat", "pulse", "--skip-test", "--json"},
			timeout:        5 * time.Minute,
			wantExit0:      true,
			outputContains: []string{"{"},
			skipShort:      true,
			skipReason:     "pulse runs real measurements",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if tt.skipShort && testing.Short() {
				t.Skip(tt.skipReason)
			}

			// Scan/ghosts walk $HOME by default. Pin them to an empty temp
			// HOME so the test runtime doesn't depend on the developer's disk.
			var env []string
			if tt.name == "scan_json" || tt.name == "ghosts" {
				homeDir := t.TempDir()
				env = []string{
					"HOME=" + homeDir,
					"XDG_CONFIG_HOME=" + filepath.Join(homeDir, ".config"),
					"XDG_CACHE_HOME=" + filepath.Join(homeDir, ".cache"),
				}
			}

			stdout, stderr, err := runSirsiWithEnv(t, tt.timeout, env, tt.args...)
			combined := stdout + stderr

			if tt.wantExit0 && err != nil {
				t.Fatalf("command %v failed (wanted exit 0): %v\noutput:\n%s", tt.args, err, combined)
			}

			for _, want := range tt.outputContains {
				if !strings.Contains(combined, want) {
					t.Errorf("output missing %q for command %v\noutput:\n%s", want, tt.args, combined)
				}
			}
		})
	}
}

// TestNextStepsPresent is a table-driven test verifying that commands which
// produce NextSteps suggestions include "sirsi" in their output (as a proxy
// for the suggestion containing a follow-up command).
func TestNextStepsPresent(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping NextSteps verification in short mode")
	}

	tests := []struct {
		name        string
		args        []string
		isolateHome bool
	}{
		{"scan_next_steps", []string{"scan"}, true},
		{"ghosts_next_steps", []string{"ghosts"}, true},
		{"doctor_next_steps", []string{"doctor"}, false},
		{"network_next_steps", []string{"network"}, false},
		{"hardware_next_steps", []string{"hardware"}, false},
		{"osiris_assess_next_steps", []string{"osiris", "assess"}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var env []string
			if tt.isolateHome {
				homeDir := t.TempDir()
				env = []string{
					"HOME=" + homeDir,
					"XDG_CONFIG_HOME=" + filepath.Join(homeDir, ".config"),
					"XDG_CACHE_HOME=" + filepath.Join(homeDir, ".cache"),
				}
			}

			stdout, stderr, err := runSirsiWithEnv(t, 60*time.Second, env, tt.args...)
			if err != nil {
				t.Fatalf("command %v failed: %v", tt.args, err)
			}

			combined := stdout + stderr
			// NextSteps suggestions reference follow-up sirsi commands.
			if !strings.Contains(combined, "sirsi") {
				t.Errorf("output for %v missing 'sirsi' (expected NextSteps suggestion)\noutput:\n%s",
					tt.args, combined)
			}
		})
	}
}

// TestBinaryExists verifies the test binary was built successfully.
func TestBinaryExists(t *testing.T) {
	t.Parallel()

	info, err := os.Stat(testBinary)
	if err != nil {
		t.Fatalf("test binary does not exist at %s: %v", testBinary, err)
	}
	if info.Size() == 0 {
		t.Fatal("test binary has zero size")
	}
	// Verify it is executable.
	if info.Mode()&0111 == 0 {
		t.Fatal("test binary is not executable")
	}
}

// TestUXContract_JSONClean verifies that --json commands emit only valid JSON
// to stdout with no styled UI framing (banner, header, progress text).
// This directly addresses Codex review blocking finding #3.
func TestUXContract_JSONClean(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping UX contract tests in short mode")
	}

	tests := []struct {
		name string
		args []string
	}{
		{"audit_json", []string{"maat", "audit", "--skip-test", "--json"}},
		{"risk_json", []string{"risk", "--json"}},
		{"status_json", []string{"status", "--json"}},
		{"network_json", []string{"network", "--json"}},
		{"diagnose_json", []string{"diagnose", "--json"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			stdout, _, err := runSirsi(t, 2*time.Minute, tt.args...)
			if err != nil {
				t.Fatalf("command %v failed: %v", tt.args, err)
			}

			// stdout must start with '{' or '[' (valid JSON)
			trimmed := strings.TrimSpace(stdout)
			if len(trimmed) == 0 {
				t.Fatalf("command %v produced empty stdout", tt.args)
			}
			if trimmed[0] != '{' && trimmed[0] != '[' {
				t.Errorf("command %v stdout is not clean JSON — starts with %q\nfirst 200 chars:\n%s",
					tt.args, string(trimmed[0]), trimmed[:min(200, len(trimmed))])
			}

			// stdout must NOT contain ANSI escape codes or banner text
			if strings.Contains(stdout, "P A N T H E O N") {
				t.Errorf("command %v stdout contains banner text (should be JSON only)", tt.args)
			}
			if strings.Contains(stdout, "\033[") {
				t.Errorf("command %v stdout contains ANSI escape codes", tt.args)
			}
		})
	}
}

// TestUXContract_RecommendedAction verifies that normal-mode commands emit
// an explicit follow-up. Serious diagnostic commands use the professional
// "Recommended action" brief instead of the legacy "What's Next" block.
// This directly addresses Codex review blocking finding #4.
func TestUXContract_RecommendedAction(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping UX contract tests in short mode")
	}

	tests := []struct {
		name    string
		args    []string
		timeout time.Duration
	}{
		{"scan", []string{"scan"}, 3 * time.Minute},
		{"ghosts", []string{"ghosts"}, 3 * time.Minute},
		{"diagnose", []string{"diagnose"}, 60 * time.Second},
		{"network", []string{"network"}, 60 * time.Second},
		{"risk", []string{"risk"}, 30 * time.Second},
		{"status", []string{"status"}, 30 * time.Second},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			homeDir := t.TempDir()
			env := []string{
				"HOME=" + homeDir,
				"XDG_CONFIG_HOME=" + filepath.Join(homeDir, ".config"),
				"XDG_CACHE_HOME=" + filepath.Join(homeDir, ".cache"),
			}

			stdout, stderr, err := runSirsiWithEnv(t, tt.timeout, env, tt.args...)
			if err != nil {
				t.Fatalf("command %v failed: %v", tt.args, err)
			}

			combined := stdout + stderr
			if !strings.Contains(combined, "What's Next") && !strings.Contains(combined, "Recommended action") {
				t.Errorf("command %v missing follow-up action section\noutput:\n%s",
					tt.args, combined[:min(500, len(combined))])
			}
		})
	}
}

// TestUXContract_NoDeityVocab verifies that normal-mode output does not
// expose internal deity/module names to users.
// This directly addresses Codex review requirement: deity vocabulary hidden.
func TestUXContract_NoDeityVocab(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping UX contract tests in short mode")
	}

	// These deity names should NOT appear in user-facing output (normal mode)
	deityNames := []string{"𓆄", "𓁹", "𓁐", "Anubis", "Osiris", "Isis", "Jackal", "Scarab"}

	tests := []struct {
		name string
		args []string
	}{
		{"risk", []string{"risk"}},
		{"status", []string{"status"}},
		{"diagnose", []string{"diagnose"}},
		{"network", []string{"network"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// `risk` ECHOES repo data (branch name, last commit message) — a
			// host checkout whose tip commit mentions a deity is DATA, not UI
			// chrome, and must not fail the vocab law. Run it against a
			// controlled fixture repo so the test owns every string it greps
			// (2026-07-09: the pushed tip "feat(menubar): Osiris — …" made
			// this test unpassable in its own feature worktree).
			dir := repoRoot
			if tt.name == "risk" {
				dir = t.TempDir()
				for _, ga := range [][]string{
					{"init", "-q"},
					{"config", "user.email", "vocab@test"},
					{"config", "user.name", "vocab-test"},
					{"commit", "--allow-empty", "-q", "-m", "plain fixture commit"},
				} {
					gc := exec.Command("git", ga...)
					gc.Dir = dir
					for _, kv := range os.Environ() {
						if !strings.HasPrefix(kv, "GIT_") {
							gc.Env = append(gc.Env, kv)
						}
					}
					if out, gerr := gc.CombinedOutput(); gerr != nil {
						t.Fatalf("fixture git %v: %v\n%s", ga, gerr, out)
					}
				}
			}
			stdout, stderr, err := runSirsiInDir(t, dir, 60*time.Second, tt.args...)
			if err != nil {
				t.Fatalf("command %v failed: %v", tt.args, err)
			}

			combined := stdout + stderr
			for _, deity := range deityNames {
				if strings.Contains(combined, deity) {
					t.Errorf("command %v exposes deity vocabulary %q in user-facing output",
						tt.args, deity)
				}
			}
		})
	}
}

// TestUXContract_StatusCLI verifies the new status non-interactive mode.
// This directly addresses Codex review blocking finding #2.
func TestUXContract_StatusCLI(t *testing.T) {
	t.Parallel()

	stdout, stderr, err := runSirsi(t, 30*time.Second, "status")
	if err != nil {
		t.Fatalf("sirsi status failed: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}

	combined := stdout + stderr
	// Must show health score
	if !strings.Contains(combined, "health") && !strings.Contains(combined, "Health") {
		t.Errorf("status output missing health info\noutput:\n%s", combined)
	}
	// Must show the professional action prompt.
	if !strings.Contains(combined, "Recommended action") {
		t.Errorf("status output missing recommended action section\noutput:\n%s", combined)
	}
	// Per ADR-018 the TUI was eliminated 2026-05-21; the prior
	// `--live` suggestion was removed with it. CLI status output is the
	// authoritative surface for terminal users until the native Mac app
	// ships.
}

// TestSubcommandHelp verifies every registered subcommand's --help exits 0.
func TestSubcommandHelp(t *testing.T) {
	t.Parallel()

	subcommands := []string{
		"scan", "ghosts", "dedup", "guard", "doctor", "judge", "clean",
		"network", "hardware", "quality", "diagram",
		"anubis", "seba", "osiris", "isis", "maat",
		"thoth", "seshat", "horus", "rtk", "vault",
		"version", "mcp",
	}

	for _, sub := range subcommands {
		t.Run(sub+"_help", func(t *testing.T) {
			t.Parallel()

			_, _, err := runSirsi(t, 10*time.Second, sub, "--help")
			if err != nil {
				t.Errorf("sirsi %s --help failed: %v", sub, err)
			}
		})
	}
}

// TestRouterRespondStoreOnlyItem is the command-level smoke codex-pantheon's
// PR #294 review asked for: with the cutover on (store-only items, no
// items/<id>.md), `router respond` must close the request with its Result AND
// leave exactly one type:decision inbound in the requester's queue. It also
// pins the survivable ordering — a rerun dedupes rather than double-notifying,
// which is what makes notify-before-close safe to retry.
func TestRouterRespondStoreOnlyItem(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	if err := os.MkdirAll(filepath.Join(tmp, ".agents", "idea-router"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeRouterTestAgents(t, tmp, "claude-fw", "claude-home")
	db := filepath.Join(tmp, "router.db")
	env := func() []string {
		return sirsiTestEnv(tmp, "SIRSI_ROUTER_STORE_WAKE=1", "SIRSI_ROUTER_DB="+db)
	}
	run := func(args ...string) (string, string, error) {
		t.Helper()
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, testBinary, args...)
		cmd.Dir, cmd.Env, cmd.Stdin = tmp, env(), nil
		var o, e bytes.Buffer
		cmd.Stdout, cmd.Stderr = &o, &e
		err := cmd.Run()
		return o.String(), e.String(), err
	}

	out, errOut, err := run("router", "send", "--from", "claude-fw", "--to", "claude-home",
		"--type", "review", "--title", "bind something", "--instructions", "please bind")
	if err != nil {
		t.Fatalf("send failed: %v\n%s\n%s", err, out, errOut)
	}
	id := ""
	if i := strings.LastIndex(out, ": "); i >= 0 {
		id = strings.TrimSpace(out[i+2:])
	}
	if id == "" {
		t.Fatalf("could not parse sent id from: %s", out)
	}
	// Cutover on: the item exists only as a store row.
	if _, statErr := os.Stat(filepath.Join(tmp, ".agents", "idea-router", "items", id+".md")); statErr == nil {
		t.Fatalf("expected a store-only item, but %s.md exists", id)
	}

	out, errOut, err = run("router", "respond", id, "--result", "merged at abc123")
	if err == nil || !strings.Contains(errOut, "could not resolve the current agent") {
		t.Fatalf("ambiguous respond must fail closed: err=%v\n%s\n%s", err, out, errOut)
	}
	out, errOut, err = run("router", "respond", id, "--agent", "claude-fw", "--result", "merged at abc123")
	if err == nil || !strings.Contains(errOut, `addressed to "claude-home"`) {
		t.Fatalf("wrong declared actor must fail closed: err=%v\n%s\n%s", err, out, errOut)
	}
	out, errOut, err = run("router", "respond", id, "--agent", "claude-home", "--result", "merged at abc123")
	if err != nil {
		t.Fatalf("respond failed: %v\n%s\n%s", err, out, errOut)
	}
	for _, want := range []string{"Notified claude-fw", "Closed " + id} {
		if !strings.Contains(out, want) {
			t.Fatalf("respond output missing %q:\n%s", want, out)
		}
	}

	// The requester now has exactly one response waiting...
	out, _, err = run("router", "pull", "claude-fw")
	if err != nil {
		t.Fatalf("pull requester failed: %v", err)
	}
	if !strings.Contains(out, "1 open items for claude-fw") || !strings.Contains(out, "RESPONSE: bind something") {
		t.Fatalf("requester did not get exactly one response inbound:\n%s", out)
	}
	// ...and the original request is closed, so the responder's queue is empty.
	out, _, err = run("router", "pull", "claude-home")
	if err != nil {
		t.Fatalf("pull responder failed: %v", err)
	}
	if !strings.Contains(out, "No open items for claude-home") {
		t.Fatalf("expected the request to be closed, got:\n%s", out)
	}

	// Rerunning respond on the closed item must not mint a SECOND notification
	// — the idem_key dedupe is what makes the notify-first order retry-safe.
	_, _, _ = run("router", "respond", id, "--result", "merged at abc123")
	out, _, err = run("router", "pull", "claude-fw")
	if err != nil {
		t.Fatalf("pull requester after rerun failed: %v", err)
	}
	if !strings.Contains(out, "1 open items for claude-fw") {
		t.Fatalf("rerun duplicated the response instead of deduping:\n%s", out)
	}
}
