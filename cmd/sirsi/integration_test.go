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

// testStoreDB is the router store every sirsi subprocess in this test binary
// writes to. It lives inside TestMain's MkdirTemp directory, which is unique per
// test-binary run and removed when the run ends.
//
// It must NOT be keyed on os.Getpid(). That was the previous scheme, and a pid is
// unique only among LIVE processes — it is recycled, while the file it named is
// not removed. On a long-lived self-hosted runner those files accumulate (199 were
// found in one TMPDIR on 2026-08-06, all from a single day), so a `go test` that
// draws a recycled pid opens a PREVIOUS run's store. It already contains that
// run's `claude-a → claude-b "test handoff"` row, the send idempotency window
// dedupes against it, no new item is created, and TestRouterPullModelRoundtrip
// fails at "send failed: exit status 1" — intermittently, with no code delta,
// which is exactly how it reddened PR #573 at 187d8cb1 while passing at the same
// SHA on re-run.
var testStoreDB string

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
	testBinary = filepath.Join(tmpDir, "sirsi")
	testStoreDB = filepath.Join(tmpDir, "router.db")

	buildCtx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	build := exec.CommandContext(buildCtx, "go", "build", "-o", testBinary, "./cmd/sirsi/")
	build.Dir = repoRoot
	build.Env = append(os.Environ(), "CGO_ENABLED=1")
	if out, err := build.CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "FATAL: failed to build sirsi binary:\n%s\n%v\n", out, err)
		os.Exit(1)
	}

	// os.Exit does not run deferred functions, so tmpDir must be removed
	// explicitly — a `defer os.RemoveAll(tmpDir)` here never fires and leaks the
	// build directory and the router store on every run.
	code := m.Run()
	os.RemoveAll(tmpDir)
	os.Exit(code)
}

// sirsiTestEnv returns the parent environment with every GIT_* variable removed,
// PWD pinned to cwd, and SIRSI_ROUTER_DB pinned to storeDB, for handing to a
// sirsi subprocess.
//
// cwd and storeDB are SEPARATE parameters on purpose. An earlier revision took
// one directory and derived the store from it as <dir>/router-test.db. That
// silently coupled "where the subprocess runs" to "which store it writes",
// so runSirsiWithEnv — which must run in repoRoot for the binary to resolve the
// real repo — got repoRoot/router-test.db: a repo-local database shared across
// parallel tests AND across test-binary runs, outside TestMain's cleanup. That
// is the very persistent-shared-store class this file exists to eliminate.
// Every caller now names its store explicitly; nothing is inferred from cwd.
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
func sirsiTestEnv(cwd, storeDB string, extra ...string) []string {
	dir := cwd
	base := os.Environ()
	out := make([]string, 0, len(base)+len(extra)+1)
	for _, kv := range base {
		if strings.HasPrefix(kv, "GIT_") {
			continue
		}
		// Identity inputs must come from the test, never the host. resolveCurrentAgent
		// consults $SIRSI_AGENT_ID and then the session marker at
		// ~/.claude/run/agent-by-session/$CLAUDE_CODE_SESSION_ID. Inheriting either
		// makes identity tests environment-dependent: they pass in CI (no session)
		// and fail on any developer machine running inside a CCD session, which is
		// exactly how TestRouterRespondStoreOnlyItem broke. Worse, `thread register`
		// WRITES that marker, so tests inheriting a real session id overwrite the
		// live operator's identity — 99 markers on this host had been rewritten to
		// "test-adr024-idem", including the running conduit's own.
		if strings.HasPrefix(kv, "SIRSI_AGENT_ID=") || strings.HasPrefix(kv, "CLAUDE_CODE_SESSION_ID=") {
			continue
		}
		if dir != "" && strings.HasPrefix(kv, "PWD=") {
			continue
		}
		// Strip the ambient acting-identity sources. resolveCurrentAgent
		// resolves --agent → $SIRSI_AGENT_ID → session marker → sole live
		// thread. Rungs 1/2/4 are already hermetic here (the flag is explicit,
		// the thread registry is read from the test's own routerRoot), but the
		// session marker is an ABSOLUTE host path keyed on an INHERITED env
		// var: ~/.claude/run/agent-by-session/$CLAUDE_CODE_SESSION_ID. A
		// subprocess launched from a registered agent's session therefore
		// resolves that agent's id, and a test asserting "identity is
		// ambiguous, fail closed" cannot construct ambiguity at all. That is
		// green on CI (no session id in the env) and red on any agent's
		// machine — TestRouterRespondStoreOnlyItem resolved a live `claude-home`
		// and completed a respond the test required to fail. The subprocess
		// cannot use router.sessionMarkerDirOverride (in-process only), so the
		// env is the seam.
		if strings.HasPrefix(kv, "CLAUDE_CODE_SESSION_ID=") || strings.HasPrefix(kv, "SIRSI_AGENT_ID=") {
			continue
		}
		out = append(out, kv)
	}
	if dir != "" {
		out = append(out, "PWD="+dir)
	}
	// Sandbox the router store: without this, every test send lands in the
	// LIVE ~/.sirsi/router.db (six polluted rows found there 2026-07-07).
	//
	// The caller names the store. Mutating router tests pass a path inside their
	// own t.TempDir(), so each gets a virgin store — that is what actually
	// defeats the send idempotency window: dedupe keys on (from, to, title)
	// within a time bucket, so any two tests — or any two runs — that share a
	// store and send the same logical item will see the second send return
	// "Deduped ... nothing appended" and fail on an item that was never created.
	// A per-run store narrows that race; only a per-test store closes it.
	// Read-only commands (version, help) pass testStoreDB, the per-run store
	// inside TestMain's MkdirTemp, which is removed when the run ends.
	//
	// An explicit SIRSI_ROUTER_DB in extra still wins (append order).
	out = append(out, "SIRSI_ROUTER_DB="+storeDB)
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
	// cwd is repoRoot so the binary resolves the real repo, but the store is the
	// per-run temp one — never repoRoot/router-test.db.
	cmd.Dir = repoRoot
	cmd.Env = sirsiTestEnv(repoRoot, testStoreDB, env...)
	// Prevent interactive prompts by closing stdin.
	cmd.Stdin = nil

	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf

	err = cmd.Run()
	return outBuf.String(), errBuf.String(), err
}

