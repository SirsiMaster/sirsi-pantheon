package router

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// fakeClock gates the pump's sleeps: every sleep is reported and then blocks
// until the test releases it, so the test owns time and counts exactly.
type fakeClock struct {
	slept chan time.Duration
	free  chan struct{}
}

func newFakeClock() *fakeClock {
	return &fakeClock{slept: make(chan time.Duration, 64), free: make(chan struct{}, 64)}
}

func (c *fakeClock) sleep(ctx context.Context, d time.Duration) bool {
	c.slept <- d
	select {
	case <-c.free:
		return true
	case <-ctx.Done():
		return false
	}
}

func (c *fakeClock) expectSleep(t *testing.T, want time.Duration) {
	t.Helper()
	select {
	case got := <-c.slept:
		if got != want {
			t.Fatalf("slept %s, want %s", got, want)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("pump never slept %s", want)
	}
}

func (c *fakeClock) release() { c.free <- struct{}{} }

type fakeWait struct {
	calls atomic.Int64
	// results is consumed one per call; a closed/empty channel blocks the call.
	results chan waitResult
}
type waitResult struct {
	woke bool
	err  error
}

func newFakeWait() *fakeWait { return &fakeWait{results: make(chan waitResult, 64)} }

func (f *fakeWait) fn(ctx context.Context, _ string, _ time.Duration) (bool, error) {
	f.calls.Add(1)
	select {
	case r := <-f.results:
		return r.woke, r.err
	case <-ctx.Done():
		return false, ctx.Err()
	}
}

func hint(t *testing.T, w *wakeEvents) {
	t.Helper()
	select {
	case <-w.hints():
	case <-time.After(2 * time.Second):
		t.Fatal("no wake hint delivered")
	}
}

func noHint(t *testing.T, w *wakeEvents) {
	t.Helper()
	select {
	case <-w.hints():
		t.Fatal("unexpected wake hint")
	case <-time.After(60 * time.Millisecond):
	}
}

const fb = time.Minute

func startFake(t *testing.T) (*wakeEvents, *fakeWait, *fakeClock, context.CancelFunc) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	fw, fc := newFakeWait(), newFakeClock()
	w := startWakeEvents(ctx, "lane", fw.fn, fb, fc.sleep)
	return w, fw, fc, cancel
}

func TestWakeEventsNewItemWakesEarly(t *testing.T) {
	w, fw, fc, cancel := startFake(t)
	defer cancel()
	w.observed(0, nil, false, fb) // loop saw an empty inbox: arm
	fw.results <- waitResult{woke: true}
	hint(t, w)
	fc.expectSleep(t, wakeEventMinGap)
	if n := fw.calls.Load(); n != 1 {
		t.Fatalf("source consulted %d times, want 1", n)
	}
}

func TestWakeEventsSameItemsDoNotReWake(t *testing.T) {
	w, fw, fc, cancel := startFake(t)
	defer cancel()
	w.observed(0, nil, false, fb)
	fw.results <- waitResult{woke: true}
	hint(t, w)
	// Work is now open and stays open: the loop reports depth 1 every tick.
	fc.expectSleep(t, wakeEventMinGap)
	fc.release()
	for i := 0; i < 5; i++ {
		w.observed(1, nil, false, fb)
		fw.results <- waitResult{woke: true} // a level-triggered source would fire at once
	}
	time.Sleep(40 * time.Millisecond)
	noHint(t, w)
	if n := fw.calls.Load(); n != 1 {
		t.Fatalf("source consulted %d times while work stayed open, want 1 (hot loop)", n)
	}
}

func TestWakeEventsFalseWakeHoldsOffAFullInterval(t *testing.T) {
	w, fw, fc, cancel := startFake(t)
	defer cancel()
	w.observed(0, nil, false, fb)
	fw.results <- waitResult{woke: true}
	hint(t, w)
	fc.expectSleep(t, wakeEventMinGap)
	fc.release()
	w.observed(0, nil, true, fb) // woke by hint, still nothing there
	fc.expectSleep(t, fb)
}

