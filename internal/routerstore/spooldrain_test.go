package routerstore

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// These fixtures use owned HTTP handlers and synthetic files only. A forward
// crossing cancellation must retain custody until its normal outcome settles;
// cancellation must neither replay it nor erase the late pending request.
func TestRelayDrainWaitsForSettledForward(t *testing.T) {
	for _, heldOutbox := range []bool{false, true} {
		name := "fresh"
		if heldOutbox {
			name = "outbox"
		}
		t.Run(name, func(t *testing.T) {
			rl, spool, entered, release := newHeldDrainRelay(t, heldOutbox)
			maintenance := newDrainTestCloser()
			rl.maintenanceStartup = func(context.Context) (io.Closer, error) { return maintenance, nil }
			ctx, cancel := context.WithCancel(context.Background())
			done := make(chan error, 1)
			var controlDuties sync.WaitGroup
			go func() {
				defer close(done)
				if os.Getenv("SIRSI_TEST_RELAY_FAST_CANCEL_CONTROL") == "1" {
					// Use the exact fast-return serving path, retaining a fixture-only
					// join for teardown. That join is NOT used by the assertion.
					done <- rl.serve(ctx, &controlDuties, false, nil)
					return
				}
				done <- rl.ServeDrained(ctx, time.Second)
			}()
			t.Cleanup(cancel)
			t.Cleanup(func() {
				cancel()
				release()
				select {
				case <-done:
				case <-time.After(2 * time.Second):
					t.Error("owned fixture did not exit")
				}
				joined := make(chan struct{})
				go func() { controlDuties.Wait(); close(joined) }()
				select {
				case <-joined:
				case <-time.After(2 * time.Second):
					t.Error("causal control retained active fixture duties")
				}
			})
			awaitDrainEntry(t, entered)
			cancel()
			publishDrainRequest(t, spool, "req", "late", "late")
			select {
			case err := <-done:
				t.Fatalf("acknowledged drain while the forward was held: %v", err)
			case <-time.After(40 * time.Millisecond):
			}
			if maintenance.isClosed() {
				t.Fatal("maintenance endpoint closed before its admitted forward settled")
			}
			release()
			select {
			case err := <-done:
				if err != nil {
					t.Fatalf("settled drain: %v", err)
				}
			case <-time.After(2 * time.Second):
				t.Fatal("settled forward did not finish drain")
			}
			if !maintenance.isClosed() {
				t.Fatal("maintenance endpoint remained open after normal worker settlement")
			}
			assertDrainRetainedPending(t, spool)
			if heldOutbox {
				if _, err := os.Stat(filepath.Join(spool, "lane", "outbox", "held.json")); !os.IsNotExist(err) {
					t.Fatalf("positively delivered held item was not settled: %v", err)
				}
				if _, err := os.Stat(filepath.Join(spool, "lane", "outbox", "tail.json")); err != nil {
					t.Fatalf("shutdown consumed the next held request: %v", err)
				}
			} else if _, err := os.Stat(filepath.Join(spool, "lane", "res", "held.json")); err != nil {
				t.Fatalf("fresh response not retained before acknowledgment: %v", err)
			}
			if files, _ := filepath.Glob(filepath.Join(spool, "lane", "inflight", "*.json")); len(files) != 0 {
				t.Fatalf("inflight custody remains after successful drain: %v", files)
			}
		})
	}
}

func TestRelayDrainTimeoutIsNotSuccessOrCancellationOfForward(t *testing.T) {
	rl, spool, entered, release := newHeldDrainRelay(t, false)
	maintenance := newDrainTestCloser()
	rl.maintenanceStartup = func(context.Context) (io.Closer, error) { return maintenance, nil }
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	done := make(chan error, 1)
	go func() { done <- rl.ServeDrained(ctx, 30*time.Millisecond) }()
	awaitDrainEntry(t, entered)
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, ErrRelayDrainIncomplete) {
			t.Fatalf("held duty must refuse drain, got %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("drain deadline was not bounded")
	}
	if maintenance.isClosed() {
		t.Fatal("maintenance endpoint closed while timed-out worker remained owned")
	}
	if _, err := os.Stat(filepath.Join(spool, "lane", "inflight", "held.json")); err != nil {
		t.Fatalf("timeout lost the original in-flight request: %v", err)
	}
	release()
	deadline := time.Now().Add(time.Second)
	for {
		_, responseErr := os.Stat(filepath.Join(spool, "lane", "res", "held.json"))
		_, inflightErr := os.Stat(filepath.Join(spool, "lane", "inflight", "held.json"))
		if responseErr == nil && os.IsNotExist(inflightErr) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("owned forward did not settle after the timeout")
		}
		time.Sleep(time.Millisecond)
	}
	select {
	case <-maintenance.closed:
	case <-time.After(time.Second):
		t.Fatal("timed-out maintenance owner did not close after worker settlement")
	}
}

