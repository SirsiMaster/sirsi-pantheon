package router

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/routercfg"
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

// One wake per AGENT per pass, not one per item. The adapter nudges an agent and
// its pull-loop then drains the whole inbox, so N stranded items for one agent
// must still cost exactly one adapter call — with a blocking launchctl, invoking
// per item turned 16 items into 16 sequential hangs (18m doctor wedge,
// 2026-07-29). Every item still gets annotated.
func TestWakePassInvokesOncePerAgentNotPerItem(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatalf("executable: %v", err)
	}
	a := AgentConfig{ID: "busy-worker", Type: "qwen", Cwd: t.TempDir(),
		Command: []string{exe}, Wake: WakeConfig{Mechanism: WakeCLISpawn}}
	b := AgentConfig{ID: "other-worker", Type: "qwen", Cwd: t.TempDir(),
		Command: []string{exe}, Wake: WakeConfig{Mechanism: WakeCLISpawn}}
	root := wakeTestRoot(t, a, b)

	var ids []string
	for i := 0; i < 5; i++ {
		ids = append(ids, sendItem(t, root, a.ID, fmt.Sprintf("stranded-%d", i)))
	}
	ids = append(ids, sendItem(t, root, b.ID, "other-agent"))

	// Count per agent so the assertion discovers who was woken rather than
	// assuming the loop's ordering.
	var mu sync.Mutex
	perAgent := map[string]int{}
	old := getWakeInvoke()
	t.Cleanup(func() { setWakeInvoke(old) })
	setWakeInvoke(func(cfg AgentConfig, adapter string) error {
		mu.Lock()
		defer mu.Unlock()
		perAgent[cfg.ID]++
		return nil
	})

	if _, err := WakePass(root, time.Now().UTC()); err != nil {
		t.Fatalf("WakePass: %v", err)
	}

	for agentID, n := range perAgent {
		if n != 1 {
			t.Fatalf("agent %q woken %d times in one pass, want exactly 1", agentID, n)
		}
	}
	if len(perAgent) != 2 {
		t.Fatalf("agents woken = %v, want both %q and %q exactly once", perAgent, a.ID, b.ID)
	}
	// Sharing the side effect must not cost any item its annotation.
	for _, id := range ids {
		if it := wakeStatusOf(t, root, id); it.WakeStatus != WakeStatusAttempted {
			t.Fatalf("item %s wake_status = %q, want %q", id, it.WakeStatus, WakeStatusAttempted)
		}
	}
}

// A failed wake must still mark EVERY one of that agent's items, not just the
// first — the dedup shares the error, it does not swallow it for the rest.
func TestWakePassSharedFailureAnnotatesEveryItem(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatalf("executable: %v", err)
	}
	a := AgentConfig{ID: "wedged-worker", Type: "qwen", Cwd: t.TempDir(),
		Command: []string{exe}, Wake: WakeConfig{Mechanism: WakeCLISpawn}}
	root := wakeTestRoot(t, a)
	var ids []string
	for i := 0; i < 3; i++ {
		ids = append(ids, sendItem(t, root, a.ID, fmt.Sprintf("doomed-%d", i)))
	}
	old := getWakeInvoke()
	t.Cleanup(func() { setWakeInvoke(old) })
	setWakeInvoke(func(cfg AgentConfig, adapter string) error {
		return fmt.Errorf("no response in 15s (label remained at 'spawn scheduled')")
	})

	if _, err := WakePass(root, time.Now().UTC()); err != nil {
		t.Fatalf("WakePass: %v", err)
	}
	for _, id := range ids {
		it := wakeStatusOf(t, root, id)
		if it.WakeStatus != WakeStatusUnavailable {
			t.Fatalf("item %s wake_status = %q, want %q", id, it.WakeStatus, WakeStatusUnavailable)
		}
	}
}

// The deadline must actually fire and kill a child that would otherwise block
// forever — the whole point of the fix. Drives a real blocking process against a
// shortened bound: if runBounded reverted to unbounded exec.Command this hangs
// for 30s and fails, which a bogus-subcommand launchctl call could never catch.
func TestRunBoundedKillsBlockedChild(t *testing.T) {
	if _, err := exec.LookPath("sleep"); err != nil {
		t.Skip("sleep not on PATH")
	}
	old := launchctlTimeout
	launchctlTimeout = 200 * time.Millisecond
	t.Cleanup(func() { launchctlTimeout = old })

	start := time.Now()
	err := runBounded("sleep", "30")
	elapsed := time.Since(start)

	// Run returned, so CommandContext killed and reaped the child; an unbounded
	// exec would still be inside wait4 here.
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("runBounded err = %v, want context.DeadlineExceeded", err)
	}
	if elapsed > 5*time.Second {
		t.Fatalf("runBounded took %s against a %s bound — child was not killed on expiry",
			elapsed, launchctlTimeout)
	}
	// The generic helper also carries print/bootout/checker probes, so its timeout
	// must not assert a launchd state it never observed — neither the old "parked"
	// misdiagnosis nor a bare "spawn scheduled" claim. Callers that know the
	// operation add that context themselves.
	for _, banned := range []string{"parked", "spawn scheduled"} {
		if strings.Contains(err.Error(), banned) {
			t.Fatalf("generic timeout text claims unobserved launchd state %q: %v", banned, err)
		}
	}
}

