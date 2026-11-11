package main_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
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
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, testBinary,
		"thread", "register",
		"--agent", agent, "--surface", surface, "--repo", dir,
		"--anchor-pid", strconv.Itoa(os.Getpid()), "--json")
	cmd.Dir = dir
	cmd.Env = adr024WatcherEnv(t, dir)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	if err != nil {
		t.Fatalf("register failed: %v\nstdout:%s\nstderr:%s", err, stdout.String(), stderr.String())
	}
	var out adr024RegisterOutput
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		t.Fatalf("decode register --json: %v\nstdout:%s", err, stdout.String())
	}
	return out
}

// Per-fixture HOME, not a process-global override or a shared /tmp pidfile.
func adr024WatcherEnv(t *testing.T, dir string) []string {
	t.Helper()
	home := filepath.Join(dir, "watcher-home")
	if err := os.MkdirAll(home, 0700); err != nil {
		t.Fatal(err)
	}
	return sirsiTestEnv(dir, filepath.Join(dir, "router-test.db"),
		"HOME="+home, "XDG_CONFIG_HOME="+filepath.Join(home, ".config"),
		"XDG_CACHE_HOME="+filepath.Join(home, ".cache"), "SIRSI_ROUTER_STORE_WAKE=0")
}

func adr024WatcherPidfile(dir, threadID string) string {
	key := sha256.Sum256([]byte(threadID))
	return filepath.Join(dir, "watcher-home", ".sirsi", "router-watchers", hex.EncodeToString(key[:])+".pid")
}

// Acceptance test 1: register --surface claude --json returns a watcher block
// and does NOT spawn an fs-watcher or create a private watcher publication.
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
	pidfile := adr024WatcherPidfile(tmp, out.ThreadID)
	if _, err := os.Lstat(pidfile); err == nil {
		t.Errorf("register spawned an fs-watcher pidfile %s — ADR-024 forbids this", pidfile)
	} else if !os.IsNotExist(err) {
		t.Fatal(err)
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
// A surface-armed watch-router must cooperatively yield to self-registration.
// Use the actual command we own, never a numeric PID pointing at sleep or a
// foreign process. Stale publications are retained, not mistaken for liveness.
func TestADR024_SelfRegisterSupersedesDiscoverBridge(t *testing.T) {
	tmp := setupTempRouter(t)

	// First register creates the thread and yields its stable id.
	first := registerThread(t, tmp, "test-adr024-bridge", "claude")

	// Context cleanup owns this exact child command; it is not the product's
	// persisted-PID retirement path. No agent config/items exist to dispatch.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	bridge := exec.CommandContext(ctx, testBinary, "thread", "watch-router",
		"--thread", first.ThreadID, "--agent", "test-adr024-bridge",
		"--router-root", filepath.Join(tmp, ".agents", "idea-router"),
		"--parent-pid", strconv.Itoa(os.Getpid()))
	bridge.Dir = tmp
	bridge.Env = adr024WatcherEnv(t, tmp)
	var stderr bytes.Buffer
	bridge.Stderr = &stderr
	if err := bridge.Start(); err != nil {
		t.Fatalf("start owned watcher: %v", err)
	}
	done := make(chan error, 1)
	go func() { done <- bridge.Wait() }()
	waited := false
	t.Cleanup(func() {
		cancel()
		if !waited {
			<-done // reap only the child command this fixture actually started
		}
	})
	pidfile := adr024WatcherPidfile(tmp, first.ThreadID)
	ready := false
	for deadline := time.Now().Add(3 * time.Second); time.Now().Before(deadline); {
		if data, err := os.ReadFile(pidfile); err == nil {
			var record struct {
				Version     int    `json:"version"`
				Kind        string `json:"kind"`
				ThreadID    string `json:"thread_id"`
				AgentID     string `json:"agent_id"`
				RouterRoot  string `json:"router_root"`
				PID         int    `json:"pid"`
				Incarnation string `json:"incarnation"`
			}
			if json.Unmarshal(data, &record) == nil && record.Version == 1 && record.Kind == "sirsi-thread-watch-router" &&
				record.ThreadID == first.ThreadID && record.AgentID == "test-adr024-bridge" &&
				record.RouterRoot == filepath.Join(tmp, ".agents", "idea-router") && record.PID == bridge.Process.Pid && len(record.Incarnation) == 64 {
				ready = true
				break
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !ready {
		t.Fatal("owned watch-router did not publish private custody")
	}

	// Self-register the same (agent, pid) — the authoritative session arriving.
	second := registerThread(t, tmp, "test-adr024-bridge", "claude")
	if first.ThreadID != second.ThreadID {
		t.Fatalf("idempotent register must reuse thread: %q != %q", first.ThreadID, second.ThreadID)
	}

	// Request persistence alone is not confirmation. Only this owned child's
	// successful Wait proves watcher exit; it does not prove consumer quiet.
	select {
	case err := <-done:
		waited = true
		if err != nil {
			t.Fatalf("owned watcher did not retire cooperatively: %v\n%s", err, stderr.String())
		}
	case <-time.After(2 * time.Second):
		t.Fatal("cooperative request did not confirm owned watcher exit")
	}
	publication, err := os.ReadFile(pidfile)
	if err != nil {
		t.Fatal(err)
	}
	stop, err := os.ReadFile(pidfile + ".stop")
	if err != nil || !bytes.Equal(publication, stop) {
		t.Fatalf("exact incarnation request missing: %v", err)
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
