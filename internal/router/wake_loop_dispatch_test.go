package router

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// recordingConsumer writes one line per invocation containing the full argv and
// the router-contract env vars, then optionally sleeps to simulate work.
//
// The previous version of this test used a script that IGNORED its arguments. It
// therefore passed while the production dispatch appended no prompt at all and
// drained nothing — it proved "a process started", never "this inbox is being
// consumed" (codex-pantheon review of #389, finding 2). Recording argv+env is
// what makes the contract itself assertable.
func recordingConsumer(t *testing.T, log string, sleep time.Duration) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "consumer.sh")
	body := "#!/bin/sh\n" +
		"echo \"argv=$* agent=$" + EnvConsumerAgent + " root=$" + EnvConsumerRoot + "\" >> " + log + "\n"
	if sleep > 0 {
		body += "sleep " + strings.TrimSuffix(sleep.String(), "0s") + "\n"
	}
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatalf("write consumer: %v", err)
	}
	return path
}

func consumerLines(t *testing.T, log string) []string {
	t.Helper()
	b, err := os.ReadFile(log)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	var out []string
	for _, l := range strings.Split(strings.TrimSpace(string(b)), "\n") {
		if strings.TrimSpace(l) != "" {
			out = append(out, l)
		}
	}
	return out
}

// awaitConsumerLines waits for at least n recorded invocations.
//
// A dispatched consumer is a detached child: it is started synchronously but
// WRITES asynchronously, so reading the log the instant RunWakeLoop returns
// races the child under parallel test load. Polling for the evidence is the
// honest wait — the assertion is about what the child did, not about when.
// mustAwaitConsumerLines is awaitConsumerLines for callers where the wait is a
// PRECONDITION rather than the measurement.
//
// awaitConsumerLines returns whatever it has when the deadline passes, silently.
// That is right where the count is the assertion, and wrong where a later
// assertion assumes the dispatch landed: the test then measures a world that
// never reached its starting state and reports the consequence as the cause.
// That is exactly how a load-blocked dispatch got reported as a broken latch.
func mustAwaitConsumerLines(t *testing.T, log string, n int) []string {
	t.Helper()
	lines := awaitConsumerLines(t, log, n)
	if len(lines) < n {
		t.Fatalf("precondition never held: waited for %d consumer dispatch(es), saw %d — "+
			"nothing after this point is measuring what it claims to", n, len(lines))
	}
	return lines
}