// TestIntegrationStoreIsNeverRepoLocal is the regression guard for the coupling
// that made the per-test-store fix incomplete: sirsiTestEnv used to derive the
// store from the subprocess cwd, so runSirsi/runSirsiWithEnv — which must run in
// repoRoot — silently wrote to repoRoot/router-test.db, a database shared across
// parallel tests and across test-binary runs and outside TestMain's cleanup.
//
// It asserts the property directly rather than through a subprocess, because the
// bug is in how the env is CONSTRUCTED, not in how the binary reads it.
func TestIntegrationStoreIsNeverRepoLocal(t *testing.T) {
	t.Parallel()

	storeOf := func(env []string) string {
		got := ""
		for _, kv := range env {
			if v, ok := strings.CutPrefix(kv, "SIRSI_ROUTER_DB="); ok {
				got = v // last wins, matching exec semantics
			}
		}
		return got
	}

	// The env runSirsi/runSirsiWithEnv actually hand to the subprocess.
	got := storeOf(sirsiTestEnv(repoRoot, testStoreDB))
	if got != testStoreDB {
		t.Errorf("runSirsi store = %q, want the per-run temp store %q", got, testStoreDB)
	}
	if strings.HasPrefix(got, repoRoot+string(filepath.Separator)) {
		t.Errorf("runSirsi store %q is inside the repo; it must live in TestMain's temp dir", got)
	}

	// runSirsiInDir keeps its per-test store, and two distinct dirs never collide.
	a, b := t.TempDir(), t.TempDir()
	sa := storeOf(sirsiTestEnv(a, filepath.Join(a, "router-test.db")))
	sb := storeOf(sirsiTestEnv(b, filepath.Join(b, "router-test.db")))
	if sa == sb {
		t.Errorf("two isolated dirs resolved to the same store %q", sa)
	}
	if !strings.HasPrefix(sa, a) {
		t.Errorf("runSirsiInDir store %q is not inside its own dir %q", sa, a)
	}

	// An explicit override in extra still wins (append order).
	if got := storeOf(sirsiTestEnv(repoRoot, testStoreDB, "SIRSI_ROUTER_DB=/tmp/override.db")); got != "/tmp/override.db" {
		t.Errorf("explicit SIRSI_ROUTER_DB override = %q, want it to win", got)
	}
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
// directory instead of repoRoot, with its store inside that same directory.
// Used to isolate router state mutations: dir MUST be a per-test isolated
// directory (a t.TempDir()), because every call sharing a dir shares a store.
func runSirsiInDir(t *testing.T, dir string, timeout time.Duration, args ...string) (stdout, stderr string, err error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, testBinary, args...)
	cmd.Dir = dir
	// Strip GIT_* (e.g. GIT_DIR leaked by the pre-push gate) and pin PWD so the
	// subprocess resolves its router root from dir, not the gate's repo. See
	// sirsiTestEnv for the full rationale (TestRouterAckLegacyPending flake).
	cmd.Env = sirsiTestEnv(dir, filepath.Join(dir, "router-test.db"))
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
	if testing.Short() {
		t.Skip("skipping horus scan in short mode (may take several seconds)")
	}
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
			skipShort:      true,
			skipReason:     "horus scan may take several seconds",
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
		return sirsiTestEnv(tmp, db, "SIRSI_ROUTER_STORE_WAKE=1")
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
	out, errOut, err = run("router", "respond", id, "--agent", "ghost", "--result", "merged at abc123")
	if err == nil || !strings.Contains(errOut, `acting agent "ghost"`) {
		t.Fatalf("undeclared actor must fail closed: err=%v\n%s\n%s", err, out, errOut)
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
