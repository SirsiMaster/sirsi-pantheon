package routerstore

// Event-driven dispatch — Phase 2 (PRD §2, "kill the poll loop").
//
// One dispatcher owns delivery. SendGuarded signals the addressed agent's
// in-process wait channels directly and pokes the per-agent notify FIFO under
// ~/.sirsi/notify/<agent> for cross-process waiters (no new dependencies —
// syscall.Mkfifo; a FIFO with no reader is skipped without blocking). Wait is
// a real blocking wait with the timeout as a ceiling, not a poll interval,
// plus a long safety re-check so a missed signal never strands a waiter.

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

// WaitSafetyRecheck is the fallback re-check interval: even if every signal
// is missed, a waiter re-reads its inbox this often. Var so tests can shrink it.
var WaitSafetyRecheck = 30 * time.Second

// notifyBaseDir resolves the cross-process notify directory.
func (s *SQLiteStore) notifyBaseDir() string {
	if s.notifyDir != "" {
		return s.notifyDir
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".sirsi", "notify")
}

// NotifyPath returns agent's cross-process wake FIFO path.
func (s *SQLiteStore) NotifyPath(agent string) string {
	base := s.notifyBaseDir()
	if base == "" {
		return ""
	}
	return filepath.Join(base, slugify(agent))
}

// notifyWaiters wakes every in-process waiter for agent and pokes the
// cross-process FIFO. Fire-and-forget: delivery truth lives in the store row
// (a woken waiter re-reads its inbox; a missed signal is caught by the safety
// re-check), so nothing here can fail a send.
func (s *SQLiteStore) notifyWaiters(agent string) {
	s.waitMu.Lock()
	chans := s.waiters[agent]
	delete(s.waiters, agent)
	s.waitMu.Unlock()
	for _, ch := range chans {
		close(ch)
	}
	s.pokeFIFO(agent)
}

// NotifyAgent is the exported wake for callers that mutate items outside the
// facade (e.g. an owner reopen making an item claimable again).
func (s *SQLiteStore) NotifyAgent(agent string) { s.notifyWaiters(agent) }

// pokeFIFO writes one byte to agent's notify FIFO iff a reader is listening.
// O_NONBLOCK: no reader (ENXIO) or missing FIFO is a silent no-op.
func (s *SQLiteStore) pokeFIFO(agent string) {
	path := s.NotifyPath(agent)
	if path == "" {
		return
	}
	fd, err := os.OpenFile(path, os.O_WRONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		return // no FIFO or no reader — cross-process waiter isn't listening
	}
	_, _ = fd.Write([]byte{1})
	_ = fd.Close()
}

// ListenNotify creates (if needed) and opens agent's notify FIFO, delivering
// one struct{} per wake poke until ctx ends. The returned channel is closed
// on exit. Cross-process counterpart of the in-process wait channel: a
// wake-loop process selects on this instead of polling.
func (s *SQLiteStore) ListenNotify(ctx context.Context, agent string) (<-chan struct{}, error) {
	path := s.NotifyPath(agent)
	if path == "" {
		return nil, fmt.Errorf("routerstore: ListenNotify: no home dir for notify FIFO")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("routerstore: ListenNotify: %w", err)
	}
	if err := syscall.Mkfifo(path, 0o644); err != nil && !errors.Is(err, os.ErrExist) && !errors.Is(err, syscall.EEXIST) {
		return nil, fmt.Errorf("routerstore: ListenNotify: mkfifo %s: %w", path, err)
	}
	events := make(chan struct{}, 1)
	go func() {
		defer close(events)
		buf := make([]byte, 16)
		for ctx.Err() == nil {
			// O_RDONLY blocks until a writer pokes. RDWR keeps the read side
			// open across pokes so writers never race a closing reader.
			fd, err := os.OpenFile(path, os.O_RDWR, 0)
			if err != nil {
				return
			}
			done := make(chan struct{})
			go func() { <-ctx.Done(); _ = fd.Close(); close(done) }()
			for {
				if _, err := fd.Read(buf); err != nil {
					break
				}
				select {
				case events <- struct{}{}:
				default: // a pending wake already queued — coalesce
				}
			}
			<-done
		}
	}()
	return events, nil
}

// Wait blocks until an open item is addressed to agent or the timeout/ctx
// expires. It returns (true, nil) when the inbox has work — the caller then
// claims through ClaimNext (§2b: claim is the only door to execution; Wait
// observes, it never claims). A live send wakes a waiter in well under 250ms
// (PRD /goal #1, proven by TestWaitWakesUnder250ms); the timeout is a ceiling,
// not a poll floor, and WaitSafetyRecheck catches any missed signal.
func (s *SQLiteStore) Wait(ctx context.Context, agent string, timeout time.Duration) (bool, error) {
	if timeout <= 0 {
		timeout = time.Minute
	}
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()

	for {
		items, err := s.Inbox(agent)
		if err != nil {
			return false, err
		}
		if len(items) > 0 {
			return true, nil
		}

		ch := make(chan struct{})
		s.waitMu.Lock()
		if s.waiters == nil {
			s.waiters = map[string][]chan struct{}{}
		}
		s.waiters[agent] = append(s.waiters[agent], ch)
		s.waitMu.Unlock()

		recheck := time.NewTimer(WaitSafetyRecheck)
		select {
		case <-ch:
			// woken by a send — loop re-reads the inbox
		case <-recheck.C:
			// safety net: missed signal ≠ stranded waiter — loop re-reads
		case <-deadline.C:
			recheck.Stop()
			s.dropWaiter(agent, ch)
			return false, nil
		case <-ctx.Done():
			recheck.Stop()
			s.dropWaiter(agent, ch)
			return false, ctx.Err()
		}
		recheck.Stop()
		s.dropWaiter(agent, ch)
	}
}

// dropWaiter removes one waiter channel registration (idempotent — the
// channel may already have been consumed and cleared by notifyWaiters).
func (s *SQLiteStore) dropWaiter(agent string, ch chan struct{}) {
	s.waitMu.Lock()
	defer s.waitMu.Unlock()
	list := s.waiters[agent]
	for i, c := range list {
		if c == ch {
			s.waiters[agent] = append(list[:i], list[i+1:]...)
			break
		}
	}
	if len(s.waiters[agent]) == 0 {
		delete(s.waiters, agent)
	}
}