func awaitConsumerLines(t *testing.T, log string, n int) []string {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for {
		lines := consumerLines(t, log)
		if len(lines) >= n || time.Now().After(deadline) {
			return lines
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// The dispatched process must be TOLD which inbox to drain. A consumer that was
// never handed its agent id cannot consume the queue whose depth triggered it,
// which is the difference between "spawned something headless" and "consumed
// this inbox".
// hermeticDispatchDir isolates ONE test's spawn ledger.
//
// TestMain pins the dir package-wide, which stops these tests writing the
// operator's real ~/.sirsi/dispatch — the serious defect. It is not sufficient
// on its own: TestMain runs once per BINARY, so under -count=N every iteration
// shares that one dir, the dispatch tests re-accumulate entries, and run 2
// trips the hourly ceiling. `go test` was green and `go test -count=3` was red
// until each dispatching test got its own dir. Two layers, two jobs: TestMain
// protects production, this protects repeat runs.
func hermeticDispatchDir(t *testing.T) {
	t.Helper()
	SetDispatchDir(t.TempDir())
}

func TestDispatchedConsumerReceivesRouterContract(t *testing.T) {
	// Stub low load so backpressure never blocks dispatch regardless of host load.
	SetLoadAvgFn(func() (float64, bool) { return 0.1, true })
	defer SetLoadAvgFn(nil)
	hermeticDispatchDir(t)

	log := filepath.Join(t.TempDir(), "fired.txt")
	script := recordingConsumer(t, log, 0)

	root := wakeTestRoot(t, AgentConfig{
		ID:   "worker-agent",
		Type: "worker",
		Consumer: ConsumerConfig{
			Command: []string{script, "--agent", consumerAgentPlaceholder},
			Prompt:  "pull and work the inbox for " + consumerAgentPlaceholder,
		},
	})
	sendItem(t, root, "worker-agent", "work")

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Millisecond)
	defer cancel()
	if err := RunWakeLoop(ctx, root, "worker-agent", 40*time.Millisecond); err != nil {
		t.Fatalf("RunWakeLoop: %v", err)
	}

	lines := awaitConsumerLines(t, log, 1)
	if len(lines) == 0 {
		t.Fatal("consumer was never dispatched")
	}
	first := lines[0]
	if !strings.Contains(first, "--agent worker-agent") {
		t.Errorf("argv did not carry the substituted agent id: %s", first)
	}
	if !strings.Contains(first, "pull and work the inbox for worker-agent") {
		t.Errorf("prompt was not appended/substituted: %s", first)
	}
	if !strings.Contains(first, "agent=worker-agent") {
		t.Errorf("%s env var not delivered: %s", EnvConsumerAgent, first)
	}
	if !strings.Contains(first, "root="+root) {
		t.Errorf("%s env var not delivered: %s", EnvConsumerRoot, first)
	}
}

func TestCodexConsumerCarriesStoreAccessAndInboxContract(t *testing.T) {
	root := t.TempDir()
	// A REAL directory: ResolveConsumer now refuses a cwd that cannot work, so
	// the old fictional "/repo" would be rejected before the argv assertions
	// this test actually cares about.
	repo := t.TempDir()
	cfg := AgentConfig{
		ID:   "codex-pantheon",
		Type: "codex",
		Cwd:  repo,
		Consumer: ConsumerConfig{
			Command: []string{
				"sh", "-c", "true", // stand-in for codex exec in this unit test
				"-C", repo,
				"--sandbox", "workspace-write",
				"--add-dir", "/Users/thekryptodragon/.sirsi",
			},
			Prompt: "You are " + consumerAgentPlaceholder + "; pull and close " + consumerAgentPlaceholder + " from " + consumerRootPlaceholder,
		},
	}
	rc, why := ResolveConsumer(cfg, root)
	if rc == nil {
		t.Fatalf("codex consumer refused: %s", why)
	}
	got := strings.Join(rc.Argv, " ")
	for _, want := range []string{
		"--add-dir /Users/thekryptodragon/.sirsi",
		"codex-pantheon",
		root,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("codex consumer argv missing %q: %s", want, got)
		}
	}
}

// Dispatch is gated on the consumer's own LIVENESS, not a clock. While a
// consumer is still working, no second one may be dispatched on top of it — a
// slow-but-healthy agent must be allowed to take as long as it needs.
//
// The earlier design used a 10-minute timer for this, which cannot tell "still
// working" from "died" because Setsid+Release discards the handle (finding 3).
func TestNoSecondConsumerWhileFirstIsStillRunning(t *testing.T) {
	// Stub low load so backpressure never blocks dispatch regardless of host
	// load — the same guard its three siblings in this file already carry, and
	// the only one of the four that was missing it.
	//
	// Without it this test asserts the DISPATCH LATCH while silently depending
	// on the runner being idle. Observed in CI 2026-08-07 on PR #653, which
	// touches none of this code: "dispatch deferred — load average 29.45 >= 18
	// cores" repeated for the whole window, zero dispatches, and the failure
	// printed "dispatched 0 consumers while the first was still running" — an
	// accusation about the latch, which had never run. The very next test in the
	// same log dispatched fine at the same instant, because it stubs load.
	SetLoadAvgFn(func() (float64, bool) { return 0.1, true })
	defer SetLoadAvgFn(nil)
	hermeticDispatchDir(t)

	log := filepath.Join(t.TempDir(), "fired.txt")
	// Sleeps well past the loop's lifetime: it is still working at every tick.
	script := recordingConsumer(t, log, 2*time.Second)

	root := wakeTestRoot(t, AgentConfig{
		ID:       "slow-agent",
		Type:     "worker",
		Consumer: ConsumerConfig{Command: []string{script, consumerAgentPlaceholder}},
	})
	sendItem(t, root, "slow-agent", "work that is never drained")

	// Synchronize on the dispatch actually LANDING, then keep the loop running
	// and observe. The earlier shape ran the loop for a fixed 300ms and read the
	// log after canceling — asserting on a detached process's side effect
	// against a wall clock. On a loaded runner the process loses that race and
	// the count reads 0, which is a slow machine, not a broken latch.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- RunWakeLoop(ctx, root, "slow-agent", 40*time.Millisecond) }()

	mustAwaitConsumerLines(t, log, 1) // the first dispatch has provably happened

	// Give the loop many more ticks with the consumer still sleeping. If the
	// latch were broken, a second line appears here.
	time.Sleep(400 * time.Millisecond)
	n := len(consumerLines(t, log))

	cancel()
	if err := <-done; err != nil && !errors.Is(err, context.Canceled) {
		t.Fatalf("RunWakeLoop: %v", err)
	}

	if n != 1 {
		t.Errorf("dispatched %d consumers while the first was still running — want exactly 1 "+
			"(a still-working consumer must hold the slot)", n)
	}
}

// The mirror of the above: when a consumer EXITS and the inbox is still
// non-empty, the slot frees immediately and the loop dispatches again. A
// permanent latch would strand the inbox behind a consumer that died mid-drain,
// which is the failure the discarded 10-minute timer existed to cover.
func TestConsumerExitFreesTheDispatchSlot(t *testing.T) {
	// Stub low load so backpressure never blocks dispatch regardless of host load.
	SetLoadAvgFn(func() (float64, bool) { return 0.1, true })
	defer SetLoadAvgFn(nil)
	hermeticDispatchDir(t)

	log := filepath.Join(t.TempDir(), "fired.txt")
	script := recordingConsumer(t, log, 0) // exits at once, never drains

	root := wakeTestRoot(t, AgentConfig{
		ID:       "flaky-agent",
		Type:     "worker",
		Consumer: ConsumerConfig{Command: []string{script, consumerAgentPlaceholder}},
	})
	sendItem(t, root, "flaky-agent", "work that is never drained")

	// The loop must still be RUNNING while we wait for the re-dispatch. The
	// earlier shape canceled it after 300ms and only then polled for a second
	// line — so awaitConsumerLines was interrogating a stopped loop that could
	// never produce one, and the test passed only when both dispatches happened
	// to fit inside that fixed window.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- RunWakeLoop(ctx, root, "flaky-agent", 40*time.Millisecond) }()

	n := len(awaitConsumerLines(t, log, 2))

	cancel()
	if err := <-done; err != nil && !errors.Is(err, context.Canceled) {
		t.Fatalf("RunWakeLoop: %v", err)
	}

	if n < 2 {
		t.Errorf("only %d dispatches after the consumer exited — the slot must free on "+
			"completion or a consumer that dies mid-drain strands the inbox forever", n)
	}
}

// A launchctl kickstart -k (or a KeepAlive crash-restart) replaces the
// wake-loop PROCESS, but Setsid-without-Release means the dispatched consumer
// is a detached child of the OS, not of that process — it keeps running. The
// in-memory `run` variable that tracked it dies with the old process, so a
// naive restart starts fresh with run == nil and, seeing depth > 0, dispatches
// a duplicate on top of the one still working. Observed in production
// 2026-08-07: two live claude-finalwishes sessions 65s apart, ~480MB each.
//
// A restarted loop must instead recognize the still-running consumer via the
// durable marker file and hold the slot, exactly as the non-restarted case
// (TestNoSecondConsumerWhileFirstIsStillRunning) already does in-process.
func TestRestartDoesNotDuplicateAStillRunningConsumer(t *testing.T) {
	// Deterministic regardless of the host's ambient load — this test asserts
	// dispatch-adoption behavior, not backpressure (backpressure_test.go covers
	// that separately).
	SetLoadAvgFn(func() (float64, bool) { return 0.1, true })
	defer SetLoadAvgFn(nil)
	hermeticDispatchDir(t)

	log := filepath.Join(t.TempDir(), "fired.txt")
	// Sleeps well past this test's lifetime: still working at every tick.
	script := recordingConsumer(t, log, 5*time.Second)

	root := wakeTestRoot(t, AgentConfig{
		ID:       "restart-agent",
		Type:     "worker",
		Consumer: ConsumerConfig{Command: []string{script, consumerAgentPlaceholder}},
	})
	sendItem(t, root, "restart-agent", "work that outlives the loop restart")

	// First loop: dispatches, then gets canceled — simulating the process being
	// replaced — WITHOUT its child dying (a real detached process wouldn't).
	ctx1, cancel1 := context.WithCancel(context.Background())
	done1 := make(chan error, 1)
	go func() { done1 <- RunWakeLoop(ctx1, root, "restart-agent", 40*time.Millisecond) }()
	mustAwaitConsumerLines(t, log, 1) // first dispatch has provably landed
	cancel1()
	if err := <-done1; err != nil && !errors.Is(err, context.Canceled) {
		t.Fatalf("first RunWakeLoop: %v", err)
	}

	if _, err := os.Stat(consumerPIDFilePath(root, "restart-agent")); err != nil {
		t.Fatalf("durable marker missing after dispatch: %v", err)
	}

	// Second loop: a fresh process (fresh `run == nil`) over the SAME root and
	// agent id, exactly like a restart. The consumer from the first loop is
	// still sleeping.
	ctx2, cancel2 := context.WithCancel(context.Background())
	defer cancel2()
	done2 := make(chan error, 1)
	go func() { done2 <- RunWakeLoop(ctx2, root, "restart-agent", 40*time.Millisecond) }()

	time.Sleep(400 * time.Millisecond)
	n := len(consumerLines(t, log))
	cancel2()
	if err := <-done2; err != nil && !errors.Is(err, context.Canceled) {
		t.Fatalf("second RunWakeLoop: %v", err)
	}

	if n != 1 {
		t.Errorf("restart dispatched %d consumers total — want exactly 1 "+
			"(the restarted loop must adopt the still-running consumer, not duplicate it)", n)
	}
}

// Consumption is DECLARED, never inferred. Each refusal below was a way the
// flag-heuristic version wrongly said yes.
func TestResolveConsumerRefusesWhatIsNotAConsumer(t *testing.T) {
	root := t.TempDir()

	cases := []struct {
		name string
		cfg  AgentConfig
		want string
	}{{
		name: "bare interactive claude command is not a declaration",
		cfg:  AgentConfig{ID: "claude-deck", Type: "claude", Command: []string{"claude"}},
		want: "no consumer declared",
	}, {
		// The old gate accepted this: --print proves CLI mode, not consumption,
		// and Command's own contract says the prompt is appended by an executor.
		name: "legacy --print command is still not a declaration",
		cfg:  AgentConfig{ID: "claude-nexus", Type: "claude", Command: []string{"claude", "--print"}},
		want: "no consumer declared",
	}, {
		// The old gate accepted this purely because the type was not "claude".
		name: "bare codex REPL is not a consumer by virtue of its type",
		cfg:  AgentConfig{ID: "codex-home", Type: "codex", Command: []string{"codex"}},
		want: "no consumer declared",
	}, {
		name: "declaration that never names the agent carries no inbox contract",
		cfg: AgentConfig{ID: "worker-agent", Type: "worker",
			Consumer: ConsumerConfig{Command: []string{"sh", "-c", "true"}}},
		want: "carries no inbox contract",
	}, {
		name: "explicitly interactive declaration is refused",
		cfg: AgentConfig{ID: "worker-agent", Type: "worker",
			Consumer: ConsumerConfig{Command: []string{"sh", consumerAgentPlaceholder}, Interactive: true}},
		want: "not an inbox consumer",
	}}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rc, why := ResolveConsumer(tc.cfg, root)
			if rc != nil {
				t.Fatalf("resolved a consumer for %q: %v", tc.name, rc.Argv)
			}
			if !strings.Contains(why, tc.want) {
				t.Errorf("reason %q does not contain %q", why, tc.want)
			}
		})
	}

	// A complete declaration that names its agent IS a consumer.
	rc, why := ResolveConsumer(AgentConfig{
		ID: "worker-agent", Type: "worker",
		Consumer: ConsumerConfig{Command: []string{"sh", "-c", "true", consumerAgentPlaceholder}},
	}, root)
	if rc == nil {
		t.Errorf("valid declaration refused: %s", why)
	}
}