func TestWakeEventsErrorBacksOffToTickerCadence(t *testing.T) {
	w, fw, fc, cancel := startFake(t)
	defer cancel()
	w.observed(0, nil, false, fb)
	fw.results <- waitResult{err: errors.New("service down")}
	fc.expectSleep(t, fb) // backs off a full fallback interval, no hint
	noHint(t, w)
	fc.release()
	fc.expectSleep(t, wakeEventMinGap)
	fc.release()
	// Unarmed until the loop observes an empty inbox again.
	time.Sleep(40 * time.Millisecond)
	if n := fw.calls.Load(); n != 1 {
		t.Fatalf("source consulted %d times after an error, want 1", n)
	}
}

func TestWakeEventsInstantEmptyReturnsAreRateLimited(t *testing.T) {
	w, fw, fc, cancel := startFake(t)
	defer cancel()
	w.observed(0, nil, false, fb)
	for i := 0; i < 20; i++ {
		fw.results <- waitResult{} // "unsupported": instant false, nil
	}
	fc.expectSleep(t, wakeEventMinGap)
	time.Sleep(40 * time.Millisecond)
	if n := fw.calls.Load(); n != 1 {
		t.Fatalf("source consulted %d times without the min-gap elapsing, want 1", n)
	}
	for i := 2; i <= 4; i++ {
		fc.release()
		fc.expectSleep(t, wakeEventMinGap)
		time.Sleep(20 * time.Millisecond)
		if n := fw.calls.Load(); n != int64(i) {
			t.Fatalf("after %d gaps the source was consulted %d times, want %d", i-1, n, i)
		}
	}
	noHint(t, w)
}

func TestWakeEventsBurstCoalescesToOneHint(t *testing.T) {
	w, fw, fc, cancel := startFake(t)
	defer cancel()
	for i := 0; i < 3; i++ {
		w.observed(0, nil, false, fb)
		fw.results <- waitResult{woke: true}
		fc.expectSleep(t, wakeEventMinGap)
		fc.release()
	}
	if got := len(w.ch); got != 1 {
		t.Fatalf("%d hints queued for an unconsumed burst, want 1", got)
	}
}

func TestWakeEventsStopsOnContextCancel(t *testing.T) {
	w, _, _, cancel := startFake(t)
	w.observed(0, nil, false, fb)
	cancel()
	select {
	case <-w.done:
	case <-time.After(2 * time.Second):
		t.Fatal("pump did not exit on cancel")
	}
}

func TestNilWakeEventsIsTickerOnly(t *testing.T) {
	var w *wakeEvents
	w.observed(0, nil, false, fb) // must not panic
	select {
	case <-w.hints():
		t.Fatal("a nil pump produced a hint")
	case <-time.After(20 * time.Millisecond):
	}
}

// ---- integration through RunWakeLoop --------------------------------------

type loopFixture struct {
	root, log string
	opens     atomic.Int64
	gate      chan bool // a value releases one source call with that result
	waits     atomic.Int64
}

func newLoopFixture(t *testing.T, agent string, consumerSleep time.Duration) *loopFixture {
	t.Helper()
	SetLoadAvgFn(func() (float64, bool) { return 0.1, true })
	t.Cleanup(func() { SetLoadAvgFn(nil) })
	hermeticDispatchDir(t)
	fx := &loopFixture{gate: make(chan bool, 64)}
	fx.log = filepath.Join(t.TempDir(), "fired.txt")
	script := recordingConsumer(t, fx.log, consumerSleep)
	fx.root = wakeTestRoot(t, AgentConfig{
		ID: agent, Type: "worker",
		Consumer: ConsumerConfig{Command: []string{script, consumerAgentPlaceholder}},
	})
	old := getWakeEventOpen()
	setWakeEventOpen(func(string) (wakeEventFn, func(), error) {
		fx.opens.Add(1)
		return func(ctx context.Context, _ string, _ time.Duration) (bool, error) {
			fx.waits.Add(1)
			select {
			case v := <-fx.gate:
				return v, nil
			case <-ctx.Done():
				return false, ctx.Err()
			}
		}, func() {}, nil
	})
	t.Cleanup(func() { setWakeEventOpen(old) })
	return fx
}

