package main_test

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// adr024RegisterOutput mirrors the JSON shape `thread register --json` emits
// after ADR-024: the thread fields plus the prescribed watcher block.
type adr024RegisterOutput struct {
	ThreadID string `json:"thread_id"`
	Surface  string `json:"surface"`
	Watcher  struct {
		Type               string `json:"type"`
		Mechanism          string `json:"mechanism"`
		ArmInstruction     string `json:"arm_instruction"`
		HeartbeatIntervalS int    `json:"heartbeat_interval_s"`
		WatchesInbox       bool   `json:"watches_inbox"`
		Resident           bool   `json:"resident"`
	} `json:"watcher"`
}

func setupTempRouter(t *testing.T) string {
	t.Helper()
	tmp := t.TempDir()
	if err := os.MkdirAll(filepath.Join(tmp, ".agents", "idea-router"), 0o755); err != nil {
		t.Fatal(err)
	}
	return tmp
}

func registerThread(t *testing.T, dir, agent, surface string) adr024RegisterOutput {
	t.Helper()
	stdout, stderr, err := runSirsiInDir(t, dir, 10*time.Second,
		"thread", "register",
		"--agent", agent, "--surface", surface, "--repo", dir,
		"--anchor-pid", strconv.Itoa(os.Getpid()), "--json")
	if err != nil {
		t.Fatalf("register failed: %v\nstdout:%s\nstderr:%s", err, stdout, stderr)
	}
	var out adr024RegisterOutput
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("decode register --json: %v\nstdout:%s", err, stdout)
	}
	return out
}

// Acceptance test 1: register --surface claude --json returns a watcher block
// and does NOT spawn an fs-watcher (no /tmp/sirsi-router-watch-*.pid).
func TestADR024_RegisterClaude_NoSpawn_ReturnsSpec(t *testing.T) {
	t.Parallel()
	tmp := setupTempRouter(t)
	out := registerThread(t, tmp, "test-adr024-claude", "claude")

	if out.Watcher.Type != "loop-monitor" {
		t.Errorf("watcher.type = %q, want loop-monitor", out.Watcher.Type)
	}
	if !out.Watcher.WatchesInbox {
		t.Error("claude watcher must watch the inbox")
	}
	if !strings.Contains(out.Watcher.ArmInstruction, "pgrep -f "+out.ThreadID) {
		t.Errorf("arm_instruction must key on thread_id %q; got: %s", out.ThreadID, out.Watcher.ArmInstruction)
	}
	// The defining behavior: register no longer spawns the fs-watcher.
	pidfile := "/tmp/sirsi-router-watch-" + out.ThreadID + ".pid"
	if _, err := os.Stat(pidfile); err == nil {
		os.Remove(pidfile)
		t.Errorf("register spawned an fs-watcher pidfile %s — ADR-024 forbids this", pidfile)
	}
}

// Acceptance test 2: register --surface menubar --json returns a native-runloop
// heartbeat spec, interval >=60s, and no inbox-worker requirement.
func TestADR024_RegisterMenubar_ResidentSpec(t *testing.T) {
	t.Parallel()
	tmp := setupTempRouter(t)
	out := registerThread(t, tmp, "test-adr024-menubar", "menubar")

	if out.Watcher.Type != "native-runloop" {
		t.Errorf("watcher.type = %q, want native-runloop", out.Watcher.Type)
	}
	if out.Watcher.WatchesInbox {
		t.Error("resident menubar must NOT be an inbox worker")
	}
	if !out.Watcher.Resident {
		t.Error("menubar watcher must be marked resident")
	}
	if out.Watcher.HeartbeatIntervalS < 60 {
		t.Errorf("menubar heartbeat = %d, want >=60", out.Watcher.HeartbeatIntervalS)
	}
}