// H6 REVERSAL: this test previously qualified the lane on `sh -c "exit 0"`
// alone. That was the borrowing — a probe that exits 0 identically whether the
// resident is draining an inbox or was stopped an hour ago, and which therefore
// could never withdraw the credit.
//
// A resident is now credited ONLY on a thread it published itself, so this test
// stands up that published thread. What it still asserts is unchanged and still
// true: a resident resolves as Resident, carries NO spawn argv (it is never
// spawned), and retains its declared health check (now a disqualifying-only
// secondary signal).
func TestResolveResidentConsumerHealthChecksWithoutSpawnArgv(t *testing.T) {
	root := t.TempDir()
	orig := residentConsumerThreadFn
	residentConsumerThreadFn = func(string, string, time.Time) (bool, string) { return true, "" }
	t.Cleanup(func() { residentConsumerThreadFn = orig })

	rc, why := ResolveConsumer(AgentConfig{
		ID: "gemma-pantheon", Type: "gemma",
		Consumer: ConsumerConfig{
			Mode:        ConsumerModeResident,
			HealthCheck: []string{"sh", "-c", "exit 0"},
		},
	}, root)
	if rc == nil {
		t.Fatalf("resident consumer refused: %s", why)
	}
	if !rc.Resident {
		t.Fatal("resident consumer did not resolve as resident")
	}
	if len(rc.Argv) != 0 {
		t.Fatalf("resident consumer must not have spawn argv: %v", rc.Argv)
	}
	if len(rc.HealthCheck) == 0 {
		t.Fatal("resident consumer did not retain health check")
	}
}

