package router

import (
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/work"
)

// wakeTestRoot builds an isolated router root with the given agents registered.
func wakeTestRoot(t *testing.T, agents ...AgentConfig) string {
	t.Helper()
	root := t.TempDir()
	if err := work.EnsureRoot(root); err != nil {
		t.Fatalf("ensure root: %v", err)
	}
	reg := &Registry{Agents: map[string]AgentConfig{}}
	for _, a := range agents {
		reg.Agents[a.ID] = a
	}
	if err := SaveRegistry(root, reg); err != nil {
		t.Fatalf("save registry: %v", err)
	}
	return root
}

// sendItem creates an open inbox item addressed to `to` and returns its ID.
func sendItem(t *testing.T, root, to, title string) string {
	t.Helper()
	id, err := work.Send(root, "tester", to, title, "do the thing")
	if err != nil {
		t.Fatalf("send item: %v", err)
	}
	return id
}

// armAgent registers a fresh, non-terminal thread so the agent reads as armed.
func armAgent(t *testing.T, root, agentID string) {
	t.Helper()
	if _, err := RegisterThread(root, &Thread{
		ThreadID: "thr-" + agentID + "-test",
		AgentID:  agentID,
		Surface:  "worker",
		Status:   ThreadStatusActive,
		PID:      os.Getpid(),
	}); err != nil {
		t.Fatalf("register thread: %v", err)
	}
}

// withCountingInvoker installs a wake invoker that counts calls and records the
// last adapter, restoring the prior invoker on cleanup (Rule A21).
func withCountingInvoker(t *testing.T) (calls *int32, lastAdapter *atomic.Value) {
	t.Helper()
	calls = new(int32)
	lastAdapter = &atomic.Value{}
	old := getWakeInvoke()
	t.Cleanup(func() { setWakeInvoke(old) })
	setWakeInvoke(func(cfg AgentConfig, adapter string) error {
		atomic.AddInt32(calls, 1)
		lastAdapter.Store(adapter)
		return nil
	})
	return calls, lastAdapter
}

func wakeStatusOf(t *testing.T, root, id string) work.Item {
	t.Helper()
	it, err := work.Get(root, id)
	if err != nil {
		t.Fatalf("get item: %v", err)
	}
	return it
}

// Test 1: armed agent → no-op (no spawn), item annotated armed.
func TestWakePassArmedAgentIsNoOp(t *testing.T) {
	agent := AgentConfig{ID: "gemma-worker", Type: "gemma", Wake: WakeConfig{Mechanism: WakeAPICall, Endpoint: "http://x"}}
	root := wakeTestRoot(t, agent)
	id := sendItem(t, root, agent.ID, "armed-noop")
	armAgent(t, root, agent.ID)
	calls, _ := withCountingInvoker(t)

	rep, err := WakePass(root, time.Now().UTC())
	if err != nil {
		t.Fatalf("WakePass: %v", err)
	}
	if n := atomic.LoadInt32(calls); n != 0 {
		t.Fatalf("armed agent must not be woken; invoker called %d times", n)
	}
	if len(rep.Armed) != 1 {
		t.Fatalf("expected 1 armed outcome, got %d", len(rep.Armed))
	}
	if got := wakeStatusOf(t, root, id).WakeStatus; got != WakeStatusArmed {
		t.Fatalf("item wake_status = %q, want %q", got, WakeStatusArmed)
	}
}

// Test 2: ready adapter → invoked exactly once across repeated passes (idempotent).
func TestWakePassReadyAdapterInvokedOnce(t *testing.T) {
	echo, err := os.Executable() // any real path on PATH-or-absolute works for LookPath
	if err != nil {
		t.Fatalf("executable: %v", err)
	}
	agent := AgentConfig{ID: "qwen-worker", Type: "qwen", Cwd: t.TempDir(),
		Command: []string{echo}, Wake: WakeConfig{Mechanism: WakeCLISpawn}}
	root := wakeTestRoot(t, agent)
	id := sendItem(t, root, agent.ID, "ready-once")
	calls, lastAdapter := withCountingInvoker(t)

	now := time.Now().UTC()
	for i := 0; i < 2; i++ { // two ticks
		if _, err := WakePass(root, now); err != nil {
			t.Fatalf("WakePass pass %d: %v", i, err)
		}
	}
	if n := atomic.LoadInt32(calls); n != 1 {
		t.Fatalf("ready adapter must fire exactly once across two passes, got %d", n)
	}
	if a, _ := lastAdapter.Load().(string); a != WakeCLISpawn {
		t.Fatalf("adapter = %q, want %q", a, WakeCLISpawn)
	}
	it := wakeStatusOf(t, root, id)
	if it.WakeStatus != WakeStatusAttempted || it.WakeAttemptedAt == "" || it.WakeAdapter != WakeCLISpawn {
		t.Fatalf("item wake fields = (%q,%q,%q), want attempted/non-empty/cli-spawn", it.WakeStatus, it.WakeAttemptedAt, it.WakeAdapter)
	}
}

// Test 3: no adapter (mechanism none) → wake-unavailable, never spawned.
func TestWakePassNoAdapterIsUnavailable(t *testing.T) {
	agent := AgentConfig{ID: "muted", Type: "worker", Wake: WakeConfig{Mechanism: WakeNone}}
	root := wakeTestRoot(t, agent)
	id := sendItem(t, root, agent.ID, "no-adapter")
	calls, _ := withCountingInvoker(t)

	rep, err := WakePass(root, time.Now().UTC())
	if err != nil {
		t.Fatalf("WakePass: %v", err)
	}
	if n := atomic.LoadInt32(calls); n != 0 {
		t.Fatalf("no-adapter agent must not be spawned, got %d calls", n)
	}
	if len(rep.Unavailable) != 1 {
		t.Fatalf("expected 1 unavailable, got %d", len(rep.Unavailable))
	}
	if got := wakeStatusOf(t, root, id).WakeStatus; got != WakeStatusUnavailable {
		t.Fatalf("item wake_status = %q, want %q", got, WakeStatusUnavailable)
	}
}

