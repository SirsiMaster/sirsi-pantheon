package router

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// TestDeliveryStrategyFor_TypeTable exercises ADR-065 Decision 3a's lookup
// table directly: each registered type/session-mode combination must resolve
// to exactly the strategy the ADR names, including the legacy-default
// fallback (no session_mode set) that isInteractiveSpawn already encodes.
func TestDeliveryStrategyFor_TypeTable(t *testing.T) {
	cases := []struct {
		name string
		cfg  AgentConfig
		want string
	}{
		{"codex", AgentConfig{ID: "a", Type: "codex"}, StrategyFilesystemPush},
		{"claude interactive", AgentConfig{ID: "a", Type: "claude", Wake: WakeConfig{SessionMode: "interactive"}}, StrategySessionMessage},
		{"claude headless", AgentConfig{ID: "a", Type: "claude", Wake: WakeConfig{SessionMode: "headless"}}, StrategyCLISpawn},
		{"claude legacy default (no session_mode)", AgentConfig{ID: "a", Type: "claude"}, StrategySessionMessage},
		{"gemma", AgentConfig{ID: "a", Type: "gemma"}, StrategyResidentNotify},
		{"qwen", AgentConfig{ID: "a", Type: "qwen"}, StrategyResidentNotify},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := DeliveryStrategyFor(tc.cfg)
			if err != nil {
				t.Fatalf("DeliveryStrategyFor: %v", err)
			}
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

// TestDeliveryStrategyFor_UnknownTypeErrors is the negative control for an
// unregistered type: the table must refuse to guess rather than default to
// some strategy silently.
func TestDeliveryStrategyFor_UnknownTypeErrors(t *testing.T) {
	_, err := DeliveryStrategyFor(AgentConfig{ID: "a", Type: "human"})
	if err == nil {
		t.Fatal("expected an error for an unmapped type, got nil")
	}
}

// TestDeliveryStrategyNarrowsNotWidens is the task-3 regression test for
// Decision 3a's "a lane may narrow but not widen its type's strategy (a
// codex lane cannot opt into HTTP)." A declared Delivery.Target that matches
// its strategy's one allowed address narrows cleanly; any other declared
// Target — including the HTTP-shaped "endpoint" on a codex lane, and any
// address at all on a strategy with no address concept — is a widen attempt
// and must be refused, not silently accepted.
func TestDeliveryStrategyNarrowsNotWidens(t *testing.T) {
	t.Run("codex narrows to its own spool-dir", func(t *testing.T) {
		cfg := AgentConfig{ID: "codex-a", Type: "codex", Delivery: &DeliveryConfig{Target: "spool-dir", SpoolDir: "/relay/codex-a"}}
		got, err := DeliveryStrategyFor(cfg)
		if err != nil {
			t.Fatalf("DeliveryStrategyFor: %v", err)
		}
		if got != StrategyFilesystemPush {
			t.Errorf("got %q, want %q", got, StrategyFilesystemPush)
		}
	})

	t.Run("codex widening to endpoint is refused", func(t *testing.T) {
		cfg := AgentConfig{ID: "codex-a", Type: "codex", Delivery: &DeliveryConfig{Target: "endpoint", Endpoint: "https://example"}}
		if _, err := DeliveryStrategyFor(cfg); err == nil {
			t.Error("expected a widen-attempt error for codex+endpoint, got nil — rs-22e's no-network boundary would be bypassed by construction")
		}
	})

	t.Run("claude headless declaring spool-dir is refused", func(t *testing.T) {
		cfg := AgentConfig{ID: "claude-a", Type: "claude", Wake: WakeConfig{SessionMode: "headless"}, Delivery: &DeliveryConfig{Target: "spool-dir", SpoolDir: "/relay/claude-a"}}
		if _, err := DeliveryStrategyFor(cfg); err == nil {
			t.Error("expected an error — cli-spawn has no address to narrow, spool-dir is not a subset of it")
		}
	})

	t.Run("no declared Delivery leaves the mandated strategy untouched", func(t *testing.T) {
		got, err := DeliveryStrategyFor(AgentConfig{ID: "codex-a", Type: "codex"})
		if err != nil {
			t.Fatalf("DeliveryStrategyFor: %v", err)
		}
		if got != StrategyFilesystemPush {
			t.Errorf("got %q, want %q", got, StrategyFilesystemPush)
		}
	})
}

// TestInformerCoexistenceNegativeControl6_SimultaneousEdgeNoDuplicateSpawn is
// ADR-065 Phase 1 Negative Control 6 (rs-37 task 5), exercised at the
// informer's own call site: the informer's runInformerLane and a simulated
// RunWakeLoop dispatch (the identical admitConsumer call wake.go's dispatch
// site makes) observe the SAME edge for the SAME agent concurrently. Exactly
// one process must actually spawn.
//
// Red-before-green (A35) verified manually: with runInformerLane's admission
// call swapped for a direct dispatchConsumer (bypassing the shared flock —
// the bug this boundary exists to prevent), this test reproduced 2 spawns in
// every run; restored to admitConsumer, it is 1/1. Not shipped as the
// swapped variant — that would be a standing flaky/broken test, not evidence.
func TestInformerCoexistenceNegativeControl6_SimultaneousEdgeNoDuplicateSpawn(t *testing.T) {
	// The real load-average guard shells out to `top -l 2 -n 0 -s 1` (~2-4s,
	// backpressure.go) on every dispatch attempt — legitimate production
	// latency, but on a host slow enough it can outlast this fixture's
	// consumer and turn a live-process adoption into a false "already
	// exited, spawn fresh" read. Deterministic/fast stand-in via the
	// existing test seam; the real shell-out is covered by its own tests.
	SetLoadAvgFn(func() (float64, bool) { return 0, true })
	t.Cleanup(func() { SetLoadAvgFn(nil) })

	root := t.TempDir()
	logPath := filepath.Join(root, "invocations.log")
	consumer := &ResolvedConsumer{Argv: []string{sleepConsumer(t, logPath)}}
	events := make(chan struct{}, 1)

	var wg sync.WaitGroup
	wg.Add(2)
	var wakeErr error
	go func() {
		defer wg.Done()
		runInformerLane(context.Background(), root, "edge-agent", consumer, events)
	}()
	go func() {
		defer wg.Done()
		// Simulates RunWakeLoop's own dispatch call site (wake.go) observing
		// the identical edge concurrently with the informer.
		_, _, wakeErr = admitConsumer(root, "edge-agent", consumer)
	}()

	events <- struct{}{} // the simultaneous edge both observers see
	close(events)        // lets runInformerLane's range exit once drained

	wg.Wait()
	if wakeErr != nil {
		t.Fatalf("simulated RunWakeLoop admitConsumer: %v", wakeErr)
	}

	// wg.Wait() only proves both admission attempts RETURNED (Start() begun);
	// a spawned child's own "echo $$ >> log" still needs a scheduler turn to
	// actually run. Settle for the full window rather than breaking on the
	// first line seen — a second, slower spawn's echo arriving after an
	// early break would read as "1 spawn" when two processes actually ran.
	var lines []string
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		time.Sleep(200 * time.Millisecond)
		b, _ := os.ReadFile(logPath)
		lines = splitNonEmptyLines(string(b))
	}
	if len(lines) != 1 {
		t.Fatalf("got %d process spawn(s) (%v) for one edge observed by both the informer and a simulated "+
			"RunWakeLoop, want exactly 1 — a shared PID filename alone is not atomic spawn admission",
			len(lines), lines)
	}
}

// TestDeliveryAttemptDoesNotImplyReadAck is ADR-065 Phase 1 Negative Control
// 5 (rs-37 task 6, Decision 2 correction): a successful delivery attempt
// (the informer's push) must record wake_status/wake_attempted_at and must
// NEVER set acked_at — only the lane's own `router acknowledge` call
// (work.SetAckedAt) does that.
func TestDeliveryAttemptDoesNotImplyReadAck(t *testing.T) {
	root := t.TempDir()
	t.Setenv("SIRSI_ROUTER_STORE_WAKE", "0")
	t.Setenv("SIRSI_ROUTER_DB", filepath.Join(root, "unused-store.db"))

	writeItem(t, root, "20261007-000001-nc5", "owner", "nc5-agent", "NC5 fixture", "open", time.Now().UTC(), time.Time{})

	before, err := OpenItems(root, "nc5-agent")
	if err != nil || len(before) != 1 {
		t.Fatalf("OpenItems before mark: items=%v err=%v", before, err)
	}
	if before[0].AckedAt != "" {
		t.Fatalf("fixture item already has acked_at %q before any mark", before[0].AckedAt)
	}

	markDeliveryAttempted(root, "nc5-agent")

	after, err := OpenItems(root, "nc5-agent")
	if err != nil || len(after) != 1 {
		t.Fatalf("OpenItems after mark: items=%v err=%v", after, err)
	}
	item := after[0]
	if item.WakeStatus != WakeStatusAttempted || item.WakeAttemptedAt == "" {
		t.Fatalf("delivery-attempt marker not recorded: wake_status=%q wake_attempted_at=%q", item.WakeStatus, item.WakeAttemptedAt)
	}
	if item.AckedAt != "" {
		t.Fatalf("delivery attempt set acked_at=%q — adapter success must never imply read-acknowledgement "+
			"(ADR-065 Decision 2 correction); only `router acknowledge` (work.SetAckedAt) may write it", item.AckedAt)
	}
}

// TestRunInformer_RemoteStoreBackend_ReportsUnsupported is the rs-37 task 7
// service-side-wiring regression (ADR-065/067 Decision-6, SSA ruling
// 20261007-230828): a RunInformer process whose resolved store is a
// *routerstore.RemoteStore — this host is a client of a router service
// elsewhere, not the authoritative backend — has no authenticated per-host
// dispatch-authorization channel yet. It must refuse explicitly with
// ErrInformerRemoteBackendUnsupported BEFORE subscribing to any lane, never
// silently run the subscribe/dispatch loop as if the in-process host-pin
// predicate (InformerHostAuthorized) had been consulted.
func TestRunInformer_RemoteStoreBackend_ReportsUnsupported(t *testing.T) {
	root := t.TempDir()
	t.Setenv("SIRSI_ROUTER_URL", "http://127.0.0.1:0") // unreachable by construction; must never be dialed
	t.Setenv("SIRSI_ROUTER_TOKEN", "test-token")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err := RunInformer(ctx, root)
	if !errors.Is(err, ErrInformerRemoteBackendUnsupported) {
		t.Fatalf("RunInformer with a RemoteStore backend: got err=%v, want ErrInformerRemoteBackendUnsupported", err)
	}
}

// TestRunInformer_LocalBackend_NotReportedUnsupported is the positive control
// for the above: a local SQLite backend (the authoritative host) must NOT be
// refused by the RemoteStore gate — an empty agent registry simply yields no
// lanes to subscribe and RunInformer returns nil.
func TestRunInformer_LocalBackend_NotReportedUnsupported(t *testing.T) {
	root := t.TempDir()
	t.Setenv("SIRSI_ROUTER_DB", filepath.Join(root, "local.db"))

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := RunInformer(ctx, root); err != nil {
		t.Fatalf("RunInformer with a local SQLite backend and an empty registry: got err=%v, want nil", err)
	}
}