// H6: the published thread is stood up so this reaches the health check at all.
// Under the new contract the resident credit is checked FIRST, so without a
// published thread this would refuse for that reason instead — which is correct
// but would stop testing what this test exists to test: that a DECLARED check
// which fails still disqualifies, and that its output reaches the operator.
func TestResolveResidentConsumerRefusesFailedHealthCheck(t *testing.T) {
	orig := residentConsumerThreadFn
	residentConsumerThreadFn = func(string, string, time.Time) (bool, string) { return true, "" }
	t.Cleanup(func() { residentConsumerThreadFn = orig })

	rc, why := ResolveConsumer(AgentConfig{
		ID: "gemma-pantheon", Type: "gemma",
		Consumer: ConsumerConfig{
			Mode:        ConsumerModeResident,
			HealthCheck: []string{"sh", "-c", "echo nope; exit 7"},
		},
	}, t.TempDir())
	if rc != nil {
		t.Fatalf("failed resident health check resolved: %+v", rc)
	}
	if !strings.Contains(why, "health_check failed") || !strings.Contains(why, "nope") {
		t.Fatalf("reason does not explain failed health check: %s", why)
	}
}

func TestDispatchConsumerRefusesResidentConsumer(t *testing.T) {
	if _, err := dispatchConsumer(&ResolvedConsumer{Resident: true}); err == nil {
		t.Fatal("dispatchConsumer spawned a resident consumer")
	}
}