// Test 4: legacy Command (no explicit wake.mechanism) → NEVER blind-spawned;
// wake-unavailable. This is the constraint-4 guard.
func TestWakePassLegacyCommandIsNotBlindSpawned(t *testing.T) {
	echo, _ := os.Executable()
	agent := AgentConfig{ID: "legacy", Type: "worker", Cwd: t.TempDir(),
		Command: []string{echo}} // Command set, Wake.Mechanism unset → no explicit intent
	root := wakeTestRoot(t, agent)
	id := sendItem(t, root, agent.ID, "legacy-cmd")
	calls, _ := withCountingInvoker(t)

	rep, err := WakePass(root, time.Now().UTC())
	if err != nil {
		t.Fatalf("WakePass: %v", err)
	}
	if n := atomic.LoadInt32(calls); n != 0 {
		t.Fatalf("legacy command agent must NOT be blind-spawned, got %d calls", n)
	}
	if len(rep.Unavailable) != 1 {
		t.Fatalf("expected 1 unavailable, got %d", len(rep.Unavailable))
	}
	it := wakeStatusOf(t, root, id)
	if it.WakeStatus != WakeStatusUnavailable || it.WakeError == "" {
		t.Fatalf("item = (%q, err=%q), want wake-unavailable with a reason", it.WakeStatus, it.WakeError)
	}
}

// Test 5: explicit cli-spawn — ready (command on PATH) is allowed; unready
// (command absent) is wake-unavailable. Plus: an interactive claude agent is
// NEVER blind-spawned even with explicit cli-spawn (constraint 3).
func TestWakePassExplicitCliSpawnReadyVsUnready(t *testing.T) {
	echo, _ := os.Executable()
	ready := AgentConfig{ID: "ready-worker", Type: "gemini", Cwd: t.TempDir(),
		Command: []string{echo}, Wake: WakeConfig{Mechanism: WakeCLISpawn}}
	unready := AgentConfig{ID: "unready-worker", Type: "gemini", Cwd: t.TempDir(),
		Command: []string{"sirsi-no-such-binary-zzz"}, Wake: WakeConfig{Mechanism: WakeCLISpawn}}
	claude := AgentConfig{ID: "claude-x", Type: "claude", Cwd: t.TempDir(),
		Command: []string{echo}, Wake: WakeConfig{Mechanism: WakeCLISpawn}}
	root := wakeTestRoot(t, ready, unready, claude)
	rID := sendItem(t, root, ready.ID, "ready")
	uID := sendItem(t, root, unready.ID, "unready")
	cID := sendItem(t, root, claude.ID, "claude")
	calls, _ := withCountingInvoker(t)

	if _, err := WakePass(root, time.Now().UTC()); err != nil {
		t.Fatalf("WakePass: %v", err)
	}
	if n := atomic.LoadInt32(calls); n != 1 {
		t.Fatalf("only the ready worker should be invoked (once), got %d", n)
	}
	if got := wakeStatusOf(t, root, rID).WakeStatus; got != WakeStatusAttempted {
		t.Fatalf("ready item wake_status = %q, want %q", got, WakeStatusAttempted)
	}
	if got := wakeStatusOf(t, root, uID).WakeStatus; got != WakeStatusUnavailable {
		t.Fatalf("unready item wake_status = %q, want %q", got, WakeStatusUnavailable)
	}
	if got := wakeStatusOf(t, root, cID).WakeStatus; got != WakeStatusUnavailable {
		t.Fatalf("interactive claude item wake_status = %q, want %q (never blind-spawned)", got, WakeStatusUnavailable)
	}
}

// Test 6: LaunchAgent install is idempotent — second install reports no change.
func TestInstallWakeLaunchAgentIdempotent(t *testing.T) {
	dir := t.TempDir()
	launchAgentsDirOverride = dir
	t.Cleanup(func() { launchAgentsDirOverride = "" })

	cfg := AgentConfig{ID: "gemma-pull", Type: "gemma"}
	changed1, path1, err := InstallWakeLaunchAgent(cfg, "/usr/local/bin/sirsi")
	if err != nil {
		t.Fatalf("install 1: %v", err)
	}
	if !changed1 {
		t.Fatalf("first install must report changed=true")
	}
	if _, statErr := os.Stat(path1); statErr != nil {
		t.Fatalf("plist not written: %v", statErr)
	}
	first, _ := os.ReadFile(path1)

	changed2, path2, err := InstallWakeLaunchAgent(cfg, "/usr/local/bin/sirsi")
	if err != nil {
		t.Fatalf("install 2: %v", err)
	}
	if changed2 {
		t.Fatalf("second identical install must report changed=false (idempotent)")
	}
	if path2 != path1 {
		t.Fatalf("install path changed: %q vs %q", path2, path1)
	}
	second, _ := os.ReadFile(path2)
	if string(first) != string(second) {
		t.Fatalf("idempotent install produced differing plist content")
	}
	if want := filepath.Join(dir, WakeLaunchAgentLabel(cfg.ID)+".plist"); path1 != want {
		t.Fatalf("plist path = %q, want %q", path1, want)
	}
}