func TestRelayDrainRefusesLostResponseAndRetainsOutcomeCustody(t *testing.T) {
	rl, spool, entered, release := newHeldDrainRelay(t, false)
	// A regular file at res prevents publication without depending on uid/mode
	// permissions. The server still positively answers the admitted request.
	if err := os.WriteFile(filepath.Join(spool, "lane", "res"), []byte("fixture obstruction"), 0o600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	done := make(chan error, 1)
	go func() { done <- rl.ServeDrained(ctx, time.Second) }()
	awaitDrainEntry(t, entered)
	cancel()
	release()
	select {
	case err := <-done:
		if !errors.Is(err, ErrRelaySettlementIncomplete) {
			t.Errorf("failed publication must return settlement refusal, got %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("failed settlement did not return a bounded refusal")
	}
	if _, err := os.Stat(filepath.Join(spool, "lane", "inflight", "held.json")); err != nil {
		t.Errorf("failed response lost outcome custody: %v", err)
	}
}

func TestRelayDrainRecoveryPreservesUnpublishedUnknownOutcome(t *testing.T) {
	spool := t.TempDir()
	publishDrainRequest(t, spool, "inflight", "prior", "prior")
	inflight := filepath.Join(spool, "lane", "inflight", "prior.json")
	original, err := os.ReadFile(inflight)
	if err != nil {
		t.Fatal(err)
	}
	inflightInfo, err := os.Stat(inflight)
	if err != nil {
		t.Fatal(err)
	}
	want, err := json.Marshal(spoolRequest{Method: "ListAll", Headers: map[string]string{"X-Sirsi-Session": "prior"}, Body: []byte(`{"args":[]}`)})
	if err != nil {
		t.Fatal(err)
	}
	if string(original) != string(want) {
		t.Fatal("fixture request bytes differ from the exact expected envelope")
	}
	failedDir := filepath.Join(spool, "lane", "failed")
	if err := os.Mkdir(failedDir, 0o700); err != nil {
		t.Fatal(err)
	}
	occupied := []byte("pre-existing failed custody for a distinct envelope")
	failed := filepath.Join(failedDir, "prior.json")
	if err := os.WriteFile(failed, occupied, 0o600); err != nil {
		t.Fatal(err)
	}
	failedInfo, err := os.Stat(failed)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(spool, "lane", "res"), []byte("fixture obstruction"), 0o600); err != nil {
		t.Fatal(err)
	}
	rl := &Relay{Spool: spool, Base: "http://127.0.0.1", Token: "fixture", Client: &http.Client{}, Log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := rl.ServeDrained(ctx, time.Second); !errors.Is(err, ErrRelaySettlementIncomplete) {
		t.Fatalf("startup recovery lost its publication refusal: %v", err)
	}
	if !rl.settlementFailed.Load() {
		t.Fatal("failed outcome-unknown publication did not leave sticky settlement debt")
	}
	got, err := os.ReadFile(failed)
	gotFailedInfo, statErr := os.Stat(failed)
	if err != nil || string(got) != string(occupied) || statErr != nil || !os.SameFile(failedInfo, gotFailedInfo) {
		t.Fatalf("occupied failed custody changed: bytes=%q read=%v stat=%v", got, err, statErr)
	}
	got, err = os.ReadFile(inflight)
	gotInflightInfo, statErr := os.Stat(inflight)
	if err != nil || string(got) != string(original) || statErr != nil || !os.SameFile(inflightInfo, gotInflightInfo) {
		t.Fatalf("incoming prior envelope changed on occupied destination: bytes=%q read=%v stat=%v", got, err, statErr)
	}
	for _, path := range []string{
		filepath.Join(spool, "lane", "req", "prior.json"),
		filepath.Join(spool, "lane", "outbox", "prior.json"),
	} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("original envelope was duplicated into an active/retry path %s: %v", path, err)
		}
	}
}

func TestRelayDrainRefusesInflightCleanupFailure(t *testing.T) {
	spool := t.TempDir()
	retained := filepath.Join(spool, "retained")
	if err := os.Mkdir(retained, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(retained, "owned"), []byte("fixture"), 0o600); err != nil {
		t.Fatal(err)
	}
	rl := &Relay{Spool: spool, Base: "http://127.0.0.1", Token: "fixture", Client: &http.Client{}, Log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	if rl.removeSettledInflight(retained) {
		t.Fatal("nonempty-directory cleanup unexpectedly reported success")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := rl.ServeDrained(ctx, time.Second); !errors.Is(err, ErrRelaySettlementIncomplete) {
		t.Fatalf("cleanup failure was not sticky in drain: %v", err)
	}
	if _, err := os.Stat(filepath.Join(retained, "owned")); err != nil {
		t.Fatalf("failed cleanup changed retained data: %v", err)
	}
}

func TestRelayPrepareDrainKeepsLateForwardOwnedAndResumesOnTimeout(t *testing.T) {
	rl, spool, entered, release := newHeldDrainRelay(t, false)
	finished := make(chan bool, 1)
	go func() {
		finished <- rl.handleOne(context.Background(), "lane", filepath.Join(spool, "lane", "req", "held.json"))
	}()
	awaitDrainEntry(t, entered)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	err := rl.prepareDrain(ctx)
	cancel()
	if !errors.Is(err, ErrRelayDrainIncomplete) {
		t.Fatalf("held admitted forward was not a bounded refusal: %v", err)
	}
	if !rl.beginDuty() {
		t.Fatal("timeout left the active relay paused")
	}
	rl.endDuty()
	if _, err := os.Stat(filepath.Join(spool, "lane", "inflight", "held.json")); err != nil {
		t.Fatalf("timeout lost active forward custody: %v", err)
	}
	release()
	select {
	case handled := <-finished:
		if !handled {
			t.Fatal("late forward did not settle normally")
		}
	case <-time.After(time.Second):
		t.Fatal("late forward was canceled or abandoned")
	}
	if err := rl.prepareDrain(context.Background()); err == nil {
		t.Fatal("unbounded control context authorized drain")
	}
	bounded, stop := context.WithTimeout(context.Background(), time.Second)
	defer stop()
	if err := rl.prepareDrain(bounded); err != nil {
		t.Fatalf("settled active relay did not acknowledge: %v", err)
	}
	if rl.beginDuty() {
		rl.endDuty()
		t.Fatal("acknowledged drain admitted another forward")
	}
	rl.resumeAfterDrain()
	if !rl.beginDuty() {
		t.Fatal("explicit resume did not reopen the same relay")
	}
	rl.endDuty()
	if _, err := os.Stat(filepath.Join(spool, "lane", "res", "held.json")); err != nil {
		t.Fatalf("late response was not retained: %v", err)
	}
}

func TestRelayPrepareDrainWaitsForHeldOutboxRetry(t *testing.T) {
	rl, spool, entered, release := newHeldDrainRelay(t, true)
	finished := make(chan outboxOutcome, 1)
	go func() {
		finished <- rl.retryOutboxOne(context.Background(), "lane", filepath.Join(spool, "lane", "outbox", "held.json"))
	}()
	awaitDrainEntry(t, entered)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	err := rl.prepareDrain(ctx)
	cancel()
	if !errors.Is(err, ErrRelayDrainIncomplete) {
		t.Fatalf("held outbox retry received a false acknowledgment: %v", err)
	}
	release()
	select {
	case result := <-finished:
		if result != outboxDelivered {
			t.Fatalf("late outbox completion was not retained: %v", result)
		}
	case <-time.After(time.Second):
		t.Fatal("bounded pause canceled the already-started retry")
	}
	if _, err := os.Stat(filepath.Join(spool, "lane", "outbox", "held.json")); !os.IsNotExist(err) {
		t.Fatalf("settled held item remains retryable: %v", err)
	}
	if _, err := os.Stat(filepath.Join(spool, "lane", "outbox", "tail.json")); err != nil {
		t.Fatalf("timeout consumed a later outbox item: %v", err)
	}
	bounded, stop := context.WithTimeout(context.Background(), time.Second)
	defer stop()
	if err := rl.prepareDrain(bounded); err != nil {
		t.Fatalf("settled outbox retry did not drain: %v", err)
	}
}

func TestRelayDrainProductionCleanupFailureStaysSticky(t *testing.T) {
	rl, spool, entered, release := newHeldDrainRelay(t, false)
	// Match the root identity Serve binds: macOS may expose the temp directory
	// through /var while CheckSpoolDir returns its /private canonical path.
	spool, err := filepath.EvalSymlinks(spool)
	if err != nil {
		t.Fatalf("canonicalize fixture spool root: %v", err)
	}
	removeCalled := make(chan string, 1)
	rl.removeInflightFn = func(path string) error {
		removeCalled <- path
		return errors.New("fixture cleanup refusal")
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	done := make(chan error, 1)
	go func() { done <- rl.ServeDrained(ctx, time.Second) }()
	awaitDrainEntry(t, entered)
	cancel()
	release()
	select {
	case err := <-done:
		if !errors.Is(err, ErrRelaySettlementIncomplete) {
			t.Fatalf("production cleanup failure was not part of drain result: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("production cleanup failure did not settle the worker")
	}
	if !rl.settlementFailed.Load() {
		t.Fatal("production cleanup refusal did not set sticky settlement debt")
	}
	select {
	case path := <-removeCalled:
		if path != filepath.Join(spool, "lane", "inflight", "held.json") {
			t.Fatalf("cleanup injection reached unexpected path: %s", path)
		}
	default:
		t.Fatal("production response path did not attempt inflight cleanup")
	}
	if _, err := os.Stat(filepath.Join(spool, "lane", "res", "held.json")); err != nil {
		t.Fatalf("fixture did not publish the positive response before cleanup failure: %v", err)
	}
	if _, err := os.Stat(filepath.Join(spool, "lane", "inflight", "held.json")); err != nil {
		t.Fatalf("production cleanup failure lost inflight custody: %v", err)
	}
}

func TestRelayUnknownOutboxParkingFailureNeverRequeues(t *testing.T) {
	spool := t.TempDir()
	publishDrainRequest(t, spool, "outbox", "unknown", "unknown")
	if err := os.WriteFile(filepath.Join(spool, "lane", "failed"), []byte("fixture obstruction"), 0o600); err != nil {
		t.Fatal(err)
	}
	svc := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	t.Cleanup(svc.Close)
	rl := &Relay{Spool: spool, Base: svc.URL, Token: "fixture", Client: svc.Client(), Log: slog.New(slog.NewTextHandler(io.Discard, nil)), now: time.Now}
	if got := rl.retryOutboxOne(context.Background(), "lane", filepath.Join(spool, "lane", "outbox", "unknown.json")); got != outboxUnknown {
		t.Fatalf("expected unknown outcome, got %v", got)
	}
	if !rl.settlementFailed.Load() {
		t.Fatal("failed outcome parking did not retain settlement debt")
	}
	if _, err := os.Stat(filepath.Join(spool, "lane", "inflight", "unknown.json")); err != nil {
		t.Fatalf("unknown outcome is not held in inflight: %v", err)
	}
	if _, err := os.Stat(filepath.Join(spool, "lane", "outbox", "unknown.json")); !os.IsNotExist(err) {
		t.Fatalf("unknown mutation returned to retryable outbox: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := rl.prepareDrain(ctx); !errors.Is(err, ErrRelaySettlementIncomplete) {
		t.Fatalf("unsettled local unknown-outcome custody authorized a drain: %v", err)
	}
	if _, err := os.Stat(filepath.Join(spool, "lane", "inflight", "unknown.json")); err != nil {
		t.Fatalf("drain refusal lost unknown-outcome custody: %v", err)
	}
}

func TestRelayServeCancellationWaitsForSettlement(t *testing.T) {
	rl, spool, entered, release := newHeldDrainRelay(t, false)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	done := make(chan error, 1)
	go func() { done <- rl.Serve(ctx) }()
	awaitDrainEntry(t, entered)
	cancel()
	select {
	case err := <-done:
		t.Fatalf("Serve returned before its admitted worker settled: %v", err)
	case <-time.After(300 * time.Millisecond):
	}
	if _, err := os.Stat(filepath.Join(spool, "lane", "inflight", "held.json")); err != nil {
		t.Fatalf("Serve lost outstanding custody while waiting: %v", err)
	}
	release()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Serve after settlement: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Serve did not return after its admitted worker settled")
	}
}

func TestRelayDrainRefusesInvalidDeadlineAndReusedInstance(t *testing.T) {
	rl := &Relay{}
	for _, timeout := range []time.Duration{0, -time.Second, 3 * time.Minute} {
		if err := rl.ServeDrained(context.Background(), timeout); err == nil {
			t.Fatal("invalid drain deadline accepted")
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	rl.Spool, rl.Base, rl.Token = t.TempDir(), "http://127.0.0.1", "fixture"
	if err := rl.ServeDrained(ctx, time.Second); err != nil {
		t.Fatal(err)
	}
	if err := rl.ServeDrained(ctx, time.Second); err == nil {
		t.Fatal("reuse of an instance with retired lane channels accepted")
	}
}

func newHeldDrainRelay(t *testing.T, outbox bool) (*Relay, string, <-chan struct{}, func()) {
	t.Helper()
	spool := t.TempDir()
	entered, release := make(chan struct{}), make(chan struct{})
	var enteredOnce, releaseOnce sync.Once
	free := func() { releaseOnce.Do(func() { close(release) }) }
	svc := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Sirsi-Session") == "held" {
			enteredOnce.Do(func() { close(entered) })
			<-release
		}
		_, _ = w.Write([]byte(`{"result":[]}`))
	}))
	t.Cleanup(svc.Close)
	t.Cleanup(free) // release handlers before closing the fixture server
	queue := "req"
	if outbox {
		queue = "outbox"
		publishDrainRequest(t, spool, "req", "trigger", "trigger")
		publishDrainRequest(t, spool, "outbox", "tail", "tail")
	}
	publishDrainRequest(t, spool, queue, "held", "held")
	rl := &Relay{Spool: spool, Base: svc.URL, Token: "fixture", Client: svc.Client(), Log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	return rl, spool, entered, free
}

type drainTestCloser struct {
	closed chan struct{}
	once   sync.Once
}

func newDrainTestCloser() *drainTestCloser {
	return &drainTestCloser{closed: make(chan struct{})}
}

func (c *drainTestCloser) Close() error {
	c.once.Do(func() { close(c.closed) })
	return nil
}

func (c *drainTestCloser) isClosed() bool {
	select {
	case <-c.closed:
		return true
	default:
		return false
	}
}

func publishDrainRequest(t *testing.T, spool, queue, id, session string) {
	t.Helper()
	dir := filepath.Join(spool, "lane", queue)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := writeAtomic(filepath.Join(dir, id+".json"), spoolRequest{Method: "ListAll", Headers: map[string]string{"X-Sirsi-Session": session}, Body: []byte(`{"args":[]}`)}, 0o600); err != nil {
		t.Fatal(err)
	}
}

func awaitDrainEntry(t *testing.T, entered <-chan struct{}) {
	t.Helper()
	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		t.Fatal("owned forward never entered its handler")
	}
}

func assertDrainRetainedPending(t *testing.T, spool string) {
	t.Helper()
	if _, err := os.Stat(filepath.Join(spool, "lane", "req", "late.json")); err != nil {
		t.Fatalf("late pending request was consumed or lost: %v", err)
	}
	if _, err := os.Stat(filepath.Join(spool, "lane", "res", "late.json")); !os.IsNotExist(err) {
		t.Fatalf("late request was answered after cancellation: %v", err)
	}
}