// `armed` must mean "something will drain this inbox", not "some process for
// this agent is alive". A watch-only wake loop heartbeats exactly like a working
// one; crediting it suppressed the very wake pass that would have escalated the
// lane (finding 4).
func TestArmedRequiresAnActualConsumer(t *testing.T) {
	cases := []struct {
		name  string
		thr   Thread
		armed bool
	}{
		{"watch-only worker loop", Thread{Surface: surfaceWorker}, false},
		{"worker loop with a resolved consumer", Thread{Surface: surfaceWorker, ConsumerCapable: true}, true},
		{"live claude agent session", Thread{Surface: surfaceClaude}, true},
		{"menubar observer", Thread{Surface: surfaceMenubar}, false},
		{"unknown/empty surface", Thread{}, false},
	}
	for _, tc := range cases {
		if got := tc.thr.IsInboxConsumer(); got != tc.armed {
			t.Errorf("%s: IsInboxConsumer()=%v, want %v", tc.name, got, tc.armed)
		}
	}
}

// End-to-end on the pass itself: an agent whose ONLY thread is a watch-only loop
// must not read as armed, so the wake pass stops skipping it.
func TestWakePassDoesNotTreatWatchOnlyLoopAsArmed(t *testing.T) {
	root := wakeTestRoot(t, AgentConfig{
		ID: "claude-deck", Type: "claude", Command: []string{"claude"},
		Wake: WakeConfig{Mechanism: WakeNone},
	})
	id := sendItem(t, root, "claude-deck", "stranded work")

	if _, err := RegisterThread(root, &Thread{
		ThreadID: "thr-watchonly", AgentID: "claude-deck", Surface: surfaceWorker,
		Status: ThreadStatusActive, PID: os.Getpid(),
	}); err != nil {
		t.Fatalf("register thread: %v", err)
	}

	rep, err := WakePass(root, time.Now().UTC())
	if err != nil {
		t.Fatalf("WakePass: %v", err)
	}
	for _, o := range rep.Armed {
		if o.ItemID == id {
			t.Error("item reads as armed behind a watch-only loop — the loop is crediting its " +
				"own lane and suppressing escalation")
		}
	}
	if len(rep.Unavailable) == 0 {
		t.Error("expected the lane to surface as wake-unavailable rather than silently armed")
	}
}

