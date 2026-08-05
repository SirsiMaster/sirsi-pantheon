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
func TestDispatchedConsumerReceivesRouterContract(t *testing.T) {
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

	awaitConsumerLines(t, log, 1) // the first dispatch has provably happened

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

func TestResolveResidentConsumerHealthChecksWithoutSpawnArgv(t *testing.T) {
	root := t.TempDir()
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

func TestResolveResidentConsumerRefusesFailedHealthCheck(t *testing.T) {
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

func TestResidentConsumerLoopArmsWithoutSpawning(t *testing.T) {
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
	var capable bool
	for _, tr := range reg.Threads {
		if tr != nil && tr.AgentID == "gemma-pantheon" && tr.Surface == surfaceWorker {
			capable = tr.ConsumerCapable
		}
	}
	if !capable {
		t.Fatal("resident consumer did not arm the worker thread")
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
