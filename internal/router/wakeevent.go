package router

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/dispatch"
)

// WakeEventTickEnv opts a wake loop into event-driven early wake (A27, ADR-065
// Decision 1). DEFAULT OFF: unset or anything but "1" leaves the loop on its
// fixed ticker, byte-for-byte as before.
const WakeEventTickEnv = "SIRSI_WAKE_EVENT_TICK"

// wakeEventMinGap bounds how often the event source may be consulted, whatever
// it returns. It is the floor under every hot-loop failure mode.
const wakeEventMinGap = time.Second

// wakeEventFn blocks until work is open for agent (true), the timeout lapses
// (false), or it fails. dispatch.Facade.WaitInbox is the production source.
type wakeEventFn func(ctx context.Context, agent string, timeout time.Duration) (bool, error)

var (
	wakeEventOpenMu sync.RWMutex
	wakeEventOpenFn = openWakeEventSource
)

func getWakeEventOpen() func(string) (wakeEventFn, func(), error) {
	wakeEventOpenMu.RLock()
	defer wakeEventOpenMu.RUnlock()
	return wakeEventOpenFn
}

func setWakeEventOpen(fn func(string) (wakeEventFn, func(), error)) {
	wakeEventOpenMu.Lock()
	defer wakeEventOpenMu.Unlock()
	wakeEventOpenFn = fn
}

func openWakeEventSource(routerRoot string) (wakeEventFn, func(), error) {
	f, err := dispatch.OpenRoot(routerRoot)
	if err != nil {
		return nil, nil, err
	}
	return f.WaitInbox, func() { _ = f.Close() }, nil
}

// wakeEvents turns a level-triggered wait into a coalesced, rate-limited
// "inbox went non-empty" hint. It only ever SHORTENS the wait for the next loop
// iteration; the iteration itself runs every existing dispatch gate (progress
// gate, spawn ceiling, holds, backoff), so a hint can never cause a spawn the
// ticker would not have caused one tick later.
//
// Hot-loop defenses, each independently sufficient:
//   - the loop re-arms the source only when it has OBSERVED an empty inbox, so a
//     level-triggered "true" while work is already open is never re-consumed;
//   - a hint that was followed by an empty observation (false wake) holds the
//     source off for a full fallback interval;
//   - any error holds it off for a full fallback interval (ticker cadence);
//   - consecutive source calls are at least wakeEventMinGap apart;
//   - the hint channel has capacity 1, so bursts coalesce into one wake.
type wakeEvents struct {
	ch    chan struct{}
	rearm chan time.Duration
	done  chan struct{} // closed when the pump goroutine has exited
}

// startWakeEvents launches the pump. sleep returns false when ctx ended; it is
// injected so tests can drive it with a fake clock.
func startWakeEvents(ctx context.Context, agent string, wait wakeEventFn, fallback time.Duration,
	sleep func(context.Context, time.Duration) bool) *wakeEvents {
	w := &wakeEvents{ch: make(chan struct{}, 1), rearm: make(chan time.Duration, 1), done: make(chan struct{})}
	go func() {
		defer close(w.done)
		armed := false
		for {
			if !armed {
				select {
				case <-ctx.Done():
					return
				case hold := <-w.rearm:
					if hold > 0 && !sleep(ctx, hold) {
						return
					}
					armed = true
				}
			}
			woke, err := wait(ctx, agent, fallback)
			if ctx.Err() != nil {
				return
			}
			switch {
			case err != nil:
				armed = false
				if !sleep(ctx, fallback) { // back off to the ticker cadence
					return
				}
			case woke:
				armed = false
				select { // drop re-arms queued before this hint was consumed
				case <-w.rearm:
				default:
				}
				select {
				case w.ch <- struct{}{}:
				default: // a hint is already pending: coalesce
				}
			}
			if !sleep(ctx, wakeEventMinGap) {
				return
			}
		}
	}()
	return w
}

// observed tells the pump what the loop saw this iteration. Call it once per
// iteration, immediately before waiting. byEvent is whether this iteration was
// started by a hint.
func (w *wakeEvents) observed(depth int, readErr error, byEvent bool, fallback time.Duration) {
	if w == nil || readErr != nil || depth != 0 {
		return // work is open (or unknown): the ticker and the consumer own it
	}
	hold := time.Duration(0)
	if byEvent {
		hold = fallback // the hint found nothing: do not trust the source for a while
	}
	select {
	case w.rearm <- hold:
	default:
	}
}

func (w *wakeEvents) hints() <-chan struct{} {
	if w == nil {
		return nil // nil channel blocks forever: identical to the ticker-only select
	}
	return w.ch
}

func realSleep(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}

// startLoopWakeEvents is RunWakeLoop's single entry: nil unless explicitly
// enabled AND the source opens. Failure to open is logged and degrades to the
// plain ticker; it is never an error for the loop.
func startLoopWakeEvents(ctx context.Context, routerRoot, agentID string, interval time.Duration, enabled bool) (*wakeEvents, func()) {
	if !enabled {
		return nil, func() {}
	}
	wait, closeFn, err := getWakeEventOpen()(routerRoot)
	if err != nil || wait == nil {
		log.Printf("wake-loop %s: event wake unavailable, using ticker only: %v", agentID, err)
		return nil, func() {}
	}
	pctx, cancel := context.WithCancel(ctx)
	w := startWakeEvents(pctx, agentID, wait, interval, realSleep)
	// Stop the pump and join it BEFORE closing the store it is blocked in.
	return w, func() { cancel(); <-w.done; closeFn() }
}