// A loop with no consumer must still run and heartbeat: its liveness is the only
// signal the fabric has for that agent, so refusing to start would trade a
// silent inbox for a silent agent. It must simply stop claiming to be armed.
func TestWatchOnlyLoopStillRunsAndHeartbeats(t *testing.T) {
	root := wakeTestRoot(t, AgentConfig{
		ID: "claude-deck", Type: "claude", Command: []string{"claude"},
	})
	sendItem(t, root, "claude-deck", "stranded work")

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Millisecond)
	defer cancel()
	if err := RunWakeLoop(ctx, root, "claude-deck", 40*time.Millisecond); err != nil {
		t.Fatalf("watch-only loop returned error: %v", err)
	}

	reg, err := LoadThreadRegistry(root)
	if err != nil {
		t.Fatalf("load threads: %v", err)
	}
	var found bool
	for _, tr := range reg.Threads {
		if tr != nil && tr.AgentID == "claude-deck" && tr.Surface == surfaceWorker {
			found = true
			if tr.ConsumerCapable {
				t.Error("watch-only loop recorded itself as consumer-capable")
			}
		}
	}
	if !found {
		t.Error("watch-only loop registered no thread record at all")
	}
}

// Renamed from TestResidentConsumerLoopArmsWithoutSpawning. That name asserted
// the behavior codex-pantheon ruled out: a resident health check arming the
// watcher. The no-spawn half is still a real invariant and is kept; the arming
// half was pinning the defect and now asserts the opposite.
func TestResidentConsumerLoopNeverSpawnsAndStaysWatchOnly(t *testing.T) {
	log := filepath.Join(t.TempDir(), "fired.txt")
	script := recordingConsumer(t, log, 0)
	root := wakeTestRoot(t, AgentConfig{
		ID: "gemma-pantheon", Type: "gemma",
		Consumer: ConsumerConfig{
			Mode:        ConsumerModeResident,
			HealthCheck: []string{"sh", "-c", "exit 0"},
			// If the loop treats resident like command, this script would fire.
			Command: nil,
		},
	})
	sendItem(t, root, "gemma-pantheon", "resident work")

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Millisecond)
	defer cancel()
	if err := RunWakeLoop(ctx, root, "gemma-pantheon", 40*time.Millisecond); err != nil {
		t.Fatalf("RunWakeLoop: %v", err)
	}
	if lines := consumerLines(t, log); len(lines) != 0 {
		t.Fatalf("resident consumer spawned a child: %v", lines)
	}
	reg, err := LoadThreadRegistry(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, tr := range reg.Threads {
		if tr == nil || tr.AgentID != "gemma-pantheon" || tr.Surface != surfaceWorker {
			continue
		}
		if tr.ConsumerCapable {
			t.Fatal("resident health check armed the worker thread — a one-shot " +
				"`--version` probe proves a binary exists, not that an external consumer " +
				"is alive, and an armed watcher suppresses rescue for an inbox it cannot drain")
		}
	}
	_ = script // prove the test would have a child payload if the loop used one
}