// The happy path must not pay the deadline: a launchctl call that exits on its
// own returns immediately.
func TestRunLaunchctlFastPathNotSlowed(t *testing.T) {
	if _, err := exec.LookPath("launchctl"); err != nil {
		t.Skip("launchctl not on PATH")
	}
	start := time.Now()
	_ = runLaunchctl("sirsi-no-such-subcommand")
	if elapsed := time.Since(start); elapsed >= launchctlTimeout {
		t.Fatalf("runLaunchctl took %s, must be bounded by %s", elapsed, launchctlTimeout)
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

// codex SME #89, finding 1+4: the rendered plist must invoke the REAL `wake-loop`
// verb as direct argv (no /bin/sh, no non-existent `--agent` flag) and XML-escape
// the bin path + agent id so spaces and shell/XML metacharacters can't break it.
func TestWakeLaunchAgentPlistUsesRealArgv(t *testing.T) {
	cfg := AgentConfig{ID: `ag&nt <x>`, Type: "gemma"}
	plist := wakeLaunchAgentPlist(WakeLaunchAgentLabel(cfg.ID), cfg, "/opt/sirsi bin/sirsi")

	if strings.Contains(plist, "/bin/sh") {
		t.Error("plist must not shell out (/bin/sh) — direct argv only (finding 4)")
	}
	if strings.Contains(plist, "--agent") {
		t.Error("plist must not use the non-existent `thread heartbeat --agent` flag (finding 1)")
	}
	if !strings.Contains(plist, "<string>wake-loop</string>") {
		t.Error("plist must invoke the real `router wake-loop` verb")
	}
	if !strings.Contains(plist, "<string>/opt/sirsi bin/sirsi</string>") {
		t.Error("a bin path with a space must remain ONE argv element")
	}
	if !strings.Contains(plist, "&amp;") || !strings.Contains(plist, "&lt;x&gt;") {
		t.Error("agent id metacharacters must be XML-escaped in the plist (finding 4)")
	}
}

// codex SME #89, finding 1: the LaunchAgent loop must maintain a CONCRETE pull-loop
// thread with a real heartbeat. RunWakeLoop registers a worker thread, heartbeats,
// and closes it on ctx cancel.
func TestRunWakeLoopRegistersAndCloses(t *testing.T) {
	root := wakeTestRoot(t, AgentConfig{ID: "gemma-pull", Type: "gemma"})
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // pre-cancel: the loop registers + heartbeats once, then exits
	if err := RunWakeLoop(ctx, root, "gemma-pull", time.Hour); err != nil {
		t.Fatalf("RunWakeLoop: %v", err)
	}
	reg, err := LoadThreadRegistry(root)
	if err != nil {
		t.Fatalf("load threads: %v", err)
	}
	var found *Thread
	for _, thr := range reg.Threads {
		if thr.AgentID == "gemma-pull" && thr.Surface == surfaceWorker {
			found = thr
			break
		}
	}
	if found == nil {
		t.Fatal("wake-loop did not register a worker pull-loop thread")
	}
	if !found.Status.IsTerminal() {
		t.Fatalf("wake-loop thread must be closed on exit; status=%q", found.Status)
	}
	if found.WakeMechanism != WakeLaunchAgent {
		t.Fatalf("wake mechanism=%q, want %q", found.WakeMechanism, WakeLaunchAgent)
	}
}

// codex SME #89, finding 2: the default cli-spawn invoker must Start()+Release()
// (detached) — return promptly without hanging or leaving a zombie.
func TestDefaultWakeInvokeCliSpawnReleases(t *testing.T) {
	truePath, err := exec.LookPath("true")
	if err != nil {
		t.Skip("`true` not on PATH")
	}
	cfg := AgentConfig{ID: "w", Type: "gemma", Cwd: t.TempDir(), Command: []string{truePath}}
	done := make(chan error, 1)
	go func() { done <- defaultWakeInvoke(cfg, WakeCLISpawn) }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("cli-spawn invoke: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("defaultWakeInvoke(cli-spawn) hung — Start()+Release() must return immediately")
	}
}

// codex SME #89, finding 3: mcp-notification readiness must be honest — it is not
// wired, so ProbeWakeReadiness must report NOT ready (report mode and the acted-on
// pass must agree).
func TestProbeWakeReadinessMCPNotReady(t *testing.T) {
	cfg := AgentConfig{ID: "m", Type: "worker", Wake: WakeConfig{Mechanism: WakeMCPNotification, MCPServer: "srv"}}
	if h := ProbeWakeReadiness(cfg); h.Ready {
		t.Fatalf("mcp-notification must report NOT ready until an invoker is wired; got ready (%q)", h.Detail)
	}
}

// Test 7: uninstall removes an installed wake LaunchAgent and is a clean no-op
// when nothing is installed (A27: install AND clean uninstall). Injected writer
// dir (Rule A16) so no real ~/Library/LaunchAgents or launchd domain is touched —
// the override also suppresses the launchctl bootout side effect.
func TestUninstallWakeLaunchAgent(t *testing.T) {
	dir := t.TempDir()
	launchAgentsDirOverride = dir
	t.Cleanup(func() { launchAgentsDirOverride = "" })

	cfg := AgentConfig{ID: "gemma-pull", Type: "gemma"}

	// Uninstall with nothing installed → clean no-op (removed=false, no error).
	removed0, path0, err := UninstallWakeLaunchAgent(cfg)
	if err != nil {
		t.Fatalf("uninstall (absent): %v", err)
	}
	if removed0 {
		t.Fatalf("uninstall with nothing installed must report removed=false")
	}
	if WakeLaunchAgentInstalled(cfg.ID) {
		t.Fatalf("nothing should be installed before the first install")
	}
	want := filepath.Join(dir, WakeLaunchAgentLabel(cfg.ID)+".plist")
	if path0 != want {
		t.Fatalf("uninstall path = %q, want %q", path0, want)
	}

	// Install, then uninstall → removed=true and the plist is gone.
	if _, _, ierr := InstallWakeLaunchAgent(cfg, "/usr/local/bin/sirsi"); ierr != nil {
		t.Fatalf("install: %v", ierr)
	}
	if !WakeLaunchAgentInstalled(cfg.ID) {
		t.Fatalf("plist must exist after install")
	}
	removed1, _, err := UninstallWakeLaunchAgent(cfg)
	if err != nil {
		t.Fatalf("uninstall (present): %v", err)
	}
	if !removed1 {
		t.Fatalf("uninstall of an installed agent must report removed=true")
	}
	if WakeLaunchAgentInstalled(cfg.ID) {
		t.Fatalf("plist must be gone after uninstall")
	}

	// Second uninstall is idempotent (no error, removed=false).
	removed2, _, err := UninstallWakeLaunchAgent(cfg)
	if err != nil {
		t.Fatalf("uninstall (repeat): %v", err)
	}
	if removed2 {
		t.Fatalf("repeat uninstall must be a no-op (removed=false)")
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

// TestCutoverReadersFailClosed pins codex-pantheon's #315 review blocker: under
// the cutover the store is the sole authority, so a store-open failure must
// SURFACE, never degrade to the frozen legacy files. Failing open there
// resurrects closed work through pull/wake/plan/board — the exact P0 #315
// fixed — precisely when the store is unavailable.
func TestCutoverReadersFailClosed(t *testing.T) {
	root := t.TempDir()
	// Seed a legacy file that says OPEN. If either reader falls back, it finds this.
	itemsDir := filepath.Join(root, "items")
	if err := os.MkdirAll(itemsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	const id = "20260101-000000-a-claude-pantheon-legacy-open"
	body := "---\nfrom: \"a\"\nto: \"claude-pantheon\"\ntitle: \"legacy open\"\nstatus: open\nopened: 2026-01-01T00:00:00Z\n---\n\n## Instructions\n\nx\n"
	if err := os.WriteFile(filepath.Join(itemsDir, id+".md"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	t.Setenv(routercfg.StoreWakeEnv, "1")
	// An unopenable store: a path whose parent exists as a FILE, so MkdirAll fails.
	blocker := filepath.Join(t.TempDir(), "not-a-dir")
	if err := os.WriteFile(blocker, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SIRSI_ROUTER_DB", filepath.Join(blocker, "router.db"))

	if items, err := OpenItems(root, "claude-pantheon"); err == nil {
		t.Errorf("OpenItems returned %d item(s) and no error — it fell back to the frozen legacy files", len(items))
	}
	if items, err := AllItems(root); err == nil {
		t.Errorf("AllItems returned %d item(s) and no error — it fell back to the frozen legacy files", len(items))
	}

	// Pre-cutover the SAME store failure must still degrade to files — that is
	// the correct behavior there, and this change must not alter it (Rule 14).
	t.Setenv(routercfg.StoreWakeEnv, "0")
	items, err := OpenItems(root, "claude-pantheon")
	if err != nil {
		t.Fatalf("pre-cutover OpenItems should read files, got %v", err)
	}
	if len(items) != 1 || items[0].ID != id {
		t.Errorf("pre-cutover OpenItems = %+v, want the one legacy file item", items)
	}
}

// ── TestLaunchctlWakeJobHasPID ───────────────────────────────────────────────
//
// These tests verify that LaunchctlWakeJobHasPID asserts on a live PID, not on
// load state. The critical case is a fixture whose `launchctl list` exits 0 (the
// label is loaded) but contains no "PID" line — this MUST still return false, so
// a crashed/exited wake job does not clear the loop-dead verdict (PR #415 review).

func TestLaunchctlWakeJobHasPID_RunningJobReturnsTrue(t *testing.T) {
	old := getLaunchctlListOutput()
	setLaunchctlListOutput(func(label string) ([]byte, error) {
		// Simulate `launchctl list ai.sirsi.router.wake.claude-io` for a running job.
		return []byte(`{
	"Label" = "ai.sirsi.router.wake.claude-io";
	"OnDemand" = false;
	"LastExitStatus" = 0;
	"PID" = 5763;
};`), nil
	})
	t.Cleanup(func() { setLaunchctlListOutput(old) })

	if !LaunchctlWakeJobHasPID("ai.sirsi.router.wake.claude-io") {
		t.Error("LaunchctlWakeJobHasPID = false for a job with a live PID, want true")
	}
}

// TestLaunchctlWakeJobHasPID_LoadedButNoPID is the regression guard for the PR
// #415 defect: a loaded-but-not-running job (PID absent, LastExitStatus nonzero)
// must NOT clear the loop-dead verdict. The old implementation used exit status
// only, so it returned "loaded == armed" — this test fails on that shape.
func TestLaunchctlWakeJobHasPID_LoadedButNoPID(t *testing.T) {
	old := getLaunchctlListOutput()
	setLaunchctlListOutput(func(label string) ([]byte, error) {
		// Simulate a crashed wake job: label is registered (exit 0), no PID.
		// Real example: ai.sirsi.hypergraph.digest — LastExitStatus=256, no PID.
		return []byte(`{
	"Label" = "ai.sirsi.router.wake.claude-nexus";
	"OnDemand" = false;
	"LastExitStatus" = 256;
};`), nil
	})
	t.Cleanup(func() { setLaunchctlListOutput(old) })

	if LaunchctlWakeJobHasPID("ai.sirsi.router.wake.claude-nexus") {
		t.Error("LaunchctlWakeJobHasPID = true for a loaded-but-not-running job (no PID field), want false — the old exit-status-only check had this bug")
	}
}

func TestLaunchctlWakeJobHasPID_SIGTERM_LoopReturnsTrue(t *testing.T) {
	old := getLaunchctlListOutput()
	setLaunchctlListOutput(func(label string) ([]byte, error) {
		// SIGTERM crash-loop: LastExitStatus=-15 but PID is still present (launchd
		// is actively respawning it). Credit as armed — it has a live consumer.
		return []byte(`{
	"Label" = "ai.sirsi.claude-worker.claude-pantheon";
	"OnDemand" = false;
	"LastExitStatus" = -15;
	"PID" = 9812;
};`), nil
	})
	t.Cleanup(func() { setLaunchctlListOutput(old) })

	if !LaunchctlWakeJobHasPID("ai.sirsi.claude-worker.claude-pantheon") {
		t.Error("LaunchctlWakeJobHasPID = false for a SIGTERM-looping job that still has a PID, want true")
	}
}

func TestLaunchctlWakeJobHasPID_NotLoaded(t *testing.T) {
	old := getLaunchctlListOutput()
	setLaunchctlListOutput(func(label string) ([]byte, error) {
		// Label not known to launchd — launchctl list exits nonzero.
		return nil, fmt.Errorf("launchctl list: exit status 113")
	})
	t.Cleanup(func() { setLaunchctlListOutput(old) })

	if LaunchctlWakeJobHasPID("ai.sirsi.router.wake.ghost-agent") {
		t.Error("LaunchctlWakeJobHasPID = true for an unknown label, want false")
	}
}