// codex P1: every registered dashboard action must resolve to a real CLI shape.
// cobra errors on an unknown flag OR unknown command even with --help, so a
// `<args> --help` that exits 0 with no "unknown flag"/"unknown command" proves
// the shape is real. Special coverage for destructive entries (codex). Mirrors
// internal/dashboard actionSpecs(); guards the network/fix --fix-vs-subcommand
// regression specifically.
func TestADR_DashboardActionShapesResolveToRealCLI(t *testing.T) {
	t.Parallel()
	shapes := [][]string{
		{"network", "--fix"}, // destructive — was wrongly "network fix" (codex P1b)
		{"ra", "deploy"},     // destructive
		{"ra", "kill"},       // destructive
		{"ra", "collect"},
		{"duplicates"},
		{"thoth", "sync"},
		{"seshat", "ingest"},
		{"net", "align"},
		{"audit"}, {"maat"}, {"risk"}, {"hardware"}, {"scan"}, {"ghosts"}, {"doctor"}, {"quality"},
	}
	for _, sh := range shapes {
		t.Run(strings.Join(sh, "_"), func(t *testing.T) {
			t.Parallel()
			args := append(append([]string{}, sh...), "--help")
			stdout, stderr, err := runSirsi(t, 10*time.Second, args...)
			if err != nil {
				t.Fatalf("`sirsi %s --help` failed (shape not real): %v\n%s%s", strings.Join(sh, " "), err, stdout, stderr)
			}
			combined := stdout + stderr
			if strings.Contains(combined, "unknown flag") || strings.Contains(combined, "unknown command") {
				t.Errorf("`sirsi %s --help` reports an unreal shape:\n%s", strings.Join(sh, " "), combined)
			}
		})
	}
}

// ADR-024 §6 (discover-bridge lifecycle guard, codex item 205359 #1):
// `discover` adopts a running claude process by spawning an fs-watcher bridge.
// When that same process self-registers, register hands back the canonical
// loop-monitor spec — and MUST supersede the adoption bridge, else the bridge
// AND /loop both run (duplicate accretion). Simulated here by planting a live
// bridge pidfile, then self-registering the same (agent, pid): the bridge
// process must be signaled dead and its pidfile removed.
func TestADR024_SelfRegisterSupersedesDiscoverBridge(t *testing.T) {
	tmp := setupTempRouter(t)

	// First register creates the thread and yields its stable id.
	first := registerThread(t, tmp, "test-adr024-bridge", "claude")

	// Plant a live "bridge" the discover path would have spawned: a real
	// process whose PID lives in the per-thread watcher pidfile.
	bridge := exec.Command("sleep", "30")
	if err := bridge.Start(); err != nil {
		t.Fatalf("start bridge stand-in: %v", err)
	}
	t.Cleanup(func() { _ = bridge.Process.Kill() })
	pidfile := "/tmp/sirsi-router-watch-" + first.ThreadID + ".pid"
	if err := os.WriteFile(pidfile, []byte(strconv.Itoa(bridge.Process.Pid)), 0o644); err != nil {
		t.Fatalf("plant bridge pidfile: %v", err)
	}
	t.Cleanup(func() { os.Remove(pidfile) })

	// Self-register the same (agent, pid) — the authoritative session arriving.
	second := registerThread(t, tmp, "test-adr024-bridge", "claude")
	if first.ThreadID != second.ThreadID {
		t.Fatalf("idempotent register must reuse thread: %q != %q", first.ThreadID, second.ThreadID)
	}

	// The bridge must be superseded: pidfile gone, process signaled to exit.
	if _, err := os.Stat(pidfile); err == nil {
		t.Errorf("bridge pidfile %s survived self-register — guard did not fire", pidfile)
	}
	// Confirm the bridge actually terminated. We are its parent, so Wait()
	// reaps it; a Wait that returns is proof the SIGTERM landed (a survivor
	// would block until the 2s watchdog kills it and the test fails).
	done := make(chan error, 1)
	go func() { done <- bridge.Wait() }()
	select {
	case err := <-done:
		// SIGTERM surfaces as an *exec.ExitError; nil would mean clean exit.
		// Either way the process is gone — the guard superseded it.
		_ = err
	case <-time.After(2 * time.Second):
		_ = bridge.Process.Kill()
		t.Errorf("bridge pid %d still alive after self-register — guard did not supersede it", bridge.Process.Pid)
	}
}

// Acceptance test 3: repeated register on the same (agent_id, pid) returns the
// same thread and the same watcher spec (idempotent).
func TestADR024_RegisterIdempotent_SameThreadAndSpec(t *testing.T) {
	t.Parallel()
	tmp := setupTempRouter(t)
	first := registerThread(t, tmp, "test-adr024-idem", "claude")
	second := registerThread(t, tmp, "test-adr024-idem", "claude")

	if first.ThreadID != second.ThreadID {
		t.Errorf("idempotent register must reuse thread: %q != %q", first.ThreadID, second.ThreadID)
	}
	if first.Watcher != second.Watcher {
		t.Errorf("idempotent register must return identical watcher spec")
	}
}