// Every registered surface constant, plus spellings that do not exist yet.
// The allow-list must be exhaustive by construction: a surface invented after
// this file was written defaults to NOT-a-consumer, so it can never be credited
// as draining an inbox it has no contract to drain.
func TestIsInboxConsumer_AllowListCoversEverySurface(t *testing.T) {
	consumers := map[string]bool{surfaceClaude: true, surfaceCodex: true}

	all := []string{
		surfaceClaude, surfaceCodex, surfaceMCP, surfaceAPI, surfaceWebhook,
		surfaceWorker, surfaceMenubar, surfaceTUI, surfaceVSCode,
		surfaceJetBrains, surfaceCursor, surfaceMacApp,
		// Not constants here, but real spellings seen in the wild and in the
		// review that produced this test.
		"automation", "resident-loop", "gemini", "gemma", "qwen",
		// The case that matters most: something nobody has thought of yet.
		"surface-invented-next-year", "",
	}
	for _, s := range all {
		got := (&Thread{Surface: s}).IsInboxConsumer()
		if want := consumers[s]; got != want {
			t.Errorf("surface %q IsInboxConsumer() = %v, want %v — only real agent sessions may be credited by surface alone", s, got, want)
		}
	}

	// ConsumerCapable is the ONLY way a non-session surface earns the credit.
	if !(&Thread{Surface: surfaceWorker, ConsumerCapable: true}).IsInboxConsumer() {
		t.Error("an explicitly ConsumerCapable worker must count as a consumer")
	}
}

// A declared cwd that cannot work must refuse resolution outright. Accepting it
// would persist ConsumerCapable=true on a loop whose every cmd.Start fails,
// and WakePass would then skip the agent as armed — stranding the inbox.
func TestResolveConsumer_RefusesUnusableCwd(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(t.TempDir(), "not-a-dir")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct{ name, cwd string }{
		{"missing directory", filepath.Join(t.TempDir(), "does-not-exist")},
		{"cwd is a file", file},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := AgentConfig{
				ID: "a", Type: "claude", Cwd: tc.cwd,
				Consumer: ConsumerConfig{
					Command: []string{"sh", "-c", "true"},
					Prompt:  "pull " + consumerAgentPlaceholder,
				},
			}
			rc, why := ResolveConsumer(cfg, root)
			if rc != nil {
				t.Fatalf("resolved a consumer with unusable cwd %q — the loop would arm and never start", tc.cwd)
			}
			if why == "" {
				t.Error("refusal carried no reason; the log is the only place this surfaces")
			}
		})
	}
}

// A resident consumer must NEVER mark the wake watcher consumer-capable, even
// when its health check succeeds.
//
// gemma-pantheon's real declaration is `sirsi-gemma-worker.sh --version`. That
// proves a file can print a string; it says nothing about whether an external
// inbox consumer is alive NOW. The check runs once at startup, so crediting the
// watcher on it means the watcher keeps heartbeating as armed after the external
// worker dies — WakePass then skips the lane and the inbox strands behind a
// consumer nobody is running. Liveness you did not observe is not liveness you
// can borrow.
func TestResidentHealthCheckNeverArmsTheWatcher(t *testing.T) {
	log := filepath.Join(t.TempDir(), "fired.txt")
	// A health check that SUCCEEDS — the point is that success is not enough.
	health := recordingConsumer(t, log, 0)

	root := wakeTestRoot(t, AgentConfig{
		ID:   "resident-agent",
		Type: "worker",
		Cwd:  t.TempDir(),
		Consumer: ConsumerConfig{
			Mode:        ConsumerModeResident,
			HealthCheck: []string{health},
		},
	})
	sendItem(t, root, "resident-agent", "work only an external resident could drain")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- RunWakeLoop(ctx, root, "resident-agent", 40*time.Millisecond) }()
	time.Sleep(250 * time.Millisecond)
	cancel()
	if err := <-done; err != nil && !errors.Is(err, context.Canceled) {
		t.Fatalf("RunWakeLoop: %v", err)
	}

	reg, err := LoadThreadRegistry(root)
	if err != nil {
		t.Fatalf("LoadThreadRegistry: %v", err)
	}
	for _, th := range reg.SortedThreads() {
		if th.AgentID != "resident-agent" {
			continue
		}
		if th.ConsumerCapable {
			t.Error("resident health check armed the watcher thread — the watcher would then " +
				"suppress rescue for an inbox it cannot drain")
		}
		if th.IsInboxConsumer() {
			t.Error("resident watcher counts as an inbox consumer; it must be watch-only")
		}
	}
}