func (fx *loopFixture) run(t *testing.T, agent string, interval time.Duration) (stop func()) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	var once sync.Once
	go func() { done <- RunWakeLoop(ctx, fx.root, agent, interval) }()
	return func() {
		once.Do(func() {
			cancel()
			if err := <-done; err != nil && !errors.Is(err, context.Canceled) {
				t.Errorf("RunWakeLoop: %v", err)
			}
		})
	}
}

func awaitWaits(t *testing.T, fx *loopFixture, n int64) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for fx.waits.Load() < n {
		if time.Now().After(deadline) {
			t.Fatalf("pump never consulted the source %d time(s)", n)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// New item, hour-long fallback tick: only the event can dispatch it.
func TestWakeLoopEventModeDispatchesNewItemBeforeTheTick(t *testing.T) {
	t.Setenv(WakeEventTickEnv, "1")
	fx := newLoopFixture(t, "ev-agent", 0)
	stop := fx.run(t, "ev-agent", time.Hour)
	defer stop()

	awaitWaits(t, fx, 1) // loop observed an empty inbox and armed the source
	sendItem(t, fx.root, "ev-agent", "new work")
	fx.gate <- true
	if n := len(awaitConsumerLines(t, fx.log, 1)); n != 1 {
		t.Fatalf("event mode dispatched %d consumers before the tick, want 1", n)
	}
}

// Negative control: identical run with the setting OFF never reaches the source
// and, with the same hour-long tick, never sees the item.
func TestWakeLoopDefaultOffIgnoresEventsEntirely(t *testing.T) {
	fx := newLoopFixture(t, "off-agent", 0)
	stop := fx.run(t, "off-agent", time.Hour)
	time.Sleep(150 * time.Millisecond)
	sendItem(t, fx.root, "off-agent", "new work")
	fx.gate <- true
	time.Sleep(400 * time.Millisecond)
	stop()
	if fx.opens.Load() != 0 || fx.waits.Load() != 0 {
		t.Fatalf("default-off loop touched the event source (opens=%d waits=%d)", fx.opens.Load(), fx.waits.Load())
	}
	if n := len(consumerLines(t, fx.log)); n != 0 {
		t.Fatalf("default-off loop with a 1h tick dispatched %d consumers; the control is not isolating the event path", n)
	}
}

// Same item sequence, ticker vs event mode with a hostile always-true source:
// identical spawn count, and the source is not hammered.
func TestWakeLoopEventModeSpawnCountMatchesTicker(t *testing.T) {
	count := func(eventMode bool) (spawns int, waits int64) {
		if eventMode {
			t.Setenv(WakeEventTickEnv, "1")
		} else {
			t.Setenv(WakeEventTickEnv, "")
		}
		fx := newLoopFixture(t, "cmp-agent", 2*time.Second) // holds the slot
		for i := 0; i < 64; i++ {
			fx.gate <- true // hostile: level-triggered, always "work"
		}
		sendItem(t, fx.root, "cmp-agent", "work never drained")
		stop := fx.run(t, "cmp-agent", 40*time.Millisecond)
		mustAwaitConsumerLines(t, fx.log, 1)
		time.Sleep(500 * time.Millisecond) // many ticks and many hostile events
		spawns = len(consumerLines(t, fx.log))
		stop()
		return spawns, fx.waits.Load()
	}
	base, _ := count(false)
	ev, waits := count(true)
	if base != 1 || ev != base {
		t.Fatalf("spawns: ticker=%d event=%d, want identical (1)", base, ev)
	}
	if waits > 3 {
		t.Fatalf("hostile source consulted %d times in ~0.6s with work open; hot loop", waits)
	}
}
